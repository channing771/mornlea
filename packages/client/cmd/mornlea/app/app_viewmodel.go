//go:build darwin

package app

import (
	"fmt"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

// viewmodelInstanceBytes 是单 viewmodel 实例的定长字节数：与 `render` 侧
// `avatarInstanceBytes` 同值（跨语言 avatar 实例布局契约）；`render` 未导出
// 该常量，此处保留字面量，漂移由 `TestViewmodelInstanceBytesMatchesEncoderOutput`
// 锁定（编码器真实输出恒为本常量的整数倍）。
const viewmodelInstanceBytes = 96

// deriveViewmodelInput 只取已确认快捷栏、同源材质与本帧相机，
// 本地动作时钟独立于裂纹和命中反馈；HUD 与第一人称显隐门不变。
func (a *Application) deriveViewmodelInput(panorama bool, _ render.BlockCrack) *render.ViewmodelInput {
	// 第三人称只渲染自身身体（见 `appendSelfAvatar`），双手 viewmodel 仅第
	// 一人称呈现：非第一人称返回 nil，编码流恒为空，帧字节与本字段引入前逐
	// 位一致（静态抓帧的无双手像素回归即系于此）。
	if a.cameraMode != client.CameraFirstPerson {
		return nil
	}
	// 双手与 HUD 常显层一体：背包/容器打开或切出游戏相位（暂停/菜单）时
	// 无输入，与血条、饥饿、快捷栏同隐同现（门控形状与 `updateItemPopup`
	// 同形，落点见 `hudLinkedHidden`，自身身体复用同一门）；回到游戏相位
	// 且界面关闭后下一帧恢复。HUD 前端本体不在此派生内。
	if a.hudLinkedHidden(panorama) {
		return nil
	}
	// 静态抓帧抑制：用户裁决静态画面一律无双手像素，静态 runner 装配后置
	// 位；动作 GIF 经 motion runner 录制，不经此门。
	if a.viewmodelSuppressed {
		return nil
	}
	hotbar, confirmed := a.inventory.Hotbar()
	if !confirmed {
		return nil
	}
	var player core.PlayerID
	if identity := a.startupOptions.Identity; identity != nil {
		player = identity.PlayerID
	}
	active, phase := a.viewmodelMotion.Phase()
	width, height := a.hudLogicalSize()
	return &render.ViewmodelInput{
		SwingActive: active, SwingPhase: phase,
		ViewportWidth:  float32(width),
		ViewportHeight: float32(height),
		FovY:           a.camera.FovY,
		Player:         player,
		Registry:       a.registry,
		Selected:       hotbar.Slots[hotbar.Selected],
		CamPos:         a.camera.Pos,
		CamYaw:         a.camera.Yaw,
		CamPitch:       a.camera.Pitch,
	}
}

// ResetViewmodel 在场景、会话与权威 reset 边界清除本地动作和输入抑制沿，
// 保留编码器的复用容量，旧会话动作不能带入新场景。
func (a *Application) ResetViewmodel() {
	a.viewmodelEncoder.ResetViewmodel()
	a.viewmodelMotion = render.ViewmodelMotion{}
	a.viewmodelPrimaryBlocked = false
}

// AdvanceViewmodel 为无头动作录制消费显式时间和主键，与交互输入共用呈现状态。
func (a *Application) AdvanceViewmodel(elapsed time.Duration, primary bool) {
	hotbar, _ := a.inventory.Hotbar()
	a.viewmodelMotion.Advance(elapsed, primary, render.ViewmodelTierOf(hotbar.Slots[hotbar.Selected]))
}

// validateViewmodelInstanceCount 校验单帧 viewmodel 实例数恒不超过
// `ViewmodelMaxInstances`：沿 `validateEntityPresentationCounts` 同形，超
// 限或非对齐的帧稳定拒绝（调用方返回 error，不截断绘制）。编码器输出恒至
// 多 264 实例，本门生产不可达，只防未来装配回归。
func validateViewmodelInstanceCount(stream []byte) error {
	if len(stream)%viewmodelInstanceBytes != 0 {
		return fmt.Errorf("viewmodel stream length %d is not a multiple of %d", len(stream), viewmodelInstanceBytes)
	}
	if count := len(stream) / viewmodelInstanceBytes; count > render.ViewmodelMaxInstances {
		return fmt.Errorf("viewmodel count %d exceeds %d", count, render.ViewmodelMaxInstances)
	}
	return nil
}
