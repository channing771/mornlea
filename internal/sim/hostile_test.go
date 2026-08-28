package sim

import (
	"math"
	"sort"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

// 本文件锁定夜行者身体契约：按 ID 严格升序、容量 64 的排序切片（无 map）、
// 恢复入口的记录校验，以及与玩家/伙伴完全相同的 per-actor `physics.Step` 积分
// （含 `physics.SubmersionFlags` 浸没标志的复用）。

// validTestHostile 返回一条可通过恢复校验的夜行者记录基线，各用例按需改字段。
func validTestHostile(id uint64) HostileMob {
	return HostileMob{
		ID:        id,
		Dimension: core.Overworld,
		State: physics.State{
			Position: mgl32.Vec3{0.5, 1, 0.5},
			OnGround: true,
		},
		Health:       core.MaxHealth,
		BurnCooldown: hostileCooldownPeriodTicks,
	}
}

// testTargetPlayerID 返回一个合法的 UUIDv4 玩家标识（version/variant 位就位）。
func testTargetPlayerID() core.PlayerID {
	var id core.PlayerID
	for index := range id {
		id[index] = byte(index + 1)
	}
	id[6] = 0x40
	id[8] = 0x80
	return id
}

func TestHostileRestoreKeepsIDsStrictlySorted(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	// 故意乱序恢复：内部集合必须按 ID 升序落位。
	for _, id := range []uint64{30, 10, 20} {
		if err := engine.RestoreHostile(validTestHostile(id)); err != nil {
			t.Fatalf("恢复夜行者 %d：%v", id, err)
		}
	}
	got := engine.HostileMobs()
	if len(got) != 3 || got[0].ID != 10 || got[1].ID != 20 || got[2].ID != 30 {
		t.Fatalf("恢复后 ID 序列=%v，想要 [10 20 30]", hostileIDs(got))
	}
	if !sort.SliceIsSorted(engine.hostiles.entries, func(i, j int) bool {
		return engine.hostiles.entries[i].id < engine.hostiles.entries[j].id
	}) {
		t.Fatal("内部夜行者切片未按 ID 升序维护")
	}
}

func TestHostileRestoreRejectsDuplicateAndSixtyFifth(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	if err := engine.RestoreHostile(validTestHostile(7)); err != nil {
		t.Fatalf("恢复夜行者 7：%v", err)
	}
	if err := engine.RestoreHostile(validTestHostile(7)); err == nil {
		t.Fatal("重复 ID 被接受，想要拒绝")
	}
	for id := uint64(1); id <= 63; id++ {
		if err := engine.RestoreHostile(validTestHostile(id * 100)); err != nil {
			t.Fatalf("恢复夜行者 %d：%v", id*100, err)
		}
	}
	if len(engine.hostiles.entries) != maxHostiles {
		t.Fatalf("夜行者数量=%d，想要 %d", len(engine.hostiles.entries), maxHostiles)
	}
	if err := engine.RestoreHostile(validTestHostile(9999)); err == nil {
		t.Fatal("第 65 只夜行者被接受，想要拒绝")
	}
	if len(engine.hostiles.entries) != maxHostiles {
		t.Fatalf("拒绝后夜行者数量=%d，想要仍为 %d", len(engine.hostiles.entries), maxHostiles)
	}
}

func TestHostileRestoreValidatesRecordFields(t *testing.T) {
	cases := map[string]func(*HostileMob){
		"零 ID":         func(m *HostileMob) { m.ID = 0 },
		"未知维度":         func(m *HostileMob) { m.Dimension = core.DimensionID(7) },
		"位置非有限":        func(m *HostileMob) { m.State.Position = mgl32.Vec3{float32(math.NaN()), 1, 0.5} },
		"速度非有限":        func(m *HostileMob) { m.State.Velocity = mgl32.Vec3{0, float32(math.Inf(1)), 0} },
		"位置高于世界":       func(m *HostileMob) { m.State.Position = mgl32.Vec3{0.5, core.MaxY, 0.5} },
		"位置低于世界":       func(m *HostileMob) { m.State.Position = mgl32.Vec3{0.5, core.MinY - 1, 0.5} },
		"生命为零":         func(m *HostileMob) { m.Health = 0 },
		"生命超过上限":       func(m *HostileMob) { m.Health = core.MaxHealth + 1 },
		"攻击冷却越界":       func(m *HostileMob) { m.AttackCooldown = hostileCooldownPeriodTicks + 1 },
		"受击冷却越界":       func(m *HostileMob) { m.HurtCooldown = hostileCooldownPeriodTicks + 1 },
		"灼烧冷却越界":       func(m *HostileMob) { m.BurnCooldown = hostileCooldownPeriodTicks + 1 },
		"远离累计越界":       func(m *HostileMob) { m.DistantTicks = maxHostileDistantTicks + 1 },
		"无目标却带玩家 ID":   func(m *HostileMob) { m.PlayerID = testTargetPlayerID() },
		"有目标但玩家 ID 为零": func(m *HostileMob) { m.HasTarget = true },
		"有目标但非 UUIDv4": func(m *HostileMob) {
			m.HasTarget = true
			m.PlayerID = testTargetPlayerID()
			m.PlayerID[6] = 0x10
		},
	}
	for name, mutate := range cases {
		engine := NewEngine(0, 0, 0)
		mob := validTestHostile(42)
		mutate(&mob)
		if err := engine.RestoreHostile(mob); err == nil {
			t.Fatalf("%s：非法记录被接受，想要拒绝", name)
		}
		if len(engine.hostiles.entries) != 0 {
			t.Fatalf("%s：被拒绝的记录仍留在集合中", name)
		}
	}
}

func hostileIDs(mobs []HostileMob) []uint64 {
	out := make([]uint64, 0, len(mobs))
	for _, mob := range mobs {
		out = append(out, mob.ID)
	}
	return out
}

func TestHostileMovementReusesPlayerPhysicsStep(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 与玩家出生同形的落点：从半空坠落至 flat 世界地表。
	start := physics.State{Position: mgl32.Vec3{2.5, 10, 2.5}}
	mob := validTestHostile(11)
	mob.State = start
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	source := dimensionCollisionSource{dimension: engine.dimension(core.Overworld)}
	engine.advanceHostileMovement()
	entry := &engine.hostiles.entries[0]
	want := physics.Step(start, entry.input, source).State
	if entry.state != want {
		t.Fatalf("夜行者积分结果=%+v，想要与 `physics.Step` 完全一致的 %+v", entry.state, want)
	}
	if entry.state.OnGround {
		t.Fatal("单步后不应已落地")
	}
	// 持续推进到落地：落点必须与逐 tick 复算的 `physics.Step` 一致，且水平中性
	// 输入下没有任何水平位移。
	for range 200 {
		if engine.hostiles.entries[0].state.OnGround {
			break
		}
		engine.advanceHostileMovement()
	}
	entry = &engine.hostiles.entries[0]
	if !entry.state.OnGround {
		t.Fatal("夜行者 200 tick 内未落地")
	}
	if entry.state.Position.X() != 2.5 || entry.state.Position.Z() != 2.5 {
		t.Fatalf("中性输入下夜行者发生了水平位移：%+v", entry.state.Position)
	}
	if entry.state.Position.Y() != 1 {
		t.Fatalf("夜行者落点 Y=%v，想要 1", entry.state.Position.Y())
	}
}

func TestHostileMovementReusesSubmersionFlags(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// flat 世界的 (0,1,0) 换成水源方块：夜行者恰好站在这一格里。
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 1, Z: 0}, core.WaterSourceID)

	mob := validTestHostile(13)
	mob.State = physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	engine.advanceHostileMovement()
	source := dimensionCollisionSource{dimension: engine.dimension(core.Overworld)}
	bodyInFluid, eyeInFluid := physics.SubmersionFlags(mgl32.Vec3{0.5, 1, 0.5}, source)
	if !bodyInFluid || eyeInFluid {
		t.Fatalf("SubmersionFlags=(%v,%v)，想要身体浸没且眼睛在水上", bodyInFluid, eyeInFluid)
	}
	want := physics.Step(
		physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}},
		physics.Input{BodyInFluid: bodyInFluid, EyeInFluid: eyeInFluid},
		source,
	).State
	entry := &engine.hostiles.entries[0]
	if entry.state != want {
		t.Fatalf("水中积分=%+v，想要复用 `physics.SubmersionFlags` 的 %+v", entry.state, want)
	}
}

func TestHostileMovementAdvancesEachActorOnceInIDOrder(t *testing.T) {
	// 夜行者阶段位于统一物理阶段（玩家 → 伙伴）之后，由权威 tick 的阶段顺序契约
	// 锁定；这里锁定夜行者内部按 ID 升序逐 actor 各步进恰好一次。
	engine, _ := readyMovementPlayer(t)
	for _, id := range []uint64{900, 100} {
		mob := validTestHostile(id)
		mob.State = physics.State{Position: mgl32.Vec3{4.5, 6, 4.5}}
		if err := engine.RestoreHostile(mob); err != nil {
			t.Fatalf("恢复夜行者 %d：%v", id, err)
		}
	}
	engine.advanceHostileMovement()
	for index, wantID := range []uint64{100, 900} {
		entry := &engine.hostiles.entries[index]
		if entry.id != wantID {
			t.Fatalf("内部切片第 %d 项 ID=%d，想要 %d", index, entry.id, wantID)
		}
		if entry.state.Position.Y() >= 6 {
			t.Fatalf("夜行者 %d 未被物理步进：%+v", entry.id, entry.state)
		}
	}
}
