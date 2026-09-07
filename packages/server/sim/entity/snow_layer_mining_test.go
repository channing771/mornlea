package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// snowLayerBlocks 是四档雪层的稳定编号样本，按档位 1..4 排列。
var snowLayerMiningBlocks = [...]core.BlockID{
	core.SnowLayer1BlockID,
	core.SnowLayer2BlockID,
	core.SnowLayer3BlockID,
	core.SnowLayer4BlockID,
}

// TestSnowLayerMiningRuleOneTickAnyHeldNoDrop 锁定雪层采掘规则：四档在任意手持
// 状态（空手、普通物品、完好或损坏工具）下都是最小权威量子 1 tick——徒手即可
// 采除；0 是 miningRule 的「不可采掘」哨兵，雪层用 0 会永远挖不动。
// harvestable=false：雪层没有掉落资格，任何手持完成采掘都不产生物品。
func TestSnowLayerMiningRuleOneTickAnyHeldNoDrop(t *testing.T) {
	helds := [...]core.ItemID{
		core.ItemNone,
		core.ItemDirt,
		core.ItemStonePickaxe,
		core.ItemIronPickaxe,
		core.ItemBrokenStonePickaxe,
		core.ItemStoneHoe,
		core.ItemIronSword,
	}
	for _, block := range snowLayerMiningBlocks {
		for _, held := range helds {
			ticks, harvestable := miningRule(block, held)
			if ticks != 1 || harvestable {
				t.Fatalf("miningRule(雪层 %d, %d) = (%d,%v)，想要 (1,false)",
					block, held, ticks, harvestable)
			}
		}
	}
}

// TestMiningSnowLayerClearsBlockWithoutDrop 覆盖雪层采掘 Scenario「徒手移除无
// 掉落」：空手 1 tick 完成采除，目标格变空气、不产生任何掉落物、区块 revision
// 恰好推进一次；四档同规则。
func TestMiningSnowLayerClearsBlockWithoutDrop(t *testing.T) {
	for _, block := range snowLayerMiningBlocks {
		engine, _, targets := readyMiningPlayers(t, 1)
		target := targets[0]
		engine.SetBlockForTest(target, block)
		beforeRevision := miningTargetRecord(t, engine, target).Revision

		result := advanceMiningOnce(engine)

		if len(result.Rejected) != 0 {
			t.Fatalf("徒手采除雪层 %d 被拒绝=%+v", block, result.Rejected)
		}
		record := miningTargetRecord(t, engine, target)
		x, _, z := target.Local()
		if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
			t.Fatalf("雪层 %d 采除后方块=%d，想要空气", block, got)
		}
		if drops := miningDropTotals(record.Chunk); len(drops) != 0 {
			t.Fatalf("雪层 %d 采除掉落=%+v，雪层不得产生任何掉落物", block, drops)
		}
		if record.Revision != beforeRevision+1 {
			t.Fatalf("雪层 %d 采除 revision=%d，想要按一次普通方块修改推进到 %d",
				block, record.Revision, beforeRevision+1)
		}
	}
}

// TestMiningSnowLayerSucceedsWithFullDropCapacity 覆盖「雪层采掘不预留 drop 槽，
// 掉落容量满也必须成功」（短草未命中路径的同形守护）：雪层无掉落语义，清块
// 不得因容量不足而拒绝或改动掉落槽——`DropsHash` 逐字节不变，revision 恰好
// 推进一次；四档同规则。
func TestMiningSnowLayerSucceedsWithFullDropCapacity(t *testing.T) {
	for _, block := range snowLayerMiningBlocks {
		engine, _, targets := readyMiningPlayers(t, 1)
		target := targets[0]
		engine.SetBlockForTest(target, block)
		fillMiningDrops(engine, target)
		record := miningTargetRecord(t, engine, target)
		beforeDrops := record.Chunk.DropsHash()
		beforeRevision := record.Revision

		result := advanceMiningOnce(engine)

		if len(result.Rejected) != 0 {
			t.Fatalf("容量满的雪层 %d 采除被拒绝=%+v", block, result.Rejected)
		}
		x, _, z := target.Local()
		if got := record.Chunk.BlockAt(x, target.Y, z); got != core.AirID {
			t.Fatalf("容量满的雪层 %d 采除后方块=%d，想要空气", block, got)
		}
		if got := record.Chunk.DropsHash(); got != beforeDrops {
			t.Fatalf("雪层 %d 采除修改了掉落槽: %x/%x", block, got, beforeDrops)
		}
		if after := miningTargetRecord(t, engine, target).Revision; after != beforeRevision+1 {
			t.Fatalf("雪层 %d 采除 revision=%d，想要推进一次到 %d",
				block, after, beforeRevision+1)
		}
	}
}

// TestCompanionMineableBlockRejectsSnowLayers 锁定伙伴防御清单对雪层的拒绝：
// 雪层只由积雪机制产生、没有 BlockDrop 登记，通用判据今天碰巧也会拒绝它，
// 但按短草先例契约要求显式拒绝——若未来有人给雪层补上 BlockDrop 登记，只有
// 显式谓词还站着。
func TestCompanionMineableBlockRejectsSnowLayers(t *testing.T) {
	for _, block := range snowLayerMiningBlocks {
		if companionMineableBlock(block) {
			t.Fatalf("companionMineableBlock(雪层 %d) = true，雪层必须是显式拒绝的伙伴采掘目标", block)
		}
	}
}
