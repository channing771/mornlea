package entity

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// naturalSourceBlocks 是「缺省世界生成确实会产出」的方块清单，也是价值闭包的
// 唯一起点：闭包只从世界本身写出的方块出发，绝不把「曾经出现在某个玩家背包里」
// 当成来源——正是这一条纪律让「某个物品悄悄失去自然来源」成为可判定的事实。
//
// 本包刻意不 import `packages/shared/worldgen`（跨模块依赖由
// `packages/audit/dependency_test.go` 钉住），因此清单在这里逐项写出并注明溯源，
// 而不是引用生成器常量。两个来源是：
//
//   - `packages/shared/worldgen/generator.go` 的 15 项材料表，即生成器写进
//     MGW1 header 的全部方块编号（第 1 项 `AirID` 不是产物，不在此列）；
//   - Rust 世界生成器
//     `packages/engine/crates/mornlea_engine/src/worldgen.rs` 的矿石、橡树与
//     短草规则，它们把材料表里的编号实际铺进区块（矿脉、树干树冠、地表装饰）。
//
// `BedrockID` 与 `WaterSourceID` 同样在列：前者是地形底层且没有任何采掘规则
// （`miningRule` 返回「不可采掘」），后者不是采掘目标、只是取水命令唯一的流体源。
// 两者都不会凭空产出物品——列在这里是为了让「生成器写出的方块」与「闭包起点」
// 逐项对齐，任何一项将来获得采掘或交互规则都会被本清单覆盖到，而不是被漏掉。
var naturalSourceBlocks = [...]core.BlockID{
	core.StoneID,       // 材料表第 2 项：地形主体，徒手即 30 tick 可采掘，掉 1 石料
	core.DirtID,        // 材料表第 3 项：地表层，徒手 5 tick，掉 1 泥土
	core.GrassID,       // 材料表第 4 项：地表层，徒手 5 tick，掉 1 草方块
	core.BedrockID,     // 材料表第 5 项：底层，`miningRule` 无规则，永不可采掘
	core.SnowBlockID,   // 材料表第 6 项：雪原地表层，徒手 5 tick，掉 1 雪块
	core.SandID,        // 材料表第 7 项：沙层，徒手 5 tick，掉 1 沙子（熔炼成玻璃）
	core.ClayID,        // 材料表第 8 项：黏土层，徒手 5 tick，掉 1 黏土（熔炼成砖块）
	core.GravelID,      // 材料表第 9 项：砾石层，徒手 5 tick，掉 1 砾石
	core.IronOreID,     // 材料表第 10 项 + 矿石规则：只对完好石镐/铁镐可采掘，掉 1 粗铁
	core.CoalOreID,     // 材料表第 11 项 + 矿石规则：只对完好石镐/铁镐可采掘，掉 1 煤炭
	core.OakLogID,      // 材料表第 12 项 + 橡树规则：树干，任意手持 15 tick，掉 1 原木
	core.LeavesID,      // 材料表第 13 项 + 橡树规则：树冠，任意手持 5 tick，掉 1 树叶
	core.WaterSourceID, // 材料表第 14 项（海平面注水）：取水命令的源格，本身不掉落
	core.ShortGrassID,  // 材料表第 15 项 + 短草装饰规则：小麦种子唯一的自然来源
}

// naturalSourceContributes 报告清单里的一项方块是否真的接入闭包：由
// `core.BlockDrop` 给出确定掉落的方块算，走模型自己专用分支的方块也算——短草的
// 小麦种子（`core.IsWildGrass`，规则一里的短草分支）与水源的取水链
// （`core.IsFluid`，规则四）。基岩这类对任何手持都没有采掘规则的方块按设计放行：
// 它们是清单上方说明里刻意保留的「与生成器对齐」项，本来就不产出物品。
//
// 反过来说，能被真实 `miningRule` 采掘、却既没有 `BlockDrop` 也不接任何专用分支
// 的方块会返回 false：它看起来是个来源，实际一条路径都算不出来。这条判据让清单
// 本身可以被证伪，而不是一份只能人工核对的名单。
func naturalSourceContributes(block core.BlockID) bool {
	if _, ok := core.BlockDrop(block); ok {
		return true
	}
	if core.IsWildGrass(block) || core.IsFluid(block) {
		return true
	}
	for held := core.ItemNone; held < core.ItemIDMax; held++ {
		if required, _ := miningRule(block, held); required != 0 {
			return false
		}
	}
	return true
}

