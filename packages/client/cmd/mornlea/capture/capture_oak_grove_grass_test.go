package capture

// capture_oak_grove_grass_test.go：自然短草视觉 provenance 的钉死回归。规格
// delta visual-verification 要求 oak-grove 的固定夹具包含至少一株在相机中
// 可辨识的短草，且必须经既有四 quad alpha-cutout 植物路径呈现（透明边缘与
// 交叉轮廓，而不是实心立方体或不透明矩形）。正式场景清单在 HUD 三场景退役
// 与 mining-crack 对加入后恰好 24 项（完整冻结顺序由
// capture_scene_order_test.go 承担）、既有双阈值与无窗口链路同样是本变更不得
// 放宽的门禁，这里一并钉住。

import (
	"image"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// oakGroveShortGrassCells 枚举 oak-grove 固定夹具（3×3 生成区块）里全部自然
// 短草格。夹具本身由 `prepareOakGrove` 经生产 `worldgen.New` 装入，这里只读
// mirror，不在测试侧复制短草分布。
func oakGroveShortGrassCells(t *testing.T, app SceneApplication) []core.BlockPos {
	t.Helper()
	var cells []core.BlockPos
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			for y := int32(core.MinY); y < core.MaxY; y++ {
				for localZ := int32(0); localZ < core.SectionSize; localZ++ {
					for localX := int32(0); localX < core.SectionSize; localX++ {
						position := core.BlockPos{
							X: x*core.SectionSize + localX, Y: y, Z: z*core.SectionSize + localZ,
						}
						block, loaded := app.Mirror().BlockAt(core.Overworld, position)
						if !loaded {
							t.Fatalf("oak-grove mirror 未加载 chunk=(%d,%d) 的方块", x, z)
						}
						if block == core.ShortGrassID {
							cells = append(cells, position)
						}
					}
				}
			}
		}
	}
	return cells
}

// TestOakGroveFixtureGrowsNaturalShortGrass 钉住夹具的数据面：固定种子 42 的
// 生产 worldgen 在 oak-grove 覆盖的 3×3 区块里确实生成自然短草，且每株都
// 立在 GrassID 表面上（worldgen 的装饰不变量）。画面层的可辨识性由
// TestOakGroveSceneShowsIdentifiableNaturalShortGrass 承担。
func TestOakGroveFixtureGrowsNaturalShortGrass(t *testing.T) {
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	if err := prepareOakGrove(app); err != nil {
		t.Fatalf("准备 oak-grove: %v", err)
	}
	cells := oakGroveShortGrassCells(t, app)
	if len(cells) == 0 {
		t.Fatal("oak-grove 夹具没有任何自然短草：固定种子 42 的生产 worldgen 应在草地表面生成短草")
	}
	for _, cell := range cells {
		support, loaded := app.Mirror().BlockAt(core.Overworld,
			core.BlockPos{X: cell.X, Y: cell.Y - 1, Z: cell.Z})
		if !loaded || support != core.GrassID {
			t.Fatalf("短草 %+v 的支撑格 = %d/%v，想要 GrassID/true（短草只应立在草地表面上）",
				cell, support, loaded)
		}
	}
}

// oakGroveGrassPixelDelta 是差分像素的噪声下限：同机两次完整抓帧的实测漂移
// 在个位数 LSB（既有双阈值据此定为通道差 2），这里取 8 倍余量，把「短草造成
// 的真实差异」与编码漂移明确分开。
const oakGroveGrassPixelDelta = 8

// prepareOakGroveWithoutShortGrass 装入与 `prepareOakGrove` 完全相同的 3×3
// 生成区块，唯独把每格自然短草改写为空气。差分夹具与生产夹具的唯一差别因此
// 就是短草本身——两张图的差分即短草的视觉足迹，可直接逐格归因。
func prepareOakGroveWithoutShortGrass(app SceneApplication) error {
	generator := worldgen.New(captureOakGroveSeed, config.Defaults().FluidEnabled)
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk := generator.GenerateChunk(core.ChunkPos{X: x, Z: z})
			for y := int32(core.MinY); y < core.MaxY; y++ {
				for localZ := int32(0); localZ < core.SectionSize; localZ++ {
					for localX := int32(0); localX < core.SectionSize; localX++ {
						if chunk.BlockAt(int(localX), y, int(localZ)) == core.ShortGrassID {
							chunk.SetBlock(int(localX), y, int(localZ), core.AirID)
						}
					}
				}
			}
			if err := applyCaptureMirror(app, captureOakGroveSnapshot(chunk)); err != nil {
				return err
			}
		}
	}
	return nil
}

