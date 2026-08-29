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

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/storage/storagedef"
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

func TestPlayerCodecRoundTrip(t *testing.T) {
	if CurrentSchema != 8 {
		t.Fatalf("玩家 schema=%d，想要 8", CurrentSchema)
	}
	id := fixturePlayerID()
	want := fixturePlayerSave(id, 7)
	want.RespawnPresent = true
	want.RespawnPosition = respawnFixturePosition
	want.RespawnDimension = core.Overworld
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
		got.ExhaustionMilli != want.ExhaustionMilli {
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

// TestPlayerV8Fixture 冻结当前 schema 的编码结果，防止字节布局无声漂移。
//
// 冻结的 v7 golden（testdata/player-v7.bin）刻意保留在原处不再生成：它是
// "旧存档仍然可读"的唯一真实证据，见 TestPlayerV7FixtureMigratesToNoRespawn。
func TestPlayerV8Fixture(t *testing.T) {
	want1 := fixturePlayerSave(fixturePlayerID(), 19)
	want1.RespawnPresent = true
	want1.RespawnPosition = respawnFixturePosition
	want1.RespawnDimension = core.Overworld
	encoded, err := Encode(want1)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "player-v8.bin")
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
		t.Fatal("v8 fixture drift; change schema version")
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
	// v8 起再追加重生点三字段，从末尾定位快捷栏的偏移量必须按倒序跳过这三段
	// 尾巴。这里写成具名常量而不是字面数字：往尾部追加字段时就有一条断言静默
	// 改指了新字段，而不是悄悄破坏快捷栏。
	offset := len(wire) - playerRespawnBytes - playerHungerBytes - playerHealthBytes -
		playerBackpackBytes - playerHotbarBytes
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
		{"invalid dimension", func(save *PlayerSave) { save.Current.Dimension = 1 }},
		{"nonfinite current position", func(save *PlayerSave) { save.Current.Position[0] = float32(math.Inf(1)) }},
		{"nonfinite yaw", func(save *PlayerSave) { save.Yaw = float32(math.NaN()) }},
		{"nonfinite pitch", func(save *PlayerSave) { save.Pitch = float32(math.Inf(-1)) }},
		{"pitch too high", func(save *PlayerSave) { save.Pitch = float32(math.Pi/2) + 0.01 }},
		{"invalid safe dimension", func(save *PlayerSave) { save.Safe.Dimension = 1 }},
		{"health above max", func(save *PlayerSave) { save.Health = core.MaxHealth + 1 }},
		{"hunger above max", func(save *PlayerSave) { save.Hunger = core.MaxHunger + 1 }},
		{"saturation above hunger", func(save *PlayerSave) {
			save.SaturationMilli = uint16(save.Hunger)*core.SaturationMilliPerPoint + 1
		}},
		{"invalid respawn dimension", func(save *PlayerSave) {
			save.RespawnPresent = true
			save.RespawnPosition = respawnFixturePosition
			save.RespawnDimension = 1
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
			binary.LittleEndian.PutUint32(p[52:], 1)
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
			binary.LittleEndian.PutUint32(p[77:], 1)
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"safe x", func() []byte { return badFloat(81) }, storagedef.ErrCorrupt},
		{"safe y", func() []byte { return badFloat(85) }, storagedef.ErrCorrupt},
		{"safe z", func() []byte { return badFloat(89) }, storagedef.ErrCorrupt},
		{"invalid health", func() []byte {
			p := bytes.Clone(encoded)
			// 生命值不再是末字节：v7 在它之后追加了三层饥饿状态，v8 再追加重生点。
			// 写成 len(p)-playerRespawnBytes-playerHungerBytes-playerHealthBytes 而
			// 不是 len(p)-1，否则这条断言会静默改指重生点字段，"生命值越界被拒"
			// 就不再被任何用例覆盖。
			p[len(p)-playerRespawnBytes-playerHungerBytes-playerHealthBytes] = core.MaxHealth + 1
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"invalid hunger", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerRespawnBytes-playerHungerBytes] = core.MaxHunger + 1
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"saturation above hunger", func() []byte {
			p := bytes.Clone(encoded)
			// 饱和度紧随饥饿值，取 hunger×1000 + 1 恰好越过上界一个千分位。
			hungerOffset := len(p) - playerRespawnBytes - playerHungerBytes
			binary.LittleEndian.PutUint16(
				p[hungerOffset+1:],
				uint16(p[hungerOffset])*core.SaturationMilliPerPoint+1,
			)
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		{"respawn flag", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerRespawnBytes] = 2
			repairPlayerCRC(p)
			return p
		}, storagedef.ErrCorrupt},
		// 位置与维度字节只在 present=1 时携带语义（present=0 时规范为零），
		// 因此这几条先置位 flag 再投毒，保证变异真正抵达校验层。
		{"respawn x", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerRespawnBytes] = 1
			return badFloatAt(p, len(p)-playerRespawnBytes+1)
		}, storagedef.ErrCorrupt},
		{"respawn y", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerRespawnBytes] = 1
			return badFloatAt(p, len(p)-playerRespawnBytes+5)
		}, storagedef.ErrCorrupt},
		{"respawn z", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerRespawnBytes] = 1
			return badFloatAt(p, len(p)-playerRespawnBytes+9)
		}, storagedef.ErrCorrupt},
		{"respawn dimension", func() []byte {
			p := bytes.Clone(encoded)
			p[len(p)-playerRespawnBytes] = 1
			binary.LittleEndian.PutUint32(p[len(p)-playerRespawnBytes+13:], 1)
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

// TestPlayerSchemaV8KeepsM4EItems 原先位于 chunk 域的 chunk_furnace_test.go：
// 拆分按「跟随被测主体」落位，其被测主体是 player codec，随 player 域入包。
func TestPlayerSchemaV8KeepsM4EItems(t *testing.T) {
	if CurrentSchema != 8 {
		t.Fatalf("玩家 schema = %d，想要 8", CurrentSchema)
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
