package capture

// motion_break_burst.go：破碎 burst 的 motion 演示入口。产物是 24 帧 GIF，
// 只验呈现、不进任何比对门禁：场景值不追加进 `captureScenes`（27 张 PNG
// 纪律不动），`RunCapture`/`visual-check`/`visual-update` 都不感知它。
//
// 演示驱动的是真实 `RenderFrame` 破碎链路（掉落物镜像 + 合成 tick 经
// `AppendBreakBurstInstances` 编码），只是 tick 来源是合成推进而非真实
// 无头 tick：真实权威 tick 取决于加载收敛花了多久，随机器速度漂移
// （见 `captureScene` 的注释），演示必须逐帧确定才钉得住 24 帧约定。
// 链路本身已有逐帧编码测试与接线测试覆盖，这里不管链路、只管呈现。

import (
	"bytes"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

const (
	// breakBurstMotionFrameCount 是演示 GIF 的固定帧数：20 tick 粒子寿命 +
	// 首尾各 2 帧余量（首帧 burst 诞生、尾帧过期清空都有像素证据）。
	breakBurstMotionFrameCount = 24
	// breakBurstMotionTickBase 是合成 tick 序列的起点：取值任意、固定即可，
	// burst 年龄只与相对差有关，绝对值不进画面。
	breakBurstMotionTickBase = uint64(1)
	// breakBurstMotionFrameDelay 是 GIF 单帧延迟（百分之一秒）：约 8fps，
	// 与开发捕获服务的 GIF 口径一致，24 帧循环约 3.1 秒。
	breakBurstMotionFrameDelay = 13
)

// breakBurstMotionDropBlock 是合成泥土掉落的锚定方块：与牛群场景验证过的
// 掉落机位同值，相机姿态也沿用该机位，burst 粒子落在画面近景中央。
var breakBurstMotionDropBlock = core.BlockPos{X: 0, Y: 1, Z: 3}

// breakBurstMotionScene 是演示的收敛场景值：仅本文件内部使用，绝不追加进
// `captureScenes`。世界夹具复用牛群场景的草地（空气邻域 + y=0 草地条），
// 呈现实体（牛群/掉落）一律不装，画面里只有草地与第 0 帧后注入的合成掉落。
var breakBurstMotionScene = captureScene{
	Name:         "break-burst-motion",
	WarmupFrames: 8,
	Prepare:      preparePassiveHerdDay,
	Apply:        applyBreakBurstMotionState,
}

// applyBreakBurstMotionState 钉死演示的全部呈现状态：固定正午、牛群场景的
// 已验证机位、前序场景残留清空（与牛群 `Apply` 同一份清理语义，但不注入
// 任何实体——合成掉落在收敛之后、`RunBreakBurstMotion` 的抓帧循环之前注入）。
func applyBreakBurstMotionState(app SceneApplication) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{0.5, 2.8, 6.5}, Yaw: 0, Pitch: -0.12,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetBlockTargetReset(false)
	return nil
}

// breakBurstMotionDropUpsert 构造第 0 帧注入的合成泥土掉落：单个泥土堆，
// 经与权威消息相同的 `Apply` 入口进入掉落物镜像；颜色由 `RenderFrame` 经
// 既有掉落物/burst 路径走 `ItemColor`，演示侧不另起一套取色。
func breakBurstMotionDropUpsert() network.ItemDropUpserts {
	blockIndex, _ := world.ChunkBlockIndex(breakBurstMotionDropBlock)
	return network.ItemDropUpserts{
		ServerTick: breakBurstMotionTickBase,
		Drops: []network.ItemDrop{{
			ID:         core.DropID{Dimension: core.Overworld, Chunk: breakBurstMotionDropBlock.Chunk(), Slot: 0, Generation: 1},
			BlockIndex: blockIndex,
			Item:       core.ItemDirt,
			Count:      1,
		}},
	}
}

