package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定玩家弓的拉弓状态机与发射结算（spec player-bow）：
//
//   - 拉弓是持弓时主输入位的权威语义：逐 tick 累加、(slot, item) 快照失配
//     即重置、松开是唯一发射判定点，进度 <6 不发射、6..19 短档、≥20 满档；
//   - 发射在单一 tick 内原子完成：`ConsumeItem(ItemArrow)` 失败整体不发生，
//     成功扣弓耐久 1（归零换损坏形态）并从眼位沿视线生成箭；
//   - 中断矩阵（切格/开容器/受伤/死亡/复位/视野）清零不发射，且中断优先于
//     同 tick 的发射结算（沿进食先例）；
//   - 持弓排除近战意图与采掘推进，损坏的弓没有任何拉弓语义。
//
// 弹药预检与消耗的固定扫描序（快捷栏 0..8 → 背包 9..35）由 core 侧测试锁定，
// 这里经发射路径验证两层防线：进入拉弓的只读预检 + 发射时 `ConsumeItem` 兜底。

// bowTestSession 是本文件全部夹具共用的会话号；每条用例各建一个引擎，号码
// 不会互相干扰。发射的 owner 身份就是它（spec：箭不命中持有者本人，命中扫描
// 按会话 ID 排除）。
const bowTestSession = SessionID(83)

// fullDurabilityBow 返回一把满耐久的完好弓栈。
func fullDurabilityBow() core.ItemStack {
	durability, _ := core.ItemMaxDurability(core.ItemBow)
	return core.ItemStack{Item: core.ItemBow, Count: 1, Durability: durability}
}

// readyBowPlayer 返回一名已激活、站在平坦地面上、生命与饥饿全满的玩家，并在
// 快捷栏 0 号格放一把满耐久弓、选中它、视线沿 -Z（yaw=0、pitch=0）。
func readyBowPlayer(t *testing.T) (*Engine, *playerState) {
	t.Helper()
	engine := readyRegenPlayer(t, bowTestSession, core.MaxHealth)
	player := engine.sessions[bowTestSession].player
	player.inventory.Hotbar.Slots[0] = fullDurabilityBow()
	player.inventory.Hotbar.Selected = 0
	player.yaw = 0
	player.pitch = 0
	return engine, player
}

// countItem 统计完整物品状态中某物品的总数量，供「精确不变/恰好减一」类断言
// 使用。零数量残留栈按 0 计入，与消耗语义一致。
func countItem(inventory core.Inventory, id core.ItemID) uint8 {
	total := uint8(0)
	for slot := uint8(0); slot < core.InventorySlots; slot++ {
		stack, _ := inventory.Slot(slot)
		if stack.Item == id {
			total += stack.Count
		}
	}
	return total
}

// holdBowDraw 让玩家按住主输入位推进 ticks 个权威 tick。夹具沿进食用例的
// 纪律直写权威结构体而不是命令：状态机的输入是「本 tick 主输入位是否按住」，
// 命令层路径由非法输入用例单独覆盖。
func holdBowDraw(engine *Engine, player *playerState, ticks int) {
	player.miningHeld = true
	for range ticks {
		advanceActorsTick(engine)
	}
}

// releaseBow 松开主输入位并推进一个权威 tick：松开即唯一的发射判定点。
func releaseBow(engine *Engine, player *playerState) {
	player.miningHeld = false
	advanceActorsTick(engine)
}

// requireNoFire 断言发射未发生：无投射物、弹药总数与选中格弓栈逐位不变。
func requireNoFire(
	t *testing.T,
	engine *Engine,
	player *playerState,
	arrowsBefore uint8,
	bowBefore core.ItemStack,
) {
	t.Helper()
	if got := len(engine.projectiles.entries); got != 0 {
		t.Fatalf("投射物数量=%d，想要 0（发射未发生）", got)
	}
	if got := countItem(player.inventory, core.ItemArrow); got != arrowsBefore {
		t.Fatalf("箭数量=%d，想要精确保持 %d", got, arrowsBefore)
	}
	if got := player.inventory.Hotbar.Slots[0]; got != bowBefore {
		t.Fatalf("弓所在格=%+v，想要精确保持 %+v（耐久不得消耗）", got, bowBefore)
	}
}

