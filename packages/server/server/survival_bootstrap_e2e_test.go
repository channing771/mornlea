package server

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/sim/runtime"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// 空背包起家冒烟的冻结样本。全部常量都在缺省种子 42、流体开启的生产 worldgen 上
// 离线核实后冻结，运行时不得搜索、不得改写方块：
//
//   - 出生锚点区块 {19,30} 的原点列 (304,480) 恰是一棵普通橡树的**树干列**：
//     树冠顶叶格 71、其下 70 是叶、69..65 是五格树干、64 草、63..61 三格泥土、
//     60 起是石料。出生搜索在列内自顶向下取第一个「有碰撞体、玩家包围盒自由、
//     支撑完整」的落点，因此新玩家出生在**树冠顶**（脚层 72）；向下笔直采掘即可
//     依次拿到树叶、五格原木、泥土与石料。
//   - 正交邻居 (305,480)（+X 方向）的 65/66 是空气、64 草、63..61 泥土、60 石料：
//     它给出工作台唯一需要的「可放置空气格 + 可被权威射线命中的支撑面」。
//
// 换任何常量都必须先离线重新核实这两条。
const survivalBootstrapSeed int64 = 42

var survivalBootstrapAnchor = core.ChunkPos{X: 19, Z: 30}

// survivalBootstrapColumn 是出生列（锚点区块原点列），survivalBootstrapNeighbor
// 是工作台放置方向的正交邻居列。
var (
	survivalBootstrapColumn   = core.BlockPos{X: 304, Z: 480}
	survivalBootstrapNeighbor = core.BlockPos{X: 305, Z: 480}
)

const (
	// survivalBootstrapCanopyTop 是出生列树冠顶的叶格 Y：出生脚层是它加一。
	survivalBootstrapCanopyTop int32 = 71
	// survivalBootstrapLogTop/Bottom 是五格树干的首尾 Y。
	survivalBootstrapLogTop    int32 = 69
	survivalBootstrapLogBottom int32 = 65
	// survivalBootstrapGrassY 是树干正下方的草格 Y。
	survivalBootstrapGrassY int32 = 64
	// survivalBootstrapStoneTop 是石料顶格 Y（其上是三格泥土）。
	survivalBootstrapStoneTop int32 = 60
	// survivalBootstrapLogCount 是树干提供的原木数：四次取出木板 + 一栈余料。
	survivalBootstrapLogCount = 5

	// survivalBootstrapMiningBudget 是单格采掘的 tick 预算：最硬的徒手石料
	// 30 tick，留一倍余量。它只把「永不收敛」变成可诊断失败，不是性能门禁。
	survivalBootstrapMiningBudget = 60
	// survivalBootstrapPickupBudget 是掉落物过完 10 tick 拾取延迟并入包的 tick 预算。
	survivalBootstrapPickupBudget = 30
	// survivalBootstrapSettleTicks 是一条状态命令之后让发布收敛的 tick 数。
	survivalBootstrapSettleTicks = 3
)

// survivalBootstrapAim 返回从 eye 指向 target 的 yaw/pitch：它是 sim 侧
// `LookDirection` 的逆运算，脚本用它在权威眼睛位置上瞄准某一格的中心。
//
// pitch 收敛到协议接受的开区间上界之内（`validPlayerLook` 拒绝恰好 ±π/2 的
// 俯仰）：正下方那一格的瞄准会被钳到 -π/2 + 0.01，射线因此带一点点水平偏移，
// 与既有翻地脚本的 `tillSoilLookDown` 完全同形，落点仍在同一列之内。
func survivalBootstrapAim(eye, target mgl32.Vec3) (yaw, pitch float32) {
	const maxPitch = float32(math.Pi/2 - 0.01)
	direction := target.Sub(eye)
	direction = direction.Mul(1 / direction.Len())
	pitch = float32(math.Asin(float64(direction[1])))
	pitch = max(-maxPitch, min(maxPitch, pitch))
	return float32(math.Atan2(float64(-direction[0]), float64(-direction[2]))), pitch
}

// survivalBootstrapBlockCenter 返回某一格几何中心的世界坐标。
func survivalBootstrapBlockCenter(position core.BlockPos) mgl32.Vec3 {
	return mgl32.Vec3{
		float32(position.X) + 0.5,
		float32(position.Y) + 0.5,
		float32(position.Z) + 0.5,
	}
}

