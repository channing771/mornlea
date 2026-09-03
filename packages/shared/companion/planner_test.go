package companion

import (
	"bytes"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/pathfind"
)

// testPlayerUUID 与 testCompanionUUID 是合法 UUIDv4 文本，仅测试使用。
const (
	testPlayerUUID    = "0f2a3b4c-5d6e-4f7a-8b9c-0d1e2f3a4b5c"
	testCompanionUUID = "1a2b3c4d-5e6f-4a7b-9c8d-0e1f2a3b4c5d"
	// testSecondPlayerUUID 是快照在线集合里另一名玩家的合法 UUIDv4 文本。
	testSecondPlayerUUID = "2b3c4d5e-6f70-4a81-9b2d-1f2a3b4c5d6e"
)

// testSnapshot 返回一份字段全部合法的观察快照，供各测试在其上做变异。
// 快照的权威构造（server 侧 tick 边界）属后续任务，这里只覆盖类型不变量。
// 快照携带两名在线玩家（发令玩家 + 另一名玩家，按 ID 升序），伙伴快捷栏持有
// oak_planks×3——两者共同支撑 follow/place 解码契约的合法与非法路径。
func testSnapshot() PlanSnapshot {
	issuer, err := core.ParsePlayerID(testPlayerUUID)
	if err != nil {
		panic(err)
	}
	companionID, err := ParseID(testCompanionUUID)
	if err != nil {
		panic(err)
	}
	secondPlayer, err := core.ParsePlayerID(testSecondPlayerUUID)
	if err != nil {
		panic(err)
	}
	issuerPlayer := PlanPlayer{
		ID:         issuer,
		Position:   [3]float32{8.5, 65, -1.5},
		Yaw:        0.25,
		Pitch:      -0.1,
		LookHit:    core.BlockPos{X: 9, Y: 64, Z: -1},
		HasLookHit: true,
	}
	terrain := NewTerrainProjection(core.BlockPos{X: -10, Y: 57, Z: -16})
	for x := int32(-10); x <= 22; x++ {
		for z := int32(-16); z <= 16; z++ {
			if !terrain.SetReadyColumn(x, z, 64) {
				panic("test terrain column outside projection")
			}
		}
	}
	if !terrain.SetBlock(core.BlockPos{X: 8, Y: 63, Z: -2}, core.GrassID) ||
		!terrain.SetBlock(core.BlockPos{X: 6, Y: 64, Z: 0}, core.StoneID) {
		panic("test terrain block outside projection")
	}
	return PlanSnapshot{
		Command: "去那棵橡树旁边",
		Issuer:  issuerPlayer,
		Companion: PlanCompanion{
			ID:       companionID,
			Position: [3]float32{6.5, 65, 0.5},
			Yaw:      3,
			Pitch:    0,
			Inventory: core.Inventory{Hotbar: core.Hotbar{Slots: [core.HotbarSlots]core.ItemStack{
				{Item: core.ItemOakPlanks, Count: 3},
			}}},
			TaskStatus: "空闲",
		},
		OnlinePlayers: []PlanPlayer{
			issuerPlayer,
			{ID: secondPlayer, Position: [3]float32{-3.5, 66, 12.25}, Yaw: 1.5, Pitch: 0},
		},
		ExposedBlocks: []PlanBlock{
			{Pos: core.BlockPos{X: 8, Y: 63, Z: -2}, Block: core.GrassID},
			{Pos: core.BlockPos{X: 9, Y: 63, Z: -2}, Block: core.StoneID},
			{Pos: core.BlockPos{X: 9, Y: 64, Z: -1}, Block: core.OakLogID},
		},
		Heights:        []PlanHeight{{X: 8, Z: -2, Height: 63}, {X: 9, Z: -1, Height: 64}},
		Terrain:        terrain,
		ChunkRevisions: []pathfind.ChunkRevision{{Chunk: core.ChunkPos{X: 0, Z: -1}, Revision: 7}},
		WorldTimeTicks: 6000,
	}
}

