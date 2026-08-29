//go:build darwin

package app

// accessors.go 是 `Application` 面向 main（及后续 capture/benchmark 子包）
// 的最小访问面。字段保持非导出；这里只暴露迁移完成后消费方实际读写的
// 成员，语义与原字段访问一致。对称性补全（无人消费的 getter/setter）
// 是被明确禁止的：新增导出前必须先有真实调用方。

import (
	"time"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/lod"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/render/hud"
	"github.com/channing771/mornlea/internal/server"
)

// Camera 返回相机状态的可变指针。capture 场景直接改写偏航/俯仰等单个
// 字段，与旧字段访问同一份内存，不引入拷贝语义。
func (a *Application) Camera() *client.Camera { return &a.camera }

// Inventory 返回背包镜像。消费方在其上调用 Apply/Reset 并读取槽位视图。
func (a *Application) Inventory() *client.InventoryMirror { return &a.inventory }

// Furnace 返回熔炉镜像。
func (a *Application) Furnace() *client.FurnaceMirror { return &a.furnace }

// Chest 返回箱子镜像。
func (a *Application) Chest() *client.ChestMirror { return &a.chest }

// Crafting 返回权威合成网格镜像。
func (a *Application) Crafting() *client.CraftingMirror { return &a.crafting }

// RemotePlayers 返回远端玩家 roster。
func (a *Application) RemotePlayers() *client.RemotePlayers { return a.remotePlayers }

// Mirror 返回权威世界镜像。
func (a *Application) Mirror() *client.Mirror { return a.mirror }

// Mesher 返回网格化 worker 句柄；测试装配路径可以整体替换它。
func (a *Application) Mesher() *client.Mesher { return a.mesher }

// SetMesher 替换网格化 worker 句柄，仅限测试装配。
func (a *Application) SetMesher(mesher *client.Mesher) { a.mesher = mesher }

// SetMirror 替换权威世界镜像，仅限测试装配。
func (a *Application) SetMirror(mirror *client.Mirror) { a.mirror = mirror }

// SetRemotePlayers 替换远端玩家 roster，仅限测试装配预置确定性玩家。
func (a *Application) SetRemotePlayers(remotePlayers *client.RemotePlayers) {
	a.remotePlayers = remotePlayers
}

// SetLODScheduler 注入远环 LOD 调度器（测试可注入空调度器探测接线事实）。
func (a *Application) SetLODScheduler(scheduler *lod.Scheduler) { a.lodScheduler = scheduler }

// SetRender 写入渲染生效配置快照，仅限测试装配。
func (a *Application) SetRender(render config.Render) { a.render = render }

// Renderer 返回 Rust 渲染器句柄（离屏或窗口模式）。
func (a *Application) Renderer() *client.Renderer { return a.renderer }

// Scheduler 返回 mesh 上传调度器。
func (a *Application) Scheduler() *render.SectionScheduler { return a.scheduler }

// Window 返回窗口接口；benchmark 的无头驱动循环以同一接口驱动帧节奏。
func (a *Application) Window() Window { return a.window }

// Server 返回同进程权威服务端（仅 benchmark 观察者形态非 nil）。
func (a *Application) Server() *server.Server { return a.server }

// SetServer 注入同进程权威服务端，仅限 benchmark 场景测试装配。
func (a *Application) SetServer(running *server.Server) { a.server = running }

// Companions 返回伙伴呈现镜像。
func (a *Application) Companions() *client.Companions { return a.companions }

// ChatEvents 返回聊天事件环。
func (a *Application) ChatEvents() *client.ChatEvents { return a.chatEvents }

// ItemDrops 返回掉落物镜像。
func (a *Application) ItemDrops() *client.ItemDrops { return a.itemDrops }

// RemotePresentations 返回远端玩家呈现缓冲。
func (a *Application) RemotePresentations() []client.RemotePresentation {
	return a.remotePresentations
}

// SetRemotePresentations 写入远端玩家呈现缓冲（测试夹具按帧截断/重建）。
func (a *Application) SetRemotePresentations(presentations []client.RemotePresentation) {
	a.remotePresentations = presentations
}

// CompanionPresentations 返回伙伴呈现缓冲。
func (a *Application) CompanionPresentations() []client.CompanionPresentation {
	return a.companionPresentations
}

