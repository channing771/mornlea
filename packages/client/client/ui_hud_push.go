package client

// ui_hud_push.go 是游戏相位 HUD 下行的推送纪律层：只裁决「何时下行、下行几
// 次」，不解释任何字段语义——「已确认镜像值 → 语义字段」的换算住在
// `ui_hud_state.go`，由注入的组装函数在冲刷时刻求值。
//
// 纪律三条，与桥 spec「HUD 状态按权威 tick 合并下行」逐句对应：
//
//  1. 同一权威 tick 内的多次状态变化合并为至多一次下行：变化源只调用 `Mark`
//     置脏，脏标记在 tick 边界的 `Flush` 一并消耗，载荷由组装函数此刻从已确认
//     镜像一次性求值，天然携带终态，调用方无需自行合并增量；
//  2. 无变化零推送：无脏标记的 tick 连组装求值都不发生；有脏标记但组装结果与
//     上一次下行逐字节相同时同样不下行（重复或虚假标记不产生空推）；
//  3. 推送不绑定渲染帧：`Flush` 的接线点是权威 tick 边界（应用层每轮排空权威
//     消息后至多一次），呈现帧计数类变化（marker 到期、窗口尺寸变化）只置脏，
//     等下一个冲刷点下行，绝不逐帧无条件推送。
//
// 与既有菜单「变化时推送」的关系：`packages/client/cmd/mornlea/app` 的菜单/设置/暂停相位继续
// 走自己的文本比对下行，本层只覆盖 hud 分节，两侧互不感知、互不回归。

import "encoding/json"

// UIHudSink 是 HUD 下行出口的最小接口。出口收到的是纪律层 marshal 出的 hud
// 分节载荷本体；wire 的 `uiState` 信封必填 `phase`，信封包裹属调用方接线的
// 职责——生产接线的 sink 是「包 `phase` 再交 `Window.PushUIState`」的适配，
// 裸 hud 分节不构成合法下行（前端 `parseState` 必拒）。测试用记录替身实现，
// 纪律层因此能在不创建真实窗口的前提下独立单测。
type UIHudSink interface {
	PushUIState(payload []byte)
}

// UIHudPushScheduler 合并权威 tick 内的 HUD 脏标记并按变化下行。非并发安全：
// 与镜像读、窗口推送同属窗口/tick 线程，不加锁。
type UIHudPushScheduler struct {
	// sink 是下行出口；nil 表示无窗口的无头路径（基准/capture），整个冲刷
	// 退化为空操作，连组装求值都不发生。
	sink UIHudSink
	// assemble 在冲刷时刻从已确认镜像求值整份 hud 分节；只在确要下行前调用，
	// 无变化的 tick 零求值。
	assemble func() UIHudState
	// dirty 是本 tick 内的合并脏标记。下行载体是单份完整 hud 分节 JSON，分节
	// 粒度的脏位不会改变「推或不推」的决策，因此刻意只有一个位；任一变化源
	// 置位即代表「镜像相对上次下行可能已变」。
	dirty bool
	// lastPushed 是上一次成功下行的载荷文本，充当同一会话内「变化」判据的
	// 基线；`Reset` 会清空它，理由见该方法注释。
	lastPushed string
}

// NewUIHudPushScheduler 装配纪律层。`assemble` 必须提供且只读已确认镜像：纪律
// 层不缓存任何 HUD 数值，终态永远来自冲刷时刻的求值。
func NewUIHudPushScheduler(sink UIHudSink, assemble func() UIHudState) *UIHudPushScheduler {
	if assemble == nil {
		panic("client: HUD 推送纪律层缺少组装函数")
	}
	return &UIHudPushScheduler{sink: sink, assemble: assemble}
}

// Mark 记录一次 HUD 状态变化。同一权威 tick 内任意多次调用合并为同一个脏位；
// 标记不携带载荷，终态由 `Flush` 时刻的组装函数给出。权威镜像确认、弹条窗口
// 起止、marker 武装/到期与窗口尺寸变化都是合法的变化源。
func (s *UIHudPushScheduler) Mark() { s.dirty = true }

// Flush 在权威 tick 边界冲刷脏标记：有脏标记才求值组装，组装结果与上一次下行
// 不同才把 hud 分节载荷交给出口，随后脏标记必然清空。载荷是 hud 分节本体的
// JSON，`phase` 信封由出口一侧的接线包裹（见 `UIHudSink`）。返回是否实际下行，
// 供应用层诊断与测试断言；重复调用无变化即空操作。
func (s *UIHudPushScheduler) Flush() bool {
	dirty := s.dirty
	s.dirty = false
	if !dirty || s.sink == nil {
		return false
	}
	payload, err := json.Marshal(s.assemble())
	if err != nil {
		// 组装函数只产出生成器认可的标量与切片（NaN 已在 `NewUIHudEating`
		// 拒绝），失败只可能是绕过组装层的编程错误，按既有窗口操作口径 panic。
		panic("client: HUD 下行状态 JSON 组装失败: " + err.Error())
	}
	text := string(payload)
	if text == s.lastPushed {
		return false
	}
	s.sink.PushUIState(payload)
	s.lastPushed = text
	return true
}

// Reset 表达会话边界（权威 reset、断线、退回菜单）：既丢弃尚未冲刷的脏标记，
// 让旧会话的变化不再驱动下行；也清空已下行基线。后者不可省——前端每次下行都
// 整体替换状态，菜单/暂停相位的载荷会把它的 hud 知识清成缺席，因此回到游戏
// 相位后的第一次冲刷必须无条件下行一份完整 hud 分节；否则逐字节相同的重组装
// 结果（新开局满血、空快捷栏即触发）会被旧基线静默拦截，HUD 持续不呈现，相位
// 进入时刻的 `Mark` 也救不回来。会话边界多推一份载荷的代价可忽略。
func (s *UIHudPushScheduler) Reset() {
	s.dirty = false
	s.lastPushed = ""
}
