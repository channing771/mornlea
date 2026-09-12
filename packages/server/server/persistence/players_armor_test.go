package persistence

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

// armorTestWorn 构造一份「部分穿戴、含非满耐久」的装备区夹具：头部一件磨损
// 完好铁头盔（160/165）、胸部一件完好铁胸甲，腿脚两槽为空。装备区快照的五处
// 持久化接线（读取缓存、恢复、保存与两处脏检测）共用同一夹具，任何一处漏带
// 装备都会让对应断言变红。
func armorTestWorn() [core.ArmorSlotCount]core.ItemStack {
	var worn [core.ArmorSlotCount]core.ItemStack
	worn[core.ArmorSlotHead] = core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 160}
	worn[core.ArmorSlotChest] = core.ItemStack{Item: core.ItemIronChestplate, Count: 1, Durability: 240}
	return worn
}

// armorTestStoredPlayer 构造一份装备区非空、其余字段沿
// `storedPlayerForTest` 默认值的存档记录。
func armorTestStoredPlayer(id core.PlayerID, name string) storage.StoredPlayer {
	stored := storedPlayerForTest(id, 7, name, testPlayerSnapshot(3))
	stored.Armor = armorTestWorn()
	return stored
}

// 捕获：磁盘加载构造的缓存快照与恢复视图都必须携带装备区。任何一处漏带，
// 重启（或缓存逐出后重连）都会让玩家静默落回空装备。
func TestPlayerPersistenceRestoresArmorFromStored(t *testing.T) {
	store := newCachePlayerStore()
	id := playerID(131)
	store.put(armorTestStoredPlayer(id, "Armored"))

	cached := cachedPlayerFromStored(armorTestStoredPlayer(id, "Armored"), "Armored")
	if cached.snapshot.Armor != armorTestWorn() {
		t.Fatalf("缓存快照装备区 = %+v，想要 %v", cached.snapshot.Armor, armorTestWorn())
	}
	restore := cached.restore(testMetadata())
	if restore.Armor != armorTestWorn() {
		t.Fatalf("恢复视图装备区 = %+v，想要 %v", restore.Armor, armorTestWorn())
	}

	// 走一遍完整 Prepare 入口，钉住在线恢复路径与直接构造同形。
	players := NewPlayers(store, playerPersistenceTestConfig())
	t.Cleanup(players.CloseWorker)
	prepared, err := players.Prepare(context.Background(), id, "Armored", testMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Armor != armorTestWorn() {
		t.Fatalf("Prepare 恢复装备区 = %+v，想要 %v", prepared.Armor, armorTestWorn())
	}
	players.Abort(id)
}

// 捕获：`save` 必须把缓存快照的装备区写进 `PlayerSave`——漏写会让每次落盘
// 都把玩家装备覆写成空。
func TestPlayerPersistenceSaveCarriesArmor(t *testing.T) {
	cached := cachedPlayerFromStored(armorTestStoredPlayer(playerID(132), "Armored"), "Armored")
	save := cached.save(8)
	if save.Armor != armorTestWorn() {
		t.Fatalf("保存载荷装备区 = %+v，想要 %v", save.Armor, armorTestWorn())
	}
	if save.Inventory != cached.snapshot.Inventory {
		t.Fatalf("保存载荷背包 = %+v，想要 %v", save.Inventory, cached.snapshot.Inventory)
	}
}

// 捕获：`matchesSave` 必须比较装备区。它在保存完成回调与重试分发处判定
// 「在途保存是否仍等于当前状态」——漏比较会让仅耐久变化的玩家被判为已落盘，
// 耐久损耗从此永不进存档。
func TestPlayerPersistenceMatchesSaveDetectsArmorDelta(t *testing.T) {
	cached := cachedPlayerFromStored(armorTestStoredPlayer(playerID(133), "Armored"), "Armored")
	save := cached.save(8)
	if !cached.matchesSave(save) {
		t.Fatalf("与自身快照一致的保存被判不匹配")
	}

	save.Armor[core.ArmorSlotHead].Durability--
	if cached.matchesSave(save) {
		t.Fatal("耐久变化后的保存被判匹配，脏检测会漏掉仅耐久变化的 tick")
	}
	save.Armor = armorTestWorn()
	save.Armor[core.ArmorSlotChest] = core.ItemStack{}
	if cached.matchesSave(save) {
		t.Fatal("卸下胸甲后的保存被判匹配，脏检测会漏掉装备卸下")
	}
	save.Armor = armorTestWorn()
	save.Armor[core.ArmorSlotLegs] = core.ItemStack{Item: core.ItemIronLeggings, Count: 1, Durability: 225}
	if cached.matchesSave(save) {
		t.Fatal("新穿护腿后的保存被判匹配，脏检测会漏掉装备穿上")
	}
}

// 捕获：快照相等性必须比较装备区——这是 `Observe` 判脏的依据，漏比较会让
// 「只有装备/耐久变了」的 tick 被判无变化而永不落盘。
func TestPlayerSnapshotsEqualDetectsArmorDelta(t *testing.T) {
	base := contract.PlayerSnapshot{
		Current: contract.PlayerLocation{Dimension: core.Overworld},
		Armor:   armorTestWorn(),
	}
	if !playerSnapshotsEqual(base, base) {
		t.Fatal("完全一致的快照被判不等")
	}

	onlyDurability := base
	onlyDurability.Armor[core.ArmorSlotHead].Durability--
	if playerSnapshotsEqual(base, onlyDurability) {
		t.Fatal("仅耐久不同的快照被判相等，仅耐久变化的 tick 会永不落盘")
	}

	onlyWorn := base
	onlyWorn.Armor[core.ArmorSlotChest] = core.ItemStack{}
	if playerSnapshotsEqual(base, onlyWorn) {
		t.Fatal("仅穿戴件不同的快照被判相等，装备互换会永不落盘")
	}
}

// 捕获：端到端判脏语义——与缓存一致的观察不得误报脏，仅装备区变化的观察
// 必须置脏。这是「打一场减免仗只掉耐久也要落盘」的持久化层前提。
func TestPlayerPersistenceObserveDirtiesOnArmorOnlyDelta(t *testing.T) {
	store := newCachePlayerStore()
	id := playerID(134)
	store.put(armorTestStoredPlayer(id, "Armored"))
	players := NewPlayers(store, playerPersistenceTestConfig())
	t.Cleanup(players.CloseWorker)

	if _, err := players.Prepare(context.Background(), id, "Armored", testMetadata()); err != nil {
		t.Fatal(err)
	}
	if err := players.Activate(id, "Armored"); err != nil {
		t.Fatal(err)
	}
	players.Confirm(id)
	if players.PlayerIsDirty(id) {
		t.Fatal("同名 Confirm 不应置脏")
	}

	players.mu.Lock()
	base := clonePlayerSnapshot(players.cache[id].snapshot)
	players.mu.Unlock()
	if err := players.Observe(id, "Armored", base, 20, false); err != nil {
		t.Fatal(err)
	}
	if players.PlayerIsDirty(id) {
		t.Fatal("与缓存一致的观察不应置脏")
	}

	damaged := clonePlayerSnapshot(base)
	damaged.Armor[core.ArmorSlotHead].Durability--
	if err := players.Observe(id, "Armored", damaged, 21, false); err != nil {
		t.Fatal(err)
	}
	// 脏标志是「自上次落盘以来变过」：只置不清，随后的自动保存会带上
	// 最新装备区。这里不触发保存，避免与阻塞式测试存档握手。
	if !players.PlayerIsDirty(id) {
		t.Fatal("仅耐久变化的观察未置脏，耐久损耗将永不落盘")
	}
}
