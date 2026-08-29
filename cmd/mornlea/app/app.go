//go:build darwin

package app

import (
	"context"
	"sync"
	"time"

	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/lod"
	"github.com/channing771/mornlea/internal/mesh"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/render/hud"
	"github.com/channing771/mornlea/internal/server"
)

const (
	// maxFrameAvatars 是 App 层每帧身体上限（与 `render.maxAvatars` 的
	// 75 同步）：第 76 具在帧边界被原子拒绝，不触碰 GPU 上传。
	maxFrameAvatars  = 75
	MaxFrameNameTags = 12
)

// BenchmarkSeed 是 benchmark 固定工作负载的世界种子。它是基准身份的一部分：
// benchmark 的测量世界、选项默认值与本包内 benchmark 观察者路径的测试都以
// 它为准，因此随共享常量下沉到本包，capture 与 benchmark 两侧不各自复制。
const BenchmarkSeed = 20260726

type Options struct {
	Companions []companion.Definition
	// AIModel 是从配置 ai 组解析出的模型运行时设置；AI 关闭（无伙伴）时为
	// 零值，本地权威服务端据此保持 M5B 的启动边界（见 server.NewHost）。
	AIModel companion.ModelSettings
	// AIAPIKey 是按 AIModel.APIKeyEnv 从环境变量解析出的密钥值。它只在
	// 进程内存中向本地 server.Config 传递，不写日志、不落配置文件与存档。
	AIAPIKey           string
	Seed               int64
	Benchmark          bool
	BenchmarkTransport string
	WorldPath          string
	Connect            string
	Identity           *network.Identity
	// CaptureDir 非空时进入视觉抓帧模式：走无头设备，按固定场景抓帧写 PNG。
	CaptureDir string
	// Dev 为真时启用调试面板（F3 切换）；不影响配置文件是否生效。
	Dev bool
	// Render 是渲染相关的生效配置（视距、FOV、鼠标灵敏度），由 cmd/mornlea 从
	// 加载后的 config.Config 下传并自行消费——config.Config.Apply 不处理它。
	Render config.Render
	// `AudioVolume` 是本地确认提示音的总音量，只在图形客户端创建播放器时读取。
	AudioVolume float32
	// ConfigPath 是设置页（D-01）的配置保存目标；调试面板不落盘配置（D-03 移除了
	// 面板 F5 保存）。benchmark/capture 不使用。
	ConfigPath string
	// TexturePackPath 是配置文件中的材质包目录原文，设置页必须无损回显与保存。
	TexturePackPath string
	// ResolvedTexturePackPath 是按配置文件目录解析后的启动材质路径；空值只用
	// 内嵌默认材质。当前进程构造 registry 后不再热更新它。
	ResolvedTexturePackPath string
	// WindowSize 是交互式窗口的固定逻辑尺寸预设；benchmark/capture 只携带
	// 配置默认值但不消费它，继续走固定离屏尺寸。
	WindowSize config.WindowSize
	// FluidEnabled 是配置 fluidEnabled 的生效值，下传给本地权威世界的
	// worldgen.New 门控海平面注水。远程连接模式下不使用它——世界内容由
	// 服务端权威决定。
	FluidEnabled bool
	// StartAtMenu 为真时交互客户端启动停留在主菜单，世界装配延迟到点击
	// 「进入游戏」之后（spec egui-tool-ui「交互客户端启动停留在主菜单」）。
	// 其余路径（-connect/benchmark/capture）保持 false，行为与引入菜单前一致。
	StartAtMenu bool
}

