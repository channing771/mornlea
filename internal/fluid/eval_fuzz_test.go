package fluid

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/nativeabi"
)

// eval_fuzz_test.go：fluid eval kernel 的随机输入不变量门禁，形态沿包外
// `internal/physics/physics_fuzz_test.go` 的 FuzzStepKeepsFiniteNonOverlappingState。
//
// fuzz 不对照 oracle（那是 eval_differential_test.go 的职责），只断言对**任意**
// 7 格输入——包括未注册的方块编号——kernel 输出永远满足规则集的结构不变量：
//
//   - 确定性：同一输入两次调用逐字节一致；
//   - 自格（槽位 0）只可能写空气（非源消亡），且自格是源时槽位 0 恒无写入
//     （「源永不自然消失」）——源格自身永不被写为任何值，包括空气；
//   - 上邻（槽位 1）永不被写（规则只有向下与水平两个写入方向）；
//   - 每条流体写入的等级 ∈ 1..7（永远不会写出源方块本身或越界等级）；
//   - 写入只落在「可替换」的目标上：空气、植物（作物与短草）、开启下半门
//     或更弱的流动水（经 `Replaceable` 判定，含任意未注册编号一律按实心
//     不可替换）；
//   - 垂直写入（槽位 2）恒为等级 1；水平写入（槽位 3..6）恒为自格等级 +1
//     （源的等级读作 0，故为 1）；
//   - 自格非流体时（陈旧项）不产生任何写入。

func FuzzFluidEval(f *testing.F) {
	f.Add(uint16(27), uint16(0), uint16(0), uint16(0), uint16(0), uint16(0), uint16(0))           // 源垂直
	f.Add(uint16(27), uint16(0), uint16(2), uint16(0), uint16(0), uint16(0), uint16(0))           // 源水平
	f.Add(uint16(34), uint16(28), uint16(2), uint16(0), uint16(0), uint16(0), uint16(0))          // 等级7到界
	f.Add(uint16(30), uint16(0), uint16(0), uint16(0), uint16(0), uint16(0), uint16(0))           // 无支撑消亡
	f.Add(uint16(0), uint16(0), uint16(0), uint16(0), uint16(0), uint16(0), uint16(0))            // 陈旧项
	f.Add(uint16(28), uint16(27), uint16(2), uint16(63), uint16(62), uint16(70), uint16(0))       // 门与源邻格
	f.Add(uint16(27), uint16(0), uint16(84), uint16(0), uint16(84), uint16(33), uint16(3))        // 短草目标
	f.Add(uint16(9999), uint16(27), uint16(34), uint16(45), uint16(60), uint16(1), uint16(65535)) // 未注册编号
	f.Fuzz(func(t *testing.T, self, above, below, plusX, minusX, plusZ, minusZ uint16) {
		cells := [7]uint16{self, above, below, plusX, minusX, plusZ, minusZ}
		input := make([]byte, fluidEvalHeaderBytes+fluidEvalItemBytes)
		binary.LittleEndian.PutUint32(input[0:], fluidEvalLayoutVersion)
		binary.LittleEndian.PutUint32(input[4:], 1)
		for i, id := range cells {
			binary.LittleEndian.PutUint16(input[fluidEvalHeaderBytes+2*i:], id)
		}
		output := make([]byte, fluidEvalItemOutputBytes)
		nativeabi.FluidEvalBatch(input, output)
		repeat := make([]byte, fluidEvalItemOutputBytes)
		nativeabi.FluidEvalBatch(input, repeat)
		if !bytes.Equal(output, repeat) {
			t.Fatalf("kernel 不确定：first % x second % x", output, repeat)
		}

		selfID := core.BlockID(self)
		for j := range fluidEvalWritesPerItem {
			entry := output[j*3 : j*3+3]
			slot := entry[0]
			if slot == fluidEvalNoWriteSlot {
				continue
			}
			if slot > 6 {
				t.Fatalf("非法槽位 %d: % x", slot, output)
			}
			id := core.BlockID(binary.LittleEndian.Uint16(entry[1:3]))
			if slot == 0 {
				if selfID == core.WaterSourceID {
					t.Fatalf("源格自格永不被写（源不死）: % x", output)
				}
				if id != core.AirID {
					t.Fatalf("自格只能写空气，got %d: % x", id, output)
				}
				continue
			}
			if slot == 1 {
				t.Fatalf("上邻永不被写: % x", output)
			}
			if !core.IsFluid(id) || id == core.WaterSourceID {
				t.Fatalf("写入值必须是等级 1..7 的流动水，got %d: % x", id, output)
			}
			level := core.FluidLevel(id)
			if level < 1 || level > 7 {
				t.Fatalf("写入等级越界 %d: % x", level, output)
			}
			target := core.BlockID(cells[slot])
			if !Replaceable(target, level) {
				t.Fatalf("写入了不可替换的目标（槽位 %d 现值 %d 写等级 %d）: % x", slot, target, level, output)
			}
			if slot == 2 {
				if level != 1 {
					t.Fatalf("垂直写入恒为等级 1，got %d: % x", level, output)
				}
				continue
			}
			want := core.FluidLevel(selfID) + 1
			if level != want {
				t.Fatalf("水平写入恒为自格等级 +1（自格 %d 期待等级 %d），got %d: % x", selfID, want, level, output)
			}
		}
		if !core.IsFluid(selfID) {
			for j := range fluidEvalWritesPerItem {
				if output[j*3] != fluidEvalNoWriteSlot {
					t.Fatalf("自格非流体（陈旧项）不应产生写入: % x", output)
				}
			}
		}
	})
}
