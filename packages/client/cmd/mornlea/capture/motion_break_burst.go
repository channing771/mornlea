package capture

// motion_break_burst.go：完整采掘生命周期的 motion 演示入口。产物是 50 帧 GIF，
// 只验呈现、不进任何比对门禁：场景值不追加进 `captureScenes`（27 张 PNG
// 纪律不动），`RunCapture`/`visual-check`/`visual-update` 都不感知它。
//
// 时间线（帧号 = 合成 tick 偏移，延迟沿用 13cs）：F0–4 目标静置无采掘；
// F5–24 采掘爬坡（`RequiredTicks=200`，`ProgressTicks=(i-5)*10`，裂纹 0→9 扫完）；
// F25 破坏同帧三件事（镜像目标置空 + `MiningOverlay` 熄灭 + 泥土掉落注入，
// burst 年龄 0 从此帧起算）；F25–44 粒子存续 + 掉落下落（3 格落差重力积分约
// 9 tick，F34 着陆）；F34–49 粒子过期、掉落静置留存，裂纹不再出现
// （`mining-crack-presentation` 破坏即清理语义）。
//
// 演示驱动的是真实 `RenderFrame` 破碎链路（掉落物镜像 + 合成 tick 经
// `AppendBreakBurstInstances` 编码），只是 tick 来源是合成推进而非真实
// 无头 tick：真实权威 tick 取决于加载收敛花了多久，随机器速度漂移
// （见 `captureScene` 的注释），演示必须逐帧确定才钉得住 50 帧约定。
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

	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

const (
	// breakBurstMotionFrameCount 是演示 GIF 的固定帧数：5 帧静置 + 20 帧采掘
	// 爬坡 + 破坏帧起重力下落着陆（3 格约 9 tick）+ 余帧静置掉落（含着陆帧本身）。
	breakBurstMotionFrameCount = 50
	// breakBurstMotionTickBase 是合成 tick 序列的起点：取值任意、固定即可，
	// burst 年龄只与相对差有关，绝对值不进画面。
	breakBurstMotionTickBase = uint64(1)
	// breakBurstMotionFrameDelay 是 GIF 单帧延迟（百分之一秒）：约 8fps，
	// 与开发捕获服务的 GIF 口径一致，50 帧循环约 6.5 秒。
	breakBurstMotionFrameDelay = 13
	// breakBurstMotionMiningStartFrame 是采掘爬坡的首帧：之前静置无采掘。
	breakBurstMotionMiningStartFrame = 5
	// breakBurstMotionBreakFrame 是破坏帧：镜像目标置空、采掘熄灭、掉落注入
	// 三件事同帧发生，burst 年龄 0 从此帧起算。
	breakBurstMotionBreakFrame = 25
	// breakBurstMotionMiningRequiredTicks 是演示采掘的总需求 tick：20 帧爬坡
	// 每帧 +10，与破坏帧对齐时恰好差一档（末帧 190/200，阶段 9 不提前破坏）。
	breakBurstMotionMiningRequiredTicks = uint16(200)
	// breakBurstMotionMiningStepTicks 是爬坡每帧的进度步长。
	breakBurstMotionMiningStepTicks = uint16(10)
	// breakBurstMotionSettleMaxFrames 是破坏帧回读前网格重收敛的固定轮次上限：
	// 单格置空只脏数十个区段，单 worker 数轮即收敛；`-race` 下网格化慢一个
	// 数量级，需数十轮，上限按数倍余量取整。轮次固定、无墙钟依赖（刻意不用
	// 抓帧管线的墙钟超时，演示逐字节确定性不随机器速度漂移）；收敛与否只看
	// 网格与上传状态，与 tick 无关，循环内不推进 tick，burst 年龄仍从破坏帧
	// 起算。触及上限仍未收敛直接报错，不回读旧网格的假帧。
	breakBurstMotionSettleMaxFrames = 512
)

