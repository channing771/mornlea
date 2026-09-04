package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 本文件锁定被动牛的权威吃草事件：`splitmix64` 确定性抽选命中 + 站草 +
// `chunk` 就绪才触发，持续 20 `tick` 低头后把触发格草变为泥土；事件期间
// 受击、移动或脚下方块变化立即中断且不写块；事件态为瞬态，重启不恢复。

// newGrazeEngine 返回无会话的吃草测试引擎：周围 5×5 区块是 y=0 草地。
// 无会话意味着生成阶段恒早退，集合里只有用例亲手恢复的牛。
func newGrazeEngine(t *testing.T, seed int64) *Engine {
	t.Helper()
	engine := NewEngine(0, 0, seed)
	loadFlatChunks(t, engine.dimension(core.Overworld), -2, 2, -2, 2)
	return engine
}

// restoreGrazeCow 把一头满血牛放到指定落点。
func restoreGrazeCow(t *testing.T, engine *Engine, id uint64, position mgl32.Vec3) {
	t.Helper()
	mob := validTestPassive(id)
	mob.State = physics.State{Position: position, OnGround: true}
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
}

// advanceGrazeTick 按生产顺序推进被动阶段一整拍（含吃草）并提交写入。
func advanceGrazeTick(engine *Engine, tick uint64, result *TickResult) {
	engine.tick.Store(tick)
	pending := engine.newMutation()
	engine.advancePassives(pending)
	engine.finishChanges(pending, result)
}

// triggerGraze 以生产路径推进到吃草事件触发，返回触发 `tick` 与触发格。
func triggerGraze(t *testing.T, engine *Engine, id uint64) (uint64, core.BlockPos) {
	t.Helper()
	for tick := uint64(0); tick < 20000; tick++ {
		advanceGrazeTick(engine, tick, &TickResult{})
		index := engine.passives.findIndex(id)
		if index < 0 {
			t.Fatalf("等待触发时被动牛 %d 消失", id)
		}
		if entry := &engine.passives.entries[index]; entry.grazeTicks > 0 {
			return tick, entry.grazePos
		}
	}
	t.Fatal("20000 tick 内吃草事件未触发，想要命中一次")
	return 0, core.BlockPos{}
}

// firstGrazeHitTick 纯算出首个抽选命中的 `tick`，供“命中时刻站在非草上”用例定位时钟。
func firstGrazeHitTick(t *testing.T, seed int64, id uint64) uint64 {
	t.Helper()
	for tick := uint64(0); tick < 20000; tick++ {
		if passiveGrazeHit(seed, tick, id) {
			return tick
		}
	}
	t.Fatal("20000 tick 内抽选从未命中，想要命中一次")
	return 0
}

// dirtCellsAtY 统计某区块指定高度上的泥土格数。
func dirtCellsAtY(t *testing.T, engine *Engine, pos core.ChunkPos, y int32) int {
	t.Helper()
	dimension := engine.dimension(core.Overworld)
	count := 0
	for x := int32(0); x < core.SectionSize; x++ {
		for z := int32(0); z < core.SectionSize; z++ {
			block, ready := dimension.BlockAt(core.BlockPos{
				X: pos.X*core.SectionSize + x, Y: y, Z: pos.Z*core.SectionSize + z,
			})
			if !ready {
				t.Fatalf("读取区块 %+v 时未就绪", pos)
			}
			if block == core.DirtID {
				count++
			}
		}
	}
	return count
}

