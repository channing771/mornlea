package client

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 本文件锁定厚雪减速的客户端预测一半（snow-cover-accumulation）：权威模拟与
// 客户端预测共用 physics.Step，减速判定同表自动一致的前提是 MirrorCollisionSource
// 与权威侧 dimensionCollisionSource 交付同一份落足格方块视图。测试用「镜像侧
// 预测器 vs 权威形态适配器」同输入对照逐位锁死这一点。

// snowSlowdownChunk 构造区块 (0,0)：y=0 铺满承载地面，footBlock 非空气时 y=1
// 铺满该方块（脚部所在格）。玩家从 (0.5,1,0.5) 向 +X 走 40 tick 至 x≈9，
// 全程不离开本区块。
func snowSlowdownChunk(footBlock core.BlockID) *world.Chunk {
	chunk := world.NewChunk(core.ChunkPos{})
	for z := 0; z < core.SectionSize; z++ {
		for x := 0; x < core.SectionSize; x++ {
			chunk.SetBlock(x, 0, z, core.DirtID)
			if footBlock != core.AirID {
				chunk.SetBlock(x, 1, z, footBlock)
			}
		}
	}
	return chunk
}

// authoritativeFootSource 以 map 形态复刻权威侧 dimensionCollisionSource 的
// 方块视图语义（map 未命中即空气、恒已加载），供与镜像侧做同输入对照。
type authoritativeFootSource map[core.BlockPos]core.BlockID

func (s authoritativeFootSource) CollisionBoxes(position core.BlockPos) physics.CollisionBoxSet {
	return physics.BlockCollisionBoxes(s[position], true)
}

func (authoritativeFootSource) IsFluidAt(core.BlockPos) bool { return false }

func (s authoritativeFootSource) BlockIDAt(position core.BlockPos) (core.BlockID, bool) {
	return s[position], true
}

// walkPredictorEast 在镜像世界上驱动预测器按住 +X 走 tickCount 个固定步，
// 返回预测末态。
func walkPredictorEast(
	t *testing.T,
	source physics.WorldSource,
	tickCount int,
) physics.State {
	t.Helper()
	predictor := NewPredictor()
	if err := predictor.Begin(network.PlayerState{
		ServerTick: 1,
		Dimension:  core.Overworld,
		Position:   mgl32.Vec3{0.5, 1, 0.5},
		OnGround:   true,
		Ready:      true,
	}); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	sequence := uint64(0)
	for range tickCount {
		if err := predictor.Advance(
			physics.FixedDelta,
			Control{MoveX: 1},
			source,
			func() uint64 { sequence++; return sequence },
			func(network.PlayerInput) error { return nil },
		); err != nil {
			t.Fatalf("Advance: %v", err)
		}
	}
	state, ok := predictor.State()
	if !ok {
		t.Fatal("预测器未就绪")
	}
	return state
}