// breakBurstMotionTarget 是演示的采掘目标：泥土方块，直接取裂纹场景的目标坐标
// （同一格，机位也沿用裂纹姿势，选框射线天然命中它——裂纹与选框同源门控，
// 不允许摆拍在空气坐标上）；与砖夹具的区别仅在于方块种类。
var breakBurstMotionTarget = captureMiningCrackTarget

// breakBurstMotionScene 是演示的收敛场景值：仅本文件内部使用，绝不追加进
// `captureScenes`。世界夹具是泥土目标（`prepareMiningLifecycleDirt`），
// 呈现状态钉死沿用裂纹场景（`applyMiningCrackCaptureState`：正午 + 同机位 +
// 前序残留清理），不另起一套摆拍。
var breakBurstMotionScene = captureScene{
	Name:         "break-burst-motion",
	WarmupFrames: 8,
	Prepare:      prepareMiningLifecycleDirt,
	Apply:        applyMiningCrackCaptureState,
}

// prepareMiningLifecycleDirt 装入采掘目标与着陆草地：空气邻域基线上在目标
// 坐标摆一格泥土，并在 y=-1 铺一小块草地（顶面 y=0）。写法照抄
// `prepareTargetBlockFeedback`（砖从哪来、怎么进镜像——泥土照此办理），
// 只换方块种类：经 headless mirror 既有写接口进入，不手造第二套世界。
// 草地跨两个区块（x=-2..-1 与 x=0..2 分属相邻区块），按区块各发一条
// `BlockChanges`（版本号都紧随空气快照的 1 → 2）；破坏帧的置空只动目标所在
// 区块（2 → 3），草地区块版本号此后不再变化。
func prepareMiningLifecycleDirt(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	var west, east []network.BlockChange
	for z := int32(-5); z <= -1; z++ {
		for x := int32(-2); x <= 2; x++ {
			change := network.BlockChange{
				Position: core.BlockPos{X: x, Y: -1, Z: z},
				Block:    core.GrassID,
			}
			if x < 0 {
				west = append(west, change)
			} else {
				east = append(east, change)
			}
		}
	}
	if err := applyCaptureMirror(app, network.BlockChanges{
		Dimension:    core.Overworld,
		Chunk:        core.ChunkPos{X: -1, Z: -1},
		BaseRevision: 1,
		NewRevision:  2,
		Changes:      west,
	}); err != nil {
		return err
	}
	east = append(east, network.BlockChange{
		Position: breakBurstMotionTarget,
		Block:    core.DirtID,
	})
	return applyCaptureMirror(app, network.BlockChanges{
		Dimension:    core.Overworld,
		Chunk:        core.ChunkPos{Z: -1},
		BaseRevision: 1,
		NewRevision:  2,
		Changes:      east,
	})
}

// breakBurstMotionTick 把帧号映射为合成 tick：逐帧 +1。
func breakBurstMotionTick(frame int) uint64 {
	return breakBurstMotionTickBase + uint64(frame)
}

// breakBurstMotionOverlay 给出指定帧的采掘镜像：F5–24  active 爬坡，其余帧
// 一律熄灭（F0–4 尚未开掘，F25 起已破坏，裂纹即行清理）。
func breakBurstMotionOverlay(frame int) hud.MiningOverlay {
	if frame < breakBurstMotionMiningStartFrame || frame >= breakBurstMotionBreakFrame {
		return hud.MiningOverlay{}
	}
	return hud.MiningOverlay{
		Active: true, HasTarget: true, Target: breakBurstMotionTarget,
		ProgressTicks: uint16(frame-breakBurstMotionMiningStartFrame) * breakBurstMotionMiningStepTicks,
		RequiredTicks: breakBurstMotionMiningRequiredTicks,
	}
}

