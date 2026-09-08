package render

import (
	"bytes"
	"github.com/channing771/mornlea/packages/shared/core"
	"testing"
	"time"
)

func viewmodelTestInput(player core.PlayerID, stack core.ItemStack, _ uint64) *ViewmodelInput {
	return &ViewmodelInput{Player: player, Selected: stack}
}

func TestViewmodelElapsedMotionRatesReplayAndLongHold(t *testing.T) {
	for tier := ViewmodelTierEmptyHand; tier <= ViewmodelTierAxe; tier++ {
		for _, total := range []time.Duration{100 * time.Millisecond, 350 * time.Millisecond, 2100 * time.Millisecond, 24*time.Hour + 123*time.Millisecond} {
			var direct ViewmodelMotion
			direct.Advance(0, true, tier)
			direct.Advance(total, true, tier)
			wa, wp := direct.Phase()
			for _, hz := range []int{30, 60, 144} {
				var motion ViewmodelMotion
				motion.Advance(0, true, tier)
				elapsed := time.Duration(0)
				// 长时间挂起一次跳过，随后仍按真实帧间隔推进，工作量不随历史周期增长。
				if total > time.Hour {
					motion.Advance(24*time.Hour, true, tier)
					elapsed = 24 * time.Hour
				}
				for elapsed < total {
					step := min(time.Second/time.Duration(hz), total-elapsed)
					motion.Advance(step, true, tier)
					elapsed += step
				}
				ga, gp := motion.Phase()
				if ga != wa || gp != wp {
					t.Fatalf("tier %d hz %d total %v phase %v != %v", tier, hz, total, gp, wp)
				}
				var e ViewmodelEncoder
				in := ViewmodelInput{SwingActive: ga, SwingPhase: gp, Selected: core.ItemStack{Item: core.ItemIronSword, Count: 1}}
				first := e.EncodeViewmodelInstances(nil, &in)
				for i := 0; i < 10; i++ {
					if !bytes.Equal(first, e.EncodeViewmodelInstances(nil, &in)) {
						t.Fatal("confirmation/frame changed elapsed pose")
					}
				}
			}
		}
	}
}

func TestViewmodelClickMotionRepeatedClicksCompleteStroke(t *testing.T) {
	var m ViewmodelMotion
	m.Advance(0, true, ViewmodelTierSword)
	m.Advance(100*time.Millisecond, false, ViewmodelTierSword)
	_, before := m.Phase()
	m.Advance(0, true, ViewmodelTierSword)
	if _, after := m.Phase(); after != before {
		t.Fatal("repeat truncated stroke")
	}
	m.Advance(300*time.Millisecond, false, ViewmodelTierSword)
	if active, _ := m.Phase(); active {
		t.Fatal("release failed to complete")
	}
	m.Advance(0, true, ViewmodelTierSword)
	if active, phase := m.Phase(); !active || phase != 0 {
		t.Fatal("fresh click failed")
	}
	m.Advance(-time.Second, true, ViewmodelTierSword)
	if _, phase := m.Phase(); phase != 0 {
		t.Fatal("negative time moved phase")
	}
}

func TestViewmodelClickTrajectoryHasForwardDownstroke(t *testing.T) {
	in := ViewmodelInput{}
	neutral := viewmodelGripRoot(&in, 0)
	windup := viewmodelGripRoot(&in, ViewmodelClickAngle(true, .2, ViewmodelTierEmptyHand))
	hit := viewmodelGripRoot(&in, ViewmodelClickAngle(true, .48, ViewmodelTierEmptyHand))
	if !(hit[14] < neutral[14]-.08 && hit[13] < windup[13]-.035) {
		t.Fatalf("missing forward/down displacement: rest %v windup %v hit %v", neutral.Col(3), windup.Col(3), hit.Col(3))
	}
	if ViewmodelClickAngle(true, 0, ViewmodelTierEmptyHand) == 0 || ViewmodelClickAngle(true, 1, ViewmodelTierEmptyHand) != 0 {
		t.Fatal("missing immediate windup or recovery")
	}
}
