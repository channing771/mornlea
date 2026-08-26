package server

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/internal/world"
)

// 本文件覆盖变更 flood-destroys-crops 的 Memory/TCP 双传输一致性（tasks.md 2.3）：
// 同一次「溃坝冲毁农田」在两种传输下，广播的方块变更逐格一致、最终掉落物集合
// 一致。组织方式复用同目录 fluid_transport_parity_test.go 的溃坝先例：逐 tick
// 保序录制方块变更，任何一侧的广播漏发、重排或改写都会让 DeepEqual 变红。
//
// 与先例的一点刻意差异是**触发方式**。先例的水源就在初始视野里，从「就绪」直接
// 开录即可；本场景若照抄，冲毁会落在「玩家 active 但镜像还没收完快照」的就绪
// 漂移区里——实测两种传输的就绪耗时可以差出两个数量级（这正是既有 transcript
// parity 把 PlayerState 绝对 tick 归零的原因）。因此这里把触发事件改成
// **脚本化的确定性行走**：夹具放在出生兴趣范围之外、但初始视野之内（ViewRadius
// 大于 `core.DropInterestRadius`）的区块，让它在两侧各自就绪的阶段——也就是录制
// 开始之前——就已加载完毕，彻底移除跨区块异步生成时序这个不可锁定的变量；随后
// 同一条行走脚本把玩家走进夹具的兴趣范围，边界重扫唤醒种子水源，冲毁在两侧落在
// 完全相同的相对 tick 上。
//
// 就绪耗时不同还意味着两侧在录制开始时的**绝对 tick** 不同（实测差约十几 tick，
// 高负载下更大）。方块变更与 revision 都只依赖相对次序，这无关紧要；但成熟作物
// 的掉落数量自 crop-random-drop-count 起按 (seed, 权威绝对 tick, 维度, 坐标) 取
// 哈希——绝对 tick 不对齐，同一株小麦在两种传输下会收到不同的「合法」产量，逐件
// 比对就会假红。因此两侧就绪后、开录前各自空转到同一个绝对 tick
// （floodCropParityAlignTicks）：对齐后行走脚本的每个事件都落在相同的绝对 tick
// 上，冲毁结算与产量哈希的输入逐项相同。空转期间世界完全静止（随机 tick 已置零、
// 兴趣范围内无流体），不产生任何录像内容。
//
// 对时间平移**不**不变的唯一剩余活动是作物的随机 tick（抽样哈希含绝对 tick）：
// 耕地干湿转换会在不可预测的相对位置插进录像，破坏逐 tick 比对。录制前用
// `sim.SetTunables` 把 `RandomTicksPerSection` 置零（farming_loop_e2e_test.go 的
// 同款先例），本测试的契约只关心流体冲毁，不需要随机 tick 参与。

// floodCropFarmland 是夹具中作物格正下方的耕地。
var floodCropFarmland = core.BlockPos{X: 52, Y: 0, Z: 8}

// floodCropCell 是夹具中的成熟小麦格。
var floodCropCell = core.BlockPos{X: 52, Y: 1, Z: 8}

// floodCropSource 是夹具里唯一的水源：悬在作物正上方一格。种子阶段就把它写进
// 生成器，区块进入推进范围时的边界重扫会自动发现并入队它，无需任何注入或客户端
// 指令。
var floodCropSource = core.BlockPos{X: 52, Y: 2, Z: 8}

const (
	// floodCropParityViewRadius 是本测试的服务端视野半径：必须大于
	// core.DropInterestRadius（2），使夹具所在区块 (3,0) 落在「初始视野之内、
	// 出生兴趣范围之外」的窗口里。
	floodCropParityViewRadius = 5

	// floodCropParityWalkTicks 是行走脚本的长度。WalkSpeed 默认 4.3 格/秒、每
	// tick 约 0.215 格：兴趣范围在中心进入区块 (1,0)（x≈16，约 73 tick）时覆盖
	// 到夹具区块并触发冲毁；245 约停在 x≈53，留在夹具区块内部——行走段结束后
	// 玩家必须停留在夹具的兴趣半径内，否则区块卸载会把已发布的掉落物经 remove
	// 撤回，录像尾部就不再反映冲毁的结果。
	floodCropParityWalkTicks = 245

	// floodCropParityTicks 是整段录像的长度：行走段加上冲毁与铺开的收敛段，
	// 末尾自然留出静默期。
	floodCropParityTicks = 400

	// floodCropParityAlignTicks 是两侧录制开始前共同空转到的绝对 tick。取值
	// 只需同时满足两个不等式：大于任何可预期的就绪耗时（实测几十 tick，留出
	// 一个数量级以上余量），又远小于会让测试明显变慢的量级（每 tick 亚毫秒级）。
	// 就绪耗时一旦超过它，测试会带着明确指示失败——那是「对齐前提失效」的响亮
	// 报警，不是可以静默重试的 flake。
	floodCropParityAlignTicks = 256
)