// TestBowFullDrawReleaseFiresFullArrow 覆盖 Scenario「拉满松开发射」：按住
// 20 tick 松开，消耗恰好 1 支箭，生成满档箭（伤害 5、初速 30 格/秒），出生点
// 是眼位、初速沿视线、owner 是持有者会话，弓耐久恰减 1。
func TestBowFullDrawReleaseFiresFullArrow(t *testing.T) {
	engine, player := readyBowPlayer(t)
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 5}
	arrowsBefore := countItem(player.inventory, core.ItemArrow)
	bowBefore := player.inventory.Hotbar.Slots[0]

	holdBowDraw(engine, player, 20)
	if player.bow.progressTicks != 20 {
		t.Fatalf("拉弓进度=%d，想要 20（开始的那一 tick 算第 1 tick，与采掘/进食同义）",
			player.bow.progressTicks)
	}
	releaseBow(engine, player)

	if got := len(engine.projectiles.entries); got != 1 {
		t.Fatalf("发射后投射物数量=%d，想要恰好 1", got)
	}
	arrow := engine.projectiles.entries[0]
	if arrow.kind != projectileKindArrow {
		t.Fatalf("弹种=%d，想要箭 %d", arrow.kind, projectileKindArrow)
	}
	if arrow.owner != uint64(bowTestSession) {
		t.Fatalf("owner=%d，想要持有者会话 %d", arrow.owner, uint64(bowTestSession))
	}
	if arrow.damage != projectileArrowFullDamage {
		t.Fatalf("伤害=%d，想要满档 %d", arrow.damage, projectileArrowFullDamage)
	}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	if arrow.position != eye {
		t.Fatalf("出生点=%v，想要眼位 %v", arrow.position, eye)
	}
	if want := LookDirection(player.yaw, player.pitch).Mul(projectileArrowFullSpeed); arrow.velocity != want {
		t.Fatalf("初速=%v，想要视线方向 ×%v 格/秒", arrow.velocity, want)
	}
	if got := countItem(player.inventory, core.ItemArrow); got != arrowsBefore-1 {
		t.Fatalf("发射后箭数量=%d，想要恰好 %d（消耗 1 支）", got, arrowsBefore-1)
	}
	if got := player.inventory.Hotbar.Slots[0]; got.Item != core.ItemBow ||
		got.Durability != bowBefore.Durability-1 {
		t.Fatalf("发射后弓=%+v，想要完好形态且耐久恰减 1（%d）", got, bowBefore.Durability-1)
	}
	if player.bow != (bowState{}) {
		t.Fatalf("发射后拉弓状态=%+v，想要清空", player.bow)
	}
}

// TestBowShortDrawReleaseFiresShortArrow 覆盖短档发射：6..19 tick 松开为短档
// （伤害 2、初速 16 格/秒），耐久与弹药同样恰耗 1。
func TestBowShortDrawReleaseFiresShortArrow(t *testing.T) {
	engine, player := readyBowPlayer(t)
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 2}
	arrowsBefore := countItem(player.inventory, core.ItemArrow)
	durabilityBefore := player.inventory.Hotbar.Slots[0].Durability

	holdBowDraw(engine, player, 10)
	releaseBow(engine, player)

	if got := len(engine.projectiles.entries); got != 1 {
		t.Fatalf("发射后投射物数量=%d，想要恰好 1", got)
	}
	arrow := engine.projectiles.entries[0]
	if arrow.damage != projectileArrowShortDamage {
		t.Fatalf("伤害=%d，想要短档 %d", arrow.damage, projectileArrowShortDamage)
	}
	speed := arrow.velocity.Len()
	if speed != projectileArrowShortSpeed {
		t.Fatalf("初速=%v，想要 %v 格/秒", speed, projectileArrowShortSpeed)
	}
	if got := countItem(player.inventory, core.ItemArrow); got != arrowsBefore-1 {
		t.Fatalf("发射后箭数量=%d，想要恰好 %d", got, arrowsBefore-1)
	}
	if got := player.inventory.Hotbar.Slots[0].Durability; got != durabilityBefore-1 {
		t.Fatalf("发射后弓耐久=%d，想要 %d", got, durabilityBefore-1)
	}
}

