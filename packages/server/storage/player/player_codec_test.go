package player

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

// updateStorageFixtures 与根包及 chunk 包测试共用同一命令行开关名：按域拆分后
// 各包测试持有同名 flag，重写各自域的 committed fixture。
var updateStorageFixtures = flag.Bool(
	"update-storage-fixtures", false, "rewrite committed storage fixtures",
)

func fixturePlayerID() core.PlayerID {
	return core.PlayerID{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
}

func fixturePlayerSave(id core.PlayerID, revision uint64) PlayerSave {
	safe := PlayerLocation{Dimension: core.Overworld, Position: [3]float32{1.5, 65, -2.5}}
	return PlayerSave{
		PlayerID: id, Revision: revision, DisplayName: "Chen",
		Current: PlayerLocation{Dimension: core.Overworld, Position: [3]float32{2.5, 70, -3.5}},
		Yaw:     1.25, Pitch: -0.5, Safe: &safe, Inventory: fixturePlayerInventory(),
		Health: 13,
		// 三层饥饿状态**全部取非初值**（初值是 20 / core.InitialSaturationMilli / 0）：
		// 任何一个字段在编码、迁移或接线里被漏写，读回来都会落在初值上，
		// 与这里的取值不同，往返与迁移用例因此才承重。饱和 2500 ≤ 12×1000，
		// 满足 validatePlayerDTO 的上界。
		Hunger: 12, SaturationMilli: 2500, ExhaustionMilli: 1750,
	}
}

func fixturePlayerInventory() core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Selected = 3
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}
	stoneFull, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	ironFull, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
	inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: stoneFull}
	inventory.Hotbar.Slots[6] = core.ItemStack{Item: core.ItemGrass, Count: 1}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemDirt, Count: 12}
	inventory.Backpack[7] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: ironFull}
	inventory.Backpack[core.BackpackSlots-1] = core.ItemStack{Item: core.ItemStone, Count: 5}
	return inventory
}

// respawnFixturePosition 是重生点往返用例的床尾格坐标：三个分量全部非零且
// Y 分量落在世界高度区间内。任何一个字段在编码或恢复路径上被漏写，读回来
// 都会落在零值/缺失上，与这里的取值不同，往返用例因此承重。
var respawnFixturePosition = [3]float32{7, 65, -9}

// armorFixtureStacks 是装备区往返用例的四槽取值，刻意覆盖编码边界：头盔取
// 满耐久 165（完好上界），胸甲取耐久 0（损坏形态），腿部放数量为 0 但携带
// 耐久的残值栈（sim 侧不持有护甲的形态），脚部留空。装备区是纯保真字段，
// 这些形态必须逐位往返，任何一层把它们规范化或抹平都会让断言落空。
var armorFixtureStacks = [core.ArmorSlotCount]core.ItemStack{
	{Item: core.ItemIronHelmet, Count: 1, Durability: 165},
	{Item: core.ItemIronChestplate, Count: 1},
	{Item: core.ItemIronLeggings, Count: 0, Durability: 165},
	{},
}

// storedPlayerToSave 把解码结果逐字段搬回保存类型。装备字段在两型之间必须
// 完整传递：漏掉任何一侧，「读回再保存」都会悄悄丢状态。根包持久化层有同形
// 转换，本副本只服务本包测试。
func storedPlayerToSave(stored StoredPlayer) PlayerSave {
	return PlayerSave{
		PlayerID: stored.PlayerID, Revision: stored.Revision, DisplayName: stored.DisplayName,
		Current: stored.Current, Yaw: stored.Yaw, Pitch: stored.Pitch, Safe: stored.Safe,
		Inventory: stored.Inventory, Health: stored.Health,
		Hunger:           stored.Hunger,
		SaturationMilli:  stored.SaturationMilli,
		ExhaustionMilli:  stored.ExhaustionMilli,
		RespawnPresent:   stored.RespawnPresent,
		RespawnPosition:  stored.RespawnPosition,
		RespawnDimension: stored.RespawnDimension,
		Armor:            stored.Armor,
	}
}

