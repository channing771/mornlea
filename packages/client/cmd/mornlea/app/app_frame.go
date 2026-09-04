//go:build darwin

package app

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
)

// baseVisibleRadius 是相机不在流体时的可见 section 搜索半径（区段），
// 与本变更之前硬编码在 VisibleSectionsInto 调用处的 32 是同一个值。
const baseVisibleRadius = 32

func (a *Application) UpdateCenter() {
	center := CameraChunk(a.camera.Pos)
	if center == a.center {
		return
	}
	a.center = center
	if err := a.requestTrustedObserverCenter(center); err != nil {
		slog.Warn("更新视距中心失败", "error", err)
	}
}

func (a *Application) requestTrustedObserverCenter(center core.ChunkPos) error {
	_, _, sequence, _ := a.server.AppliedTrustedObserverCenter()
	a.observerFloor = sequence
	return a.server.SetTrustedObserverCenter(core.Overworld, center)
}

func (a *Application) nextSequence() uint64 {
	a.sequence++
	return a.sequence
}

// updateItemPopup 检测已确认镜像选中下标的变化并组装本帧弹条输入。
//
// 触发与抑制规则：
//
//   - 只比较 `InventoryMirror` 的已确认选中下标——本地选择请求绝不推进基线，
//     服务端确认到达那一刻才可能触发（「未确认变化不触发」）；
//   - 背包/容器界面打开或菜单相位（含 capture 菜单快照）期间，确认值变化只
//     推进基线、不记录弹条，保证抑制期间的变化不会在相位恢复后延迟出现；
//   - 变化落在无显示名的栏位（空栏位、未注册物品）时清空既有弹条——
//     「均缺省则不显示」。
//
// 检测每帧运行（HUD 隐藏时基线也要跟进）；返回值在此基础上注入当前权威
// tick，40 tick 可见窗口判定由 HUD 布局完成，保持 tick 驱动的确定性。
func (a *Application) updateItemPopup() hud.PopupOverlay {
	if hotbar, confirmed := a.inventory.Hotbar(); confirmed {
		switch {
		case !a.popupSelectionSeen:
			a.popupSelectionSeen = true
			a.popupSelection = hotbar.Selected
		case hotbar.Selected != a.popupSelection:
			a.popupSelection = hotbar.Selected
			suppressed := a.inventoryOpen || a.menu.phase != MenuPhaseGame
			if !suppressed {
				if name, ok := core.ItemDisplayName(hotbar.Slots[hotbar.Selected].Item); ok {
					a.itemPopup = hud.PopupOverlay{Text: name, ShownAtTick: a.serverTick, Valid: true}
				} else {
					a.itemPopup = hud.PopupOverlay{}
				}
			}
		}
	}
	popup := a.itemPopup
	popup.WorldTick = a.serverTick
	// 呈现抑制：界面打开或菜单相位期间一个字形都不产生（delta「容器与菜单
	// 抑制」不只约束变化触发，也约束呈现）；已记录的弹条在相位恢复且仍在
	// 40 tick 窗口内时继续显示剩余时长——抑制是隐藏而非清除。
	if a.inventoryOpen || a.menu.phase != MenuPhaseGame {
		return hud.PopupOverlay{}
	}
	return popup
}

// frame 应用服务端消息后绘制一帧。
func (a *Application) Frame(drainMax, meshWorkMax int, elapsed time.Duration) (bool, error) {
	a.DrainServerMessages(drainMax)
	if a.receiver != nil {
		if err := a.receiver.Err(); err != nil {
			a.CloseClientSession(err)
			return false, err
		}
	}
	health, ready := a.predictor.Health()
	a.damageStrength = a.damageFeedback.update(health, ready, elapsed)
	if a.remotePlayers != nil {
		a.remotePlayers.Advance(elapsed)
	}
	if a.companions != nil {
		a.companions.Advance(elapsed)
	}
	if a.hostiles != nil {
		a.hostiles.Advance(elapsed)
	}
	if a.passives != nil {
		a.passives.Advance(elapsed)
	}
	return a.RenderFrame(meshWorkMax)
}

