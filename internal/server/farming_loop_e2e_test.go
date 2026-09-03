package server

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/sim/runtime"
	"github.com/channing771/mornlea/internal/sim/tuning"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/internal/world"
	"github.com/channing771/mornlea/internal/worldgen"
)

// 自然种子闭环的冻结样本。这些常量是**事先离线核实后冻结**的固定事实，运行时
// 不得搜索、不得改写：
//
//   - 世界 seed 42、Overworld、流体开启（生产 `config.Defaults().FluidEnabled`
//     的编译期默认值；本包不依赖 internal/config，按该默认值显式开流体）；
//   - 出生锚点 {2,14} 使出生列恰为 (32,224)：该列地形高度 64，是海平面草架，
//     表面为 GrassID、正上方 (32,65,224) 由 Rust worldgen 自然生成 ShortGrassID，
//     其上直到世界顶部再无任何方块（没有树），玩家原地出生即站在短草格内；
//   - 同格的 1/8 除草掉落判定命中（`runtime.ShortGrassSeedDropRoll`）；
//   - (32..36,64,228) 五格海平面水源位于耕地 (32,64,224) 的 9×9 湿润窗口内
//     （水平距离 4、同层），翻地后不需要任何夹具水源即可被自然海水润湿。
//
// 换任何常量都必须先离线重新核实全部四条再冻结。
const (
	naturalFarmingSeed int64 = 42
	// naturalFarmingGrassY 是自然短草格的 Y：海平面草架表面正上方。
	naturalFarmingGrassY int32 = 65
)

var (
	naturalFarmingAnchor   = core.ChunkPos{X: 2, Z: 14}
	naturalFarmingGrass    = core.BlockPos{X: 32, Y: naturalFarmingGrassY, Z: 224}
	naturalFarmingFarmland = core.BlockPos{X: 32, Y: naturalFarmingGrassY - 1, Z: 224}
)

const (
	// naturalFarmingPickupDelaySteps 是采掘掉落从产生到入包的活动 tick 数。
	// `DropPickupDelayTicks` 默认 10；掉落产生于完成 tick 的采掘阶段，而掉落
	// 推进在同一 tick 更早的命令阶段已经跑过，因此完成 tick 之后恰好第 10
	// 步才拾取。这个数字钉住「前 9 个活动 tick 不拾取、第 10 个拾取」。
	naturalFarmingPickupDelaySteps = 10
	// naturalFarmingMoistureBudget 是翻地后等自然海水把干耕地润湿的 tick 预
	// 算。湿度队列下一 tick 就会处理耕地格，预算只防挂起，不是性能断言。
	naturalFarmingMoistureBudget = 30
	// naturalFarmingGrowthBudget 是七次作物阶段推进的 tick 预算（取值理由同
	// 旧平坦闭环：RandomTicksPerSection 置 64 时期望约 448 tick，6000 吸收
	// 哈希分布的抖动）。
	naturalFarmingGrowthBudget = 6000
	// naturalFarmingHarvestPickupBudget 是等收获产物过完拾取延迟并入包的
	// tick 预算，一倍余量。
	naturalFarmingHarvestPickupBudget = 40
	// naturalFarmingSettleTicks 是一条命令之后让方块变更与背包发布收敛的
	// tick 数，与旧平坦闭环一致。
	naturalFarmingSettleTicks = 3
)

