package client

import (
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
)

type Movement struct {
	MoveX int8
	MoveZ int8
	Jump  bool
	// Sneaking 是 Shift 潜行的本帧按住态，Sprinting 是双击 W 锁存的本帧疾跑
	// 意图。两者都只是上行意图，真正的速度门控在服务端与预测侧；零值即不行
	// 潜行不疾跑，既有只填方向的构造不受影响。
	Sneaking  bool
	Sprinting bool
}

func MovementFromKeys(w, a, s, d, jump bool) Movement {
	var movement Movement
	if d {
		movement.MoveX++
	}
	if a {
		movement.MoveX--
	}
	if w {
		movement.MoveZ++
	}
	if s {
		movement.MoveZ--
	}
	movement.Jump = jump
	return movement
}

// Actions 是一帧内需要上行的意图。选择只发送请求，
// 客户端不据此改写任何已确认的权威快捷栏状态。
type Actions struct {
	Mining     bool
	Place      bool
	Select     bool
	SelectSlot uint8
	// ToggleInventory 是 E 键的上升沿；界面开关只影响本地输入路由。
	ToggleInventory bool
	// Click 是背包界面打开时的左键上升沿。
	Click bool
	// Drop 是 Q 的有效上升沿：请求丢弃权威选中栏位中的一个物品。
	Drop bool
	// Use 是「使用」键的当前按住状态，与只给上升沿的 Place 是同一个物理键的
	// 两种形态。放置、翻地和开容器都是一次性命令，只能吃上升沿，否则长按会连发；
	// 进食却是**持续输入驱动**的权威动作，服务端要逐 tick 看见「还按着」才推进
	// 进度，松开的那一 tick 立刻清零。两种语义无法共用一位，因此各占一位。
	Use bool
}

type InputState struct {
	primaryDown   bool
	secondaryDown bool
	numberDown    int
	inventoryDown bool
	dropDown      bool
	// wWasHeld 是上一帧 W 的按住态，lastWRelease/wReleaseArmed 记录上次 W
	// 释放时刻与双击窗口有效性，sprintLatched 是双击锁存。时钟由调用方注入，
	// 窗口常量集中在 sprintDoubleTapWindow 一处。
	wWasHeld      bool
	lastWRelease  time.Time
	wReleaseArmed bool
	sprintLatched bool
}

// sprintDoubleTapWindow 是双击 W 判定为疾跑的最大释放→按下间隔。
const sprintDoubleTapWindow = 300 * time.Millisecond

// UpdateSprint 由交互层每帧调用。wHeld 为本帧 W 是否按住，sneakHeld 为 Shift
// 是否按住，uiOpen 为任一界面/聊天/暂停/面板是否打开（打开即清零并清除 armed
// 时刻）。返回本帧是否请求疾跑；真正的速度门控仍在服务端与预测侧。
func (state *InputState) UpdateSprint(wHeld, sneakHeld, uiOpen bool, now time.Time) bool {
	if uiOpen || sneakHeld {
		state.sprintLatched = false
		state.wReleaseArmed = false
		state.wWasHeld = wHeld
		return false
	}
	if wHeld && !state.wWasHeld {
		if state.wReleaseArmed && now.Sub(state.lastWRelease) <= sprintDoubleTapWindow {
			state.sprintLatched = true
		}
		state.wReleaseArmed = false
	}
	if !wHeld && state.wWasHeld {
		state.lastWRelease = now
		state.wReleaseArmed = true
		state.sprintLatched = false
	}
	state.wWasHeld = wHeld
	return state.sprintLatched && wHeld
}

// Update 把数字键 1..9 转换为一次快捷栏选择请求，把 E 与 Q 的上升沿分别转换为
// 界面开关和丢弃请求。inventoryOpen 为 true 时抑制挖掘、放置、快捷栏选择和丢弃，
// 只保留界面点击。
func (state *InputState) Update(
	primary, secondary bool,
	number int,
	inventoryKey, dropKey, inventoryOpen bool,
) Actions {
	rising := primary && !state.primaryDown
	actions := Actions{
		ToggleInventory: inventoryKey && !state.inventoryDown,
		// 界面打开时抑制丢弃，但下方仍记录 Q 的物理状态，
		// 使抑制期间按住的 Q 在恢复后不会被当成新的上升沿。
		Drop: dropKey && !state.dropDown && !inventoryOpen,
	}
	if inventoryOpen {
		actions.Click = rising
	} else {
		actions.Mining = primary
		actions.Place = secondary && !state.secondaryDown
		actions.Use = secondary
		if number >= 1 && number <= core.HotbarSlots && number != state.numberDown {
			actions.Select = true
			actions.SelectSlot = uint8(number - 1)
		}
	}
	state.primaryDown = primary
	state.secondaryDown = secondary
	state.numberDown = number
	state.inventoryDown = inventoryKey
	state.dropDown = dropKey
	return actions
}
