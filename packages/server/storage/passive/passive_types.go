// Package passive 承载 passive（被动牛）存档域：passive_mobs.bin 聚合文件
// （PMST 信封、schema v1）的编解码、记录字段校验与被动牛存档值类型。
//
// 本包是纯 codec 域：被动牛身体类型在本包自包含定义（权威侧身体类型属于
// `packages/server/sim`，存储不得依赖 sim），只依赖 core 值类型并经 storagedef 取
// 哨兵；不感知根包编排（DiskStore/MemoryStore 的 passive_mobs.bin 原子替换
// 与路径编排），PassiveMobStore 接口属根包存储契约家族，定义留在根包
// types.go。
package passive

import (
	"errors"

	"github.com/channing771/mornlea/packages/shared/core"
)

// ErrPassiveMobsNotFound 表示世界尚无被动牛聚合存档；调用方视同空集合。
// 根包以 var 绑定同一错误值再导出，保持既有 storage.ErrPassiveMobsNotFound
// 引用与 errors.Is 身份不变。
var ErrPassiveMobsNotFound = errors.New("storage: passive mobs not found")

// MaxPassiveMobs 是一份被动牛存档可包含的记录数上限，与权威侧的全服
// 被动牛上限同源（32）。编码与解码两侧都在解析前按本值拒绝越界。根包以
// 常量再导出保持 storage.MaxPassiveMobs 引用不变。
const MaxPassiveMobs = 32

// StoredPassiveMob 是一头被动牛在存档中的持久化记录。牛的身体事实
// 与权威侧 `physics.State` 的 position/velocity/onGround 同义，`Yaw` 是朝向
// 角，`Health` 是剩余生命。逃跑计时、出生区块与新生标记是运行时派生物，
// 刻意不落盘：恢复后逃跑清零、出生区块按加载位置重新锚定。
//
// 类型在本包内自包含定义：权威侧的被动牛身体类型属于 `packages/server/sim`
// 而存储子树不得依赖 sim（archcheck 依赖矩阵），server 装配层负责在两者
// 之间转换。
type StoredPassiveMob struct {
	// ID 是稳定非零的被动牛标识；存档内按本值严格升序排列。
	ID uint64
	// Dimension 是被动牛所在维度；当前只有 Overworld 合法。
	Dimension core.DimensionID
	// Position 与 Velocity 是世界坐标（脚底中心）与速度，全部分量必须有限，
	// 且 Position.Y 必须落在 [core.MinY, core.MaxY)。
	Position [3]float32
	Velocity [3]float32
	// OnGround 是保存时刻的着地标志。
	OnGround bool
	// Yaw 是朝向角，必须有限。
	Yaw float32
	// Health 是剩余生命值，合法区间 1..core.MaxHealth（健康牛恒正，
	// 归零即移除、不会出现在存档里）。
	Health uint8
}

// StoredPassiveMobs 是从聚合存档恢复的被动牛集合快照；记录按 ID 严格
// 升序排列。
type StoredPassiveMobs struct {
	Revision uint64
	Records  []StoredPassiveMob
}

// PassiveMobsSave 是一次被动牛集合保存请求。编码只读取载荷，绝不修改
// 调用方切片；记录顺序不必有序，编码端统一按 ID 升序写出规范形态。
type PassiveMobsSave struct {
	Revision uint64
	Records  []StoredPassiveMob
}
