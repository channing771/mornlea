package worldgen_test

// 本文件锁定运行时树形几何桥(`TreeBlocks`)的 ABI 边界与几何契约。Go 侧只做
// 28 字节请求编码与记录解码,几何本身由 engine 计算,因此这里只断言生产出口
// 上可观察的确定性、有界性、输入依赖与失败语义,不复制树冠层形——层形由
// engine 侧的主题单测与 FFI 测试覆盖。

import (
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// treeBlocksSampleRoots 是一批覆盖正负坐标与树冠跨界位置的根坐标。
func treeBlocksSampleRoots() []core.BlockPos {
	return []core.BlockPos{
		{X: 0, Y: 64, Z: 0},
		{X: -137, Y: 70, Z: 902},
		{X: 15, Y: 64, Z: -16},
		{X: 1_000_003, Y: 100, Z: -2_000_004},
	}
}

func TestTreeBlocksIsDeterministicAndBounded(t *testing.T) {
	for _, seed := range []int64{1, 42, -7} {
		for _, root := range treeBlocksSampleRoots() {
			first := worldgen.TreeBlocks(seed, root)
			second := worldgen.TreeBlocks(seed, root)
			if len(first) != len(second) {
				t.Fatalf("seed=%d root=%v 两次记录数不同: %d vs %d", seed, root, len(first), len(second))
			}
			for index := range first {
				if first[index] != second[index] {
					t.Fatalf("seed=%d root=%v 记录 %d 不一致: %+v vs %+v", seed, root, index, first[index], second[index])
				}
			}
			if len(first) == 0 || len(first) > 128 {
				t.Fatalf("seed=%d root=%v 记录数=%d，想要 1..128", seed, root, len(first))
			}
			// 根格自身是第一条记录,恒为树干底原木(偏移全零)。
			if first[0] != (worldgen.TreeBlock{Block: core.OakLogID}) {
				t.Fatalf("seed=%d root=%v 首条记录=%+v，想要偏移全零的原木", seed, root, first[0])
			}
			trunk := 0
			for _, record := range first {
				if record.DX < -2 || record.DX > 2 || record.DZ < -2 || record.DZ > 2 {
					t.Fatalf("seed=%d root=%v 记录 %+v 超出水平半径 2", seed, root, record)
				}
				if record.DY < 0 {
					t.Fatalf("seed=%d root=%v 记录 %+v 落在根坐标以下", seed, root, record)
				}
				switch record.Block {
				case core.OakLogID:
					trunk++
					if record.DX != 0 || record.DZ != 0 {
						t.Fatalf("seed=%d root=%v 原木偏离根列(分杈?): %+v", seed, root, record)
					}
				case core.LeavesID:
				default:
					t.Fatalf("seed=%d root=%v 出现非原木/树叶方块 %d", seed, root, record.Block)
				}
			}
			if trunk < 5 || trunk > 7 {
				t.Fatalf("seed=%d root=%v 树高=%d，想要 5..7", seed, root, trunk)
			}
			if leaves := len(first) - trunk; leaves == 0 {
				t.Fatalf("seed=%d root=%v 没有树叶记录", seed, root)
			}
		}
	}
}

// TestTreeBlocksGeometryDependsOnSeedAndRoot 守住「参数由独立 salt 从
// (世界种子, 根坐标) 派生」:几何只有高度与蓬松两档共 6 种组合,单看一对
// 输入可能偶然相同,因此按多样本判断而不是逐对判断。
func TestTreeBlocksGeometryDependsOnSeedAndRoot(t *testing.T) {
	root := core.BlockPos{X: 3, Y: 64, Z: -3}
	base := worldgen.TreeBlocks(42, root)
	seedChanged := false
	for seed := int64(0); seed < 16; seed++ {
		records := worldgen.TreeBlocks(seed, root)
		if len(records) != len(base) {
			seedChanged = true
			break
		}
		for index := range records {
			if records[index] != base[index] {
				seedChanged = true
				break
			}
		}
		if seedChanged {
			break
		}
	}
	if !seedChanged {
		t.Fatal("换种子几何不变:树形参数没有随世界种子派生")
	}
	rootChanged := false
	for offset := int32(0); offset < 16; offset++ {
		records := worldgen.TreeBlocks(42, core.BlockPos{X: root.X + offset, Y: root.Y, Z: root.Z})
		if len(records) != len(base) {
			rootChanged = true
			break
		}
		for index := range records {
			if records[index] != base[index] {
				rootChanged = true
				break
			}
		}
		if rootChanged {
			break
		}
	}
	if !rootChanged {
		t.Fatal("换根坐标几何不变:树形参数没有随根坐标派生")
	}
}

// TestTreeBlocksPanicsOnOutOfRangeRoot 钉住失败语义:engine 对越界根坐标返回
// 输入违约,桥必须按既有 panic 文案暴露而不是静默返回空几何——空几何会被
// 生长逻辑误读成「几何为空、可以写入」。
func TestTreeBlocksPanicsOnOutOfRangeRoot(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("越界根坐标必须 panic,而不是静默返回空几何")
		}
		text, ok := recovered.(string)
		if !ok || !strings.Contains(text, "tree blocks") {
			t.Fatalf("panic 文案=%v，想要包含 tree blocks 的稳定文案", recovered)
		}
	}()
	worldgen.TreeBlocks(42, core.BlockPos{X: 0, Y: core.MaxY - 8, Z: 0})
}
