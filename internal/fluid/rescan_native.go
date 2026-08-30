package fluid

import (
	"encoding/binary"
	"math"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/nativeabi"
)

// rescan_native.go：流体重扫扫描的 native 接线（MFL1 布局 v1，engine ABI v9）。
//
// 本文件是重扫侧唯一的 `nativeabi.FluidRescan` 调用点。盒体（中心区块区段记录
// + 裙边列）与元数据表由 sim 侧编码（realm 持有 `Dimension` 与区块数据，见
// `internal/sim/realm/environment.go` 的 `encodeRescanBox`）；本包装只负责三件
// 事：拼 26 字节 header、调 kernel 并处理 OUTPUT_OVERFLOW 的两段式扩容重试、
// 解码坐标流与 summary。盒体字节对本文件是不透明数据——区段记录的均匀性判定
// 与两类不动点捷径全部留在 kernel，Go 生产路径不再复刻 `Replaceable` 判定表
// （本体冻结在 `rules.go`，仅供 oracle 与性质测试使用）。
//
// 续扫契约：kernel 不返回显式游标，`done=false` 时续扫区段由本文件的记账重放
// 推出——kernel 只在区段入口查额度，必然停在区段边界；从 `StartSection` 起按
// 序累加每区段的记账额，第一个「入口前累计 ≥ spent」的区段即续扫起点（与 Go
// oracle `enqueueChunkFluids` 的 `>=` 检查逐字一致）。每区段记账额不是在 Go
// 里重新推导（那需要第二份可替换性判定），而是以 budget=1 对同一输入盒探测
// kernel 本身：入口检查 `0 >= 1` 恒假，kernel 必然进入并完整计账该区段，返回
// 的 `spent` 就是该段记账（捷径段 1、逐格段为列数×16）。判定的事实源只有
// kernel 一份，Go 侧重放不可能与计账分叉。

// 重扫 ABI 的布局常量，与 engine `fluid_rescan.rs` 逐字一致；两侧注释互指，
// 改布局必须同步升输入布局版本号并两侧同改。
const (
	// rescanLayoutVersion 是输入头部 layout_version 字段的唯一合法值。
	rescanLayoutVersion uint32 = 1
	// rescanHeaderBytes 是输入头部长度（字段和钉位：4+4+4+2+2+2+2+1+1+4）。
	rescanHeaderBytes = 26
	// rescanPositionBytes 是单条输出坐标字节数：u32 x、u32 y、u32 z（LE）。
	rescanPositionBytes = 12
	// rescanSummaryBytes 是输出尾部 summary 字节数：u32 spent + u8 done + u8[3] pad。
	rescanSummaryBytes = 8
	// rescanProbeBudget 是续扫重放探测单区段记账时使用的预算：入口检查
	// `spent(0) >= 1` 恒假，kernel 必然进入该区段并完整计账后停止。
	rescanProbeBudget = 1
	// rescanOutputPositionCap 是输出缓冲预分配的坐标条数上限：按预算上界
	// 全额预分配在极端预算下会试图一次性预留数十 GB，故封顶在覆盖 sim 生产
	// tunable（`FluidRescanCellsPerTick` 量级）的 2^20 条，超出部分交给两段式
	// overflow 扩容重试精确补足。
	rescanOutputPositionCap = 1 << 20
)

// RescanRegion 描述一次重扫调用的扫描区域与预算。
type RescanRegion struct {
	// Center 是被扫描区块（MFL1 盒中心区块）：五段平面各自以自己的「当前
	// 区块」为盒中心编码整盒，扫描条带永远落在盒中心列 1..16 内。
	Center core.ChunkPos
	// X0..Z1 是扫描区域的盒内局部列闭区间（0..17 含裙边；生产五段平面只传
	// 中心列 1..16）。列布局与中心区块的映射由 engine header 契约钉位。
	X0, X1, Z0, Z1 int
	// StartSection 是续扫起始区段（0..`core.SectionsPerChunk-1`）。
	StartSection int
	// Budget 是本次调用额度，区段入口检查 `spent >= budget` 即停；≤0 视同 0
	// （零进度返回未完成，镜像 Go oracle 对非正剩余额度的语义）。超过 u32
	// 上界时钳制到 u32 最大值（header 契约宽度）。
	Budget int
}