// naturalSeedFarmingResult 是自然种子固定脚本（设计决策 6 的 1–4 步）在一种
// 传输上跑完后的全部可观察结果。Memory 与 TCP 各跑一遍后必须逐字段相等。
type naturalSeedFarmingResult struct {
	// SpawnCell 是登录站稳后的出生方块列（X,Z）：必须就是冻结样本列，证明
	// 脚本没有任何移动输入、短草在玩家脚边。
	SpawnCell [2]int32
	// InventoryAfterLogin 是登录即得的完整权威背包：材料包 14 叠、无任何种子。
	InventoryAfterLogin core.Inventory
	// TargetAfterMine 是采除后短草格的镜像读数（必须空气）。
	TargetAfterMine core.BlockID
	// DropItem/DropCount 是采除短草产生的权威世界掉落。
	DropItem  core.ItemID
	DropCount uint8
	// PickupDelaySteps 是完成 tick 之后第几步拾取（规格值 10）。
	PickupDelaySteps int
	// SeedSlot 是拾取到的种子所在的快捷栏格（放置命令需要显式栏位）。
	SeedSlot uint8
	// PlantRejection 是「未翻地就种」的完整拒绝消息。
	PlantRejection network.CommandRejected
	// Farmland 是最终耕地编号（必须是湿耕地：自然海水润湿）。
	Farmland core.BlockID
	// Crop 是短草格上种出的作物（WheatStage0ID）。
	Crop core.BlockID
	// Inventory 是种植后（种子归零）的完整权威背包。
	Inventory core.Inventory
	// HoeDurability 是夹具直给锄头翻地一次后的剩余耐久。
	HoeDurability uint16
}

// naturalSeedFarmingTools 暴露固定脚本结束后的续跑操作：完整 Memory 闭环在
// 「种植、种子归零」之后继续生长、收获、再种，与固定脚本共用同一套真实命令
// 通道与读数闭包。命令以「序号构造器」而非现成消息传入：序号由脚本统一递增，
// 调用方不可能发出过期或重复序号。
type naturalSeedFarmingTools struct {
	// Step 推进一个权威 tick；任何命令拒绝都直接 Fatal。
	Step func()
	// Send 发送一条真实客户端命令并推进一个 tick。
	Send func(build func(sequence uint64) network.ClientMessage)
	// Settle 推进固定脚本的收敛 tick 数。
	Settle func()
	// Inventory 读取最终权威背包。
	Inventory func() core.Inventory
	// MirrorBlock 从客户端镜像读一个方块。
	MirrorBlock func(position core.BlockPos) core.BlockID
	// Grass/Farmland 是冻结样本的两个关键格位。
	Grass    core.BlockPos
	Farmland core.BlockPos
}

