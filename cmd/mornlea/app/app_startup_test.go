//go:build darwin

package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/storage"
)

// `TestNewApplicationCreatesAudioOnlyForWindowedMode` 防止无头 benchmark 或抓帧
// 触碰本机音频设备；普通窗口路径则只构造一个播放器。
func TestNewApplicationCreatesAudioOnlyForWindowedMode(t *testing.T) {
	windowErr := errors.New("stop after audio setup")
	for _, test := range []struct {
		name      string
		options   Options
		configure func(*Dependencies)
		wantCalls int
	}{
		{
			name:    "窗口",
			options: remoteConnectionOptions(),
			configure: func(dependencies *Dependencies) {
				stream := &connectionTestClientStream{}
				endpoint, serverEndpoint := network.NewMemoryPair(1)
				t.Cleanup(func() { _ = serverEndpoint.Close() })
				dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
					return stream, nil
				}
				dependencies.LoginClient = func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error) {
					return endpoint, 0, nil
				}
				dependencies.NewWindow = func(int, int, string) (Window, error) {
					return nil, windowErr
				}
			},
			wantCalls: 1,
		},
		{
			name: "benchmark",
			options: func() Options {
				options := LocalConnectionTestOptions()
				options.Benchmark = true
				return options
			}(),
			configure: func(dependencies *Dependencies) {
				dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
					return NewConnectionTestStore(42), nil
				}
				dependencies.NewOffscreenRenderer = func(int, int) (*client.Renderer, error) {
					return nil, windowErr
				}
			},
		},
		{
			name: "抓帧",
			options: func() Options {
				options := remoteConnectionOptions()
				options.CaptureDir = t.TempDir()
				return options
			}(),
			configure: func(dependencies *Dependencies) {
				stream := &connectionTestClientStream{}
				endpoint, serverEndpoint := network.NewMemoryPair(1)
				t.Cleanup(func() { _ = serverEndpoint.Close() })
				dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
					return stream, nil
				}
				dependencies.LoginClient = func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error) {
					return endpoint, 0, nil
				}
				dependencies.NewOffscreenRenderer = func(int, int) (*client.Renderer, error) {
					return nil, windowErr
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependencies := NewConnectionTestDependencies(t)
			calls, closes := 0, 0
			dependencies.NewAudioPlayer = func(volume float32) (func(audio.Cue), func()) {
				calls++
				if volume != 0 {
					t.Fatalf("audio volume = %v，want 0", volume)
				}
				return func(audio.Cue) {}, func() { closes++ }
			}
			test.configure(&dependencies)

			_, err := NewWithDependencies(test.options, dependencies)
			if !errors.Is(err, windowErr) {
				t.Fatalf("New error = %v，want %v", err, windowErr)
			}
			if calls != test.wantCalls {
				t.Fatalf("audio constructor calls = %d，want %d", calls, test.wantCalls)
			}
			if closes != test.wantCalls {
				t.Fatalf("audio close calls after failed startup = %d，want %d", closes, test.wantCalls)
			}
		})
	}
}

// `TestApplicationCloseClosesAudioOnce` 防止重复 `Close` 重复释放播放器拥有的队列。
func TestApplicationCloseClosesAudioOnce(t *testing.T) {
	closes := 0
	app := &Application{itemDrops: client.NewItemDrops(), closeAudio: func() { closes++ }}
	app.releaseResources = app.releaseOwnedResources

	if err := app.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if closes != 1 {
		t.Fatalf("audio close calls = %d，want 1", closes)
	}
}

