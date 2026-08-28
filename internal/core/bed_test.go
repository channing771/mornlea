package core_test

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// TestBedBlockIDsAppendAfterTorches 锁定床方块的稳定编号：八个形态（床尾/床头
// × 四向）必须紧随 TorchWallNegZID（火把批次占 71..75）连续追加，顺序冻结为
// 76..79=床尾南/西/北/东、80..83=床头南/西/北/东。形态名中的方向与门先例
// （DoorDir 的南 0、西 1、北 2、东 3 编码）同序，表示床头相对床尾所指的方向。
// 编号是协议稳定值：插入或重排会平移后续编号，破坏既有存档与线上字节。
func TestBedBlockIDsAppendAfterTorches(t *testing.T) {
	ordered := []core.BlockID{
		core.BedFootSouthID,
		core.BedFootWestID,
		core.BedFootNorthID,
		core.BedFootEastID,
		core.BedHeadSouthID,
		core.BedHeadWestID,
		core.BedHeadNorthID,
		core.BedHeadEastID,
	}
	for i, id := range ordered {
		if want := core.TorchWallNegZID + 1 + core.BlockID(i); id != want {
			t.Fatalf("床形态 %d 的编号 = %d，想要紧随 TorchWallNegZID 之后的 %d", i, id, want)
		}
		if !core.RegisteredBlock(id) {
			t.Fatalf("床形态 %d 未注册", id)
		}
		if core.IsFluid(id) || core.IsCrop(id) || core.IsTorch(id) || core.IsDoor(id) {
			t.Fatalf("床形态 %d 被既有谓词误判", id)
		}
		if !core.IsBed(id) {
			t.Fatalf("床形态 %d 未被 IsBed 覆盖", id)
		}
		if name, ok := core.BlockDisplayName(id); !ok || name == "" {
			t.Fatalf("床形态 %d 没有显示名", id)
		}
	}
	if core.BedFootSouthID != 76 {
		t.Fatalf("BedFootSouthID = %d，必须稳定为 76", core.BedFootSouthID)
	}
	// 床头段与床尾段同序平移 4：同方向床头 = 床尾 + 4，供放置与采掘的双格
	// 配对用纯算术完成。
	for dir := 0; dir < 4; dir++ {
		if got, want := core.BedHeadID(dir), core.BedFootID(dir)+4; got != want {
			t.Fatalf("方向 %d 的床头编号 = %d，想要床尾编号 + 4 = %d", dir, got, want)
		}
	}
	if core.BlockIDMax != core.BedHeadEastID+1 {
		t.Fatalf("BlockIDMax = %d，必须紧随 BedHeadEastID(%d)",
			core.BlockIDMax, core.BedHeadEastID)
	}
	if core.BlockIDMax != 84 {
		t.Fatalf("BlockIDMax = %d，必须后移到 84", core.BlockIDMax)
	}
	// 形态编号两两不同：八个形态必须解析为八个不同的方块。
	seen := map[core.BlockID]bool{}
	for _, id := range ordered {
		if seen[id] {
			t.Fatalf("床形态编号 %d 重复", id)
		}
		seen[id] = true
	}
}

// TestBedPredicatesRejectNonBedBlocks 锁定床谓词的成员边界：IsBed/IsBedFoot/
// IsBedHead 只覆盖床的八个稳定编号，其余全部已注册方块（含未注册与越界编号）
// 一律 false。与 IsTorch/IsDoor 同一口径：属性判定不得在别处复制编号区间。
func TestBedPredicatesRejectNonBedBlocks(t *testing.T) {
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if core.IsBed(id) {
			continue
		}
		if core.IsBedFoot(id) || core.IsBedHead(id) {
			t.Fatalf("非床方块 %d 被床半边谓词误判", id)
		}
		if core.BedDir(id) != -1 {
			t.Fatalf("非床方块 %d 的 BedDir = %d，想要 -1", id, core.BedDir(id))
		}
	}
	for _, id := range []core.BlockID{core.BlockIDMax, core.BlockIDMax + 1, core.BlockID(65535)} {
		if core.IsBed(id) || core.IsBedFoot(id) || core.IsBedHead(id) {
			t.Fatalf("越界编号 %d 被床谓词误判", id)
		}
		if core.BedDir(id) != -1 {
			t.Fatalf("越界编号 %d 的 BedDir = %d，想要 -1", id, core.BedDir(id))
		}
	}
	// 两个半边谓词互斥且并起来恰是 IsBed。
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if core.IsBedFoot(id) == core.IsBedHead(id) {
			continue // 两者同为 false：非床方块
		}
		if !core.IsBed(id) {
			t.Fatalf("床半边方块 %d 未被 IsBed 覆盖", id)
		}
	}
}

