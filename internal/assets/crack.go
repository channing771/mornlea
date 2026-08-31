package assets

// crackStageCount 是采掘裂纹的阶段数，与呈现层的 10 级离散进度阶段一一对应
// （LayerCrack0..LayerCrack9 各占一层）。
const crackStageCount = 10

// crackNever 是「该像素永不属于裂纹」的哨兵出生阶段：大于任何合法阶段，
// 因此「出生阶段 ≤ 当前阶段」的比较天然把它排除在裂纹之外。
const crackNever = 255

// crackShades 是裂纹像素的原创配色：三档暖深灰棕（R≥G≥B，全部落在
// 0x2a..0x4a 的深色域内），按像素散布做出裂纹内部的明暗层次。全部取值是
// 本仓原创像素，不复用也不引入任何外部美术资源。
var crackShades = [3]rgb{
	{R: 46, G: 43, B: 42},
	{R: 58, G: 52, B: 46},
	{R: 72, G: 64, B: 56},
}

// 裂纹生成的噪声盐。与 procedural.go 既有的 hash2 确定性噪声习惯一致：
// 全部随机性都来自固定种子的整数散列，不引入 math/rand 全局源或时间，
// 同阶段重复生成因此逐字节一致。各盐互不相同，避免不同步骤的抖动同相。
const (
	crackTrunkSalt   = 0xC7A1
	crackBranchSalt  = 0xC7A2
	crackThickenSalt = 0xC7A4
	crackShadeSalt   = 0xC7A5
)

// crackDirTable 是 8 邻域方向表，索引即方向编号，±1（mod 8）即 ±45° 折转。
// 裂纹的像素风来自这些折线步进：1px 宽、逐像素 8 邻域连通、无平滑曲线、
// 无抗锯齿。
var crackDirTable = [8][2]int{
	{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1},
}

