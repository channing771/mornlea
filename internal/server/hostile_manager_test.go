package server

import (
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/pathfind"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/world"
)

// 本文件锁定有界追逐编排（hostileManager）的 tick 边界契约：目标选择（最近
// active 同维 live 玩家、等距按 PlayerID 字节序）、每 tick 至多 2 份不可变路
// 径快照（ID 最小且到期，其余顺延）、快照覆盖 33×9×33 窗口与 chunk
// revisions、满槽非阻塞投递、结果按 ID 序应用且过期丢弃、权威 tick 绝不等
// 待 A*、终点超窗钳到朝玩家方向的窗缘可站立格、waypoint 提交前重验、失效清
// path 并把重规划排到下一 tick、到攻击距离停移并冻结一次攻击意图、无路径绝
// 不穿墙直线移动。

// chaseNightTicks 是夹具世界时间的夜间相位代表值：后续推进全部落在夜间窗口
// 内，灼烧与生成判定不干扰追逐断言（生成候选落在未加载区块被拒）。
const chaseNightTicks = uint64(15000)

// chasePlayerID 派生一个合法且以 seed 主导字节序的玩家标识（version/variant
// 位就位），供等距裁决的确定性断言使用。
func chasePlayerID(seed byte) core.PlayerID {
	var id core.PlayerID
	for index := range id {
		id[index] = seed + byte(index)
	}
	id[6] = 0x40
	id[8] = 0x80
	return id
}

// chaseTarget 构造一条注入编排层的在线玩家事实。
func chaseTarget(seed byte, session contract.SessionID, position [3]float32) hostileTargetPlayer {
	return hostileTargetPlayer{
		id:        chasePlayerID(seed),
		session:   session,
		dimension: core.Overworld,
		position:  position,
	}
}

// chaseMob 构造一只可通过恢复校验的夜行者记录基线。
func chaseMob(id uint64, position mgl32.Vec3) contract.HostileMob {
	return contract.HostileMob{
		ID:           id,
		Dimension:    core.Overworld,
		State:        physics.State{Position: position, OnGround: true},
		Health:       core.MaxHealth,
		BurnCooldown: 20,
	}
}

// restoreChaseHostile 把一只夜行者恢复进引擎集合。
func restoreChaseHostile(t *testing.T, engine *sim.Engine, mob contract.HostileMob) {
	t.Helper()
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者 %d：%v", mob.ID, err)
	}
}

// chaseFlatChunk 构造 y=0 一层草地的 flat 区块。
func chaseFlatChunk(pos core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(pos)
	for x := 0; x < core.SectionSize; x++ {
		for z := 0; z < core.SectionSize; z++ {
			chunk.SetBlock(x, 0, z, core.GrassID)
		}
	}
	return chunk
}

// chaseBoxedChunk 在原点区块 (2,2) 列周围造一间封顶石盒：盒内夜行者的任何
// A* 展开都被实体方块阻挡，寻路必然不可达。
func chaseBoxedChunk(pos core.ChunkPos) *world.Chunk {
	chunk := chaseFlatChunk(pos)
	if pos != (core.ChunkPos{}) {
		return chunk
	}
	for y := int32(1); y <= 2; y++ {
		for x := int32(1); x <= 3; x++ {
			for z := int32(1); z <= 3; z++ {
				if x == 2 && z == 2 {
					continue
				}
				chunk.SetBlock(int(x), y, int(z), core.StoneID)
			}
		}
	}
	for x := int32(1); x <= 3; x++ {
		for z := int32(1); z <= 3; z++ {
			chunk.SetBlock(int(x), 3, int(z), core.StoneID)
		}
	}
	return chunk
}

// newHostileChaseWorld 构造夜间相位、±2 区块 flat 世界已装载的引擎与裸
// manager；返回 setTargets 供用例注入在线玩家事实（与真实会话解耦，攻击
// 用例单独传入真实会话 ID）。chunkFactory 允许用例替换区块形状。
func newHostileChaseWorld(
	t *testing.T,
	chunkFactory func(core.ChunkPos) *world.Chunk,
) (*sim.Engine, *hostileManager, func([]hostileTargetPlayer)) {
	t.Helper()
	if chunkFactory == nil {
		chunkFactory = chaseFlatChunk
	}
	engine := sim.NewEngine(2, chaseNightTicks, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	for range 40 {
		result := engine.Step()
		for _, key := range result.Acquire {
			engine.SubmitAcquired(contract.AcquiredChunk{Key: key, Missing: true})
		}
		for _, key := range result.Generate {
			engine.SubmitGenerated(contract.GeneratedChunk{
				Dimension: core.Overworld,
				Pos:       key.Pos,
				Chunk:     chunkFactory(key.Pos),
			})
		}
		if len(result.Acquire) == 0 && len(result.Generate) == 0 {
			break
		}
	}
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			info, ok := engine.ChunkInfo(core.ChunkKey{
				Dimension: core.Overworld,
				Pos:       core.ChunkPos{X: dx, Z: dz},
			})
			if !ok || info.State != contract.ChunkReady {
				t.Fatalf("夹具世界区块 (%d,%d) 未就绪", dx, dz)
			}
		}
	}
	manager := newHostileManager(engine)
	var targets []hostileTargetPlayer
	manager.onlinePlayers = func() []hostileTargetPlayer { return targets }
	t.Cleanup(manager.close)
	return engine, manager, func(list []hostileTargetPlayer) { targets = list }
}