// TestBedDirAndPlacementMapping 锁定「朝向 ↔ 编号」纯函数映射表：BedDir 把
// 八个形态解析为门先例同序的四向编码（南 0、西 1、北 2、东 3），BedFootID/
// BedHeadID 反向构造，BedHeadNeighbor 给出床头格相对床尾格的偏移
// （南 +Z、西 −X、北 −Z、东 +X，与门方向的坐标约定一致）。
func TestBedDirAndPlacementMapping(t *testing.T) {
	if core.BedDir(core.BedFootSouthID) != 0 || core.BedDir(core.BedHeadSouthID) != 0 {
		t.Fatal("南向床尾/床头的 BedDir 必须都是 0")
	}
	if core.BedDir(core.BedFootWestID) != 1 || core.BedDir(core.BedHeadWestID) != 1 {
		t.Fatal("西向床尾/床头的 BedDir 必须都是 1")
	}
	if core.BedDir(core.BedFootNorthID) != 2 || core.BedDir(core.BedHeadNorthID) != 2 {
		t.Fatal("北向床尾/床头的 BedDir 必须都是 2")
	}
	if core.BedDir(core.BedFootEastID) != 3 || core.BedDir(core.BedHeadEastID) != 3 {
		t.Fatal("东向床尾/床头的 BedDir 必须都是 3")
	}
	wantFoot := []core.BlockID{core.BedFootSouthID, core.BedFootWestID, core.BedFootNorthID, core.BedFootEastID}
	wantHead := []core.BlockID{core.BedHeadSouthID, core.BedHeadWestID, core.BedHeadNorthID, core.BedHeadEastID}
	wantOffset := []mgl32.Vec3{
		{0, 0, 1},  // 南：床头在 +Z 侧
		{-1, 0, 0}, // 西：床头在 −X 侧
		{0, 0, -1}, // 北：床头在 −Z 侧
		{1, 0, 0},  // 东：床头在 +X 侧
	}
	for dir := 0; dir < 4; dir++ {
		if got := core.BedFootID(dir); got != wantFoot[dir] {
			t.Fatalf("BedFootID(%d) = %d，想要 %d", dir, got, wantFoot[dir])
		}
		if got := core.BedHeadID(dir); got != wantHead[dir] {
			t.Fatalf("BedHeadID(%d) = %d，想要 %d", dir, got, wantHead[dir])
		}
		foot := core.BlockPos{X: 10, Y: 20, Z: 30}
		got := core.BedHeadNeighbor(foot, dir)
		want := core.BlockPos{
			X: foot.X + int32(wantOffset[dir].X()),
			Y: foot.Y + int32(wantOffset[dir].Y()),
			Z: foot.Z + int32(wantOffset[dir].Z()),
		}
		if got != want {
			t.Fatalf("BedHeadNeighbor(%v, %d) = %v，想要 %v", foot, dir, got, want)
		}
	}
	// 越界方向必须拒绝而不是回绕：放置执行方依赖非法 dir 的显式失败。
	for _, dir := range []int{-1, 4, 100} {
		if got := core.BedFootID(dir); got != core.AirID {
			t.Fatalf("BedFootID(%d) = %d，想要 AirID", dir, got)
		}
		if got := core.BedHeadID(dir); got != core.AirID {
			t.Fatalf("BedHeadID(%d) = %d，想要 AirID", dir, got)
		}
	}
}

