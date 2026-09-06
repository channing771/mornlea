package capture

import (
	"testing"
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
