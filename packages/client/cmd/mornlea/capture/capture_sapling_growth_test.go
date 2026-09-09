package capture

// capture_sapling_growth_test.go：树苗与橡树再生基线场景的钉死回归。
// `prepareSaplingGrowth` 装入草地支撑面上的树苗与运行时树形长成的橡树，
// `applySaplingGrowthCaptureState` 钉死正午与固定机位；本文件断言夹具数据面
// （树苗立在草地上、橡树逐格等于运行时树形几何）与场景内像素可辨识性
// （树苗经四 quad cutout 路径呈现、橡树的树干与树冠可辨）。golden 基线由
// `make visual-update` 显式产出，这里只做活帧差分，不比较任何 golden 文件。

import (
	"image"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// saplingGrowthIdentifiableDiffPixels 是树苗可辨识的差分下限：机位距树苗约
// 4.6 格，交叉斜面在画面里是数十像素量级；200px 远高于同机重复抓帧的个位数
// LSB 漂移（`oakGroveGrassPixelDelta` 的 25 倍余量口径），误报只能来自树苗
// 本身。
const saplingGrowthIdentifiableDiffPixels = 200

// saplingGrowthOakTrunkDiffPixels / saplingGrowthOakCrownDiffPixels 是橡树
// 树干与树冠可辨识的差分下限：橡树距相机约 14.5 格，树干列在画面里约 20px
// 宽、树冠外接矩形约 100×60px，两个下限都远低于实测值而远高于噪声。
const (
	saplingGrowthOakTrunkDiffPixels = 100
	saplingGrowthOakCrownDiffPixels = 300
)

// TestSaplingGrowthFixtureUsesRuntimeTreeShape 钉住夹具的数据面：树苗有且
// 只有冻结的一格且正下方是草地，橡树逐格等于 `worldgen.TreeBlocks` 的运行时
// 记录（本场景因此同时覆盖新 ABI 在真实客户端进程里的展开），窗口内没有
// 手工多摆的第二棵橡树。
func TestSaplingGrowthFixtureUsesRuntimeTreeShape(t *testing.T) {
	scene := captureSceneByName(t, "sapling-growth")
	if scene.Prepare == nil || scene.Apply == nil || scene.WarmupFrames != 8 {
		t.Fatalf("sapling-growth 场景不完整: %+v", scene)
	}

	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 sapling-growth: %v", err)
	}

	// 树苗：冻结的一格，正下方是草地（树苗没有自己的碰撞与光照，只能立在
	// 泥土或草地上，与世界生成的不变式一致）。
	if block, loaded := app.Mirror().BlockAt(core.Overworld, captureSaplingGrowthSapling); !loaded || block != core.SaplingID {
		t.Fatalf("树苗格 %+v = %d/%v，想要 SaplingID/true",
			captureSaplingGrowthSapling, block, loaded)
	}
	support := core.BlockPos{
		X: captureSaplingGrowthSapling.X,
		Y: captureSaplingGrowthSapling.Y - 1,
		Z: captureSaplingGrowthSapling.Z,
	}
	if block, loaded := app.Mirror().BlockAt(core.Overworld, support); !loaded || block != core.GrassID {
		t.Fatalf("树苗支撑格 %+v = %d/%v，想要 GrassID/true", support, block, loaded)
	}

	// 橡树：逐格等于运行时树形几何的记录，且窗口内没有第二棵橡树。
	records := worldgen.TreeBlocks(captureSaplingGrowthSeed, captureSaplingGrowthRoot)
	if len(records) < 2 || records[0] != (worldgen.TreeBlock{Block: core.OakLogID}) {
		t.Fatalf("运行时树形几何=%+v，想要以树干底原木开头的多条记录", records)
	}
	oakCells := make(map[core.BlockPos]core.BlockID, len(records))
	for _, record := range records {
		position := core.BlockPos{
			X: captureSaplingGrowthRoot.X + int32(record.DX),
			Y: captureSaplingGrowthRoot.Y + int32(record.DY),
			Z: captureSaplingGrowthRoot.Z + int32(record.DZ),
		}
		oakCells[position] = record.Block
	}
	if got := len(oakCells); got != len(records) {
		t.Fatalf("树形记录有重复格：记录 %d 条、去重后 %d 格", len(records), got)
	}
	trunk := 0
	for position, want := range oakCells {
		block, loaded := app.Mirror().BlockAt(core.Overworld, position)
		if !loaded || block != want {
			t.Fatalf("橡树格 %+v = %d/%v，想要 %d/true", position, block, loaded, want)
		}
		if want == core.OakLogID {
			trunk++
		}
	}
	if trunk < 5 || trunk > 7 {
		t.Fatalf("橡树树干高=%d，想要 5..7", trunk)
	}
	// 邻域扫描：空气快照覆盖区块 -1..1（x/z ∈ [-16,31]），夹具只写 y=-1..1
	// 与树形记录；该窗口内原木与树叶有且只有树形记录的那些格。
	for z := int32(-16); z <= 31; z++ {
		for x := int32(-16); x <= 31; x++ {
			for y := int32(-1); y <= 9; y++ {
				position := core.BlockPos{X: x, Y: y, Z: z}
				block, loaded := app.Mirror().BlockAt(core.Overworld, position)
				if !loaded {
					t.Fatalf("树苗生长 mirror 未加载 %+v", position)
				}
				switch block {
				case core.OakLogID, core.LeavesID:
					if want, ok := oakCells[position]; !ok || want != block {
						t.Fatalf("窗口内出现树形几何之外的原木/树叶 %+v=%d", position, block)
					}
				}
			}
		}
	}
	if got := mesher.Stats().DirtySections; got == 0 {
		t.Fatal("sapling-growth 通过 mirror 装入后 mesher 没有 dirty section")
	}
}

