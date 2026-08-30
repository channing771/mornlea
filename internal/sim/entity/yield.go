package entity

import (
	"github.com/channing771/mornlea/internal/core"
)

// 本文件实现作物的**随机 tick 推进**：抽样、生长规则与作物环境判定。
//
// # 为什么是随机 tick 而不是显式待更新队列（design.md D3）
//
// 流体用队列，作物用抽样，差别来自两者的容错性质而不是口味：
//
//   - 流体是**向平衡态收敛**的。漏掉一次更新，水就永远停在错误状态，因此必须
//     有队列保证「该更新的格一个都不能丢」，代价是队列规模随水体规模增长
//     （瀑布场景实测稳定在 50 万项）。
//   - 作物是**按时间推进**的。漏抽一次只是这株麦子长得慢一点，不破坏任何
//     不变量。于是它可以用一个成本与作物数量完全无关的机制：每 tick 只在每个
//     区段里抽固定几格看一眼。
//
// 这正是 spec「单个 tick 内被考察的格数 MUST 只正比于活动兴趣范围内的区段数，
// MUST NOT 随世界中作物的数量增长」的实现依据。被否决的替代是「每格存下次生长
// tick」：要么每 tick 扫全部已加载作物格（无界），要么还得为它建索引——绕回队列。
//
// # 确定性
//
// 抽样与生长概率都是 (worldSeed, tick, 位置) 的纯整数哈希，不用 math/rand、
// 不用浮点、不遍历 map 决定顺序。枚举顺序由 activeInterestKeys()（已按
// chunkKeyLess 全序）与区段索引升序共同固定。

// splitmix64 是 SplittableRandom 的 mix64 终结器。
//
// 选它而不是 FNV-1a 或 xorshift，理由有三条：
//
//  1. 它是**纯整数**的（加、乘、异或、右移），没有任何浮点运算，在所有平台上
//     逐位一致——权威模拟的确定性要求哈希结果不能有平台差异。
//  2. 雪崩性质好且**低位也好**。抽样最后要取 mod 4096，用的正是最低 12 位；
//     FNV-1a 的低位雪崩很弱，直接取低 12 位会出现可见的条带，而 splitmix64 的
//     两轮 xor-shift-multiply 让每一位都依赖全部输入位。
//  3. 实现只有四行、没有状态、可被逐字复核——权威路径上的哈希越无聊越好。
//
// 三个常量分别是 2^64/φ（黄金比例倒数）与 splitmix64 原始论文给出的两个乘子，
// 不得随意替换：换掉它们会改变全世界的生长抽样序列。
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// cropSectionHash 把 (worldSeed, tick, 维度, 区块坐标, 区段索引) 折叠成该区段
// 本 tick 的抽样基准哈希。
//
// 逐个输入串行折叠而不是先拼成一个整数再哈希一次：拼接要为每个字段划定位宽，
// 而区块坐标是 32 位有符号、tick 是 64 位，拼不进一个 uint64；串行折叠没有
// 位宽预算，也不会因为某两个字段互换而产生相同结果。
//
// 有符号量一律先转 uint32 再零扩展，保证负坐标（例如 X=-7）与它的补码表示
// 一一对应，不会因为符号扩展与另一个正坐标撞车。
func cropSectionHash(seed int64, tick uint64, key core.ChunkKey, sectionY int) uint64 {
	hash := splitmix64(uint64(seed))
	hash = splitmix64(hash ^ tick)
	hash = splitmix64(hash ^ uint64(uint32(key.Dimension)))
	hash = splitmix64(hash ^ uint64(uint32(key.Pos.X)))
	hash = splitmix64(hash ^ uint64(uint32(key.Pos.Z)))
	return splitmix64(hash ^ uint64(uint32(sectionY)))
}

// sampleCells 返回 (key, sectionY) 这个区段在 tick 上要考察的 n 个格下标。
//
// 下标是区段内的紧凑编号 y*256 + z*16 + x，取值 0..4095，与
// world.ChunkBlockIndex 在区段内使用的编号规则一致。
//
// 结果写入 out[:0] 并返回：调用方（advanceCrops）复用同一个 scratch，因此这条
// 每 tick 执行「区块数 × 24」次的路径上没有任何分配。传 nil 时按需分配，测试
// 因此可以直接 sampleCells(..., nil)。
//
// **允许同一 tick 抽到重复下标**（与 MC 的随机 tick 同）：去重要引入 map 或
// 排序，成本高于「偶尔白看一格」的收益，而且重复只是让本 tick 的有效抽样数
// 略少，不破坏任何不变量。
func sampleCells(seed int64, tick uint64, key core.ChunkKey, sectionY, n int, out []int) []int {
	cells := out[:0]
	if n <= 0 {
		return cells
	}
	base := cropSectionHash(seed, tick, key, sectionY)
	for index := range n {
		// 每条抽样再过一次 splitmix64：base 只算一次并在 n 条之间摊薄，
		// 而 index 的低位差异经这一轮后已经完全雪崩开。
		cells = append(cells, int(splitmix64(base^uint64(index))%core.BlocksPerSection))
	}
	return cells
}