// oakGroveCellDiffStats 统计同一屏幕矩形内两张图的差分像素（任一 RGB 通道差
// ≥ oakGroveGrassPixelDelta），并返回矩形顶部带（上 1/4）与底部带（下 1/3）
// 各自的差分数，用来区分「从底部长出、上缘透空的交叉植物」与「实心立方体
// 或不透明矩形」。
type oakGroveCellDiffStats struct {
	rect       image.Rectangle
	diff       int
	topDiff    int
	bottomDiff int
}

func oakGroveCellDiff(t *testing.T, with, without *image.NRGBA, rect image.Rectangle) oakGroveCellDiffStats {
	t.Helper()
	stats := oakGroveCellDiffStats{rect: rect}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
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
			if delta < oakGroveGrassPixelDelta {
				continue
			}
			stats.diff++
			bandTop := rect.Min.Y + rect.Dy()/4
			bandBottom := rect.Max.Y - rect.Dy()/3
			if y < bandTop {
				stats.topDiff++
			}
			if y >= bandBottom {
				stats.bottomDiff++
			}
		}
	}
	return stats
}

// TestOakGroveSceneShowsIdentifiableNaturalShortGrass 是 oak-grove 的场景内
// 像素断言（spec delta visual-verification「oak-grove 明确承重短草外观」）：
// 经完整无窗口链路（离屏 renderer、预热、装夹具、收敛、回读）抓两次帧——一次
// 生产夹具、一次仅剔除短草的差分夹具——短草的视觉足迹就是两图差分。可辨识
// 的判据是：
//   - 至少一株短草格的屏幕外接矩形内差分像素足够多（在画面里可见，而不是
//     远成 1~2 个噪声像素）；
//   - 该矩形内差分不铺满（alpha cutout 透过背景，不是不透明矩形）；
//   - 矩形顶部带几乎没有差分（叶片不达纹理顶部，上缘透空，不是实心立方体）；
//   - 矩形底部带有差分（叶片从贴地一侧长出，交叉斜面真实渲染）。
func TestOakGroveSceneShowsIdentifiableNaturalShortGrass(t *testing.T) {
	app := newCaptureSceneRenderApplication(t)
	if app.Window() != nil {
		t.Fatal("oak-grove 像素断言必须走无窗口离屏链路，当前存在交互窗口")
	}
	scene := captureSceneByName(t, "oak-grove")
	withGrass, err := captureSceneImage(app, scene)
	if err != nil {
		t.Fatalf("抓取 oak-grove: %v", err)
	}
	cells := oakGroveShortGrassCells(t, app)
	if len(cells) == 0 {
		t.Fatal("oak-grove 夹具没有任何自然短草")
	}
	camera := *app.Camera()

	bareApp := newCaptureSceneRenderApplication(t)
	grassless := captureScene{
		Name:         "oak-grove-grassless",
		WarmupFrames: scene.WarmupFrames,
		Prepare:      prepareOakGroveWithoutShortGrass,
		Apply:        applyOakGroveCaptureState,
	}
	withoutGrass, err := captureSceneImage(bareApp, grassless)
	if err != nil {
		t.Fatalf("抓取剔除短草的 oak-grove 差分夹具: %v", err)
	}

	frame := image.Rect(0, 0, captureWidth, captureHeight)
	forward := camera.Forward()
	var candidates []oakGroveCellDiffStats
	for _, cell := range cells {
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
	t.Logf("夹具短草 %d 株，其中 %d 株屏幕矩形完整在画面内且高度 ≥6px", len(cells), len(candidates))
	if len(candidates) == 0 {
		t.Fatal("画面内没有任何尺寸可辨识的短草格")
	}
	// 规格只要求「至少一株可辨识」：逐格寻找同时满足可见量、cutout 透空、
	// 上缘透空与贴地叶片四个判据的格子。顶带判据按带宽的 10% 容忍远处的
	// 其他短草落入本格矩形上缘的透视重叠。
	var identifiable *oakGroveCellDiffStats
	for index := range candidates {
		stats := candidates[index]
		area := stats.rect.Dx() * stats.rect.Dy()
		topBand := stats.rect.Dy() / 4
		if stats.diff < 8 || stats.diff*10 >= area*9 {
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
		t.Fatalf("画面内 %d 个短草格没有一个满足可辨识判据（可见 ≥8px、非不透明矩形、上缘透空、贴地叶片）",
			len(candidates))
	}
	t.Logf("可辨识短草格：%v 差分=%d 顶带=%d 底带=%d", identifiable.rect, identifiable.diff,
		identifiable.topDiff, identifiable.bottomDiff)
}

// TestCaptureOfficialSceneListStaysAtTwentyFour 钉住正式场景清单在 HUD 三场景
// 退役与 mining-crack 对加入后恰好 24 项；自然短草不得为它新增第 25 个正式
// 场景，完整数量与冻结顺序断言由 capture_scene_order_test.go 的清单守卫承担。
func TestCaptureOfficialSceneListStaysAtTwentyFour(t *testing.T) {
	if len(captureScenes) != 24 {
		t.Fatalf("正式 capture 场景数=%d，想要恰好 24", len(captureScenes))
	}
}

// TestCaptureCompareThresholdsUnchanged 钉住既有双阈值（单像素最大通道差与
// 差异像素占比）不得放宽——数值来自同机重复抓帧的实测漂移分布，见
// capture_image.go 的说明。
func TestCaptureCompareThresholdsUnchanged(t *testing.T) {
	want := diffThreshold{MaxChannelDelta: 2, MaxDiffPixelRatio: 0.0001}
	if captureThresholds != want {
		t.Fatalf("captureThresholds=%+v，想要 %+v（不得放宽既有双阈值）", captureThresholds, want)
	}
}

// fakeCaptureWindow 以嵌入 nil 接口的方式提供一个非 nil 的 application.Window
// 值：validateCaptureApplication 只做非 nil 判定，不会调用其方法。
type fakeCaptureWindow struct{ application.Window }

// windowedCaptureApp 把任意无头 application 包装成「带窗口」形态，只覆盖
// Window/FramebufferSize 两个判定面，其余行为保持原样。
type windowedCaptureApp struct{ SceneApplication }

func (a windowedCaptureApp) Window() application.Window  { return fakeCaptureWindow{} }
func (a windowedCaptureApp) FramebufferSize() (int, int) { return 1, 1 }

// TestCaptureRequiresHeadlessOffscreenChain 钉住完整无窗口链路门禁：
// `validateCaptureApplication` 必须拒绝带交互窗口的 application 与不符合
// capture 固定分辨率的 framebuffer；真实离屏 application 必须通过同一校验。
func TestCaptureRequiresHeadlessOffscreenChain(t *testing.T) {
	app := newCaptureSceneRenderApplication(t)
	if err := validateCaptureApplication(app); err != nil {
		t.Fatalf("离屏 application 应通过无头校验: %v", err)
	}
	if err := validateCaptureApplication(windowedCaptureApp{app}); err == nil {
		t.Fatal("带交互窗口的 application 必须被无头校验拒绝")
	}
	if width, height := app.FramebufferSize(); width != captureWidth || height != captureHeight {
		t.Fatalf("离屏 framebuffer=%dx%d，想要 %dx%d", width, height, captureWidth, captureHeight)
	}
}

// TestPrepareOakGroveWithoutShortGrassMatchesFixtureExceptGrass 钉住差分夹具
// 的前提：剔除短草后的 3×3 区块与生产夹具逐格一致，唯独 ShortGrassID 全部
// 变为 AirID——差分图像的任何差异因此只能来自短草。
func TestPrepareOakGroveWithoutShortGrassMatchesFixtureExceptGrass(t *testing.T) {
	fullMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(fullMesher.Close)
	full := &application.Application{}
	full.SetMirror(client.NewMirror())
	full.SetMesher(fullMesher)
	if err := prepareOakGrove(full); err != nil {
		t.Fatalf("准备生产夹具: %v", err)
	}
	bareMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(bareMesher.Close)
	bare := &application.Application{}
	bare.SetMirror(client.NewMirror())
	bare.SetMesher(bareMesher)
	if err := prepareOakGroveWithoutShortGrass(bare); err != nil {
		t.Fatalf("准备差分夹具: %v", err)
	}
	grass := 0
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			for y := int32(core.MinY); y < core.MaxY; y++ {
				for localZ := int32(0); localZ < core.SectionSize; localZ++ {
					for localX := int32(0); localX < core.SectionSize; localX++ {
						position := core.BlockPos{
							X: x*core.SectionSize + localX, Y: y, Z: z*core.SectionSize + localZ,
						}
						want, loaded := full.Mirror().BlockAt(core.Overworld, position)
						if !loaded {
							t.Fatalf("生产夹具未加载 chunk=(%d,%d)", x, z)
						}
						got, loaded := bare.Mirror().BlockAt(core.Overworld, position)
						if !loaded {
							t.Fatalf("差分夹具未加载 chunk=(%d,%d)", x, z)
						}
						if want == core.ShortGrassID {
							grass++
							want = core.AirID
						}
						if got != want {
							t.Fatalf("差分夹具在 %+v = %d，除短草外应与生产夹具一致（%d）", position, got, want)
						}
					}
				}
			}
		}
	}
	if grass == 0 {
		t.Fatal("生产夹具没有短草，差分夹具失去了差分意义")
	}
}
