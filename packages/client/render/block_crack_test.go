package render

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestBlockCrackStageBoundaries 钉住进度→阶段的权威映射公式
// `min(9, floor(clamp(progressTicks/requiredTicks, 0, 1) × 10))`：
// requiredTicks 为 0 返回无效哨兵；进度恰落在 1/10 的整数倍边界时按 f32
// 除法/乘法结果稳定进入新阶段（例如 3/30 稳定得到第 1 阶段）；进度比例
// 饱和到 1（含 progress 超过 required 的钳制）必须是第 9 阶段。
func TestBlockCrackStageBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		progress uint16
		required uint16
		want     int
	}{
		{"required 为零是无效哨兵", 0, 0, BlockCrackStageNone},
		{"progress 非零但 required 为零仍是哨兵", 5, 0, BlockCrackStageNone},
		{"零进度是第 0 阶段", 0, 30, 0},
		{"首 tick 仍是第 0 阶段", 1, 30, 0},
		{"恰在 1/10 边界进入第 1 阶段", 3, 30, 1},
		{"区段中部取整向下", 4, 30, 1},
		{"恰在 3/10 边界进入第 3 阶段", 9, 30, 3},
		{"恰在半程边界进入第 5 阶段", 15, 30, 5},
		{"恰在 7/10 边界进入第 7 阶段", 21, 30, 7},
		{"接近完成是第 9 阶段", 29, 30, 9},
		{"完成进度饱和到第 9 阶段", 30, 30, 9},
		{"progress 超过 required 钳制到第 9 阶段", 31, 30, 9},
		{"单 tick 方块直接最深", 1, 1, 9},
		{"满量程输入稳定", 65535, 65535, 9},
		{"十分之一整倍数边界稳定（1,10）", 1, 10, 1},
		{"十分之一整倍数边界稳定（9,10）", 9, 10, 9},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := BlockCrackStage(test.progress, test.required); got != test.want {
				t.Fatalf("BlockCrackStage(%d, %d) = %d，想要 %d",
					test.progress, test.required, got, test.want)
			}
		})
	}
}

// TestEncodeBlockCrackInstancesByteLayout 钉住裂纹实例的跨语言字节契约：
// 恰 80 字节；0..63 是「方块原点平移 × 中心平移 0.5 × 各向外扩 0.001
// （合成边长 1.002）」的 mat4；64..68 是 little-endian f32 的
// `assets.LayerCrack0 + stage` atlas 层号；68..80 零填充。
func TestEncodeBlockCrackInstancesByteLayout(t *testing.T) {
	encoder := &InstanceEncoder{}
	crack := BlockCrack{
		Visible:  true,
		Position: core.BlockPos{X: 4, Y: -2, Z: 6},
		Stage:    3,
	}
	stream := encoder.EncodeBlockCrackInstances(nil, crack)
	if len(stream) != blockCrackInstanceBytes {
		t.Fatalf("裂纹流=%d 字节，想要 %d", len(stream), blockCrackInstanceBytes)
	}
	readF32 := func(offset int) float32 {
		return math.Float32frombits(binary.LittleEndian.Uint32(stream[offset:]))
	}

	// mat4 与几何构建函数逐元素一致；平移分量必须含方块原点与 0.5 中心
	// 偏移（4.5/-1.5/6.5），主对角缩放为外扩后的 1.002。
	want := mgl32.Translate3D(4, -2, 6).
		Mul4(mgl32.Translate3D(0.5, 0.5, 0.5)).
		Mul4(mgl32.Scale3D(1+2*blockCrackExpand, 1+2*blockCrackExpand, 1+2*blockCrackExpand))
	for index, value := range want {
		if got := readF32(index * 4); got != value {
			t.Fatalf("mat4[%d] = %v，想要 %v", index, got, value)
		}
	}
	if got := readF32(12 * 4); got != 4.5 {
		t.Fatalf("平移 X = %v，想要 4.5", got)
	}
	if got := readF32(13 * 4); got != -1.5 {
		t.Fatalf("平移 Y = %v，想要 -1.5", got)
	}
	if got := readF32(14 * 4); got != 6.5 {
		t.Fatalf("平移 Z = %v，想要 6.5", got)
	}
	if got := readF32(0); got != 1+2*blockCrackExpand {
		t.Fatalf("缩放 m[0] = %v，想要 %v", got, 1+2*blockCrackExpand)
	}

	// 64..68 是 atlas 层号 f32：LayerCrack0 + stage，不复制常量。
	wantLayer := float32(int(assets.LayerCrack0) + crack.Stage)
	if got := readF32(64); got != wantLayer {
		t.Fatalf("atlas 层号 = %v，想要 %v", got, wantLayer)
	}
	// 尾部 12 字节必须为零填充。
	for offset, b := range stream[68:] {
		if b != 0 {
			t.Fatalf("零填充字节 %d = %#x", offset, b)
		}
	}
}