// floodCropGenerator 生成 y=0 一层草地的世界，并在区块 (3,0) 内种下
// 「耕地 + 成熟小麦 + 悬空水源」。其余区块就是纯草地。
type floodCropGenerator struct{}

// GenerateChunk 实现 Generator。
func (floodCropGenerator) GenerateChunk(position core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(position)
	for x := range core.SectionSize {
		for z := range core.SectionSize {
			chunk.SetBlock(x, 0, z, core.GrassID)
		}
	}
	if position == floodCropCell.Chunk() {
		farmlandX, _, farmlandZ := floodCropFarmland.Local()
		cellX, _, cellZ := floodCropCell.Local()
		sourceX, _, sourceZ := floodCropSource.Local()
		chunk.SetBlock(farmlandX, floodCropFarmland.Y, farmlandZ, core.FarmlandDryID)
		chunk.SetBlock(cellX, floodCropCell.Y, cellZ, core.WheatStage7ID)
		chunk.SetBlock(sourceX, floodCropSource.Y, sourceZ, core.WaterSourceID)
	}
	chunk.Compact()
	return chunk
}

// fluidCropDropEntry 是掉落物的语义投影：只保留「哪一格、什么物品、多少个」，
// 槽位号与 generation 属于分配细节，不参与跨传输比对。
type fluidCropDropEntry struct {
	BlockIndex uint32
	Item       core.ItemID
	Count      uint8
}

// fluidCropParityRecord 是一次冲毁在某种传输下的完整录像：逐 tick 保序的方块
// 变更投影加上收尾时的掉落物集合与「夹具真的发生了冲毁」标记。
type fluidCropParityRecord struct {
	Ticks [][]string
	Drops []fluidCropDropEntry
	Flood bool
}

// TestMemoryTCPFluidCropFloodBroadcastParity 覆盖 tasks.md 2.3：同一次溃坝冲毁
// 农田场景下，两种传输的方块变更逐格一致、最终掉落物集合一致。
func TestMemoryTCPFluidCropFloodBroadcastParity(t *testing.T) {
	memory := recordFluidCropParity(t, "memory")
	tcp := recordFluidCropParity(t, "tcp")
	if !reflect.DeepEqual(tcp.Ticks, memory.Ticks) {
		index, memoryTick, tcpTick := firstDamParityMismatch(
			damParityRecord{Ticks: memory.Ticks}, damParityRecord{Ticks: tcp.Ticks},
		)
		t.Fatalf("冲毁农田广播 Memory/TCP 在第 %d 个 tick 起不一致（flood@memory=%d tcp=%d）\nmemory=%v\ntcp=%v",
			index, fluidCropFloodIndex(memory), fluidCropFloodIndex(tcp),
			memoryTick, tcpTick)
	}
	if !reflect.DeepEqual(tcp.Drops, memory.Drops) {
		t.Fatalf("冲毁农田掉落物 Memory/TCP 不一致\nmemory=%+v\ntcp=%+v",
			memory.Drops, tcp.Drops)
	}

	// 夹具前提守卫排在真实断言之后（与溃坝先例同一纪律）：真实的传输差异必须
	// 先报出自己的诊断，而不是被「夹具没水」抢先误导。这里同时钉住本测试的
	// 业务内容——冲毁确实发生在录制窗口内，且产出是采掘同表的小麦 + 种子两类
	// 各一堆，数量由 (seed, 结算 tick, 维度, 坐标) 的确定性哈希给出、各落在
	// [1,3]。两侧的绝对 tick 已在开录前对齐，跨传输逐件一致由上面的 DeepEqual
	// 锁定；这组结构断言是独立于比对的形状守卫，精确重放另由 sim 侧按
	// `cropYieldRolls` 现算的用例锁定。
	if !memory.Flood {
		t.Fatal("夹具失效：整个录制窗口内没有出现作物格被写成流动水的变更")
	}
	cropIndex, indexed := world.ChunkBlockIndex(floodCropCell)
	if !indexed {
		t.Fatal("作物格没有区块索引")
	}
	hasWheat, hasSeeds := false, false
	for _, entry := range memory.Drops {
		switch {
		case entry.BlockIndex == cropIndex && entry.Item == core.ItemWheat:
			hasWheat = true
			if entry.Count < 1 || entry.Count > 3 {
				t.Fatalf("小麦数量=%d，想要 [1,3]（全集=%+v）", entry.Count, memory.Drops)
			}
		case entry.BlockIndex == cropIndex && entry.Item == core.ItemWheatSeeds:
			hasSeeds = true
			if entry.Count < 1 || entry.Count > 3 {
				t.Fatalf("种子数量=%d，想要 [1,3]（全集=%+v）", entry.Count, memory.Drops)
			}
		default:
			t.Fatalf("意外掉落物 %+v，想要作物格上的小麦与种子", entry)
		}
	}
	if !hasWheat || !hasSeeds {
		t.Fatalf("最终掉落物=%+v，想要采掘同表的小麦与种子各一堆", memory.Drops)
	}
}

