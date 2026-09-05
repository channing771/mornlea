//go:build darwin

package capture

// motion_break_burst_test.go：完整采掘生命周期 motion 演示的回归测试。
// 50 帧时间线：F0–4 静置 → F5–24 采掘爬坡（裂纹 0→9 扫完）→ F25 破坏同帧
// （镜像置空 + overlay 熄灭 + 泥土掉落注入，burst 年龄 0 起算）→ F25–44
// 粒子存续 + 掉落下落（重力积分约 9 tick 着陆）→ F34–49 掉落静置留存、裂纹不再出现。
// 链路正确性由破碎 burst 的逐帧测试与 `RenderFrame` 接线测试承接，
// 这里只钉时间线落点与产物约定。

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

// TestBreakBurstMotionCaptures50FramesInOrder 钉住演示抓帧的帧约定：50 帧、
// 帧号 0→49 顺序递进、同一固定序列两次抓取逐帧字节一致。
func TestBreakBurstMotionCaptures50FramesInOrder(t *testing.T) {
	capture := func(record *[]int) func(int) (*image.NRGBA, error) {
		return func(frame int) (*image.NRGBA, error) {
			*record = append(*record, frame)
			img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
			img.Pix[0] = uint8(frame)
			return img, nil
		}
	}
	var firstFrames []int
	first, err := captureBreakBurstMotionFrames(capture(&firstFrames))
	if err != nil {
		t.Fatalf("抓取 motion 帧序列: %v", err)
	}
	if len(first) != breakBurstMotionFrameCount {
		t.Fatalf("motion 帧数=%d，想要 %d", len(first), breakBurstMotionFrameCount)
	}
	if breakBurstMotionFrameCount != 50 {
		t.Fatalf("motion 帧数常量=%d，想要 50", breakBurstMotionFrameCount)
	}
	for index, frame := range firstFrames {
		if frame != index {
			t.Fatalf("第 %d 次抓取帧号=%d，想要 %d", index, frame, index)
		}
	}
	var secondFrames []int
	second, err := captureBreakBurstMotionFrames(capture(&secondFrames))
	if err != nil {
		t.Fatalf("重抓 motion 帧序列: %v", err)
	}
	for index := range first {
		if !bytes.Equal(first[index].Pix, second[index].Pix) {
			t.Fatalf("第 %d 帧两次抓取字节不一致", index)
		}
	}
}

// TestBreakBurstMotionTickAdvancesOnePerFrame 钉住合成 tick 与帧号的映射：
// 逐帧 +1，破坏帧 F25 的 tick 即掉落注入的 ServerTick（burst 年龄 0 起算点）。
func TestBreakBurstMotionTickAdvancesOnePerFrame(t *testing.T) {
	for frame := range breakBurstMotionFrameCount {
		if got, want := breakBurstMotionTick(frame), breakBurstMotionTickBase+uint64(frame); got != want {
			t.Fatalf("第 %d 帧 tick=%d，想要 %d", frame, got, want)
		}
	}
}

// TestBreakBurstMotionMiningRampCoversAllCrackStages 钉住采掘爬坡：F0–4 静置
// 无采掘；F5–24 进度按 (i-5)*10 爬坡且裂纹阶段 0→9 每阶段至少出现 1 帧；
// F25 起 overlay 熄灭（破坏即清理，裂纹不得再出现）。
func TestBreakBurstMotionMiningRampCoversAllCrackStages(t *testing.T) {
	inactive := hud.MiningOverlay{}
	for frame := 0; frame < 5; frame++ {
		if got := breakBurstMotionOverlay(frame); got != inactive {
			t.Fatalf("第 %d 帧 overlay=%+v，想要静置无采掘", frame, got)
		}
	}
	seen := make(map[int]bool)
	for frame := 5; frame <= 24; frame++ {
		want := hud.MiningOverlay{
			Active: true, HasTarget: true, Target: breakBurstMotionTarget,
			ProgressTicks: uint16(frame-5) * 10, RequiredTicks: 200,
		}
		if got := breakBurstMotionOverlay(frame); got != want {
			t.Fatalf("第 %d 帧 overlay=%+v，想要 %+v", frame, got, want)
		}
		seen[render.BlockCrackStage(want.ProgressTicks, want.RequiredTicks)] = true
	}
	for stage := 0; stage <= 9; stage++ {
		if !seen[stage] {
			t.Fatalf("裂纹阶段 %d 在 F5–24 未出现，实际覆盖 %v", stage, seen)
		}
	}
	for frame := 25; frame < breakBurstMotionFrameCount; frame++ {
		if got := breakBurstMotionOverlay(frame); got != inactive {
			t.Fatalf("第 %d 帧 overlay=%+v，想要破坏后熄灭", frame, got)
		}
	}
}