func TestPlayerCodecRoundTrip(t *testing.T) {
	if CurrentSchema != 9 {
		t.Fatalf("玩家 schema=%d，想要 9", CurrentSchema)
	}
	id := fixturePlayerID()
	want := fixturePlayerSave(id, 7)
	want.RespawnPresent = true
	want.RespawnPosition = respawnFixturePosition
	want.RespawnDimension = core.Overworld
	want.Armor = armorFixtureStacks
	encoded, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(id, encoded)
	if err != nil || got.PlayerID != want.PlayerID || got.Revision != want.Revision ||
		got.DisplayName != want.DisplayName || got.Current != want.Current ||
		got.Yaw != want.Yaw || got.Pitch != want.Pitch || got.Safe == nil || *got.Safe != *want.Safe ||
		got.Inventory != want.Inventory || got.Health != want.Health ||
		got.Hunger != want.Hunger || got.SaturationMilli != want.SaturationMilli ||
		got.ExhaustionMilli != want.ExhaustionMilli ||
		got.Armor != want.Armor {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if !got.RespawnPresent {
		t.Fatal("带重生点的存档往返后 RespawnPresent = false")
	}
	if got.RespawnPosition != want.RespawnPosition || got.RespawnDimension != want.RespawnDimension {
		t.Fatalf("重生点往返 = (%+v, %d)，想要 (%+v, %d)",
			got.RespawnPosition, got.RespawnDimension, want.RespawnPosition, want.RespawnDimension)
	}
	if got.NeedsRewrite {
		t.Fatal("当前 schema 玩家意外需要重写")
	}
	got.Safe.Position[0] = 99
	if want.Safe.Position[0] == 99 {
		t.Fatal("decoded safe location aliases save")
	}
}

// TestPlayerCodecRoundTripsDepthsLocations 覆盖多维存档的玩家位置值域：
// `Current`、`Safe` 与个人重生点落在 `Depths` 时编解码必须往返，维度 2
// 及以上仍被拒绝（见 `TestPlayerCodecRejectsInvalidSave`）。
func TestPlayerCodecRoundTripsDepthsLocations(t *testing.T) {
	id := fixturePlayerID()
	want := fixturePlayerSave(id, 7)
	want.Current.Dimension = core.Depths
	want.Safe.Dimension = core.Depths
	want.RespawnPresent = true
	want.RespawnPosition = respawnFixturePosition
	want.RespawnDimension = core.Depths
	encoded, err := Encode(want)
	if err != nil {
		t.Fatalf("depths 玩家存档编码: %v", err)
	}
	got, err := Decode(id, encoded)
	if err != nil {
		t.Fatalf("depths 玩家存档解码: %v", err)
	}
	if got.Current != want.Current || got.Safe == nil || *got.Safe != *want.Safe ||
		!got.RespawnPresent || got.RespawnPosition != want.RespawnPosition ||
		got.RespawnDimension != want.RespawnDimension {
		t.Fatalf("depths 位置往返 = (%+v, %+v, %+v/%d)，想要 (%+v, %+v, %+v/%d)",
			got.Current, got.Safe, got.RespawnPosition, got.RespawnDimension,
			want.Current, want.Safe, want.RespawnPosition, want.RespawnDimension)
	}
}

func TestPlayerCodecCurrentSchemaRoundTripsSwordItems(t *testing.T) {
	want := fixturePlayerSave(fixturePlayerID(), 27)
	stacks := [...]core.ItemStack{
		{Item: core.ItemWoodenSword, Count: 1, Durability: 58},
		{Item: core.ItemStoneSword, Count: 1, Durability: 130},
		{Item: core.ItemIronSword, Count: 1, Durability: 249},
		{Item: core.ItemBrokenWoodenSword, Count: 1},
		{Item: core.ItemBrokenStoneSword, Count: 1},
		{Item: core.ItemBrokenIronSword, Count: 1},
	}
	copy(want.Inventory.Hotbar.Slots[:], stacks[:])

	encoded, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(want.PlayerID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Inventory != want.Inventory || got.NeedsRewrite {
		t.Fatalf("剑物品往返 inventory=%+v needsRewrite=%v，想要 %+v / false",
			got.Inventory, got.NeedsRewrite, want.Inventory)
	}
}

// TestPlayerCodecRoundTripWithoutRespawn 覆盖重生点缺失的一半：present=0 是
// 「无重生点」的规范形态，往返后必须保持缺失，且编码对调用方留在
// RespawnPosition 里的残值不敏感——present=0 时位置字节不携带语义，同一份
// 逻辑状态无论残值是什么都必须得到逐字节相同的编码。
func TestPlayerCodecRoundTripWithoutRespawn(t *testing.T) {
	id := fixturePlayerID()
	want := fixturePlayerSave(id, 7)
	encoded, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(id, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.RespawnPresent || got.RespawnPosition != ([3]float32{}) || got.RespawnDimension != 0 {
		t.Fatalf("无重生点存档往返 = (present %v, %+v, %d)，想要全零",
			got.RespawnPresent, got.RespawnPosition, got.RespawnDimension)
	}

	residue := want
	residue.RespawnPosition = [3]float32{1, 2, 3}
	residue.RespawnDimension = core.Overworld
	residueEncoded, err := Encode(residue)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, residueEncoded) {
		t.Fatal("present=0 的编码依赖了 RespawnPosition 残值，不再是确定性的")
	}
}

// TestPlayerCodecRoundTripsEquippedArmor 覆盖装备区的逐位无损往返：四槽按
// core.ArmorSlot 顺序（头/胸/腿/脚）编码为 4×5 字节定长尾部，每槽沿用背包格
// 同一 5 字节栈编码（item u16 小端 + count 1 字节 + durability u16 小端）。
// codec 两端都不做语义校验（数量、耐久与物品注册均不查），损坏形态与残值
// 形态必须原样往返；当前 schema 记录读回不需要重写。
func TestPlayerCodecRoundTripsEquippedArmor(t *testing.T) {
	want := fixturePlayerSave(fixturePlayerID(), 9)
	want.RespawnPresent = true
	want.RespawnPosition = respawnFixturePosition
	want.RespawnDimension = core.Overworld
	want.Armor = armorFixtureStacks
	encoded, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	// 写侧 MUST 只出当前 schema：首次保存的 wire schema 字段就是 CurrentSchema。
	if schema := binary.LittleEndian.Uint32(encoded[8:12]); schema != CurrentSchema {
		t.Fatalf("首次保存 schema = %d，想要 %d", schema, CurrentSchema)
	}
	// 装备区是负载末尾的定长 20 字节：清空装备只改这 20 字节，负载其余部分与
	// 总长都不变——与重生点同理，空槽写零、恒占满，不携带「缺失」语义。
	unequipped, err := Encode(func() PlayerSave {
		withoutArmor := want
		withoutArmor.Armor = [core.ArmorSlotCount]core.ItemStack{}
		return withoutArmor
	}())
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != len(unequipped) {
		t.Fatalf("装备区取值改变了记录总长 %d != %d", len(encoded), len(unequipped))
	}
	// 只比负载段：信封 CRC 随负载内容合法地变化，不在本断言范围内。
	if !bytes.Equal(
		encoded[EnvelopeLength:len(encoded)-playerArmorBytes],
		unequipped[EnvelopeLength:len(unequipped)-playerArmorBytes],
	) {
		t.Fatal("装备区不位于负载末尾的定长装备段")
	}
	if bytes.Equal(encoded[len(encoded)-playerArmorBytes:], unequipped[len(unequipped)-playerArmorBytes:]) {
		t.Fatal("夹具失效：装备区取值全为零，末段定位断言没有承重")
	}
	got, err := Decode(want.PlayerID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Armor != want.Armor {
		t.Fatalf("装备区往返 = %+v，想要 %+v", got.Armor, want.Armor)
	}
	if got.NeedsRewrite {
		t.Fatal("当前 schema 玩家意外需要重写")
	}
	// 逐位无损：读回结果经两型传递再编码，必须与原字节完全一致——任何一端
	// 把损坏/残值形态规范化都会在这里现形。
	reencoded, err := Encode(storedPlayerToSave(got))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatal("装备区往返不是逐位无损")
	}
}

// TestPlayerCodecKeepsArmorRegionUnvalidated 钉死「装备区纯保真」的边界：
// 未知物品、数量越界、耐久超上限等语义非法形态在编解码两端都不被拒绝——
// 穿戴合法性由 sim 层 fail closed 判定（`core.ArmorPoints` 对非法形态记 0 点），
// codec 若顺手校验，反而会让携带这类字节的合法历史存档不可读。负载其余区域
// 的校验语义不受影响（快捷栏/背包的注册表校验在既有用例里）。
func TestPlayerCodecKeepsArmorRegionUnvalidated(t *testing.T) {
	id := fixturePlayerID()
	save := fixturePlayerSave(id, 3)
	save.Armor = [core.ArmorSlotCount]core.ItemStack{{Item: core.ItemIronBoots, Count: 1}}
	encoded, err := Encode(save)
	if err != nil {
		t.Fatal(err)
	}
	// 装备区在负载末尾：从头盔槽写入未知物品、越界数量与越界耐久，再修 CRC。
	offset := len(encoded) - playerArmorBytes
	binary.LittleEndian.PutUint16(encoded[offset:], uint16(core.ItemID(4242)))
	encoded[offset+2] = core.MaxStackCount + 1
	binary.LittleEndian.PutUint16(encoded[offset+3:], 999)
	repairPlayerCRC(encoded)
	got, err := Decode(id, encoded)
	if err != nil {
		t.Fatalf("装备区语义非法形态被拒绝: %v", err)
	}
	want := [core.ArmorSlotCount]core.ItemStack{
		{Item: core.ItemID(4242), Count: core.MaxStackCount + 1, Durability: 999},
	}
	if got.Armor != want {
		t.Fatalf("装备区保真 = %+v，想要 %+v", got.Armor, want)
	}
}

func TestPlayerSchemaV6RoundTripsNewBlockItems(t *testing.T) {
	id := fixturePlayerID()
	want := fixturePlayerSave(id, 8)
	want.Inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemLightBlock, Count: 17}
	want.Inventory.Backpack[3] = core.ItemStack{
		Item: core.ItemMossyCobblestone, Count: core.MaxStackCount,
	}
	encoded, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(id, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Inventory != want.Inventory || got.NeedsRewrite {
		t.Fatalf("新方块物品往返 inventory=%+v needsRewrite=%v", got.Inventory, got.NeedsRewrite)
	}
}

func TestPlayerSchemaV4DecodeKeepsWornDurability(t *testing.T) {
	save := fixturePlayerSave(fixturePlayerID(), 7)
	save.Inventory.Hotbar.Slots[4].Durability = 73
	save.Inventory.Backpack[7].Durability = 149

	encoded, err := Encode(save)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(save.PlayerID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Inventory != save.Inventory {
		t.Fatalf("v4 磨损工具往返后 inventory=%+v，想要 %+v", got.Inventory, save.Inventory)
	}
}

// TestPlayerV4FixtureMigratesToFullHealth 把冻结的 v4 存档（没有生命值字段）
// 当作迁移输入：物品状态必须无损，生命值必须迁移为满血，且必须标记为需要重写。
func TestPlayerV4FixtureMigratesToFullHealth(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("testdata", "player-v4.bin"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(fixturePlayerID(), encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Inventory != fixturePlayerInventory() {
		t.Fatalf("v4 迁移改变了物品状态: %+v", got.Inventory)
	}
	if got.Health != core.MaxHealth {
		t.Fatalf("v4 迁移生命值 = %d，想要满血 %d", got.Health, core.MaxHealth)
	}
	if !got.NeedsRewrite {
		t.Fatal("v4 玩家必须标记为需要重写")
	}
}

// TestPlayerV5FixtureMigratesLosslessly 冻结 v5 负载布局，并验证 v6 identity migration。
func TestPlayerV5FixtureMigratesLosslessly(t *testing.T) {
	path := filepath.Join("testdata", "player-v5.bin")
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := fixturePlayerSave(fixturePlayerID(), 19)
	got, err := Decode(want.PlayerID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlayerID != want.PlayerID || got.Revision != want.Revision ||
		got.DisplayName != want.DisplayName || got.Current != want.Current ||
		got.Yaw != want.Yaw || got.Pitch != want.Pitch || got.Safe == nil || *got.Safe != *want.Safe ||
		got.Inventory != want.Inventory || got.Health != want.Health || !got.NeedsRewrite {
		t.Fatalf("v5 identity migration = %+v", got)
	}
	// v5 同样没有饥饿字段：迁移链 v5→v6→v7 必须走到 v6 那一步的初值填充。
	assertMigratedToInitialHunger(t, "v5", got)
}

// assertMigratedToInitialHunger 断言一份迁移自 v7 之前 schema 的存档拿到了固定的
// 饥饿初值。它是"旧存档按初值迁移"这条 Scenario 的共用断言。
func assertMigratedToInitialHunger(t *testing.T, schema string, got StoredPlayer) {
	t.Helper()
	if got.Hunger != core.MaxHunger || got.SaturationMilli != core.InitialSaturationMilli ||
		got.ExhaustionMilli != 0 {
		t.Fatalf("%s 迁移后三层饥饿状态 = (%d, %d, %d)，想要初值 (%d, %d, 0)",
			schema, got.Hunger, got.SaturationMilli, got.ExhaustionMilli,
			core.MaxHunger, core.InitialSaturationMilli)
	}
}

// TestPlayerV6FixtureMigratesToInitialHunger 覆盖 Scenario「旧存档按初值迁移」。
//
// 输入是**冻结的 v6 字节**（testdata/player-v6.bin，本变更一字不改），不是当前
// 编码器现场生成的负载：当前编码器已经写 v7，用它"生成 v6"只会得到一份带饥饿
// 字段的 v7 记录，迁移分支根本不会被执行，用例会全绿而什么都没测。
//
// 断言分两半：三层饥饿必须是初值；**其余字段逐字段不变**（身份、修订号、昵称、
// 位置、朝向、安全点、背包与耐久、生命值），迁移不得顺手改动任何既有状态。
func TestPlayerV6FixtureMigratesToInitialHunger(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("testdata", "player-v6.bin"))
	if err != nil {
		t.Fatal(err)
	}
	want := fixturePlayerSave(fixturePlayerID(), 19)
	got, err := Decode(want.PlayerID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	assertMigratedToInitialHunger(t, "v6", got)
	if got.PlayerID != want.PlayerID {
		t.Fatalf("v6 迁移后身份 = %v，想要 %v", got.PlayerID, want.PlayerID)
	}
	if got.Revision != want.Revision {
		t.Fatalf("v6 迁移后修订号 = %d，想要 %d", got.Revision, want.Revision)
	}
	if got.DisplayName != want.DisplayName {
		t.Fatalf("v6 迁移后昵称 = %q，想要 %q", got.DisplayName, want.DisplayName)
	}
	if got.Current != want.Current {
		t.Fatalf("v6 迁移后位置 = %+v，想要 %+v", got.Current, want.Current)
	}
	if got.Yaw != want.Yaw || got.Pitch != want.Pitch {
		t.Fatalf("v6 迁移后朝向 = (%v, %v)，想要 (%v, %v)",
			got.Yaw, got.Pitch, want.Yaw, want.Pitch)
	}
	if got.Safe == nil || *got.Safe != *want.Safe {
		t.Fatalf("v6 迁移后安全点 = %+v，想要 %+v", got.Safe, want.Safe)
	}
	if got.Inventory != want.Inventory {
		t.Fatalf("v6 迁移后物品状态 = %+v，想要 %+v", got.Inventory, want.Inventory)
	}
	if got.Health != want.Health {
		t.Fatalf("v6 迁移后生命值 = %d，想要 %d", got.Health, want.Health)
	}
	if !got.NeedsRewrite {
		t.Fatal("v6 玩家必须标记为需要重写")
	}
}

// TestPlayerV9Fixture 冻结当前 schema 的编码结果，防止字节布局无声漂移。
// 装备区取非平凡取值：头盔完好（满耐久 165）、胸甲损坏（耐久 0）、腿脚为空，
// 让 12 字节装备区的前 6 字节承重、后 6 字节钉死「空槽写零」。
//
// 冻结的 v8 golden（testdata/player-v8.bin）刻意保留在原处不再生成：它是
// "旧存档仍然可读"的唯一真实证据，见 TestPlayerV8FixtureMigratesToEmptyArmor。
func TestPlayerV9Fixture(t *testing.T) {
	want1 := fixturePlayerSave(fixturePlayerID(), 19)
	want1.RespawnPresent = true
	want1.RespawnPosition = respawnFixturePosition
	want1.RespawnDimension = core.Overworld
	want1.Armor = [core.ArmorSlotCount]core.ItemStack{
		{Item: core.ItemIronHelmet, Count: 1, Durability: 165},
		{Item: core.ItemIronChestplate, Count: 1},
	}
	encoded, err := Encode(want1)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "player-v9.bin")
	if *updateStorageFixtures {
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, encoded) {
		t.Fatal("v9 fixture drift; change schema version")
	}
}

// TestPlayerV7FixtureMigratesToNoRespawn 覆盖 Scenario「v7 玩家档迁移后行为
// 不变」：输入是**冻结的 v7 字节**（testdata/player-v7.bin，本变更一字不改），
// 不是当前编码器现场生成的负载——当前编码器已经写 v8，用它"生成 v7"只会得到
// 一份带重生点字段的 v8 记录，迁移分支根本不会被执行，用例会全绿而什么都没测。
//
// 迁移语义是「无重生点」：present 必须为假（死亡回到世界锚点，与升级前的
// 行为一致），其余字段逐字段不变，且必须标记为需要重写（下次保存写为 v8）。
func TestPlayerV7FixtureMigratesToNoRespawn(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("testdata", "player-v7.bin"))
	if err != nil {
		t.Fatal(err)
	}
	want := fixturePlayerSave(fixturePlayerID(), 19)
	got, err := Decode(want.PlayerID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.RespawnPresent {
		t.Fatal("v7 迁移后不应携带重生点")
	}
	if got.PlayerID != want.PlayerID {
		t.Fatalf("v7 迁移后身份 = %v，想要 %v", got.PlayerID, want.PlayerID)
	}
	if got.Revision != want.Revision {
		t.Fatalf("v7 迁移后修订号 = %d，想要 %d", got.Revision, want.Revision)
	}
	if got.DisplayName != want.DisplayName {
		t.Fatalf("v7 迁移后昵称 = %q，想要 %q", got.DisplayName, want.DisplayName)
	}
	if got.Current != want.Current {
		t.Fatalf("v7 迁移后位置 = %+v，想要 %+v", got.Current, want.Current)
	}
	if got.Yaw != want.Yaw || got.Pitch != want.Pitch {
		t.Fatalf("v7 迁移后朝向 = (%v, %v)，想要 (%v, %v)",
			got.Yaw, got.Pitch, want.Yaw, want.Pitch)
	}
	if got.Safe == nil || *got.Safe != *want.Safe {
		t.Fatalf("v7 迁移后安全点 = %+v，想要 %+v", got.Safe, want.Safe)
	}
	if got.Inventory != want.Inventory {
		t.Fatalf("v7 迁移后物品状态 = %+v，想要 %+v", got.Inventory, want.Inventory)
	}
	if got.Health != want.Health {
		t.Fatalf("v7 迁移后生命值 = %d，想要 %d", got.Health, want.Health)
	}
	// v7 自带三层饥饿字段（夹具取的是非初值 12/2500/1750），迁移必须原样保留
	// 而不是重置：与 v6 那条「补初值」迁移不同，v7 缺的只有重生点。
	if got.Hunger != want.Hunger || got.SaturationMilli != want.SaturationMilli ||
		got.ExhaustionMilli != want.ExhaustionMilli {
		t.Fatalf("v7 迁移三层饥饿状态 = (%d, %d, %d)，想要原值 (%d, %d, %d)",
			got.Hunger, got.SaturationMilli, got.ExhaustionMilli,
			want.Hunger, want.SaturationMilli, want.ExhaustionMilli)
	}
	if !got.NeedsRewrite {
		t.Fatal("v7 玩家必须标记为需要重写")
	}
}

// TestPlayerV8FixtureMigratesToEmptyArmor 覆盖 Scenario「升级前的 v8 存档加载
// 成功且四槽为空，保存写出 schema v9 且原 v8 字节段逐位保留」。
//
// 输入是**冻结的 v8 字节**（testdata/player-v8.bin，本变更一字不改），不是当前
// 编码器现场生成的负载：当前编码器已经写 v9，用它"生成 v8"只会得到一份带装备
// 区的 v9 记录，迁移分支根本不会被执行，用例会全绿而什么都没测。
//
// 「原字节段保留」按段断言：装备区是尾部追加，v8 负载必须逐位成为 v9 负载的
// 前缀、随后跟 12 字节空装备区；信封里只有 schema 号（恰推进一格）与随负载
// 变化的 payload 长度/CRC 两处不同，身份段（magic、信封版本、PlayerID、修订号）
// 逐位不变。
func TestPlayerV8FixtureMigratesToEmptyArmor(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("testdata", "player-v8.bin"))
	if err != nil {
		t.Fatal(err)
	}
	want := fixturePlayerSave(fixturePlayerID(), 19)
	want.RespawnPresent = true
	want.RespawnPosition = respawnFixturePosition
	want.RespawnDimension = core.Overworld
	got, err := Decode(want.PlayerID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Armor != ([core.ArmorSlotCount]core.ItemStack{}) {
		t.Fatalf("v8 迁移装备区 = %+v，想要四空槽", got.Armor)
	}
	if got.PlayerID != want.PlayerID || got.Revision != want.Revision ||
		got.DisplayName != want.DisplayName || got.Current != want.Current ||
		got.Yaw != want.Yaw || got.Pitch != want.Pitch || got.Safe == nil || *got.Safe != *want.Safe ||
		got.Inventory != want.Inventory || got.Health != want.Health ||
		got.Hunger != want.Hunger || got.SaturationMilli != want.SaturationMilli ||
		got.ExhaustionMilli != want.ExhaustionMilli ||
		!got.RespawnPresent || got.RespawnPosition != want.RespawnPosition ||
		got.RespawnDimension != want.RespawnDimension {
		t.Fatalf("v8 迁移改动了既有字段: %+v", got)
	}
	if !got.NeedsRewrite {
		t.Fatal("v8 玩家必须标记为需要重写")
	}
	rewritten, err := Encode(storedPlayerToSave(got))
	if err != nil {
		t.Fatal(err)
	}
	if schema := binary.LittleEndian.Uint32(rewritten[8:12]); schema != CurrentSchema {
		t.Fatalf("重写 schema = %d，想要 %d", schema, CurrentSchema)
	}
	v8Payload := encoded[EnvelopeLength:]
	if !bytes.Equal(rewritten[:8], encoded[:8]) || !bytes.Equal(rewritten[12:36], encoded[12:36]) {
		t.Fatal("v8 存档重写改动了信封身份段")
	}
	if !bytes.Equal(rewritten[EnvelopeLength:EnvelopeLength+len(v8Payload)], v8Payload) {
		t.Fatal("v8 负载字节未逐位保留为 v9 负载前缀")
	}
	if tail := rewritten[EnvelopeLength+len(v8Payload):]; !bytes.Equal(tail, make([]byte, playerArmorBytes)) {
		t.Fatalf("v9 负载追加的装备区 = %x，想要 %d 字节全零空槽", tail, playerArmorBytes)
	}
}

func TestPlayerV3FixtureMigratesLosslessly(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("testdata", "player-v3.bin"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(fixturePlayerID(), encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Inventory != fixturePlayerInventory() {
		t.Fatalf("v3 迁移改变了物品状态: %+v", got.Inventory)
	}
	if got.Health != core.MaxHealth {
		t.Fatalf("v3 迁移生命值 = %d，想要满血 %d", got.Health, core.MaxHealth)
	}
	if !got.NeedsRewrite {
		t.Fatal("v3 玩家必须标记为需要重写")
	}
}

func TestPlayerV1FixtureMigratesToEmptyHotbar(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("testdata", "player-v1.bin"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(fixturePlayerID(), encoded)
	if err != nil {
		t.Fatal(err)
	}
	want := fixturePlayerSave(fixturePlayerID(), 19)
	if got.DisplayName != want.DisplayName || got.Current != want.Current ||
		got.Yaw != want.Yaw || got.Pitch != want.Pitch ||
		got.Safe == nil || *got.Safe != *want.Safe {
		t.Fatalf("v1 迁移改变了既有字段: %+v", got)
	}
	if got.Inventory != (core.Inventory{}) {
		t.Fatalf("v1 迁移物品状态 = %+v，想要空快捷栏与空背包", got.Inventory)
	}
	if got.Health != core.MaxHealth {
		t.Fatalf("v1 迁移生命值 = %d，想要满血 %d", got.Health, core.MaxHealth)
	}
	if !got.NeedsRewrite {
		t.Fatal("v1 存档必须标记为需要重写")
	}
}

func TestPlayerCodecRejectsInvalidHotbarPayload(t *testing.T) {
	id := fixturePlayerID()
	invalid := []struct {
		name   string
		mutate func(*core.Hotbar)
	}{
		{"选中栏位越界", func(h *core.Hotbar) { h.Selected = core.HotbarSlots }},
		{"数量超过上限", func(h *core.Hotbar) {
			h.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount + 1}
		}},
		// M4E 二进制的注册范围止于 9，因此同样会拒绝 ID 10/11；
		// 这里保留真正未知的 4242，不能复制一份冻结的旧 decoder。
		{"未知物品", func(h *core.Hotbar) {
			h.Slots[2] = core.ItemStack{Item: core.ItemID(4242), Count: 1}
		}},
		{"物品哨兵", func(h *core.Hotbar) {
			h.Slots[2] = core.ItemStack{Item: core.ItemIDMax, Count: 1}
		}},
		{"空物品非零数量", func(h *core.Hotbar) {
			h.Slots[3] = core.ItemStack{Item: core.ItemNone, Count: 5}
		}},
		{"非工具携带耐久", func(h *core.Hotbar) {
			h.Slots[5] = core.ItemStack{Item: core.ItemStone, Count: 1, Durability: 1}
		}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			save := fixturePlayerSave(id, 3)
			tc.mutate(&save.Inventory.Hotbar)
			if _, err := Encode(save); !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("encode error = %v，想要 storagedef.ErrCorrupt", err)
			}
			encoded := playerWireWithHotbar(t, id, save.Inventory.Hotbar)
			if _, err := Decode(id, encoded); !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("decode error = %v，想要 storagedef.ErrCorrupt", err)
			}
		})
	}
}

// playerWireWithHotbar 用合法存档换掉快捷栏负载并修正 CRC，绕过编码器校验。
func playerWireWithHotbar(t *testing.T, id core.PlayerID, hotbar core.Hotbar) []byte {
	t.Helper()
	encoded, err := Encode(fixturePlayerSave(id, 3))
	if err != nil {
		t.Fatal(err)
	}
	wire := bytes.Clone(encoded)
	// v5 起负载在快捷栏/背包之后追加了 1 字节生命值，v7 起再追加三层饥饿状态，
	// v8 起再追加重生点三字段，v9 起再追加四槽装备区，从末尾定位快捷栏的偏移量
	// 必须按倒序跳过这四段尾巴。这里写成具名常量而不是字面数字：往尾部追加字段
	// 时就有一条断言静默改指了新字段，而不是悄悄破坏快捷栏。
	offset := len(wire) - playerArmorBytes - playerRespawnBytes - playerHungerBytes -
		playerHealthBytes - playerBackpackBytes - playerHotbarBytes
	wire[offset] = hotbar.Selected
	offset++
	for _, stack := range hotbar.Slots {
		binary.LittleEndian.PutUint16(wire[offset:], uint16(stack.Item))
		wire[offset+2] = stack.Count
		binary.LittleEndian.PutUint16(wire[offset+3:], stack.Durability)
		offset += 5
	}
	hasher := crc32.New(playerCRCTable)
	_, _ = hasher.Write(wire[8:40])
	_, _ = hasher.Write(wire[EnvelopeLength:])
	binary.LittleEndian.PutUint32(wire[40:], hasher.Sum32())
	return wire
}

func TestPlayerCodecRejectsInvalidSave(t *testing.T) {
	valid := fixturePlayerSave(fixturePlayerID(), 1)
	tests := []struct {
		name   string
		mutate func(*PlayerSave)
	}{
		{"invalid player ID", func(save *PlayerSave) { save.PlayerID = core.PlayerID{} }},
		{"zero revision", func(save *PlayerSave) { save.Revision = 0 }},
		{"unnormalized name", func(save *PlayerSave) { save.DisplayName = " Chen " }},
		{"invalid dimension", func(save *PlayerSave) { save.Current.Dimension = 2 }},
		{"nonfinite current position", func(save *PlayerSave) { save.Current.Position[0] = float32(math.Inf(1)) }},
		{"nonfinite yaw", func(save *PlayerSave) { save.Yaw = float32(math.NaN()) }},
		{"nonfinite pitch", func(save *PlayerSave) { save.Pitch = float32(math.Inf(-1)) }},
		{"pitch too high", func(save *PlayerSave) { save.Pitch = float32(math.Pi/2) + 0.01 }},
		{"invalid safe dimension", func(save *PlayerSave) { save.Safe.Dimension = 2 }},
		{"health above max", func(save *PlayerSave) { save.Health = core.MaxHealth + 1 }},
		{"hunger above max", func(save *PlayerSave) { save.Hunger = core.MaxHunger + 1 }},
		{"saturation above hunger", func(save *PlayerSave) {
			save.SaturationMilli = uint16(save.Hunger)*core.SaturationMilliPerPoint + 1
		}},
		{"invalid respawn dimension", func(save *PlayerSave) {
			save.RespawnPresent = true
			save.RespawnPosition = respawnFixturePosition
			save.RespawnDimension = 2
		}},
		{"nonfinite respawn position", func(save *PlayerSave) {
			save.RespawnPresent = true
			save.RespawnDimension = core.Overworld
			save.RespawnPosition = [3]float32{1, float32(math.NaN()), 3}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			save := valid
			if valid.Safe != nil {
				safe := *valid.Safe
				save.Safe = &safe
			}
			tc.mutate(&save)
			if _, err := Encode(save); !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("encode error = %v, want storagedef.ErrCorrupt", err)
			}
		})
	}
}

func TestPlayerCodecRejectsCorruptEnvelope(t *testing.T) {
	id := fixturePlayerID()
	encoded, err := Encode(fixturePlayerSave(id, 19))
	if err != nil {
		t.Fatal(err)
	}
	badFloat := func(offset int) []byte {
		return badFloatAt(bytes.Clone(encoded), offset)
	}
	tests := []struct {
		name    string
		payload func() []byte
		want    error
	}{
		{"magic", func() []byte { p := bytes.Clone(encoded); p[0] ^= 1; return p }, storagedef.ErrCorrupt},
		{"old envelope", func() []byte { p := bytes.Clone(encoded); binary.LittleEndian.PutUint32(p[4:], 0); return p }, storagedef.ErrCorrupt},
		{"future envelope", func() []byte { p := bytes.Clone(encoded); binary.LittleEndian.PutUint32(p[4:], 2); return p }, storagedef.ErrFutureVersion},
		{"invalid schema", func() []byte { p := bytes.Clone(encoded); binary.LittleEndian.PutUint32(p[8:], 0); return p }, storagedef.ErrCorrupt},
		{"future schema", func() []byte {
			p := bytes.Clone(encoded)
			binary.LittleEndian.PutUint32(p[8:], CurrentSchema+1)
			return p
		}, storagedef.ErrFutureVersion},
		{"invalid player ID", func() []byte { p := bytes.Clone(encoded); clear(p[12:28]); repairPlayerCRC(p); return p }, storagedef.ErrCorrupt},
		{"mismatched player ID", func() []byte { p := bytes.Clone(encoded); p[27] ^= 1; repairPlayerCRC(p); return p }, storagedef.ErrCorrupt},
		{"zero revision", func() []byte { p := bytes.Clone(encoded); clear(p[28:36]); repairPlayerCRC(p); return p }, storagedef.ErrCorrupt},
		{"payload length mismatch", func() []byte {
			p := bytes.Clone(encoded)
			binary.LittleEndian.PutUint32(p[36:], uint32(len(p)))
			return p
		}, storagedef.ErrCorrupt},
		{"CRC", func() []byte { p := bytes.Clone(encoded); p[40] ^= 1; return p }, storagedef.ErrCorrupt},
		{"invalid nickname", func() []byte { p := bytes.Clone(encoded); p[48] = '\n'; repairPlayerCRC(p); return p }, storagedef.ErrCorrupt},
		{"current dimension", func() []byte {
			p := bytes.Clone(encoded)
			binary.LittleEndian.PutUint32(p[52:], 2)
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"current x", func() []byte { return badFloat(56) }, storagedef.ErrCorrupt},
		{"current y", func() []byte { return badFloat(60) }, storagedef.ErrCorrupt},
		{"current z", func() []byte { return badFloat(64) }, storagedef.ErrCorrupt},
		{"yaw", func() []byte { return badFloat(68) }, storagedef.ErrCorrupt},
		{"pitch", func() []byte { return badFloat(72) }, storagedef.ErrCorrupt},
		{"pitch outside range", func() []byte {
			p := bytes.Clone(encoded)
			binary.LittleEndian.PutUint32(p[72:], math.Float32bits(2))
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"safe flag", func() []byte { p := bytes.Clone(encoded); p[76] = 2; repairPlayerCRC(p); return p }, storagedef.ErrCorrupt},
		{"safe dimension", func() []byte {
			p := bytes.Clone(encoded)
			binary.LittleEndian.PutUint32(p[77:], 2)
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"safe x", func() []byte { return badFloat(81) }, storagedef.ErrCorrupt},
		{"safe y", func() []byte { return badFloat(85) }, storagedef.ErrCorrupt},
		{"safe z", func() []byte { return badFloat(89) }, storagedef.ErrCorrupt},
		{"invalid health", func() []byte {
			p := bytes.Clone(encoded)
			// 生命值不再是末字节：v7 在它之后追加了三层饥饿状态，v8 再追加重生点，
			// v9 再追加装备区。写成 len(p)-playerArmorBytes-playerRespawnBytes-
			// playerHungerBytes-playerHealthBytes 而不是 len(p)-1，否则这条断言会
			// 静默改指装备区字段，"生命值越界被拒"就不再被任何用例覆盖。
			p[len(p)-playerArmorBytes-playerRespawnBytes-playerHungerBytes-playerHealthBytes] = core.MaxHealth + 1
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"invalid hunger", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerArmorBytes-playerRespawnBytes-playerHungerBytes] = core.MaxHunger + 1
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"saturation above hunger", func() []byte {
			p := bytes.Clone(encoded)
			// 饱和度紧随饥饿值，取 hunger×1000 + 1 恰好越过上界一个千分位。
			hungerOffset := len(p) - playerArmorBytes - playerRespawnBytes - playerHungerBytes
			binary.LittleEndian.PutUint16(
				p[hungerOffset+1:],
				uint16(p[hungerOffset])*core.SaturationMilliPerPoint+1,
			)
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"respawn flag", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerArmorBytes-playerRespawnBytes] = 2
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		// 位置与维度字节只在 present=1 时携带语义（present=0 时规范为零），
		// 因此这几条先置位 flag 再投毒，保证变异真正抵达校验层。
		{"respawn x", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerArmorBytes-playerRespawnBytes] = 1
			return badFloatAt(p, len(p)-playerArmorBytes-playerRespawnBytes+1)
		}, storagedef.ErrCorrupt},
		{"respawn y", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerArmorBytes-playerRespawnBytes] = 1
			return badFloatAt(p, len(p)-playerArmorBytes-playerRespawnBytes+5)
		}, storagedef.ErrCorrupt},
		{"respawn z", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerArmorBytes-playerRespawnBytes] = 1
			return badFloatAt(p, len(p)-playerArmorBytes-playerRespawnBytes+9)
		}, storagedef.ErrCorrupt},
		{"respawn dimension", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerArmorBytes-playerRespawnBytes] = 1
			binary.LittleEndian.PutUint32(p[len(p)-playerArmorBytes-playerRespawnBytes+13:], 2)
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"trailing byte", func() []byte { return append(bytes.Clone(encoded), 0) }, storagedef.ErrCorrupt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(id, tc.payload())
			if !errors.Is(err, tc.want) {
				t.Fatalf("decode error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPlayerCodecRejectsPayloadOverLimitBeforeAllocation(t *testing.T) {
	id := fixturePlayerID()
	payload := make([]byte, EnvelopeLength)
	copy(payload, "MCPL")
	binary.LittleEndian.PutUint32(payload[4:], playerEnvelopeVersion)
	binary.LittleEndian.PutUint32(payload[8:], CurrentSchema)
	copy(payload[12:28], id[:])
	binary.LittleEndian.PutUint64(payload[28:], 1)
	binary.LittleEndian.PutUint32(payload[36:], MaxPayload+1)
	if _, err := Decode(id, payload); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("decode error = %v, want storagedef.ErrCorrupt", err)
	}
}

func repairPlayerCRC(payload []byte) {
	hasher := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = hasher.Write(payload[8:40])
	_, _ = hasher.Write(payload[EnvelopeLength:])
	binary.LittleEndian.PutUint32(payload[40:], hasher.Sum32())
}

// badFloatAt 把 payload 指定偏移处的 float32 改成 NaN 并修正 CRC，返回同一缓冲。
// 调用方传入的 payload 必须已经携带其他想叠加的变异。
func badFloatAt(payload []byte, offset int) []byte {
	binary.LittleEndian.PutUint32(payload[offset:], math.Float32bits(float32(math.NaN())))
	repairPlayerCRC(payload)
	return payload
}

// TestPlayerSchemaV9KeepsM4EItems 原先位于 chunk 域的 chunk_furnace_test.go：
// 拆分按「跟随被测主体」落位，其被测主体是 player codec，随 player 域入包。
func TestPlayerSchemaV9KeepsM4EItems(t *testing.T) {
	if CurrentSchema != 9 {
		t.Fatalf("玩家 schema = %d，想要 9", CurrentSchema)
	}
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemCoal, Count: 12}
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemRawIron, Count: 5}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemIronIngot, Count: 64}
	inventory.Backpack[1] = core.ItemStack{Item: core.ItemFurnace, Count: 1}
	inventory.Backpack[2] = core.ItemStack{Item: core.ItemIronBlock, Count: 3}

	save := fixturePlayerSave(fixturePlayerID(), 7)
	save.Inventory = inventory
	encoded, err := Encode(save)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(save.PlayerID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Inventory != inventory {
		t.Fatalf("M4E 物品未往返: %+v", got.Inventory)
	}
}
