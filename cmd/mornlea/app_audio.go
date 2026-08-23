//go:build darwin

package main

import (
	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
)

// localAudioFeedback 只保存确认消息之间的极小匹配状态，不参与预测或权威状态。
type localAudioFeedback struct {
	hasHealth       bool
	health          uint8
	hasHunger       bool
	hunger          uint8
	hasMiningTarget bool
	miningTarget    core.BlockPos
	hasStack        bool
	selected        uint8
	stack           core.ItemStack
	foodDecrease    bool
	hungerIncrease  bool
}

// Reset 丢弃会话、重生或权威重置前的全部确认基线。
func (feedback *localAudioFeedback) Reset() {
	*feedback = localAudioFeedback{}
}

// ObservePlayerState 在新鲜且已应用的权威状态上匹配伤害、采掘目标和进食另一半。
func (feedback *localAudioFeedback) ObservePlayerState(state network.PlayerState) (audio.Cue, bool) {
	if state.Reset || !state.Ready {
		feedback.Reset()
		return 0, false
	}
	damaged := feedback.hasHealth && state.Health < feedback.health
	feedback.hasHealth = true
	feedback.health = state.Health
	if state.MiningActive {
		feedback.hasMiningTarget = true
		feedback.miningTarget = state.MiningTarget
	}
	hungerIncrease := feedback.hasHunger && state.Hunger > feedback.hunger
	feedback.hasHunger = true
	feedback.hunger = state.Hunger
	if hungerIncrease {
		if feedback.foodDecrease {
			feedback.clearEating()
			if !damaged {
				return audio.CueEatingComplete, true
			}
		} else {
			feedback.hungerIncrease = true
		}
	} else {
		feedback.foodDecrease = false
	}
	if damaged {
		return audio.CueDamage, true
	}
	return 0, false
}

// ObserveInventoryState 在已验证并镜像的背包状态上匹配选中食物恰减一件。
func (feedback *localAudioFeedback) ObserveInventoryState(state network.InventoryState) (audio.Cue, bool) {
	selected := state.Inventory.Hotbar.Selected
	stack := state.Inventory.Hotbar.Slots[selected]
	if !feedback.hasStack {
		feedback.setStack(selected, stack)
		return 0, false
	}
	if selected != feedback.selected {
		feedback.clearEating()
		feedback.setStack(selected, stack)
		return 0, false
	}
	decreased := foodStackDecreasedByOne(feedback.stack, stack)
	feedback.setStack(selected, stack)
	if feedback.hungerIncrease {
		feedback.hungerIncrease = false
		if decreased {
			feedback.foodDecrease = false
			return audio.CueEatingComplete, true
		}
		return 0, false
	}
	if decreased {
		feedback.foodDecrease = true
	}
	return 0, false
}

// ObserveBlockChanges 只在已应用的方块增量将最近权威采掘目标变为空气时完成一次采掘 cue。
func (feedback *localAudioFeedback) ObserveBlockChanges(changes network.BlockChanges) (audio.Cue, bool) {
	if !feedback.hasMiningTarget {
		return 0, false
	}
	for _, change := range changes.Changes {
		if change.Position == feedback.miningTarget && change.Block == core.AirID {
			feedback.hasMiningTarget = false
			return audio.CueMiningComplete, true
		}
	}
	return 0, false
}

func (feedback *localAudioFeedback) clearEating() {
	feedback.foodDecrease = false
	feedback.hungerIncrease = false
}

func (feedback *localAudioFeedback) setStack(selected uint8, stack core.ItemStack) {
	feedback.hasStack = true
	feedback.selected = selected
	feedback.stack = stack
}

func foodStackDecreasedByOne(previous, current core.ItemStack) bool {
	if _, _, food := core.FoodValue(previous.Item); !food || previous.Count == 0 {
		return false
	}
	if previous.Count == 1 {
		return current == (core.ItemStack{})
	}
	return current.Item == previous.Item && current.Count == previous.Count-1
}

// playLocalCue 把可选的本地播放器隔离在客户端边界；无声降级保持 nil 即可。
func (a *application) playLocalCue(cue audio.Cue) {
	if a.playCue != nil {
		a.playCue(cue)
	}
}
