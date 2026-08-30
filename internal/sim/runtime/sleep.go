package runtime

import (
	"errors"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

// 入睡与跳夜：夜间对床右键使玩家入睡并记录床尾重生点；全员入睡时在 tick 边界
// 设置显示相位偏移跳到白昼。判夜只经 `core.DisplayDayPhase` 读引擎偏移，与
// 夜行者行共享同一份夜间定义（13000..23000）；跳夜不查询任何敌怪状态。

// executeInteractBed 处理对床的右键交互（门 `executeInteractDoor` 先例的床版本）：
// 经权威射线定位目标，命中床时按显示相位判定入睡——夜间窗内置入睡位并把重生点
// 记录为床尾格，窗外拒绝且零状态变化。命中非床方块与门同构为静默成功（no-op）。
// 入睡不消耗任何物品、不写任何方块。
func (engine *Engine) executeInteractBed(command Command) (RejectReason, bool) {
	session := engine.sessions[command.Session]
	if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	dimensionID := session.dimension
	dimension := engine.dimension(dimensionID)
	if dimension == nil || !session.hasView {
		return RejectInvalidRay, true
	}
	origin := session.player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	direction := LookDirection(command.Yaw, command.Pitch)
	hit, ok, err := core.RaycastBlocks(origin, direction, engine.tunables.InteractionReach, blockRaycastSampler(dimension))
	if err != nil {
		if errors.Is(err, ErrChunkNotReady) {
			return RejectChunkNotReady, true
		}
		return RejectInvalidRay, true
	}
	if !ok {
		return RejectNoTarget, true
	}
	block, ready := dimension.BlockAt(hit.Block)
	if !ready {
		return RejectChunkNotReady, true
	}
	if !core.IsBed(block) {
		// 非床目标与门交互同构：静默成功，客户端不等待任何结果。
		return 0, false
	}
	if !core.IsDisplayNightPhase(engine.displayDayPhase()) {
		// 白天用床拒绝且零状态变化。拒绝原因沿用冻结枚举里「命中方块不接受
		// 该交互」的既有语义（翻地/骨粉同款），不为时间窗新增 wire 值。
		return RejectInvalidBlock, true
	}
	foot, _, ok := bedHalfPositions(hit.Block, block)
	if !ok {
		// IsBed 已保证可解析，这里是防御分支：拿到 foot 才写重生点。
		return RejectInvalidBlock, true
	}
	player := session.player
	player.sleeping = true
	// 重生点恒记床尾格：它是 D3「床尾格 + 延迟校验」的记录基准，床头格只在
	// 重生校验时经 `core.BedHeadNeighbor` 重算，不落第二份坐标。
	player.respawnPresent = true
	player.respawnPos = foot
	player.respawnDim = dimensionID
	return 0, false
}

// settleSleepThroughNight 是跳夜的固定结算阶段：当**全部活跃玩家**同时处于入睡
// 状态时，设置显示相位偏移使本 tick 完成后的显示相位落到周期起点（白昼），并
// 清除全部入睡状态。只要有任一活跃玩家未入睡（或没有任何活跃玩家），偏移与
// 入睡状态一律保持不变。
//
// 边界与取舍：
//   - 判定基数是「活跃玩家」（`PlayerActive`）：待重生玩家不在世界里，既不触发
//     也不阻挡跳夜。
//   - 偏移按本 tick 完成后的绝对时间计算（`worldTime + 1`，`advanceWorldTime`
//     尚未执行）：这样跳夜 tick 一结束 `DisplayDayPhase(WorldTimeTicks, offset)`
//     就恰好是 0，与「服务端完成该 tick 后相位为周期起点」的可观察契约对齐。
//   - offset 只进入显示相位，`WorldTimeTicks` 的推进节奏与全部绝对时间消费者
//     （作物/流体/掉落寿命）不受影响；也刻意不查询敌怪状态——跳夜后露天敌怪
//     按其既有白昼规则自然结算。
//   - 工作量为活跃玩家数的两遍线性扫描（判全睡 + 清全醒），O(activePlayers)，
//     无分配、无有界外工作；每 tick 恰好执行一次，位置固定在玩家命令结算之后。
func (engine *Engine) settleSleepThroughNight() {
	sessions := engine.sortedActiveSessions()
	if len(sessions) == 0 {
		return
	}
	for _, id := range sessions {
		if !engine.sessions[id].player.sleeping {
			return
		}
	}
	// 本 tick 完成后的绝对时间：使相位「立即」落在周期起点的是这个值。
	completed := engine.worldTime.Load() + 1
	offset := (core.DayLengthTicks - completed%core.DayLengthTicks) % core.DayLengthTicks
	engine.dayPhaseOffset.Store(uint64(offset))
	// 清全部入睡状态，不限于本 tick 活跃名单：任何残留的入睡位都可能在玩家
	// 重新激活后污染下一次全员判定。
	for _, session := range engine.sessions {
		if session.player != nil {
			session.player.sleeping = false
		}
	}
}

// respawnPositionFromBlock 把床尾格坐标展开为存档用的 float 三元组。三个分量
// 都是整数且在 float32 逐位精确的整数范围内，与 Current/Safe 位置的既有精度
// 约定一致。
func respawnPositionFromBlock(pos core.BlockPos) [3]float32 {
	return [3]float32{float32(pos.X), float32(pos.Y), float32(pos.Z)}
}

// respawnBlockFromPosition 把存档恢复的 float 三元组收拢回床尾格。写入侧保证
// 分量恒为整数；四舍五入（而不是向零截断）使任何尾数误差都回到原格而不是
// 相邻格，与 respawnPositionFromBlock 构成逐格可逆的一对转换。
func respawnBlockFromPosition(position [3]float32) core.BlockPos {
	return core.BlockPos{
		X: int32(math.Round(float64(position[0]))),
		Y: int32(math.Round(float64(position[1]))),
		Z: int32(math.Round(float64(position[2]))),
	}
}

// bedStandHeight 是站在床顶面时的脚底高度（相对床所在格原点）。从 physics 的
// 公开碰撞表读取床形态的碰撞盒顶面，而不是在 sim 里复制 9/16 这一形状常量。
var bedStandHeight = physics.BlockCollisionBoxes(core.BedFootSouthID, true).Boxes[0].Max.Y()

// bedRespawnCandidate 在死亡结算时对个人重生点做延迟校验（「两格仍为同一张
// 床的床尾与床头」才可用）。返回 nil 表示回落世界出生锚点：
//   - 无重生点，或床尾/床头所在区块未就绪——后一种是「无法证明床已失效」：
//     本次死亡先回落锚点（重生不得因等待远处区块而停摆），记录保留给下一次
//     死亡再验；
//   - 床尾不再是床尾形态、床头不再是同方向的床头形态（采掘/半破坏/支撑失效
//     扫除后的残留）——证明床已失效，清除 present 位（D3：延迟校验自然覆盖，
//     无需方块变更反向通知玩家状态）。
//
// 校验通过时返回一个指向床尾格的出生候选（站立在床顶面），经既有的
// restoreCandidate 路径复用区块就绪等待、落点校验与 `activate`，不另写重生
// 位置赋值。校验只读世界，不影响其他玩家的重生点。
func (engine *Engine) bedRespawnCandidate(player *playerState) *restoreCandidate {
	if !player.respawnPresent {
		return nil
	}
	dimension := engine.dimension(player.respawnDim)
	if dimension == nil {
		return nil
	}
	footBlock, footReady := dimension.BlockAt(player.respawnPos)
	if !footReady {
		return nil
	}
	if !core.IsBedFoot(footBlock) {
		player.respawnPresent = false
		return nil
	}
	dir := core.BedDir(footBlock)
	headPos := core.BedHeadNeighbor(player.respawnPos, dir)
	headBlock, headReady := dimension.BlockAt(headPos)
	if !headReady {
		return nil
	}
	// 床头缺失或两格方向编码不一致（同方向床头 = 床尾方向）都判「不属同一
	// 张床」，半破坏与异床拼接在这里一起落网。
	if !core.IsBedHead(headBlock) || core.BedDir(headBlock) != dir {
		player.respawnPresent = false
		return nil
	}
	return &restoreCandidate{location: PlayerLocation{
		Dimension: player.respawnDim,
		Position: mgl32.Vec3{
			float32(player.respawnPos.X) + 0.5,
			float32(player.respawnPos.Y) + bedStandHeight,
			float32(player.respawnPos.Z) + 0.5,
		},
	}}
}