// RescanScratch 是重扫 native 调用的复用 scratch：输入拼装缓冲（header + 盒体
// + 元数据表）、输出缓冲与解码出的坐标切片。三者按需增长且跨调用复用，容量
// 稳定后单次扫描不再分配。调用方独占持有（权威 tick 单写者），返回的坐标切片
// 视图在下次调用前有效。
type RescanScratch struct {
	input     []byte
	output    []byte
	positions []core.BlockPos
}

// ScanRescanRegion 执行一次 (区块, 平面) 重扫扫描单元。
//
// box 是盒体字节（24 条区段记录 + 68 列裙边），meta 是 9 区块 × 24 区段的
// 元数据表，两者由 sim 侧按 MFL1 布局编码；本函数拼 header、调
// `nativeabi.FluidRescan` 并解码。返回产出坐标（按 kernel 扫描序，调用方据此
// `Enqueue`）、本次记账 `spent`、是否完成 `done`，以及 `done=false` 时的续扫
// 区段返回值（`done=true` 时恒 0）。OUTPUT_OVERFLOW 按两段式探测扩容重试；
// 其余非 OK 状态以稳定中文文案 panic（输入由 Go 编码、长度编码时即知，非 OK
// 只能是契约 bug）。
func ScanRescanRegion(
	box, meta []byte,
	region RescanRegion,
	scratch *RescanScratch,
) (positions []core.BlockPos, spent int, done bool, resume int) {
	budget := region.Budget
	if budget < 0 {
		budget = 0
	}
	if budget > math.MaxUint32 {
		budget = math.MaxUint32
	}
	input := rescanAssembleInput(scratch, box, meta, region, budget)
	output := rescanEnsureOutput(scratch, region, budget)
	status, written := nativeabi.FluidRescan(input, output)
	if status == nativeabi.StatusOutputOverflow {
		// 两段式探测：overflow 时 engine 把所需字节数写入 written 且不触碰
		// 输出缓冲；确定性纯函数下按所需容量重试必成功。
		output = make([]byte, written)
		scratch.output = output
		status, written = nativeabi.FluidRescan(input, output)
	}
	if status != nativeabi.StatusOK {
		panic(rescanStatusPanicText(status))
	}
	positions, spent, done = rescanDecode(scratch, output, written)
	if done {
		return positions, spent, true, 0
	}
	return positions, spent, false, rescanResumeSection(input, scratch, region.StartSection, spent)
}

// rescanAssembleInput 拼装完整输入：header 26B + 盒体 + 元数据表。header 每次
// 整体重写，续扫探测只回写 start_section/budget 两个域。
func rescanAssembleInput(
	scratch *RescanScratch,
	box, meta []byte,
	region RescanRegion,
	budget int,
) []byte {
	input := scratch.input[:0]
	var header [rescanHeaderBytes]byte
	binary.LittleEndian.PutUint32(header[0:], rescanLayoutVersion)
	binary.LittleEndian.PutUint32(header[4:], uint32(region.Center.X))
	binary.LittleEndian.PutUint32(header[8:], uint32(region.Center.Z))
	binary.LittleEndian.PutUint16(header[12:], uint16(region.X0))
	binary.LittleEndian.PutUint16(header[14:], uint16(region.X1))
	binary.LittleEndian.PutUint16(header[16:], uint16(region.Z0))
	binary.LittleEndian.PutUint16(header[18:], uint16(region.Z1))
	header[20] = byte(region.StartSection)
	header[21] = 0
	binary.LittleEndian.PutUint32(header[22:], uint32(budget))
	input = append(input, header[:]...)
	input = append(input, box...)
	input = append(input, meta...)
	scratch.input = input
	return input
}

// rescanEnsureOutput 按输出上界预留缓冲：坐标数 ≤ 记账格数，而单次调用至多
// 计账 `budget-1 + 一个区段的满额`（进入前 spent < budget，进入后整段完成），
// 区段满额由区域列数推出（布局域含裙边列，取 18×18 上界防御一般调用方）。
// 该上界再经 `rescanOutputPositionCap` 封顶；生产预算量级下正常路径一次调用
// 成功，两段式扩容重试兜住极端预算与契约防御。
func rescanEnsureOutput(scratch *RescanScratch, region RescanRegion, budget int) []byte {
	columns := (region.X1 - region.X0 + 1) * (region.Z1 - region.Z0 + 1)
	if columns <= 0 {
		columns = 0
	}
	sectionCells := columns * core.SectionSize
	bound := sectionCells
	if budget > 0 {
		bound += budget - 1
	}
	if bound > rescanOutputPositionCap {
		bound = rescanOutputPositionCap
	}
	need := bound*rescanPositionBytes + rescanSummaryBytes
	if len(scratch.output) < need {
		scratch.output = make([]byte, need)
	}
	return scratch.output
}