// waitForChaseResults 等待 worker 把 count 份结果送回 channel（flat 世界的
// A* 在毫秒内完成；超时视为 worker 纪律破坏）。
func waitForChaseResults(t *testing.T, manager *hostileManager, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(manager.results) >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("超时等待寻路结果：想要 %d 份，实得 %d", count, len(manager.results))
}

// chaseSlot 返回某只夜行者的编排槽位（缺失即失败，调用点都假定已建档）。
func chaseSlot(t *testing.T, manager *hostileManager, id uint64) *hostileChaseSlot {
	t.Helper()
	slot := manager.slots[id]
	if slot == nil {
		t.Fatalf("夜行者 %d 缺少编排槽位", id)
	}
	return slot
}

func TestHostileChaseSelectsNearestSameDimensionLivePlayer(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{
		chaseTarget(0x01, 1, [3]float32{20.5, 1, 0.5}), // 距 20
		chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5}), // 距 10
	})
	manager.advance()
	if got := engine.HostileMobs()[0].PlayerID; got != chasePlayerID(0x02) {
		t.Fatalf("目标=%v，想要距离更近的 %v", got, chasePlayerID(0x02))
	}
}

func TestHostileChaseTieBreaksEquidistantByPlayerIDBytes(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	// 与夜行者等距（各 10 格）：字节序较小者（seed 0x03）必须胜出。
	setTargets([]hostileTargetPlayer{
		chaseTarget(0x05, 1, [3]float32{10.5, 1, 0.5}),
		chaseTarget(0x03, 1, [3]float32{-9.5, 1, 0.5}),
	})
	manager.advance()
	if got := engine.HostileMobs()[0].PlayerID; got != chasePlayerID(0x03) {
		t.Fatalf("等距目标=%v，想要字节序较小的 %v", got, chasePlayerID(0x03))
	}
}

func TestHostileChaseIgnoresOtherDimensionAndClearsStaleTarget(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	mob := chaseMob(11, mgl32.Vec3{0.5, 1, 0.5})
	mob.HasTarget = true
	mob.PlayerID = chasePlayerID(0x07)
	restoreChaseHostile(t, engine, mob)
	// 唯一候选在其它维度：不可选；旧目标事实必须被清除并排期下一 tick 重选。
	foreign := chaseTarget(0x07, 1, [3]float32{10.5, 1, 0.5})
	foreign.dimension = core.DimensionID(9)
	setTargets([]hostileTargetPlayer{foreign})
	now := engine.WorldTime()
	manager.advance()
	updated := engine.HostileMobs()[0]
	if updated.HasTarget {
		t.Fatal("其它维度的玩家被选为目标")
	}
	if updated.PlayerID != (core.PlayerID{}) {
		t.Fatalf("清除目标后玩家 ID=%v，想要零值", updated.PlayerID)
	}
	if updated.NextRepathTicks != now+1 {
		t.Fatalf("无目标重规划 tick=%d，想要下一 tick %d", updated.NextRepathTicks, now+1)
	}
}