func TestNewApplicationReturnsRegistryErrorBeforeClientSideEffects(t *testing.T) {
	want := errors.New("registry failure")
	sideEffectErr := errors.New("unexpected client side effect")
	local := LocalConnectionTestOptions()
	remote := remoteConnectionOptions()
	headless := LocalConnectionTestOptions()
	headless.Benchmark = true
	for _, test := range []struct {
		name      string
		options   Options
		configure func(*Dependencies, func(string))
	}{
		{name: "本地交互", options: local},
		{name: "远程连接", options: remote},
		{
			name:    "benchmark 无头",
			options: headless,
			configure: func(dependencies *Dependencies, called func(string)) {
				dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
					called("OpenStore")
					return storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42}), nil
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := make(map[string]int)
			called := func(name string) { calls[name]++ }
			dependencies := defaultDependencies()
			dependencies.NewRegistry = func(path string) (*assets.Registry, error) {
				if path != "/missing/texture-pack" {
					t.Fatalf("registry path = %q", path)
				}
				return nil, want
			}
			dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
				called("OpenStore")
				return nil, sideEffectErr
			}
			dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
				called("DialTCP")
				return nil, sideEffectErr
			}
			dependencies.LoginClient = func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error) {
				called("LoginClient")
				return nil, 0, sideEffectErr
			}
			dependencies.NewHost = func(context.Context, server.Config, server.Generator, storage.WorldStore) (Host, error) {
				called("NewHost")
				return nil, sideEffectErr
			}
			dependencies.NewMemoryStreamPair = func(int) (network.ClientPacketStream, network.ServerPacketStream, error) {
				called("NewMemoryStreamPair")
				return nil, nil, sideEffectErr
			}
			dependencies.NewWindow = func(int, int, string) (Window, error) {
				called("NewWindow")
				return nil, sideEffectErr
			}
			dependencies.NewWindowedRenderer = func(Window) (*client.Renderer, error) {
				called("NewWindowedRenderer")
				return nil, sideEffectErr
			}
			dependencies.NewOffscreenRenderer = func(int, int) (*client.Renderer, error) {
				called("NewOffscreenRenderer")
				return nil, sideEffectErr
			}
			if test.configure != nil {
				test.configure(&dependencies, called)
			}

			test.options.TexturePackPath = "/raw/missing/texture-pack"
			test.options.ResolvedTexturePackPath = "/missing/texture-pack"
			_, err := NewWithDependencies(test.options, dependencies)
			for _, name := range []string{
				"OpenStore", "DialTCP", "LoginClient", "NewHost", "NewMemoryStreamPair",
				"NewWindow", "NewWindowedRenderer", "NewOffscreenRenderer",
			} {
				if calls[name] != 0 {
					t.Errorf("材质加载失败后 %s calls = %d，want 0", name, calls[name])
				}
			}
			if !errors.Is(err, want) {
				t.Errorf("New error = %v，want %v", err, want)
			}
			if err == nil || !strings.Contains(err.Error(), `加载材质包 "/missing/texture-pack"`) {
				t.Errorf("New error = %q，want path context", err)
			}
		})
	}
}

func TestNewApplicationDefaultRegistryDependencyUsesEmbeddedDefault(t *testing.T) {
	got, err := defaultDependencies().NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	want := assets.NewDefaultRegistry()
	gotLayers, gotPixels := got.AtlasPixels()
	wantLayers, wantPixels := want.AtlasPixels()
	if gotLayers != wantLayers || !bytes.Equal(gotPixels, wantPixels) {
		t.Fatal("空路径没有构造内嵌默认材质注册表")
	}
}

func TestNewApplicationDefaultRegistryDependencyAppliesDirectoryOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "textures"), 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack.json"), []byte(`{"format":1,"name":"startup test"}`), 0o600); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	want := color.NRGBA{R: 17, G: 34, B: 51, A: 68}
	texture := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for offset := 0; offset < len(texture.Pix); offset += 4 {
		copy(texture.Pix[offset:offset+4], []byte{want.R, want.G, want.B, want.A})
	}
	file, err := os.Create(filepath.Join(dir, "textures", "stone.png"))
	if err != nil {
		t.Fatalf("Create texture: %v", err)
	}
	if err := png.Encode(file, texture); err != nil {
		_ = file.Close()
		t.Fatalf("Encode texture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close texture: %v", err)
	}

	got, err := defaultDependencies().NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	pixels := got.LayerRGBA(int(assets.LayerStone))
	for offset := 0; offset < len(pixels); offset += 4 {
		if pixel := (color.NRGBA{R: pixels[offset], G: pixels[offset+1], B: pixels[offset+2], A: pixels[offset+3]}); pixel != want {
			t.Fatalf("stone pixel %d = %+v，want %+v", offset/4, pixel, want)
		}
	}
}
