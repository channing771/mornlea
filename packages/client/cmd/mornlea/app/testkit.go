//go:build darwin

package app

// testkit.go 收敛跨包测试装配入口。capture/benchmark 域的测试（现居
// `packages/client/cmd/mornlea`，后续迁入各自子包）需要构造携带非导出状态的最小
// `Application`：离屏渲染器、性能录制器替身、关闭链路替身等。这些构造器
// 是测试装配入口而非生产 API，只供本包与客户端各子包的测试使用；生产
// 装配一律走 `New`/`NewWithDependencies`。导出面以「实际被引用」为准，
// 不为对称性补全。

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/server/server"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// IntegrationGlyphSource 是渲染测试共用的字形源替身：固定 8×8 字形并统计
// 请求与落盘次数。FlushErr 非空时 FlushUploads 返回该错误，用于注入字形
// 上传失败路径。
type IntegrationGlyphSource struct {
	FlushErr error
	requests int
	flushes  int
}

func (source *IntegrationGlyphSource) Request(string) { source.requests++ }

func (source *IntegrationGlyphSource) FlushUploads(*render.UploadBudget) error {
	source.flushes++
	return source.FlushErr
}

func (source *IntegrationGlyphSource) Glyph(rune) render.Glyph {
	return render.Glyph{Width: 8, Height: 8}
}

func (source *IntegrationGlyphSource) Kern(rune, rune) float32 { return 0 }

// integrationPlayerID 构造测试用确定性玩家 ID（末字节承载序号）。
func integrationPlayerID(last byte) core.PlayerID {
	return core.PlayerID{0: 0x12, 6: 0x40, 8: 0x80, 15: last}
}

// RemoteSpawn 是测试装配入口：按给定序号、名字与位置构造远端玩家出生消息，
// 供 capture/benchmark 场景与本包渲染测试共同钉住确定性玩家集合。
func RemoteSpawn(id byte, name string, tick uint64, position mgl32.Vec3) network.RemotePlayerSpawn {
	return network.RemotePlayerSpawn{PlayerID: integrationPlayerID(id), DisplayName: name, ServerTick: tick, Dimension: core.Overworld, Position: position}
}

// NewOffscreenRenderApplicationForTest 是测试装配入口：构造带真实离屏 Rust
// 渲染器的最小 `Application`，名牌/HUD 使用 layout-only 变体并注入给定
// GlyphSource。无 GPU 适配器时跳过测试；renderConfig 传 `config.Render{}`
// 即与渲染替身的旧形态一致（零值视距只适用于不触发 DropOutside 的用例）。
func NewOffscreenRenderApplicationForTest(
	t *testing.T,
	glyphs render.GlyphSource,
	width,
	height int,
	renderConfig config.Render,
) *Application {
	t.Helper()
	renderer, err := client.NewRenderer(width, height)
	if errors.Is(err, client.ErrNoGPUAdapter) {
		t.Skip("无 GPU 适配器")
	}
	if err != nil {
		t.Fatal(err)
	}
	reg := assets.NewDefaultRegistry()
	layers, pixels := reg.AtlasPixels()
	renderer.UploadAtlas(layers, pixels)
	application := &Application{
		renderer:        renderer,
		scheduler:       render.NewSectionScheduler(renderer, applicationUploadPerFrame),
		frameWidth:      width,
		frameHeight:     height,
		nameTagRenderer: render.NewNameTagLayouter(glyphs),
		hotbarRenderer:  hud.NewHotbarLayout(glyphs, reg),
		itemDrops:       client.NewItemDrops(),
		remotePlayers:   client.NewRemotePlayers(),
		companions:      &client.Companions{},
		passives:        &client.Passives{},
		// 聊天事件环是 capture 共享清理闭包的必查呈现件（空环即关闭态聊天
		// HUD），离屏装配补齐它，capture 场景闭包才能在本装配上原样运行。
		chatEvents:     &client.ChatEvents{},
		remoteNameTags: make([]render.NameTag, 0, MaxFrameNameTags),
		mirror:         client.NewMirror(),
		predictor:      client.NewPredictor(),
		mesher:         client.NewMesher(reg, 1),
		registry:       reg,
		camera: client.Camera{
			FovY:   mgl32.DegToRad(70),
			Aspect: float32(width) / float32(height),
			Near:   0.1,
			Far:    100,
		},
		loadedChunks: make(map[core.ChunkPos]struct{}),
		render:       renderConfig,
	}
	application.releaseResources = application.releaseOwnedResources
	t.Cleanup(func() { _ = application.Close() })
	return application
}

// NewCloseTrackedApplicationForTest 是测试装配入口：构造不携带任何资源、
// 关闭时只回调 onRelease 的最小 `Application` 替身，供 run 装配序列测试
// 断言构造与关闭的配对顺序。
func NewCloseTrackedApplicationForTest(onRelease func()) *Application {
	return &Application{
		itemDrops:        client.NewItemDrops(),
		releaseResources: onRelease,
	}
}