func TestHostileChaseBuildsAtMostTwoSnapshotsSmallestIDsFirst(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	for _, id := range []uint64{9, 5, 7} { // 故意乱序恢复
		restoreChaseHostile(t, engine, chaseMob(id, mgl32.Vec3{0.5, 1, float32(id)}))
	}
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	manager.advance()
	if !chaseSlot(t, manager, 5).pathInFlight {
		t.Fatal("ID 最小的到期夜行者 5 未被派发")
	}
	if !chaseSlot(t, manager, 7).pathInFlight {
		t.Fatal("ID 次小的到期夜行者 7 未被派发")
	}
	// 第三份：预算耗尽，本次顺延——既不在途，也没有目标事实写入（结构上
	// 保证它没有读取世界构造快照）。
	if chaseSlot(t, manager, 9).pathInFlight {
		t.Fatal("第三只夜行者仍被派发，突破了每 tick 2 份快照预算")
	}
	if got := engine.HostileMobs()[2]; got.ID != 9 || got.HasTarget {
		t.Fatalf("顺延的夜行者 9 不应产生追逐事实：%+v", got)
	}
	// 其余顺延：结果落地、名额归还后的下一次推进轮到 ID 9。
	waitForChaseResults(t, manager, 2)
	manager.advance()
	waitForChaseResults(t, manager, 1)
	manager.advance()
	if !chaseSlot(t, manager, 9).pathInFlight && chaseSlot(t, manager, 9).path == nil {
		t.Fatal("顺延的夜行者 9 未在后续 tick 获得重规划")
	}
}

func TestHostileChaseDefersUntilRepathDue(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	mob := chaseMob(11, mgl32.Vec3{0.5, 1, 0.5})
	mob.NextRepathTicks = engine.WorldTime() + 3
	restoreChaseHostile(t, engine, mob)
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	manager.advance()
	if chaseSlot(t, manager, 11).pathInFlight {
		t.Fatal("未到期的夜行者被派发")
	}
	for range 3 {
		engine.Step()
	}
	manager.advance()
	if !chaseSlot(t, manager, 11).pathInFlight {
		t.Fatal("到期夜行者未被派发")
	}
}

func TestHostileChaseFullSlotsDeferWithoutBlocking(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	// 双槽占满：推进必须立即返回（非阻塞 select），该夜行者本次顺延。
	manager.semaphore <- struct{}{}
	manager.semaphore <- struct{}{}
	done := make(chan struct{})
	go func() {
		manager.advance()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("满槽时 advance 被阻塞，破坏了权威 tick 非阻塞投递契约")
	}
	if chaseSlot(t, manager, 11).pathInFlight {
		t.Fatal("满槽时仍完成派发，想要本次顺延")
	}
	// 归还名额后的下一 tick 完成重规划：顺延契约的「下一 tick」以权威世界
	// 时间推进为准。
	<-manager.semaphore
	<-manager.semaphore
	engine.Step()
	manager.advance()
	if !chaseSlot(t, manager, 11).pathInFlight {
		t.Fatal("名额归还后夜行者未被重新派发")
	}
}

func TestHostileChaseAppliesResultsByIDAndDropsStale(t *testing.T) {
	engine, manager, _ := newHostileChaseWorld(t, nil)
	for _, id := range []uint64{5, 9} {
		restoreChaseHostile(t, engine, chaseMob(id, mgl32.Vec3{0.5, 1, float32(id)}))
	}
	target := chasePlayerID(0x02)
	manager.slots[5] = &hostileChaseSlot{target: target, hasTarget: true, generation: 4, pathInFlight: true}
	manager.slots[9] = &hostileChaseSlot{target: target, hasTarget: true, generation: 4, pathInFlight: true}
	// 故意按 9 → 5 的逆序回送：应用阶段按 ID 升序消费，两份都落地。
	manager.results <- hostilePathOutcome{
		mobID: 9, generation: 4, target: target, dimension: core.Overworld,
		result: pathfind.PathResult{Waypoints: []pathfind.PathCell{{X: 1, Y: 1, Z: 9}}},
	}
	manager.results <- hostilePathOutcome{
		mobID: 5, generation: 4, target: target, dimension: core.Overworld,
		result: pathfind.PathResult{Waypoints: []pathfind.PathCell{{X: 1, Y: 1, Z: 5}}},
	}
	manager.advance()
	if got := chaseSlot(t, manager, 5).path; got == nil || len(got.Waypoints) != 1 {
		t.Fatalf("夜行者 5 的结果未应用：%+v", got)
	}
	if got := chaseSlot(t, manager, 9).path; got == nil || len(got.Waypoints) != 1 {
		t.Fatalf("夜行者 9 的结果未应用：%+v", got)
	}

	// 世代过期：派发后被更新的槽位不再接受旧世代结果。
	chaseSlot(t, manager, 5).path = nil
	chaseSlot(t, manager, 5).pathInFlight = true
	chaseSlot(t, manager, 5).generation = 5
	manager.results <- hostilePathOutcome{
		mobID: 5, generation: 4, target: target, dimension: core.Overworld,
		result: pathfind.PathResult{Waypoints: []pathfind.PathCell{{X: 2, Y: 1, Z: 5}}},
	}
	manager.advance()
	if got := chaseSlot(t, manager, 5).path; got != nil {
		t.Fatal("世代过期的结果被应用")
	}

	// 目标变化：派发时的目标与回送结果不一致即丢弃。
	chaseSlot(t, manager, 5).pathInFlight = true
	chaseSlot(t, manager, 5).generation = 5
	manager.results <- hostilePathOutcome{
		mobID: 5, generation: 5, target: chasePlayerID(0x09), dimension: core.Overworld,
		result: pathfind.PathResult{Waypoints: []pathfind.PathCell{{X: 2, Y: 1, Z: 5}}},
	}
	manager.advance()
	if got := chaseSlot(t, manager, 5).path; got != nil {
		t.Fatal("目标变化的结果被应用")
	}
}

