package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 本文件锁定护甲装备互换、近战减免、耐久消耗与死亡掉落的权威结算：
// 装备区唯一写者是权威 sim；减免只挂在近战结算点（settleCombatIntent），
// 摔落/溺水/饥饿经 applyDamage 的路径 MUST NOT 减免。

// intactIronArmor 返回一套完好铁质护甲（耐久取各自上限），点数合计 15。
func intactIronArmor() [core.ArmorSlotCount]core.ItemStack {
	return [core.ArmorSlotCount]core.ItemStack{
		core.ArmorSlotHead:  {Item: core.ItemIronHelmet, Count: 1, Durability: 165},
		core.ArmorSlotChest: {Item: core.ItemIronChestplate, Count: 1, Durability: 240},
		core.ArmorSlotLegs:  {Item: core.ItemIronLeggings, Count: 1, Durability: 225},
		core.ArmorSlotFeet:  {Item: core.ItemIronBoots, Count: 1, Durability: 195},
	}
}

// brokenIronArmor 返回一套损坏形态（耐久 0）的铁质护甲：保留在槽内、贡献 0 点。
func brokenIronArmor() [core.ArmorSlotCount]core.ItemStack {
	return [core.ArmorSlotCount]core.ItemStack{
		core.ArmorSlotHead:  {Item: core.ItemIronHelmet, Count: 1},
		core.ArmorSlotChest: {Item: core.ItemIronChestplate, Count: 1},
		core.ArmorSlotLegs:  {Item: core.ItemIronLeggings, Count: 1},
		core.ArmorSlotFeet:  {Item: core.ItemIronBoots, Count: 1},
	}
}

// armorDurabilityCap 返回护甲件的耐久上限；缺失上限在测试内直接失败。
func armorDurabilityCap(t *testing.T, item core.ItemID) uint16 {
	t.Helper()
	max, ok := core.ItemMaxDurability(item)
	if !ok {
		t.Fatalf("物品 %d 应登记耐久上限", item)
	}
	return max
}

// equipArmorCommand 构造一条装备互换命令：目标槽由件类唯一确定，载荷只有序号。
func equipArmorCommand(session SessionID) Command {
	return Command{Session: session, Sequence: 7, Kind: CommandEquipArmor}
}