// TestItemBedRegistration 锁定床物品的稳定语义：编号紧随腐肉物品（夜行者行
// 先合并占 45，床顺延为 46）、哨兵后移到 47、堆叠 64、无耐久、不是食物也不是
// 工具；放置映射按门先例给出默认形态（南向床尾），采掘映射把八个床形态全部
// 还原成恰好 1 个床物品。
func TestItemBedRegistration(t *testing.T) {
	if core.ItemBed != core.ItemRottenFlesh+1 {
		t.Fatalf("ItemBed = %d，必须紧随 ItemRottenFlesh(%d)", core.ItemBed, core.ItemRottenFlesh)
	}
	if core.ItemBed != 46 {
		t.Fatalf("ItemBed = %d，必须稳定为 46", core.ItemBed)
	}
	if core.ItemIDMax != 47 {
		t.Fatalf("ItemIDMax = %d，必须紧随 ItemBed(%d) 后移到 47", core.ItemIDMax, core.ItemBed)
	}
	if !core.RegisteredItem(core.ItemBed) {
		t.Fatal("ItemBed 未注册")
	}
	if limit, ok := core.ItemStackLimit(core.ItemBed); !ok || limit != core.MaxStackCount {
		t.Fatalf("ItemStackLimit(床) = (%d,%v)，想要 (%d,true)", limit, ok, core.MaxStackCount)
	}
	if _, hasDurability := core.ItemMaxDurability(core.ItemBed); hasDurability {
		t.Fatal("床不应该有耐久上限")
	}
	if _, broken := core.ItemBrokenForm(core.ItemBed); broken {
		t.Fatal("床不应该有损坏形态")
	}
	// 放置映射与门同形：单值窗口给出默认形态（南向床尾），双格原子写入由
	// 放置执行方按朝向展开。
	if block, ok := core.ItemPlacement(core.ItemBed); !ok || block != core.BedFootSouthID {
		t.Fatalf("ItemPlacement(床) = (%d, %v)，想要 (%d, true)", block, ok, core.BedFootSouthID)
	}
	// 采掘映射：八个形态任意一半都掉回恰好 1 个床物品（数量语义在掉落路径）。
	for _, id := range []core.BlockID{
		core.BedFootSouthID, core.BedFootWestID, core.BedFootNorthID, core.BedFootEastID,
		core.BedHeadSouthID, core.BedHeadWestID, core.BedHeadNorthID, core.BedHeadEastID,
	} {
		if item, ok := core.BlockDrop(id); !ok || item != core.ItemBed {
			t.Fatalf("BlockDrop(床形态 %d) = (%d, %v)，想要 (床物品, true)", id, item, ok)
		}
	}
}

// TestBedFormsAreRaycastTargets 锁定「半高碰撞不豁免选取」：床尾与床头都是
// 交互射线的合法命中目标（瞄准、采掘与入睡交互都靠它选格），八个形态逐一
// 断言。InteractionTarget 是全部交互调用点共用的唯一 solid 谓词。
func TestBedFormsAreRaycastTargets(t *testing.T) {
	for _, id := range []core.BlockID{
		core.BedFootSouthID, core.BedFootWestID, core.BedFootNorthID, core.BedFootEastID,
		core.BedHeadSouthID, core.BedHeadWestID, core.BedHeadNorthID, core.BedHeadEastID,
	} {
		if !core.InteractionTarget(id) {
			t.Fatalf("床形态 %d 不是交互射线目标", id)
		}
	}
}

// bedRaycastWorld 是床射线用例的最小世界：南向一张床（床尾在原点、床头在南侧
// 一格），其余格为空气。
func bedRaycastWorld() func(core.BlockPos) (bool, error) {
	world := map[core.BlockPos]core.BlockID{
		{X: 0, Y: 6, Z: 0}: core.BedFootSouthID,
		{X: 0, Y: 6, Z: 1}: core.BedHeadSouthID,
	}
	return func(pos core.BlockPos) (bool, error) {
		return core.InteractionTarget(world[pos]), nil
	}
}

