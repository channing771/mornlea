package capture

import (
	"time"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/lod"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/render/hud"
)

// SceneApplication 是 capture 对宿主应用状态所需能力的消费端接口：场景表的
// Prepare/Apply/PinVolatile 闭包与抓帧管线只经由这里声明的方法读写呈现状态，
// 不感知 `application.Application` 的具体类型。`*application.Application`
// 隐式实现本接口；方法集以 capture 的实际引用为准，不为对称性添加方法，
// 也不得反向扩散 app 的内部字段。`Panel`/`ChatInput` 的返回类型经 app 包的
// 导出别名 `PanelState`/`ChatInput` 表达，具体结构体保持非导出。
type SceneApplication interface {
	// 帧驱动与无头渲染契约。
	Frame(drainMax, meshWorkMax int, elapsed time.Duration) (bool, error)
	RenderFrame(workMax int) (bool, error)
	DrainServerMessages(maxMessages int)
	FramebufferSize() (int, int)
	Window() application.Window
	Renderer() *client.Renderer
	Mesher() *client.Mesher
	Scheduler() *render.SectionScheduler
	LODScheduler() *lod.Scheduler
	LoadedChunks() map[core.ChunkPos]struct{}
	Render() config.Render
	LODTileCenter() lod.TilePos

	// 场景直接改写的呈现状态。
	Camera() *client.Camera
	SetWorldTimeTicks(ticks uint64)
	SetCenter(center core.ChunkPos)
	SetServerTick(tick uint64)
	SetPanelLastFrameAt(at time.Time)
	SetBlockTargetReset(reset bool)

	// 镜像与实体呈现面。
	Inventory() *client.InventoryMirror
	Furnace() *client.FurnaceMirror
	Chest() *client.ChestMirror
	Crafting() *client.CraftingMirror
	RemotePlayers() *client.RemotePlayers
	Companions() *client.Companions
	ChatEvents() *client.ChatEvents
	ChatInput() *application.ChatInput
	ItemDrops() *client.ItemDrops
	Mirror() *client.Mirror
	Predictor() *client.Predictor
	SetPredictor(predictor *client.Predictor)
	MiningOverlay() hud.MiningOverlay
	SetMiningOverlay(overlay hud.MiningOverlay)
	SetDamageFeedback(feedback application.DamageFeedback)
	SetDamageStrength(strength float32)
	RemotePresentations() []client.RemotePresentation
	SetRemotePresentations(presentations []client.RemotePresentation)
	CompanionPresentations() []client.CompanionPresentation
	SetCompanionPresentations(presentations []client.CompanionPresentation)
	RemoteAvatars() []render.Avatar
	SetRemoteAvatars(avatars []render.Avatar)
	RemoteNameTags() []render.NameTag
	SetRemoteNameTags(tags []render.NameTag)
	Hostiles() *client.Hostiles
	HostilePresentations() []client.HostilePresentation
	SetHostilePresentations(presentations []client.HostilePresentation)
	ItemDropInstances() []render.ItemDrop
	SetItemDropInstances(instances []render.ItemDrop)
	SetChatEventBuffer(buffer [client.ChatEventCapacity]network.ChatEvent)
	SetChatLines(lines [6]string)
	SetChatLineCount(count int)
	SetFormattedChatEventID(id uint64)
	ArmCombatMarker()
	ResetCombatFeedback()
	CombatMarkerVisible() bool

	// UI 相位与菜单覆盖。
	SetInventoryOpen(open bool)
	SetInventorySource(source int)
	SetMenuOverride(override *client.UIMenu)
	SetMenuPhase(phase application.MenuPhase)
	SetSettings(settings application.SettingsState)
	Panel() *application.PanelState
}
