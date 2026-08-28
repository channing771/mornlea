//go:build darwin

package app

import (
	"context"
	"errors"
	"os"

	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/network"
	networktcp "github.com/channing771/mornlea/internal/network/tcp"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/server"
	"github.com/channing771/mornlea/internal/storage"
)

type Dependencies struct {
	NewRegistry func(string) (*assets.Registry, error)
	OpenStore   func(context.Context, Options) (storage.WorldStore, error)
	DialTCP     func(context.Context, string) (network.ClientPacketStream, error)
	// LoginClient 执行客户端登录并额外返回 LoginSuccess.WorldSeed——远环
	// LOD 的播种种子经它在装配点流入(单机与 TCP 远程共用同一登录路径)。
	LoginClient          func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error)
	NewHost              func(context.Context, server.Config, server.Generator, storage.WorldStore) (Host, error)
	NewMemoryStreamPair  func(int) (network.ClientPacketStream, network.ServerPacketStream, error)
	NewWindow            func(int, int, string) (Window, error)
	NewWindowedRenderer  func(Window) (*client.Renderer, error)
	NewOffscreenRenderer func(int, int) (*client.Renderer, error)
	NewGlyphAtlas        func(render.GlyphSink) (*render.GlyphAtlas, error)
	NewAudioPlayer       func(float32) (play func(audio.Cue), close func())
	// `PatchSettings` 是设置保存事务的可测试边界；生产实现只原子 patch
	// 设置页拥有的三个 raw JSON 顶层成员，不重写同文件的其他字段。返回的
	// `PersistenceResult` 明确标记 rename 是否已提交，防止目录同步警告被误判。
	PatchSettings func(string, config.SettingsPatch) (config.PersistenceResult, error)
}

func defaultDependencies() Dependencies {
	return Dependencies{
		NewRegistry: func(path string) (*assets.Registry, error) {
			if path == "" {
				return assets.NewDefaultRegistry(), nil
			}
			return assets.NewRegistryWithOverride(os.DirFS(path))
		},
		OpenStore:   openApplicationStore,
		DialTCP:     networktcp.DialTCP,
		LoginClient: network.LoginClientWithSeed,
		NewHost: func(
			ctx context.Context,
			config server.Config,
			generator server.Generator,
			store storage.WorldStore,
		) (Host, error) {
			return server.NewHost(ctx, config, generator, store)
		},
		NewMemoryStreamPair: func(capacity int) (
			network.ClientPacketStream,
			network.ServerPacketStream,
			error,
		) {
			clientStream, serverStream := network.NewMemoryStreamPair(capacity)
			return clientStream, serverStream, nil
		},
		NewWindow: func(width, height int, title string) (Window, error) {
			return client.NewWindow(width, height, title)
		},
		NewWindowedRenderer: func(window Window) (*client.Renderer, error) {
			concrete, ok := window.(*client.Window)
			if !ok {
				return nil, errors.New("windowed 渲染器需要真实 client.Window")
			}
			return client.NewWindowedRenderer(concrete)
		},
		NewOffscreenRenderer: client.NewRenderer,
		NewGlyphAtlas:        render.NewGlyphAtlasWithSink,
		NewAudioPlayer: func(volume float32) (func(audio.Cue), func()) {
			player := audio.NewPlayer(volume)
			return player.Play, player.Close
		},
		PatchSettings: config.PatchSettings,
	}
}