// TestEquipArmorSwapsHotbarAndArmorSlot 锁定装备互换的三态：空槽穿戴、占用互换、
// 非护甲拒绝且逐位不变。互换在同一权威 tick 内原子成立，成功路径按既有背包
// 同步纪律置脏。
func TestEquipArmorSwapsHotbarAndArmorSlot(t *testing.T) {
	t.Run("空槽穿戴", func(t *testing.T) {
		engine, session := readyMovementPlayer(t)
		player := engine.sessions[session].player
		player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 165}
		player.inventory.Hotbar.Selected = 0

		result := applyPlayerCommandsTick(engine, []Command{equipArmorCommand(session)})

		if len(result.Rejected) != 0 {
			t.Fatalf("合法装备被拒绝：%+v", result.Rejected)
		}
		want := core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 165}
		if got := player.armor[core.ArmorSlotHead]; got != want {
			t.Fatalf("头部槽=%+v，想要 %+v", got, want)
		}
		if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{}) {
			t.Fatalf("所选快捷栏格=%+v，想要为空", got)
		}
		if len(result.Inventories) != 1 {
			t.Fatalf("成功互换没有发布背包更新：%+v", result.Inventories)
		}
		if got := result.Players[0].ArmorPoints; got != 2 {
			t.Fatalf("ArmorPoints=%d，想要 2", got)
		}
	})

	t.Run("占用互换", func(t *testing.T) {
		engine, session := readyMovementPlayer(t)
		player := engine.sessions[session].player
		worn := core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 100}
		player.armor[core.ArmorSlotHead] = worn
		player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 165}
		player.inventory.Hotbar.Selected = 0

		result := applyPlayerCommandsTick(engine, []Command{equipArmorCommand(session)})

		if len(result.Rejected) != 0 {
			t.Fatalf("合法互换被拒绝：%+v", result.Rejected)
		}
		if got := player.armor[core.ArmorSlotHead]; got != (core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 165}) {
			t.Fatalf("头部槽=%+v，想要换上新头盔", got)
		}
		if got := player.inventory.Hotbar.Slots[0]; got != worn {
			t.Fatalf("所选快捷栏格=%+v，想要换回原头部护甲件 %+v", got, worn)
		}
		if got := result.Players[0].ArmorPoints; got != 2 {
			t.Fatalf("ArmorPoints=%d，想要 2", got)
		}
	})

	t.Run("跨槽位穿戴不触碰其他槽", func(t *testing.T) {
		engine, session := readyMovementPlayer(t)
		player := engine.sessions[session].player
		worn := core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 100}
		player.armor[core.ArmorSlotHead] = worn
		player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronChestplate, Count: 1, Durability: 240}
		player.inventory.Hotbar.Selected = 0

		result := applyPlayerCommandsTick(engine, []Command{equipArmorCommand(session)})

		if len(result.Rejected) != 0 {
			t.Fatalf("合法装备被拒绝：%+v", result.Rejected)
		}
		// 目标槽由件类唯一确定：胸甲换的是胸部槽，头部槽原样保留。
		if got := player.armor[core.ArmorSlotChest]; got != (core.ItemStack{Item: core.ItemIronChestplate, Count: 1, Durability: 240}) {
			t.Fatalf("胸部槽=%+v，想要换上胸甲", got)
		}
		if got := player.armor[core.ArmorSlotHead]; got != worn {
			t.Fatalf("头部槽=%+v，想要保持原护甲件", got)
		}
		if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{}) {
			t.Fatalf("所选快捷栏格=%+v，想要为空", got)
		}
		if got := result.Players[0].ArmorPoints; got != 8 {
			t.Fatalf("ArmorPoints=%d，想要 8", got)
		}
	})

	t.Run("非护甲拒绝且逐位不变", func(t *testing.T) {
		engine, session := readyMovementPlayer(t)
		player := engine.sessions[session].player
		player.armor = intactIronArmor()
		player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStoneSword, Count: 1, Durability: 131}
		player.inventory.Hotbar.Selected = 0
		wantArmor := player.armor
		wantInventory := player.inventory

		result := applyPlayerCommandsTick(engine, []Command{equipArmorCommand(session)})

		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectNotArmor {
			t.Fatalf("拒绝=%+v，想要恰好一条 not_armor", result.Rejected)
		}
		if result.Rejected[0].Sequence != 7 {
			t.Fatalf("拒绝序号=%d，想要 7", result.Rejected[0].Sequence)
		}
		if player.armor != wantArmor {
			t.Fatalf("拒绝路径改动了装备槽：%+v", player.armor)
		}
		if player.inventory != wantInventory {
			t.Fatalf("拒绝路径改动了背包：%+v", player.inventory)
		}
		if len(result.Inventories) != 0 {
			t.Fatalf("拒绝路径发布了背包更新：%+v", result.Inventories)
		}
	})

	t.Run("未激活玩家拒绝", func(t *testing.T) {
		engine, session := readyMovementPlayer(t)
		player := engine.sessions[session].player
		player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 165}
		player.lifecycle = PlayerPendingSpawn
		wantArmor := player.armor

		result := applyPlayerCommandsTick(engine, []Command{equipArmorCommand(session)})

		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectPlayerNotReady {
			t.Fatalf("拒绝=%+v，想要 player_not_ready", result.Rejected)
		}
		if player.armor != wantArmor {
			t.Fatalf("拒绝路径改动了装备槽：%+v", player.armor)
		}
	})
}

// TestEquipArmorKeepsSingleStackInvariant 锁定装备槽的 Count≤1 不变量：互换路径
// 写入装备槽的栈必须恰好一件，任何多件栈都进不了装备区。
func TestEquipArmorKeepsSingleStackInvariant(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	// 多件栈在规范背包里不可构造（护甲堆叠上限 1）；这里直接构造越界输入，
	// 钉住互换入口的防御判定，让不变量不依赖上游纪律。
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronHelmet, Count: 3, Durability: 165}
	player.inventory.Hotbar.Selected = 0
	wantArmor := player.armor
	wantInventory := player.inventory

	result := applyPlayerCommandsTick(engine, []Command{equipArmorCommand(session)})

	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectNotArmor {
		t.Fatalf("拒绝=%+v，想要 not_armor", result.Rejected)
	}
	if player.armor != wantArmor || player.inventory != wantInventory {
		t.Fatalf("拒绝路径改动了状态：armor=%+v inventory=%+v", player.armor, player.inventory)
	}
}

