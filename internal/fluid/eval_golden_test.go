package fluid

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/nativeabi"
)

// eval_golden_test.go：fluid eval kernel 的位级确定性 golden 向量。
//
// 契约：`Queue.Advance` 的单格求值已迁入 Rust engine kernel，重放一致性要求
// 同一 7 格输入在所有平台上产出**逐字节相同**的候选写入流；oracle 差分只能锁
// 语义等价，锁不住 kernel 编码本身的位漂移（槽位序、条目排布、哨兵填充），
// 源码字面向量是唯一廉价的位级回归网——可评审、可 diff、零 I/O，任何一位
// 翻转都会在此显形。风格与 `packages/shared/physics/step_golden_vectors_test.go` 一致。
//
// 向量来源：2026-08 从当前生产路径（Go 编码 → Rust `mornlea_engine` 求值，
// 即唯一的生产行为）经 `nativeabi.FluidEvalBatch` 一次性采集，人工逐条复核
// 槽位与等级后固化为字面量。输入侧用具名 `core` 方块常量保持可读（编码是
// 确定性的 u16 LE，不值得字面量化），输出侧逐字节固化。
//
// 场景按单格求值分支逐一枚举：垂直优先、水平扩散、水平等级 +1 且源邻格
// 跳过、等级 7 到界、非源消亡写自格空气、上方流体存活 + 垂直替换弱水、
// 陈旧项空写、作物可替换（实心对照）、门四态（开启流入/关闭与上半不可）、
// `core.BarrierID` 邻格（sim 侧 scope 外读语义）、源不可替换、弱水被强水
// 水平替换。向量断言的是 kernel 原始输出，槽位序 0=自格、1=上、2=下、
// 3=+x、4=−x、5=+z、6=−z，无写条目为 FF 00 00。

