package capture

// capture_grass_closeup_test.go：短草近景场景的钉死回归。`prepareGrassCloseup`
// 装入手工夹具（空气邻域基线上一条草地支撑条，条上 3 列短草），
// `applyGrassCloseupCaptureState` 钉死正午与近景机位；本文件只断言夹具数据面
// （短草列全立于草地正上方）与场景内像素可辨识性。golden 基线由后续任务承接，
// 这里只做活帧差分，不比较任何 golden 文件。

import (
	"image"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
)

// grassCloseupShortGrassCells 是冻结的 3 列短草坐标（见 `prepareGrassCloseup`），
// 测试侧只读，不重新推导分布。
var grassCloseupShortGrassCells = []core.BlockPos{
	{X: -1, Y: 1, Z: -3},
	{X: 0, Y: 1, Z: -3},
	{X: 1, Y: 1, Z: -2},
}

// grassCloseupIdentifiableDiffPixels 是近景短草可辨识的差分下限：机位距短草列
// 约 4 格，交叉面片在画面里是数十像素量级；150px 远高于同机重复抓帧的个位数
// LSB 漂移（`oakGroveGrassPixelDelta` 的 8 倍余量口径），误报只能来自短草本身。
const grassCloseupIdentifiableDiffPixels = 150

// TestGrassCloseupFixtureStandsOnGrassSupport 钉住夹具的数据面：支撑条
// x=-2..2、z=-4..-2（y=0）整片是草地，短草不多不少就是冻结的 3 列，且每列正
// 下方紧贴草地（短草没有自己的碰撞与光照，世界生成的同款不变式）。画面层的
// 可辨识性由 TestGrassCloseupSceneShowsIdentifiableShortGrass 承担。
func TestGrassCloseupFixtureStandsOnGrassSupport(t *testing.T) {
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	if err := prepareGrassCloseup(app); err != nil {
		t.Fatalf("准备短草近景夹具: %v", err)
	}
	for z := int32(-4); z <= -2; z++ {
		for x := int32(-2); x <= 2; x++ {
			got, loaded := app.Mirror().BlockAt(core.Overworld, core.BlockPos{X: x, Y: 0, Z: z})
			if !loaded || got != core.GrassID {
				t.Fatalf("支撑条 (%d,0,%d)=%d/%v，想要 GrassID/true", x, z, got, loaded)
			}
		}
	}
	// 邻域扫描：空气快照覆盖区块 -1..1（算术右移取整，x/z ∈ [-16,31]），夹具
	// 只写 y=0..1；该窗口内短草有且仅有冻结的 3 列，多一列少一列都是错。
	want := make(map[core.BlockPos]struct{}, len(grassCloseupShortGrassCells))
	for _, cell := range grassCloseupShortGrassCells {
		want[cell] = struct{}{}
	}
	var grasses []core.BlockPos
	for z := int32(-16); z <= 31; z++ {
		for x := int32(-16); x <= 31; x++ {
			for y := int32(0); y <= 2; y++ {
				position := core.BlockPos{X: x, Y: y, Z: z}
				block, loaded := app.Mirror().BlockAt(core.Overworld, position)
				if !loaded {
					t.Fatalf("短草近景 mirror 未加载 %+v", position)
				}
				if block == core.ShortGrassID {
					grasses = append(grasses, position)
				}
			}
		}
	}
	if len(grasses) != len(grassCloseupShortGrassCells) {
		t.Fatalf("短草列数=%d，想要 %d（%v）", len(grasses), len(grassCloseupShortGrassCells), grasses)
	}
	for _, cell := range grasses {
		if _, ok := want[cell]; !ok {
			t.Fatalf("短草 %+v 不在冻结坐标 %v 内", cell, grassCloseupShortGrassCells)
		}
		support, loaded := app.Mirror().BlockAt(core.Overworld,
			core.BlockPos{X: cell.X, Y: cell.Y - 1, Z: cell.Z})
		if !loaded || support != core.GrassID {
			t.Fatalf("短草 %+v 的支撑格 = %d/%v，想要 GrassID/true（短草只应立在草地正上方）",
				cell, support, loaded)
		}
	}
}

// prepareGrassCloseupWithoutShortGrass 装入与 `prepareGrassCloseup` 相同的草地
// 支撑条，唯独不摆 3 列短草。差分夹具与生产夹具的唯一差别因此就是短草本身——
// 两张图的差分即短草的视觉足迹，可直接逐格归因。支撑条循环与生产夹具逐字一致
// （不得顺手重构生产夹具），差别只在省略短草三行。
func prepareGrassCloseupWithoutShortGrass(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	setBlock := func(position core.BlockPos, block core.BlockID) {
		chunk := position.Chunk()
		if blocks[chunk] == nil {
			blocks[chunk] = make(map[core.BlockPos]core.BlockID)
		}
		blocks[chunk][position] = block
	}
	for z := int32(-4); z <= -2; z++ {
		for x := int32(-2); x <= 2; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.GrassID)
		}
	}
	return applyCaptureBlocks(app, blocks, 1, "短草近景差分")
}

