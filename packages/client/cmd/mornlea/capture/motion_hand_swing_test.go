package capture

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/client/render/hud"
)

// 本文件覆盖手持挥动两剧本（挖掘/打击）的确定性映射：合成 tick、采掘镜像
// 与标记重武装周期，以及剧本与 PNG 场景表、主 dispatch 的接线。

func TestHandSwingMotionTickAdvancesOnePerFrame(t *testing.T) {
	if handSwingMotionTick(0) != handSwingMotionTickBase {
		t.Fatalf("首帧 tick=%d，想要基点 %d", handSwingMotionTick(0), handSwingMotionTickBase)
	}
	for _, frame := range []int{1, 11, 119} {
		if handSwingMotionTick(frame) != handSwingMotionTickBase+uint64(frame) {
			t.Fatalf("帧 %d tick=%d，想要逐帧加一", frame, handSwingMotionTick(frame))
		}
	}
}

func TestHandSwingMotionMiningOverlayStaysShallow(t *testing.T) {
	for _, frame := range []int{0, 1, 59, 119} {
		overlay := handSwingMotionOverlay(frame)
		if !overlay.Active || !overlay.HasTarget || overlay.Target != captureMiningCrackTarget ||
			overlay.ProgressTicks != 6 || overlay.RequiredTicks != 30 {
			t.Fatalf("帧 %d overlay=%+v，想要恒定浅阶段 6/30", frame, overlay)
		}
	}
}

func TestHandSwingMotionAttackRearmsEveryTwelveFrames(t *testing.T) {
	for frame := 0; frame < handSwingMotionFrameCount; frame++ {
		want := frame%handSwingMotionAttackPeriod == 0
		if handSwingMotionRearmAttack(frame) != want {
			t.Fatalf("帧 %d 重武装=%v，想要 %v（周期 %d）",
				frame, handSwingMotionRearmAttack(frame), want, handSwingMotionAttackPeriod)
		}
	}
}

func TestHandSwingMotionFrameBudgetFitsRecorder(t *testing.T) {
	if handSwingMotionFrameCount <= 0 || handSwingMotionFrameCount > motionMaxFrames {
		t.Fatalf("挥动剧本帧数=%d，想要落在录制循环预算内（1..%d）",
			handSwingMotionFrameCount, motionMaxFrames)
	}
	// 挥动周期与录制长度互质无关：镐周期 10 tick 下 120 帧恰好 12 次完整挥
	// 动，打击按 12 帧周期重武装恰好 10 次完整挥动，不断尾。
	if handSwingMotionFrameCount%10 != 0 || handSwingMotionFrameCount%handSwingMotionAttackPeriod != 0 {
		t.Fatalf("帧数=%d，想要同时被挖掘周期 10 与打击周期 %d 整除",
			handSwingMotionFrameCount, handSwingMotionAttackPeriod)
	}
}

func TestHandSwingMotionScenesStayOutOfCaptureScenes(t *testing.T) {
	for _, scene := range captureScenes {
		if scene.Name == "hand-mining-motion" || scene.Name == "hand-attack-motion" {
			t.Fatalf("motion 收敛场景 %q 不得进入 PNG 场景表", scene.Name)
		}
	}
}

func TestRunMotionRejectsUnknownHandScene(t *testing.T) {
	if err := RunHandSwingMotion(nil, "", "unknown"); err == nil {
		t.Fatal("未知挥动剧本想要报错，实际通过")
	}
}

// handSwingMotionRecorder 记录时间线驱动的调用：内嵌接口只覆写驱动实际调
// 用的三个方法，其余保持 nil（驱动不碰它们）。
type handSwingMotionRecorder struct {
	SceneApplication
	ticks    []uint64
	observed []uint64
	overlays int
}

func (r *handSwingMotionRecorder) SetServerTick(tick uint64) { r.ticks = append(r.ticks, tick) }

func (r *handSwingMotionRecorder) ObserveCombatHitForCapture(tick uint64) {
	r.observed = append(r.observed, tick)
}

func (r *handSwingMotionRecorder) SetMiningOverlay(_ hud.MiningOverlay) { r.overlays++ }

// TestHandSwingMotionFrameDrivesCaptureSeam 锁定时间线驱动与抓帧缝的接线：
// 打击剧本只在重武装帧合成确认沿（沿值即该帧合成 tick，严格递增），挖掘剧
// 本逐帧重装采掘镜像且永不合成沿。
func TestHandSwingMotionFrameDrivesCaptureSeam(t *testing.T) {
	attack := &handSwingMotionRecorder{}
	for frame := 0; frame < 2*handSwingMotionAttackPeriod; frame++ {
		if err := applyHandSwingMotionFrame(attack, "hand-attack", frame); err != nil {
			t.Fatalf("帧 %d: %v", frame, err)
		}
	}
	if len(attack.ticks) != 2*handSwingMotionAttackPeriod {
		t.Fatalf("合成 tick 推进=%d，想要逐帧一次共 %d", len(attack.ticks), 2*handSwingMotionAttackPeriod)
	}
	want := []uint64{handSwingMotionTick(0), handSwingMotionTick(handSwingMotionAttackPeriod)}
	if !slices.Equal(attack.observed, want) {
		t.Fatalf("合成确认沿=%v，想要 %v（严格递增）", attack.observed, want)
	}

	mining := &handSwingMotionRecorder{}
	for frame := 0; frame < 2*handSwingMotionAttackPeriod; frame++ {
		if err := applyHandSwingMotionFrame(mining, "hand-mining", frame); err != nil {
			t.Fatalf("帧 %d: %v", frame, err)
		}
	}
	if mining.overlays != 2*handSwingMotionAttackPeriod {
		t.Fatalf("采掘镜像重装=%d，想要逐帧一次", mining.overlays)
	}
	if len(mining.observed) != 0 {
		t.Fatalf("挖掘剧本合成确认沿=%v，想要永不合成", mining.observed)
	}
}
