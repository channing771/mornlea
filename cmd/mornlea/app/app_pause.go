//go:build darwin

package app

import (
	"encoding/binary"
	"errors"
	"log/slog"
)

// applicationPauseGate 是暂停控制的可选能力接口：装配点对拿到手的宿主（或
// benchmark 同进程可信服）做类型断言，有能力就捕获成门、没有就是 nil。
// 远程 TCP 形态下 Application 没有任何本地权威世界，门必须保持 nil——此时
// 暂停层仅是本地呈现覆盖层，绝不调用任何服务端接口。
type applicationPauseGate interface {
	Pause()
	Resume()
}

// applicationPauseGateOf 把任意受控对象收敛为可选暂停门：非实现者安全返回
// nil，调用方以 nil 判定「本形态无权威模拟可冻结」。
func applicationPauseGateOf(controlled any) applicationPauseGate {
	gate, _ := controlled.(applicationPauseGate)
	return gate
}

// uiPauseLayoutVersion 是暂停页 UI 下行段的内部布局版本，与 Rust 侧
// engine/crates/mornlea_client/src/ui.rs 的 UI_PAUSE_LAYOUT_VERSION 同值互钉；
// 既有主菜单 1 / 设置页 2 / 调试面板 3 的常量与线格式不动。任何一侧不得单方面
// 改动数字。
const uiPauseLayoutVersion = 4

// uiPauseFlagVisible 是 layout v4 段内 visible 布尔的置位值；段只在暂停相位
// 编码，因此可见位恒为真。
const uiPauseFlagVisible = 1

// encodeUIPauseSegment 构造一帧暂停页段字节（小端）：版本号之后仅两个 u32
// 布尔——可见位与远程位，与 Rust decode_pause_frame 逐值对应。remote 为真表示
// 本会话经远程连接，页面将注明「世界不会停止」；此时不存在可调用的本地暂停门，
// 该标志只是呈现语义，不改变服务端推进。段无变长字段，恒 12 字节。
func encodeUIPauseSegment(remote bool) []byte {
	out := make([]byte, 0, 12)
	out = binary.LittleEndian.AppendUint32(out, uiPauseLayoutVersion)
	out = binary.LittleEndian.AppendUint32(out, uiPauseFlagVisible)
	if remote {
		out = binary.LittleEndian.AppendUint32(out, 1)
	} else {
		out = binary.LittleEndian.AppendUint32(out, 0)
	}
	return out
}

// pauseState 承载一个开合周期内的恢复防重入哨兵。打开覆盖层时武装；Esc 键位
// 边沿与「返回游戏」按钮点击可能同帧相继到达（Escape 键事件由 Go 键位栈裁决，
// Rust 侧不合成返回动作），哨兵保证只有第一条真正触发恢复，其余全部按幂等
// 忽略——即规格「重复触发 MUST 只生效一次」。
type pauseState struct {
	resumable bool
}

// arm 在打开覆盖层时武装哨兵；重复打开由相位防线拦截，不会二次武装同一个
// 开合周期内的状态。
func (state *pauseState) arm() {
	state.resumable = true
}

// takeResume 消费哨兵并报告本次调用是否为有效恢复触发。
func (state *pauseState) takeResume() bool {
	if !state.resumable {
		return false
	}
	state.resumable = false
	return true
}

// pauseVisible 报告暂停覆盖层当前是否占据交互栈顶。它与背包/面板同属「界面
// 打开」一族：期间游戏输入整体静音（移动/动作/准星回传一律中性）、点击不重捕
// 光标、Enter 不新开聊天（spec「暂停期不新开聊天」）、F3 不叠加面板。
func (a *Application) pauseVisible() bool {
	return a.menu.phase == menuPhasePaused
}

// openPauseOverlay 打开暂停覆盖层并释放光标；本地形态经捕获的暂停门冻结嵌入
// 权威模拟，远程形态只呈现不宣称。幂等：已在暂停相位时重复到达的打开事件被
// 忽略，门的 Pause 至多每开合周期调用一次。
func (a *Application) openPauseOverlay() {
	if a.menu.phase == menuPhasePaused {
		return
	}
	a.pause.arm()
	if a.pauseGate != nil {
		a.pauseGate.Pause()
	}
	a.menu.phase = menuPhasePaused
	if a.window != nil {
		a.window.SetCursorCaptured(false)
	}
}

// closePauseOverlay 关闭暂停覆盖层回到游戏相位：解除冻结、重新捕获光标。
// 防重入哨兵使 Esc 边沿与重复动作事件在同帧多次到达时只生效一次。
func (a *Application) closePauseOverlay() {
	if a.menu.phase != menuPhasePaused || !a.pause.takeResume() {
		return
	}
	if a.pauseGate != nil {
		a.pauseGate.Resume()
	}
	a.menu.phase = MenuPhaseGame
	if a.window != nil {
		a.window.SetCursorCaptured(true)
	}
}

// quitToMenuFromPause 从暂停相位退回主菜单：先解除可能存在的冻结（远程形态
// 门为 nil 时跳过），再复用既有会话拆链语义收摊——`resetSessionOwnedState`
// 清空全部会话态镜像，`releaseWorldConnection` 按 startWorld 失败清理同序关闭
// 接收器、取消 Host 运行上下文并等待持久化 Shutdown 完成。字段复位后主菜单
// 的「进入游戏」可以全新装配；再次进入用新镜像，绝无上一世界的残留状态。
//
// 幂等边界：只有暂停相位能发起拆链，拆链后的后续事件按菜单语义路由或忽略，
// 二次到达的退出动作不可能重放半途状态。
func (a *Application) quitToMenuFromPause() {
	if a.menu.phase != menuPhasePaused {
		return
	}
	a.menu.phase = MenuPhaseMenu
	if a.pauseGate != nil {
		a.pauseGate.Resume()
	}
	a.resetSessionOwnedState()
	// 关停超时取与延迟装配同一配置来源：两条路径共享 DefaultConfig 的同一份
	// ShutdownTimeout，避免第三处字面量漂移。
	shutdownTimeout := buildApplicationServerConfig(
		a.startupOptions, a.ticks, a.saves,
	).ShutdownTimeout
	if err := a.releaseWorldConnection(shutdownTimeout); err != nil {
		slog.Warn("退回主菜单释放世界连接失败", "error", err)
	}
}

// errRemoteStartWorldRejected 是 -connect 形态迟回主菜单后再点「进入游戏」的
// 防御性拒绝文案。该形态的世界由远端持有，进程内既无存档也无 Host 可装配；
// 若不做这道闸，nil 存档会在 server.NewHost 直接 panic 而不是给出可读提示。
var errRemoteStartWorldRejected = errors.New("远程连接形态不支持本地世界装配")