// TestEquipBrokenArmorFormIsWearable 锁定损坏形态可装备：耐久 0 的护甲件可穿戴、
// 贡献 0 点、不消失。
func TestEquipBrokenArmorFormIsWearable(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronHelmet, Count: 1}
	player.inventory.Hotbar.Selected = 0

	result := applyPlayerCommandsTick(engine, []Command{equipArmorCommand(session)})

	if len(result.Rejected) != 0 {
		t.Fatalf("损坏形态装备被拒绝：%+v", result.Rejected)
	}
	want := core.ItemStack{Item: core.ItemIronHelmet, Count: 1}
	if got := player.armor[core.ArmorSlotHead]; got != want {
		t.Fatalf("头部槽=%+v，想要损坏形态 %+v", got, want)
	}
	if got := result.Players[0].ArmorPoints; got != 0 {
		t.Fatalf("损坏形态 ArmorPoints=%d，想要 0", got)
	}
}

// TestRestoreCarriesArmorAndSnapshot 锁定装备沿恢复/快照两端进出：恢复装载四槽，
// 快照原样携带，点数随玩家状态投影；缺失恢复路径得到全空装备。
func TestRestoreCarriesArmorAndSnapshot(t *testing.T) {
	armor := brokenIronArmor()
	armor[core.ArmorSlotChest] = core.ItemStack{Item: core.ItemIronChestplate, Count: 1, Durability: 240}
	const id = SessionID(33)
	engine := NewEngine(0, 0, 0)
	engine.RegisterPlayer(id, PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
		Armor:          armor,
	})
	loadMovementChunk(t, engine.dimension(core.Overworld), movementFlatChunk(core.ChunkPos{}))
	result := advanceActorsTick(engine)

	player := engine.sessions[id].player
	if player.armor != armor {
		t.Fatalf("激活后装备槽=%+v，想要 %+v", player.armor, armor)
	}
	snapshot, ok := engine.PlayerSnapshot(id)
	if !ok {
		t.Fatal("已出生玩家必须可快照")
	}
	if snapshot.Armor != armor {
		t.Fatalf("快照装备槽=%+v，想要 %+v", snapshot.Armor, armor)
	}
	if got := result.Players[0].ArmorPoints; got != 6 {
		t.Fatalf("ArmorPoints=%d，想要 6", got)
	}

	engine.RegisterSession(SessionID(34), core.Overworld, core.ChunkPos{})
	if got := engine.sessions[SessionID(34)].player.armor; got != ([core.ArmorSlotCount]core.ItemStack{}) {
		t.Fatalf("默认注册装备槽=%+v，想要全空", got)
	}
}

// TestRegisterPlayerRejectsMalformedArmor 锁定恢复输入的装备区校验：多件栈、
// 错槽、非护甲物品、超上限耐久与非空残迹都在注册边界 fail fast，绝不静默接受。
func TestRegisterPlayerRejectsMalformedArmor(t *testing.T) {
	cases := map[string][core.ArmorSlotCount]core.ItemStack{
		"多件栈": {
			core.ArmorSlotHead: {Item: core.ItemIronHelmet, Count: 2, Durability: 165},
		},
		"错槽": {
			core.ArmorSlotChest: {Item: core.ItemIronHelmet, Count: 1, Durability: 165},
		},
		"非护甲物品": {
			core.ArmorSlotHead: {Item: core.ItemStone, Count: 1},
		},
		"超上限耐久": {
			core.ArmorSlotHead: {Item: core.ItemIronHelmet, Count: 1, Durability: 166},
		},
		"非空残迹": {
			core.ArmorSlotHead: {Item: core.ItemNone, Count: 0, Durability: 3},
		},
	}
	for name, armor := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("非法装备恢复输入没有被拒绝")
				}
			}()
			engine := NewEngine(0, 0, 0)
			engine.RegisterPlayer(SessionID(35), PlayerRestore{
				SpawnDimension: core.Overworld,
				SpawnAnchor:    core.ChunkPos{},
				Armor:          armor,
			})
		})
	}
}