// crackBirthMap 计算 16×16 每个像素的「出生阶段」：像素首次进入裂纹的最早
// 阶段（0..9），crackNever 表示永不成为裂纹。
//
// 生长结构（由内向外、后期加密加粗）：
//  1. 阶段 0：中心附近的少量初始裂点（撞击点）；
//  2. 阶段 1..6：8 条自中心向外的 1px 折线主干逐段延伸，第 step 步的像素
//     出生阶段即 step——主干按步数线性向外长；
//  3. 阶段 7：各主干中段的分枝（垂直折出，整枝一起出现）；
//  4. 阶段 8：各主干后段的短枝（与分枝错开方向）；
//  5. 阶段 9：按约半数概率对既有裂纹做单侧平行加粗（密集处目标位已被
//     裂纹占据，实际新增约为既有的四分之一），末阶段大面积破裂但仍以
//     透明背景为主。
//
// 增量生长由此**结构性成立**：阶段 i 的像素集合恰好是「出生阶段 ≤ i」的
// 全体像素，第 i-1 阶段的集合是其子集，不存在「后期移动或消失」的路径。
// 重复访问的像素取最早出生阶段，图案仍完全确定。
func crackBirthMap() [texSize][texSize]uint8 {
	var birth [texSize][texSize]uint8
	for row := range birth {
		for col := range birth[row] {
			birth[row][col] = crackNever
		}
	}
	mark := func(x, y int, stage uint8) {
		if x < 0 || x >= texSize || y < 0 || y >= texSize {
			return
		}
		if stage < birth[y][x] {
			birth[y][x] = stage
		}
	}
	// 阶段 0：中心附近少量初始裂点。
	for _, point := range [][2]int{{7, 7}, {8, 7}, {7, 8}, {8, 8}, {9, 7}} {
		mark(point[0], point[1], 0)
	}
	// 主干与分枝/短枝：8 条折线各自带独立抖动序列（折线索引进盐），
	// 互不同相，避免 8 条裂纹长成对称的星形。
	for walker := 0; walker < 8; walker++ {
		x, y := 7, 7
		dir := walker
		for step := 1; step <= 6; step++ {
			jitter := hash2(uint32(walker), uint32(step), crackTrunkSalt)
			if jitter%3 == 0 {
				dir = (dir + int(jitter/3)%2*2 - 1 + 8) % 8
			}
			x += crackDirTable[dir][0]
			y += crackDirTable[dir][1]
			if x < 0 || x >= texSize || y < 0 || y >= texSize {
				break
			}
			mark(x, y, uint8(step))
			if step == 3 {
				// 分枝：从中段垂直折出，3 像素，整枝出生在阶段 7。
				branchDir := (dir + 2) % 8
				if hash2(uint32(walker), 1, crackBranchSalt)%2 == 0 {
					branchDir = (dir + 6) % 8
				}
				bx, by := x, y
				for length := 0; length < 3; length++ {
					bJitter := hash2(uint32(walker), uint32(length), crackBranchSalt)
					if bJitter%4 == 0 {
						branchDir = (branchDir + int(bJitter/4)%2*2 - 1 + 8) % 8
					}
					bx += crackDirTable[branchDir][0]
					by += crackDirTable[branchDir][1]
					if bx < 0 || bx >= texSize || by < 0 || by >= texSize {
						break
					}
					mark(bx, by, 7)
				}
			}
			if step == 5 {
				// 短枝：后段向另一侧折出 2 像素，出生在阶段 8，与分枝
				// 错开方向，裂纹网在收尾前先加密一层。
				twigDir := (dir + 6) % 8
				if hash2(uint32(walker), 2, crackBranchSalt)%2 == 0 {
					twigDir = (dir + 2) % 8
				}
				tx, ty := x, y
				for length := 0; length < 2; length++ {
					tJitter := hash2(uint32(walker), uint32(length+4), crackBranchSalt)
					if tJitter%4 == 0 {
						twigDir = (twigDir + int(tJitter/4)%2*2 - 1 + 8) % 8
					}
					tx += crackDirTable[twigDir][0]
					ty += crackDirTable[twigDir][1]
					if tx < 0 || tx >= texSize || ty < 0 || ty >= texSize {
						break
					}
					mark(tx, ty, 8)
				}
			}
		}
	}
	// 阶段 9：平行加粗。按扫描序对既有裂纹像素以约半数概率在其右/下单侧
	// 补一枚相邻像素——加粗只在已裂像素的一侧，读作裂缝变宽而非噪点扩散；
	// 扫描序保证新标记的像素不会被同轮再处理，加粗不连锁。
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			if birth[y][x] > 8 {
				continue
			}
			side := hash2(uint32(x), uint32(y), crackThickenSalt)
			if side%2 != 0 {
				continue
			}
			ox, oy := x+1, y
			if side%4 >= 2 {
				ox, oy = x, y+1
			}
			if ox < texSize && oy < texSize && birth[oy][ox] == crackNever {
				birth[oy][ox] = 9
			}
		}
	}
	return birth
}

// crackTexture 生成采掘裂纹第 stage（0..9）阶段的 cutout 材质：16×16 RGBA、
// 背景全透明、裂纹像素不透明的原创程序化像素。
//
// 像素集合由 crackBirthMap 的出生阶段决定（≤ stage 即呈现），因此各阶段
// 严格增量生长；alpha 二值（0/255）与 isCutoutLayer 的 cutout 分类配套，
// mip 链由 downsampleCutout 保住稀疏裂纹的覆盖率。渲染侧按 `alpha < 0.5`
// discard 呈现为原方块材质之上的透明叠加层。确定性：全部随机性来自固定
// 种子的 hash2，同阶段重复调用逐字节一致（守卫见 TestCrackTextureIsDeterministic）。
func crackTexture(stage int) []byte {
	if stage < 0 {
		stage = 0
	}
	if stage >= crackStageCount {
		stage = crackStageCount - 1
	}
	px := make([]byte, texSize*texSize*4)
	birth := crackBirthMap()
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			if birth[y][x] > uint8(stage) {
				continue
			}
			paint(px, x, y, crackShades[hash2(uint32(x), uint32(y), crackShadeSalt)%uint32(len(crackShades))])
		}
	}
	return px
}