// breakBurstMotionClearTarget 构造破坏帧的镜像置空消息：目标格泥土→空气。
// 版本号紧随 `prepareMiningLifecycleDirt`（快照 1 → 装入 2 → 置空 3）：
// 收敛与抓帧期间不再 drain 权威消息，版本号是确定的，不存在竞态。
func breakBurstMotionClearTarget() network.BlockChanges {
	return network.BlockChanges{
		Dimension:    core.Overworld,
		Chunk:        core.ChunkPos{Z: -1},
		BaseRevision: 2,
		NewRevision:  3,
		Changes: []network.BlockChange{{
			Position: breakBurstMotionTarget,
			Block:    core.AirID,
		}},
	}
}

// breakBurstMotionDropUpsert 构造破坏帧注入的合成泥土掉落：单个泥土堆，
// 经与权威消息相同的 `Apply` 入口进入掉落物镜像；颜色由 `RenderFrame` 经
// 既有掉落物/burst 路径走 `ItemColor`，演示侧不另起一套取色。`ServerTick`
// 取破坏帧 tick，与该帧合成 tick 同值——burst 年龄 0 从此帧起算。
func breakBurstMotionDropUpsert() network.ItemDropUpserts {
	blockIndex, _ := world.ChunkBlockIndex(breakBurstMotionTarget)
	return network.ItemDropUpserts{
		ServerTick: breakBurstMotionTick(breakBurstMotionBreakFrame),
		Drops: []network.ItemDrop{{
			ID:         core.DropID{Dimension: core.Overworld, Chunk: breakBurstMotionTarget.Chunk(), Slot: 0, Generation: 1},
			BlockIndex: blockIndex,
			Item:       core.ItemDirt,
			Count:      1,
		}},
	}
}

// applyMiningLifecycleFrame 推进一帧时间线状态：合成 tick 直写 + 采掘镜像直装；
// 破坏帧额外做镜像置空与掉落注入（overlay 熄灭已由 `breakBurstMotionOverlay`
// 覆盖）。调用方随后走真实 `RenderFrame` 抓帧。
func applyMiningLifecycleFrame(app SceneApplication, frame int) error {
	app.SetServerTick(breakBurstMotionTick(frame))
	app.SetMiningOverlay(breakBurstMotionOverlay(frame))
	if frame != breakBurstMotionBreakFrame {
		return nil
	}
	if err := applyCaptureMirror(app, breakBurstMotionClearTarget()); err != nil {
		return fmt.Errorf("破坏帧置空采掘目标: %w", err)
	}
	if err := app.ItemDrops().Apply(breakBurstMotionDropUpsert()); err != nil {
		return fmt.Errorf("破坏帧注入合成泥土掉落: %w", err)
	}
	return nil
}

