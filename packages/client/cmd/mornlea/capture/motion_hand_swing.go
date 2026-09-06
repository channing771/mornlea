package capture

// motion_hand_swing.go：双手挥动两剧本的 motion 演示入口。产物是 120 帧
// GIF，只验呈现、不进任何比对门禁：收敛场景值不追加进 `captureScenes`
//（世界 PNG 纪律独立），`RunCapture`/`visual-check`/`visual-update` 都不
// 感知它。
//
// 时间线（帧号 = 合成 tick 偏移，延迟 5cs 即 20Hz，与权威 tick 同频）：
// hand-mining：铁镐在手、浅裂纹恒定（6/30），右手以镐档周期 10 tick 正弦
// 挥动，120 帧恰好 12 次完整挥动；hand-attack：铁剑在手、每 12 帧合成一次
// 确认沿（编码器窗语义为 6 帧挥动 + 6 帧中立，120 帧对应 10 个窗口沿），确
// 认沿经抓帧专用缝写入，与线上 `CombatHit` 同语义。
//
// tick 来源是合成推进而非真实无头 tick：真实权威 tick 取决于加载收敛花了
// 多久，随机器速度漂移（见 `captureScene` 的注释），演示必须逐帧确定才钉
// 得住 120 帧约定。首帧合成 tick 远小于收敛期的真实 tick，编码器走回退分
// 支重锚，旧相位不延续。

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/channing771/mornlea/packages/client/render/hud"
)

const (
	// handSwingMotionFrameCount 是挥动演示的固定帧数：同时被挖掘周期 10 与
	// 打击重武装周期 12 整除，不断尾。
	handSwingMotionFrameCount = 120
	// handSwingMotionTickBase 是合成 tick 序列的起点：取值任意、固定即可；
	// 远小于收敛期的真实 tick，首帧必走编码器回退重锚。
	handSwingMotionTickBase = uint64(1)
	// handSwingMotionFrameDelay 是 GIF 单帧延迟（百分之一秒）：20Hz，与权
	// 威 tick 同频，120 帧循环 6 秒。
	handSwingMotionFrameDelay = 5
	// handSwingMotionAttackPeriod 是打击重武装周期（帧）：6 帧挥动窗 + 6 帧
	// 中立，循环往复。
	handSwingMotionAttackPeriod = 12
)

// handSwingMotionTick 把帧号映射为合成 tick：逐帧 +1。
func handSwingMotionTick(frame int) uint64 {
	return handSwingMotionTickBase + uint64(frame)
}

// handSwingMotionOverlay 给出挖掘剧本的采掘镜像：全程恒定浅阶段（6/30），
// 裂纹只作背景，挥动由合成 tick 驱动。
func handSwingMotionOverlay(_ int) hud.MiningOverlay {
	return hud.MiningOverlay{
		Active: true, HasTarget: true, Target: captureMiningCrackTarget,
		ProgressTicks: 6, RequiredTicks: 30,
	}
}

// handSwingMotionRearmAttack 报告打击剧本在该帧是否重武装标记：周期首帧武
// 装，窗内 6 帧挥动、窗外 6 帧中立。
func handSwingMotionRearmAttack(frame int) bool {
	return frame%handSwingMotionAttackPeriod == 0
}

// applyHandSwingMotionFrame 推进一帧时间线状态：合成 tick 直写；挖掘剧本
// 重装恒定采掘镜像，打击剧本按周期合成确认沿（`Observe` 既置确认点开编码
// 器窗口、又重武装标记帧数，一举两得）。调用方随后走真实 `RenderFrame` 抓帧。
func applyHandSwingMotionFrame(app SceneApplication, scene string, frame int) error {
	app.SetServerTick(handSwingMotionTick(frame))
	switch scene {
	case "hand-mining":
		app.SetMiningOverlay(handSwingMotionOverlay(frame))
	case "hand-attack":
		if handSwingMotionRearmAttack(frame) {
			app.ObserveCombatHitForCapture(handSwingMotionTick(frame))
		}
	default:
		return fmt.Errorf("未知挥动剧本 %q", scene)
	}
	return nil
}

