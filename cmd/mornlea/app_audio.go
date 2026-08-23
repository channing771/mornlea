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
	placement       pendingPlacement
}

// pendingPlacement 保存单条已发送放置请求的本地证据；世界写入与扣料必须都确认。
type pendingPlacement struct {
	active    bool
	sequence  uint64
	target    core.BlockPos
	block     core.BlockID
	selected  uint8
	item      core.ItemID
	count     uint8
	blockSeen bool
	itemSeen  bool
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

// BeginPlacement 用已成功发送请求时的本地只读快照替换上一条未完成放置。
func (feedback *localAudioFeedback) BeginPlacement(
	sequence uint64,
	target core.BlockPos,
	block core.BlockID,
	selected uint8,
	stack core.ItemStack,
) {
	feedback.placement = pendingPlacement{
		active: true, sequence: sequence, target: target, block: block, selected: selected,
		item: stack.Item, count: stack.Count,
	}
}

// ClearPlacement 丢弃未完成放置，避免拒绝或会话边界与后续状态错误配对。
func (feedback *localAudioFeedback) ClearPlacement() {
	feedback.placement = pendingPlacement{}
}

// RejectPlacement 仅清除同一条放置请求，避免无关命令拒绝吞掉已到达的一半确认。
func (feedback *localAudioFeedback) RejectPlacement(sequence uint64) {
	if feedback.placement.active && feedback.placement.sequence == sequence {
		feedback.ClearPlacement()
	}
}

// ObservePlacementBlockChanges 只接受精确目标写入预期方块的已应用世界增量。
func (feedback *localAudioFeedback) ObservePlacementBlockChanges(changes network.BlockChanges) (audio.Cue, bool) {
	pending := &feedback.placement
	if !pending.active {
		return 0, false
	}
	for _, change := range changes.Changes {
		if change.Position == pending.target && change.Block == pending.block {
			pending.blockSeen = true
			if pending.itemSeen {
				feedback.ClearPlacement()
				return audio.CueUIClick, true
			}
			return 0, false
		}
	}
	return 0, false
}

// ObservePlacementInventoryState 只接受原选中放置物恰好扣减一件的权威库存。
func (feedback *localAudioFeedback) ObservePlacementInventoryState(state network.InventoryState) (audio.Cue, bool) {
	pending := &feedback.placement
	if !pending.active {
		return 0, false
	}
	stack := state.Inventory.Hotbar.Slots[pending.selected]
	if !itemStackDecreasedByOne(pending.item, pending.count, stack) {
		return 0, false
	}
	pending.itemSeen = true
	if pending.blockSeen {
		feedback.ClearPlacement()
		return audio.CueUIClick, true
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

func itemStackDecreasedByOne(item core.ItemID, count uint8, current core.ItemStack) bool {
	if item == core.ItemNone || count == 0 {
		return false
	}
	if count == 1 {
		return current == (core.ItemStack{})
	}
	return current.Item == item && current.Count == count-1
}

// placementTarget 复现权威放置的命中面邻格规则；无法表达的原点命中不建 pending。
func placementTarget(hit core.RayHit) (core.BlockPos, bool) {
	target := hit.Block
	switch hit.Face {
	case core.BlockFaceNegX:
		target.X--
	case core.BlockFacePosX:
		target.X++
	case core.BlockFaceNegY:
		target.Y--
	case core.BlockFacePosY:
		target.Y++
	case core.BlockFaceNegZ:
		target.Z--
	case core.BlockFacePosZ:
		target.Z++
	default:
		return core.BlockPos{}, false
	}
	return target, target.Y >= core.MinY && target.Y < core.MaxY
}

// playLocalCue 把可选的本地播放器隔离在客户端边界；无声降级保持 nil 即可。
func (a *application) playLocalCue(cue audio.Cue) {
	if a.playCue != nil {
		a.playCue(cue)
	}
}