// TestGrassCloseupSceneShowsIdentifiableShortGrass 是短草近景的场景内像素断言：
// 经完整无窗口链路（离屏 renderer、预热、装夹具、收敛、回读）抓两次帧——一次
// 生产夹具、一次仅剔除短草的差分夹具——短草的视觉足迹就是两图差分。可辨识的
// 判据沿用 oak-grove 的四判据形状（`oakGroveCellDiff` 口径），唯独可见量下限
// 提到近景量级的 150px：
//   - 至少一株短草格的屏幕外接矩形内差分像素足够多（在画面里可见，而不是
//     远成 1~2 个噪声像素）；
//   - 该矩形内差分不铺满（alpha cutout 透过背景，不是不透明矩形）；
//   - 矩形顶部带几乎没有差分（叶片不达纹理顶部，上缘透空，不是实心立方体）；
//   - 矩形底部带有差分（叶片从贴地一侧长出，交叉斜面真实渲染）。
func TestGrassCloseupSceneShowsIdentifiableShortGrass(t *testing.T) {
	app := newCaptureSceneRenderApplication(t)
	if app.Window() != nil {
		t.Fatal("短草近景像素断言必须走无窗口离屏链路，当前存在交互窗口")
	}
	scene := captureSceneByName(t, "grass-closeup")
	withGrass, err := captureSceneImage(app, scene)
	if err != nil {
		t.Fatalf("抓取 grass-closeup: %v", err)
	}
	camera := *app.Camera()

	bareApp := newCaptureSceneRenderApplication(t)
	grassless := captureScene{
		Name:         "grass-closeup-grassless",
		WarmupFrames: scene.WarmupFrames,
		Prepare:      prepareGrassCloseupWithoutShortGrass,
		Apply:        applyGrassCloseupCaptureState,
	}
	withoutGrass, err := captureSceneImage(bareApp, grassless)
	if err != nil {
		t.Fatalf("抓取剔除短草的 grass-closeup 差分夹具: %v", err)
	}

	frame := image.Rect(0, 0, captureWidth, captureHeight)
	forward := camera.Forward()
	var candidates []oakGroveCellDiffStats
	for _, cell := range grassCloseupShortGrassCells {
		// 相机身后或贴着脸面的格子直接跳过：它们的 corner 投影不可定义，
		// 也不可能是「可辨识」的候选。单元立方体 corner 到中心的最大偏移
		// 是 √3/2，中心沿前向的投影余量取 1 即可保证全部 corner 在相机前。
		center := mgl32.Vec3{float32(cell.X) + 0.5, float32(cell.Y) + 0.5, float32(cell.Z) + 0.5}
		if center.Sub(camera.Pos).Dot(forward) < 1 {
			continue
		}
		rect := captureSceneCellRect(t, camera, cell)
		if !rect.In(frame) || rect.Dy() < 6 {
			continue
		}
		candidates = append(candidates, oakGroveCellDiff(t, withGrass, withoutGrass, rect))
	}
	t.Logf("夹具短草 %d 株，其中 %d 株屏幕矩形完整在画面内且高度 ≥6px",
		len(grassCloseupShortGrassCells), len(candidates))
	if len(candidates) == 0 {
		t.Fatal("画面内没有任何尺寸可辨识的短草格")
	}
	// 规格只要求「至少一株可辨识」：逐格寻找同时满足可见量、cutout 透空、
	// 上缘透空与贴地叶片四个判据的格子。顶带判据按带宽的 10% 容忍其他短草
	// 落入本格矩形上缘的透视重叠。
	var identifiable *oakGroveCellDiffStats
	for index := range candidates {
		stats := candidates[index]
		area := stats.rect.Dx() * stats.rect.Dy()
		topBand := stats.rect.Dy() / 4
		if stats.diff < grassCloseupIdentifiableDiffPixels || stats.diff*10 >= area*9 {
			continue
		}
		if topBand > 0 && stats.topDiff*10 > topBand*stats.rect.Dx() {
			continue
		}
		if stats.bottomDiff < 4 {
			continue
		}
		identifiable = &stats
		break
	}
	if identifiable == nil {
		for _, stats := range candidates {
			t.Logf("候选格矩形=%v 差分=%d 顶带=%d 底带=%d", stats.rect, stats.diff, stats.topDiff, stats.bottomDiff)
		}
		t.Fatalf("画面内 %d 个短草格没有一个满足可辨识判据（可见 ≥%dpx、非不透明矩形、上缘透空、贴地叶片）",
			len(candidates), grassCloseupIdentifiableDiffPixels)
	}
	t.Logf("可辨识短草格：%v 差分=%d 顶带=%d 底带=%d", identifiable.rect, identifiable.diff,
		identifiable.topDiff, identifiable.bottomDiff)
}
