//go:build darwin

package capture

// motion_break_burst_test.go：完整采掘生命周期 motion 演示的回归测试。
// 45 帧时间线：F0–4 静置 → F5–24 采掘爬坡（裂纹 0→9 扫完）→ F25 破坏同帧
// （镜像置空 + overlay 熄灭 + 泥土掉落注入，burst 年龄 0 起算）→ F25–44
// 粒子存续 + 掉落留存、裂纹不再出现。链路正确性由破碎 burst 的逐帧测试与
// `RenderFrame` 接线测试承接，这里只钉时间线落点与产物约定。

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

// TestBreakBurstMotionCaptures45FramesInOrder 钉住演示抓帧的帧约定：45 帧、
// 帧号 0→44 顺序递进、同一固定序列两次抓取逐帧字节一致。
func TestBreakBurstMotionCaptures45FramesInOrder(t *testing.T) {
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
	if breakBurstMotionFrameCount != 45 {
		t.Fatalf("motion 帧数常量=%d，想要 45", breakBurstMotionFrameCount)
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

// TestBreakBurstMotionStaysClearAfterBreak 钉住 F26–44：overlay 保持熄灭
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

// TestBreakBurstMotionGIFDecodesTo45Frames 钉住编码产物可解码且帧数符合约定。
func TestBreakBurstMotionGIFDecodesTo45Frames(t *testing.T) {
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
// 同一份 45 帧两次编码逐字节一致。
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
		t.Fatalf("同一 45 帧两次编码字节不一致（%d vs %d 字节）", len(first), len(second))
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
// 27 张 PNG 纪律与 `visual-check` 比对都不感知 motion 演示。
func TestBreakBurstMotionSceneStaysOutOfCaptureScenes(t *testing.T) {
	for _, scene := range captureScenes {
		lowered := strings.ToLower(scene.Name)
		if strings.Contains(lowered, "motion") || strings.Contains(lowered, "burst") {
			t.Fatalf("正式场景表混入演示场景 %q", scene.Name)
		}
	}
}