// TestBowReleaseTierBoundaries 钉住发射档位的逐 tick 边界：<6 不发射、6 短档、
// 19 仍短档、20 满档、更长按住保持满档。每条子用例各建引擎，只看「是否发射、
// 落在哪一档」这一对读数。
func TestBowReleaseTierBoundaries(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		heldTicks int
		wantFire  bool
		wantFull  bool
	}{
		{"拉弓 3 tick 过早松开", 3, false, false},
		{"拉弓 5 tick 仍不足", 5, false, false},
		{"拉弓 6 tick 恰达短档", 6, true, false},
		{"拉弓 19 tick 仍短档", 19, true, false},
		{"拉弓 20 tick 恰达满档", 20, true, true},
		{"拉弓 40 tick 保持满档", 40, true, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			engine, player := readyBowPlayer(t)
			player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 3}
			arrowsBefore := countItem(player.inventory, core.ItemArrow)
			bowBefore := player.inventory.Hotbar.Slots[0]

			holdBowDraw(engine, player, testCase.heldTicks)
			releaseBow(engine, player)

			if !testCase.wantFire {
				requireNoFire(t, engine, player, arrowsBefore, bowBefore)
				return
			}
			if got := len(engine.projectiles.entries); got != 1 {
				t.Fatalf("发射后投射物数量=%d，想要恰好 1", got)
			}
			arrow := engine.projectiles.entries[0]
			wantDamage := projectileArrowShortDamage
			wantSpeed := projectileArrowShortSpeed
			if testCase.wantFull {
				wantDamage = projectileArrowFullDamage
				wantSpeed = projectileArrowFullSpeed
			}
			if arrow.damage != wantDamage {
				t.Fatalf("伤害=%d，想要 %d", arrow.damage, wantDamage)
			}
			if got := arrow.velocity.Len(); got != wantSpeed {
				t.Fatalf("初速=%v，想要 %v 格/秒", got, wantSpeed)
			}
			if got := countItem(player.inventory, core.ItemArrow); got != arrowsBefore-1 {
				t.Fatalf("发射后箭数量=%d，想要恰好 %d", got, arrowsBefore-1)
			}
		})
	}
}

