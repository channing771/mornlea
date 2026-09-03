package player

import (
	"fmt"

	"github.com/channing771/mornlea/internal/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

const oldestPlayerSchema uint32 = 1

type playerDTO struct {
	PlayerID    core.PlayerID
	Revision    uint64
	DisplayName string
	Current     PlayerLocation
	Yaw, Pitch  float32
	Safe        *PlayerLocation
	Inventory   core.Inventory
	// Health 是权威生命值，0..core.MaxHealth。
	Health uint8
	// Hunger 是权威饥饿值，0..core.MaxHunger。
	Hunger uint8
	// SaturationMilli 是权威饱和度（千分位），上界是 Hunger×core.SaturationMilliPerPoint。
	SaturationMilli uint16
	// ExhaustionMilli 是权威疲劳值（千分位）。
	ExhaustionMilli uint16
	// RespawnPresent 为真时 RespawnPosition/RespawnDimension 携带个人重生点；
	// 只有 schema v8 起的负载才会解码出真值，更旧的 schema 由迁移链清零。
	RespawnPresent   bool
	RespawnPosition  [3]float32
	RespawnDimension core.DimensionID
}

type playerMigration func(playerDTO) (playerDTO, error)

var playerMigrations = map[uint32]playerMigration{
	// v1 没有物品负载，确定性迁移为空快捷栏且选中栏位 0。
	1: func(dto playerDTO) (playerDTO, error) {
		dto.Inventory.Hotbar = core.Hotbar{}
		return dto, nil
	},
	// v2 没有背包负载，确定性迁移为空背包并保留既有快捷栏。
	2: func(dto playerDTO) (playerDTO, error) {
		dto.Inventory.Backpack = [core.BackpackSlots]core.ItemStack{}
		return dto, nil
	},
	// v3 没有耐久字段，旧工具一律迁移为满耐久。
	3: func(dto playerDTO) (playerDTO, error) {
		for slot, stack := range dto.Inventory.Hotbar.Slots {
			dto.Inventory.Hotbar.Slots[slot] = fillFullDurability(stack)
		}
		for slot, stack := range dto.Inventory.Backpack {
			dto.Inventory.Backpack[slot] = fillFullDurability(stack)
		}
		return dto, nil
	},
	// v4 没有生命值字段，历史存档一律迁移为满血。
	4: func(dto playerDTO) (playerDTO, error) {
		dto.Health = core.MaxHealth
		return dto, nil
	},
	// v6 与 v5 的 payload 布局相同，只扩展合法物品注册表。
	5: func(dto playerDTO) (playerDTO, error) { return dto, nil },
	// v6 没有三层饥饿状态，历史存档一律迁移为新玩家初值。初值与 sim 侧的
	// resetHunger 同源（core.MaxHunger / core.InitialSaturationMilli / 0），
	// 因此"旧存档登录"与"新玩家登录"得到的饥饿状态逐字段相同。
	6: func(dto playerDTO) (playerDTO, error) {
		dto.Hunger = core.MaxHunger
		dto.SaturationMilli = core.InitialSaturationMilli
		dto.ExhaustionMilli = 0
		return dto, nil
	},
	// v7 没有个人重生点字段，历史存档一律迁移为「无重生点」：死亡重生沿用
	// 世界出生锚点，与升级前的行为完全一致。坐标/维度零值只是规范形态，
	// present 位为假时它们不参与任何行为。
	7: func(dto playerDTO) (playerDTO, error) {
		dto.RespawnPresent = false
		dto.RespawnPosition = [3]float32{}
		dto.RespawnDimension = 0
		return dto, nil
	},
}

func migratePlayer(from uint32, dto playerDTO) (playerDTO, bool, error) {
	if from > CurrentSchema {
		return playerDTO{}, false, fmt.Errorf("%w: player schema %d", storagedef.ErrFutureVersion, from)
	}
	migrated := false
	for version := from; version < CurrentSchema; version++ {
		migration, ok := playerMigrations[version]
		if !ok {
			return playerDTO{}, false, fmt.Errorf("storage: missing player migration %d", version)
		}
		var err error
		dto, err = migration(clonePlayerDTO(dto))
		if err != nil {
			return playerDTO{}, false, fmt.Errorf("migrate player %d: %w", version, err)
		}
		migrated = true
	}
	return dto, migrated, nil
}

func clonePlayerDTO(dto playerDTO) playerDTO {
	clone := dto
	if dto.Safe != nil {
		safe := *dto.Safe
		clone.Safe = &safe
	}
	return clone
}

// fillFullDurability 把没有耐久的旧工具补为满耐久，非工具保持零值。
// 与 chunk 包 migration.go 的同名迁移助手同源：按域拆分后各域持有自己的副本，
// 域内迁移表是这些副本的唯一消费方。
func fillFullDurability(stack core.ItemStack) core.ItemStack {
	full, ok := core.ItemMaxDurability(stack.Item)
	if !ok || stack.Durability != 0 {
		return stack
	}
	stack.Durability = full
	return stack
}
