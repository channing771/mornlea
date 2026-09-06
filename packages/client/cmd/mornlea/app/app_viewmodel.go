//go:build darwin

package app

import (
	"fmt"

	"github.com/channing771/mornlea/packages/client/render"
)

// viewmodelInstanceBytes 是单 viewmodel 实例的定长字节数：`render` 侧
// avatar 实例布局的跨语言契约，`EncodeViewmodelInstances` 输出按此切分；
// 计数门用它把字节流折算为实例数，本文件不复述布局细节。
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
// 身份取零值：本地不渲染自身第三人称身体，手色按同一派生口径由传入身份确
// 定；登录身份当前不保留，零值保证确定可重放，待应用层保留身份后再直通。
func (a *Application) deriveViewmodelInput(panorama bool, crack render.BlockCrack) *render.ViewmodelInput {
	if panorama || a.clientSessionClosed {
		return nil
	}
	hotbar, confirmed := a.inventory.Hotbar()
	if !confirmed {
		return nil
	}
	return &render.ViewmodelInput{
		Selected:   hotbar.Slots[hotbar.Selected],
		Tick:       a.serverTick,
		Mining:     crack.Visible,
		AttackTick: a.combatFeedback.lastServerTick,
	}
}

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
