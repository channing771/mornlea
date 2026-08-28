// Package player 承载 player 存档域：MCPL 信封编解码、schema v1..v8 迁移链
// 与玩家存档值类型。
//
// 本包是纯 codec 域：只依赖 core 值类型与 storagedef 哨兵，不感知根包编排
// （DiskStore/MemoryStore 的 player 文件读写）或其他域子包；`PlayerStore`
// 接口属根包存储契约家族，定义留在根包 types.go。
package player

import (
	"github.com/channing771/mornlea/internal/core"
)

type PlayerLocation struct {
	Dimension core.DimensionID
	Position  [3]float32
}

type StoredPlayer struct {
	PlayerID    core.PlayerID
	Revision    uint64
	DisplayName string
	Current     PlayerLocation
	Yaw, Pitch  float32
	Safe        *PlayerLocation
	Inventory   core.Inventory
	// Health 是权威生命值，0..core.MaxHealth。
	Health uint8
	// Hunger、SaturationMilli、ExhaustionMilli 是三层权威饥饿状态，schema v7 起
	// 落盘；更旧的存档读入时按 core 的固定初值迁移。
	Hunger          uint8
	SaturationMilli uint16
	ExhaustionMilli uint16
	// RespawnPresent 报告存档是否携带个人重生点（床尾格），schema v8 起落盘；
	// 更旧的存档读入时迁移为「无重生点」，死亡重生沿用世界出生锚点。
	RespawnPresent bool
	// RespawnPosition 是重生点床尾格的方块坐标（X/Y/Z 逐分量 float32）。
	RespawnPosition [3]float32
	// RespawnDimension 是重生点所在的维度。
	RespawnDimension core.DimensionID
	NeedsRewrite     bool
}

type PlayerSave struct {
	PlayerID    core.PlayerID
	Revision    uint64
	DisplayName string
	Current     PlayerLocation
	Yaw, Pitch  float32
	Safe        *PlayerLocation
	Inventory   core.Inventory
	// Health 是权威生命值，0..core.MaxHealth。
	Health uint8
	// Hunger、SaturationMilli、ExhaustionMilli 是三层权威饥饿状态，schema v7 起
	// 落盘。三者都可能合法地为零（饿到零、无饱和、无疲劳），因此**没有**
	// "零值代表缺失"这种约定：缺失只可能来自更旧的 schema，由迁移链补初值。
	Hunger          uint8
	SaturationMilli uint16
	ExhaustionMilli uint16
	// RespawnPresent 为真时 RespawnPosition/RespawnDimension 携带个人重生点
	// （床尾格），schema v8 起落盘；为假时位置与维度不携带语义，编码规范为零。
	RespawnPresent   bool
	RespawnPosition  [3]float32
	RespawnDimension core.DimensionID
}