// TestPlanSnapshotValidateBounds 覆盖快照全部字段的边界：指令字节上限、环境方块
// 数量与顺序、高度样本数量与取值、revision 条数与顺序、任务状态摘要长度、身份
// 有效性与浮点有限性。
func TestPlanSnapshotValidateBounds(t *testing.T) {
	if err := testSnapshot().Validate(); err != nil {
		t.Fatalf("基准快照被拒绝: %v", err)
	}

	commandOver := strings.Repeat("走", 342) // 1026 bytes > 1024
	blocksOver := make([]PlanBlock, MaxPlanExposedBlocks+1)
	for index := range blocksOver {
		blocksOver[index] = PlanBlock{Pos: core.BlockPos{X: int32(index), Y: 0, Z: 0}, Block: core.StoneID}
	}
	// 高度样本按 33×33 上界多放一条。
	heightsOver := make([]PlanHeight, MaxPlanHeightSamples+1)
	for index := range heightsOver {
		heightsOver[index] = PlanHeight{X: int32(index % 33), Z: int32(index / 33), Height: 63}
	}
	revisionsOver := make([]pathfind.ChunkRevision, pathfind.MaxPlanChunkRevisions+1)
	for index := range revisionsOver {
		revisionsOver[index] = pathfind.ChunkRevision{Chunk: core.ChunkPos{X: int32(index - 5), Z: 0}, Revision: 1}
	}
	statusOver := strings.Repeat("忙", MaxPlanTaskStatusBytes/3+1)

	for name, mutate := range map[string]func(*PlanSnapshot){
		"指令为空":       func(s *PlanSnapshot) { s.Command = "" },
		"指令超长":       func(s *PlanSnapshot) { s.Command = commandOver },
		"指令非 UTF-8":  func(s *PlanSnapshot) { s.Command = "\xff\xfe" },
		"指令含控制字符":    func(s *PlanSnapshot) { s.Command = "走\x00" },
		"发令玩家 ID 无效": func(s *PlanSnapshot) { s.Issuer.ID = core.PlayerID{} },
		"伙伴 ID 无效":   func(s *PlanSnapshot) { s.Companion.ID = ID{} },
		"玩家位置 NaN":   func(s *PlanSnapshot) { s.Issuer.Position = [3]float32{float32(math.NaN()), 1, 1} },
		"伙伴位置 Inf":   func(s *PlanSnapshot) { s.Companion.Position = [3]float32{1, float32(math.Inf(1)), 1} },
		"玩家朝向 NaN":   func(s *PlanSnapshot) { s.Issuer.Yaw = float32(math.NaN()) },
		"环境方块超 256":  func(s *PlanSnapshot) { s.ExposedBlocks = blocksOver },
		"环境方块乱序": func(s *PlanSnapshot) {
			s.ExposedBlocks = []PlanBlock{s.ExposedBlocks[1], s.ExposedBlocks[0], s.ExposedBlocks[2]}
		},
		"环境方块坐标重复":     func(s *PlanSnapshot) { s.ExposedBlocks[1] = s.ExposedBlocks[0] },
		"环境方块为空气":      func(s *PlanSnapshot) { s.ExposedBlocks[0].Block = core.AirID },
		"环境方块未注册":      func(s *PlanSnapshot) { s.ExposedBlocks[0].Block = core.BlockID(9999) },
		"环境方块 Y 越界":    func(s *PlanSnapshot) { s.ExposedBlocks[0].Pos.Y = core.MaxY },
		"高度样本超上界":      func(s *PlanSnapshot) { s.Heights = heightsOver },
		"高度样本乱序":       func(s *PlanSnapshot) { s.Heights = []PlanHeight{{X: 9, Z: -1}, {X: 8, Z: -2}} },
		"高度样本 Y 越界":    func(s *PlanSnapshot) { s.Heights[0].Height = core.MaxY },
		"高度样本空列越界":     func(s *PlanSnapshot) { s.Heights[0].Height = core.MinY - 2 },
		"revision 超上界": func(s *PlanSnapshot) { s.ChunkRevisions = revisionsOver },
		"revision 乱序": func(s *PlanSnapshot) {
			s.ChunkRevisions = []pathfind.ChunkRevision{{Chunk: core.ChunkPos{X: 1, Z: 0}, Revision: 2}, {Chunk: core.ChunkPos{X: 0, Z: 0}, Revision: 1}}
		},
		"revision 坐标重复": func(s *PlanSnapshot) {
			s.ChunkRevisions = append(s.ChunkRevisions, pathfind.ChunkRevision{Chunk: core.ChunkPos{X: 0, Z: -1}, Revision: 9})
		},
		"任务状态摘要超长":      func(s *PlanSnapshot) { s.Companion.TaskStatus = statusOver },
		"任务状态摘要非 UTF-8": func(s *PlanSnapshot) { s.Companion.TaskStatus = "\xff" },
		"背包非法": func(s *PlanSnapshot) {
			s.Companion.Inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 0}
		},
	} {
		snapshot := testSnapshot()
		snapshot.ExposedBlocks = append([]PlanBlock(nil), snapshot.ExposedBlocks...)
		snapshot.Heights = append([]PlanHeight(nil), snapshot.Heights...)
		snapshot.ChunkRevisions = append([]pathfind.ChunkRevision(nil), snapshot.ChunkRevisions...)
		mutate(&snapshot)
		if err := snapshot.Validate(); err == nil {
			t.Errorf("%s 被接受", name)
		}
	}
}