// TestHostileMeleeReducedByArmor 走完整战斗阶段：满套 15 点的玩家被夜行者
// 近战命中（intent 伤害 3），实际扣血 1，且同一次结算全件各耗恰好 1 耐久。
func TestHostileMeleeReducedByArmor(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{2.0, 1, 0.5})
	engine.sessions[session].player.armor = intactIronArmor()
	action := HostileAction{ID: 3, AttackTarget: true, TargetSession: session}
	if !validHostileAction(action) {
		t.Fatal("hostile attack intent 不是合法 owner 输入")
	}

	advanceHostilesTick(engine, []HostileAction{action})

	player := engine.sessions[session].player
	if got := player.health; got != core.MaxHealth-1 {
		t.Fatalf("减免后 health=%d，想要 %d", got, core.MaxHealth-1)
	}
	for slot, stack := range player.armor {
		if want := armorDurabilityCap(t, stack.Item) - 1; stack.Durability != want {
			t.Fatalf("槽位 %d 耐久=%d，想要恰好耗 1 后的 %d", slot, stack.Durability, want)
		}
	}
}

// TestPvPMeleeReducedByArmor 锁定玩家→玩家 intent 同样按冻结点数减免：
// 木剑 4 伤 × 15 点 → 扣 1，战斗确认广播保持原始伤害语义。
func TestPvPMeleeReducedByArmor(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	attacker := engine.sessions[sessions[0]].player
	attacker.inventory.Hotbar.Selected = 0
	attacker.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemWoodenSword, Count: 1, Durability: 59}
	target := engine.sessions[sessions[1]].player
	target.armor = intactIronArmor()
	attacker.miningHeld = true

	result := advanceHostilesTick(engine, nil)

	if got := target.health; got != core.MaxHealth-1 {
		t.Fatalf("减免后 target health=%d，想要 %d", got, core.MaxHealth-1)
	}
	for slot, stack := range target.armor {
		if want := armorDurabilityCap(t, stack.Item) - 1; stack.Durability != want {
			t.Fatalf("槽位 %d 耐久=%d，想要恰好耗 1 后的 %d", slot, stack.Durability, want)
		}
	}
	if len(result.CombatHits) != 1 || result.CombatHits[0].Damage != 4 {
		t.Fatalf("CombatHits=%+v，想要原始伤害 4 的单条确认", result.CombatHits)
	}
}

// TestFallDamageNotReduced 锁定摔落伤害不进护甲减免：同一摔落曲线下，穿甲与
// 裸装扣血逐位相同，且护甲减免绝不进入 applyDamage 共用入口。
func TestFallDamageNotReduced(t *testing.T) {
	fallHealth := func(wearArmor bool) uint8 {
		engine, session := readyMovementPlayer(t)
		player := engine.sessions[session].player
		if wearArmor {
			player.armor = intactIronArmor()
		}
		player.health = core.MaxHealth
		player.peakY = player.state.Position.Y() + 10
		player.applyFallDamage()
		return player.health
	}
	bare := fallHealth(false)
	armored := fallHealth(true)
	if armored != bare {
		t.Fatalf("穿甲摔落 health=%d，想要与裸装 %d 逐位相同", armored, bare)
	}
	if armored >= core.MaxHealth {
		t.Fatalf("夹具没有产生摔落伤害：health=%d", armored)
	}
}

// TestCombatKnockbackUnaffectedByArmor 锁定击退只按原始 intent 结算：
// 同一几何下，有无护甲点数的击退速度逐位一致。
func TestCombatKnockbackUnaffectedByArmor(t *testing.T) {
	knockbackVelocity := func(t *testing.T, armorPoints uint8) mgl32.Vec3 {
		t.Helper()
		engine, sessions := readyMeleePlayers(t, 2)
		attackerPosition := mgl32.Vec3{0, 1, 0}
		targetPosition := mgl32.Vec3{3, 1, 4}
		setMeleePlayer(engine, sessions[0], attackerPosition, 0)
		setMeleePlayer(engine, sessions[1], targetPosition, 0)
		target := engine.sessions[sessions[1]].player
		target.state.Velocity = mgl32.Vec3{1, 2, 3}
		intent := combatIntent{
			attacker:          combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[0])},
			target:            combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[1])},
			damage:            3,
			targetArmorPoints: armorPoints,
			attackerPosition:  attackerPosition,
			targetPosition:    targetPosition,
		}
		if !engine.settleCombatIntent(&TickResult{}, intent) {
			t.Fatal("合法 intent 未结算")
		}
		return target.state.Velocity
	}
	bare := knockbackVelocity(t, 0)
	armored := knockbackVelocity(t, 15)
	if armored != bare {
		t.Fatalf("穿甲击退=%v，想要与裸装 %v 逐位相同", armored, bare)
	}
}

