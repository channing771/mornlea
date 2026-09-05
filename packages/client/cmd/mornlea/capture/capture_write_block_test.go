package capture

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestSetCaptureBlockSwitchesExactlyOneCell 锁定 capture-only 写块口：只切换
// 请求的一格，其余格不动；生产代码不得消费本方法（调用点审计：除本测试与
// GIF 剧本外无其他引用，见报告）。
func TestSetCaptureBlockSwitchesExactlyOneCell(t *testing.T) {
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := newCaptureAICompanionState()
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		t.Fatalf("准备空气邻域: %v", err)
	}
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	for _, position := range []core.BlockPos{{X: 0, Y: 0, Z: 0}, {X: 1, Y: 0, Z: 0}} {
		chunk := position.Chunk()
		if blocks[chunk] == nil {
			blocks[chunk] = make(map[core.BlockPos]core.BlockID)
		}
		blocks[chunk][position] = core.GrassID
	}
	if err := applyCaptureBlocks(app, blocks, 0, "单格测试草地"); err != nil {
		t.Fatalf("装入草地: %v", err)
	}
	trigger := core.BlockPos{X: 0, Y: 0, Z: 0}
	if err := app.SetCaptureBlock(trigger, core.DirtID); err != nil {
		t.Fatalf("SetCaptureBlock: %v", err)
	}
	if got, loaded := app.Mirror().BlockAt(core.Overworld, trigger); !loaded || got != core.DirtID {
		t.Fatalf("触发格=%d/%v，想要泥土/true", got, loaded)
	}
	neighbor := core.BlockPos{X: 1, Y: 0, Z: 0}
	if got, loaded := app.Mirror().BlockAt(core.Overworld, neighbor); !loaded || got != core.GrassID {
		t.Fatalf("邻格=%d/%v，想要草地/true（只允许切换一格）", got, loaded)
	}
}