// survivalBootstrapViewLoaded 报告冻结样本周围的 3×3 视野是否都已进入镜像
// （ViewRadius=1 的视野即锚点周围的九个区块）。
func survivalBootstrapViewLoaded(mirror *client.Mirror) bool {
	for x := survivalBootstrapAnchor.X - 1; x <= survivalBootstrapAnchor.X+1; x++ {
		for z := survivalBootstrapAnchor.Z - 1; z <= survivalBootstrapAnchor.Z+1; z++ {
			if _, ok := mirror.Chunk(core.Overworld, core.ChunkPos{X: x, Z: z}); !ok {
				return false
			}
		}
	}
	return true
}

// TestSurvivalBootstrapEmptyInventoryToStonePickaxe 是空背包起家的端到端冒烟：
// 一名从未存在过的新玩家在**真实生成的缺省世界**里以完全空的背包登录，此后只用
// 真实玩家命令把「原木 → 木板 → 工作台 → 放置工作台 → 3×3 网格 → 石镐」推进到底。
//
// 它证明的是规则真的接进了世界：规则表层面的可达性由价值闭包守卫覆盖，但「配方与
// 掉落登记存在」不等于「真实世界坐标上能徒手挖到、掉落能捡回、材料能摆进网格」。
// 因此脚本不使用 `SetPlayerInventoryForTest` 或任何直给物品的后门，每一步的每一件
// 物品都来自真实命令与世界结算：
//
//  1. 登录背包必须是 36 格全空（新玩家初始背包为空的契约）。
//  2. 出生在自然橡树的树冠顶上，向下笔直采掘。脚底那一格的掉落中心恒在脚下半格，
//     永远落在 1.25 格拾取范围内——这是全程不需要移动与跳跃的原因；五格原木的
//     采掘足够慢，掉落先过完 10 tick 拾取延迟并入包（逐格断言），树叶这类 5 tick
//     的方块则可能在玩家落进下一格后才到点、留在世界里。
//  3. 原木经权威合成网格取出木板。整堆移动不能拆堆，而工作台与石镐都要求同一种
//     材料占多个独立格，脚本用两条真实路径造出独立栈：连续取出同一条配方并把产物
//     逐栈挪位（木板、木棍），以及逐格采掘后先挪走再采下一格（石料）。
//  4. 工作台在真实世界里放置、经权威射线打开，网格尺寸由 2 升到 3。
//  5. 空手采掘石料（30 tick、掉落石料；石料的采掘规则只看选中物，因此每格开凿
//     前都把选中栏换成真正的空手），与木棍在 3×3 网格里合成石镐，断言石镐及其
//     完整耐久落在权威背包里。
func TestSurvivalBootstrapEmptyInventoryToStonePickaxe(t *testing.T) {
	// —— 玩家输入前的样本前提断言：任何一条失败都说明冻结样本失效 ——
	//
	// 出生列自树冠顶向下逐格核对；邻居列核对工作台放置几何（脚层与上一层为空、
	// 其下是草与可被射线命中的石料）。运行时只读，不搜索、不改写。
	probe := worldgen.New(survivalBootstrapSeed, true)
	columnAt := func(y int32) core.BlockPos {
		return core.BlockPos{X: survivalBootstrapColumn.X, Y: y, Z: survivalBootstrapColumn.Z}
	}
	neighborAt := func(y int32) core.BlockPos {
		return core.BlockPos{X: survivalBootstrapNeighbor.X, Y: y, Z: survivalBootstrapNeighbor.Z}
	}
	for _, entry := range []struct {
		position core.BlockPos
		want     core.BlockID
	}{
		{columnAt(survivalBootstrapCanopyTop), core.LeavesID},
		{columnAt(survivalBootstrapCanopyTop + 1), core.AirID},
		{columnAt(survivalBootstrapCanopyTop + 2), core.AirID},
		{columnAt(survivalBootstrapLogTop), core.OakLogID},
		{columnAt(survivalBootstrapLogBottom), core.OakLogID},
		{columnAt(survivalBootstrapGrassY), core.GrassID},
		{columnAt(survivalBootstrapStoneTop), core.StoneID},
		{columnAt(survivalBootstrapStoneTop - 3), core.StoneID},
		{neighborAt(survivalBootstrapGrassY + 1), core.AirID},
		{neighborAt(survivalBootstrapGrassY), core.GrassID},
		{neighborAt(survivalBootstrapGrassY - 1), core.DirtID},
		{neighborAt(survivalBootstrapStoneTop + 1), core.DirtID},
		{neighborAt(survivalBootstrapStoneTop), core.StoneID},
	} {
		if got := probe.BaseBlockAt(core.Overworld, entry.position); got != entry.want {
			t.Fatalf("冻结样本前提失效：%+v = %d，想要 %d", entry.position, got, entry.want)
		}
	}

	identity := integrationIdentity(0x9b, "EmptyStartSurvivor")
	// 刻意不预存玩家：只有 `LoadPlayer` 返回 `ErrPlayerNotFound` 的路径才会构造
	// 新玩家的空初始背包，脚本第一步要看的正是这份空背包。生成器用生产
	// `worldgen.New`（流体开启 = 生产默认配置），不经任何测试生成旁路。
	store := storage.NewMemory(storage.Metadata{
		FormatVersion:     5,
		Seed:              survivalBootstrapSeed,
		SpawnDimension:    core.Overworld,
		SpawnAnchor:       survivalBootstrapAnchor,
		DepthsSpawnAnchor: survivalBootstrapAnchor,
		DepthsSeedSalt:    0x9E3779B97F4A7C15,
	})
	config := hostTestConfig()
	config.ViewRadius = 1
	config.AutosaveTicks = 1 << 20
	host := mustNewHost(t, config, worldgen.New(survivalBootstrapSeed, true), store)
	endpoint, _, closeTransport := openParityTransport(t, host, "memory", identity)
	t.Cleanup(func() {
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	})

	mirror := client.NewMirror()
	session := func() contract.SessionID {
		t.Helper()
		host.mu.Lock()
		defer host.mu.Unlock()
		active := host.activeByPlayer[identity.PlayerID]
		if active == nil {
			t.Fatal("玩家还没有 active 会话")
		}
		return active.Session
	}
	authoritativeSnapshot := func() contract.PlayerSnapshot {
		t.Helper()
		snapshot, ok := host.world.PlayerSnapshotFor(session())
		if !ok {
			t.Fatal("没有权威玩家快照")
		}
		return snapshot
	}
	authoritativeInventory := func() core.Inventory {
		t.Helper()
		return authoritativeSnapshot().Inventory
	}
	// authoritativeCrafting 是只读观察点：脚本从不写入网格，只读权威值断言它。
	authoritativeCrafting := func() runtime.CraftingGrid {
		t.Helper()
		grid, _, ok := host.world.engine.PlayerCrafting(session())
		if !ok {
			t.Fatal("没有权威合成网格")
		}
		return grid
	}

	// —— 第 1 步：空背包登录，出生在树冠顶上 ——
	//
	// 就绪信号取「携带权威背包的 `network.InventoryState` 已到达」这一事实本身，
	// 不看背包内容：新玩家初始背包为空是契约，以「背包非空」为信号会空转到预算耗尽。
	ready, inventoryPublished := false, false
	waitIntegrationLoginReady(
		t,
		"survival bootstrap empty login",
		func() bool { return ready && inventoryPublished && survivalBootstrapViewLoaded(mirror) },
		func() string {
			return fmt.Sprintf("ready=%v 背包已发布=%v 视野已加载=%v",
				ready, inventoryPublished, survivalBootstrapViewLoaded(mirror))
		},
		func() {
			_, messages := parityStep(t, host, endpoint, mirror)
			for _, message := range messages {
				switch message := message.(type) {
				case network.PlayerState:
					ready = ready || message.Ready
				case network.InventoryState:
					inventoryPublished = true
				}
			}
		},
	)
	login := authoritativeSnapshot()
	// 空背包断言扫全部 36 格：任何一格出现材料、种子、工具或食物都要在这里红。
	forEachStack(login.Inventory, func(slot int, stack core.ItemStack) {
		if stack != (core.ItemStack{}) {
			t.Fatalf("登录时统一索引 %d = %+v，想要空槽（新玩家初始背包为空）", slot, stack)
		}
	})
	spawn := login.Current.Position
	if int32(math.Floor(float64(spawn.X()))) != survivalBootstrapColumn.X ||
		int32(math.Floor(float64(spawn.Z()))) != survivalBootstrapColumn.Z ||
		spawn.Y() < float32(survivalBootstrapCanopyTop+1) || spawn.Y() >= float32(survivalBootstrapCanopyTop+2) {
		t.Fatalf("出生点 = (%.2f,%.2f,%.2f)，想要自然树冠顶 (%d,%d,%d) 上方",
			spawn.X(), spawn.Y(), spawn.Z(),
			survivalBootstrapColumn.X, survivalBootstrapCanopyTop+1, survivalBootstrapColumn.Z)
	}

	// —— 命令通道：序号由脚本统一递增，脚本外的拒绝一律直接 Fatal ——
	sequence := uint64(0)
	next := func() uint64 { sequence++; return sequence }
	step := func() {
		t.Helper()
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			if rejected, ok := message.(network.CommandRejected); ok {
				t.Fatalf("空背包冒烟的命令被拒绝: %+v", rejected)
			}
		}
	}
	send := func(build func(sequence uint64) network.ClientMessage) {
		t.Helper()
		sendIntegration(t, endpoint, build(next()))
		waitIntegrationCondition(t, "survival bootstrap command queued", func() bool {
			return len(host.world.incoming) > 0
		})
		step()
	}
	settle := func() {
		t.Helper()
		for range survivalBootstrapSettleTicks {
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
	waitInventory := func(label string, want func(core.Inventory) bool) {
		t.Helper()
		for ticks := 0; !want(authoritativeInventory()); ticks++ {
			if ticks > survivalBootstrapPickupBudget {
				t.Fatalf("%s：等了 %d 个 tick 仍未收敛，当前背包 = %+v",
					label, ticks, authoritativeInventory())
			}
			step()
		}
	}
	aimAt := func(position core.BlockPos) (float32, float32) {
		t.Helper()
		eye := authoritativeSnapshot().Current.Position.Add(
			mgl32.Vec3{0, runtime.ActiveTickTunables().Physics.EyeHeight, 0},
		)
		return survivalBootstrapAim(eye, survivalBootstrapBlockCenter(position))
	}
	// mineBlock 用一次真实持续 primary 输入采掘目标格：先按权威眼睛位置瞄准该格
	// 中心，发一次持续采掘输入，等镜像读到空气后立刻松键。
	mineBlock := func(target core.BlockPos, label string) {
		t.Helper()
		yaw, pitch := aimAt(target)
		send(func(sequence uint64) network.ClientMessage {
			return network.PlayerInput{Sequence: sequence, Yaw: yaw, Pitch: pitch, Mining: true}
		})
		for ticks := 0; mirrorBlock(target) != core.AirID; ticks++ {
			if ticks > survivalBootstrapMiningBudget {
				t.Fatalf("%s：推进 %d 个 tick 后 %+v 仍是 %d，想要空气",
					label, ticks, target, mirrorBlock(target))
			}
			step()
		}
		send(func(sequence uint64) network.ClientMessage {
			return network.PlayerInput{Sequence: sequence, Yaw: yaw, Pitch: pitch}
		})
	}

	// —— 第 2 步：从树冠顶向下笔直采掘，逐格断言并等掉落物入包 ——
	//
	// 每格先断言镜像里脚底那一格就是冻结样本的方块，再发持续采掘输入；权威进度
	// 跑满后该格变空气。拾取延迟 10 tick 是按「采掘完成后仍在范围内的活动 tick」
	// 计的：原木 15 tick 的采掘足够掉落物先入包，树叶这类 5 tick 的方块则可能在
	// 玩家落进下一格后才到点，脚本因此只对链路真正依赖的原木做入包断言。
	feet := survivalBootstrapCanopyTop + 1
	for y := survivalBootstrapCanopyTop; y >= survivalBootstrapLogBottom; y-- {
		position := core.BlockPos{X: survivalBootstrapColumn.X, Y: feet - 1, Z: survivalBootstrapColumn.Z}
		want := core.LeavesID
		if y >= survivalBootstrapLogBottom && y <= survivalBootstrapLogTop {
			want = core.OakLogID
		}
		if got := mirrorBlock(position); got != want {
			t.Fatalf("徒手采掘前 %+v 镜像 = %d，想要 %d", position, got, want)
		}
		mineBlock(position, fmt.Sprintf("徒手采掘 Y=%d", y))
		if want == core.OakLogID {
			mined := int(survivalBootstrapLogTop - y + 1)
			waitInventory(fmt.Sprintf("Y=%d 原木入包", y), func(inventory core.Inventory) bool {
				return countItem(inventory, core.ItemOakLog) == mined
			})
		}
		feet--
	}
	if got := countItem(authoritativeInventory(), core.ItemOakLog); got != survivalBootstrapLogCount {
		t.Fatalf("树干采掘后背包原木 = %d，想要 %d（徒手伐木必须真的把原木捡回来）",
			got, survivalBootstrapLogCount)
	}
	// 继续下挖到石料顶：草与三格泥土的掉落同样走真实采掘与拾取路径。
	for y := survivalBootstrapGrassY; y > survivalBootstrapStoneTop; y-- {
		want := core.DirtID
		if y == survivalBootstrapGrassY {
			want = core.GrassID
		}
		position := core.BlockPos{X: survivalBootstrapColumn.X, Y: feet - 1, Z: survivalBootstrapColumn.Z}
		if got := mirrorBlock(position); got != want {
			t.Fatalf("下挖前 %+v 镜像 = %d，想要 %d", position, got, want)
		}
		mineBlock(position, fmt.Sprintf("徒手采掘 Y=%d", y))
		feet--
	}
	if feet != survivalBootstrapStoneTop+1 {
		t.Fatalf("下挖到底后脚层 = %d，想要石料顶上一层 %d", feet, survivalBootstrapStoneTop+1)
	}
	belowFeet := core.BlockPos{X: survivalBootstrapColumn.X, Y: feet - 1, Z: survivalBootstrapColumn.Z}
	if got := mirrorBlock(belowFeet); got != core.StoneID {
		t.Fatalf("下挖到底后脚底 %+v = %d，想要 %d", belowFeet, got, core.StoneID)
	}

	// —— 第 3 步：原木合成木板（连续取出 + 产物逐栈挪出快捷栏造出独立栈）——
	//
	// 产物按稳定插入顺序落到快捷栏第一个空格：取出后立刻把这一栈挪进背包，下一次
	// 取出的产物才会落到新的快捷栏空格而不是并进它，于是得到四栈互不合并的木板
	// ——工作台的 2×2 需要四格各有一份独立木板栈。
	inventorySlot := func(inventory core.Inventory, item core.ItemID) (uint8, bool) {
		for slot := uint8(0); slot < core.InventorySlots; slot++ {
			stack, _ := inventory.Slot(slot)
			if stack.Item == item {
				return slot, true
			}
		}
		return 0, false
	}
	emptySlot := func(inventory core.Inventory) (uint8, bool) {
		for slot := uint8(0); slot < core.InventorySlots; slot++ {
			stack, _ := inventory.Slot(slot)
			if stack == (core.ItemStack{}) {
				return slot, true
			}
		}
		return 0, false
	}
	emptyBackpackSlot := func(inventory core.Inventory) (uint8, bool) {
		for slot := uint8(core.HotbarSlots); slot < core.InventorySlots; slot++ {
			stack, _ := inventory.Slot(slot)
			if stack == (core.ItemStack{}) {
				return slot, true
			}
		}
		return 0, false
	}
	moveWithinInventory := func(from, to uint8) {
		t.Helper()
		send(func(sequence uint64) network.ClientMessage {
			return network.MoveInventoryStack{Sequence: sequence, From: from, To: to}
		})
		settle()
	}
	moveToGrid := func(from, to uint8) {
		t.Helper()
		send(func(sequence uint64) network.ClientMessage {
			return network.MoveCraftingStack{Sequence: sequence, From: from, To: to}
		})
		settle()
	}
	takeCraftingOutput := func() {
		t.Helper()
		send(func(sequence uint64) network.ClientMessage {
			return network.TakeCraftingOutput{Sequence: sequence}
		})
		settle()
	}
	// parkInInventory 把产物栈挪进背包：`AddStack` 的合并阶段先扫快捷栏，产物栈
	// 只要留在快捷栏里，下一次取出就会并进它；挪出快捷栏后下一次取出才会落成新的
	// 独立栈——这正是「整堆移动不能拆堆」下造出多个同物品栈的唯一真实路径。
	parkInInventory := func(item core.ItemID) {
		t.Helper()
		inventory := authoritativeInventory()
		slot, ok := inventorySlot(inventory, item)
		if !ok {
			t.Fatalf("背包里找不到 %d", item)
		}
		if slot >= core.HotbarSlots {
			return
		}
		target, ok := emptyBackpackSlot(inventory)
		if !ok {
			t.Fatalf("背包里没有空格安放 %d 的独立栈", item)
		}
		moveWithinInventory(slot, target)
	}

	logSlot, ok := inventorySlot(authoritativeInventory(), core.ItemOakLog)
	if !ok {
		t.Fatal("背包里找不到原木栈")
	}
	moveToGrid(logSlot+core.CraftingGridSlots, 0)
	for index := range 4 {
		takeCraftingOutput()
		want := uint8((index + 1) * 4)
		if got := countItem(authoritativeInventory(), core.ItemOakPlanks); got != int(want) {
			t.Fatalf("第 %d 次取出木板后背包木板 = %d，想要 %d", index+1, got, want)
		}
		parkInInventory(core.ItemOakPlanks)
	}
	// 树干栈留在网格格 0（取四次只消费四格原木）：把它挪回背包，网格恢复为空，
	// 工作台的 2×2 才不被别的格子撑大包围盒。
	if grid := authoritativeCrafting(); grid.Slots[0] != (core.ItemStack{Item: core.ItemOakLog, Count: 1}) {
		t.Fatalf("四次取出后网格格 0 = %+v，想要余下的一格原木", grid.Slots[0])
	}
	rest, ok := emptySlot(authoritativeInventory())
	if !ok {
		t.Fatal("背包里没有空格安放网格里剩余的原木")
	}
	moveToGrid(0, rest+core.CraftingGridSlots)
	if grid := authoritativeCrafting(); grid.Slots != [core.CraftingGridSlots]core.ItemStack{} {
		t.Fatalf("木板取出后网格 = %+v，想要空网格", grid.Slots)
	}

	// —— 第 4 步：2×2 个人网格合成工作台 ——
	//
	// 四个独立木板栈逐格搬进格 0..3，取出即得工作台。
	for index := range 4 {
		slot, ok := inventorySlot(authoritativeInventory(), core.ItemOakPlanks)
		if !ok {
			t.Fatalf("找不到第 %d 个独立木板栈", index+1)
		}
		moveToGrid(slot+core.CraftingGridSlots, uint8(index))
	}
	takeCraftingOutput()
	if got := countItem(authoritativeInventory(), core.ItemWorkbench); got != 1 {
		t.Fatalf("2×2 网格取出后背包工作台 = %d，想要 1", got)
	}

	// —— 第 5 步：木棍（两条独立木棍栈，供石镐中列）——
	//
	// 取出工作台后四格各剩三块木板，包围盒是 2×2、不匹配任何配方：先把格 1/3 的
	// 木板挪空，只留 0/2 两格——个人网格的纵向两格正好是木棍的 1×2 形状。逐次取出
	// 两次得到两条独立木棍栈，正好铺满石镐中列两格。
	for _, cell := range []uint8{1, 3} {
		target, ok := emptySlot(authoritativeInventory())
		if !ok {
			t.Fatal("背包里没有空格安放腾出的木板栈")
		}
		moveToGrid(cell, target+core.CraftingGridSlots)
	}
	for index := range 2 {
		takeCraftingOutput()
		want := uint8((index + 1) * 4)
		if got := countItem(authoritativeInventory(), core.ItemStick); got != int(want) {
			t.Fatalf("第 %d 次取出木棍后背包木棍 = %d，想要 %d", index+1, got, want)
		}
		parkInInventory(core.ItemStick)
	}
	// 网格只留木棍配方残留的木板，全部挪回背包，为石料让出格 0..2。
	for _, cell := range []uint8{0, 2} {
		target, ok := emptySlot(authoritativeInventory())
		if !ok {
			t.Fatal("背包里没有空格安放残余木板栈")
		}
		moveToGrid(cell, target+core.CraftingGridSlots)
	}
	if grid := authoritativeCrafting(); grid.Slots != [core.CraftingGridSlots]core.ItemStack{} {
		t.Fatalf("木棍取出后网格 = %+v，想要空网格", grid.Slots)
	}

	// —— 第 6 步：清出工作台放置面 ——
	//
	// 邻居列脚层之下是三层泥土：视线必须先越过更上面那一格，因此先挖掉 Y=62 再挖
	// Y=61。62 的掉落物在脚层上方一格半、超出 1.25 格拾取范围，留在世界里；61 的
	// 掉落物落在范围内，按真实拾取入包——这里用泥土数量逐份断言它真的被捡回来了。
	dirtBefore := countItem(authoritativeInventory(), core.ItemDirt)
	for _, y := range []int32{survivalBootstrapGrassY - 2, survivalBootstrapGrassY - 3} {
		position := core.BlockPos{X: survivalBootstrapNeighbor.X, Y: y, Z: survivalBootstrapNeighbor.Z}
		if got := mirrorBlock(position); got != core.DirtID {
			t.Fatalf("清理工作台放置面前 %+v 镜像 = %d，想要 %d", position, got, core.DirtID)
		}
		mineBlock(position, fmt.Sprintf("清理邻居列 Y=%d", y))
	}
	waitInventory("邻居列泥土入包", func(inventory core.Inventory) bool {
		return countItem(inventory, core.ItemDirt) == dirtBefore+1
	})
	// 工作台落点：邻居列石料顶上一格，与玩家脚层同级、一列之差。
	placement := core.BlockPos{
		X: survivalBootstrapNeighbor.X,
		Y: survivalBootstrapStoneTop + 1,
		Z: survivalBootstrapNeighbor.Z,
	}
	if got := mirrorBlock(placement); got != core.AirID {
		t.Fatalf("工作台落点 %+v 镜像 = %d，想要空气", placement, got)
	}

	// —— 第 7 步：放置工作台并经权威射线打开，网格尺寸升到 3 ——
	workbenchSlot, ok := inventorySlot(authoritativeInventory(), core.ItemWorkbench)
	if !ok || workbenchSlot >= core.HotbarSlots {
		t.Fatal("快捷栏里找不到工作台")
	}
	// 放置落点由权威射线决定：瞄准邻居列石料格顶面——它上方两格刚被清空，射线
	// 沿这一列落下时首个命中就是石料顶面，落点即石料顶上一格。
	yaw, pitch := aimAt(core.BlockPos{
		X: survivalBootstrapNeighbor.X, Y: survivalBootstrapStoneTop, Z: survivalBootstrapNeighbor.Z,
	})
	send(func(sequence uint64) network.ClientMessage {
		return network.PlaceBlock{Sequence: sequence, Yaw: yaw, Pitch: pitch, Slot: workbenchSlot}
	})
	settle()
	if got := mirrorBlock(placement); got != core.WorkbenchID {
		t.Fatalf("放置后 %+v 镜像 = %d，想要工作台 %d", placement, got, core.WorkbenchID)
	}
	if got := countItem(authoritativeInventory(), core.ItemWorkbench); got != 0 {
		t.Fatalf("放置后背包工作台 = %d，想要 0（工作台必须真的被放进世界）", got)
	}
	yaw, pitch = aimAt(placement)
	send(func(sequence uint64) network.ClientMessage {
		return network.OpenContainer{Sequence: sequence, Yaw: yaw, Pitch: pitch}
	})
	settle()
	if grid := authoritativeCrafting(); grid.Size != runtime.CraftingGridSizeWorkbench {
		t.Fatalf("打开工作台后网格尺寸 = %d，想要 %d", grid.Size, runtime.CraftingGridSizeWorkbench)
	}

	// —— 第 8 步：徒手采掘石料，逐格入包后立刻挪进网格顶排 ——
	//
	// 石镐顶排要三份互相独立的石料：每挖一格先等它的掉落物入包，再立刻把这一栈挪进
	// 网格格 0/1/2——下一格的掉落物才会落成新的独立栈，顶排三格因此各得一份石料。
	for index := range 3 {
		position := core.BlockPos{X: survivalBootstrapColumn.X, Y: feet - 1, Z: survivalBootstrapColumn.Z}
		if got := mirrorBlock(position); got != core.StoneID {
			t.Fatalf("采石前 %+v 镜像 = %d，想要 %d", position, got, core.StoneID)
		}
		// 「徒手」必须把选中栏换成真正的空手：石料的采掘规则只看选中物——空手
		// （与损坏的镐）掉石料，手持别的物品虽然同样 30 tick 破坏却**不掉落**。
		// 拾取可能落进原先选中的空格，因此每一格开凿前都重新选一个空格。
		inventory := authoritativeInventory()
		vacant := uint8(core.HotbarSlots)
		for slot, stack := range inventory.Hotbar.Slots {
			if stack == (core.ItemStack{}) {
				vacant = uint8(slot)
				break
			}
		}
		if vacant == core.HotbarSlots {
			t.Fatal("快捷栏没有空格可选为空手")
		}
		send(func(sequence uint64) network.ClientMessage {
			return network.SelectHotbar{Sequence: sequence, Slot: vacant}
		})
		settle()
		before := countItem(authoritativeInventory(), core.ItemStone)
		mineBlock(position, fmt.Sprintf("徒手采掘石料 Y=%d", position.Y))
		waitInventory(fmt.Sprintf("石料 Y=%d 入包", position.Y), func(inventory core.Inventory) bool {
			return countItem(inventory, core.ItemStone) == before+1
		})
		slot, ok := inventorySlot(authoritativeInventory(), core.ItemStone)
		if !ok {
			t.Fatal("采掘后背包里找不到石料栈")
		}
		moveToGrid(slot+core.CraftingGridSlots, uint8(index))
		if stack := authoritativeCrafting().Slots[index]; stack != (core.ItemStack{Item: core.ItemStone, Count: 1}) {
			t.Fatalf("网格格 %d = %+v，想要单份石料", index, stack)
		}
		if got := countItem(authoritativeInventory(), core.ItemStone); got != 0 {
			t.Fatalf("石料挪进网格后背包仍有 %d 份石料，想要 0", got)
		}
		feet--
	}

	// —— 第 9 步：木棍铺中列两格，3×3 取出石镐并断言完整耐久 ——
	for _, cell := range []uint8{4, 7} {
		slot, ok := inventorySlot(authoritativeInventory(), core.ItemStick)
		if !ok {
			t.Fatalf("背包里找不到木棍栈（网格格 %d）", cell)
		}
		moveToGrid(slot+core.CraftingGridSlots, cell)
	}
	fullDurability, ok := core.ItemMaxDurability(core.ItemStonePickaxe)
	if !ok {
		t.Fatal("石镐没有耐久上限登记")
	}
	wantPickaxe := core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: fullDurability}
	takeCraftingOutput()
	inventory := authoritativeInventory()
	if got := countItem(inventory, core.ItemStonePickaxe); got != 1 {
		t.Fatalf("3×3 取出后背包石镐 = %d，想要 1（空背包起家链的终点）", got)
	}
	slot, ok := inventorySlot(inventory, core.ItemStonePickaxe)
	if !ok || slot >= core.HotbarSlots {
		t.Fatal("取出后快捷键栏里找不到石镐")
	}
	if got := inventory.Hotbar.Slots[slot]; got != wantPickaxe {
		t.Fatalf("石镐 = %+v，想要满耐久 %+v", got, wantPickaxe)
	}
	grid := authoritativeCrafting()
	for index := range 3 {
		if grid.Slots[index] != (core.ItemStack{}) {
			t.Fatalf("取出后顶排第 %d 格 = %+v，想要空（石料恰好被消费一份）", index, grid.Slots[index])
		}
	}
	for _, cell := range []uint8{4, 7} {
		if stack := grid.Slots[cell]; stack.Item != core.ItemStick || stack.Count != 3 {
			t.Fatalf("取出后网格格 %d = %+v，想要余下三根木棍", cell, stack)
		}
	}
}