// recordFluidCropParity 在指定传输上跑一次「就绪 → 行走近农田 → 冲毁」并录下
// 方块变更与掉落物集合。
func recordFluidCropParity(t *testing.T, transport string) fluidCropParityRecord {
	t.Helper()
	// 关掉作物随机 tick（见文件头注释）：抽样数为 0 时 advanceCrops 直接返回，
	// 录像里只剩对时间平移不变的流体与移动事件。引擎每个 Step 入口从全局快照
	// 生效参数，因此这里在创建 host 之前设置即可覆盖本次录制的全部 tick。
	t.Cleanup(func() { sim.SetTunables(sim.DefaultTunables()) })
	tunables := sim.ActiveTunables()
	tunables.RandomTicksPerSection = 0
	sim.SetTunables(tunables)

	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 2, Seed: 42, SpawnDimension: core.Overworld,
	})
	config := hostTestConfig()
	config.ViewRadius = floodCropParityViewRadius
	// 关掉自动存盘：本测试只观察广播路径，存档行为由既有任务组覆盖。
	config.AutosaveTicks = 1 << 30
	host := mustNewHost(t, config, floodCropGenerator{}, store)
	identity := integrationIdentity(0x7c, "FluidCrop")
	endpoint, _, closeTransport := openParityTransport(t, host, transport, identity)
	defer func() {
		_ = endpoint.Close()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		_ = host.Shutdown(ctx)
		closeTransport()
	}()

	mirror := client.NewMirror()
	drops := client.NewItemDrops()
	record := fluidCropParityRecord{Ticks: make([][]string, 0, floodCropParityTicks)}
	ready := false
	readySteps := 0
	for !ready || !parityViewLoaded(mirror) ||
		!fluidCropFixtureLoaded(mirror) {
		_, messages := parityStep(t, host, endpoint, mirror)
		applyFluidCropParityMessages(t, drops, messages)
		for _, message := range messages {
			if state, ok := message.(network.PlayerState); ok && state.Ready {
				ready = true
			}
		}
		readySteps++
	}
	if readySteps > floodCropParityAlignTicks {
		t.Fatalf("%s 就绪耗时 %d tick 已超过对齐预算 %d：绝对 tick 对齐前提失效，"+
			"请上调 floodCropParityAlignTicks 而不是放宽比对", transport, readySteps,
			floodCropParityAlignTicks)
	}
	// 绝对 tick 对齐（见文件头注释）：空转到两侧共用的开录绝对 tick。空转期间
	// 世界静止、不产生值得关心的消息，但仍照常应用到镜像，保持与录制段同一套
	// 消费纪律。
	for step := readySteps; step < floodCropParityAlignTicks; step++ {
		_, messages := parityStep(t, host, endpoint, mirror)
		applyFluidCropParityMessages(t, drops, messages)
	}

	// 行走脚本：yaw=-π/2 让 forward=(1,0,0)（physics 的 forward=(-sin yaw,
	// 0,-cos yaw)），MoveZ=1 即沿 +X 前进。命令逐 tick 发送并等它进入权威队列，
	// 与采掘 parity 脚本同一节奏。**PlayerInput 是持续按住的意图状态**：行走段
	// 结束时必须显式发一条全零输入把意图清零，否则玩家会带着最后一次的 MoveZ=1
	// 一直走下去。
	sequence := uint64(1)
	for tick := 0; tick < floodCropParityTicks; tick++ {
		if tick < floodCropParityWalkTicks {
			sendIntegration(t, endpoint, network.PlayerInput{
				Sequence: sequence, MoveZ: 1,
				Yaw: -float32(math.Pi) / 2, Pitch: -0.2,
			})
			sequence++
			waitIntegrationCondition(t, fmt.Sprintf("%s walk %d queued", transport, tick),
				func() bool { return len(host.world.incoming) > 0 })
		} else if tick == floodCropParityWalkTicks {
			sendIntegration(t, endpoint, network.PlayerInput{Sequence: sequence})
			sequence++
			waitIntegrationCondition(t, fmt.Sprintf("%s halt queued", transport),
				func() bool { return len(host.world.incoming) > 0 })
		}
		_, messages := parityStep(t, host, endpoint, mirror)
		applyFluidCropParityMessages(t, drops, messages)
		tickChanges := make([]string, 0, 8)
		for _, message := range messages {
			changes, ok := message.(network.BlockChanges)
			if !ok {
				continue
			}
			for _, change := range changes.Changes {
				tickChanges = append(tickChanges, damParityEntry(changes, change))
				if change.Position == floodCropCell &&
					change.Block == core.WaterLevel1ID {
					record.Flood = true
				}
			}
		}
		record.Ticks = append(record.Ticks, tickChanges)
	}
	record.Drops = fluidCropDropProjection(drops)
	return record
}

