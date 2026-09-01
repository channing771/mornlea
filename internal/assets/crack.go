package assets

// crackStageCount 是采掘裂纹的阶段数，与呈现层的 10 级离散进度阶段一一对应
// （LayerCrack0..LayerCrack9 各占一层）。
const crackStageCount = 10

// crackNever 是「该像素永不属于裂纹」的哨兵出生阶段：大于任何合法阶段，
// 因此「出生阶段 ≤ 当前阶段」的比较天然把它排除在裂纹之外。
const crackNever = 255

// crackShades 是裂纹像素的原创配色：以近黑暖棕为裂缝主色，两档稍浅的暖棕
// 承担碎屑与次级裂纹的明暗层次（R≥G≥B，全部落在 0x10..0x38 的近黑深色域）。
// 参照同类体素游戏「裂纹读作方块表面的黑色阴影缝」的可观察样式：对比度
// 压足，裂纹在任何方块材质上都可辨认。全部取值是本仓原创像素，不复用也不
// 引入任何外部美术资源。
var crackShades = [3]rgb{
	{R: 22, G: 18, B: 16},
	{R: 34, G: 28, B: 23},
	{R: 48, G: 40, B: 32},
}

// 裂纹生成的噪声盐。与 procedural.go 既有的 hash2 确定性噪声习惯一致：
// 全部随机性都来自固定种子的整数散列，不引入 math/rand 全局源或时间，
// 同阶段重复生成因此逐字节一致。各盐互不相同，避免不同步骤的抖动同相。
const (
	crackPathSalt  = 0xC7B1
	crackTwigSalt  = 0xC7B2
	crackSpeckSalt = 0xC7B3
	crackPitSalt   = 0xC7B4
	crackThickSalt = 0xC7B5
	crackShadeSalt = 0xC7B6
	crackSpawnSalt = 0xC7B7
)

// crackDirTable 是 8 邻域方向表，索引即方向编号，±1（mod 8）即 ±45° 折转。
// 裂纹的像素风来自这些折线步进：1px 宽、逐像素 8 邻域连通、无平滑曲线、
// 无抗锯齿。
var crackDirTable = [8][2]int{
	{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1},
}

// crackMainPaths 是自主裂点放射而出的主裂缝条数：罗盘八向各一条（起点
// 方向 ±1 抖动、步进高频折角，相邻条不同相，避免长成对称星形）。八向
// 全开让裂纹网均匀铺满整个面，不偏向单一象限；路径长 10..12，从中心到
// 边约需 8..9 步，末段因此总能出边。
const crackMainPaths = 8