// TestArmorDurabilityConsumesOnlyOnReduction 锁定耐久消耗的门控：只有产生减免
// 的结算才全件耗 1；点数为 0 或伤害被下限托住（无实际减免）时零消耗；耐久归零
// 就地转损坏形态。
func TestArmorDurabilityConsumesOnlyOnReduction(t *testing.T) {
	t.Run("减免时全件耗 1", func(t *testing.T) {
		engine, player, intent := armoredMeleeTarget(t, 3)
		if !engine.settleCombatIntent(&TickResult{}, intent) {
			t.Fatal("合法 intent 未结算")
		}
		for slot, stack := range player.armor {
			if want := armorDurabilityCap(t, stack.Item) - 1; stack.Durability != want {
				t.Fatalf("槽位 %d 耐久=%d，想要恰好耗 1 后的 %d", slot, stack.Durability, want)
			}
		}
	})

	t.Run("伤害被下限托住不耗", func(t *testing.T) {
		engine, player, intent := armoredMeleeTarget(t, 1)
		health := player.health
		if !engine.settleCombatIntent(&TickResult{}, intent) {
			t.Fatal("合法 intent 未结算")
		}
		if player.health != health-1 {
			t.Fatalf("health=%d，想要下限托住后的 %d", player.health, health-1)
		}
		for slot, stack := range player.armor {
			if want := armorDurabilityCap(t, stack.Item); stack.Durability != want {
				t.Fatalf("槽位 %d 耐久=%d，想要保持原值 %d", slot, stack.Durability, want)
			}
		}
	})

	t.Run("损坏形态 0 点不耗", func(t *testing.T) {
		engine, player, intent := armoredMeleeTarget(t, 3)
		player.armor = brokenIronArmor()
		intent.targetArmorPoints = 0
		health := player.health
		if !engine.settleCombatIntent(&TickResult{}, intent) {
			t.Fatal("合法 intent 未结算")
		}
		if player.health != health-3 {
			t.Fatalf("损坏形态 health=%d，想要全额扣 3 后的 %d", player.health, health-3)
		}
		for slot, stack := range player.armor {
			if stack.Count != 1 || stack.Durability != 0 {
				t.Fatalf("槽位 %d=%+v，想要损坏形态原样保留", slot, stack)
			}
		}
	})

	t.Run("耐久 1 就地转损坏", func(t *testing.T) {
		engine, player, intent := armoredMeleeTarget(t, 3)
		player.armor = [core.ArmorSlotCount]core.ItemStack{}
		player.armor[core.ArmorSlotHead] = core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 1}
		intent.targetArmorPoints = 2
		if !engine.settleCombatIntent(&TickResult{}, intent) {
			t.Fatal("合法 intent 未结算")
		}
		want := core.ItemStack{Item: core.ItemIronHelmet, Count: 1}
		if got := player.armor[core.ArmorSlotHead]; got != want {
			t.Fatalf("头部槽=%+v，想要损坏形态 %+v", got, want)
		}
	})
}

// armoredMeleeTarget 构造「玩家目标 + 敌怪攻击者」的直结算夹具：目标穿满套
// 完好铁甲，intent 携带快照口径的冻结点数与给定伤害。
func armoredMeleeTarget(t *testing.T, damage int32) (*Engine, *playerState, combatIntent) {
	t.Helper()
	engine, session := hostileCombatEngine(t)
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{2.0, 1, 0.5})
	target := engine.sessions[session].player
	target.armor = intactIronArmor()
	return engine, target, combatIntent{
		attacker:          combatActor{kind: core.CombatTargetHostile, id: 3},
		target:            combatActor{kind: core.CombatTargetPlayer, id: uint64(session)},
		dimension:         core.Overworld,
		damage:            damage,
		targetArmorPoints: core.ArmorPoints(target.armor),
		attackerPosition:  mgl32.Vec3{2.0, 1, 0.5},
		targetPosition:    mgl32.Vec3{0.5, 1, 0.5},
	}
}