type Application struct {
	window Window
	// renderer 是 Rust 渲染器句柄:窗口模式呈现到 surface,无头模式离屏。
	renderer *client.Renderer
	// scheduler 承接 mesh 上传调度(pending/预算/近距优先),下沉给 renderer。
	scheduler   *render.SectionScheduler
	frameWidth  int
	frameHeight int
	// lodScheduler 是远环 LOD 壳的生成/上传调度器;lodEnabled=false 或
	// benchmark 观察者路径下保持 nil(零参与:不建、不消费种子、帧循环只做
	// nil 检查)。生命周期随 Application,Close 时先于渲染器释放。
	lodScheduler *lod.Scheduler
	// lodTileCenter 是最近一次已播种/增量入队的 tile 中心,用于跨 tile 边界
	// 检测;初始值在 attachLodScheduler 的远环带播种(Ruling 19:近环内盘
	// 不入队)时写入。
	lodTileCenter lod.TilePos
	// visibleScratch/visibleSections 是每帧可见性计算的复用缓冲。
	visibleScratch  mesh.VisibilityScratch
	visibleSections []core.SectionPos
	rustVisible     [][3]int32
	avatarStream    []byte
	dropStream      []byte
	outlineStream   []byte
	billboardBytes  []byte
	entityEncoder   render.InstanceEncoder
	lastFrameStats  render.FrameStats
	remotePlayers   *client.RemotePlayers
	companions      *client.Companions
	hostiles        *client.Hostiles
	chatEvents      *client.ChatEvents
	chatInput       chatInput
	// chatEventBuffer 是 refreshChatLines 的复用缓冲，容量与 client.ChatEventCapacity
	// 同源（E9/C9）：事件环最多回放 32 条，缓冲按同一常量分配保证零扩容刷新。
	chatEventBuffer [client.ChatEventCapacity]network.ChatEvent
	// chatLines 的 6 是 HUD 聊天显示行数：openspec companion-client-presentation
	// 规格「HUD 显示最近最多 6 条」，与 internal/render/hud/chat.go 的 maxChatLines 同值。
	chatLines              [6]string
	chatLineCount          int
	formattedChatEventID   uint64
	remotePresentations    []client.RemotePresentation
	companionPresentations []client.CompanionPresentation
	remoteAvatars          []render.Avatar
	hostilePresentations   []client.HostilePresentation
	remoteNameTags         []render.NameTag
	nameTagRenderer        *render.NameTagRenderer
	hotbarRenderer         *hud.HotbarRenderer
	damageFeedback         DamageFeedback
	damageStrength         float32
	// panel 是调试面板的交互状态；只在 Options.Dev 为真时创建。
	panel *panelState
	// panelLastFrameAt 是上一帧调试面板读数的采样时刻，用于计算 PanelReadout.FrameMillis。
	panelLastFrameAt time.Time
	inventory        client.InventoryMirror
	furnace          client.FurnaceMirror
	chest            client.ChestMirror
	// crafting 是权威合成网格的 latest-wins 只读镜像：网格内容、产物与有效
	// 尺寸全部以服务端状态为准，客户端不预测；尺寸 3 表示工作台视图。
	crafting      client.CraftingMirror
	miningOverlay hud.MiningOverlay
	// itemPopup 是已记录的物品名弹条：Text/ShownAtTick/Valid 在已确认镜像的
	// 选中下标变化时记录一次，WorldTick 每帧由 `updateItemPopup` 注入当前权威
	// tick，可见窗口判定留给 HUD 布局（tick 驱动，golden 确定）。
	itemPopup hud.PopupOverlay
	// popupSelection 与 popupSelectionSeen 是弹条触发的确认基线：只消费
	// `InventoryMirror` 的已确认选中下标，本地选择请求不推进基线。
	popupSelection     uint8
	popupSelectionSeen bool
	// eatingTracker 是进食进度的客户端预测状态机：`RenderFrame` 在
	// `Prepare` 调用处按帧间时长推进；纯呈现，不进入权威或预测物理状态。
	eatingTracker     client.EatingProgressTracker
	itemDrops         *client.ItemDrops
	itemDropInstances []render.ItemDrop
	inventoryOpen     bool
	inventorySource   int
	serverTick        uint64
	combatFeedback    combatFeedback
	// worldTimeTicks 是最后确认的权威绝对世界时间，只在接受更新状态时前进。
	// dayPhaseOffset 是同一份状态携带的显示相位偏移（0..23999），与世界时间
	// 同一接受纪律：偏移只平移昼夜呈现，绝不回写绝对时间。
	worldTimeTicks          uint64
	dayPhaseOffset          uint16
	glyphAtlas              *render.GlyphAtlas
	clientEndpoint          network.ClientEndpoint
	receiver                *client.Receiver
	server                  *server.Server
	host                    Host
	serverCancel            context.CancelFunc
	serverDone              chan error
	mirror                  *client.Mirror
	predictor               *client.Predictor
	mesher                  *client.Mesher
	camera                  client.Camera
	center                  core.ChunkPos
	sequence                uint64
	loadedChunks            map[core.ChunkPos]struct{}
	ticks                   *TickRecorder
	saves                   *SaveRecorder
	observerFloor           uint64
	benchmarkTransport      string
	multiplayerRenderTiming *MultiplayerRenderTiming
	multiplayerRenderNow    func() time.Time
	closeOnce               sync.Once
	closeErr                error
	clientCloseOnce         sync.Once
	clientCloseErr          error
	clientSessionClosed     bool
	blockTargetReset        bool
	releaseResources        func()
	// `playCue` 与 `closeAudio` 只拥有本地图形客户端的音频生命周期，不参与权威状态。
	playCue    func(audio.Cue)
	closeAudio func()
	// audioFeedback 只匹配已应用的服务端确认，零值表示尚未收到本会话的基线。
	audioFeedback localAudioFeedback
	// render 是渲染相关的生效配置快照，在构造时从 Options.Render 复制，
	// 供渲染热路径（DropOutside 视距、鼠标灵敏度等）读取，不随配置文件热更新。
	render config.Render
	// startupOptions 是 StartAtMenu 装配路径保存的构造参数快照：NewWithDependencies
	// 在菜单相位跳过世界装配，把 options 与 dependencies 存入 Application，供「进入游戏」
	// 时的 startWorld 复用同一份配置与注入载体。
	startupOptions Options
	startupDeps    Dependencies
	// menu 是主菜单的语义状态（相位/按钮/版本/错误），由 cmd/mornlea 拥有；只有
	// StartAtMenu 路径在构造时初始化为菜单相位，其余路径保持零值（MenuPhaseGame）。
	menu menuState
	// settings 是世界启动前设置页的已保存值与草稿；仅 `MenuPhaseSettings`
	// 接受结构化设置事件，保存成功前不触碰运行态。
	settings SettingsState
	// pause 是暂停覆盖层的开合周期状态（恢复防重入哨兵），与相位机同属 Go 侧
	// 菜单语义；远程形态下除哨兵外不产生任何权威副作用。
	pause pauseState
	// pauseGate 是装配点捕获的可选暂停门：本地形态指向嵌入宿主（或 benchmark
	// 可信服）内持的权威世界，远程 TCP 形态恒为 nil。nil 即「本会话无权威模拟
	// 可冻结」，开层只呈现、不调用任何服务端接口。
	pauseGate applicationPauseGate
	// pushedUIState 是上次下行的 UI 状态 JSON 原文,驱动「状态变化才推送」
	// 的事件驱动语义(见 app_ui_state.go);零值表示尚未推送过任何状态。
	pushedUIState string
}

type Window interface {
	SetCursorCaptured(bool)
	CursorPos() (float64, float64)
	ShouldClose() bool
	Poll()
	DrainTextInput([]rune) ([]rune, bool)
	KeyDown(client.Key) bool
	PrimaryButtonDown() bool
	SecondaryButtonDown() bool
	CursorCaptured() bool
	FramebufferSize() (int, int)
	ContentSize() (int, int)
	SetContentSize(int, int)
	CancelClose()
	Close()
	// Focus 请求把窗口前置并聚焦(交互启动体验:后台启动时窗口偶发不前置)。
	Focus()
	// PushUIState 下行一份菜单层 UI 状态 JSON(client ABI v12);测试桩
	// 记录调用供断言,无头实现可空操作。
	PushUIState([]byte)
}

type Host interface {
	Run(context.Context, network.Listener) error
	AcceptStream(context.Context, network.ServerPacketStream) error
	Shutdown(context.Context) error
}