// crackBirthMap 计算 16×16 每个像素的「出生阶段」：像素首次进入裂纹的最早
// 阶段（0..9），crackNever 表示永不成为裂纹。
//
// 生长结构参照同类体素游戏 destroy 阶段的可观察节奏——裂纹不是「整条线
// 一次性出现」，而是**每条裂缝沿自身路径由内向外逐段显形**：
//  1. 阶段 0：中心 2×2 撞击点加两枚邻点，只构成一小簇初始裂口；
//  2. 阶段 1..9：六条锯齿状主裂缝各自向外延伸，第 step 步的出生阶段按
//     弧长线性铺满 1..9——早阶段每条只露出靠中心的 1..3 像素短刺，随阶段
//     推进逐渐伸长、折转、分叉，末段触及纹理边界（出界即断）；
//  3. 侧枝：每条主裂缝在弧长 1/3 与 2/3 处各折出一根 2..4 像素短枝，枝上
//     像素沿枝长顺延出生（基点出生 +1、+2…），裂纹网因此「先疏后密」；
//  4. 剥落碎屑：阶段 5 起沿既有裂缝边缘零星掉落单像素碎点（出生 5..9）；
//  5. 阶段 9：既有裂缝按约四成概率做单侧平行加粗（读作裂缝变宽），并在
//     中心邻域补三处 2×2 破损孔洞——末阶段是大面积碎裂网，仍保留透明
//     背景为主。
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
	// 阶段 0：中心撞击簇。2×2 核心 + 两枚对角裂点，读作「刚磕出的细裂口」。
	for _, point := range [][2]int{{7, 7}, {8, 7}, {7, 8}, {8, 8}, {6, 6}, {9, 9}} {
		mark(point[0], point[1], 0)
	}

	// 主裂缝：八条锯齿放射路径（罗盘八向各一），出生阶段按弧长线性铺满
	// 1..9。走线是「分段折角」模型：2..4 像素一小段直行，段尾一次 ±45°
	// （少数 ±90°）折角——同类体素游戏的裂缝读作干脆的折线而非蠕动曲线；
	// 偏移守卫（相对主方向的累计折角被钳回）保证路径持续向外推进、不在
	// 中心区打转成团。
	for walker := 0; walker < crackMainPaths; walker++ {
		x := 7 + int(hash2(uint32(walker), 11, crackPathSalt)%2)
		y := 7 + int(hash2(uint32(walker), 12, crackPathSalt)%2)
		base := (walker + 8) % 8
		dir := base
		length := 12 + int(hash2(uint32(walker), 14, crackPathSalt)%2)
		// 侧枝落点：弧长 1/3 处必有、2/3 处约半数路径才有（±1 抖动），
		// 基点出生阶段 +1 起顺延。
		spawnA := length/3 + int(hash2(uint32(walker), 15, crackSpawnSalt)%2)
		spawnB := length * 2 / 3
		if hash2(uint32(walker), 16, crackSpawnSalt)%2 != 0 {
			spawnB = length + 1
		}
		spawnB += int(hash2(uint32(walker), 17, crackSpawnSalt) % 2)
		segment := 3 + int(hash2(uint32(walker), 18, crackPathSalt)%2)
		deviation := 0
		for step := 1; step <= length; step++ {
			if step > 1 {
				segment--
				if segment <= 0 {
					// 段尾折角：多数 ±45°、少数 ±90°；折角方向由散列定，
					// 但累计偏移达到 ±2 时强制反向折回，路径不会卷成团。
					roll := hash2(uint32(walker), uint32(step), crackPathSalt)
					magnitude := 1
					if roll%7 == 0 {
						magnitude = 2
					}
					sign := 1
					if roll%2 == 0 {
						sign = -1
					}
					if deviation >= 2 {
						sign = -1
					} else if deviation <= -2 {
						sign = 1
					}
					deviation += sign * magnitude
					dir = (base + deviation + 8) % 8
					segment = 1 + int(hash2(uint32(walker), uint32(step+64), crackPathSalt)%3)
				}
			}
			x += crackDirTable[dir][0]
			y += crackDirTable[dir][1]
			if x < 0 || x >= texSize || y < 0 || y >= texSize {
				break
			}
			origin := uint8(1 + step*9/(length+1))
			if origin > 9 {
				origin = 9
			}
			mark(x, y, origin)
			// 侧枝：主裂缝方向 ±2（约垂直）折出，枝上像素逐像素顺延出生，
			// 与主裂缝「由内向外」的节奏一致，不会整枝突现。
			if step == spawnA || step == spawnB {
				twigDir := (dir + 2) % 8
				if hash2(uint32(walker), uint32(step), crackTwigSalt)%2 == 0 {
					twigDir = (dir + 6) % 8
				}
				twigLength := 2 + int(hash2(uint32(walker), uint32(step+1), crackTwigSalt)%2)
				tx, ty := x, y
				for twigStep := 1; twigStep <= twigLength; twigStep++ {
					if hash2(uint32(walker), uint32(step*8+twigStep), crackTwigSalt)%4 == 0 {
						twigDir = (twigDir + 1) % 8
					}
					tx += crackDirTable[twigDir][0]
					ty += crackDirTable[twigDir][1]
					if tx < 0 || tx >= texSize || ty < 0 || ty >= texSize {
						break
					}
					twigBirth := origin + uint8(twigStep)
					if twigBirth > 9 {
						twigBirth = 9
					}
					mark(tx, ty, twigBirth)
				}
			}
		}
	}
	// 剥落碎屑：沿早期裂缝（出生 ≤6 的像素）边缘零星掉落单像素，出生
	// 6..9 且概率压低（约九分之一）——碎屑只做「表面开始剥落」的点缀，
	// 密了会把裂缝的折线轮廓糊成噪点团。
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			if birth[y][x] > 6 {
				continue
			}
			roll := hash2(uint32(x), uint32(y), crackSpeckSalt)
			if roll%9 != 0 {
				continue
			}
			offset := crackDirTable[roll%8]
			sx, sy := x+offset[0], y+offset[1]
			if sx < 0 || sx >= texSize || sy < 0 || sy >= texSize {
				continue
			}
			if birth[sy][sx] != crackNever {
				continue
			}
			speckBirth := uint8(6 + roll%4)
			if speckBirth > 9 {
				speckBirth = 9
			}
			birth[sy][sx] = speckBirth
		}
	}
	// 阶段 9：平行加粗。按扫描序对出生 ≤8 的既有像素以约三分之一概率在
	// 其右/下单侧补一枚相邻像素——加粗只在已裂像素的一侧，读作裂缝变宽
	// 而非噪点扩散；扫描序保证新标记的像素不会被同轮再处理，加粗不连锁，
	// 末段（出生 9）不再加粗以免边界糊死。
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			if birth[y][x] == crackNever || birth[y][x] > 8 {
				continue
			}
			side := hash2(uint32(x), uint32(y), crackThickSalt)
			if side%6 != 0 {
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
	// 阶段 9：中心邻域三处 2×2 破损孔洞。锚点取盐选的既有裂纹像素（距中心
	// 切比雪夫距离 ≤4），整块出生在 9——末阶段「中心碎出一块」的观感。
	pits := 0
	for y := 0; y < texSize && pits < 3; y++ {
		for x := 0; x < texSize && pits < 3; x++ {
			if birth[y][x] == crackNever || birth[y][x] == 0 {
				continue
			}
			if dx, dy := x-8, y-8; dx < -4 || dx > 4 || dy < -4 || dy > 4 {
				continue
			}
			if hash2(uint32(x), uint32(y), crackPitSalt)%7 != 0 {
				continue
			}
			for _, offset := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				mark(x+offset[0], y+offset[1], 9)
			}
			pits++
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
// discard 呈现为原方块材质之上的透明叠加层。配色按「位置 + 出生阶段」在
// 三档明暗间循环：沿裂缝走向逐像素轮转出细碎的明暗颗粒（读作断面的
// 粗糙感），新裂开的边缘相位错开、稍浅于裂芯；确定性取值同时保证像素
// 最少的阶段 0 也用满三档。确定性：全部随机性来自固定种子的 hash2，
// 同阶段重复调用逐字节一致（守卫见 TestCrackTextureIsDeterministic）。
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
			shade := (x + y*2 + int(birth[y][x])) % len(crackShades)
			paint(px, x, y, crackShades[shade])
		}
	}
	return px
}
