// Package hostile 承载 hostile（夜行者）存档域：hostile_mobs.bin 聚合文件
// （MHST 信封、schema v1）的编解码、记录字段校验与夜行者存档值类型。
//
// 本包是纯 codec 域：夜行者身体类型在本包自包含定义（权威侧身体类型属于
// internal/sim，存储不得依赖 sim），只依赖 core 值类型并经 storagedef 取
// 哨兵；不感知根包编排（DiskStore/MemoryStore 的 hostile_mobs.bin 原子替换
// 与路径编排），HostileMobStore 接口属根包存储契约家族，定义留在根包
// types.go。
package hostile

import (
	"errors"

	"github.com/channing771/mornlea/internal/core"
)

// ErrHostileMobsNotFound 表示世界尚无夜行者聚合存档；调用方视同空集合。
// 根包以 var 绑定同一错误值再导出，保持既有 storage.ErrHostileMobsNotFound
// 引用与 errors.Is 身份不变。
var ErrHostileMobsNotFound = errors.New("storage: hostile mobs not found")

// MaxHostileMobs 是一份夜行者存档可包含的记录数上限，与权威侧的全服
// 夜行者上限同源（64）。编码与解码两侧都在解析前按本值拒绝越界。根包以
// 常量再导出保持既有 storage.MaxHostileMobs 引用不变。
const MaxHostileMobs = 64

// StoredHostileMob 是一只夜行者在存档中的持久化记录。夜行者的身体事实
// 与权威侧 `physics.State` 的 position/velocity/onGround 同义，冷却字段是
// 各自 20-tick 周期计时器的剩余值，`DistantTicks` 是远离全部玩家的累计
// active tick（达到 despawn 阈值后归零移除）。路径与规划世代是运行时
// 派生物，刻意不落盘：恢复后路径为空，首 tick 重新规划。
//
// 类型在本包内自包含定义：权威侧的夜行者身体类型属于 `internal/sim`
// 而存储子树不得依赖 sim（archcheck 依赖矩阵），server 装配层负责在两者
// 之间转换。
type StoredHostileMob struct {
	// ID 是稳定非零的夜行者标识；存档内按本值严格升序排列。
	ID uint64
	// Dimension 是夜行者所在维度；当前只有 Overworld 合法。
	Dimension core.DimensionID
	// Position 与 Velocity 是世界坐标（脚底中心）与速度，全部分量必须有限，
	// 且 Position.Y 必须落在 [core.MinY, core.MaxY)。
	Position [3]float32
	Velocity [3]float32
	// OnGround 是保存时刻的着地标志。
	OnGround bool
	// Yaw 是朝向角，必须有限。
	Yaw float32
	// Health 是剩余生命值，合法区间 1..core.MaxHealth（健康夜行者恒正，
	// 归零即移除、不会出现在存档里）。
	Health uint8
	// AttackCooldown/HurtCooldown/BurnCooldown 是三个 20-tick 周期计时器
	// 的剩余 tick，越界即损坏。
	AttackCooldown uint8
	HurtCooldown   uint8
	BurnCooldown   uint8
	// HasTarget 与 PlayerID 成对表达追逐目标：无目标时 PlayerID 必须为零
	// 值，有目标时必须是合法 UUIDv4——未用字段零值约束保证编码不丢数据、
	// round-trip 精确（companion 计划步骤同款纪律）。
	HasTarget bool
	PlayerID  core.PlayerID
	// NextRepathTicks 是持久化世界时间轴上的下一次重规划 tick；路径不落
	// 盘，但重规划节奏属于跨重启保留的 AI 事实。
	NextRepathTicks uint64
	// DistantTicks 是远离全部 active 玩家的累计 tick，合法区间 0..600。
	DistantTicks uint16
}

// StoredHostileMobs 是从聚合存档恢复的夜行者集合快照；记录按 ID 严格
// 升序排列。
type StoredHostileMobs struct {
	Revision uint64
	Records  []StoredHostileMob
}

// HostileMobsSave 是一次夜行者集合保存请求。编码只读取载荷，绝不修改
// 调用方切片；记录顺序不必有序，编码端统一按 ID 升序写出规范形态。
type HostileMobsSave struct {
	Revision uint64
	Records  []StoredHostileMob
}