// TestEncodeBlockCrackInstancesHiddenReturnsEmptyStream 钉住「不可见或阶段
// 无效即空流」：这是无裂纹帧字节与变更前逐位一致的前提；同时钉住复用缓冲
// 时陈旧字节不得漏进零填充区（`growEncodeBuffer` 不清零旧内容）。
func TestEncodeBlockCrackInstancesHiddenReturnsEmptyStream(t *testing.T) {
	encoder := &InstanceEncoder{}
	stale := make([]byte, blockCrackInstanceBytes)
	for index := range stale {
		stale[index] = 0xFF
	}
	if got := encoder.EncodeBlockCrackInstances(stale, BlockCrack{}); len(got) != 0 {
		t.Fatalf("不可见裂纹流=%d 字节，想要空", len(got))
	}
	for _, stage := range []int{BlockCrackStageNone, -2, 10, 99} {
		crack := BlockCrack{Visible: true, Position: core.BlockPos{X: 1, Y: 2, Z: 3}, Stage: stage}
		if got := encoder.EncodeBlockCrackInstances(stale, crack); len(got) != 0 {
			t.Fatalf("stage=%d 的裂纹流=%d 字节，想要空", stage, len(got))
		}
	}

	// 复用充满 0xFF 的缓冲编码可见裂纹：尾部 12 字节仍必须是零。
	crack := BlockCrack{Visible: true, Position: core.BlockPos{X: 0, Y: 0, Z: 0}, Stage: 0}
	stream := encoder.EncodeBlockCrackInstances(stale, crack)
	if len(stream) != blockCrackInstanceBytes {
		t.Fatalf("裂纹流=%d 字节，想要 %d", len(stream), blockCrackInstanceBytes)
	}
	for offset, b := range stream[68:] {
		if b != 0 {
			t.Fatalf("复用缓冲后零填充字节 %d = %#x", offset, b)
		}
	}
}

// TestEncodeBlockCrackInstancesZeroAllocation 钉住稳定呈现的固定有界：预热
// 后每帧更新可见性并重编码不产生堆分配（单一可复用实例容量恰为 1）。
func TestEncodeBlockCrackInstancesZeroAllocation(t *testing.T) {
	encoder := &InstanceEncoder{}
	visible := BlockCrack{Visible: true, Position: core.BlockPos{X: 1, Y: 2, Z: 3}, Stage: 4}
	stream := encoder.EncodeBlockCrackInstances(nil, visible)
	if len(stream) != blockCrackInstanceBytes {
		t.Fatalf("预热裂纹流=%d 字节，想要 %d", len(stream), blockCrackInstanceBytes)
	}
	run := func() {
		stream = encoder.EncodeBlockCrackInstances(stream, visible)
		stream = encoder.EncodeBlockCrackInstances(stream, BlockCrack{})
		stream = encoder.EncodeBlockCrackInstances(stream, visible)
	}
	if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
		t.Fatalf("稳定裂纹路径分配=%v，想要 0", allocations)
	}
}