// TestBreakBurstMotionBreakFrameClearsTargetAndSeedsDrop 钉住 F25 破坏同帧三件事：
// 镜像目标置空（泥土→空气）+ overlay 熄灭 + 泥土掉落注入且 burst 年龄为 0
// （合成 tick 与注入 ServerTick 同值）；破坏前一帧目标仍是泥土且选框命中它。
func TestBreakBurstMotionBreakFrameClearsTargetAndSeedsDrop(t *testing.T) {
	app := newCaptureAICompanionState()
	app.SetMirror(client.NewMirror())
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app.SetMesher(mesher)
	predictor := client.NewPredictor()
	if err := predictor.Begin(network.PlayerState{
		Dimension: core.Overworld, Position: captureCrackCameraPos,
		Yaw: 0, Pitch: 0, Ready: true,
		Health: core.MaxHealth, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	app.SetPredictor(predictor)
	if err := prepareMiningLifecycleDirt(app); err != nil {
		t.Fatalf("装入泥土目标: %v", err)
	}
	if err := applyMiningCrackCaptureState(app); err != nil {
		t.Fatalf("装入裂纹机位: %v", err)
	}
	for frame := 0; frame < 25; frame++ {
		if err := applyMiningLifecycleFrame(app, frame); err != nil {
			t.Fatalf("推进第 %d 帧: %v", frame, err)
		}
	}
	if got, loaded := app.Mirror().BlockAt(core.Overworld, breakBurstMotionTarget); !loaded || got != core.DirtID {
		t.Fatalf("破坏前目标 BlockAt=%d/%v，想要泥土/true", got, loaded)
	}
	if target, ok := app.CurrentBlockTarget(); !ok || target.Position != breakBurstMotionTarget {
		t.Fatalf("破坏前 CurrentBlockTarget=%+v/%v，想要命中 %v", target, ok, breakBurstMotionTarget)
	}
	if err := applyMiningLifecycleFrame(app, 25); err != nil {
		t.Fatalf("推进破坏帧: %v", err)
	}
	if got, loaded := app.Mirror().BlockAt(core.Overworld, breakBurstMotionTarget); !loaded || got != core.AirID {
		t.Fatalf("破坏帧目标 BlockAt=%d/%v，想要空气/true", got, loaded)
	}
	if got := app.MiningOverlay(); got != (hud.MiningOverlay{}) {
		t.Fatalf("破坏帧 overlay=%+v，想要熄灭", got)
	}
	drops := app.ItemDrops().Presentations()
	if len(drops) != 1 || drops[0].Item != core.ItemDirt {
		t.Fatalf("破坏帧掉落=%+v，想要 1 个泥土", drops)
	}
	if upsert := breakBurstMotionDropUpsert(); app.ServerTick() != upsert.ServerTick {
		t.Fatalf("破坏帧 tick=%d，注入 ServerTick=%d，burst 年龄非 0",
			app.ServerTick(), upsert.ServerTick)
	}
	if _, ok := app.CurrentBlockTarget(); ok {
		t.Fatalf("破坏帧选框仍有命中，目标置空后应脱靶")
	}
}

// TestBreakBurstMotionBreakFrameGrabsSettledPixels 钉住破坏帧抓帧时刻的诚实性：
// 生产抓帧缝（`captureMotionFrame`）回读的那一刻，镜像置空必须已落到网格，
// 否则抓到的是重建前的旧网格（方块残留一帧，而叠加层熄灭是 CPU 即时量、裂纹
// 已消失——恰是旧 GIF 里 F25 的样子）。断言三层：状态与抓帧同帧（镜像空气 +
// 熄灭 + 掉落年龄 0）；该帧编码的裂纹实例流为空；抓帧时刻网格已收敛且目标区域
// 像素稳定（重收敛重抓零差异，即回读像素已是置空后的网格）。
func TestBreakBurstMotionBreakFrameGrabsSettledPixels(t *testing.T) {
	app := application.NewOffscreenRenderApplicationForTest(
		t, &application.IntegrationGlyphSource{}, captureWidth, captureHeight,
		config.Defaults().Render)
	predictor := client.NewPredictor()
	if err := predictor.Begin(network.PlayerState{
		Dimension: core.Overworld, Position: captureCrackCameraPos,
		Yaw: 0, Pitch: 0, Ready: true,
		Health: core.MaxHealth, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	app.SetPredictor(predictor)
	// 收敛帧走生产同款场景值（装入泥土目标 + 裂纹机位并落网格，产出丢弃）：
	// 此后 0→25 帧与 `RunBreakBurstMotion` 的循环完全同构。
	if _, err := captureSceneImage(app, breakBurstMotionScene); err != nil {
		t.Fatalf("收敛 motion 场景: %v", err)
	}
	var breakImg *image.NRGBA
	for frame := 0; frame <= breakBurstMotionBreakFrame; frame++ {
		img, err := captureMotionFrame(app, frame)
		if err != nil {
			t.Fatalf("抓取第 %d 帧: %v", frame, err)
		}
		if frame == breakBurstMotionBreakFrame {
			breakImg = img
		}
	}
	if got, loaded := app.Mirror().BlockAt(core.Overworld, breakBurstMotionTarget); !loaded || got != core.AirID {
		t.Fatalf("抓帧时刻目标 BlockAt=%d/%v，想要空气/true", got, loaded)
	}
	if got := app.MiningOverlay(); got != (hud.MiningOverlay{}) {
		t.Fatalf("抓帧时刻 overlay=%+v，想要熄灭", got)
	}
	drops := app.ItemDrops().Presentations()
	if len(drops) != 1 || drops[0].Item != core.ItemDirt {
		t.Fatalf("抓帧时刻掉落=%+v，想要 1 个泥土", drops)
	}
	if upsert := breakBurstMotionDropUpsert(); app.ServerTick() != upsert.ServerTick {
		t.Fatalf("抓帧时刻 tick=%d，注入 ServerTick=%d，burst 年龄非 0",
			app.ServerTick(), upsert.ServerTick)
	}
	// 裂纹实例流为空即该帧零裂纹像素：裂纹通道的唯一生产者是 `RenderFrame`
	// 内的裂纹实例编码（消费端是 Rust 裂纹管线），流为空则零实例下行，也就
	// 没有任何裂纹像素来源；派生门控（选框脱靶 + 熄灭）由 app 侧裂纹测试钉住，
	// 这里不断言门控、只断言进入渲染器的实例流本身。
	if got := app.CrackInstances(); len(got) != 0 {
		t.Fatalf("破坏帧裂纹实例流=%d 字节，想要 0", len(got))
	}
	// 抓帧时刻已收敛：生产缝回读前落过网格，统计量当场即满足抓帧判据；单次
	// `RenderFrame` 后立刻回读会留下数十个脏 section，该断言在旧缝下失败。
	stats, pending := app.Mesher().Stats(), app.Scheduler().PendingUploads()
	lodBusy := 0
	if app.LODScheduler() != nil {
		lodBusy = app.LODScheduler().Busy()
	}
	if vista := app.MenuVistaPending(); !captureSettled(stats, pending, lodBusy, vista) {
		t.Fatalf("破坏帧抓帧时刻未收敛：mesher=%+v pending=%d lodBusy=%d vista=%d",
			stats, pending, lodBusy, vista)
	}
	// 目标区域像素稳定：再落一轮网格并重抓，区域零差异——回读像素已是置空后
	// 的网格，而非重建前的旧网格。窗口复用裂纹像素测试的断言窗口（同机位同目标）。
	if err := settleMotionBreakFrame(app); err != nil {
		t.Fatalf("重收敛: %v", err)
	}
	if _, err := app.RenderFrame(captureDrainMax); err != nil {
		t.Fatalf("重抓渲染: %v", err)
	}
	again := bgraToNRGBA(app.Renderer().Readback(), captureWidth, captureHeight)
	crop := func(img *image.NRGBA) *image.NRGBA {
		cropped := image.NewNRGBA(image.Rect(0, 0, captureCrackTargetRegion.Dx(), captureCrackTargetRegion.Dy()))
		draw.Draw(cropped, cropped.Bounds(), img, captureCrackTargetRegion.Min, draw.Src)
		return cropped
	}
	diff, _, err := compareImages(crop(breakImg), crop(again))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("破坏帧目标区域重抓差异：%s", diff)
	if diff.DiffPixels != 0 {
		t.Fatalf("破坏帧目标区域重抓漂移：%s，抓帧时刻像素仍是旧网格", diff)
	}
}

// TestBreakBurstMotionFloorAndFrameBudgetFitLanding 钉住着陆演示的两项静态
// 前提：目标正下方有草地（顶面 y=0），3 格落差按重力积分约 9 tick 着陆
// （`render` 呈现下落与角色同形）；破坏帧之后留够 9 tick 下落 +
// 至少 3 帧静置掉落。
func TestBreakBurstMotionFloorAndFrameBudgetFitLanding(t *testing.T) {
	app := newCaptureAICompanionState()
	app.SetMirror(client.NewMirror())
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app.SetMesher(mesher)
	if err := prepareMiningLifecycleDirt(app); err != nil {
		t.Fatalf("装入泥土目标: %v", err)
	}
	if got, loaded := app.Mirror().BlockAt(core.Overworld, breakBurstMotionTarget); !loaded || got != core.DirtID {
		t.Fatalf("目标 BlockAt=%d/%v，想要泥土/true", got, loaded)
	}
	for y := int32(0); y <= 2; y++ {
		position := core.BlockPos{X: 0, Y: y, Z: -3}
		if got, loaded := app.Mirror().BlockAt(core.Overworld, position); !loaded || got != core.AirID {
			t.Fatalf("下落通道 %+v BlockAt=%d/%v，想要空气/true", position, got, loaded)
		}
	}
	floor := core.BlockPos{X: 0, Y: -1, Z: -3}
	if got, loaded := app.Mirror().BlockAt(core.Overworld, floor); !loaded || got != core.GrassID {
		t.Fatalf("支撑地 BlockAt=%d/%v，想要草地/true（顶面 y=0）", got, loaded)
	}
	const wantFallTicks = 9
	const wantSettledFrames = 3
	if got := breakBurstMotionFrameCount - 1 - breakBurstMotionBreakFrame; got < wantFallTicks+wantSettledFrames {
		t.Fatalf("破坏帧后帧数=%d，想要 ≥%d（%d tick 下落 + ≥%d 帧静置）",
			got, wantFallTicks+wantSettledFrames, wantFallTicks, wantSettledFrames)
	}
}

// TestBreakBurstMotionStaysClearAfterBreak 钉住 F26–49：overlay 保持熄灭
// （裂纹实例无来源）、目标保持空气、掉落留存。
func TestBreakBurstMotionStaysClearAfterBreak(t *testing.T) {
	app := newCaptureAICompanionState()
	app.SetMirror(client.NewMirror())
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app.SetMesher(mesher)
	if err := prepareMiningLifecycleDirt(app); err != nil {
		t.Fatalf("装入泥土目标: %v", err)
	}
	if err := applyMiningCrackCaptureState(app); err != nil {
		t.Fatalf("装入裂纹机位: %v", err)
	}
	for frame := 0; frame < breakBurstMotionFrameCount; frame++ {
		if err := applyMiningLifecycleFrame(app, frame); err != nil {
			t.Fatalf("推进第 %d 帧: %v", frame, err)
		}
		if frame < 25 {
			continue
		}
		if got := app.MiningOverlay(); got != (hud.MiningOverlay{}) {
			t.Fatalf("第 %d 帧 overlay=%+v，想要保持熄灭", frame, got)
		}
		if got, loaded := app.Mirror().BlockAt(core.Overworld, breakBurstMotionTarget); !loaded || got != core.AirID {
			t.Fatalf("第 %d 帧目标 BlockAt=%d/%v，想要保持空气", frame, got, loaded)
		}
	}
	if drops := app.ItemDrops().Presentations(); len(drops) != 1 || drops[0].Item != core.ItemDirt {
		t.Fatalf("尾帧掉落=%+v，想要泥土留存", drops)
	}
}

// TestBreakBurstMotionGIFDecodesTo50Frames 钉住编码产物可解码且帧数符合约定。
func TestBreakBurstMotionGIFDecodesTo50Frames(t *testing.T) {
	frames := make([]*image.NRGBA, 0, breakBurstMotionFrameCount)
	for index := range breakBurstMotionFrameCount {
		img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
		fill := color.NRGBA{R: uint8(index * 5), G: 120, B: 60, A: 255}
		for y := 0; y < 3; y++ {
			for x := 0; x < 4; x++ {
				img.SetNRGBA(x, y, fill)
			}
		}
		frames = append(frames, img)
	}
	data, err := encodeBreakBurstMotionGIF(frames)
	if err != nil {
		t.Fatalf("编码 motion GIF: %v", err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码 motion GIF: %v", err)
	}
	if len(decoded.Image) != breakBurstMotionFrameCount {
		t.Fatalf("解码帧数=%d，想要 %d", len(decoded.Image), breakBurstMotionFrameCount)
	}
	if decoded.Config.Width != 4 || decoded.Config.Height != 3 {
		t.Fatalf("解码尺寸=%dx%d，想要 4x3", decoded.Config.Width, decoded.Config.Height)
	}
}

// TestBreakBurstMotionGIFEncodingIsDeterministic 钉住固定输入→固定字节：
// 同一份 50 帧两次编码逐字节一致。
func TestBreakBurstMotionGIFEncodingIsDeterministic(t *testing.T) {
	frames := make([]*image.NRGBA, 0, breakBurstMotionFrameCount)
	for range breakBurstMotionFrameCount {
		frames = append(frames, image.NewNRGBA(image.Rect(0, 0, 4, 3)))
	}
	first, err := encodeBreakBurstMotionGIF(frames)
	if err != nil {
		t.Fatalf("首次编码 motion GIF: %v", err)
	}
	second, err := encodeBreakBurstMotionGIF(frames)
	if err != nil {
		t.Fatalf("重编码 motion GIF: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("同一 50 帧两次编码字节不一致（%d vs %d 字节）", len(first), len(second))
	}
}

// TestBreakBurstMotionGIFRefusesEmptyFrames 钉住空输入直接失败：
// 坏帧留在产物里只会制造假证据。
func TestBreakBurstMotionGIFRefusesEmptyFrames(t *testing.T) {
	if _, err := encodeBreakBurstMotionGIF(nil); err == nil {
		t.Fatal("空帧编码 motion GIF 想要报错，实际通过")
	}
}

// TestBreakBurstMotionDropIsDirtAtFixedBlock 钉住破坏帧注入的合成掉落：
// 泥土物品（颜色经 `RenderFrame` 走 `ItemColor`）、合法 ID、锚定采掘目标可还原、
// ServerTick 恰为破坏帧 tick（burst 年龄 0 起算点）。
func TestBreakBurstMotionDropIsDirtAtFixedBlock(t *testing.T) {
	upsert := breakBurstMotionDropUpsert()
	if err := upsert.Validate(); err != nil {
		t.Fatalf("合成掉落校验失败: %v", err)
	}
	if len(upsert.Drops) != 1 {
		t.Fatalf("合成掉落数=%d，想要 1", len(upsert.Drops))
	}
	drop := upsert.Drops[0]
	if drop.Item != core.ItemDirt {
		t.Fatalf("合成掉落物品=%d，想要泥土 %d", drop.Item, core.ItemDirt)
	}
	if !drop.ID.Valid() {
		t.Fatalf("合成掉落 ID=%+v 非法", drop.ID)
	}
	wantIndex, ok := world.ChunkBlockIndex(breakBurstMotionTarget)
	if !ok {
		t.Fatalf("合成掉落锚点 %+v 不在区块索引内", breakBurstMotionTarget)
	}
	if drop.BlockIndex != wantIndex {
		t.Fatalf("合成掉落块索引=%d，想要锚点还原值 %d", drop.BlockIndex, wantIndex)
	}
	if upsert.ServerTick != breakBurstMotionTick(breakBurstMotionBreakFrame) {
		t.Fatalf("合成掉落 ServerTick=%d，想要破坏帧 tick %d",
			upsert.ServerTick, breakBurstMotionTick(breakBurstMotionBreakFrame))
	}
}

// TestBreakBurstMotionSceneStaysOutOfCaptureScenes 钉住演示场景不进正式表：
// 世界 PNG 纪律与 `visual-check` 比对都不感知 motion 演示。
func TestBreakBurstMotionSceneStaysOutOfCaptureScenes(t *testing.T) {
	for _, scene := range captureScenes {
		lowered := strings.ToLower(scene.Name)
		if strings.Contains(lowered, "motion") || strings.Contains(lowered, "burst") {
			t.Fatalf("正式场景表混入演示场景 %q", scene.Name)
		}
	}
}