// NewServerTeardownApplicationForTest 是测试装配入口：构造只携带服务端
// 关闭链路（serverCancel/serverDone）与 onRelease 回调的 `Application`
// 替身，供生命周期测试断言 Close 的等待与错误聚合语义。
func NewServerTeardownApplicationForTest(serverDone chan error, onRelease func()) *Application {
	return &Application{
		serverCancel:     func() {},
		serverDone:       serverDone,
		releaseResources: onRelease,
	}
}

// NewPresentationApplicationForTest 是测试装配入口：构造仅携带客户端呈现
// 状态（远端玩家、伙伴、聊天事件环、掉落物、容器镜像哨兵与隐藏调试面板）
// 的最小 `Application`，供 capture 场景顺序与 AI 伙伴主题测试断言确定性
// 呈现状态。
func NewPresentationApplicationForTest() *Application {
	return &Application{
		remotePlayers: client.NewRemotePlayers(),
		companions:    &client.Companions{},
		chatEvents:    &client.ChatEvents{},
		itemDrops:     client.NewItemDrops(),
		panel:         &panelState{},
	}
}

// connectionTestWindowFactory 返回一个必然失败的窗口工厂：连接装配测试
// 不应走到窗口创建，创建即记数并报错。
func connectionTestWindowFactory(calls *int) func(int, int, string) (Window, error) {
	return func(int, int, string) (Window, error) {
		*calls++
		return nil, errors.New("unexpected window creation")
	}
}

// NewConnectionTestDependencies 是测试装配入口：返回除调用方显式覆盖的
// 能力外、其余依赖被调用即失败（t.Fatalf）的 `Dependencies` 载体，供连接
// 装配测试钉住「哪条路径允许触碰哪个依赖」。
func NewConnectionTestDependencies(t *testing.T) Dependencies {
	t.Helper()
	unexpected := func(name string) {
		t.Helper()
		t.Fatalf("unexpected application dependency call: %s", name)
	}
	return Dependencies{
		OpenStore: func(context.Context, Options) (storage.WorldStore, error) {
			unexpected("OpenStore")
			return nil, nil
		},
		DialTCP: func(context.Context, string) (network.ClientPacketStream, error) {
			unexpected("DialTCP")
			return nil, nil
		},
		LoginClient: func(context.Context, network.ClientPacketStream, network.Identity) (network.ClientEndpoint, uint64, error) {
			unexpected("LoginClient")
			return nil, 0, nil
		},
		NewHost: func(context.Context, server.Config, server.Generator, storage.WorldStore) (Host, error) {
			unexpected("NewHost")
			return nil, nil
		},
		NewMemoryStreamPair: func(int) (network.ClientPacketStream, network.ServerPacketStream, error) {
			unexpected("NewMemoryStreamPair")
			return nil, nil, nil
		},
		NewWindow: connectionTestWindowFactory(new(int)),
	}
}

// LocalConnectionTestOptions 是测试装配入口：返回带固定身份与默认渲染
// 配置的本地连接 `Options`，供连接与 AI 注入主题测试复用同一份起点。
func LocalConnectionTestOptions() Options {
	identity := network.Identity{
		PlayerID:    core.PlayerID{6: 0x40, 8: 0x80, 15: 1},
		DisplayName: "Test Player",
	}
	return Options{
		Seed: 42, WorldPath: "unused", Identity: &identity, Render: config.Defaults().Render,
	}
}

// ConnectionTestEndpoint 是客户端端点计数替身：在真实端点之上统计 Close
// 次数，供断线清理语义测试断言「恰好关闭一次」。
type ConnectionTestEndpoint struct {
	network.ClientEndpoint
	closeCalls atomic.Int32
}

// Close 关闭底层端点并累加计数。
func (endpoint *ConnectionTestEndpoint) Close() error {
	endpoint.closeCalls.Add(1)
	return endpoint.ClientEndpoint.Close()
}

// CloseCalls 返回累计关闭次数。
func (endpoint *ConnectionTestEndpoint) CloseCalls() int {
	return int(endpoint.closeCalls.Load())
}

// ConnectionTestStore 是内存存档替身：在 `storage.MemoryStore` 之上提供
// 可注入的 LoadPlayer 失败与关闭计数。字段仅供本包测试注入。
type ConnectionTestStore struct {
	*storage.MemoryStore
	loadPlayerErr error
	closeCalls    atomic.Int32
}

// NewConnectionTestStore 是测试装配入口：按给定种子构造连接测试用内存存档。
func NewConnectionTestStore(seed int64) *ConnectionTestStore {
	return &ConnectionTestStore{MemoryStore: storage.NewMemory(storage.Metadata{
		FormatVersion: 3,
		Seed:          seed,
	})}
}

func (store *ConnectionTestStore) LoadPlayer(
	ctx context.Context,
	id core.PlayerID,
) (storage.StoredPlayer, error) {
	if store.loadPlayerErr != nil {
		return storage.StoredPlayer{}, store.loadPlayerErr
	}
	return store.MemoryStore.LoadPlayer(ctx, id)
}

func (store *ConnectionTestStore) Close() error {
	store.closeCalls.Add(1)
	return store.MemoryStore.Close()
}
