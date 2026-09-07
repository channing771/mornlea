package physics_test

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// TestSnowLayersHaveNoCollision 锁定雪层四档的零碰撞契约：已加载但零碰撞体，
// 与流体、植物、火把同形状——雪层是贴地装饰层，实体由下方承载方块支撑、脚部
// 占据雪层格自由穿行。
func TestSnowLayersHaveNoCollision(t *testing.T) {
	for id := core.SnowLayer1BlockID; id <= core.SnowLayer4BlockID; id++ {
		boxes := physics.BlockCollisionBoxes(id, true)
		if !boxes.Loaded || boxes.Count != 0 {
			t.Fatalf("BlockCollisionBoxes(雪层 %d, true) = %+v，想要 (Loaded:true, Count:0)", id, boxes)
		}
	}
	// 未加载格与既有语义一致：整体视为空集（含 Loaded=false）。
	if boxes := physics.BlockCollisionBoxes(core.SnowLayer3BlockID, false); boxes.Loaded || boxes.Count != 0 {
		t.Fatalf("BlockCollisionBoxes(雪层, false) = %+v，想要 (Loaded:false, Count:0)", boxes)
	}
}

// TestEntityPassesThroughSnowLayerBlock 端到端验证雪层零碰撞：同一堵墙，石头会
// 挡住实体，换成 4 档雪层则实体可自由穿行——与流体、火把同一判定路径。
func TestEntityPassesThroughSnowLayerBlock(t *testing.T) {
	snowWorld := idSource{core.BlockPos{X: 1, Y: 1, Z: 0}: core.SnowLayer4BlockID}
	passed := physics.Step(physics.State{
		Position: mgl32.Vec3{0.5, 1, 0.5},
		Velocity: mgl32.Vec3{10, 0, 0},
		OnGround: true,
	}, physics.Input{}, snowWorld).State
	if passed.Position.X() <= 0.7+1e-5 {
		t.Fatalf("雪层阻挡了实体穿行: %+v", passed)
	}
}