// RenderFrame 绘制一帧，返回 surface 是否实际取得了可呈现纹理。
func (a *Application) RenderFrame(workMax int) (bool, error) {
	blockTargetReset := a.blockTargetReset
	width, height := a.FramebufferSize()
	if width == 0 || height == 0 {
		return false, nil
	}
	a.remotePresentations = a.remotePlayers.AppendPresentations(a.remotePresentations[:0])
	a.remoteAvatars, a.remoteNameTags = RemoteRenderPresentationsSortedInto(
		a.remoteAvatars[:0],
		a.remoteNameTags[:0],
		a.remotePresentations,
	)
	if a.companions != nil {
		a.companionPresentations = a.companions.AppendPresentations(a.companionPresentations[:0])
		a.remoteAvatars, a.remoteNameTags = AppendCompanionRenderPresentationsInto(
			a.remoteAvatars,
			a.remoteNameTags,
			a.companionPresentations,
		)
	}
	if a.hostiles != nil {
		a.hostilePresentations = a.hostiles.AppendPresentations(a.hostilePresentations[:0])
		a.remoteAvatars = AppendHostileRenderPresentationsInto(
			a.remoteAvatars,
			a.hostilePresentations,
		)
	}
	if a.passives != nil {
		a.passivePresentations = a.passives.AppendPresentations(a.passivePresentations[:0])
		a.remoteAvatars = AppendPassiveRenderPresentationsInto(
			a.remoteAvatars,
			a.passivePresentations,
		)
	}
	blockOutline := render.BlockOutline{}
	if !blockTargetReset && !a.clientSessionClosed {
		a.remoteNameTags, blockOutline = a.appendCurrentBlockTarget(a.remoteNameTags)
	}
	crack := a.deriveBlockCrack(blockOutline)
	avatars, tags := a.remoteAvatars, a.remoteNameTags
	if err := validateEntityPresentationCounts(avatars, tags); err != nil {
		return false, fmt.Errorf("准备实体呈现: %w", err)
	}
	a.blockTargetReset = false
	if a.window != nil && (width != a.frameWidth || height != a.frameHeight) {
		a.renderer.Resize(width, height)
		a.frameWidth, a.frameHeight = width, height
		a.camera.Aspect = float32(width) / float32(height)
		// framebuffer 尺寸随 hud 分节的 viewport 下行:resize 属合法变化源,
		// 置脏等下一个冲刷点下行,绝不逐帧无条件推送。
		a.hudPush.Mark()
	}

	// 菜单全景：主菜单/设置页相位返回惰性构建的全景管线（游戏相位 nil，
	// 与引入全景前逐字节一致）。全景接管本帧的世界内容与相机；游戏调度器
	// 与远环带在其间完全冻结，呈现状态互不渗透。
	vista := a.menuVistaForFrame()
	activeScheduler := a.scheduler
	a.scheduler.BeginFrame()
	if vista != nil {
		activeScheduler = vista.scheduler
		vista.pump(workMax)
	} else {
		a.mesher.Schedule(a.mirror, workMax)
		for _, result := range a.mesher.Drain(a.mirror, workMax) {
			if result.Dimension != core.Overworld {
				continue
			}
			a.scheduler.SetConnectivity(result.Pos, result.Conn)
			a.scheduler.QueueSection(result.Pos, result.Quads)
		}
		a.scheduler.FlushUploads(a.center)
	}
	renderTiming := a.multiplayerRenderTiming
	var renderNow func() time.Time
	var nameTagDuration time.Duration
	if renderTiming != nil {
		renderNow = a.multiplayerRenderNow
		if renderNow == nil {
			renderNow = time.Now
		}
		started := renderNow()
		if err := a.nameTagRenderer.Prepare(tags, a.scheduler.UploadBudget()); err != nil {
			return false, fmt.Errorf("准备世界名牌: %w", err)
		}
		nameTagDuration = renderNow().Sub(started)
	} else if err := a.nameTagRenderer.Prepare(tags, a.scheduler.UploadBudget()); err != nil {
		return false, fmt.Errorf("准备世界名牌: %w", err)
	}
	inventory, inventoryConfirmed := a.inventory.State()
	// 合成视图只画最后确认的权威网格；未确认时传 nil，HUD 按空的个人 2×2
	// 呈现——3×3 工作台视图只在收到尺寸 3 的权威状态后出现，绝不预测。
	var craftingOverlay *hud.CraftingOverlay
	if crafting, confirmed := a.crafting.State(); confirmed {
		craftingOverlay = &hud.CraftingOverlay{
			Size:   crafting.Size,
			Slots:  crafting.Slots,
			Output: crafting.Output,
		}
	}
	var overlay *hud.FurnaceOverlay
	if furnace, opened := a.furnace.State(); opened {
		overlay = &hud.FurnaceOverlay{
			Input:         furnace.Input,
			Fuel:          furnace.Fuel,
			Output:        furnace.Output,
			ProgressTicks: furnace.ProgressTicks,
			BurnTicks:     furnace.BurnTicks,
		}
	}
	var chestOverlay *hud.ChestOverlay
	if chest, opened := a.chest.State(); opened {
		chestOverlay = &hud.ChestOverlay{Items: chest.Items}
	}
	// 生命值、氧气与饥饿值由 hud 分节组装路径直接读取权威确认镜像（见
	// `assembleHUDState`），GPU 保留面不再消费它们。饥饿值例外：进食输入位的
	// 派生需要权威确认的未满判断。
	hunger, hungerReady := a.predictor.Hunger()
	// 弹条检测每帧运行（HUD 隐藏时也要推进确认基线），抑制相位只推进不记录；
	// 组装结果再注入本帧权威 tick 供 40 tick 窗口判定，弹条经 hud 分节下行给
	// WebView 组件呈现。
	a.framePopup = a.updateItemPopup()
	a.markHUDPresentationChanges(hunger, hungerReady)
	// 容器悬停 tooltip：界面打开时把本帧指针坐标传入渲染层，与点击命中同一
	// 坐标源（`window.CursorPos`）；无头路径 window 为 nil，恒为无效输入，
	// 零实例。
	tooltip := hud.TooltipOverlay{}
	if a.inventoryOpen && a.window != nil {
		cursorX, cursorY := a.window.CursorPos()
		tooltip = hud.TooltipOverlay{Valid: true, CursorX: cursorX, CursorY: cursorY}
	}
	// 容器保留面只在「有已装配世界且物品镜像已确认」时准备：常显层已迁
	// WebView，关闭容器界面时 GPU 保留面零实例。
	hudVisible := a.menuHUDVisible() && inventoryConfirmed
	if hudVisible {
		if err := a.hotbarRenderer.Prepare(
			inventory, inventoryConfirmed, a.inventoryOpen, a.inventorySource, craftingOverlay, overlay, chestOverlay,
			tooltip,
			uint32(width), uint32(height), a.scheduler.UploadBudget(),
		); err != nil {
			return false, fmt.Errorf("准备容器保留面 HUD: %w", err)
		}
	}
	// 相位窗口先同步再冲刷：进入游戏相位的当帧就能携带首次 hud 下行（同步是
	// 幂等的边界检测，`pushUIStateIfChanged` 里会再判一次）。hud 分节在权威
	// tick 边界冲刷，脏标记由镜像确认、采掘/进食推进、弹条窗口与 marker 武装/
	// 到期等变化源置位，载荷不变时零下行。
	a.syncHUDPushWindow()
	a.flushHUDState()
	// 菜单层已迁 WebView:每帧一次「状态变化才下行」的 UI 状态推送,替代
	// 旧的帧内 UI 段组装;无窗口(基准/capture)恒为空操作。
	a.pushUIStateIfChanged()
	// 游戏世界的调度半部在全景相位完全冻结：近环视距丢弃与远环增量入队
	// 都以游戏中心为圆心，跑一次就会把全景内容或游戏内容错手释放。
	if vista == nil {
		a.scheduler.DropOutside(a.center, a.render.ViewDistance)
		// 远环半部:跨 tile 边界增量入队 → BeginFrame → FlushUploads →
		// DropOutside(远环半径)。全部非阻塞;禁用时 lodScheduler 为 nil,
		// pumpLodFrame 只做一次 nil 检查即返回。
		a.pumpLodFrame()
	}

	// 每帧只从最后确认的权威世界时间与显示相位偏移计算一次昼夜（云层漂移仍
	// 由绝对时间驱动）;ViewProj 及其逆矩阵同样只计算一次。全景相位钉死在
	// 正午，昼夜呈现不随任何权威时间漂移。
	worldTime, dayPhaseOffset := a.worldTimeTicks, a.dayPhaseOffset
	if vista != nil {
		worldTime, dayPhaseOffset = menuVistaWorldTimeTicks, 0
	}
	dayNight := render.DayNightAt(worldTime, dayPhaseOffset)
	cloud := render.CloudOffsetAt(worldTime)
	cam := &a.camera
	if vista != nil {
		posed := vista.pose(a.camera)
		cam = &posed
	}
	viewProj := cam.ViewProj()
	viewProjInv := viewProj.Inv()

	// 水下视觉:判定复用 Predictor 最近一次 physics.SubmersionFlags 算出的那一个
	// 眼睛浸没标志——与服务端氧气结算同源,不另起一套(spec fluid-presentation
	// 「视觉与溺水判定一致」)。全景相位强制干眼：全景没有权威玩家状态，
	// 前序场景残留的浸没标志不得渗入菜单底图。
	eyeInFluid := a.predictor.EyeInFluid()
	if vista != nil {
		eyeInFluid = false
	}
	underwater := render.UnderwaterViewFor(eyeInFluid, baseVisibleRadius)

	// 可见列表:BFS 连通性 + frustum,与旧 Go 渲染器同一算法与顺序。
	// 半径在水下被压低,是"压低远处可见度"的落点。
	a.visibleSections = mesh.VisibleSectionsInto(
		a.visibleSections[:0], &a.visibleScratch,
		cameraSectionPos(cam.Pos), underwater.VisibleRadius,
		core.FrustumFrom(viewProj), activeScheduler.Connectivity,
	)
	a.lastFrameStats = activeScheduler.FrameStats(a.visibleSections)
	if cap(a.rustVisible) < len(a.visibleSections) {
		a.rustVisible = make([][3]int32, 0, len(a.visibleSections))
	}
	a.rustVisible = a.rustVisible[:0]
	for _, p := range a.visibleSections {
		a.rustVisible = append(a.rustVisible, [3]int32{p.X, p.Y, p.Z})
	}

	var started time.Time
	if renderTiming != nil {
		started = renderNow()
	}
	a.avatarStream = a.entityEncoder.EncodeAvatarInstances(a.avatarStream, avatars)
	if renderTiming != nil {
		renderTiming.recordAvatar(renderNow().Sub(started))
		started = renderNow()
	}
	a.itemDropInstances = appendItemDropInstances(
		a.itemDropInstances[:0], a.itemDrops.Presentations(),
	)
	a.dropStream = a.entityEncoder.EncodeItemDropInstances(a.dropStream, a.serverTick, a.itemDropInstances)
	// 破碎 burst 与掉落物本体共用同一份 serverTick + 掉落物输入,跟踪表在
	// entityEncoder 内跨帧存续;输出并入 avatar 实例段(段预算不足时 burst 让路)。
	a.avatarStream = a.entityEncoder.AppendBreakBurstInstances(a.avatarStream, a.serverTick, a.itemDropInstances)
	a.outlineStream = a.entityEncoder.EncodeBlockOutlineInstances(a.outlineStream, blockOutline)
	a.crackStream = a.entityEncoder.EncodeBlockCrackInstances(a.crackStream, crack)

	right := mgl32.Vec3{
		float32(math.Cos(float64(cam.Yaw))),
		0,
		-float32(math.Sin(float64(cam.Yaw))),
	}
	billboard := render.BillboardCamera{
		ViewProj: viewProj,
		Right:    right,
		Up:       right.Cross(cam.Forward()).Normalize(),
	}
	a.billboardBytes = render.EncodeBillboardCameraBytes(a.billboardBytes, billboard)
	nameTagBackgrounds, nameTagGlyphs := a.nameTagRenderer.FrameStreams()
	nameTagSegment := client.EncodeQuadSegment(
		a.billboardBytes, nameTagBackgrounds, nameTagGlyphs, 64,
	)
	if renderTiming != nil {
		renderTiming.recordNameTag(nameTagDuration + renderNow().Sub(started))
	}
	var hudSegment []byte
	if hudVisible {
		hudViewport, hudQuads, hudGlyphs := a.hotbarRenderer.FrameStreams()
		hudSegment = client.EncodeQuadSegment(hudViewport, hudQuads, hudGlyphs, 48)
	}
	rendered := a.renderer.RenderFrame(client.RenderFrame{
		ViewProj:         viewProj,
		ViewProjInv:      viewProjInv,
		Pos:              cam.Pos,
		Daylight:         dayNight.Daylight,
		SunDirection:     dayNight.SunDirection,
		StarVisibility:   dayNight.StarVisibility,
		SkyColor:         dayNight.ClearColor,
		CloudMacroX:      cloud.MacroX,
		CloudLocal:       cloud.Local,
		Visible:          a.rustVisible,
		AvatarInstances:  a.avatarStream,
		DropInstances:    a.dropStream,
		OutlineInstances: a.outlineStream,
		CrackInstances:   a.crackStream,
		OverlayStrength:  a.damageStrength,
		WaterTint:        underwater.Tint,
		NameTagSegment:   nameTagSegment,
		HUDSegment:       hudSegment,
	})
	if a.combatFeedback.AfterRender(rendered) {
		// marker 到期：显隐由 WebView 组件按 hud 分节下行驱动，置脏等下一个
		// 冲刷点。
		a.hudPush.Mark()
	}
	if vista != nil {
		// 自转时钟在渲染之后推进：本帧画面严格是 pose(tick)，capture 钉住
		// N 的下一帧恰好渲染 pose(N)。
		vista.tick++
	}
	if !rendered {
		return false, nil
	}
	return true, nil
}

