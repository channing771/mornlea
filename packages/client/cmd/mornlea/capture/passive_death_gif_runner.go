package capture

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

// gifScript 是一条 GIF 动态基线的剧本：与 `captureScenes` 解耦（不进场景表、
// 不占 PNG 顺序），按固定 tick 步进抓帧。Setup 装入静态世界与初始夹具一次，
// Step 推进第 frame 帧的权威镜像（tick 递增的 state/despawn 注入），每帧经
// `Frame(0, …, FixedDelta)` 以固定步长推进呈现（drain 预算为 0：收敛后的夹
// 具不再消费服务端消息，与 PNG 场景“Apply 后不再 drain”同纪律），随后回读
// 一帧。全部时间量来自权威 tick，禁用墙钟。
type gifScript struct {
	Name   string
	Frames int
	Setup  func(SceneApplication) error
	Step   func(app SceneApplication, frame int) error
}

// recordGIFScript 执行一条剧本并返回逐帧 NRGBA：帧预算在首帧捕获之前校验。
func recordGIFScript(app SceneApplication, script gifScript) ([]*image.NRGBA, error) {
	if err := validateGIFFrames(script.Frames); err != nil {
		return nil, err
	}
	if script.Setup == nil || script.Step == nil {
		return nil, fmt.Errorf("GIF 剧本 %s 缺少 Setup 或 Step", script.Name)
	}
	app.DrainServerMessages(captureDrainMax)
	if err := script.Setup(app); err != nil {
		return nil, fmt.Errorf("准备 GIF 剧本 %s: %w", script.Name, err)
	}
	// GIF 剧本看到的必须是干眼画面：PNG 表末位 water-underwater 把预测器浸
	// 没标志写成 true，而同一 application 的预测器在 GIF 全程不再被推进——
	// 不重钉的话水色全屏叠加会渗入全部四条基线（青草地/粉泥土的直接成因：叠
	// 加色 RGBA(0.12,0.34,0.52,0.45) 正好把草绿推向青色；调色板只解释色带不
	// 解释色相漂移，渲染器按残留状态叠加并无辜）。经与水下场景相同的 Apply
	// 入口把预测器钉回牧场空气格，浸没标志按当前镜像重算为 false；本地玩家
	// 不进任何呈现通道，块目标仍由相机射线决定。
	if err := pinGIFPredictorAboveWater(app); err != nil {
		return nil, fmt.Errorf("准备 GIF 剧本 %s 干眼状态: %w", script.Name, err)
	}
	app.SetMenuPhase(application.MenuPhaseGame)
	settleDeadline := time.Now().Add(captureSettleTimeout)
	for i := 0; ; i++ {
		if _, err := app.RenderFrame(captureDrainMax); err != nil {
			return nil, fmt.Errorf("GIF 剧本 %s 收敛第 %d 帧: %w", script.Name, i, err)
		}
		stats, pending := app.Mesher().Stats(), app.Scheduler().PendingUploads()
		lodBusy := 0
		if app.LODScheduler() != nil {
			lodBusy = app.LODScheduler().Busy()
		}
		if i+1 >= captureGlyphSettleFrames && captureSettled(stats, pending, lodBusy, app.MenuVistaPending()) {
			break
		}
		if time.Now().After(settleDeadline) {
			return nil, fmt.Errorf("GIF 剧本 %s 未收敛：mesher=%+v pending=%d lodBusy=%d",
				script.Name, stats, pending, lodBusy)
		}
	}
	frames := make([]*image.NRGBA, 0, script.Frames)
	for frame := 0; frame < script.Frames; frame++ {
		if err := script.Step(app, frame); err != nil {
			return nil, fmt.Errorf("GIF 剧本 %s 第 %d 帧: %w", script.Name, frame, err)
		}
		// 固定步长推进一帧：drain 预算为 0（夹具不受服务端消息覆盖），呈现
		// 插值恰好前进一个 `FixedDelta`，与机器速度无关。
		if _, err := app.Frame(0, captureDrainMax, physics.FixedDelta); err != nil {
			return nil, fmt.Errorf("GIF 剧本 %s 步进第 %d 帧: %w", script.Name, frame, err)
		}
		frames = append(frames, bgraToNRGBA(app.Renderer().Readback(), captureWidth, captureHeight))
	}
	return frames, nil
}