// TestDeathDropsEquippedArmorAndClearsSlots 锁定死亡掉落：四槽护甲按既有环形
// 外扩纪律与背包一并掉进世界并清空装备槽，掉落物携带原耐久形态。
func TestDeathDropsEquippedArmorAndClearsSlots(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	player.armor = intactIronArmor()
	player.health = 0

	advanceHostilesTick(engine, nil)

	if got := player.armor; got != ([core.ArmorSlotCount]core.ItemStack{}) {
		t.Fatalf("死亡后装备槽=%+v，想要全空", got)
	}
	if player.lifecycle != PlayerPendingSpawn || player.health != core.MaxHealth {
		t.Fatalf("死亡结算后 (lifecycle, health)=(%d, %d)，想要待重生满血",
			player.lifecycle, player.health)
	}
	dropped := map[core.ItemID]uint16{}
	chunk, ok := engine.dimension(core.Overworld).ReadyChunk(core.ChunkPos{})
	if !ok {
		t.Fatal("origin chunk is not ready")
	}
	for slot := range core.DropsPerChunk {
		drop := chunk.Drop(slot)
		if drop.Active && drop.Stack.Item != core.ItemNone {
			dropped[drop.Stack.Item] = drop.Stack.Durability
		}
	}
	for item, want := range map[core.ItemID]uint16{
		core.ItemIronHelmet:     165,
		core.ItemIronChestplate: 240,
		core.ItemIronLeggings:   225,
		core.ItemIronBoots:      195,
	} {
		if got, found := dropped[item]; !found {
			t.Fatalf("死亡掉落缺少护甲件 %d：%+v", item, dropped)
		} else if got != want {
			t.Fatalf("护甲件 %d 掉落耐久=%d，想要 %d", item, got, want)
		}
	}
}

// TestDeathKeepsArmorWhenNoDropCapacity 锁定掉落纪律的另一端：全部掉落槽占满时
// 装备保留在槽内跟随重生，死亡不被阻止、物品不被销毁。
func TestDeathKeepsArmorWhenNoDropCapacity(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	wantArmor := intactIronArmor()
	player.armor = wantArmor
	chunk, ok := engine.dimension(core.Overworld).ReadyChunk(core.ChunkPos{})
	if !ok {
		t.Fatal("origin chunk is not ready")
	}
	for slot := range core.DropsPerChunk {
		chunk.SetDrop(slot, world.DropSlot{
			Active:     true,
			Stack:      core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount},
			BlockIndex: uint32(slot),
		})
	}
	player.health = 0

	advanceHostilesTick(engine, nil)

	if player.armor != wantArmor {
		t.Fatalf("无容量时装备槽=%+v，想要原样保留", player.armor)
	}
	if player.health != core.MaxHealth || player.lifecycle != PlayerPendingSpawn {
		t.Fatalf("死亡被掉落容量阻塞：(health, lifecycle)=(%d, %d)", player.health, player.lifecycle)
	}
}

// TestPlayerHashCoversArmor 锁定 PlayerHash 覆盖装备区：穿戴、耐久变化与换件
// 都必须改变哈希，卸下后回到基线。
func TestPlayerHashCoversArmor(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	baseline, ok := engine.PlayerHash(session)
	if !ok {
		t.Fatal("权威玩家 hash 不可用")
	}
	player := engine.sessions[session].player

	player.armor[core.ArmorSlotHead] = core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 165}
	worn, _ := engine.PlayerHash(session)
	if worn == baseline {
		t.Fatal("穿戴护甲没有改变 hash")
	}

	player.armor[core.ArmorSlotHead] = core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 164}
	damaged, _ := engine.PlayerHash(session)
	if damaged == worn {
		t.Fatal("耐久变化没有改变 hash")
	}

	player.armor[core.ArmorSlotHead] = core.ItemStack{Item: core.ItemIronChestplate, Count: 1, Durability: 240}
	swapped, _ := engine.PlayerHash(session)
	if swapped == worn {
		t.Fatal("换件没有改变 hash")
	}

	player.armor = [core.ArmorSlotCount]core.ItemStack{}
	again, _ := engine.PlayerHash(session)
	if again != baseline {
		t.Fatal("卸下护甲没有回到基线 hash")
	}
}