// TestBowInterruptionMatrixClearsWithoutFiring 覆盖 Scenario「中断矩阵清零不
// 发射」：拉弓进行中依次发生切格、打开容器、受伤、复位、视野未就绪，每种情形
// 清零拉弓状态且不结算任何发射、弹药或耐久消耗。
//
// 切格子用例按 (slot, item) 失配语义拆成两半：切到非弓格即清零；切到另一把弓
// 则进度从 1 重置（防「拉 A 弓射 B 弓」），两者都不得触发发射或消耗。
func TestBowInterruptionMatrixClearsWithoutFiring(t *testing.T) {
	const drawnAt = 10

	t.Run("切换到非弓格位即清零", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Backpack[0] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		holdBowDraw(engine, player, drawnAt)
		player.inventory.Hotbar.Selected = 2
		holdBowDraw(engine, player, 1)
		if player.bow != (bowState{}) {
			t.Fatalf("切到非弓格后拉弓状态=%+v，想要清空", player.bow)
		}
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
	})

	t.Run("切换到另一把弓从 1 重置且不发射", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[1] = fullDurabilityBow()
		player.inventory.Backpack[0] = core.ItemStack{Item: core.ItemArrow, Count: 4}

		holdBowDraw(engine, player, drawnAt)
		player.inventory.Hotbar.Selected = 1
		holdBowDraw(engine, player, 1)
		want := bowState{slot: 1, item: core.ItemBow, progressTicks: 1}
		if player.bow != want {
			t.Fatalf("切到另一把弓后拉弓状态=%+v，想要 %+v（进度必须从 1 重新计时）",
				player.bow, want)
		}
		releaseBow(engine, player)
		// 重置后的进度只有 1，松开不得发射；两把弓的耐久都必须原封不动。
		requireNoFire(t, engine, player, 4, player.inventory.Hotbar.Slots[0])
		if got := player.inventory.Hotbar.Slots[1]; got != fullDurabilityBow() {
			t.Fatalf("新弓=%+v，想要精确保持 %+v", got, fullDurabilityBow())
		}
	})

	t.Run("同格换物即清零", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Backpack[0] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		holdBowDraw(engine, player, drawnAt)
		// 弓整件离开选中格，换成三块泥土：不切 `Selected`，只换内容。
		player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 3}
		holdBowDraw(engine, player, 1)
		if player.bow != (bowState{}) {
			t.Fatalf("换物后拉弓状态=%+v，想要清空", player.bow)
		}
		player.inventory.Hotbar.Slots[0] = bowBefore
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
	})

	t.Run("打开容器即清零", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		holdBowDraw(engine, player, drawnAt)
		// 容器界面打开那一 tick 拉弓输入仍按住：中断发生在 tick 内的拉弓推进。
		// 夹具直写权威标志（与进食用例同形）；发布阶段会把零值容器引用清掉，
		// 因此这里只推进一个 tick。
		engine.sessions[bowTestSession].viewContainer = true
		advanceActorsTick(engine)
		if player.bow != (bowState{}) {
			t.Fatalf("开箱后拉弓状态=%+v，想要清空", player.bow)
		}
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
	})

	t.Run("受到伤害即清零", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		holdBowDraw(engine, player, drawnAt)
		healthBefore := player.health
		player.applyDamage(1)
		if player.health != healthBefore-1 {
			t.Fatalf("受伤后生命值=%d，想要 %d（夹具必须真的扣血）", player.health, healthBefore-1)
		}
		if player.bow != (bowState{}) {
			t.Fatalf("受伤后拉弓状态=%+v，想要清空", player.bow)
		}
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
	})

	t.Run("复位态即清零", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		holdBowDraw(engine, player, drawnAt)
		player.reset = true
		advanceActorsTick(engine)
		if player.bow != (bowState{}) {
			t.Fatalf("复位后拉弓状态=%+v，想要清空", player.bow)
		}
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
	})

	t.Run("视野未就绪不推进", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		player.miningHeld = true
		for tick := 1; tick <= 64; tick++ {
			// 逐 tick 注入空视图快照（与进食/采掘的「视野丢失」夹具同形）：
			// 拉弓进度必须恒为零，而不是推进到某一档后借松开发射。
			fixture := engine.beginTick()
			fixture.context.SetViews(ViewSnapshot{})
			fixture.context.AdvanceActors()
			publishFixture(engine, &fixture)
			if player.bow != (bowState{}) {
				t.Fatalf("视野未就绪第 %d tick 拉弓状态=%+v，想要恒为空", tick, player.bow)
			}
		}
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
	})
}

// TestBowDeathClearsDrawWithoutFiring 覆盖「死亡中断」：生命值直接置零（不走
// 伤害入口，那半边由上一条用例覆盖），死亡结算那 tick 之后拉弓状态必须清空、
// 不得生成投射物；背包里的箭与弓随死亡掉落进世界，而不是被发射消耗。
func TestBowDeathClearsDrawWithoutFiring(t *testing.T) {
	engine, player := readyBowPlayer(t)
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}

	holdBowDraw(engine, player, 10)
	player.health = 0
	player.miningHeld = false
	tick := engine.beginTick()
	tick.context.AdvanceActors()
	tick.context.AdvanceHostiles(nil, &tick.result)
	commitMutation(tick.mutation, &tick.result)
	publishFixture(engine, &tick)
	if player.bow != (bowState{}) {
		t.Fatalf("死亡结算后拉弓状态=%+v，想要清空", player.bow)
	}
	if got := len(engine.projectiles.entries); got != 0 {
		t.Fatalf("死亡结算生成投射物=%d，想要 0", got)
	}
	// 死亡掉落是箭离开背包的唯一正当途径：3×3 已加载区块里必须能找到那 4 支
	// 箭与那把弓，而不是凭空消失。
	dimension := engine.dimension(core.Overworld)
	totals := make(map[core.ItemID]uint8)
	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			info, ok := dimension.Info(core.ChunkPos{X: x, Z: z})
			if !ok || info.Chunk == nil {
				continue
			}
			for item, count := range miningDropTotals(info.Chunk) {
				totals[item] += count
			}
		}
	}
	if totals[core.ItemArrow] != 4 || totals[core.ItemBow] != 1 {
		t.Fatalf("死亡掉落=%+v，想要 4 支箭与 1 把弓", totals)
	}
}