func TestHostileChaseDropsResultWhenRevisionChanged(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	manager.advance()
	waitForChaseResults(t, manager, 1)
	// 在应用前让结果携带的 revision 过期：应用阶段必须整体丢弃并把重规划
	// 排到下一 tick。
	outcome := <-manager.results
	for index := range outcome.result.Revisions {
		outcome.result.Revisions[index].Revision++
	}
	manager.results <- outcome
	now := engine.WorldTime()
	manager.advance()
	if got := chaseSlot(t, manager, 11).path; got != nil {
		t.Fatal("revision 过期的结果被应用")
	}
	if got := engine.HostileMobs()[0].NextRepathTicks; got != now+1 {
		t.Fatalf("过期后的重规划 tick=%d，想要下一 tick %d", got, now+1)
	}
}

func TestHostileChaseDispatchDoesNotWaitForAStar(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	// 派发式推进在同一次调用内绝不等待 A*：返回时结果尚未应用。
	manager.advance()
	slot := chaseSlot(t, manager, 11)
	if !slot.pathInFlight {
		t.Fatal("派发未进入在途状态")
	}
	if slot.path != nil {
		t.Fatal("同一次推进内应用了 A* 结果，权威 tick 等待了寻路")
	}
	waitForChaseResults(t, manager, 1)
}

func TestHostileChaseClampsGoalToWindowEdgeTowardPlayer(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	// 目标水平 20 格，越出 ±16 窗口：终点必须钳到朝玩家方向的窗缘可站立格。
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{20.5, 1, 0.5})})
	manager.advance()
	waitForChaseResults(t, manager, 1)
	manager.advance()
	path := chaseSlot(t, manager, 11).path
	if path == nil {
		t.Fatal("钳窗后的路径未应用")
	}
	last := path.Waypoints[len(path.Waypoints)-1]
	if want := (pathfind.PathCell{X: 16, Y: 1, Z: 0}); last != want {
		t.Fatalf("终点=%v，想要朝玩家方向的窗缘可站立格 %v", last, want)
	}
	if first := path.Waypoints[0]; first != (pathfind.PathCell{X: 0, Y: 1, Z: 0}) {
		t.Fatalf("起点=%v，想要夜行者站立格 (0,1,0)", first)
	}
	// 窗口覆盖契约：全部 waypoint 落在以夜行者为中心的 ±16/±4 窗口内；
	// revisions 与当前权威 3×3 区块 revision 逐条一致。
	for _, cell := range path.Waypoints {
		if cell.X < -16 || cell.X > 16 || cell.Z < -16 || cell.Z > 16 ||
			cell.Y < -3 || cell.Y > 5 {
			t.Fatalf("waypoint %v 越出 33×9×33 窗口", cell)
		}
	}
	if len(path.Revisions) != 9 {
		t.Fatalf("路径携带 %d 条 revision，想要 3×3 区块共 9 条", len(path.Revisions))
	}
	for _, want := range path.Revisions {
		info, ok := engine.ChunkInfo(core.ChunkKey{Dimension: core.Overworld, Pos: want.Chunk})
		if !ok || info.State != contract.ChunkReady || info.Revision != want.Revision {
			t.Fatalf("路径 revision 与权威状态不符：%+v", want)
		}
	}
}

func TestHostileChaseInvalidatesPathOnRevisionMismatch(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	manager.advance()
	waitForChaseResults(t, manager, 1)
	manager.advance()
	slot := chaseSlot(t, manager, 11)
	if slot.path == nil {
		t.Fatal("前置失败：路径未应用")
	}
	// 结果携带的 revision 与当前权威状态失配（模拟快照后世界已变化）：
	// waypoint 提交前的重验必须清空路径并把重规划排到下一 tick。
	slot.path.Revisions[0].Revision++
	now := engine.WorldTime()
	manager.advance()
	if slot.path != nil {
		t.Fatal("revision 失配后旧路径未被清空")
	}
	if got := engine.HostileMobs()[0].NextRepathTicks; got != now+1 {
		t.Fatalf("失效后的重规划 tick=%d，想要下一 tick %d", got, now+1)
	}
}