// TestFarmingLoopEndToEndMemory 是从**自然取得的第一颗种子**出发的完整农业
// 闭环集成回归：一名从未存在过的玩家在生产 Rust worldgen 生成的自然世界里
// 登录（身上没有任何种子），原地采除脚下自然生成的短草，等权威世界掉落物过
// 完拾取延迟入包，翻地（被自然海水润湿）、种下唯一的一颗种子，靠随机 tick
// 长到成熟、收获、再种。
//
// 与旧平坦闭环相比的刻意设计约束：
//
//  1. **短草必须是自然生成的**。世界由生产 `worldgen.New` 经真实 Rust worldgen
//     生成（流体开启）；脚本在发送任何玩家输入前同时断言单点出口
//     `BaseBlockAt(target) == ShortGrassID`、已加载权威区块同格也是
//     ShortGrassID、以及服务端 1/8 判定命中。没有 `flatGenerator`、没有手工
//     `SetBlock` 布草、也没有测试侧复制的分布算法。
//  2. **种子必须走既有世界掉落物路径**：权威 drop → 前 9 个活动 tick 不拾取
//     → 第 10 个活动 tick 拾取，一步不少、一步不多。
//  3. **成熟必须靠生长跑出来**：只写入 WheatStage0ID，之后把生长概率置 100、
//     抽样率置上限，等随机 tick 自己走到 WheatStage7ID。
//  4. **收获钉在哈希产量的完整区间上**：小麦与种子各落闭区间 [1,3]，单种子
//     起步意味着收获后种子 ∈ [1,3]，再种扣一颗后 ∈ [0,2]；下界 1 正是
//     「一轮种收不亏种子」的契约——掉 0 颗会让闭环因随机性断种。
//
// 锄头由夹具在拾取之后直给而不是现场合成：本脚本验证的是自然种子闭环本身，
// 石锄的网格合成链（真实命令、两种传输）由 `TestPlantSeedsMemoryTCPParity`
// 与 `TestMemoryTCPCraftingGridConvergence` 覆盖；在自然世界里徒手挖穿土层取
// 石只会把数千 tick 的挖掘编排混进农业断言。耕地润湿刻意**不用**夹具水源：
// 冻结样本的 9×9 窗口里有自然海平面水，「翻地 → 湿耕地」这步由生产湿度系统
// 自己完成。
func TestFarmingLoopEndToEndMemory(t *testing.T) {
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })

	var growthTicks int
	runNaturalSeedFarmingScript(t, "memory", func(tools naturalSeedFarmingTools) {
		// —— 第 5 步：靠随机 tick 生长到成熟 ——
		//
		// 只调概率与抽样率两个 tunable，生长规则本身一行没动：脚本从不写入
		// WheatStage1..7 中的任何一个，成熟完全是 advanceCrops 跑出来的。
		tunables := tuning.DefaultTunables()
		tunables.RandomTicksPerSection = 64
		tunables.CropGrowthChancePercent = 100
		tuning.SetTunables(tunables)
		for ; tools.MirrorBlock(tools.Grass) != core.WheatStage7ID; growthTicks++ {
			if growthTicks > naturalFarmingGrowthBudget {
				t.Fatalf("推进 %d 个 tick 后作物仍是 %d，想要成熟 %d（耕地 = %d）",
					growthTicks, tools.MirrorBlock(tools.Grass), core.WheatStage7ID,
					tools.MirrorBlock(tools.Farmland))
			}
			tools.Step()
		}
		if growthTicks == 0 {
			t.Fatal("作物在零个 tick 内就成熟了：生长根本没被推进")
		}
		t.Logf("作物由随机 tick 从 stage0 长到成熟用了 %d 个权威 tick", growthTicks)
		if got := tools.MirrorBlock(tools.Farmland); got != core.FarmlandWetID {
			t.Fatalf("成熟时耕地 = %d，想要湿耕地 %d（生长的前置条件）",
				got, core.FarmlandWetID)
		}

		// —— 第 6 步：收获，哈希产量的两类产物经既有拾取路径入包 ——
		//
		// 成熟小麦掉多少由 `sim.cropYieldRolls` 对 (世界种子, 完成采掘的权威
		// tick, 维度, 坐标) 的哈希决定，小麦与种子各落在闭区间 [1,3]；两类
		// 产物在同一完成 tick 入掉落区、共享同一拾取延迟。
		tools.Send(func(sequence uint64) network.ClientMessage {
			return network.PlayerInput{Sequence: sequence, Pitch: tillSoilLookDown, Mining: true}
		})
		tools.Send(func(sequence uint64) network.ClientMessage {
			return network.PlayerInput{Sequence: sequence, Pitch: tillSoilLookDown}
		})
		if got := tools.MirrorBlock(tools.Grass); got != core.AirID {
			t.Fatalf("收获后作物格 = %d，想要空气（成熟作物 1 tick 破坏）", got)
		}
		harvestReady := func() bool {
			harvested := tools.Inventory()
			return countItem(harvested, core.ItemWheat) >= 1 &&
				countItem(harvested, core.ItemWheatSeeds) >= 1
		}
		for ticks := 0; !harvestReady(); ticks++ {
			if ticks > naturalFarmingHarvestPickupBudget {
				t.Fatalf("等了 %d 个 tick 收获产物仍未入包，当前背包 = %+v",
					ticks, tools.Inventory())
			}
			tools.Step()
		}
		harvested := tools.Inventory()
		wheat := countItem(harvested, core.ItemWheat)
		seedsAfterHarvest := countItem(harvested, core.ItemWheatSeeds)
		if wheat < 1 || wheat > 3 {
			t.Fatalf("收获后小麦 = %d，想要落在闭区间 [1,3]", wheat)
		}
		// 单种子起步：种下前种子恰为 0，收获后全部种子都来自掉落，[1,3] 的
		// 下界 1 就是「一轮种收不亏种子」——掉 0 颗会让闭环断种。
		if seedsAfterHarvest < 1 || seedsAfterHarvest > 3 {
			t.Fatalf("收获后种子 = %d，想要落在闭区间 [1,3]", seedsAfterHarvest)
		}
		if got := tools.MirrorBlock(tools.Farmland); !core.IsFarmland(got) {
			t.Fatalf("收获后耕地格 = %d，想要仍是耕地", got)
		}

		// —— 第 7 步：再种一颗，闭环继续 ——
		//
		// 种子格位由收获后的真实背包动态解析：产物入包的格位取决于拾取时的
		// 空位分布，写死格位会让脚本对 AddStack 的落点敏感。
		seedSlot, ok := naturalSeedFarmingSeedSlot(harvested)
		if !ok {
			t.Fatalf("收获后快捷栏里找不到种子: %+v", harvested.Hotbar)
		}
		tools.Send(func(sequence uint64) network.ClientMessage {
			return network.PlaceBlock{Sequence: sequence, Pitch: tillSoilLookDown, Slot: seedSlot}
		})
		tools.Settle()
		if got := tools.MirrorBlock(tools.Grass); got != core.WheatStage0ID {
			t.Fatalf("再种后作物格 = %d，想要第一阶段 %d", got, core.WheatStage0ID)
		}
		seedsAfterReplant := countItem(tools.Inventory(), core.ItemWheatSeeds)
		if seedsAfterReplant != seedsAfterHarvest-1 {
			t.Fatalf("再种后种子 = %d，想要 %d（收获后存量减去种下的一颗）",
				seedsAfterReplant, seedsAfterHarvest-1)
		}
	})
}