// TestBowInterruptionBeatsSettlementOnReleaseTick 钉住优先序：中断条件与发射
// 条件在同一 tick 同时成立时，中断必须先短路——进度已够发射的一箭不得借松开
// 结算出去。三种中断源各覆盖一条。
func TestBowInterruptionBeatsSettlementOnReleaseTick(t *testing.T) {
	const drawnAt = 19

	t.Run("松开同 tick 打开容器", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		holdBowDraw(engine, player, drawnAt)
		engine.sessions[bowTestSession].viewContainer = true
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
		if player.bow != (bowState{}) {
			t.Fatalf("结算 tick 中断后拉弓状态=%+v，想要清空", player.bow)
		}
	})

	t.Run("松开同 tick 复位态", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		holdBowDraw(engine, player, drawnAt)
		player.reset = true
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
	})

	t.Run("松开同 tick 受伤", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
		arrowsBefore := countItem(player.inventory, core.ItemArrow)
		bowBefore := player.inventory.Hotbar.Slots[0]

		holdBowDraw(engine, player, drawnAt)
		player.applyDamage(1)
		releaseBow(engine, player)
		requireNoFire(t, engine, player, arrowsBefore, bowBefore)
	})
}

// TestBowNeverStartsWithoutArrow 覆盖 Scenario「无箭不开弓」：进入拉弓的起始
// tick 校验背包有箭，无箭时按住任意时长进度逐 tick 恒为零，松开不发射。
func TestBowNeverStartsWithoutArrow(t *testing.T) {
	engine, player := readyBowPlayer(t)
	bowBefore := player.inventory.Hotbar.Slots[0]

	player.miningHeld = true
	for tick := 1; tick <= 64; tick++ {
		advanceActorsTick(engine)
		if player.bow != (bowState{}) {
			t.Fatalf("无箭第 %d tick 拉弓状态=%+v，想要恒为空", tick, player.bow)
		}
	}
	player.miningHeld = false
	advanceActorsTick(engine)
	requireNoFire(t, engine, player, 0, bowBefore)
}

// TestBowMidDrawArrowLossFailsAtomicAtRelease 钉住发射原子的第二层防线：进入
// 拉弓时背包有箭，途中箭被移走不中断拉弓（中断矩阵不含这一条），松开时
// `ConsumeItem` 失败使发射整体不发生——无投射物、无耐久消耗、无进度残留。
func TestBowMidDrawArrowLossFailsAtomicAtRelease(t *testing.T) {
	engine, player := readyBowPlayer(t)
	player.inventory.Backpack[0] = core.ItemStack{Item: core.ItemArrow, Count: 2}
	bowBefore := player.inventory.Hotbar.Slots[0]

	holdBowDraw(engine, player, 10)
	if player.bow.progressTicks != 10 {
		t.Fatalf("移走箭前进度=%d，想要 10", player.bow.progressTicks)
	}
	player.inventory.Backpack[0] = core.ItemStack{}
	holdBowDraw(engine, player, 15)
	if player.bow.progressTicks != 25 {
		t.Fatalf("移走箭后进度=%d，想要 25（箭被移走不是中断条件）", player.bow.progressTicks)
	}
	releaseBow(engine, player)

	requireNoFire(t, engine, player, 0, bowBefore)
	if player.bow != (bowState{}) {
		t.Fatalf("发射失败后拉弓状态=%+v，想要清空（无进度残留）", player.bow)
	}
}