// cropGrowthRollSalt 让生长概率的哈希流与抽样的哈希流互相独立。
//
// 没有它，两者会在相同的 (seed, tick) 前缀上同源，「这一格被抽中」与「这一格
// 通过概率判定」之间就可能出现结构性相关——例如某些区段永远抽中却永远不通过。
const cropGrowthRollSalt = 0xc0ffee5eedca11ed

// cropGrowthRoll 报告 position 上的作物在 tick 上是否通过生长概率判定。
//
// 与抽样同源的设计：纯整数哈希、无全局 RNG、无浮点，结果只依赖
// (worldSeed, tick, 维度, 方块坐标)，因此重放同一段 tick 必然得到同一串判定。
//
// **维度必须与坐标一起折进哈希**，理由与 cropSectionHash 折维度完全相同：
// core.BlockPos 不携带维度，不折的话两个维度里坐标相同的作物每 tick 拿到逐位
// 相同的概率判定，两个世界的生长会同步得一模一样，而任何单维度测试都照样全绿。
//
// `hash % 100` 有理论上的取模偏差（2^64 不是 100 的整数倍），最大偏差量级约
// 1e-17，远小于任何可观察的游戏效果，不值得为它引入拒绝采样循环——而拒绝采样
// 的循环次数不定，反而给权威 tick 引入一个无上界的分支。
func cropGrowthRoll(
	seed int64,
	tick uint64,
	dimension core.DimensionID,
	position core.BlockPos,
	chancePercent uint8,
) bool {
	// 0% 与 100% 单独短路：它们是测试与调试面板最常用的两个端点值，短路让
	// 「设成 100 必长」这条测试前提不依赖哈希分布。
	if chancePercent == 0 {
		return false
	}
	if chancePercent >= 100 {
		return true
	}
	hash := splitmix64(uint64(seed) ^ cropGrowthRollSalt)
	hash = splitmix64(hash ^ tick)
	hash = splitmix64(hash ^ uint64(uint32(dimension)))
	hash = splitmix64(hash ^ uint64(uint32(position.X)))
	hash = splitmix64(hash ^ uint64(uint32(position.Y)))
	hash = splitmix64(hash ^ uint64(uint32(position.Z)))
	return hash%100 < uint64(chancePercent)
}

// cropYieldRollSalt 让收获产量的哈希流与生长判定的哈希流互相独立。
//
// 理由与 `cropGrowthRollSalt` 完全同构：没有它，「这一格通过生长判定」与「这一格
// 成熟后掉多少」会在相同的 (seed, tick) 前缀上同源，可能出现结构性相关——例如某
// 些格子永远长得快又永远高产，而任何单维度的分布测试都照样全绿。产量是玩家每季
// 收成都要读的数值，它的随机性必须与生长节奏解耦，故单独占一个常量。
const cropYieldRollSalt = 0x5eedfeedfaceface