// TestMirrorCollisionSourceFootBlockSampling 锁定 MirrorCollisionSource 的落足
// 格采样视图：已加载区块交付真实方块 ID；缺失或已失同步的区块返回不可用
// （减速安全回退为不减速）；超出世界高度的格视为空气（已加载）——与权威侧
// Dimension.BlockAt 的语义逐条对应。
func TestMirrorCollisionSourceFootBlockSampling(t *testing.T) {
	snow := core.BlockPos{X: 2, Y: 1, Z: 3}
	chunk := world.NewChunk(core.ChunkPos{})
	chunk.SetBlock(2, snow.Y, 3, core.SnowLayer3BlockID)
	source := MirrorCollisionSource{Mirror: mirrorWithChunk(t, core.Overworld, chunk), Dimension: core.Overworld}

	if got, ok := source.BlockIDAt(snow); !ok || got != core.SnowLayer3BlockID {
		t.Fatalf("雪层格 BlockIDAt = (%d, %t)，想要 (%d, true)", got, ok, core.SnowLayer3BlockID)
	}
	if got, ok := source.BlockIDAt(core.BlockPos{X: 5, Y: 2, Z: 5}); !ok || got != core.AirID {
		t.Fatalf("空气格 BlockIDAt = (%d, %t)，想要 (%d, true)", got, ok, core.AirID)
	}
	if got, ok := source.BlockIDAt(core.BlockPos{X: 32, Y: 1, Z: 0}); ok || got != 0 {
		t.Fatalf("缺失区块 BlockIDAt = (%d, %t)，想要 (0, false)", got, ok)
	}
	for _, outside := range []core.BlockPos{
		{X: 2, Y: core.MinY - 1, Z: 3},
		{X: 2, Y: core.MaxY, Z: 3},
	} {
		if got, ok := source.BlockIDAt(outside); !ok || got != core.AirID {
			t.Fatalf("世界高度外 %+v BlockIDAt = (%d, %t)，想要 (%d, true)", outside, got, ok, core.AirID)
		}
	}

	// 失同步窗口与 CollisionBoxes 同判：revision 缺口期间该区块的一切采样都
	// 不可用，替换快照到达后恢复。
	gap := network.BlockChanges{
		Dimension:    core.Overworld,
		Chunk:        core.ChunkPos{},
		BaseRevision: 2,
		NewRevision:  3,
		Changes: []network.BlockChange{{
			Position: snow,
			Block:    core.StoneID,
		}},
	}
	if update, err := source.Mirror.Apply(gap); err != nil || update.Resync == nil {
		t.Fatalf("revision gap update=%+v err=%v，想要 resync", update, err)
	}
	if got, ok := source.BlockIDAt(snow); ok || got != 0 {
		t.Fatalf("失同步区块 BlockIDAt = (%d, %t)，想要 (0, false)", got, ok)
	}
}

// TestPredictorDeepSnowSlowdownMatchesAuthority 覆盖 spec Scenario「权威与预测
// 同速」的客户端一半：同一 3 档雪层世界上，预测器（MirrorCollisionSource）
// 与权威形态适配器各自积分相同时长，末态 MUST 逐位一致（同表判定、无橡皮筋
// 回拉），且水平位移 MUST 明显低于无雪地面（≈0.7 倍）；2 档镜世界与无雪
// 逐位一致。
func TestPredictorDeepSnowSlowdownMatchesAuthority(t *testing.T) {
	const ticks = 40
	deepMirror := MirrorCollisionSource{
		Mirror:    mirrorWithChunk(t, core.Overworld, snowSlowdownChunk(core.SnowLayer3BlockID)),
		Dimension: core.Overworld,
	}
	predicted := walkPredictorEast(t, deepMirror, ticks)

	authority := authoritativeFootSource{}
	for z := int32(0); z < core.SectionSize; z++ {
		for x := int32(0); x < core.SectionSize; x++ {
			authority[core.BlockPos{X: x, Y: 0, Z: z}] = core.DirtID
			authority[core.BlockPos{X: x, Y: 1, Z: z}] = core.SnowLayer3BlockID
		}
	}
	state := physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true}
	for range ticks {
		state = physics.Step(state, physics.Input{MoveX: 1}, authority).State
	}
	if predicted != state {
		t.Fatalf("预测与权威形态末态分叉：预测=%+v 权威=%+v", predicted, state)
	}

	bareMirror := MirrorCollisionSource{
		Mirror:    mirrorWithChunk(t, core.Overworld, snowSlowdownChunk(core.AirID)),
		Dimension: core.Overworld,
	}
	thinMirror := MirrorCollisionSource{
		Mirror:    mirrorWithChunk(t, core.Overworld, snowSlowdownChunk(core.SnowLayer2BlockID)),
		Dimension: core.Overworld,
	}
	bare := walkPredictorEast(t, bareMirror, ticks)
	thin := walkPredictorEast(t, thinMirror, ticks)
	if thin != bare {
		t.Fatalf("2 档镜世界改变了预测末态：2档=%+v 无雪=%+v", thin, bare)
	}
	if travelled := bare.Position.X() - 0.5; travelled < 4 {
		t.Fatalf("无雪预测 40 tick 只走了 %v 格，夹具位移不足", travelled)
	}
	ratio := (predicted.Position.X() - 0.5) / (bare.Position.X() - 0.5)
	if ratio < 0.69 || ratio > 0.72 {
		t.Fatalf("3 档预测位移比例 = %v，想要 ≈0.7（预测=%v 无雪=%v）",
			ratio, predicted.Position.X(), bare.Position.X())
	}
}
