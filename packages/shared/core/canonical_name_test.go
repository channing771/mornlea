package core

import (
	"math"
	"regexp"
	"testing"
)

var canonicalMachineNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

func TestCanonicalNameRegistriesAreExhaustiveUniqueAndFailClosed(t *testing.T) {
	blockNames := make(map[string]BlockID, BlockIDMax)
	for id := BlockID(0); id < BlockIDMax; id++ {
		name, ok := CanonicalBlockName(id)
		if !ok {
			t.Fatalf("CanonicalBlockName(%d) 未注册", id)
		}
		assertCanonicalMachineName(t, "方块", uint16(id), name)
		if previous, exists := blockNames[name]; exists {
			t.Fatalf("方块名 %q 同时属于 %d 与 %d", name, previous, id)
		}
		blockNames[name] = id
		parsed, ok := BlockIDByCanonicalName(name)
		if !ok || parsed != id {
			t.Fatalf("BlockIDByCanonicalName(%q)=(%d,%v)，want (%d,true)", name, parsed, ok, id)
		}
	}
	if name, ok := CanonicalBlockName(AirID); !ok || name != "air" {
		t.Fatalf("空气 machine name=(%q,%v)，want (air,true)", name, ok)
	}
	for _, id := range []BlockID{BlockIDMax, BlockID(math.MaxUint16)} {
		if name, ok := CanonicalBlockName(id); ok || name != "" {
			t.Fatalf("非法方块 %d 得到回退名 (%q,%v)", id, name, ok)
		}
	}

	itemNames := make(map[string]ItemID, ItemIDMax-1)
	for id := ItemID(1); id < ItemIDMax; id++ {
		name, ok := CanonicalItemName(id)
		if !ok {
			t.Fatalf("CanonicalItemName(%d) 未注册", id)
		}
		assertCanonicalMachineName(t, "物品", uint16(id), name)
		if previous, exists := itemNames[name]; exists {
			t.Fatalf("物品名 %q 同时属于 %d 与 %d", name, previous, id)
		}
		itemNames[name] = id
		parsed, ok := ItemIDByCanonicalName(name)
		if !ok || parsed != id {
			t.Fatalf("ItemIDByCanonicalName(%q)=(%d,%v)，want (%d,true)", name, parsed, ok, id)
		}
	}
	for _, id := range []ItemID{ItemNone, ItemIDMax, ItemID(math.MaxUint16)} {
		if name, ok := CanonicalItemName(id); ok || name != "" {
			t.Fatalf("非法物品 %d 得到回退名 (%q,%v)", id, name, ok)
		}
	}
	for _, name := range []string{"", "Stone", "石头", "unknown_65535", "stone "} {
		if id, ok := BlockIDByCanonicalName(name); ok || id != AirID {
			t.Fatalf("未知方块名 %q 得到 (%d,%v)", name, id, ok)
		}
		if id, ok := ItemIDByCanonicalName(name); ok || id != ItemNone {
			t.Fatalf("未知物品名 %q 得到 (%d,%v)", name, id, ok)
		}
	}
}

func TestCanonicalFullBlockItemsReuseBlockNames(t *testing.T) {
	// 这些物品的放置目标是状态/多格形态，不是与物品一一对应的完整方块；它们
	// 必须使用自己的物品名。其余 `ItemPlacement` 成员都是完整方块物品，应直接
	// 复用目标方块的 machine name，避免维护平行字符串白名单。
	stateful := map[ItemID]struct{}{
		ItemWheatSeeds: {},
		ItemPotato:     {},
		ItemCarrot:     {},
		ItemDoor:       {},
		ItemBed:        {},
	}
	for item := ItemID(1); item < ItemIDMax; item++ {
		block, placeable := ItemPlacement(item)
		if !placeable {
			continue
		}
		itemName, itemOK := CanonicalItemName(item)
		blockName, blockOK := CanonicalBlockName(block)
		if !itemOK || !blockOK {
			t.Fatalf("placement (%d -> %d) 缺少 canonical name", item, block)
		}
		_, isStateful := stateful[item]
		if isStateful {
			if itemName == blockName {
				t.Fatalf("状态/多格物品 %d 错误复用形态方块名 %q", item, itemName)
			}
			continue
		}
		if itemName != blockName {
			t.Fatalf("完整方块物品 %d 名 %q 未复用方块 %d 名 %q", item, itemName, block, blockName)
		}
	}
}

func assertCanonicalMachineName(t *testing.T, domain string, id uint16, name string) {
	t.Helper()
	if len(name) == 0 || len(name) > 64 {
		t.Fatalf("%s %d machine name 长度=%d，want 1..64", domain, id, len(name))
	}
	if !canonicalMachineNamePattern.MatchString(name) {
		t.Fatalf("%s %d machine name %q 不是 lower ASCII snake_case", domain, id, name)
	}
}
