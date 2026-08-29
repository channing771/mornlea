//go:build darwin

package app

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/mesh"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/render/hud"
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
			suppressed := a.inventoryOpen || a.menu.phase != MenuPhaseGame || a.menuOverride != nil
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
	if a.inventoryOpen || a.menu.phase != MenuPhaseGame || a.menuOverride != nil {
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
	blockOutline := render.BlockOutline{}
	if !blockTargetReset && !a.clientSessionClosed {
		a.remoteNameTags, blockOutline = a.appendCurrentBlockTarget(a.remoteNameTags)
	}
	avatars, tags := a.remoteAvatars, a.remoteNameTags
	if err := validateEntityPresentationCounts(avatars, tags); err != nil {
		return false, fmt.Errorf("准备实体呈现: %w", err)
	}
	a.blockTargetReset = false
	if a.window != nil && (width != a.frameWidth || height != a.frameHeight) {
		a.renderer.Resize(width, height)
		a.frameWidth, a.frameHeight = width, height
		a.camera.Aspect = float32(width) / float32(height)
	}

	a.scheduler.BeginFrame()
	a.mesher.Schedule(a.mirror, workMax)
	for _, result := range a.mesher.Drain(a.mirror, workMax) {
		if result.Dimension != core.Overworld {
			continue
		}
		a.scheduler.SetConnectivity(result.Pos, result.Conn)
		a.scheduler.QueueSection(result.Pos, result.Quads)
	}
	a.scheduler.FlushUploads(a.center)
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
	// 生命值、氧气、饥饿值和聊天都独立于背包确认状态；未确认时 renderer 只跳过物品布局。
	health, healthReady := a.predictor.Health()
	oxygen, oxygenReady := a.predictor.Oxygen()
	// 饥饿值同生命值与氧气：只取权威确认镜像，客户端不推算也不预测。
	hunger, hungerReady := a.predictor.Hunger()
	saturationZero, _ := a.predictor.SaturationZero()
	chatOverlay := a.ChatOverlay()
	// 弹条检测每帧运行（HUD 隐藏时也要推进确认基线），抑制相位只推进不记录；
	// 组装结果再注入本帧权威 tick 供 HUD 做 40 tick 窗口判定。
	popup := a.updateItemPopup()
	// 准星只在游戏相位（主菜单、设置页、暂停覆盖层与菜单快照覆盖之外）呈现；
	// HUD 段本身仍由 hudVisible 门控。
	crosshair := hud.CrosshairOverlay{
		Visible: a.menu.phase == MenuPhaseGame && a.menuOverride == nil,
	}
	// 容器悬停 tooltip：界面打开时把本帧指针坐标传入渲染层，与点击命中同一
	// 坐标源（`window.CursorPos`）；无头路径 window 为 nil，恒为无效输入，
	// 零实例。
	tooltip := hud.TooltipOverlay{}
	if a.inventoryOpen && a.window != nil {
		cursorX, cursorY := a.window.CursorPos()
		tooltip = hud.TooltipOverlay{Valid: true, CursorX: cursorX, CursorY: cursorY}
	}
	combatMarker := a.combatFeedback.MarkerVisible()
	hudVisible := inventoryConfirmed || (healthReady && !a.clientSessionClosed) ||
		chatOverlay.Open || len(chatOverlay.Lines) != 0 || combatMarker
	if hudVisible {
		// 进食进度条：纯客户端预测。输入位在 `RenderFrame` 作用域没有现成的
		// 当帧 `Control.Eating`，故按 `interactive.go` 置位的同源状态派生（光标
		// 捕获 + 次键按住 + 已确认手持食物 + 权威确认饥饿未满）：开箱/菜单/聊天
		// 都会释放光标，天然归零；唯一偏差是刚刚重新捕获的那一帧会超前一个帧
		// 时长，不可感知。饥饿门控对齐权威侧 `sim/eating.go` 的「饥饿已满不
		// 推进」——满值时输入位恒为假，进度条不出现（spec Scenario「饥饿已满
		// 不呈现进度条」）。tracker 以帧间 elapsed 按权威 tick 周期累积，切格/
		// 换物/数量变化（权威结算吃掉一件）由状态机清零；无头路径（benchmark/
		// capture）window 为 nil，输入位恒为假，既有场景输出逐字节不变。
		eatingSample := client.EatingSample{}
		if hotbar, confirmed := a.inventory.Hotbar(); confirmed {
			stack := hotbar.Slots[hotbar.Selected]
			_, _, food := core.FoodValue(stack.Item)
			eatingSample = client.EatingSample{
				Eating: food && hungerReady && hunger < core.MaxHunger &&
					a.window != nil && a.window.CursorCaptured() &&
					a.window.SecondaryButtonDown(),
				Slot: hotbar.Selected, Item: stack.Item, Count: stack.Count,
			}
		}
		eatingActive, eatingProgress := a.eatingTracker.Observe(time.Now(), eatingSample)
		if err := a.hotbarRenderer.Prepare(
			inventory, inventoryConfirmed, a.inventoryOpen, a.inventorySource, craftingOverlay, overlay, chestOverlay,
			a.miningOverlay,
			hud.EatingOverlay{Active: eatingActive, Progress: eatingProgress},
			hud.HealthOverlay{Confirmed: healthReady, Value: health},
			hud.OxygenOverlay{Confirmed: oxygenReady, Value: oxygen},
			hud.HungerOverlay{Confirmed: hungerReady, Value: hunger, SaturationZero: saturationZero}, chatOverlay, combatMarker,
			popup, crosshair, tooltip,
			uint32(width), uint32(height), a.scheduler.UploadBudget(),
		); err != nil {
			return false, fmt.Errorf("准备快捷栏 HUD: %w", err)
		}
	}
	var panelUISegment []byte
	if a.panel != nil {
		// 面板读数与参数行只构造一次：喂给 layout v3 段（egui 面板）。
		readout, rows := a.panelFrameInput(time.Now())
		panelUISegment = encodeDebugPanelSegment(a.panel.visible, a.panel.editing, readout, rows)
	}
	a.scheduler.DropOutside(a.center, a.render.ViewDistance)
	// 远环半部:跨 tile 边界增量入队 → BeginFrame → FlushUploads →
	// DropOutside(远环半径)。全部非阻塞;禁用时 lodScheduler 为 nil,
	// pumpLodFrame 只做一次 nil 检查即返回。
	a.pumpLodFrame()

	// 每帧只从最后确认的权威世界时间与显示相位偏移计算一次昼夜（云层漂移仍
	// 由绝对时间驱动）;ViewProj 及其逆矩阵同样只计算一次。
	dayNight := render.DayNightAt(a.worldTimeTicks, a.dayPhaseOffset)
	cloud := render.CloudOffsetAt(a.worldTimeTicks)
	viewProj := a.camera.ViewProj()
	viewProjInv := viewProj.Inv()

	// 水下视觉:判定复用 Predictor 最近一次 physics.SubmersionFlags 算出的那一个
	// 眼睛浸没标志——与服务端氧气结算同源,不另起一套(spec fluid-presentation
	// 「视觉与溺水判定一致」)。
	underwater := render.UnderwaterViewFor(a.predictor.EyeInFluid(), baseVisibleRadius)

	// 可见列表:BFS 连通性 + frustum,与旧 Go 渲染器同一算法与顺序。
	// 半径在水下被压低,是"压低远处可见度"的落点。
	a.visibleSections = mesh.VisibleSectionsInto(
		a.visibleSections[:0], &a.visibleScratch,
		cameraSectionPos(a.camera.Pos), underwater.VisibleRadius,
		core.FrustumFrom(viewProj), a.scheduler.Connectivity,
	)
	a.lastFrameStats = a.scheduler.FrameStats(a.visibleSections)
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
	a.outlineStream = a.entityEncoder.EncodeBlockOutlineInstances(a.outlineStream, blockOutline)

	right := mgl32.Vec3{
		float32(math.Cos(float64(a.camera.Yaw))),
		0,
		-float32(math.Sin(float64(a.camera.Yaw))),
	}
	billboard := render.BillboardCamera{
		ViewProj: viewProj,
		Right:    right,
		Up:       right.Cross(a.camera.Forward()).Normalize(),
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
	// UI 段：菜单相位走 layout v1/v2；游戏相位的调试面板走 layout v3，
	// 两种相位互斥，优先级为菜单段优先。
	uiSegment := a.uiSegment()
	if uiSegment == nil {
		uiSegment = panelUISegment
	}
	rendered := a.renderer.RenderFrame(client.RenderFrame{
		ViewProj:         viewProj,
		ViewProjInv:      viewProjInv,
		Pos:              a.camera.Pos,
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
		OverlayStrength:  a.damageStrength,
		WaterTint:        underwater.Tint,
		NameTagSegment:   nameTagSegment,
		HUDSegment:       hudSegment,
		UISegment:        uiSegment,
	})
	a.combatFeedback.AfterRender(rendered)
	if !rendered {
		return false, nil
	}
	return true, nil
}
