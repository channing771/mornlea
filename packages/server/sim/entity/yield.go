package entity

import (
	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件原持有 entity 侧的 splitmix64 哈希链副本（作物生长/产量、毒土豆、
// 短草种子、树叶树苗掉落等判定）与 `sim/realm` 各一份的采样哈希：历史上两侧
// 逐字复制、各自漂移。变更 unified-block-updates-world-streaming 已把全部纯
// 函数哈希链收敛到 `packages/server/updates` 的随机面 `Sampler`——盐值、搅拌
// 输入顺序与次数逐位不变，由 updates 的已知答案测试钉住；entity 侧只保留
// 采掘/踩踏/掉落结算对 `Sampler` 的调用与本文件的导出入口。
//
// 结算侧的确定性语义不变：判定只依赖 (worldSeed, 域盐值, tick, 维度, 位置)，
// 不读进程级随机源、不遍历 map、零分配；同一输入重放逐位一致，掉落容量被拒
// 后的重试不可能重掷判定。

// sampler 是 entity 侧随机面判定器的统一调用点：零值 `updates.Sampler` 是
// 无状态纯函数集合，包级变量只是固定「判定真相在 updates」的写法——各结算
// 分支（采掘、踩踏、吃草、生成、漫游）都从这里取判定，不再出现第二份本地
// 哈希链。
var sampler = updates.Sampler{}

// ShortGrassSeedDropRoll 是短草采除掉落种子判定的导出入口：服务端的自然种子
// 端到端夹具需要在发送任何玩家输入前断言冻结位置的 1/8 判定必然命中，该入口
// 经 `sim/runtime` 的既有委托暴露。规则本体是 `updates.Sampler` 的
// `ShortGrassSeedDropRoll`——哈希链只有这一份真相，本函数只做转发，不引入
// 第二份常量、参数或哈希流。
func ShortGrassSeedDropRoll(
	seed int64,
	dimension core.DimensionID,
	position core.BlockPos,
) bool {
	return sampler.ShortGrassSeedDropRoll(seed, dimension, position)
}