// TestPlanSnapshotExposedBlocksBoundedSorted 验证环境摘要的排序与截断：超过
// 256 个方块时保留按坐标确定性排序的前 256 个，同一集合以不同输入顺序进入
// 得到相同结果（确定性），且源切片不被改动。
func TestPlanSnapshotExposedBlocksBoundedSorted(t *testing.T) {
	const total = MaxPlanExposedBlocks + 44
	blocks := make([]PlanBlock, 0, total)
	random := rand.New(rand.NewSource(7))
	for index := 0; index < total; index++ {
		blocks = append(blocks, PlanBlock{
			Pos: core.BlockPos{
				X: int32(random.Intn(33)) - 16,
				Y: core.MinY + int32(random.Intn(17)),
				Z: int32(random.Intn(33)) - 16,
			},
			Block: core.BlockID(1 + random.Intn(8)),
		})
	}
	source := append([]PlanBlock(nil), blocks...)

	bounded := BoundExposedBlocks(blocks)
	if len(bounded) != MaxPlanExposedBlocks {
		t.Fatalf("保留数量 = %d，want %d", len(bounded), MaxPlanExposedBlocks)
	}
	for index := 1; index < len(bounded); index++ {
		previous, current := bounded[index-1].Pos, bounded[index].Pos
		if previous.X > current.X ||
			(previous.X == current.X && previous.Y > current.Y) ||
			(previous.X == current.X && previous.Y == current.Y && previous.Z >= current.Z) {
			t.Fatalf("方块 %d 未按 (X,Y,Z) 严格升序: %+v 后跟 %+v", index-1, previous, current)
		}
	}

	shuffled := append([]PlanBlock(nil), blocks...)
	random.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	again := BoundExposedBlocks(shuffled)
	if len(again) != len(bounded) {
		t.Fatalf("两次保留数量不一致: %d vs %d", len(again), len(bounded))
	}
	for index := range bounded {
		if bounded[index] != again[index] {
			t.Fatalf("截断结果不确定：位置 %d 得到 %+v 与 %+v", index, bounded[index], again[index])
		}
	}

	// 源切片不被原地改动（调用方仍持有原始观察数据）。
	for index := range blocks {
		if blocks[index] != source[index] {
			t.Fatalf("输入切片被改动：位置 %d", index)
		}
	}
	snapshot := testSnapshot()
	snapshot.ExposedBlocks = bounded
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("截断后的快照应通过校验: %v", err)
	}
}

// testOnlinePlayer 构造第 index 名（0 起）在线玩家：ID 只靠首字节区分并固定
// UUIDv4 版本与变体位，天然互异且按 index 升序，供快照集合边界测试使用。
func testOnlinePlayer(index int) PlanPlayer {
	var id core.PlayerID
	id[0] = byte(index + 1)
	id[6] = 0x40 // UUIDv4 版本位。
	id[8] = 0x80 // RFC 4122 变体位。
	return PlanPlayer{
		ID:       id,
		Position: [3]float32{float32(index), 65.5, 0},
		Yaw:      0,
		Pitch:    0,
	}
}