// TestBowAmmoConsumedInFixedScanOrder 经发射路径验证弹药消耗的固定扫描序：
// 先快捷栏 0..8、后背包 9..35，区段内取最低索引。
func TestBowAmmoConsumedInFixedScanOrder(t *testing.T) {
	t.Run("仅背包有箭时从背包消耗", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Backpack[4] = core.ItemStack{Item: core.ItemArrow, Count: 3}

		holdBowDraw(engine, player, 20)
		releaseBow(engine, player)

		if got := len(engine.projectiles.entries); got != 1 {
			t.Fatalf("发射后投射物数量=%d，想要恰好 1", got)
		}
		if got := player.inventory.Backpack[4]; got != (core.ItemStack{Item: core.ItemArrow, Count: 2}) {
			t.Fatalf("背包 9 号区格=%+v，想要剩 2 支", got)
		}
	})

	t.Run("快捷栏优先于背包", func(t *testing.T) {
		engine, player := readyBowPlayer(t)
		player.inventory.Hotbar.Slots[7] = core.ItemStack{Item: core.ItemArrow, Count: 2}
		player.inventory.Backpack[0] = core.ItemStack{Item: core.ItemArrow, Count: 5}

		holdBowDraw(engine, player, 20)
		releaseBow(engine, player)

		if got := len(engine.projectiles.entries); got != 1 {
			t.Fatalf("发射后投射物数量=%d，想要恰好 1", got)
		}
		if got := player.inventory.Hotbar.Slots[7]; got != (core.ItemStack{Item: core.ItemArrow, Count: 1}) {
			t.Fatalf("快捷栏 7 号格=%+v，想要剩 1 支（快捷栏先耗）", got)
		}
		if got := player.inventory.Backpack[0]; got != (core.ItemStack{Item: core.ItemArrow, Count: 5}) {
			t.Fatalf("背包首格=%+v，想要精确保持 5 支", got)
		}
	})
}

// TestBowLastDurabilityPointSwapsToBrokenForm 覆盖 Scenario「弓耐久归零换损坏
// 形态」：所选格的弓耐久恰剩 1 点时发射，该格换为损坏形态，本次发射照常生成
// 投射物并消耗弹药。
func TestBowLastDurabilityPointSwapsToBrokenForm(t *testing.T) {
	engine, player := readyBowPlayer(t)
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemBow, Count: 1, Durability: 1}
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 1}

	holdBowDraw(engine, player, 20)
	releaseBow(engine, player)

	if got := len(engine.projectiles.entries); got != 1 {
		t.Fatalf("发射后投射物数量=%d，想要恰好 1（换形态不得吞掉本次发射）", got)
	}
	if got := player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemBrokenBow, Count: 1}) {
		t.Fatalf("发射后选中格=%+v，想要损坏形态的弓", got)
	}
	if got := countItem(player.inventory, core.ItemArrow); got != 0 {
		t.Fatalf("发射后箭数量=%d，想要恰好耗尽", got)
	}
	if player.bow != (bowState{}) {
		t.Fatalf("发射后拉弓状态=%+v，想要清空", player.bow)
	}

	// 弹药耗尽的那次发射之后（spec Scenario 后半句）：再拉弓必须因无箭不开弓
	// ——进度逐 tick 恒为零，损坏形态也拉不起来。
	player.miningHeld = true
	for tick := 1; tick <= 8; tick++ {
		advanceActorsTick(engine)
		if player.bow != (bowState{}) {
			t.Fatalf("耗尽弹药后第 %d tick 拉弓状态=%+v，想要恒为空", tick, player.bow)
		}
	}
	player.miningHeld = false
	advanceActorsTick(engine)
	if got := len(engine.projectiles.entries); got != 1 {
		t.Fatalf("补拉的 tick 又生成投射物=%d，想要仍是发射那一次的 1", got)
	}
}

// TestBowBrokenBowHasNoDrawSemantics 覆盖 Scenario「损坏弓不可拉弓」：手持
// 损坏形态按住主输入位任意时长，不得产生拉弓进度，松开不发射。
func TestBowBrokenBowHasNoDrawSemantics(t *testing.T) {
	engine, player := readyBowPlayer(t)
	broken := core.ItemStack{Item: core.ItemBrokenBow, Count: 1}
	player.inventory.Hotbar.Slots[0] = broken
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
	arrowsBefore := countItem(player.inventory, core.ItemArrow)

	player.miningHeld = true
	for tick := 1; tick <= 64; tick++ {
		advanceActorsTick(engine)
		if player.bow != (bowState{}) {
			t.Fatalf("损坏弓第 %d tick 拉弓状态=%+v，想要恒为空", tick, player.bow)
		}
	}
	player.miningHeld = false
	advanceActorsTick(engine)
	requireNoFire(t, engine, player, arrowsBefore, broken)
}