func TestFluidEvalBatchGoldenVectors(t *testing.T) {
	tests := []struct {
		name string
		// cells 是槽位序的 7 格输入：0=自格、1=上、2=下、3=+x、4=−x、5=+z、6=−z。
		cells [7]core.BlockID
		// want 是 kernel 单项原始输出（12 字节 = 4 条 × 槽位 u8 + BlockID u16 LE）。
		want [fluidEvalItemOutputBytes]byte
	}{
		{
			// 源悬空：垂直优先命中，下方（槽位 2）写等级 1，本次无任何水平写入。
			name:  "源悬空垂直优先",
			cells: [7]core.BlockID{core.WaterSourceID, core.AirID, core.AirID, core.AirID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x02, 0x1c, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 源下方实心：进入水平分支，四个水平槽位（3..6）各写等级 1。
			name:  "源下方实心水平扩散",
			cells: [7]core.BlockID{core.WaterSourceID, core.AirID, core.StoneID, core.AirID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x03, 0x1c, 0x00, 0x04, 0x1c, 0x00, 0x05, 0x1c, 0x00, 0x06, 0x1c, 0x00},
		},
		{
			// 等级 3 存活（+z 的源既撑存活又不可替换）：其余三个水平方向写等级 4。
			name:  "等级3水平传播等级4且源邻格跳过",
			cells: [7]core.BlockID{core.WaterLevel3ID, core.AirID, core.StoneID, core.AirID, core.AirID, core.WaterSourceID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x03, 0x1f, 0x00, 0x04, 0x1f, 0x00, 0x06, 0x1f, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 等级 7 到界：上方流体保活，但等级 +1 越过 7，不产生任何写入。
			name:  "等级7到界不传播",
			cells: [7]core.BlockID{core.WaterLevel7ID, core.WaterLevel1ID, core.StoneID, core.AirID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0xff, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 非源无支撑：自格（槽位 0）写空气，不再传播。
			name:  "非源无支撑消亡",
			cells: [7]core.BlockID{core.WaterLevel3ID, core.AirID, core.AirID, core.AirID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x00, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 上方流体保活且下方是更弱流动水：垂直优先把下方替换为等级 1。
			name:  "上方流体存活垂直替换弱水",
			cells: [7]core.BlockID{core.WaterLevel2ID, core.WaterLevel1ID, core.WaterLevel5ID, core.AirID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x02, 0x1c, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 陈旧项：自格非流体，产出空写集（与非源消亡是两回事）。
			name:  "陈旧项空写",
			cells: [7]core.BlockID{core.AirID, core.AirID, core.AirID, core.AirID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0xff, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 作物可替换、实心不可：+x 小麦与 +z 马铃薯被写等级 1，−x 石头跳过。
			name:  "作物与实心混合邻格",
			cells: [7]core.BlockID{core.WaterSourceID, core.AirID, core.StoneID, core.WheatStage2ID, core.StoneID, core.PotatoStage5ID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x03, 0x1c, 0x00, 0x05, 0x1c, 0x00, 0x06, 0x1c, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 门四态：开启下半门（+x）可流入，关闭下半门（−x）与上半门（+z）不可。
			name:  "门四态",
			cells: [7]core.BlockID{core.WaterSourceID, core.AirID, core.StoneID, core.DoorLowerSouthOpen, core.DoorLowerNorthClosed, core.DoorUpper, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x03, 0x1c, 0x00, 0x06, 0x1c, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// Barrier 邻格：下方与 +x 均为 Barrier（sim 侧 scope 外读语义），
			// 视作实心：垂直被挡，水平只写其余三个空气方向。
			name:  "Barrier邻格",
			cells: [7]core.BlockID{core.WaterSourceID, core.AirID, core.BarrierID, core.BarrierID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x04, 0x1c, 0x00, 0x05, 0x1c, 0x00, 0x06, 0x1c, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 源不可替换：+x 是源（兼作存活支撑），水平只写其余三向的等级 3。
			name:  "源不可替换",
			cells: [7]core.BlockID{core.WaterLevel2ID, core.AirID, core.StoneID, core.WaterSourceID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x04, 0x1e, 0x00, 0x05, 0x1e, 0x00, 0x06, 0x1e, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 弱水被强水水平替换：等级 1（上方源保活）向四向写等级 2，
			// +x 原有的等级 5 弱水被覆盖。
			name:  "弱水被强水水平替换",
			cells: [7]core.BlockID{core.WaterLevel1ID, core.WaterSourceID, core.StoneID, core.WaterLevel5ID, core.AirID, core.AirID, core.AirID},
			want:  [fluidEvalItemOutputBytes]byte{0x03, 0x1d, 0x00, 0x04, 0x1d, 0x00, 0x05, 0x1d, 0x00, 0x06, 0x1d, 0x00},
		},
		{
			// 短草可替换（垂直分支）：下方短草触发垂直优先，只写下方一条等级 1；
			// 掉落侧零产出由 sim 写入侧保证，kernel 只负责放行写入。
			name:  "短草下方垂直优先",
			cells: [7]core.BlockID{core.WaterSourceID, core.AirID, core.ShortGrassID, core.StoneID, core.StoneID, core.StoneID, core.StoneID},
			want:  [fluidEvalItemOutputBytes]byte{0x02, 0x1c, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00, 0xff, 0x00, 0x00},
		},
		{
			// 短草可替换（水平分支）：+x 与 −z 是短草、−x 是草方块（挡水对照），
			// 水平只写空气与短草三个方向，草方块方向跳过。
			name:  "短草水平可替换草方块挡水",
			cells: [7]core.BlockID{core.WaterSourceID, core.AirID, core.StoneID, core.ShortGrassID, core.GrassID, core.AirID, core.ShortGrassID},
			want:  [fluidEvalItemOutputBytes]byte{0x03, 0x1c, 0x00, 0x05, 0x1c, 0x00, 0x06, 0x1c, 0x00, 0xff, 0x00, 0x00},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := make([]byte, fluidEvalHeaderBytes+fluidEvalItemBytes)
			binary.LittleEndian.PutUint32(input[0:], fluidEvalLayoutVersion)
			binary.LittleEndian.PutUint32(input[4:], 1)
			for i, id := range test.cells {
				binary.LittleEndian.PutUint16(input[fluidEvalHeaderBytes+2*i:], uint16(id))
			}
			output := make([]byte, fluidEvalItemOutputBytes)
			nativeabi.FluidEvalBatch(input, output)
			if !bytes.Equal(output, test.want[:]) {
				t.Fatalf("kernel 输出=% x，want % x", output, test.want)
			}
		})
	}
}
