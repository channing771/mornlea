//go:build darwin

package app

import (
	"fmt"

	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

// viewmodelInstanceBytes 是单 viewmodel 实例的定长字节数：与 `render` 侧
// `avatarInstanceBytes` 同值（跨语言 avatar 实例布局契约）；`render` 未导出
// 该常量，此处保留字面量，漂移由 `TestViewmodelInstanceBytesMatchesEncoderOutput`
// 锁定（编码器真实输出恒为本常量的整数倍）。
const viewmodelInstanceBytes = 96

// deriveViewmodelInput 从已确认镜像派生单帧 viewmodel 编码输入：选中形态
// 只认 `InventoryMirror` 的 `Hotbar` 已确认值（本地选择未确认时调用方无处
// 可取，自然保持旧确认）；挖掘挥动复用本帧裂纹可见性（`deriveBlockCrack`
// 已把选框、采掘 active、裂纹阶段与游戏相位收敛为一比特）；攻击沿直通战斗
// 确认的最后 tick（严格递增才开窗由 `ViewmodelEncoder` 判定，重复与陈旧确
// 认在此原样透传）。
//
// 全景接管（panorama 为真）与会话关闭时返回 nil：前者底图无第一人称双手，
// 后者已无可信镜像；nil 输入不扰动编码器状态，全景期的攻击窗就此冻结，会话
// 重置的清零见 `resetSessionOwnedState`。未确认快捷栏同样返回 nil——首个确
// 认到达前没有可呈现的选中。
//
// 身份直通装配点保留的登录身份：三条真实登录路径全要求 `Identity` 非 nil，
// 双手与远端所见自身身体按同一键着色；无头与测试装配未保留时回落零值，仍确
// 定可重放。
//
// 相机位姿直通本帧呈现相机：根变换由它派生，相机空间偏移经根变换烘焙为世界
// 变换后由既有世界投影绘制；全景相位返回 nil，无需位姿。
func (a *Application) deriveViewmodelInput(panorama bool, crack render.BlockCrack) *render.ViewmodelInput {
	if panorama || a.clientSessionClosed {
		return nil
	}
	// 双手与 HUD 常显层一体：背包/容器打开或切出游戏相位（暂停/菜单）时
	// 无输入，与血条、饥饿、快捷栏同隐同现（门控形状与 `updateItemPopup`
	// 同形）；回到游戏相位且界面关闭后下一帧恢复。HUD 前端本体不在此派生内。
	if a.inventoryOpen || a.menu.phase != MenuPhaseGame {
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
	return &render.ViewmodelInput{
		Player:     player,
		Selected:   hotbar.Slots[hotbar.Selected],
		Tick:       a.serverTick,
		Mining:     crack.Visible,
		AttackTick: a.combatFeedback.lastServerTick,
		CamPos:     a.camera.Pos,
		CamYaw:     a.camera.Yaw,
		CamPitch:   a.camera.Pitch,
	}
}

// ResetViewmodel 丢弃双手编码器的挥动边沿（挖掘锚、攻击窗）：场景切换的公共
// 清场经它调用，旧场景的挥动不得带入新场景首帧；会话重置与权威 reset 直调
// 编码器重置，与本落点同语义。
func (a *Application) ResetViewmodel() { a.viewmodelEncoder.ResetViewmodel() }

// validateViewmodelInstanceCount 校验单帧 viewmodel 实例数恒不超过
// `ViewmodelMaxInstances`：沿 `validateEntityPresentationCounts` 同形，超
// 限或非对齐的帧稳定拒绝（调用方返回 error，不截断绘制）。编码器输出恒至
// 多三实例，本门生产不可达，只防未来装配回归。
func validateViewmodelInstanceCount(stream []byte) error {
	if len(stream)%viewmodelInstanceBytes != 0 {
		return fmt.Errorf("viewmodel stream length %d is not a multiple of %d", len(stream), viewmodelInstanceBytes)
	}
	if count := len(stream) / viewmodelInstanceBytes; count > render.ViewmodelMaxInstances {
		return fmt.Errorf("viewmodel count %d exceeds %d", count, render.ViewmodelMaxInstances)
	}
	return nil
}