// SetCompanionPresentations 写入伙伴呈现缓冲。
func (a *Application) SetCompanionPresentations(presentations []client.CompanionPresentation) {
	a.companionPresentations = presentations
}

// RemoteAvatars 返回远端玩家 avatar 批次缓冲。
func (a *Application) RemoteAvatars() []render.Avatar { return a.remoteAvatars }

// SetRemoteAvatars 写入远端玩家 avatar 批次缓冲。
func (a *Application) SetRemoteAvatars(avatars []render.Avatar) { a.remoteAvatars = avatars }

// RemoteNameTags 返回名牌批次缓冲。
func (a *Application) RemoteNameTags() []render.NameTag { return a.remoteNameTags }

// Hostiles 返回夜行者镜像的可变指针，供 capture 场景注入与恢复夹具个体；
// 未装配夜行者镜像时为 nil，调用方需先判空。
func (a *Application) Hostiles() *client.Hostiles {
	return a.hostiles
}

// SetHostiles 整体替换夜行者镜像，仅限测试装配路径使用。
func (a *Application) SetHostiles(hostiles *client.Hostiles) {
	a.hostiles = hostiles
}

// HostilePresentations 返回夜行者派生呈现缓存切片。与其余呈现缓存一致：
// 同帧内复用同一底层数组，消费方只应截断复用，不得持有跨帧引用。
func (a *Application) HostilePresentations() []client.HostilePresentation {
	return a.hostilePresentations
}

// SetHostilePresentations 整体替换夜行者派生呈现缓存，写入方须传入
// 截断复用后的切片。
func (a *Application) SetHostilePresentations(v []client.HostilePresentation) {
	a.hostilePresentations = v
}

// SetRemoteNameTags 写入名牌批次缓冲。
func (a *Application) SetRemoteNameTags(tags []render.NameTag) { a.remoteNameTags = tags }

// ItemDropInstances 返回掉落物呈现实例缓冲。
func (a *Application) ItemDropInstances() []render.ItemDrop { return a.itemDropInstances }

// SetItemDropInstances 写入掉落物呈现实例缓冲。
func (a *Application) SetItemDropInstances(instances []render.ItemDrop) {
	a.itemDropInstances = instances
}

// Predictor 返回输入预测器（当前方块目标的射线来源）。
func (a *Application) Predictor() *client.Predictor { return a.predictor }

// SetPredictor 替换输入预测器，仅限 capture HUD 夹具的换装/恢复。
func (a *Application) SetPredictor(predictor *client.Predictor) { a.predictor = predictor }

// NameTagRenderer 返回名牌批次渲染器。
func (a *Application) NameTagRenderer() *render.NameTagRenderer { return a.nameTagRenderer }

// LODScheduler 返回远环 LOD 调度器；禁用与 benchmark 观察者路径为 nil。
func (a *Application) LODScheduler() *lod.Scheduler { return a.lodScheduler }

// PanelState 是 `panelState` 的导出别名：`Panel()` 的返回类型需要能被
// capture 子包的消费端接口签名命名，而具体结构体保持非导出。别名与方法集
// 零变化，不得当作扩大面板内部访问面的依据。
type PanelState = panelState

// Panel 返回调试面板交互状态；未启用 Dev 时为 nil。
func (a *Application) Panel() *panelState { return a.panel }

// ChatInput 是 `chatInput` 的导出别名：`ChatInput()` 的返回类型需要能被
// capture 子包的消费端接口签名命名，而具体结构体保持非导出。别名与方法集
// 零变化，不得当作扩大聊天输入内部访问面的依据。
type ChatInput = chatInput

// ChatInput 返回聊天输入框状态（可调用 Open/Append/Cancel 等动作）。
func (a *Application) ChatInput() *chatInput { return &a.chatInput }

// Ticks 返回权威 tick 录制器。
func (a *Application) Ticks() *TickRecorder { return a.ticks }

// Saves 返回持久化耗时录制器。
func (a *Application) Saves() *SaveRecorder { return a.saves }

// MultiplayerRenderTiming 返回多人 benchmark 的渲染段延迟记录器（可比较
// 身份以实现 probe 的接管/释放语义）。
func (a *Application) MultiplayerRenderTiming() *MultiplayerRenderTiming {
	return a.multiplayerRenderTiming
}