// rescanDecode 解码输出：坐标流每条 12B（u32 世界坐标 LE，负值按二进制补码，
// 以 int32 重读）+ 尾部 summary（u32 spent | u8 done | u8[3] pad）。
func rescanDecode(scratch *RescanScratch, output []byte, written int) ([]core.BlockPos, int, bool) {
	if written < rescanSummaryBytes || (written-rescanSummaryBytes)%rescanPositionBytes != 0 {
		panic("internal/fluid: fluid rescan 输出长度非法")
	}
	payload := written - rescanSummaryBytes
	positions := scratch.positions[:0]
	for offset := 0; offset < payload; offset += rescanPositionBytes {
		positions = append(positions, core.BlockPos{
			X: int32(binary.LittleEndian.Uint32(output[offset:])),
			Y: int32(binary.LittleEndian.Uint32(output[offset+4:])),
			Z: int32(binary.LittleEndian.Uint32(output[offset+8:])),
		})
	}
	scratch.positions = positions
	summary := output[payload:]
	spent := int(binary.LittleEndian.Uint32(summary))
	if summary[4] > 1 {
		panic("internal/fluid: fluid rescan summary 非法")
	}
	return positions, spent, summary[4] == 1
}

// rescanResumeSection 是续扫契约的记账重放：从 startSection 起依序累加每区段
// 记账，第一个「入口前累计 ≥ spent」的区段即续扫起点。区段入口检查语义与 Go
// oracle 的 `if spent >= budget` 逐字一致——预算恰好落在段边界时，下一个区段
// 尚未进入，正是续扫起点。done=false 保证存在未进入的区段，循环必然中途命中。
func rescanResumeSection(input []byte, scratch *RescanScratch, startSection, spent int) int {
	cumulative := 0
	for section := startSection; section < core.SectionsPerChunk; section++ {
		if cumulative >= spent {
			return section
		}
		input[20] = byte(section)
		cumulative += rescanProbeCharge(input, scratch)
	}
	// 不可达：done=false 蕴含 spent 恰等于某前缀和且存在未进入区段；走到这里
	// 说明 kernel 记账契约被破坏，立即暴露而不是带着错位游标静默重扫。
	panic("internal/fluid: fluid rescan 续扫游标重放失配")
}

// rescanProbeCharge 以 budget=1 探测单个区段的记账额：header 的 start_section
// 与 budget 域由调用方先行回写。探测产出的坐标流被丢弃——该区段尚未被本次
// 调用计账产出，真正的产出由续扫调用完成。
func rescanProbeCharge(input []byte, scratch *RescanScratch) int {
	binary.LittleEndian.PutUint32(input[rescanHeaderBytes-4:], rescanProbeBudget)
	status, written := nativeabi.FluidRescan(input, scratch.output)
	if status == nativeabi.StatusOutputOverflow {
		scratch.output = make([]byte, written)
		status, written = nativeabi.FluidRescan(input, scratch.output)
	}
	if status != nativeabi.StatusOK {
		panic(rescanStatusPanicText(status))
	}
	if written < rescanSummaryBytes {
		panic("internal/fluid: fluid rescan 输出长度非法")
	}
	return int(binary.LittleEndian.Uint32(scratch.output[written-rescanSummaryBytes : written-4]))
}

// rescanStatusPanicText 把非 OK 状态映射为稳定中文文案，镜像本包
// `fluidEvalStatusPanicText` 的先例。
func rescanStatusPanicText(status nativeabi.Status) string {
	switch status {
	case nativeabi.StatusABIVersion:
		return "nativeabi: fluid rescan ABI 版本不匹配"
	case nativeabi.StatusInvalidArgument:
		return "nativeabi: fluid rescan 参数非法"
	case nativeabi.StatusInput:
		return "nativeabi: fluid rescan 输入非法"
	case nativeabi.StatusOutputOverflow:
		return "nativeabi: fluid rescan output 过短"
	case nativeabi.StatusPanic:
		return "nativeabi: fluid rescan Rust panic"
	default:
		return "nativeabi: fluid rescan 未知状态"
	}
}