// cropYieldRolls 返回 position 上一株成熟作物在 tick 结算收获时的两类产物数量：
// 小麦与种子各一个，取值都落在闭区间 [1,3]。
//
// 折叠方式逐字复用 `cropGrowthRoll` 的 `splitmix64` 链式模式（design.md D1）：
// 纯整数、无浮点、无全局 RNG、零分配，结果只依赖 (worldSeed, tick, 维度, 方块
// 坐标)，因此同一株作物在同一权威 tick 上重新结算必然得到同一串数量——重放契约
// 不依赖任何进程级随机源或哈希遍历顺序。调用点只有 `completeMining` 的成熟小麦
// 分支，每次收获结算恰好调用一次。
//
// **维度必须与坐标一起折进哈希**，理由与 `cropGrowthRoll` 相同：`core.BlockPos`
// 不携带维度，不折的话两个维度里坐标相同的作物会拿到逐位相同的产量序列，两个
// 世界的收成同步得一模一样。
//
// 两类数量是**两次独立的抽取**，不是同一个余数的两种解读：小麦取自六折后的哈希，
// 种子取自继续折叠一次后的新哈希——链上多过一轮 `splitmix64` 后低位已完全雪崩，
// 两个值不同源，不存在「小麦高产则种子必高产」的可利用相关性。
//
// `% 3` 有理论上的取模偏差（2^64 不是 3 的整数倍）：每个余数的概率偏离理想值至
// 多 1 个计数（绝对偏差上界 `1/2^64`），远小于任何可观察的游戏效果，不值得为它
// 引入拒绝采样循环——拒绝采样的循环次数不定，反而给权威结算路径引入无上界分支。
//
// 种子下限 1 是规格条款「始终不亏种子」的实现：任何一次成熟收获都至少返还一颗
// 种子，耕种循环不会因随机性中断（接替 authoritative-farming design.md D9 的
// 固定掉落决策）。
func cropYieldRolls(
	seed int64,
	tick uint64,
	dimension core.DimensionID,
	position core.BlockPos,
) (wheat uint8, seeds uint8) {
	hash := splitmix64(uint64(seed) ^ cropYieldRollSalt)
	hash = splitmix64(hash ^ tick)
	hash = splitmix64(hash ^ uint64(uint32(dimension)))
	hash = splitmix64(hash ^ uint64(uint32(position.X)))
	hash = splitmix64(hash ^ uint64(uint32(position.Y)))
	hash = splitmix64(hash ^ uint64(uint32(position.Z)))
	wheat = uint8(hash%3) + 1
	hash = splitmix64(hash)
	seeds = uint8(hash%3) + 1
	return wheat, seeds
}

// cropYieldPotatoSalt 让马铃薯产量的哈希流与小麦及胡萝卜互相独立。
const cropYieldPotatoSalt = 0x70a70a515eedface

// cropYieldCarrotSalt 让胡萝卜产量的哈希流与马铃薯及小麦互相独立。
const cropYieldCarrotSalt = 0xca7707701ace5eed

// poisonPotatoSalt 让毒土豆 2% 判定的哈希流与两种作物的产量流互相独立。
const poisonPotatoSalt = 0xdeadbeefcafe1234

// cropYieldRollsPotato 返回位置上成熟马铃薯在 tick 结算时的块数，落在 [1,4]。
//
// 链式 splitmix64 形状复用 `cropYieldRolls` 的确定性模式，仅 salt 不同，保证
// 马铃薯与胡萝卜、小麦及毒土豆判定相互独立，可重放。
func cropYieldRollsPotato(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
	hash := splitmix64(uint64(seed) ^ cropYieldPotatoSalt)
	hash = splitmix64(hash ^ tick)
	hash = splitmix64(hash ^ uint64(uint32(dim)))
	hash = splitmix64(hash ^ uint64(uint32(pos.X)))
	hash = splitmix64(hash ^ uint64(uint32(pos.Y)))
	hash = splitmix64(hash ^ uint64(uint32(pos.Z)))
	return uint8(hash%4) + 1
}

// cropYieldRollsCarrot 返回位置上成熟胡萝卜在 tick 结算时的块数，落在 [1,4]。
func cropYieldRollsCarrot(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
	hash := splitmix64(uint64(seed) ^ cropYieldCarrotSalt)
	hash = splitmix64(hash ^ tick)
	hash = splitmix64(hash ^ uint64(uint32(dim)))
	hash = splitmix64(hash ^ uint64(uint32(pos.X)))
	hash = splitmix64(hash ^ uint64(uint32(pos.Y)))
	hash = splitmix64(hash ^ uint64(uint32(pos.Z)))
	return uint8(hash%4) + 1
}

// poisonRoll 报告位置上成熟马铃薯是否额外掉落 1 个毒土豆（2% 概率）。
//
// 同为 splitmix64 链，用独立 salt 保证与产量流解耦；`hash%50==0` 即 2%。
func poisonRoll(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) bool {
	hash := splitmix64(uint64(seed) ^ poisonPotatoSalt)
	hash = splitmix64(hash ^ tick)
	hash = splitmix64(hash ^ uint64(uint32(dim)))
	hash = splitmix64(hash ^ uint64(uint32(pos.X)))
	hash = splitmix64(hash ^ uint64(uint32(pos.Y)))
	hash = splitmix64(hash ^ uint64(uint32(pos.Z)))
	return hash%50 == 0
}

// growCrop 是生长规则本身：给定方块与它所处的环境，返回下一个方块编号与是否
