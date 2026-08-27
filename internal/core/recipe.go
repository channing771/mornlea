package core

// RecipeID 是稳定的配方编号；0 保留为无效值。
type RecipeID uint8

const (
	// RecipeStoneBricks 用 2×2 石头合成 4 个石砖。
	RecipeStoneBricks RecipeID = iota + 1
	// RecipeFurnace 用 3×3 圆石圆环（中格为空）合成 1 个熔炉。
	RecipeFurnace
	// RecipeIronBlock 用 3×3 铁锭合成 1 个铁块。
	RecipeIronBlock
	// RecipeStonePickaxe 顶排 3 个石头加中列 2 根木棍合成 1 把石镐。
	RecipeStonePickaxe
	// RecipeIronPickaxe 顶排 3 个铁锭加中列 2 根木棍合成 1 把铁镐。
	RecipeIronPickaxe
	// RecipeChest 用 3×3 橡木木板圆环（中格为空）合成 1 个箱子。
	RecipeChest
	// RecipeOakPlanks 用 1 个橡木原木合成 4 个橡木木板。
	RecipeOakPlanks
	// RecipeLightBlock 用 2×2 玻璃合成 4 个发光方块。
	RecipeLightBlock
	// RecipeStoneHoe 石头纵列旁接木棍纵列（2×2）合成 1 把石锄。
	// 锄头比同材质的镐少用一份矿材（2 对 3）：锄头只作用于翻地这一件事，
	// 产出也不是资源而是地块状态，与镐同价会让第一块耕地的门槛高得没有道理。
	RecipeStoneHoe
	// RecipeIronHoe 铁锭纵列旁接木棍纵列（2×2）合成 1 把铁锄。
	RecipeIronHoe
	// RecipeBread 横排 3 个小麦合成 1 个面包。
	//
	// 它是农业闭环的出口：小麦除了合成面包没有任何用途，这条配方是「种地」
	// 与「吃饭」之间唯一的通路。3 换 1 与参考实现同值。
	RecipeBread
	// RecipeStick 纵向 2 块橡木木板合成 4 根木棍。
	//
	// 它是全部工具配方的前置：镐与锄的中列木棍都从这里出发，先有木棍
	// 才谈得上任何工具。
	RecipeStick
	// RecipeWorkbench 2×2 橡木木板合成 1 个工作台。
	//
	// 工作台把玩家合成网格的有效尺寸从 2 提到 3，是形状合成的自举配方：
	// 2×2 网格摆得出的最大形状就是它自己。
	RecipeWorkbench
	// RecipeDoor 用两列满的 6 个橡木木板（2×3）合成 3 个木门。
	RecipeDoor
	// RecipeTorch 纵向 2 格、煤炭位于木棍正上方，合成 4 个火把。
	//
	// 它是夜间照明的前置：1 煤 + 1 木棍换 4 枚火把，原料走采掘与伐木两条既有
	// 线。形状宽 1 高 2，恰好放进 2×2 个人网格（无需先造工作台），镜像与自身
	// 相同；倒置（木棍在煤炭上方）是垂直翻转，永不匹配。
	RecipeTorch
)

// Recipe 返回 id 的固定形状配方；未知 ID 返回 false。
// recipe ID 只用于注册表与 UI 身份：新路径的线上消息不携带配方编号，
// 配方编号也不落盘（见 spec authoritative-crafting）。
func Recipe(id RecipeID) (RecipePattern, bool) {
	return recipePattern(id)
}

// CraftingGridSlots 是合成网格的统一格数：个人 2×2 与工作台 3×3 共用同一份
// 9 格存储（个人网格只使用格 0..3），协议侧的网格状态消息（本批次任务组 3
// 落地）以此 9 格为固定编码上界。网格状态本身归属 sim 与网络镜像
// （见 design.md D2），core 只定义格数与形状匹配。
const CraftingGridSlots = 9

// RecipePattern 是一条裁边后的固定形状配方。
//
// Cells 恒按 3×3 行主序存储（下标 = y*3+x），形状只占据左上角 Width×Height
// 子矩形、其余格必须保持 `ItemNone`——即使配方宽不足 3 也用 stride 3，让
// 「格 4 恒为中格」这类推理对全部配方成立。Width/Height 是裁边后的值：外围
// 空行列已经裁掉，内部空洞保留在 Cells 里。
//
// Mirror 声明该配方是否接受水平镜像摆放（design.md D3）：工具类配方关闭，
// 避免同一形状出现左右手双解；对称形状的镜像与自身相同，位值不影响结果。
// 垂直翻转与旋转永远不在匹配语义里。
type RecipePattern struct {
	Width, Height uint8
	Cells         [CraftingGridSlots]ItemID
	Output        ItemStack
	Mirror        bool
}