// handSwingMotionStage 返回指定剧本的收敛场景值：仅本文件内部使用，绝不追
// 加进 `captureScenes`。世界夹具与呈现装入复用静态四景的 Apply（同一机位、
// 同一背包、同一裂纹/标记语义），动态只来自合成 tick 与重武装。
func handSwingMotionStage(scene string) (captureScene, error) {
	switch scene {
	case "hand-mining":
		return captureScene{
			Name:         "hand-mining-motion",
			WarmupFrames: 8,
			Prepare:      prepareTargetBlockFeedback,
			Apply:        applyHandMiningCaptureState,
		}, nil
	case "hand-attack":
		return captureScene{
			Name:         "hand-attack-motion",
			WarmupFrames: 8,
			Prepare:      prepareAICompanion,
			Apply:        applyHandAttackCaptureState,
		}, nil
	default:
		return captureScene{}, fmt.Errorf("未知挥动剧本 %q", scene)
	}
}

// captureHandSwingMotionFrame 是挥动时间线单帧的生产抓帧缝：合成 tick 直写
// + 镜像/标记推进 → 真实 `RenderFrame` + 回读。
func captureHandSwingMotionFrame(app SceneApplication, scene string, frame int) (*image.NRGBA, error) {
	if err := applyHandSwingMotionFrame(app, scene, frame); err != nil {
		return nil, err
	}
	if _, err := app.RenderFrame(captureDrainMax); err != nil {
		return nil, err
	}
	return bgraToNRGBA(app.Renderer().Readback(), captureWidth, captureHeight), nil
}

// RunHandSwingMotion 是挥动演示的独立入口：收敛世界 → 120 帧时间线连抓 →
// 标准库编码写盘。只写传入的输出路径那一个文件（另加旁路 `-frames/` 审查
// 目录），不碰 `captureScenes` 与任何 PNG 基线。
func RunHandSwingMotion(app SceneApplication, outPath, scene string) error {
	stage, err := handSwingMotionStage(scene)
	if err != nil {
		return err
	}
	if outPath == "" {
		return fmt.Errorf("motion 演示输出路径为空")
	}
	if app == nil {
		return fmt.Errorf("motion 演示缺少应用实例")
	}
	if err := prepareCaptureApplication(app); err != nil {
		return err
	}
	// 昼夜冻结与正式抓帧同一理由：收敛帧期间到达的权威时间会改写钉死的正午，
	// 120 帧的天空光因此随进程启动漂移。
	app.SetWorldTimeFrozen(true)
	defer app.SetWorldTimeFrozen(false)
	// 收敛帧先行（产出帧丢弃）：网格化与上传收敛后，循环里的画面只随合成
	// tick、采掘镜像与标记窗口变化，不混入渐进加载像素。
	if _, err := captureSceneImage(app, stage); err != nil {
		return fmt.Errorf("收敛 motion 场景: %w", err)
	}
	// 收敛后丢弃编码器边沿：循环首帧合成 tick 远小于收敛期真实 tick，本来
	// 也会走回退重锚；显式重置让时间线的起点不依赖收敛细节（挖掘上升沿、
	// 攻击窗关闭，首个重武装沿重开）。
	app.ResetViewmodel()
	frames, err := captureBoundedMotionFrames(handSwingMotionFrameCount,
		func(frame int) (*image.NRGBA, error) {
			return captureHandSwingMotionFrame(app, scene, frame)
		})
	if err != nil {
		return err
	}
	frameDir := outPath + "-frames"
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return err
	}
	for index, frame := range frames {
		if index%10 == 0 || index == len(frames)-1 {
			if err := writePNG(filepath.Join(frameDir, fmt.Sprintf("%03d.png", index)), frame); err != nil {
				return err
			}
		}
	}
	data, err := encodeMotionGIF(frames, handSwingMotionFrameDelay)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("创建 motion 输出目录 %s: %w", filepath.Dir(outPath), err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("写出 motion GIF %s: %w", outPath, err)
	}
	fmt.Printf("已生成 motion %s：%s（%d 帧，20Hz）\n", scene, outPath, len(frames))
	return nil
}