// settleMotionBreakFrame 把破坏帧的镜像置空落到网格：回读前复用 `captureSettled`
// 判据循环 `RenderFrame`，直到网格与上传队列收敛或触及固定轮次上限。单次
// `RenderFrame` 只推进一轮网格化与上传，置空后立刻回读会抓到重建前的旧网格
// （破坏像素滞后一帧）；`MiningOverlay` 熄灭是 CPU 即时量，不经网格管线，故
// 裂纹消失而方块残留——这正是破坏帧必须先落网格再抓帧的原因。`RenderFrame`
// 不 drain 服务端消息，循环内注入的掉落镜像与钉死的 tick 不被权威消息覆盖。
func settleMotionBreakFrame(app SceneApplication) error {
	for i := 0; i < breakBurstMotionSettleMaxFrames; i++ {
		if _, err := app.RenderFrame(captureDrainMax); err != nil {
			return fmt.Errorf("破坏帧收敛第 %d 轮: %w", i, err)
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
	stats, pending := app.Mesher().Stats(), app.Scheduler().PendingUploads()
	lodBusy := 0
	if app.LODScheduler() != nil {
		lodBusy = app.LODScheduler().Busy()
	}
	return fmt.Errorf("破坏帧 %d 轮内未收敛：mesher=%+v pending=%d lodBusy=%d vista=%d",
		breakBurstMotionSettleMaxFrames, stats, pending, lodBusy, app.MenuVistaPending())
}

// captureMotionFrame 是演示时间线单帧的生产抓帧缝：合成 tick 直写 + 采掘镜像
// 直装（破坏帧兼做镜像置空与掉落注入）→ 破坏帧先落网格 → 真实 `RenderFrame` +
// 回读。`RunBreakBurstMotion` 与回归测试共用它，测试抓到的即产物抓到的。
func captureMotionFrame(app SceneApplication, frame int) (*image.NRGBA, error) {
	// 合成 tick 直写呈现量：`RenderFrame` 不 drain 服务端消息，注入的
	// 掉落镜像与钉死的 tick 在 50 帧内不被任何权威消息覆盖。
	if err := applyMiningLifecycleFrame(app, frame); err != nil {
		return nil, err
	}
	if frame == breakBurstMotionBreakFrame {
		if err := settleMotionBreakFrame(app); err != nil {
			return nil, err
		}
	}
	if _, err := app.RenderFrame(captureDrainMax); err != nil {
		return nil, err
	}
	return bgraToNRGBA(app.Renderer().Readback(), captureWidth, captureHeight), nil
}

// captureBreakBurstMotionFrames 以帧号 0→44 连抓固定帧数：抓帧回调由调用方
// 注入（生产走 `captureMotionFrame`，测试走合成帧），
// 本函数只钉住帧数与帧序，是演示确定性的落点。
func captureBreakBurstMotionFrames(
	capture func(frame int) (*image.NRGBA, error),
) ([]*image.NRGBA, error) {
	frames := make([]*image.NRGBA, 0, breakBurstMotionFrameCount)
	for frame := range breakBurstMotionFrameCount {
		img, err := capture(frame)
		if err != nil {
			return nil, fmt.Errorf("抓取 motion 第 %d 帧（tick %d）: %w", frame, breakBurstMotionTick(frame), err)
		}
		if img == nil {
			return nil, fmt.Errorf("抓取 motion 第 %d 帧（tick %d）: 空帧", frame, breakBurstMotionTick(frame))
		}
		frames = append(frames, img)
	}
	return frames, nil
}

// encodeBreakBurstMotionGIF 把 50 帧 NRGBA 编成 GIF 字节：固定 Plan9 调色板
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

// RunBreakBurstMotion 是 motion 演示的独立入口：收敛世界 → 50 帧时间线连抓
// （合成 tick + 采掘镜像 + 破坏帧置空/掉落注入）→ 标准库编码写盘。只写传入的
// 输出路径那一个文件，不碰 `captureScenes` 与任何 PNG 基线。
func RunBreakBurstMotion(app SceneApplication, outPath string) error {
	if outPath == "" {
		return fmt.Errorf("motion 演示输出路径为空")
	}
	if err := prepareCaptureApplication(app); err != nil {
		return err
	}
	// 昼夜冻结与正式抓帧同一理由：收敛帧期间到达的权威时间会改写钉死的正午，
	// 最终 50 帧的天空光因此随进程启动漂移。
	app.SetWorldTimeFrozen(true)
	defer app.SetWorldTimeFrozen(false)
	// 收敛帧先行（产出帧丢弃）：网格化与上传收敛后，50 帧循环里的画面只随
	// 合成 tick、采掘镜像与 burst 年龄变化，不混入渐进加载像素。掉落不在此处
	// 注入：收敛帧会提前触发 burst 并钉错首现 tick，破坏帧的年龄 0 起算点就错位了。
	if _, err := captureSceneImage(app, breakBurstMotionScene); err != nil {
		return fmt.Errorf("收敛 motion 场景: %w", err)
	}
	frames, err := captureBreakBurstMotionFrames(func(frame int) (*image.NRGBA, error) {
		return captureMotionFrame(app, frame)
	})
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