// fluidCropFixtureLoaded 报告夹具区块是否已经出现在镜像里。就绪循环等它到位，
// 保证录制开始前夹具已加载——这是文件头注释里「移除异步生成时序」的落实点。
func fluidCropFixtureLoaded(mirror *client.Mirror) bool {
	_, ok := mirror.Chunk(core.Overworld, floodCropCell.Chunk())
	return ok
}

// applyFluidCropParityMessages 把掉落物镜像消息应用到 client.ItemDrops；
// 其余消息类型由 parityStep 内部的 mirror 应用负责。
func applyFluidCropParityMessages(
	t *testing.T,
	drops *client.ItemDrops,
	messages []network.ServerMessage,
) {
	t.Helper()
	for _, message := range messages {
		switch message.(type) {
		case network.ItemDropUpserts, network.ItemDropRemoves:
			if err := drops.Apply(message); err != nil {
				t.Fatalf("掉落镜像 Apply(%T): %v", message, err)
			}
		}
	}
}

// fluidCropFloodIndex 返回录像中首次出现作物格被写成流动水的 tick 下标；
// 未出现返回 -1。供失败诊断使用。
func fluidCropFloodIndex(record fluidCropParityRecord) int {
	for i, tick := range record.Ticks {
		for _, entry := range tick {
			var chunkX, chunkZ int32
			var base, next uint64
			var x, y, z int32
			var block uint16
			if _, err := fmt.Sscanf(
				entry, "chunk(%d,%d)@%d->%d %d,%d,%d=%d",
				&chunkX, &chunkZ, &base, &next, &x, &y, &z, &block,
			); err != nil {
				continue
			}
			hit := core.BlockPos{X: x, Y: y, Z: z}
			if hit == floodCropCell && core.BlockID(block) == core.WaterLevel1ID {
				return i
			}
		}
	}
	return -1
}

// fluidCropDropProjection 把掉落镜像压成按 (BlockIndex, Item) 稳定排序的语义
// 投影。Presentations 自身的排序键是 DropID（含槽位与 generation），压掉 ID
// 之后必须自己给出确定的全序，否则跨传输比对会把「同集合不同序」误判为不等。
func fluidCropDropProjection(drops *client.ItemDrops) []fluidCropDropEntry {
	presentations := drops.Presentations()
	projection := make([]fluidCropDropEntry, 0, len(presentations))
	for _, presentation := range presentations {
		projection = append(projection, fluidCropDropEntry{
			BlockIndex: presentation.BlockIndex,
			Item:       presentation.Item,
			Count:      presentation.Count,
		})
	}
	sort.Slice(projection, func(i, j int) bool {
		if projection[i].BlockIndex != projection[j].BlockIndex {
			return projection[i].BlockIndex < projection[j].BlockIndex
		}
		return projection[i].Item < projection[j].Item
	})
	return projection
}