func TestPassiveGrazeTurnsGrassToDirtAfterTwentyTicks(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	restoreGrazeCow(t, engine, 11, mgl32.Vec3{2.5, 1, 2.5})
	start, pos := triggerGraze(t, engine, 11)
	if got := engine.PassiveMobs()[0].Grazing; !got {
		t.Fatal("触发后快照放牧位未置位，想要低头呈现")
	}
	frozen := engine.passives.entries[0].state.Position
	// 触发拍计为事件第 1 tick：再推进 18 拍仍在低头且不写块，第 20 拍结算。
	for offset := uint64(1); offset <= 18; offset++ {
		advanceGrazeTick(engine, start+offset, &TickResult{})
		entry := &engine.passives.entries[0]
		if entry.grazeTicks == 0 {
			t.Fatalf("第 %d tick 事件提前结束，想要持续低头", offset+1)
		}
		if entry.state.Position != frozen {
			t.Fatalf("事件第 %d tick 牛位移，想要静止低头", offset+1)
		}
		if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block != core.GrassID {
			t.Fatalf("第 %d tick 触发格=%d，想要结算前保持为草", offset+1, block)
		}
	}
	result := &TickResult{}
	advanceGrazeTick(engine, start+19, result)
	if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block != core.DirtID {
		t.Fatalf("第 20 tick 触发格=%d，想要变为泥土", block)
	}
	if got := engine.PassiveMobs()[0].Grazing; got {
		t.Fatal("结算后快照放牧位仍置位，想要恢复漫游位姿")
	}
	if got := dirtCellsAtY(t, engine, pos.Chunk(), pos.Y); got != 1 {
		t.Fatalf("事件写入泥土=%d 格，想要恰好单格", got)
	}
	found := false
	for _, batch := range result.Changes {
		for _, change := range batch.Changes {
			if change.Position == pos && change.Block == core.DirtID {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("结算批次里没有触发格的草→泥土变更，想要经 mutation 广播")
	}
	if len(engine.passives.entries) != 1 {
		t.Fatal("结算后被动牛消失，想要吃草不影响存活")
	}
}

func TestPassiveGrazeDamageInterruptsWithoutWriting(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	restoreGrazeCow(t, engine, 12, mgl32.Vec3{2.5, 1, 2.5})
	start, pos := triggerGraze(t, engine, 12)
	from := mgl32.Vec3{10.5, 1, 2.5}
	if !engine.DamagePassive(12, 1, from) {
		t.Fatal("已知个体的有效伤害被拒绝，想要受理")
	}
	if got := engine.PassiveMobs()[0].Grazing; got {
		t.Fatal("受击后快照放牧位仍置位，想要立即终止事件")
	}
	if entry := &engine.passives.entries[0]; entry.fleeTicks == 0 {
		t.Fatal("受击后未进入逃跑，想要逃跑优先")
	}
	for offset := uint64(1); offset <= 30; offset++ {
		advanceGrazeTick(engine, start+offset, &TickResult{})
	}
	if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block != core.GrassID {
		t.Fatalf("中断后触发格=%d，想要保持为草", block)
	}
	if got := dirtCellsAtY(t, engine, pos.Chunk(), pos.Y); got != 0 {
		t.Fatalf("中断后出现 %d 格泥土，想要零写入", got)
	}
}

func TestPassiveGrazeMoveInterruptsWithoutWriting(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	restoreGrazeCow(t, engine, 13, mgl32.Vec3{2.5, 1, 2.5})
	start, pos := triggerGraze(t, engine, 13)
	// 外力挪到同区块另一草格：支撑格变化即“移动”，事件必须终结。
	engine.passives.entries[0].state.Position = mgl32.Vec3{10.5, 1, 10.5}
	advanceGrazeTick(engine, start+1, &TickResult{})
	if got := engine.PassiveMobs()[0].Grazing; got {
		t.Fatal("换格后快照放牧位仍置位，想要移动中断事件")
	}
	for offset := uint64(2); offset <= 30; offset++ {
		advanceGrazeTick(engine, start+offset, &TickResult{})
	}
	if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block != core.GrassID {
		t.Fatalf("中断后原触发格=%d，想要保持为草", block)
	}
	if got := dirtCellsAtY(t, engine, pos.Chunk(), pos.Y); got != 0 {
		t.Fatalf("中断后出现 %d 格泥土，想要零写入", got)
	}
}

func TestPassiveGrazeBlockChangeInterruptsWithoutWriting(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	restoreGrazeCow(t, engine, 14, mgl32.Vec3{2.5, 1, 2.5})
	start, pos := triggerGraze(t, engine, 14)
	// 同 tick 其他写者（如翻地）先改掉了触发格：事件终结且不再写块。
	// 用石头而不用泥土——结算产物正是泥土，石头才能证明“没有写”。
	engine.SetBlockForTest(pos, core.StoneID)
	advanceGrazeTick(engine, start+1, &TickResult{})
	if got := engine.PassiveMobs()[0].Grazing; got {
		t.Fatal("触发格被改后快照放牧位仍置位，想要换块中断事件")
	}
	if block, _ := engine.dimension(core.Overworld).BlockAt(pos); block != core.StoneID {
		t.Fatalf("中断后触发格=%d，想要保持改写后的石头", block)
	}
}

func TestPassiveGrazeNeedsStandingGrass(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block core.BlockID
	}{
		{"泥土上不触发", core.DirtID},
		{"石头上不触发", core.StoneID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newGrazeEngine(t, 0)
			engine.SetBlockForTest(core.BlockPos{X: 2, Y: 0, Z: 2}, tc.block)
			restoreGrazeCow(t, engine, 15, mgl32.Vec3{2.5, 1, 2.5})
			// 把时钟拨到必命中的 tick：若站草判定缺失，这里必会触发。
			hit := firstGrazeHitTick(t, 0, 15)
			engine.tick.Store(hit)
			pending := engine.newMutation()
			engine.advancePassives(pending)
			if entry := &engine.passives.entries[0]; entry.grazeTicks > 0 {
				t.Fatal("站在非草上触发了吃草事件，想要拒绝")
			}
			if pending.Len() != 0 {
				t.Fatalf("拒绝触发却登记了 %d 个区块变更，想要零写入", pending.Len())
			}
		})
	}
}