// TestSaplingGrowthApplyPinsNoonCameraAndClearsPredecessors 钉住场景的全部
// 呈现状态：固定正午与固定机位，前序场景留下的共享呈现状态经公共清场统一
// 清空（本场景排在前景丰富的 oak-grove 之后，雨天、第三人称、牛群与伙伴都
// 可能残留），不依赖场景表顺序。
func TestSaplingGrowthApplyPinsNoonCameraAndClearsPredecessors(t *testing.T) {
	app := application.NewPresentationApplicationForTest()
	// 被动生物镜像不在最小测试装配里，本场景的公共清场会复位它：显式补上
	// 才能在同一个 app 上先注入牛群、再断言清场把它清空。
	app.SetPassives(&client.Passives{})
	if err := app.SetCaptureWeather(core.WeatherRain); err != nil {
		t.Fatal(err)
	}
	app.SetCameraMode(client.CameraThirdPersonBack)
	if err := app.Passives().ApplySpawn(network.PassiveSpawn{
		ServerTick: 1,
		Spawns: []network.PassiveSpawnRecord{{
			ID: 1, Dimension: core.Overworld,
			Position: mgl32.Vec3{0, 1, 0}, Health: core.MaxHealth,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := applySaplingGrowthCaptureState(app); err != nil {
		t.Fatalf("应用 sapling-growth: %v", err)
	}
	want := client.Camera{
		Pos: mgl32.Vec3{0.5, 2.2, 3.5}, Yaw: 0, Pitch: -0.06,
		FovY: mgl32.DegToRad(70), Aspect: float32(application.CaptureWidth) / application.CaptureHeight,
		Near: 0.1, Far: 2000,
	}
	if app.WorldTimeTicks() != 6000 || *app.Camera() != want {
		t.Fatalf("固定环境 time=%d camera=%+v，想要 6000/%+v",
			app.WorldTimeTicks(), *app.Camera(), want)
	}
	if app.Weather() != core.WeatherClear {
		t.Fatalf("呈现天气=%d，想要晴天 %d（雨天泄入后继场景）", app.Weather(), core.WeatherClear)
	}
	if app.CameraMode() != client.CameraFirstPerson {
		t.Fatalf("机位=%d，想要第一人称 %d（第三人称泄入后继场景）",
			app.CameraMode(), client.CameraFirstPerson)
	}
	if len(app.Passives().AppendPresentations(nil)) != 0 {
		t.Fatal("sapling-growth 夹具残留前序场景的牛群")
	}
}

// prepareSaplingGrowthWithoutPlants 装入与 `prepareSaplingGrowth` 相同的草地
// 支撑面，唯独不摆树苗、不展开运行时树形几何。差分夹具与生产夹具的唯一差别
// 因此就是两株植物本身——两张图的差分即树苗与橡树的视觉足迹，可直接按格
// 归因。支撑面循环与生产夹具逐字一致（不得顺手重构生产夹具），差别只在省略
// 树苗与树形展开两段。
func prepareSaplingGrowthWithoutPlants(app SceneApplication) error {
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
	for z := int32(-14); z <= 4; z++ {
		for x := int32(-5); x <= 9; x++ {
			setBlock(core.BlockPos{X: x, Y: -1, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.GrassID)
		}
	}
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "树苗生长差分")
}

// saplingGrowthTopBandDiff 统计 rect 顶部 rows 行内的差分像素数，像素判据与
// `oakGroveCellDiff` 逐字一致（任一 RGB 通道差 ≥ `oakGroveGrassPixelDelta`）。
// 树苗的「上缘透空」判据比短草更严：树苗纹理最上一行（1/16 格）全透明，
// 采样带因此按 1/16 而不是四分之一取，才能把叶团上缘的稀疏像素排除在外。
func saplingGrowthTopBandDiff(with, without *image.NRGBA, rect image.Rectangle, rows int) int {
	diff := 0
	for y := rect.Min.Y; y < rect.Min.Y+rows && y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			iWith, iWithout := with.PixOffset(x, y), without.PixOffset(x, y)
			delta := 0
			for c := 0; c < 3; c++ {
				d := int(with.Pix[iWith+c]) - int(without.Pix[iWithout+c])
				if d < 0 {
					d = -d
				}
				if d > delta {
					delta = d
				}
			}
			if delta >= oakGroveGrassPixelDelta {
				diff++
			}
		}
	}
	return diff
}

// saplingGrowthCellUnionRect 返回一组方块格投影矩形的并集。
func saplingGrowthCellUnionRect(t *testing.T, camera client.Camera, cells []core.BlockPos) image.Rectangle {
	t.Helper()
	var union image.Rectangle
	for index, cell := range cells {
		rect := captureSceneCellRect(t, camera, cell)
		if index == 0 {
			union = rect
			continue
		}
		union = union.Union(rect)
	}
	return union
}

// TestSaplingGrowthSceneShowsIdentifiableSaplingAndOak 是场景内像素断言：经
// 完整无窗口链路（离屏 renderer、预热、装夹具、收敛、回读）抓两次帧——一次
// 生产夹具、一次仅剔除树苗与橡树的差分夹具——两株植物的视觉足迹就是两图
// 差分。判据分两组：
//   - 树苗：屏幕矩形内差分像素足够多（在画面里可见）；差分不铺满矩形
//     （四 quad alpha cutout 透过背景，不是不透明立方体）；矩形最上一行
//     带没有差分（纹理上缘透空）；矩形底部带有差分（叶片贴地长出）。
//   - 橡树：树干格并集与树叶格并集各自差分像素足够多（树干与树冠都可辨）。
func TestSaplingGrowthSceneShowsIdentifiableSaplingAndOak(t *testing.T) {
	app := newCaptureSceneRenderApplication(t)
	if app.Window() != nil {
		t.Fatal("sapling-growth 像素断言必须走无窗口离屏链路，当前存在交互窗口")
	}
	scene := captureSceneByName(t, "sapling-growth")
	withPlants, err := captureSceneImage(app, scene)
	if err != nil {
		t.Fatalf("抓取 sapling-growth: %v", err)
	}
	camera := *app.Camera()

	bareApp := newCaptureSceneRenderApplication(t)
	bare := captureScene{
		Name:         "sapling-growth-bare",
		WarmupFrames: scene.WarmupFrames,
		Prepare:      prepareSaplingGrowthWithoutPlants,
		Apply:        applySaplingGrowthCaptureState,
	}
	withoutPlants, err := captureSceneImage(bareApp, bare)
	if err != nil {
		t.Fatalf("抓取剔除树苗与橡树的 sapling-growth 差分夹具: %v", err)
	}

	frame := image.Rect(0, 0, captureWidth, captureHeight)

	saplingRect := captureSceneCellRect(t, camera, captureSaplingGrowthSapling)
	if !saplingRect.In(frame) || saplingRect.Dy() < 6 {
		t.Fatalf("树苗格 %+v 屏幕矩形=%v，必须完整在画面内且高度 ≥6px",
			captureSaplingGrowthSapling, saplingRect)
	}
	sapling := oakGroveCellDiff(t, withPlants, withoutPlants, saplingRect)
	area := saplingRect.Dx() * saplingRect.Dy()
	topRows := max(1, saplingRect.Dy()/16)
	topDiff := saplingGrowthTopBandDiff(withPlants, withoutPlants, saplingRect, topRows)
	t.Logf("树苗矩形=%v 差分=%d 面积=%d 顶带(%d行)=%d 底带=%d",
		saplingRect, sapling.diff, area, topRows, topDiff, sapling.bottomDiff)
	if sapling.diff < saplingGrowthIdentifiableDiffPixels {
		t.Fatalf("树苗差分=%d，想要 ≥%d（画面里不可辨）",
			sapling.diff, saplingGrowthIdentifiableDiffPixels)
	}
	if sapling.diff*10 >= area*9 {
		t.Fatalf("树苗差分=%d 铺满矩形面积 %d 的九成以上（cutout 透空不成立）",
			sapling.diff, area)
	}
	if topDiff != 0 {
		t.Fatalf("树苗矩形顶带(%d 行)差分=%d，想要 0（上缘透空不成立）", topRows, topDiff)
	}
	if sapling.bottomDiff < 4 {
		t.Fatalf("树苗矩形底带差分=%d，想要 ≥4（叶片未贴地）", sapling.bottomDiff)
	}

	records := worldgen.TreeBlocks(captureSaplingGrowthSeed, captureSaplingGrowthRoot)
	var trunkCells, crownCells []core.BlockPos
	for _, record := range records {
		position := core.BlockPos{
			X: captureSaplingGrowthRoot.X + int32(record.DX),
			Y: captureSaplingGrowthRoot.Y + int32(record.DY),
			Z: captureSaplingGrowthRoot.Z + int32(record.DZ),
		}
		if record.Block == core.OakLogID {
			trunkCells = append(trunkCells, position)
			continue
		}
		crownCells = append(crownCells, position)
	}
	trunkRect := saplingGrowthCellUnionRect(t, camera, trunkCells)
	crownRect := saplingGrowthCellUnionRect(t, camera, crownCells)
	if !trunkRect.In(frame) || !crownRect.In(frame) {
		t.Fatalf("橡树投影越出画面：trunk=%v crown=%v", trunkRect, crownRect)
	}
	trunk := oakGroveCellDiff(t, withPlants, withoutPlants, trunkRect)
	crown := oakGroveCellDiff(t, withPlants, withoutPlants, crownRect)
	t.Logf("橡树树干矩形=%v 差分=%d；树冠矩形=%v 差分=%d",
		trunkRect, trunk.diff, crownRect, crown.diff)
	if trunk.diff < saplingGrowthOakTrunkDiffPixels {
		t.Fatalf("橡树树干差分=%d，想要 ≥%d（树干不可辨）",
			trunk.diff, saplingGrowthOakTrunkDiffPixels)
	}
	if crown.diff < saplingGrowthOakCrownDiffPixels {
		t.Fatalf("橡树树冠差分=%d，想要 ≥%d（树冠不可辨）",
			crown.diff, saplingGrowthOakCrownDiffPixels)
	}
}
