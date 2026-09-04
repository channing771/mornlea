package entity

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 被动牛小麦引诱的固定数值契约（边界测试锁定，不随玩家数或被动牛数放大）。
const (
	// passiveTemptRadius 是引诱生效的水平半径（格）：同维最近 `active` 玩家
	// 在此半径内（含边界）且权威选中格握着小麦时，牛转向该玩家。
	passiveTemptRadius = 8
	// passiveTemptStopDistance 是跟随止步的水平距离（格）：进入此距离内（含
	// 边界）即停步，不再向前。
	passiveTemptStopDistance = 2.5
)

// passiveTemptTarget 在同维 `active` 玩家中找最近的手持小麦者，返回其身体位
// 置。扫描形态复用夜行者远离消失的既有模式：`sortedActiveSessions` 的升序确
// 定性、逐个比对维度、距离一律在平方域比较（避免开方）。等距并列时取会话
// `id` 更小者——会话本就升序遍历，先到者只有被严格更近者取代。
//
// 手持判定只读权威背包的选中格（`Hotbar.Slots[Hotbar.Selected]`，与翻地查锄
// 头同先例，按物品名比对 `core.ItemWheat`），不新增任何协议字段、不消耗小
// 麦。每牛每 `tick` 至多遍历一次 `active` 会话，工作有界。
func (engine *engineContext) passiveTemptTarget(entry *passiveState) (mgl32.Vec3, bool) {
	radiusSq := float32(passiveTemptRadius) * float32(passiveTemptRadius)
	var best mgl32.Vec3
	bestSq := radiusSq
	found := false
	for _, id := range engine.sortedActiveSessions() {
		session := engine.sessions[id]
		if session == nil || session.player == nil || session.dimension != entry.dimension {
			continue
		}
		held := session.player.inventory.Hotbar.Slots[session.player.inventory.Hotbar.Selected].Item
		if held != core.ItemWheat {
			continue
		}
		distSq := horizontalDistanceSq(session.player.state.Position, entry.state.Position)
		if distSq > radiusSq {
			continue
		}
		// 半径内只有严格更近者才取代当前最优：等距并列时留下会话 `id` 更小
		// 者（`sortedActiveSessions` 本就升序）。
		if found && distSq >= bestSq {
			continue
		}
		bestSq = distSq
		best = session.player.state.Position
		found = true
	}
	return best, found
}