// SetMultiplayerRenderTiming 接管/释放渲染段延迟记录器；nil 即释放。
func (a *Application) SetMultiplayerRenderTiming(timing *MultiplayerRenderTiming) {
	a.multiplayerRenderTiming = timing
}

// SetMultiplayerRenderNow 注入/清除渲染计时时钟。
func (a *Application) SetMultiplayerRenderNow(now func() time.Time) {
	a.multiplayerRenderNow = now
}

// SetClientEndpoint 替换客户端端点，仅限测试装配。
func (a *Application) SetClientEndpoint(endpoint network.ClientEndpoint) {
	a.clientEndpoint = endpoint
}

// WorldTimeTicks 读取最后确认的权威绝对世界时间。
func (a *Application) WorldTimeTicks() uint64 { return a.worldTimeTicks }

// SetWorldTimeTicks 固定环境光照时间（capture 场景钉住天空状态）。
func (a *Application) SetWorldTimeTicks(ticks uint64) { a.worldTimeTicks = ticks }

// InventoryOpen 读取容器/背包 UI 开合状态。
func (a *Application) InventoryOpen() bool { return a.inventoryOpen }

// SetInventoryOpen 写入容器/背包 UI 开合状态。
func (a *Application) SetInventoryOpen(open bool) { a.inventoryOpen = open }

// InventorySource 读取打开中的容器来源槽（-1 表示未打开）。
func (a *Application) InventorySource() int { return a.inventorySource }

// SetInventorySource 写入打开中的容器来源槽。
func (a *Application) SetInventorySource(source int) { a.inventorySource = source }

// Center 读取相机所在的中心区块。
func (a *Application) Center() core.ChunkPos { return a.center }

// SetCenter 写入相机中心区块（远环场景固定相机时同步）。
func (a *Application) SetCenter(center core.ChunkPos) { a.center = center }

// SetServerTick 固定调试面板读数用的权威 tick（capture 场景钉住读数）。
func (a *Application) SetServerTick(tick uint64) { a.serverTick = tick }

// ResetItemPopupBaseline 把物品名弹条重放回会话起点：清空已记录弹条并丢弃
// 确认选中基线，之后的第一次确认观察只建基线、不触发。交互客户端在会话开始
// 时经 `resetSessionOwnedState` 达成同一语义；无头 capture 场景共用同一个
// application，需要按场景重放该起点，让呈现静态确认状态的场景不把夹具选中
// 误当成一次选中变化（真实调用方：capture 场景 `hud-hotbar-health`、
// `hud-survival-feedback` 与弹条夹具恢复闭包）。
func (a *Application) ResetItemPopupBaseline() {
	a.itemPopup = hud.PopupOverlay{}
	a.popupSelection = 0
	a.popupSelectionSeen = false
}

// BlockTargetReset 读取「本帧隐藏方块目标」的一次性标志。
func (a *Application) BlockTargetReset() bool { return a.blockTargetReset }

// SetBlockTargetReset 写入「本帧隐藏方块目标」的一次性标志。
func (a *Application) SetBlockTargetReset(reset bool) { a.blockTargetReset = reset }

// FormattedChatEventID 读取最近一条已格式化聊天事件的 ID。
func (a *Application) FormattedChatEventID() uint64 { return a.formattedChatEventID }

// SetFormattedChatEventID 写入最近一条已格式化聊天事件的 ID。
func (a *Application) SetFormattedChatEventID(id uint64) { a.formattedChatEventID = id }

// DamageStrength 读取受击遮罩当前强度。
func (a *Application) DamageStrength() float32 { return a.damageStrength }

// SetDamageStrength 写入受击遮罩当前强度。
func (a *Application) SetDamageStrength(strength float32) { a.damageStrength = strength }

// DamageFeedback 返回受击反馈状态机的值拷贝，供确定性断言。
func (a *Application) DamageFeedback() DamageFeedback { return a.damageFeedback }

// SetDamageFeedback 写入受击反馈状态机（capture 夹具固定受伤基线）。
func (a *Application) SetDamageFeedback(feedback DamageFeedback) { a.damageFeedback = feedback }

// MiningOverlay 读取挖掘进度 overlay 快照。
func (a *Application) MiningOverlay() hud.MiningOverlay { return a.miningOverlay }