// TestBedRaycastHitsFootAndHeadSeparately 覆盖 spec Requirement「床有半高碰撞
// 体且可被选取」的选取一半：指针射线必须能分别命中床尾格与床头格，而不是被
// 某一半挡死或整体漏掉。
func TestBedRaycastHitsFootAndHeadSeparately(t *testing.T) {
	solid := bedRaycastWorld()
	// 自上方竖直向下指向床尾格中心。
	downAtFoot := mgl32.Vec3{0.5, 8, 0.5}
	if hit, ok, err := core.RaycastBlocks(downAtFoot, mgl32.Vec3{0, -1, 0}, 4, solid); err != nil || !ok {
		t.Fatalf("床尾竖直射线未命中：ok=%v err=%v", ok, err)
	} else if hit.Block != (core.BlockPos{X: 0, Y: 6, Z: 0}) {
		t.Fatalf("床尾竖直射线命中 %v，想要床尾格", hit.Block)
	}
	// 自上方竖直向下指向床头格中心。
	downAtHead := mgl32.Vec3{0.5, 8, 1.5}
	if hit, ok, err := core.RaycastBlocks(downAtHead, mgl32.Vec3{0, -1, 0}, 4, solid); err != nil || !ok {
		t.Fatalf("床头竖直射线未命中：ok=%v err=%v", ok, err)
	} else if hit.Block != (core.BlockPos{X: 0, Y: 6, Z: 1}) {
		t.Fatalf("床头竖直射线命中 %v，想要床头格", hit.Block)
	}
	// 水平掠过床顶：先床尾后床头，两次投射分别落在这两格——证明两格都不是
	// 射线盲区。
	west := mgl32.Vec3{-2, 6.5, 0.5}
	hitFoot, okFoot, err := core.RaycastBlocks(west, mgl32.Vec3{1, 0, 0}, 8, solid)
	if err != nil || !okFoot || hitFoot.Block != (core.BlockPos{X: 0, Y: 6, Z: 0}) {
		t.Fatalf("水平射线首命中 = (%v, %v, %v)，想要床尾格", hitFoot.Block, okFoot, err)
	}
	east := mgl32.Vec3{2, 6.5, 1.5}
	hitHead, okHead, err := core.RaycastBlocks(east, mgl32.Vec3{-1, 0, 0}, 8, solid)
	if err != nil || !okHead || hitHead.Block != (core.BlockPos{X: 0, Y: 6, Z: 1}) {
		t.Fatalf("反向水平射线首命中 = (%v, %v, %v)，想要床头格", hitHead.Block, okHead, err)
	}
}

// TestBedRecipeShapeIsFrozen 锁定床配方的稳定语义：编号紧随 RecipeTorch（16），
// 形状为 3×3 顶排 3 小麦、中排空、下排 3 橡木木板，产物恰好 1 个床物品；镜像
// 位声明「形状自身水平镜像等价」——形状逐行左右对称，镜像位取真值时镜像摆放
// 与原摆放逐格相同（火把配方「镜像与自身相同」的同一声明方式）。
func TestBedRecipeShapeIsFrozen(t *testing.T) {
	if core.RecipeBed != core.RecipeTorch+1 {
		t.Fatalf("RecipeBed = %d，必须紧随 RecipeTorch(%d)", core.RecipeBed, core.RecipeTorch)
	}
	if core.RecipeBed != 16 {
		t.Fatalf("RecipeBed = %d，必须稳定为 16", core.RecipeBed)
	}
	pattern, ok := core.Recipe(core.RecipeBed)
	if !ok {
		t.Fatal("Recipe(RecipeBed) 未注册")
	}
	if pattern.Width != 3 || pattern.Height != 3 {
		t.Fatalf("床配方形状 = %dx%d，想要 3×3", pattern.Width, pattern.Height)
	}
	if !pattern.Mirror {
		t.Fatal("床配方必须声明「形状自身水平镜像等价」的镜像位")
	}
	want := [core.CraftingGridSlots]core.ItemID{
		core.ItemWheat, core.ItemWheat, core.ItemWheat,
		core.ItemNone, core.ItemNone, core.ItemNone,
		core.ItemOakPlanks, core.ItemOakPlanks, core.ItemOakPlanks,
	}
	if pattern.Cells != want {
		t.Fatalf("床配方格子 = %v，想要 %v", pattern.Cells, want)
	}
	if pattern.Output != (core.ItemStack{Item: core.ItemBed, Count: 1}) {
		t.Fatalf("床配方产物 = %+v，想要恰好 1 个床物品", pattern.Output)
	}
	// 形状自身的水平镜像与自身逐格相同：这是「镜像位取真值」的形状前提。
	for y := 0; y < int(pattern.Height); y++ {
		for x := 0; x < int(pattern.Width); x++ {
			if pattern.Cells[y*3+x] != pattern.Cells[y*3+int(pattern.Width)-1-x] {
				t.Fatalf("床配方第 %d 行不是左右对称的：镜像声明不成立", y)
			}
		}
	}
}