func TestHostileChaseStopsAndFreezesAttackWithinRange(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	// 水平 1.5：攻击距离内。目标会话是真实激活玩家（sim 结算会重验会话）。
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{2.0, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{0.5, 1, 0.5})})
	engine.SetPlayerPositionForTest(1, mgl32.Vec3{0.5, 1, 0.5})
	manager.advance()
	if slot := chaseSlot(t, manager, 11); slot.path != nil || slot.pathInFlight {
		t.Fatal("攻击距离内仍发起寻路")
	}
	// 冻结的攻击意图经下一 tick 的权威模拟结算（3 点，经既有伤害入口）。
	engine.Step()
	if got, ok := engine.Player(1); !ok || got.Health != core.MaxHealth-3 {
		t.Fatalf("冻结的攻击意图未结算：生命=%v（ok=%v），想要 %d", got.Health, ok, core.MaxHealth-3)
	}
	// 停移裁决：攻击距离内不得提交任何移动意图，夜行者保持在原位。
	if got := engine.HostileMobs()[0].State.Position; got != (mgl32.Vec3{2.0, 1, 0.5}) {
		t.Fatalf("攻击距离内夜行者位移到 %v", got)
	}
}

func TestHostileChaseHoldsAttackWhileCooldownActive(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	mob := chaseMob(11, mgl32.Vec3{2.0, 1, 0.5})
	mob.AttackCooldown = 5
	restoreChaseHostile(t, engine, mob)
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{0.5, 1, 0.5})})
	engine.SetPlayerPositionForTest(1, mgl32.Vec3{0.5, 1, 0.5})
	manager.advance()
	engine.Step()
	if got, ok := engine.Player(1); !ok || got.Health != core.MaxHealth {
		t.Fatalf("冷却中的夜行者仍结算攻击：生命=%v（ok=%v）", got.Health, ok)
	}
}

func TestHostileChaseNeverCutsCornersWithoutPath(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, chaseBoxedChunk)
	// 夜行者被封顶石盒围死：A* 不可达，重规划按契约逐 tick 重试，但夜行者
	// 绝不允许不经路径直线穿墙接近目标。
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{2.5, 1, 2.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{20.5, 1, 2.5})})
	for cycle := 0; cycle < 4; cycle++ {
		manager.advance()
		if pending := len(manager.results); pending > 0 {
			manager.advance() // 应用失败结果：清路径、重规划排下一 tick
		}
		engine.Step()
	}
	if got := engine.HostileMobs()[0].State.Position; got != (mgl32.Vec3{2.5, 1, 2.5}) {
		t.Fatalf("无路径的夜行者发生了位移（穿墙直线移动）：%v", got)
	}
	if chaseSlot(t, manager, 11).path != nil {
		t.Fatal("不可达寻路的结果被当作可用路径")
	}
}

func TestHostileChaseWalksWaypointsAndAttacksTarget(t *testing.T) {
	engine, manager, setTargets := newHostileChaseWorld(t, nil)
	restoreChaseHostile(t, engine, chaseMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	// 注入目标与真实会话玩家必须同位：攻击意图在 sim 侧按会话做权威距离
	// 重验，幻影目标永远不会被结算。
	engine.SetPlayerPositionForTest(1, mgl32.Vec3{10.5, 1, 0.5})
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	attacked := false
	for cycle := 0; cycle < 200 && !attacked; cycle++ {
		manager.advance()
		if pending := len(manager.results); pending > 0 {
			manager.advance()
		}
		engine.Step()
		mob := engine.HostileMobs()[0]
		dx := 10.5 - mob.State.Position.X()
		dz := 0.5 - mob.State.Position.Z()
		if dx*dx+dz*dz <= contract.HostileAttackRange*contract.HostileAttackRange {
			attacked = true
		}
	}
	if !attacked {
		t.Fatal("夜行者未在预算周期内追至攻击距离")
	}
	// 进入攻击距离后的第一个推进冻结攻击意图，下一 tick 经权威模拟结算。
	manager.advance()
	engine.Step()
	if got, ok := engine.Player(1); !ok || got.Health >= core.MaxHealth {
		t.Fatalf("进入攻击距离后未冻结攻击意图：生命=%v（ok=%v）", got.Health, ok)
	}
	mob := engine.HostileMobs()[0]
	if got := mob.State.Position.Z(); got < -0.5 || got > 1.5 {
		t.Fatalf("夜行者偏离目标列：Z=%v", got)
	}
}
