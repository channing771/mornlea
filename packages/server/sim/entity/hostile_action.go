package entity

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/physics"
)

// validHostileAction 按载荷做防御校验：方向量必须有限且落在 [-1,1]（与玩家/
// 伙伴输入同界），攻击意图必须携带非零目标会话，射击意图必须携带非零有限的
// 瞄准方向。意图来自服务端编排层而非网络，这里仍是防御性校验——非法载荷确
// 定性丢弃，绝不把坏意图喂给权威物理。
func validHostileAction(action HostileAction) bool {
	if !finiteInputComponent(action.MoveX) || !finiteInputComponent(action.MoveZ) {
		return false
	}
	if action.MoveX < -1 || action.MoveX > 1 || action.MoveZ < -1 || action.MoveZ > 1 {
		return false
	}
	if action.AttackTarget && action.TargetSession == 0 {
		return false
	}
	if action.RangedAttack {
		aim := mgl32.Vec3{action.AimX, action.AimY, action.AimZ}
		if !projectileFiniteVec(aim) || aim.LenSqr() == 0 {
			return false
		}
	}
	return true
}

// applyHostileActions 是权威 tick 敌怪阶段的第一步（生成判定之后、统一物理
// 之前）：先把全部敌怪的输入复位为中性（仅保留当前 yaw）、攻击意图清零、
// 射击冷却递减，再按入队顺序取每个 ID 最早的一条合法意图。重复或非法意图确
// 定性丢弃，未知 ID 的意图同样丢弃。移动意图把世界轴方向折算为朝向
// （yaw=0 面 -Z，故 yaw=atan2(-x,-z)）并以前进挡前进，实际位移永远由权威物
// 理决定；攻击意图只冻结（记录目标会话），结算统一推迟到同 tick 稍后的
// advanceCombat；射击意图当场结算（校验 + 散布 + 生成骨刺），见
// settleHostileRangedShot。
//
// 与伙伴输入同款的每 tick 重写语义：无意图的敌怪与未收到任何意图的 tick 都
// 回到中性输入，重力与碰撞照常生效，敌怪在地面保持静止。
func (engine *engineContext) applyHostileActions(actions []HostileAction) {
	for index := range engine.hostiles.entries {
		entry := &engine.hostiles.entries[index]
		entry.input = physics.Input{Yaw: entry.yaw}
		entry.attackIntent = false
		entry.attackTargetSession = 0
		if entry.shootCooldown > 0 {
			entry.shootCooldown--
		}
	}
	if len(actions) == 0 {
		return
	}
	seen := make(map[uint64]struct{}, len(actions))
	for _, action := range actions {
		if _, duplicate := seen[action.ID]; duplicate {
			continue
		}
		if !validHostileAction(action) {
			continue
		}
		index := engine.hostiles.findIndex(action.ID)
		if index < 0 {
			continue
		}
		seen[action.ID] = struct{}{}
		entry := &engine.hostiles.entries[index]
		if action.RangedAttack {
			// 射击意图不携带移动语义：同 tick 的移动/攻击折叠整体跳过，
			// 冷却、存活与 kind 校验在结算点执行。
			engine.settleHostileRangedShot(entry, action)
			continue
		}
		if action.MoveX != 0 || action.MoveZ != 0 {
			yaw := normalizeYaw(float32(math.Atan2(
				float64(-action.MoveX), float64(-action.MoveZ),
			)))
			entry.input = physics.Input{MoveZ: 1, Jump: action.Jump, Yaw: yaw}
			entry.yaw = yaw
		} else {
			entry.input = physics.Input{Yaw: entry.yaw, Jump: action.Jump}
		}
		if action.AttackTarget {
			entry.attackIntent = true
			entry.attackTargetSession = action.TargetSession
		}
	}
}

// settleHostileRangedShot 结算掷骨者的射击意图：仅掷骨者（kind 门禁）、存活
// 个体且射击冷却就绪时接受；在眼位沿「基准方向 + 确定性散布」生成一条骨刺
// （伤害 3、初速 22 格/秒，经 Task 4 的 `spawnProjectile` 唯一入口），并把
// 冷却置满固定周期。基准方向由编排层按目标眼位归一化给出，「同维目标」由
// 编排层在提交前裁决；散布在本结算点按 (worldSeed, 权威 tick, 敌怪 ID) 求值，
// 保证相同输入的重放逐位一致。任一校验不成立都整体丢弃且不消耗冷却。
func (engine *engineContext) settleHostileRangedShot(entry *hostileState, action HostileAction) {
	if entry.kind != HostileKindBoneThrower {
		return
	}
	if entry.health == 0 {
		return
	}
	if entry.shootCooldown != 0 {
		return
	}
	aim := mgl32.Vec3{action.AimX, action.AimY, action.AimZ}
	if !projectileFiniteVec(aim) || aim.LenSqr() == 0 {
		return
	}
	velocity := hostileShardVelocity(engine.seed, engine.tick.Load(), entry.id, aim)
	eye := entry.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	if _, ok := engine.spawnProjectile(
		projectileKindShard, entry.dimension, eye, velocity, entry.id,
		projectileShardDamage,
	); !ok {
		// 生成被拒（防御路径：弹种/维度/有限性都已预校验）时不进入冷却，
		// 下一 tick 的就绪决策可立即重试。
		return
	}
	entry.shootCooldown = hostileShootCooldownTicks
}

// hostileShardVelocity 把基准瞄准方向折算为骨刺初速：方向取基准向量的
// yaw/pitch，加 (worldSeed, tick, 敌怪 ID) 派生的确定性散布（水平与竖直偏移
// 各自独立抽样、绝对值都 ≤ `updates.HostileShotSpreadMaxRadians`，±0.06 rad
// 固定数值契约），再按骨刺初速 22 格/秒重建方向向量。全部运算为纯函数，
// 相同输入的重放逐位一致。
func hostileShardVelocity(seed int64, tick uint64, id uint64, aim mgl32.Vec3) mgl32.Vec3 {
	unit := aim.Normalize()
	yaw := float32(math.Atan2(float64(-unit.X()), float64(-unit.Z())))
	pitch := float32(math.Asin(math.Min(1, math.Max(-1, float64(unit.Y())))))
	yawOffset, pitchOffset := sampler.HostileShotSpread(seed, tick, id)
	yaw = normalizeYaw(yaw + yawOffset)
	const quarterPi = float32(math.Pi / 2)
	pitch = min(quarterPi, max(-quarterPi, pitch+pitchOffset))
	return LookDirection(yaw, pitch).Mul(projectileShardSpeed)
}
