package fluid

import (
	"encoding/binary"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/nativeabi"
)

// eval_native.go：`Queue.Advance` 单格求值的 native 批量接线。
//
// 本文件是 `internal/fluid` 内唯一的 nativeabi 调用点：阶段一弹出至多
// budget 个到期项时逐项把 7 格邻域编码进 `Queue.evalInput`，循环结束后一次
// `nativeabi.FluidEvalBatch` 完成全部求值，再解码输出、按项还原绝对坐标并
// 经 `strongerWrite` 并入候选写入集合。队列调度、到期检查、预算计数、
// 冲突合并与排序提交全部留在 Go 侧；kernel 只做逐项无状态纯函数求值。
//
// 布局契约见 `nativeabi.FluidEvalBatch` 的注释与 engine 侧 fluid_eval 模块
// （ABI v9）：输入 = u32 layout_version=1 + u32 item_count + 每项 7×u16 LE；
// 输出 = 每项 12 字节 = 4 条候选写入 ×（目标槽位 u8 + BlockID u16 LE），
// 无写槽位为 0xFF 哨兵。

// 流体 eval ABI 的布局常量，与 engine `fluid_eval.rs` 逐字一致；两侧注释互指，
// 改布局必须同步升输入布局版本号并两侧同改。
const (
	// fluidEvalLayoutVersion 是输入头部 layout_version 字段的唯一合法值。
	fluidEvalLayoutVersion uint32 = 1
	// fluidEvalHeaderBytes 是输入头部长度：layout_version + item_count 两个 u32。
	fluidEvalHeaderBytes = 8
	// fluidEvalItemBytes 是单项输入字节数：7 个 u16 LE 槽位。
	fluidEvalItemBytes = 14
	// fluidEvalItemOutputBytes 是单项输出字节数：4 条候选写入 × 3 字节。
	fluidEvalItemOutputBytes = 12
	// fluidEvalWritesPerItem 是单项输出的候选写入条数上限（自格消亡 1 条、
	// 垂直优先 1 条或水平传播至多 4 条）。
	fluidEvalWritesPerItem = 4
	// fluidEvalNoWriteSlot 是输出条目「无写入」哨兵的槽位字节。
	fluidEvalNoWriteSlot byte = 0xFF
)

// fluidEvalZeroHeader 与 fluidEvalZeroItem 是 scratch 扩容时的零值模板：
// 用包级数组而非 `make` 追加，保证预热后的编码路径一次分配都不发生。
var (
	fluidEvalZeroHeader [fluidEvalHeaderBytes]byte
	fluidEvalZeroItem   [fluidEvalItemBytes]byte
)

// evalSlotOffsets 把输出槽位翻译为相对位移，与 `sixNeighbors` 的返回序逐槽
// 对齐：0=自格、1=上(+Y)、2=下(−Y)、3=+x、4=−x、5=+z、6=−z。kernel 侧
// 的水平槽位表用的是同一张序；改槽位序等于改 ABI。
var evalSlotOffsets = [7][3]int32{
	{0, 0, 0},
	{0, 1, 0},
	{0, -1, 0},
	{1, 0, 0},
	{-1, 0, 0},
	{0, 0, 1},
	{0, 0, -1},
}

// evalEncodeItem 把 pos 的 7 格邻域按槽位序编码进 dst（u16 LE），返回编码
// 字节数（恒为 `fluidEvalItemBytes`）。读取只经 `w.BlockAt`，不做任何 scope
// 判断：sim 生产侧 scope 外的格由 `fluidWorld.BlockAt` 读作 `BarrierID`，
// Barrier 的「实心不可替换」语义由此天然进入编码，测试侧 `memWorld` 未写入
// 的格则读作空气——编码器对两者一视同仁。
func evalEncodeItem(w FluidWorld, pos core.BlockPos, dst []byte) int {
	n := sixNeighbors(pos)
	cells := [7]core.BlockID{
		w.BlockAt(pos),
		w.BlockAt(n[0]), w.BlockAt(n[1]), // 上、下
		w.BlockAt(n[2]), w.BlockAt(n[3]), w.BlockAt(n[4]), w.BlockAt(n[5]), // +x、−x、+z、−z
	}
	for i, id := range cells {
		binary.LittleEndian.PutUint16(dst[i*2:], uint16(id))
	}
	return fluidEvalItemBytes
}

// beginEvalBatch 重置求值 scratch 并预留输入头部。item_count 在收批时按实际
// 编码项数回填——头部只是占位，先于条目存在是为了让条目编码始终是纯追加。
func (q *Queue) beginEvalBatch() {
	q.evalInput = append(q.evalInput[:0], fluidEvalZeroHeader[:]...)
	q.evalPositions = q.evalPositions[:0]
}

// enqueueEvalItem 把一条弹出项编码进 `evalInput` 并记录其绝对坐标。
// 绝对坐标是解码阶段的唯一位置来源：输出只携带 0..6 的相对槽位，还原目标
// 格必须知道「这一项是谁」。
func (q *Queue) enqueueEvalItem(w FluidWorld, pos core.BlockPos) {
	start := len(q.evalInput)
	q.evalInput = append(q.evalInput, fluidEvalZeroItem[:]...)
	evalEncodeItem(w, pos, q.evalInput[start:])
	q.evalPositions = append(q.evalPositions, pos)
}

// finishEvalBatch 回填输入头部、确保输出容量，一次性调用
// `nativeabi.FluidEvalBatch` 并解码输出：逐项把 4 条候选写入的相对槽位还原为
// 绝对坐标，经 `strongerWrite` 并入 pendingWrites（与迁移前逐项 `evalCell`
// 的合并语义一致，见 `Advance` 内的说明）。没有任何弹出项时直接返回，不发
// 空批调用。
func (q *Queue) finishEvalBatch(pendingWrites map[core.BlockPos]core.BlockID) {
	count := len(q.evalPositions)
	if count == 0 {
		return
	}
	binary.LittleEndian.PutUint32(q.evalInput[0:], fluidEvalLayoutVersion)
	binary.LittleEndian.PutUint32(q.evalInput[4:], uint32(count))
	need := count * fluidEvalItemOutputBytes
	if cap(q.evalOutput) < need {
		q.evalOutput = make([]byte, need)
	}
	output := q.evalOutput[:need]
	nativeabi.FluidEvalBatch(q.evalInput, output)
	for i, base := range q.evalPositions {
		entry := output[i*fluidEvalItemOutputBytes : (i+1)*fluidEvalItemOutputBytes]
		for j := range fluidEvalWritesPerItem {
			e := entry[j*3 : j*3+3]
			slot := e[0]
			if slot == fluidEvalNoWriteSlot {
				continue
			}
			off := evalSlotOffsets[slot]
			target := core.BlockPos{X: base.X + off[0], Y: base.Y + off[1], Z: base.Z + off[2]}
			id := core.BlockID(binary.LittleEndian.Uint16(e[1:3]))
			if existing, ok := pendingWrites[target]; ok {
				pendingWrites[target] = strongerWrite(existing, id)
			} else {
				pendingWrites[target] = id
			}
		}
	}
}