// matureCropHarvests 是成熟作物收获规则的显式表（`completeMining` 的三个成熟
// 分支）：种植要求作物物品自身在闭包内（`core.ItemPlacement` 把种子/马铃薯/
// 胡萝卜物品放置成对应作物方块的阶段 0），生长还要求耕地——耕地由锄头翻出，
// 收获规则唯一的闭包内前置就是「该作物的物品已在闭包内」。未成熟阶段误挖掉回
// 物品自身，与种植互为可逆，不产生新物品，因此不必单列。
//
// 锄头（`core.TillingTool`：石锄、铁锄）是本规则求值之外的**真实世界前置**，
// 而闭包不会为它求值，因此它不是已知前提，而是被断言钉住的：两把锄头都列在
// `TestReachabilityClosureOfSurvivalStart` 的正向能力清单里，任一把的配方消失，
// 守卫立刻报红，而不是让「食物可达」继续建立在注释里的假定上。
var matureCropHarvests = []struct {
	seed  core.ItemID
	drops []core.ItemID
}{
	// 成熟小麦掉 1..3 小麦 + 1..3 种子（`WheatStage7ID` 分支）。
	{core.ItemWheatSeeds, []core.ItemID{core.ItemWheat}},
	// 成熟马铃薯掉 1..4 马铃薯，另有独立 2% 判定的毒土豆（`PotatoStage7ID` 分支）。
	{core.ItemPotato, []core.ItemID{core.ItemPotato, core.ItemPoisonousPotato}},
	// 成熟胡萝卜掉 1..4 胡萝卜（`CarrotStage7ID` 分支）。
	{core.ItemCarrot, []core.ItemID{core.ItemCarrot}},
}

// acceptedUnobtainableItems 是显式接受「在当前规则表下没有自然来源」的物品清单。
// 反向断言把它与推导结果逐项比对，两个方向都会报红：清单之外的任何不可获得
// 物品（新配方以不可获得物品为原料、某物品悄悄失去来源，都属于这一类）必须由人
// 重新裁决；清单之内却真的获得了来源的物品则是过期声明，必须从清单里删掉。
// 每一项都必须写明被接受的理由，理由不成立就不该留在这里。
var acceptedUnobtainableItems = []core.ItemID{
	// 圆石：唯一的物品来源是采掘 `CobblestoneID`，而该方块不参与世界生成（材料表
	// 无圆石、无岩浆、无结构生成），因此它在缺省世界里没有来源。它是纯装饰材料，
	// 不是任何关键能力路径的必需输入（熔炉与石剑已改以石料为原料）。
	core.ItemCobblestone,
	// 平滑石：同圆石，方块不参与世界生成，只作装饰材料。
	core.ItemSmoothStone,
	// 苔藓圆石：同圆石，方块不参与世界生成，只作装饰材料。
	core.ItemMossyCobblestone,
	// 白色羊毛：方块不参与世界生成，世界里也没有绵羊或任何掉落羊毛的生物，
	// 只作装饰材料。
	core.ItemWhiteWool,
	// 红色瓦块：方块不参与世界生成，只作装饰材料。
	core.ItemRoofTile,
	// 马铃薯：唯一来源是种下马铃薯自身长成的成熟作物，物品本身没有自然来源，
	// 属于既有缺口（先于取消初始发放存在）。
	core.ItemPotato,
	// 胡萝卜：同马铃薯，唯一来源是种下胡萝卜自身。
	core.ItemCarrot,
	// 毒马铃薯：马铃薯成熟收获时 2% 概率的额外掉落，随马铃薯一并不可达。
	core.ItemPoisonousPotato,
	// 骨粉：落地骨粉催熟命令时来源与合成配方被一并显式延期（当时以测试直给为准），
	// 至今没有任何生产者——不在掉落表、配方表、熔炼表与生物掉落里，只被催熟命令
	// 消费。属于先于本变更存在的既有缺口，不属于四条关键能力路径，因此接受为不可
	// 获得；其自然来源另立后续行，本清单的作用就是让它不被继续隐形。
	core.ItemBoneMeal,
}