// TestBowHolderGeneratesNoMeleeIntent 覆盖 Scenario「持弓点击实体不近战」：
// 手持任一形态弓时主输入位不产生近战意图（目标不受击），主输入位让渡给拉弓
// 域。铁剑对照组证明夹具本身能造成近战伤害——排除规则只对弓成立。
func TestBowHolderGeneratesNoMeleeIntent(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		held       core.ItemID
		wantHealth uint8
	}{
		{"完好弓不产生近战意图", core.ItemBow, core.MaxHealth},
		{"损坏的弓不产生近战意图", core.ItemBrokenBow, core.MaxHealth},
		{"对照组:铁剑仍近战", core.ItemIronSword, core.MaxHealth - uint8(core.WeaponDamage(core.ItemIronSword))},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			engine, sessions := readyMeleePlayers(t, 2)
			setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
			setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
			attacker := engine.sessions[sessions[0]].player
			target := engine.sessions[sessions[1]].player
			durability, _ := core.ItemMaxDurability(testCase.held)
			attacker.inventory.Hotbar.Slots[0] = core.ItemStack{
				Item: testCase.held, Count: 1, Durability: durability,
			}
			attacker.inventory.Hotbar.Selected = 0
			attacker.miningHeld = true

			advanceHostilesTick(engine, nil)

			if got := target.health; got != testCase.wantHealth {
				t.Fatalf("手持 %v 时目标生命值=%d，想要 %d", testCase.held, got, testCase.wantHealth)
			}
		})
	}
}

// TestBowHolderMiningIsSuppressed 覆盖 Scenario「持弓不采掘」：手持任一形态弓
// 按住主输入位瞄准石头，采掘推进不得运行——石头不产生进度、不被破坏，弓的
// 耐久也不因采掘尝试变化。
func TestBowHolderMiningIsSuppressed(t *testing.T) {
	for _, testCase := range []struct {
		name string
		held core.ItemID
	}{
		{"完好弓", core.ItemBow},
		{"损坏的弓", core.ItemBrokenBow},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			engine, _, targets := readyMiningPlayers(t, 1)
			target := targets[0]
			player := engine.sessions[1].player
			setMiningHeldItem(player, testCase.held)
			if got := player.inventory.Hotbar.Slots[0].Item; got != testCase.held {
				t.Fatalf("夹具手持=%d，想要 %d", got, testCase.held)
			}

			// 徒手/无该工具挖石头需要 30 tick：超过完成阈值后石头仍在，说明
			// 采掘推进从未运行，而不是差一两个 tick。
			for range 35 {
				advanceMiningOnce(engine)
				if player.mining != (miningState{}) {
					t.Fatalf("手持弓时采掘状态=%+v，想要恒为空", player.mining)
				}
			}
			if got := fluidBlockAt(t, engine, target); got != core.StoneID {
				t.Fatalf("手持弓采掘 35 tick 后目标方块=%d，想要保持石头", got)
			}
			if got := player.inventory.Hotbar.Slots[0]; got.Item != testCase.held {
				t.Fatalf("采掘尝试后手持=%+v，想要保持 %d（弓耐久不得被采掘消耗）",
					got, testCase.held)
			}
		})
	}
}

// TestBowIsNotInAnyMiningToolTable 钉住规格条款「弓不出现在任何采掘规则的工具
// 表」：石头的工具分档对弓按「无该工具」回退（30 tick、不可收获），与徒手同价
// 不同权。
func TestBowIsNotInAnyMiningToolTable(t *testing.T) {
	for _, held := range []core.ItemID{core.ItemBow, core.ItemBrokenBow} {
		ticks, harvestable := miningRule(core.StoneID, held)
		if ticks != 30 || harvestable {
			t.Fatalf("miningRule(石头, %v)=(%d, %v)，想要按无该工具回退 (30, false)", held, ticks, harvestable)
		}
	}
}