// TestPlanSnapshotOnlinePlayers 覆盖快照在线玩家集合的边界：数量 ≤8、按 ID
// 严格升序去重、身份与浮点与视线命中合法性；以及 BoundOnlinePlayers 的有界
// 构造（任意输入顺序确定性归一、重复去重、>8 截断到 8、输入切片不被改动）。
func TestPlanSnapshotOnlinePlayers(t *testing.T) {
	snapshot := testSnapshot() // 基准快照携带两名已排序在线玩家。
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("基准快照被拒绝: %v", err)
	}

	nine := make([]PlanPlayer, MaxPlanOnlinePlayers+1)
	for index := range nine {
		nine[index] = testOnlinePlayer(index)
	}
	duplicated := []PlanPlayer{testOnlinePlayer(0), testOnlinePlayer(1), testOnlinePlayer(0)}
	shuffled := []PlanPlayer{testOnlinePlayer(2), testOnlinePlayer(0), testOnlinePlayer(3), testOnlinePlayer(1)}

	for name, mutate := range map[string]func(*PlanSnapshot){
		"玩家数超过 8":    func(s *PlanSnapshot) { s.OnlinePlayers = nine },
		"玩家未按 ID 排序": func(s *PlanSnapshot) { s.OnlinePlayers = shuffled },
		"玩家 ID 重复":   func(s *PlanSnapshot) { s.OnlinePlayers = duplicated },
		"玩家 ID 无效":   func(s *PlanSnapshot) { s.OnlinePlayers[0].ID = core.PlayerID{} },
		"玩家位置 NaN":   func(s *PlanSnapshot) { s.OnlinePlayers[0].Position = [3]float32{0, float32(math.NaN()), 0} },
		"玩家视线命中 Y 越界": func(s *PlanSnapshot) {
			s.OnlinePlayers[0].HasLookHit = true
			s.OnlinePlayers[0].LookHit = core.BlockPos{X: 0, Y: core.MaxY, Z: 0}
		},
	} {
		mutated := testSnapshot()
		mutated.OnlinePlayers = append([]PlanPlayer(nil), mutated.OnlinePlayers...)
		mutate(&mutated)
		if err := mutated.Validate(); err == nil {
			t.Errorf("%s 被接受", name)
		}
	}

	// 有界构造：12 条乱序含重复输入归一为按 ID 严格升序的前 8 名，确定性可复现。
	source := make([]PlanPlayer, 0, 12)
	for index := 11; index >= 0; index-- {
		source = append(source, testOnlinePlayer(index%10))
	}
	original := append([]PlanPlayer(nil), source...)
	bounded := BoundOnlinePlayers(source)
	if len(bounded) != MaxPlanOnlinePlayers {
		t.Fatalf("有界构造保留 %d 名，want %d", len(bounded), MaxPlanOnlinePlayers)
	}
	for index := 1; index < len(bounded); index++ {
		if bytes.Compare(bounded[index-1].ID[:], bounded[index].ID[:]) >= 0 {
			t.Fatalf("有界构造结果未按 ID 严格升序: 位置 %d", index)
		}
	}
	if again := BoundOnlinePlayers(original); len(again) != len(bounded) {
		t.Fatalf("同一集合两次构造数量不一致: %d vs %d", len(again), len(bounded))
	} else {
		for index := range bounded {
			if bounded[index] != again[index] {
				t.Fatalf("构造结果不确定：位置 %d 得到 %+v 与 %+v", index, bounded[index], again[index])
			}
		}
	}
	for index := range source {
		if source[index] != original[index] {
			t.Fatalf("输入切片被改动：位置 %d", index)
		}
	}
	// 重复 ID 输入被去重。
	if got := BoundOnlinePlayers(duplicated); len(got) != 2 {
		t.Fatalf("重复 ID 未去重: %d 名", len(got))
	}
	// 有界结果能通过完整快照校验。
	withBounded := testSnapshot()
	withBounded.OnlinePlayers = bounded
	if err := withBounded.Validate(); err != nil {
		t.Fatalf("有界构造结果被快照校验拒绝: %v", err)
	}
}