// TestReachabilityClosureOfSurvivalStart 钉住 spec 条款「新世界起点保证关键能力
// 可达」：以世界生成真正写出的方块为起点，对采掘、收获、掉落、取水、工具损坏、
// 合成与熔炼七类规则求价值闭包的不动点，再断言四条关键能力（工作台、照明、
// 采掘工具、食物）与熔炉/熔炼链（铁锭、空桶）都在闭包内，且物品注册表中不在
// 闭包内的物品恰好等于显式声明的已接受集合。
//
// 守卫落在 `packages/server/sim/entity` 包内，是为了直接调用真实 `miningRule`：
// 工具门槛（空手可采哪些、煤炭与铁矿石必须先有镐）是闭包的核心约束，复制一份
// 判定就等于让守卫与权威规则有第二套实现，而工具门槛一旦漂移，闭包结论就不再
// 描述真实世界。
func TestReachabilityClosureOfSurvivalStart(t *testing.T) {
	// 非空洞自证：守卫必须先证明起点清单本身可信，再谈断言结果。重复项不会增加
	// 起点，只会掩盖漏写的方块；混进一个既不产出物品、又不接任何专用分支的方块，
	// 则会让闭包起点凭空多出一项而不被察觉。
	seen := make(map[core.BlockID]bool, len(naturalSourceBlocks))
	for _, block := range naturalSourceBlocks {
		if seen[block] {
			t.Fatalf("自然来源方块清单重复列出方块 %d：重复项不增加起点，只会掩盖漏写的方块", block)
		}
		seen[block] = true
		if !naturalSourceContributes(block) {
			t.Fatalf("自然来源方块清单里的方块 %d 既无 `core.BlockDrop`、也不接入短草/流体分支，"+
				"却仍被真实 `miningRule` 判为可采掘：它产出不了任何物品，闭包起点却把它算作来源", block)
		}
	}
	recipes := 0
	for id := core.RecipeStoneBricks; ; id++ {
		if _, ok := core.Recipe(id); !ok {
			break
		}
		recipes++
	}
	if recipes == 0 {
		t.Fatal("配方注册表枚举到 0 条：合成规则没有参与闭包计算，本守卫会静默失效")
	}
	closure := reachabilityClosure()
	if len(closure) == 0 {
		t.Fatal("价值闭包为空：起点或规则没有接进计算，本守卫会静默失效")
	}

	// 已接受集合的每一项都必须真的在闭包外：声明过期（物品已获得自然来源）时
	// 必须报红，否则这份清单会慢慢变成一张不作数的豁免表。
	for _, item := range acceptedUnobtainableItems {
		if closure[item] {
			t.Errorf("已接受不可获得清单已过期：%s 现在位于闭包内，必须把它从清单里删掉",
				itemLabel(item))
		}
	}

	// 正向断言：四条关键能力与熔炉/熔炼链必须在闭包内。熔炉、铁锭与空桶是既有
	// 已交付内容（熔炼产物、铁制工具与取水能力都挂在它们后面），它们的回归与
	// 关键能力同等致命，因此与四条能力一起钉住。
	for _, ability := range []struct {
		name  string
		items []core.ItemID
	}{
		{"工作台", []core.ItemID{core.ItemWorkbench}},
		{"照明（火把）", []core.ItemID{core.ItemTorch}},
		{"采掘工具（石镐或铁镐）", []core.ItemID{core.ItemStonePickaxe, core.ItemIronPickaxe}},
		{"食物（面包或熟牛肉）", []core.ItemID{core.ItemBread, core.ItemCookedBeef}},
		// 耕作工具是食物路径的真实前置（见 `matureCropHarvests`）：没有锄头就翻不出
		// 耕地，小麦只能停留在「有种子」这一步。石锄与铁锄逐项断言而不是任取其一，
		// 石锄缺席意味着耕种要等到熔炉与铁锭之后才成立，「空背包起点」的保证随之作废。
		{"耕作工具（石锄）", []core.ItemID{core.ItemStoneHoe}},
		{"耕作工具（铁锄）", []core.ItemID{core.ItemIronHoe}},
		{"熔炉", []core.ItemID{core.ItemFurnace}},
		{"铁锭", []core.ItemID{core.ItemIronIngot}},
		{"空桶", []core.ItemID{core.ItemEmptyBucket}},
	} {
		if !anyReachable(closure, ability.items) {
			t.Errorf("关键能力「%s」不在价值闭包内：空背包新世界里这条路径走不通", ability.name)
		}
	}

	// 反向断言：注册表中不在闭包内的物品恰好等于已接受集合。
	declared := make(map[core.ItemID]bool, len(acceptedUnobtainableItems))
	for _, item := range acceptedUnobtainableItems {
		declared[item] = true
	}
	var unobtainable, staleDeclaration []core.ItemID
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if !core.RegisteredItem(item) {
			t.Fatalf("物品 %d 位于枚举区间内却没有注册：物品注册表出现空洞，闭包扫描会漏项", item)
		}
		switch {
		case !closure[item] && !declared[item]:
			unobtainable = append(unobtainable, item)
		case closure[item] && declared[item]:
			staleDeclaration = append(staleDeclaration, item)
		}
	}
	sort.Slice(unobtainable, func(i, j int) bool { return unobtainable[i] < unobtainable[j] })
	sort.Slice(staleDeclaration, func(i, j int) bool { return staleDeclaration[i] < staleDeclaration[j] })
	if len(unobtainable) != 0 {
		t.Errorf("发现 %d 个不可获得却未声明接受的物品：%s\n"+
			"它们没有任何自然来源，需要新增来源或经人显式裁决后写入已接受清单——不得直接放行",
			len(unobtainable), itemLabels(unobtainable))
	}
	if len(staleDeclaration) != 0 {
		t.Errorf("已接受清单里有 %d 个物品其实已经可以获得：%s\n过期声明必须删除，否则这份清单会掩盖真实来源",
			len(staleDeclaration), itemLabels(staleDeclaration))
	}
	t.Logf("价值闭包大小 %d（注册表 %d 项），已接受不可获得 %d 项",
		len(closure), int(core.ItemIDMax)-1, len(acceptedUnobtainableItems))
}