// SetMiningOverlay 写入挖掘进度 overlay 快照。
func (a *Application) SetMiningOverlay(overlay hud.MiningOverlay) { a.miningOverlay = overlay }

// ChatLines 返回 HUD 聊天行缓冲拷贝。
func (a *Application) ChatLines() [6]string { return a.chatLines }

// SetChatLines 写入 HUD 聊天行缓冲。
func (a *Application) SetChatLines(lines [6]string) { a.chatLines = lines }

// ChatLineCount 读取 HUD 聊天有效行数。
func (a *Application) ChatLineCount() int { return a.chatLineCount }

// SetChatLineCount 写入 HUD 聊天有效行数。
func (a *Application) SetChatLineCount(count int) { a.chatLineCount = count }

// ChatEventBuffer 返回聊天事件复用缓冲拷贝。
func (a *Application) ChatEventBuffer() [client.ChatEventCapacity]network.ChatEvent {
	return a.chatEventBuffer
}

// SetChatEventBuffer 写入聊天事件复用缓冲。
func (a *Application) SetChatEventBuffer(buffer [client.ChatEventCapacity]network.ChatEvent) {
	a.chatEventBuffer = buffer
}

// ObserverFloor 读取 benchmark 观察者的地板高度记录。
func (a *Application) ObserverFloor() uint64 { return a.observerFloor }

// LastFrameStats 读取上一帧的渲染统计。
func (a *Application) LastFrameStats() render.FrameStats { return a.lastFrameStats }

// ClientCloseErr 读取客户端会话关闭时聚合的错误。
func (a *Application) ClientCloseErr() error { return a.clientCloseErr }

// BenchmarkTransport 读取生效的 benchmark 传输名称（memory/tcp）。
func (a *Application) BenchmarkTransport() string { return a.benchmarkTransport }

// Render 读取渲染相关生效配置快照（视距、FOV、鼠标灵敏度）。
func (a *Application) Render() config.Render { return a.render }

// LODTileCenter 读取最近一次已播种的远环 tile 中心。
func (a *Application) LODTileCenter() lod.TilePos { return a.lodTileCenter }

// SetLODTileCenter 写入最近一次已播种的远环 tile 中心（远环场景固定相机后同步）。
func (a *Application) SetLODTileCenter(center lod.TilePos) { a.lodTileCenter = center }

// LoadedChunks 读取已加载区块表（benchmark 测量路径按帧清点/复用）。
func (a *Application) LoadedChunks() map[core.ChunkPos]struct{} { return a.loadedChunks }

// SetPanel 整体替换调试面板状态，仅限测试装配路径使用。
func (a *Application) SetPanel(panel *panelState) {
	a.panel = panel
}

// SetPanelLastFrameAt 复位调试面板读数的采样时刻（capture 场景钉住 PanelReadout）。
func (a *Application) SetPanelLastFrameAt(at time.Time) { a.panelLastFrameAt = at }

// SetMenuOverride 写入一帧菜单快照覆盖（capture 主菜单场景注入；nil 即清除）。
func (a *Application) SetMenuOverride(override *client.UIMenu) { a.menuOverride = override }

// SetSettings 整体写入设置页状态（capture 设置页场景固定 committed/draft）。
func (a *Application) SetSettings(settings SettingsState) { a.settings = settings }

// SetMenuPhase 切换菜单相位（capture 场景在设置页与游戏相位间切换）。
func (a *Application) SetMenuPhase(phase MenuPhase) { a.menu.phase = phase }

// IsOpen 报告聊天输入框是否打开。
func (input *chatInput) IsOpen() bool { return input.open }

// Text 返回聊天输入框的当前文本。
func (input *chatInput) Text() string { return input.text }

// Overflow 报告输入是否已超出字节上限。
func (input *chatInput) Overflow() bool { return input.overflow }

// SetOverflow 强制置位/清除溢出标志，仅限测试装配溢出状态。
func (input *chatInput) SetOverflow(overflow bool) { input.overflow = overflow }

// Visible 报告调试面板是否可见。
func (panel *panelState) Visible() bool { return panel.visible }

// SetVisible 写入调试面板可见性。
func (panel *panelState) SetVisible(visible bool) { panel.visible = visible }