// recipePattern 返回 id 的格子形状，是形状匹配的唯一数据源。
// 未知 ID 返回 false；本表按编号 append-only 推进，绝不重排。
func recipePattern(id RecipeID) (RecipePattern, bool) {
	switch id {
	case RecipeStoneBricks:
		return RecipePattern{
			Width: 2, Height: 2, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemStone, ItemStone, ItemNone,
				ItemStone, ItemStone, ItemNone,
			},
			Output: ItemStack{Item: ItemStoneBrick, Count: 4},
		}, true
	case RecipeFurnace:
		return RecipePattern{
			Width: 3, Height: 3, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemCobblestone, ItemCobblestone, ItemCobblestone,
				ItemCobblestone, ItemNone, ItemCobblestone,
				ItemCobblestone, ItemCobblestone, ItemCobblestone,
			},
			Output: ItemStack{Item: ItemFurnace, Count: 1},
		}, true
	case RecipeIronBlock:
		return RecipePattern{
			Width: 3, Height: 3, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemIronIngot, ItemIronIngot, ItemIronIngot,
				ItemIronIngot, ItemIronIngot, ItemIronIngot,
				ItemIronIngot, ItemIronIngot, ItemIronIngot,
			},
			Output: ItemStack{Item: ItemIronBlock, Count: 1},
		}, true
	case RecipeStonePickaxe:
		return RecipePattern{
			Width: 3, Height: 3, Mirror: false,
			Cells: [CraftingGridSlots]ItemID{
				ItemStone, ItemStone, ItemStone,
				ItemNone, ItemStick, ItemNone,
				ItemNone, ItemStick, ItemNone,
			},
			Output: ItemStack{Item: ItemStonePickaxe, Count: 1, Durability: 131},
		}, true
	case RecipeIronPickaxe:
		return RecipePattern{
			Width: 3, Height: 3, Mirror: false,
			Cells: [CraftingGridSlots]ItemID{
				ItemIronIngot, ItemIronIngot, ItemIronIngot,
				ItemNone, ItemStick, ItemNone,
				ItemNone, ItemStick, ItemNone,
			},
			Output: ItemStack{Item: ItemIronPickaxe, Count: 1, Durability: 250},
		}, true
	case RecipeChest:
		return RecipePattern{
			Width: 3, Height: 3, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemOakPlanks, ItemOakPlanks, ItemOakPlanks,
				ItemOakPlanks, ItemNone, ItemOakPlanks,
				ItemOakPlanks, ItemOakPlanks, ItemOakPlanks,
			},
			Output: ItemStack{Item: ItemChest, Count: 1},
		}, true
	case RecipeOakPlanks:
		return RecipePattern{
			Width: 1, Height: 1, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemOakLog,
			},
			Output: ItemStack{Item: ItemOakPlanks, Count: 4},
		}, true
	case RecipeLightBlock:
		return RecipePattern{
			Width: 2, Height: 2, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemGlass, ItemGlass, ItemNone,
				ItemGlass, ItemGlass, ItemNone,
			},
			Output: ItemStack{Item: ItemLightBlock, Count: 4},
		}, true
	// 锄头是「材料纵列旁接木棍纵列」的 2×2：材料列在左（x=0）。镜像位按
	// design.md D3 关闭，镜像摆放不匹配。
	case RecipeStoneHoe:
		return RecipePattern{
			Width: 2, Height: 2, Mirror: false,
			Cells: [CraftingGridSlots]ItemID{
				ItemStone, ItemStick, ItemNone,
				ItemStone, ItemStick, ItemNone,
			},
			Output: ItemStack{Item: ItemStoneHoe, Count: 1, Durability: 131},
		}, true
	case RecipeIronHoe:
		return RecipePattern{
			Width: 2, Height: 2, Mirror: false,
			Cells: [CraftingGridSlots]ItemID{
				ItemIronIngot, ItemStick, ItemNone,
				ItemIronIngot, ItemStick, ItemNone,
			},
			Output: ItemStack{Item: ItemIronHoe, Count: 1, Durability: 250},
		}, true
	case RecipeBread:
		return RecipePattern{
			Width: 3, Height: 1, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemWheat, ItemWheat, ItemWheat,
			},
			Output: ItemStack{Item: ItemBread, Count: 1},
		}, true
	// 木棍：纵向两块木板（1×2），全部工具配方的前置。
	case RecipeStick:
		return RecipePattern{
			Width: 1, Height: 2, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemOakPlanks, ItemNone, ItemNone,
				ItemOakPlanks, ItemNone, ItemNone,
			},
			Output: ItemStack{Item: ItemStick, Count: 4},
		}, true
	// 工作台：2×2 木板——个人网格摆得出的最大形状，形状合成的自举配方。
	case RecipeWorkbench:
		return RecipePattern{
			Width: 2, Height: 2, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemOakPlanks, ItemOakPlanks, ItemNone,
				ItemOakPlanks, ItemOakPlanks, ItemNone,
			},
			Output: ItemStack{Item: ItemWorkbench, Count: 1},
		}, true
	// 木门：2×3 两列满木板合成 3 个木门。
	case RecipeDoor:
		return RecipePattern{
			Width: 2, Height: 3, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemOakPlanks, ItemOakPlanks, ItemNone,
				ItemOakPlanks, ItemOakPlanks, ItemNone,
				ItemOakPlanks, ItemOakPlanks, ItemNone,
			},
			Output: ItemStack{Item: ItemDoor, Count: 3},
		}, true
	// 火把：煤炭位于木棍正上方的纵向两格（1×2），产出 4 个火把。
	case RecipeTorch:
		return RecipePattern{
			Width: 1, Height: 2, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemCoal, ItemNone, ItemNone,
				ItemStick, ItemNone, ItemNone,
			},
			Output: ItemStack{Item: ItemTorch, Count: 4},
		}, true
	default:
		return RecipePattern{}, false
	}
}