// TestBedRecipeMatchesAndDoesNotCrossMatchDoor 覆盖 delta spec Scenario「正确
// 摆放产出床」与「与门形状互不误配」：床配方在 3×3 网格按声明的三行摆放产出
// 恰好 1 个床物品；门配方的 2×3 两列木板摆放结果仍是门配方；床配方摆放绝不
// 匹配门配方。中排空是形状的一部分：把小麦行直接压在木板行上（裁边后 3×2）
// 必须无匹配——裁边只裁外围空行列，不吞掉形状内部的空行。
func TestBedRecipeMatchesAndDoesNotCrossMatchDoor(t *testing.T) {
	bedGrid := buildCraftingGrid(
		gridCell{0, core.ItemWheat}, gridCell{1, core.ItemWheat}, gridCell{2, core.ItemWheat},
		gridCell{6, core.ItemOakPlanks}, gridCell{7, core.ItemOakPlanks}, gridCell{8, core.ItemOakPlanks},
	)
	id, output, ok := core.MatchCraftingGrid(3, bedGrid)
	if !ok || id != core.RecipeBed {
		t.Fatalf("床配方摆放匹配 = (%d, %v)，想要床配方", id, ok)
	}
	if output != (core.ItemStack{Item: core.ItemBed, Count: 1}) {
		t.Fatalf("床配方产物 = %+v，想要恰好 1 个床物品", output)
	}

	doorGrid := buildCraftingGrid(
		gridCell{0, core.ItemOakPlanks}, gridCell{1, core.ItemOakPlanks},
		gridCell{3, core.ItemOakPlanks}, gridCell{4, core.ItemOakPlanks},
		gridCell{6, core.ItemOakPlanks}, gridCell{7, core.ItemOakPlanks},
	)
	id, output, ok = core.MatchCraftingGrid(3, doorGrid)
	if !ok || id != core.RecipeDoor {
		t.Fatalf("门配方摆放匹配 = (%d, %+v, %v)，想要门配方", id, output, ok)
	}
	if output.Item != core.ItemDoor {
		t.Fatalf("门配方产物 = %+v，想要木门", output)
	}

	squeezed := buildCraftingGrid(
		gridCell{0, core.ItemWheat}, gridCell{1, core.ItemWheat}, gridCell{2, core.ItemWheat},
		gridCell{3, core.ItemOakPlanks}, gridCell{4, core.ItemOakPlanks}, gridCell{5, core.ItemOakPlanks},
	)
	if id, _, ok := core.MatchCraftingGrid(3, squeezed); ok {
		t.Fatalf("压掉中排空行的 3×2 摆放匹配了配方 %d：内部空行必须保留", id)
	}

	// 个人 2×2 网格装不下任何 3 宽形状：床配方在 size=2 下必须无匹配。
	if _, _, ok := core.MatchCraftingGrid(2, bedGrid); ok {
		t.Fatal("床配方在个人 2×2 网格产生了匹配")
	}
}