// TestBowInvalidInputClearsDrawWithoutFiring 钉住命令层的防御语义：整包被拒的
// 非法输入把主输入位一并作废，拉弓进度随之清零且**不得**借「主输入位失效」
// 结算发射——被拒绝的输入不是玩家松手。本用例全程走生产命令管线。
func TestBowInvalidInputClearsDrawWithoutFiring(t *testing.T) {
	engine, player := readyBowPlayer(t)
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 4}
	arrowsBefore := countItem(player.inventory, core.ItemArrow)
	bowBefore := player.inventory.Hotbar.Slots[0]

	sequence := uint64(100)
	heldInput := Command{
		Session: bowTestSession, Kind: CommandPlayerInput,
		Yaw: 0, Pitch: 0, Mining: true,
	}
	for range 12 {
		sequence++
		next := heldInput
		next.Sequence = sequence
		advancePlayerMovementTick(engine, []Command{next})
	}
	if player.bow.progressTicks != 12 {
		t.Fatalf("命令管线拉弓进度=%d，想要 12", player.bow.progressTicks)
	}

	// 非法输入（MoveX 越界）整包拒绝：主输入位失效，拉弓进度清零、不发射。
	sequence++
	invalid := heldInput
	invalid.Sequence = sequence
	invalid.MoveX = 5
	advancePlayerMovementTick(engine, []Command{invalid})

	if player.bow != (bowState{}) {
		t.Fatalf("非法输入后拉弓状态=%+v，想要清空", player.bow)
	}
	requireNoFire(t, engine, player, arrowsBefore, bowBefore)
}

// TestBowArrowStartsMovingInSpawnTickProjectileStage 钉住阶段契约（design D9）：
// 箭在玩家推进阶段生成、同 tick 稍后的投射物阶段完成第一次弹道推进——箭不是
// 下一 tick 才起飞。
func TestBowArrowStartsMovingInSpawnTickProjectileStage(t *testing.T) {
	engine, player := readyBowPlayer(t)
	player.inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemArrow, Count: 1}

	holdBowDraw(engine, player, 20)
	player.miningHeld = false
	tick := engine.beginTick()
	tick.context.AdvanceActors()
	tick.context.AdvanceHostiles(nil, &tick.result)
	commitMutation(tick.mutation, &tick.result)
	publishFixture(engine, &tick)

	if got := len(engine.projectiles.entries); got != 1 {
		t.Fatalf("同一权威 tick 末投射物数量=%d，想要恰好 1", got)
	}
	arrow := engine.projectiles.entries[0]
	if arrow.age != 1 {
		t.Fatalf("同一权威 tick 内箭龄=%d，想要 1（生成当 tick 完成第一步推进）", arrow.age)
	}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	if arrow.position == eye {
		t.Fatal("生成当 tick 末箭仍在出生点，投射物阶段没有起步")
	}
}

// TestBowStateIsTransient 钉住「拉弓状态瞬态不持久化」：bow 进度既不进 parity
// 哈希，也不进玩家快照——直接改写进度后两个观察面都必须逐位不变。
func TestBowStateIsTransient(t *testing.T) {
	engine, player := readyBowPlayer(t)

	hashBefore, _ := engine.PlayerHash(bowTestSession)
	snapshotBefore := player.snapshot(core.Overworld)

	player.bow = bowState{slot: 0, item: core.ItemBow, progressTicks: 7}

	hashAfter, _ := engine.PlayerHash(bowTestSession)
	snapshotAfter := player.snapshot(core.Overworld)
	if hashBefore != hashAfter {
		t.Fatal("拉弓进度泄漏进 parity 哈希（瞬态字段不得参与哈希编码）")
	}
	if !samePlayerSnapshot(snapshotBefore, snapshotAfter) {
		t.Fatal("拉弓进度泄漏进玩家快照（瞬态字段不得持久化）")
	}
}

// samePlayerSnapshot 比较两份玩家快照的持久化面：`Safe` 是每次快照现拷贝的
// 指针，按指向值比较；其余字段全部值语义，清空指针后整体 `==`。
func samePlayerSnapshot(before, after PlayerSnapshot) bool {
	switch {
	case before.Safe == nil && after.Safe == nil:
	case before.Safe == nil || after.Safe == nil:
		return false
	case *before.Safe != *after.Safe:
		return false
	}
	before.Safe, after.Safe = nil, nil
	return before == after
}