// MatchCraftingGrid 在固定配方注册表中匹配一份合成网格，返回命中的配方、
// 其完整产物与是否命中。
//
// 匹配语义（spec authoritative-grid-crafting）：先按网格的有效尺寸裁掉外围
// 空行列（同一形状的摆放位置不影响结果、内部空洞保留），再与每条配方逐格
// 比较；配方开 `Mirror` 位时允许水平镜像重试一次，垂直翻转与旋转永不匹配。
// 额外物品会把裁边包围盒撑大、宽高不再相等，自然失配。
//
// size 是网格的有效尺寸（个人 2 或工作台 3），决定格内布局的行主序 stride：
// 个人网格只用格 0..3 且按 2×2 解释，工作台按 3×3 解释。size 不是 2 或 3、
// 或有效尺寸之外的格（个人网格的格 4..8）残留物品时，一律判定无匹配——
// 正常权威路径不会构造出这两种输入，这里是防御层。
//
// 实现是固定 15 条 × 至多 9 格的纯值循环，无 map/slice 分配，不建通用矩阵包
// （design.md D3）。
func MatchCraftingGrid(size uint8, slots [CraftingGridSlots]ItemStack) (RecipeID, ItemStack, bool) {
	if size != 2 && size != 3 {
		return 0, ItemStack{}, false
	}
	var cells [CraftingGridSlots]ItemID
	for i := uint8(0); i < CraftingGridSlots; i++ {
		// 数量为零的栈按空格处理：匹配只关心「物品占位」，不关心数量
		//（数量语义在取出消费那一层）。
		if slots[i].Count > 0 {
			cells[i] = slots[i].Item
		}
		// 有效尺寸之外的格必须为空：个人网格的格 4..8 由权威移动路径保证为空，
		//这里拒绝任何残留，避免缩小后的网格靠旧内容继续匹配。
		if i >= size*size && cells[i] != ItemNone {
			return 0, ItemStack{}, false
		}
	}
	originX, originY, width, height, ok := trimPattern(size, cells)
	if !ok {
		return 0, ItemStack{}, false
	}
	for id := RecipeStoneBricks; id <= RecipeTorch; id++ {
		pattern, registered := recipePattern(id)
		if !registered {
			continue
		}
		if pattern.Width != width || pattern.Height != height {
			continue
		}
		if matchesPattern(size, cells, pattern, originX, originY, false) ||
			(pattern.Mirror && matchesPattern(size, cells, pattern, originX, originY, true)) {
			return id, pattern.Output, true
		}
	}
	return 0, ItemStack{}, false
}

// trimPattern 在给定有效尺寸的网格里找非空格的裁边包围盒，返回包围盒左上角
// 原点与其宽高；全空网格返回 ok=false。行主序 stride 由 size 决定（2 或 3），
// 因此个人网格的格 0..3 会被解释成 2×2 而不是 3×3 的 L 形。
func trimPattern(size uint8, cells [CraftingGridSlots]ItemID) (originX, originY, width, height uint8, ok bool) {
	minX, minY := uint8(255), uint8(255)
	maxX, maxY := uint8(0), uint8(0)
	found := false
	for y := uint8(0); y < size; y++ {
		for x := uint8(0); x < size; x++ {
			if cells[y*size+x] == ItemNone {
				continue
			}
			found = true
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
	}
	if !found {
		return 0, 0, 0, 0, false
	}
	return minX, minY, maxX - minX + 1, maxY - minY + 1, true
}

// matchesPattern 把已裁边的网格（cells 及其包围盒原点）与一条形状逐格比较。
// mirror=true 时按水平镜像比较形状的列（pattern 的 x 列反序）；调用方保证
// 网格包围盒与形状宽高一致，因此本函数只需比较 Width×Height 块内的每一格，
// 包围盒之外的格既不可能非空（裁边性质）也不属于形状。
func matchesPattern(size uint8, cells [CraftingGridSlots]ItemID, pattern RecipePattern, originX, originY uint8, mirror bool) bool {
	for y := uint8(0); y < pattern.Height; y++ {
		for x := uint8(0); x < pattern.Width; x++ {
			patternX := x
			if mirror {
				patternX = pattern.Width - 1 - x
			}
			if pattern.Cells[y*3+patternX] != cells[(originY+y)*size+originX+x] {
				return false
			}
		}
	}
	return true
}