// menuHUDVisible 报告当前相位是否允许准备容器保留面 HUD 段：只有游戏与暂停相位
// 有已装配的世界，主菜单/设置页/装配中一律呈现纯全景底图（准星与弹条等常显层
// 已迁 WebView，由 hud 分节驱动组件呈现）。
func (a *Application) menuHUDVisible() bool {
	return a.menu.phase == MenuPhaseGame || a.menu.phase == menuPhasePaused
}

// markHUDPresentationChanges 对只能在帧内推导的 hud 呈现变化置脏：物品名弹条
// 进入/离开 40 tick 窗口，以及进食进度的逐 tick 推进。镜像确认类变化（权威
// 状态、背包、容器、聊天、命中确认）在 `DrainServerMessages` 置脏，不在这里
// 重复。
func (a *Application) markHUDPresentationChanges(hunger uint8, hungerReady bool) {
	a.observeEatingProgress(time.Now(), hunger, hungerReady)
	if text := a.popupPresentationText(); text != a.hudPopupText {
		a.hudPush.Mark()
		a.hudPopupText = text
	}
}

// observeEatingProgress 推进食进度的客户端预测并按变化置脏 hud 分节。输入位
// 按 `interactive.go` 置位的同源状态派生（光标捕获 + 次键按住 + 已确认手持食物
// + 权威确认饥饿未满）：开箱/菜单/聊天都会释放光标，天然归零；唯一偏差是刚刚
// 重新捕获的那一帧会超前一个帧时长，不可感知。饥饿门控对齐权威侧
// `sim/eating.go` 的「饥饿已满不推进」——满值时输入位恒为假。tracker 以帧间
// elapsed 按权威 tick 周期累积，切格/换物/数量变化（权威结算吃掉一件）由状态机
// 清零；无头路径（benchmark/capture）window 为 nil，输入位恒为假。
func (a *Application) observeEatingProgress(now time.Time, hunger uint8, hungerReady bool) {
	sample := client.EatingSample{}
	if hotbar, confirmed := a.inventory.Hotbar(); confirmed {
		stack := hotbar.Slots[hotbar.Selected]
		_, _, food := core.FoodValue(stack.Item)
		sample = client.EatingSample{
			Eating: food && hungerReady && hunger < core.MaxHunger &&
				a.window != nil && a.window.CursorCaptured() &&
				a.window.SecondaryButtonDown(),
			Slot: hotbar.Selected, Item: stack.Item, Count: stack.Count,
		}
	}
	active, progress := a.eatingTracker.Observe(now, sample)
	quantized := quantizeEatingProgress(progress)
	// 置脏只看量化到权威 tick 网格之后的值：连续比例在激活期每帧都不同，逐帧
	// 置脏会让下行频率跟着渲染帧率走，违反「推送绑定权威 tick 边界」；量化后
	// 同一格内的取值稳定，纪律层的逐字节去重把下行收敛回每 tick 至多一次。退出
	// 激活（含量化值回到 0）同样置脏，进度条才会在下行中消失。
	if active != a.hudEatingActive || quantized != a.hudEatingProgress {
		a.hudPush.Mark()
	}
	a.hudEatingActive = active
	a.hudEatingProgress = quantized
}

// quantizeEatingProgress 把进食填充比例量化到权威 tick 网格：分母就是 tracker
// 的累积周期（`client.EatingProgressTicks`），量化对齐前端按 tick 口径推进的
// 动画，也使「同一 tick 内的多次冲刷」拿到逐字节相同的载荷。
func quantizeEatingProgress(progress float32) float32 {
	return float32(math.Round(float64(progress)*client.EatingProgressTicks)) /
		client.EatingProgressTicks
}