func TestPassiveGrazeUnloadedChunkSettlesNothing(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	restoreGrazeCow(t, engine, 16, mgl32.Vec3{2.5, 1, 2.5})
	start, _ := triggerGraze(t, engine, 16)
	entry := &engine.passives.entries[0]
	// 白盒摆出“结算时区块已卸载”：牛与触发格同在从未加载的远处格，
	// 支撑一致但读不到方块。直接置剩余 1 tick 等价于事件末拍的状态。
	far := core.BlockPos{X: 1000, Y: 0, Z: 1000}
	entry.state.Position = mgl32.Vec3{1000.5, 1, 1000.5}
	entry.grazePos = far
	entry.grazeTicks = 1
	engine.tick.Store(start + 1)
	pending := engine.newMutation()
	engine.advancePassives(pending)
	if entry.grazeTicks > 0 {
		t.Fatal("未加载 chunk 下事件未终结，想要丢弃结算")
	}
	if pending.Len() != 0 {
		t.Fatalf("未加载 chunk 下登记了 %d 个区块变更，想要零写入且不触发加载", pending.Len())
	}
	if len(engine.passives.entries) != 1 {
		t.Fatal("丢弃结算时牛被移除，想要只终结事件")
	}
}

func TestPassiveGrazeStaysBoundedAtCapacity(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	for id := uint64(1); id <= 32; id++ {
		restoreGrazeCow(t, engine, id, mgl32.Vec3{float32(id%16) + 0.5, 1, 0.5})
	}
	// 满编单拍必须正常完成；触发只记事件态，结算才写块，单拍零写入。
	engine.tick.Store(41)
	pending := engine.newMutation()
	engine.advancePassives(pending)
	if len(engine.passives.entries) != 32 {
		t.Fatalf("满编单拍后牛=%d 头，想要 32 头无丢失", len(engine.passives.entries))
	}
	if pending.Len() != 0 {
		t.Fatalf("满编单拍登记了 %d 个区块变更，想要零写入", pending.Len())
	}
}

func TestPassiveGrazeTransientAcrossRestart(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	restoreGrazeCow(t, engine, 17, mgl32.Vec3{2.5, 1, 2.5})
	triggerGraze(t, engine, 17)
	snapshot := engine.PassiveMobs()
	if !snapshot[0].Grazing {
		t.Fatal("触发后快照放牧位未置位，想要瞬态位可被发布读到")
	}
	restarted := NewEngine(0, 0, 0)
	// 恢复入口必须忽略瞬态放牧位：即使快照里置位，重载后也恒为清位。
	if err := restarted.RestorePassive(snapshot[0]); err != nil {
		t.Fatalf("重载恢复被拒绝：%v", err)
	}
	if got := restarted.PassiveMobs()[0].Grazing; got {
		t.Fatal("重启后快照放牧位置位，想要事件消失")
	}
	if restarted.passives.entries[0].grazeTicks != 0 {
		t.Fatal("重启后事件剩余 tick 非零，想要瞬态不落盘")
	}
}