// captureBreakBurstMotionFrames 以合成 tick 逐帧 +1 连抓固定帧数：抓帧回调
// 由调用方注入（生产走真实 `RenderFrame` + 回读，测试走合成帧），本函数只
// 钉住帧数与 tick 序列，是演示确定性的落点。
func captureBreakBurstMotionFrames(
	capture func(tick uint64) (*image.NRGBA, error),
	baseTick uint64,
) ([]*image.NRGBA, error) {
	frames := make([]*image.NRGBA, 0, breakBurstMotionFrameCount)
	for index := range breakBurstMotionFrameCount {
		tick := baseTick + uint64(index)
		frame, err := capture(tick)
		if err != nil {
			return nil, fmt.Errorf("抓取 motion 第 %d 帧（tick %d）: %w", index, tick, err)
		}
		if frame == nil {
			return nil, fmt.Errorf("抓取 motion 第 %d 帧（tick %d）: 空帧", index, tick)
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

// encodeBreakBurstMotionGIF 把 24 帧 NRGBA 编成 GIF 字节：固定 Plan9 调色板
// + Floyd-Steinberg 抖动（与开发捕获服务的 GIF 口径同源，但 capture 不得
// 导入 devcapture，方向由架构门禁强制，故此处保留小体量重复）。固定调色板
// 使同输入逐字节一致；空输入直接失败，不产出零帧 GIF 假证据。
func encodeBreakBurstMotionGIF(frames []*image.NRGBA) ([]byte, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("编码 motion GIF：空帧序列")
	}
	out := &gif.GIF{Delay: make([]int, 0, len(frames))}
	for index, frame := range frames {
		if frame == nil {
			return nil, fmt.Errorf("编码 motion GIF：第 %d 帧为空", index)
		}
		paletted := image.NewPaletted(frame.Bounds(), palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, frame.Bounds(), frame, image.Point{})
		out.Image = append(out.Image, paletted)
		out.Delay = append(out.Delay, breakBurstMotionFrameDelay)
	}
	first := out.Image[0]
	out.Config = image.Config{
		Width:      first.Bounds().Dx(),
		Height:     first.Bounds().Dy(),
		ColorModel: first.Palette,
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil {
		return nil, fmt.Errorf("编码 motion GIF：%w", err)
	}
	return buf.Bytes(), nil
}

// RunBreakBurstMotion 是 motion 演示的独立入口：收敛世界 → 第 0 帧注入合成
// 泥土掉落 → 合成 tick 连抓 24 帧 → 标准库编码写盘。只写传入的输出路径那一个文件，
// 不碰 `captureScenes` 与任何 PNG 基线。
func RunBreakBurstMotion(app SceneApplication, outPath string) error {
	if outPath == "" {
		return fmt.Errorf("motion 演示输出路径为空")
	}
	if err := prepareCaptureApplication(app); err != nil {
		return err
	}
	// 昼夜冻结与正式抓帧同一理由：收敛帧期间到达的权威时间会改写钉死的正午，
	// 最终 24 帧的天空光因此随进程启动漂移。
	app.SetWorldTimeFrozen(true)
	defer app.SetWorldTimeFrozen(false)
	// 收敛帧先行（产出帧丢弃）：网格化与上传收敛后，24 帧循环里的画面只随
	// 合成 tick 与 burst 年龄变化，不混入渐进加载像素。
	if _, err := captureSceneImage(app, breakBurstMotionScene); err != nil {
		return fmt.Errorf("收敛 motion 场景: %w", err)
	}
	if err := app.ItemDrops().Apply(breakBurstMotionDropUpsert()); err != nil {
		return fmt.Errorf("注入合成泥土掉落: %w", err)
	}
	frames, err := captureBreakBurstMotionFrames(func(tick uint64) (*image.NRGBA, error) {
		// 合成 tick 直写呈现量：`RenderFrame` 不 drain 服务端消息，注入的
		// 掉落镜像与钉死的 tick 在 24 帧内不被任何权威消息覆盖。
		app.SetServerTick(tick)
		if _, err := app.RenderFrame(captureDrainMax); err != nil {
			return nil, err
		}
		return bgraToNRGBA(app.Renderer().Readback(), captureWidth, captureHeight), nil
	}, breakBurstMotionTickBase)
	if err != nil {
		return err
	}
	data, err := encodeBreakBurstMotionGIF(frames)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("创建 motion 输出目录 %s: %w", filepath.Dir(outPath), err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("写出 motion GIF %s: %w", outPath, err)
	}
	fmt.Printf("已生成 motion 演示 %s（%d 帧，%d 字节）\n", outPath, len(frames), len(data))
	return nil
}