// reachabilityClosure 从自然来源方块出发，对「采掘自然方块、成熟作物收获、
// 生物掉落、取水命令、工具损坏映射、合成、熔炼」七类规则反复求值直到不动点，
// 返回缺省世界里可达的全部物品。
//
// 单调性使不动点必然存在且与规则求值顺序无关：每轮只会把新物品加入集合，从不
// 移除，而物品总数固定为注册表大小，因此循环最多执行「注册表大小」轮。这也是
// 「任意顺序都得到同一闭包」的成立条件——规则表将来追加时不必考虑顺序。
func reachabilityClosure() map[core.ItemID]bool {
	reachable := make(map[core.ItemID]bool, int(core.ItemIDMax))
	// harvestable 报告「闭包内是否已存在某件手持物（或空手）能采收该方块」：
	// 手持物取自闭包本身，工具门槛因此是闭包的一部分而不是外部输入——煤炭与
	// 铁矿石只有在镐进入闭包之后才成为候选，这正是工具门槛的语义。
	harvestable := func(block core.BlockID) bool {
		for held := core.ItemNone; held < core.ItemIDMax; held++ {
			if held != core.ItemNone && !reachable[held] {
				continue
			}
			if required, drops := miningRule(block, held); required != 0 && drops {
				return true
			}
		}
		return false
	}
	add := func(item core.ItemID) bool {
		if item == core.ItemNone || reachable[item] {
			return false
		}
		reachable[item] = true
		return true
	}
	for changed := true; changed; {
		changed = false
		// 规则一：采掘自然来源方块。手持物准入与可收获性都由真实 `miningRule`
		// 判定，掉落由真实 `core.BlockDrop` 给出；两条位置稳定的概率掉落（短草掉
		// 种子、树叶额外掉树苗，见 `completeMining` 的两个专用分支）没有登记在
		// `BlockDrop` 里，按方块编号补在这里——只要存在一条命中路径即算可达。
		for _, block := range naturalSourceBlocks {
			if !harvestable(block) {
				continue
			}
			if drop, ok := core.BlockDrop(block); ok {
				changed = add(drop) || changed
			}
			if block == core.LeavesID {
				changed = add(core.ItemSapling) || changed
			}
			if block == core.ShortGrassID {
				changed = add(core.ItemWheatSeeds) || changed
			}
		}
		// 规则二：成熟作物收获（见 `matureCropHarvests` 的溯源）。
		for _, crop := range matureCropHarvests {
			if !reachable[crop.seed] {
				continue
			}
			for _, drop := range crop.drops {
				changed = add(drop) || changed
			}
		}
		// 规则三：生物掉落。牛与夜行者都在缺省世界里自然生成，死亡时分别掉 1
		// 生牛肉（`passive.go` 的 `dropPassiveLoot`）与 1 腐肉（`hostile.go` 的
		// `dropHostileLoot`），与闭包内容无关，因此无条件计入。
		changed = add(core.ItemRawBeef) || changed
		changed = add(core.ItemRottenFlesh) || changed
		// 规则四：取水命令（`bucket.go` 的 `ApplyBucketCollect`，见 spec
		// authoritative-fluid）：手持空桶命中水源源格，原格换成水桶。水桶不是
		// 任何配方或熔炼的产物，这条命令是它唯一的来源。
		if reachable[core.ItemEmptyBucket] {
			changed = add(core.ItemWaterBucket) || changed
		}
		// 规则五：工具损坏映射（`core.ItemBrokenForm`）。完好工具在闭包内即意味着
		// 损坏形态在闭包内：镐与锄在采掘与翻地的每次成功动作磨损 1 点
		// （`mining.go` 的 `consumeToolDurabilityAt`、`farming.go` 的翻地），剑在
		// 命中生物时磨损 1 点——战斗同样走 `consumeToolDurabilityAt`，绕过
		// `consumeToolDurability` 里的完好剑豁免（`combat.go`）。四类零磨损豁免
		// ——作物×完好锄头、短草、树苗（`consumeMiningToolDurability` 判定）与
		// 完好剑（`consumeToolDurability` 判定）——只覆盖上述特定动作，不覆盖
		// 「用镐挖石头」「用剑砍怪」这类普通动作，因此损坏形态都必然可达。
		for item := core.ItemID(1); item < core.ItemIDMax; item++ {
			if !reachable[item] {
				continue
			}
			if broken, ok := core.ItemBrokenForm(item); ok {
				changed = add(broken) || changed
			}
		}
		// 规则六：合成。枚举方式与本仓既有配方守卫同形：从 `core.RecipeStoneBricks`
		// 起逐号推进直到注册表返回 false。形状的全部非空格都在闭包内即可合成；
		// 宽或高超过 2 的形状还要求工作台在闭包内——个人网格只有 2×2，3×3 与
		// 1×3 这类形状必须先放置工作台（见 spec authoritative-grid-crafting）。
		for id := core.RecipeStoneBricks; ; id++ {
			pattern, ok := core.Recipe(id)
			if !ok {
				break
			}
			if (pattern.Width > 2 || pattern.Height > 2) && !reachable[core.ItemWorkbench] {
				continue
			}
			craftable := true
			for _, cell := range pattern.Cells {
				if cell == core.ItemNone {
					continue
				}
				if !reachable[cell] {
					craftable = false
					break
				}
			}
			if craftable {
				changed = add(pattern.Output.Item) || changed
			}
		}
		// 规则七：熔炼（`core.SmeltingOutput`）。熔炼需要一座熔炉与煤炭燃料
		// （`furnace.go` 的燃料表只接受 `ItemCoal`）：熔炉方块不参与世界生成，
		// 只能由 `RecipeFurnace` 合成，两者都必须在闭包内。
		if reachable[core.ItemFurnace] && reachable[core.ItemCoal] {
			for input := core.ItemID(1); input < core.ItemIDMax; input++ {
				if !reachable[input] {
					continue
				}
				if output, ok := core.SmeltingOutput(input); ok {
					changed = add(output) || changed
				}
			}
		}
	}
	return reachable
}

// anyReachable 报告候选物品里至少有一个位于闭包内。
func anyReachable(closure map[core.ItemID]bool, items []core.ItemID) bool {
	for _, item := range items {
		if closure[item] {
			return true
		}
	}
	return false
}

// itemLabel 返回诊断用的稳定物品标签：显示名与编号一起给出，编号是跨语言契约值，
// 显示名便于人工核对清单。
func itemLabel(item core.ItemID) string {
	name, ok := core.ItemDisplayName(item)
	if !ok {
		name = "<未命名>"
	}
	return fmt.Sprintf("%s(ItemID=%d)", name, item)
}

// itemLabels 把一组物品编号渲染成稳定序的标签串，供断言一次报出全部差异项。
func itemLabels(items []core.ItemID) string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, itemLabel(item))
	}
	return strings.Join(labels, ", ")
}