// pinGIFPredictorAboveWater 把预测器钉回牧场空气格并重算浸没标志。tick 取
// 1<<21：预测器只接受单调递增的状态，必须大于水下场景的 1<<20 钉点。位置取
// 相机下方牧场空气格（四剧本同机位同牧场 footprint，脚与眼皆为空气）。
func pinGIFPredictorAboveWater(app SceneApplication) error {
	if app.Predictor() == nil || app.Mirror() == nil {
		return fmt.Errorf("gif 干眼重钉需要预测器与世界镜像")
	}
	position := mgl32.Vec3{0.5, 2, 6.5}
	if _, err := app.Predictor().ApplyPlayerState(network.PlayerState{
		ServerTick:     1 << 21,
		Dimension:      core.Overworld,
		Position:       position,
		Yaw:            0,
		Pitch:          0,
		Ready:          true,
		Health:         core.MaxHealth,
		Oxygen:         core.MaxOxygenTicks,
		WorldTimeTicks: 6000,
	}, client.MirrorCollisionSource{
		Mirror:    app.Mirror(),
		Dimension: core.Overworld,
	}); err != nil {
		return fmt.Errorf("钉回预测器: %w", err)
	}
	if app.Predictor().EyeInFluid() {
		return fmt.Errorf("gif 干眼重钉后仍在流体中（位置 %v），水色会渗入基线", position)
	}
	return nil
}

// settleGIFMesher 在剧本中途写块后把单格 remesh 泵送落盘：网格化由 worker
// 异步完成，不泵送的话 remesh 落在哪一捕获帧完全看机器速度，结算帧边界必漂。
// 泵送只重复渲染当前 tick（不推进权威时间），收敛后交出，后续 `Frame` 回读
// 的即是落盘后的画面。
func settleGIFMesher(app SceneApplication) error {
	for i := 0; i < 120; i++ {
		if _, err := app.RenderFrame(captureDrainMax); err != nil {
			return fmt.Errorf("GIF 写块后收敛第 %d 帧: %w", i, err)
		}
		stats, pending := app.Mesher().Stats(), app.Scheduler().PendingUploads()
		lodBusy := 0
		if app.LODScheduler() != nil {
			lodBusy = app.LODScheduler().Busy()
		}
		if captureSettled(stats, pending, lodBusy, app.MenuVistaPending()) {
			return nil
		}
	}
	return fmt.Errorf("GIF 写块后 120 帧内未收敛")
}

// captureGIFOne 录制一条剧本并落盘/比对：实拍 GIF 无条件写进 dir；更新模式
// 写基线，否则逐帧解码比对（双阈值，全部帧通过方为通过）。
func captureGIFOne(app SceneApplication, dir string, script gifScript, updateGolden bool) error {
	frames, err := recordGIFScript(app, script)
	if err != nil {
		return err
	}
	data, err := encodeGIF(frames)
	if err != nil {
		return err
	}
	if err := writeGIFFile(dir, script.Name, data); err != nil {
		return err
	}
	if updateGolden {
		if err := writeGIFFile(passiveDeathGoldenDir, script.Name, data); err != nil {
			return err
		}
		fmt.Printf("已抓取 GIF 剧本 %s(写入基线)\n", script.Name)
		return nil
	}
	diff, err := compareGIFAgainstGolden(passiveDeathGoldenDir, dir, script.Name, frames, captureThresholds)
	fmt.Printf("已抓取 GIF 剧本 %s: %s\n", script.Name, diff)
	return err
}

func writeGIFFile(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 GIF 输出目录 %s: %w", dir, err)
	}
	path := filepath.Join(dir, name+".gif")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写出 GIF %s: %w", path, err)
	}
	return nil
}

// RunPassiveDeathGIFs 依次录制全部 GIF 剧本：与 PNG 场景表共用同一个已预热
// 的 application（`RunCapture` 内调用，世界时间已冻结），各剧本经公共清理
// 互不渗透；错误累积后统一返回，与 `RunCapture` 同纪律。
func RunPassiveDeathGIFs(app SceneApplication, dir string, updateGolden bool) error {
	var errs []error
	for _, script := range passiveDeathGIFScripts {
		if err := captureGIFOne(app, dir, script, updateGolden); err != nil {
			errs = append(errs, fmt.Errorf("GIF 剧本 %s: %w", script.Name, err))
		}
	}
	return errors.Join(errs...)
}

// applyGIFPastureCamera 是四条剧本共用的已验证机位（牛群场景机位）：钉在草
// 地上方平视牛群，近处掉落与远处牛群同框。
func applyGIFPastureCamera(app SceneApplication) {
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{0.5, 2.8, 6.5}, Yaw: 0, Pitch: -0.12,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetBlockTargetReset(false)
}

// gifDropUpsert 构造单掉落的权威 upsert 批次。
func gifDropUpsert(tick uint64, block core.BlockPos, item core.ItemID) (network.ItemDropUpserts, error) {
	blockIndex, ok := world.ChunkBlockIndex(block)
	if !ok {
		return network.ItemDropUpserts{}, fmt.Errorf("GIF 掉落锚点 %+v 不在区块索引内", block)
	}
	return network.ItemDropUpserts{
		ServerTick: tick,
		Drops: []network.ItemDrop{{
			ID:         core.DropID{Dimension: core.Overworld, Chunk: block.Chunk(), Slot: 0, Generation: 1},
			BlockIndex: blockIndex,
			Item:       item,
			Count:      1,
		}},
	}, nil
}