// naturalSeedFarmingSeedSlot 在快捷栏里找到小麦种子所在的格。放置命令只接受
// 快捷栏栏位；本脚本从空快捷栏起步，拾取/收获入包的种子必然先落快捷栏。
func naturalSeedFarmingSeedSlot(inventory core.Inventory) (uint8, bool) {
	for slot, stack := range inventory.Hotbar.Slots {
		if stack.Item == core.ItemWheatSeeds {
			return uint8(slot), true
		}
	}
	return 0, false
}

// naturalFarmingViewLoaded 报告冻结样本周围的 3×3 视野是否都已进入镜像：
// 玩家出生在锚点区块，ViewRadius=1 的视野即锚点周围的九个区块（通用的
// `parityViewLoaded` 钉住原点区块，不适合出生在别处的自然世界）。
func naturalFarmingViewLoaded(mirror *client.Mirror) bool {
	for x := naturalFarmingAnchor.X - 1; x <= naturalFarmingAnchor.X+1; x++ {
		for z := naturalFarmingAnchor.Z - 1; z <= naturalFarmingAnchor.Z+1; z++ {
			if _, ok := mirror.Chunk(core.Overworld, core.ChunkPos{X: x, Z: z}); !ok {
				return false
			}
		}
	}
	return true
}

// runNaturalSeedFarmingScript 在一种传输上跑完整段自然种子固定脚本（设计决策
// 6 的 1–4 步）：零种子登录 → 原地 1 tick 采除自然短草 → 权威世界掉落 → 前
// 9 个活动 tick 不拾取、第 10 个拾取 → 未翻地先种的拒绝 → 翻地（自然海水
// 润湿）→ 种植、种子归零。continueFullLoop 非空时在种植完成后用同一套通道
// 续跑（完整 Memory 闭环的生长、收获、再种）。
func runNaturalSeedFarmingScript(
	t *testing.T,
	transport string,
	continueFullLoop func(naturalSeedFarmingTools),
) naturalSeedFarmingResult {
	t.Helper()
	identity := integrationIdentity(0x9f, "NaturalFarmer")

	// —— 玩家输入前的三重预断言 ——
	//
	// 单点出口（真实 Rust worldgen 的 BaseBlockAt）、样本前提（海平面草架）
	// 与服务端权威 1/8 判定。夹具另在登录就绪后对已加载权威区块重读同一格。
	// 任何一条失败都说明冻结样本失效，脚本不降级、不搜索。
	probe := worldgen.New(naturalFarmingSeed, true)
	if got := probe.HeightAt(naturalFarmingGrass.X, naturalFarmingGrass.Z); got != naturalFarmingFarmland.Y {
		t.Fatalf("冻结样本前提失效：出生列高度 = %d，想要海平面草架 %d",
			got, naturalFarmingFarmland.Y)
	}
	if got := probe.BaseBlockAt(naturalFarmingFarmland); got != core.GrassID {
		t.Fatalf("冻结样本前提失效：样本格下方 = %d，想要 %d", got, core.GrassID)
	}
	if got := probe.BaseBlockAt(naturalFarmingGrass); got != core.ShortGrassID {
		t.Fatalf("冻结样本前提失效：样本格 = %d，想要自然生成的 %d",
			got, core.ShortGrassID)
	}
	if !runtime.ShortGrassSeedDropRoll(
		naturalFarmingSeed, core.Overworld, naturalFarmingGrass,
	) {
		t.Fatal("冻结样本前提失效：样本格的 1/8 掉落判定未命中")
	}

	// 刻意不预存玩家：只有 LoadPlayer 返回 ErrPlayerNotFound 的路径才会构造
	// 一次性材料包，脚本第一步要看的正是这份材料包不再携带种子。生成器用
	// 生产 worldgen.New（流体开启 = 生产默认配置），不经任何测试生成旁路。
	store := storage.NewMemory(storage.Metadata{
		FormatVersion:  3,
		Seed:           naturalFarmingSeed,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    naturalFarmingAnchor,
	})
	config := hostTestConfig()
	config.ViewRadius = 1
	config.AutosaveTicks = 1 << 20
	host := mustNewHost(t, config, worldgen.New(naturalFarmingSeed, true), store)
	endpoint, _, closeTransport := openParityTransport(t, host, transport, identity)
	t.Cleanup(func() {
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	})

	mirror := client.NewMirror()
	authoritativeSnapshot := func() contract.PlayerSnapshot {
		t.Helper()
		host.mu.Lock()
		active := host.activeByPlayer[identity.PlayerID]
		host.mu.Unlock()
		if active == nil {
			t.Fatal("玩家还没有 active 会话")
		}
		snapshot, ok := host.world.PlayerSnapshotFor(active.Session)
		if !ok {
			t.Fatal("没有权威玩家快照")
		}
		return snapshot
	}

	// —— 第 1 步：零种子登录，站稳在冻结样本列 ——
	ready, inventoryReady := false, false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s natural seed farming", transport),
		func() bool { return ready && inventoryReady && naturalFarmingViewLoaded(mirror) },
		func() string {
			return fmt.Sprintf("ready=%v 背包已发布=%v 视野已加载=%v",
				ready, inventoryReady, naturalFarmingViewLoaded(mirror))
		},
		func() {
			_, messages := parityStep(t, host, endpoint, mirror)
			for _, message := range messages {
				switch message := message.(type) {
				case network.PlayerState:
					ready = ready || message.Ready
				case network.InventoryState:
					inventoryReady = inventoryReady || message.Inventory != core.Inventory{}
				}
			}
		},
	)
	login := authoritativeSnapshot()
	result := naturalSeedFarmingResult{InventoryAfterLogin: login.Inventory}
	// 零种子断言扫全部 36 格：把种子挪到任何栏位、任何数量都会红。
	forEachStack(login.Inventory, func(slot int, stack core.ItemStack) {
		if stack.Item == core.ItemWheatSeeds {
			t.Fatalf("登录时统一索引 %d 已持有小麦种子 %+v，材料包不该再发种子",
				slot, stack)
		}
	})
	// 材料包本身保持 14 叠：只看种子的断言抓不住「顺手把材料也删了」的回归。
	if got := login.Inventory.Backpack[13]; got.Item != core.ItemMossyCobblestone ||
		got.Count != core.MaxStackCount {
		t.Fatalf("登录后材料包第 14 格 = %+v，想要 64 个苔石（材料清单不变）", got)
	}
	if got := login.Inventory.Backpack[14]; got != (core.ItemStack{}) {
		t.Fatalf("登录后材料包第 15 格 = %+v，想要空（种子格被取消后不得顶替）", got)
	}
	// 免移动证据：出生列就是样本列。X/Z 用方块坐标而不是浮点全等，Y 只要求
	// 落在海平面草架表面那一层（物理落定的微扰不参与语义）。
	result.SpawnCell = [2]int32{
		int32(math.Floor(float64(login.Current.Position.X()))),
		int32(math.Floor(float64(login.Current.Position.Z()))),
	}
	if result.SpawnCell != [2]int32{naturalFarmingGrass.X, naturalFarmingGrass.Z} {
		t.Fatalf("出生列 = %v，想要冻结样本列 %v（脚本不允许移动）",
			result.SpawnCell, [2]int32{naturalFarmingGrass.X, naturalFarmingGrass.Z})
	}
	if y := login.Current.Position.Y(); y < float32(naturalFarmingGrass.Y) ||
		y >= float32(naturalFarmingGrass.Y+1) {
		t.Fatalf("出生高度 Y = %v，想要站在海平面草架上（%d 层）",
			y, naturalFarmingGrass.Y)
	}
	// 已加载权威区块的同格复核：区块内容必须与单点出口一致。
	chunk, _, ok := host.world.CloneReadyChunkForTest(core.ChunkKey{
		Dimension: core.Overworld, Pos: naturalFarmingGrass.Chunk(),
	})
	if !ok {
		t.Fatal("样本区块没有进入权威 realm")
	}
	localX, _, localZ := naturalFarmingGrass.Local()
	if got := chunk.BlockAt(localX, naturalFarmingGrass.Y, localZ); got != core.ShortGrassID {
		t.Fatalf("权威区块样本格 = %d，想要 %d（加载路径与生成语义分叉）",
			got, core.ShortGrassID)
	}

	// 命令通道。序号由脚本统一递增，命令以构造器传入；任何脚本外的拒绝都直接
	// Fatal（唯一的期望拒绝在「未翻地就种」那一步显式接住）。
	sequence := uint64(0)
	next := func() uint64 { sequence++; return sequence }
	step := func() {
		t.Helper()
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			if rejected, ok := message.(network.CommandRejected); ok {
				t.Fatalf("自然种子脚本的命令被拒绝: %+v", rejected)
			}
		}
	}
	send := func(build func(sequence uint64) network.ClientMessage) {
		t.Helper()
		sendIntegration(t, endpoint, build(next()))
		waitIntegrationCondition(
			t, fmt.Sprintf("%s natural farming command queued", transport),
			func() bool { return len(host.world.incoming) > 0 },
		)
		step()
	}
	settle := func() {
		t.Helper()
		for range naturalFarmingSettleTicks {
			step()
		}
	}
	mirrorBlock := func(position core.BlockPos) core.BlockID {
		t.Helper()
		block, loaded := mirror.BlockAt(core.Overworld, position)
		if !loaded {
			t.Fatalf("%+v 没有进入客户端镜像", position)
		}
		return block
	}

	// —— 第 2 步：真实持续 primary 输入，1 tick 采除短草 ——
	//
	// 玩家站在短草格内，向下俯视的权威射线第一格就是脚边短草。输入只发一次，
	// 完成后立即松键。完成 tick 必须同时给出「方块变空气」与「恰好一颗种子
	// 的权威世界掉落」——直接入包或无掉落都会在这里红。
	blockIndex, indexed := world.ChunkBlockIndex(naturalFarmingGrass)
	if !indexed {
		t.Fatal("样本格没有区块索引")
	}
	sendIntegration(t, endpoint, network.PlayerInput{
		Sequence: next(), Pitch: tillSoilLookDown, Mining: true,
	})
	waitIntegrationCondition(t, "natural farming mining queued", func() bool {
		return len(host.world.incoming) > 0
	})
	_, completionMessages := parityStep(t, host, endpoint, mirror)
	sawBlockChange, sawDropUpsert := false, false
	for _, message := range completionMessages {
		switch message := message.(type) {
		case network.BlockChanges:
			if message.Dimension != core.Overworld ||
				message.Chunk != naturalFarmingGrass.Chunk() ||
				len(message.Changes) != 1 ||
				message.Changes[0] != (network.BlockChange{
					Position: naturalFarmingGrass, Block: core.AirID,
				}) {
				t.Fatalf("采草完成 tick 的方块变更 = %+v，想要恰好样本格变空气", message)
			}
			sawBlockChange = true
		case network.ItemDropUpserts:
			if len(message.Drops) != 1 ||
				message.Drops[0].Item != core.ItemWheatSeeds ||
				message.Drops[0].Count != 1 ||
				message.Drops[0].BlockIndex != blockIndex {
				t.Fatalf("采草完成 tick 的掉落 = %+v，想要恰好一颗小麦种子", message)
			}
			result.DropItem, result.DropCount = message.Drops[0].Item, message.Drops[0].Count
			sawDropUpsert = true
		}
	}
	if !sawBlockChange || !sawDropUpsert {
		t.Fatal("采草完成 tick 缺少方块变更或掉落发布：短草不是 1 tick 权威完成的")
	}
	// 松键命令只入队、不推进：拾取延迟按「完成 tick 之后的活动 tick」逐一步
	// 计数，这里若多推进一步，下面的第 10 步就会错位成第 9 步。命令在下一步
	// 开头被消费，采掘意图不会跨到掉落推进之后。
	sendIntegration(t, endpoint, network.PlayerInput{
		Sequence: next(), Pitch: tillSoilLookDown,
	})
	waitIntegrationCondition(t, "natural farming release queued", func() bool {
		return len(host.world.incoming) > 0
	})

	// —— 第 3 步：前 9 个活动 tick 不拾取，第 10 个才拾取 ——
	//
	// 每一步都读权威背包：任何一步提前入包或第 10 步仍未入包都会红。第一步
	// 同时消费上面的松键命令。
	for tick := 1; tick <= naturalFarmingPickupDelaySteps; tick++ {
		step()
		seeds := countItem(authoritativeSnapshot().Inventory, core.ItemWheatSeeds)
		if tick < naturalFarmingPickupDelaySteps {
			if seeds != 0 {
				t.Fatalf("拾取延迟第 %d 步背包已有种子 %d 颗，前 %d 步必须不入包",
					tick, seeds, naturalFarmingPickupDelaySteps-1)
			}
			continue
		}
		if seeds != 1 {
			t.Fatalf("拾取延迟第 %d 步背包种子 = %d 颗，想要恰好 1", tick, seeds)
		}
		result.PickupDelaySteps = tick
	}
	result.TargetAfterMine = mirrorBlock(naturalFarmingGrass)
	if result.TargetAfterMine != core.AirID {
		t.Fatalf("采除后样本格 = %d，想要空气", result.TargetAfterMine)
	}
	afterPickup := authoritativeSnapshot().Inventory
	seedSlot, ok := naturalSeedFarmingSeedSlot(afterPickup)
	if !ok {
		t.Fatalf("拾取后快捷栏里找不到种子: %+v", afterPickup.Hotbar)
	}
	result.SeedSlot = seedSlot

	// —— 第 4a 步：未翻地就种，必须被拒且一颗种子都不掉 ——
	sendIntegration(t, endpoint, network.PlaceBlock{
		Sequence: next(), Pitch: tillSoilLookDown, Slot: seedSlot,
	})
	waitIntegrationCondition(t, "natural farming plant-before-till queued", func() bool {
		return len(host.world.incoming) > 0
	})
	_, rejectMessages := parityStep(t, host, endpoint, mirror)
	sawRejection := false
	for _, message := range rejectMessages {
		if rejected, ok := message.(network.CommandRejected); ok {
			result.PlantRejection = rejected
			sawRejection = true
		}
	}
	if !sawRejection {
		t.Fatal("未翻地就种没有被拒绝：脚下还是草方块")
	}
	if result.PlantRejection.Reason != network.RejectInvalidBlock {
		t.Fatalf("未翻地就种的拒绝理由 = %v，想要 %v",
			result.PlantRejection.Reason, network.RejectInvalidBlock)
	}
	if seeds := countItem(authoritativeSnapshot().Inventory, core.ItemWheatSeeds); seeds != 1 {
		t.Fatalf("被拒的种植扣掉了种子，剩余 %d 颗，想要 1", seeds)
	}

	// —— 第 4b 步：夹具直给石锄，翻地，自然海水把耕地润湿 ——
	//
	// 锄头合成链由显式种子集成与网格 parity 脚本覆盖（见测试头注释）；这里
	// 在拾取之后直给一把满耐久石锄，专注自然种子闭环本身。
	hoeFull, _ := core.ItemMaxDurability(core.ItemStoneHoe)
	host.mu.Lock()
	active := host.activeByPlayer[identity.PlayerID]
	host.mu.Unlock()
	if active == nil {
		t.Fatal("玩家没有 active 会话")
	}
	if afterPickup.Hotbar.Slots[1] != (core.ItemStack{}) {
		t.Fatalf("夹具想在快捷栏 1 放锄头，但该格已有 %+v", afterPickup.Hotbar.Slots[1])
	}
	host.world.SetPlayerInventoryForTest(active.Session, func(inventory core.Inventory) core.Inventory {
		inventory.Hotbar.Slots[1] = core.ItemStack{
			Item: core.ItemStoneHoe, Count: 1, Durability: hoeFull,
		}
		return inventory
	})
	send(func(sequence uint64) network.ClientMessage {
		return network.SelectHotbar{Sequence: sequence, Slot: 1}
	})
	settle()
	send(func(sequence uint64) network.ClientMessage {
		return network.TillSoil{Sequence: sequence, Pitch: tillSoilLookDown}
	})
	// 翻地先落干耕地，随后生产湿度系统按 9×9 窗口内的自然海水把它润湿；等
	// 待的就是「没有夹具水源」这半句证据。
	for ticks := 0; ; ticks++ {
		got := mirrorBlock(naturalFarmingFarmland)
		if got == core.FarmlandWetID {
			result.Farmland = got
			break
		}
		if got != core.FarmlandDryID || ticks > naturalFarmingMoistureBudget {
			t.Fatalf("翻地后 %d tick 耕地 = %d，想要自然润湿的 %d",
				ticks, got, core.FarmlandWetID)
		}
		step()
	}
	wantWornHoe := core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: hoeFull - 1}
	if got := authoritativeSnapshot().Inventory.Hotbar.Slots[1]; got != wantWornHoe {
		t.Fatalf("翻地后锄头 = %+v，想要恰好扣一点的 %+v", got, wantWornHoe)
	}
	result.HoeDurability = wantWornHoe.Durability

	// —— 第 4c 步：种下唯一的一颗种子，背包种子归零 ——
	send(func(sequence uint64) network.ClientMessage {
		return network.PlaceBlock{Sequence: sequence, Pitch: tillSoilLookDown, Slot: seedSlot}
	})
	settle()
	result.Crop = mirrorBlock(naturalFarmingGrass)
	if result.Crop != core.WheatStage0ID {
		t.Fatalf("种下后样本格 = %d，想要第一阶段 %d", result.Crop, core.WheatStage0ID)
	}
	result.Inventory = authoritativeSnapshot().Inventory
	if seeds := countItem(result.Inventory, core.ItemWheatSeeds); seeds != 0 {
		t.Fatalf("种下唯一的种子后背包仍有种子 %d 颗，想要归零", seeds)
	}

	if continueFullLoop != nil {
		continueFullLoop(naturalSeedFarmingTools{
			Step:        step,
			Send:        send,
			Settle:      settle,
			Inventory:   func() core.Inventory { return authoritativeSnapshot().Inventory },
			MirrorBlock: mirrorBlock,
			Grass:       naturalFarmingGrass,
			Farmland:    naturalFarmingFarmland,
		})
	}
	return result
}

// forEachStack 按统一索引 0..35 遍历完整物品状态的每一格。
func forEachStack(inventory core.Inventory, visit func(slot int, stack core.ItemStack)) {
	for slot, stack := range inventory.Hotbar.Slots {
		visit(slot, stack)
	}
	for slot, stack := range inventory.Backpack {
		visit(core.HotbarSlots+slot, stack)
	}
}

// countItem 统计完整物品状态里某种物品的总数量。
func countItem(inventory core.Inventory, item core.ItemID) int {
	total := 0
	forEachStack(inventory, func(_ int, stack core.ItemStack) {
		if stack.Item == item {
			total += int(stack.Count)
		}
	})
	return total
}