// TestPlanDecodePlaceRegistryLock 把 place 方块名注册表与 core.ItemPlacement
// 值域双向锁定：每个名字都能经注册物品放置出唯一方块（名字 ↔ 方块双射），
// 且可放置方块一个不漏，防止注册表与 core 放置映射漂移。
func TestPlanDecodePlaceRegistryLock(t *testing.T) {
	blocks := make(map[core.BlockID]string, len(planPlaceItems))
	for name, item := range planPlaceItems {
		block, ok := core.ItemPlacement(item)
		if !ok {
			t.Fatalf("注册表名字 %s 的物品 %d 不可放置", name, item)
		}
		if other, exists := blocks[block]; exists {
			t.Fatalf("方块 %d 同时映射到名字 %s 与 %s", block, other, name)
		}
		blocks[block] = name
	}
	// planPlaceExempt 是「玩家可放置、但伙伴注册表刻意不收」的物品。收进注册表
	// 就等于顺手给伙伴开了对应的世界交互权限，因此每一条豁免都必须写明理由，
	// 并由下面两条守卫钉住，不能靠"忘了加"而存在。
	planPlaceExempt := map[core.ItemID]string{
		core.ItemWheatSeeds: "伙伴种地属 M5 系列范围；变更 authoritative-farming " +
			"的非目标明确不交付伙伴农业（design.md 遗留清单 11）",
		core.ItemPotato: "伙伴种地属 M5 系列范围；新增马铃薯种植同样不交付",
		core.ItemCarrot: "伙伴种地属 M5 系列范围；新增胡萝卜种植同样不交付",
		core.ItemDoor:   "门为双格原子放置（lower+upper），伙伴 place 单格语义不覆盖；首版不交付伙伴放门",
		core.ItemBed:    "床为双格原子放置（床尾+床头），伙伴 place 单格语义不覆盖；首版不交付伙伴放床（与门同口径）",
	}
	for item, why := range planPlaceExempt {
		// 守卫一（防豁免空转）：被豁免的物品必须**真的**是玩家可放置物品。
		// 否则豁免条目在物品被改成不可放置之后会静默变成一句废话，而下面的
		// 穷举也不会再报警。
		if _, ok := core.ItemPlacement(item); !ok {
			t.Fatalf("豁免物品 %d 已不可放置，豁免条目失效：%s", item, why)
		}
		// 守卫二（防豁免与注册表同时成立）：豁免物品不得又出现在注册表里。
		for name, registered := range planPlaceItems {
			if registered == item {
				t.Fatalf("物品 %d 同时被豁免与登记为名字 %s：%s", item, name, why)
			}
		}
	}
	// 穷举界用 core.ItemIDMax 独占哨兵而不是「<= 枚举末项」：core 的枚举末项
	// 守护断言保证追加新物品时断言变红，迫使开发者同步审视本穷举的覆盖认知，
	// 而不是让本测试静默漏掉新物品。
	for item := core.ItemID(0); item < core.ItemIDMax; item++ {
		block, ok := core.ItemPlacement(item)
		if !ok {
			continue
		}
		if _, exempted := planPlaceExempt[item]; exempted {
			continue
		}
		if _, covered := blocks[block]; !covered {
			t.Fatalf("可放置方块 %d（物品 %d）缺少注册表名字", block, item)
		}
	}
}

// TestPlaceBlocksAndDropsRoundTrip 把 place 注册表的方块→物品索引与
// core.BlockDrop 掉落表交叉锁定：buildPlanPlaceBlocks() 的每个 (B, I) 都必须
// 满足 BlockDrop(B) == (I, true)。两张表独立维护——place 表是 place 步骤的
// 扣料依据，掉落表是 mine 步骤的产物依据——任何漂移都意味着「放置消耗物品 X、
// 采掘同一方块却产出物品 Y」的复制/丢失类缺陷，本测试让漂移立即失败。
func TestPlaceBlocksAndDropsRoundTrip(t *testing.T) {
	placed := buildPlanPlaceBlocks()
	if len(placed) == 0 {
		t.Fatal("place 方块→物品索引为空")
	}
	for block, item := range placed {
		drop, ok := core.BlockDrop(block)
		if !ok || drop != item {
			t.Fatalf("方块 %d 的放置物品 %d 与掉落物 (%d, %v) 不一致",
				block, item, drop, ok)
		}
	}
}
