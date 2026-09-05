//go:build darwin

package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestHumanDetailMaterialsAndCapacity(t *testing.T) {
	for _, kind := range []EntityKind{EntityPlayer, EntityCompanion, EntityHostile, EntityPassive} {
		avatars := make([]Avatar, 75)
		for i := range avatars {
			avatars[i].Key.Kind = kind
			avatars[i].Key.ID[0] = byte(i)
		}
		parts := buildAvatarParts(nil, avatars)
		if len(parts) != 450 || len(avatarPartBytes(parts)) != 450*96 {
			t.Fatal("身体固定容量改变")
		}
		if kind == EntityPlayer || kind == EntityCompanion {
			for _, part := range parts {
				if part.material < 112 || part.material >= 160 || (part.material-112)%6 != 0 {
					t.Fatalf("人类缺少专属分面材质: %d", part.material)
				}
			}
		}
	}
}

func TestLocomotionAccumulatesHorizontalInterpolation(t *testing.T) {
	for _, kind := range []EntityKind{EntityPlayer, EntityCompanion, EntityHostile, EntityPassive} {
		var e InstanceEncoder
		key := EntityKey{Kind: kind}
		emit := func(tick uint64, x, y float32) {
			e.EncodeAvatarInstances(nil, tick, []Avatar{{Key: key, Position: mgl32.Vec3{x, y, 0}}})
		}
		emit(10, 0, 0)
		emit(11, 0.1, 0)
		emit(11, 0.2, 0)
		emit(11, 0.3, 0)
		emit(11, 0.3, 0)
		if math.Abs(float64(e.tracks[key].distance-0.3)) > 1e-6 {
			t.Fatalf("kind %d 同 tick 丢路程: %v", kind, e.tracks[key])
		}
		emit(12, 0.3, 4)
		if math.Abs(float64(e.tracks[key].distance-0.3)) > 1e-6 {
			t.Fatal("垂直运动推进步态")
		}
		emit(13, 0.3, 5)
		if e.ordered[0].Swing != 0 {
			t.Fatal("垂直/停步未回中")
		}
		emit(14, 20, 5)
		if e.tracks[key].distance != 0 || e.ordered[0].Swing != 0 {
			t.Fatal("传送未清零")
		}
		emit(15, 20.1, 5)
		emit(2, 0, 0)
		if e.tracks[key].distance != 0 || e.ordered[0].Swing != 0 {
			t.Fatal("回退未清零")
		}
		e.ResetLocomotion()
		if len(e.tracks) != 0 {
			t.Fatal("重置残留")
		}
	}
}

func TestHumanStrideMatchesLegArc(t *testing.T) {
	want := float32(4 * 0.7 * math.Sin(0.65))
	if math.Abs(float64(avatarStrideLength-want)) > 1e-6 || avatarSwingAmplitude != 0.65 {
		t.Fatalf("人类步幅/摆幅=%v/%v，想要 %v/0.65", avatarStrideLength, avatarSwingAmplitude, want)
	}
}

func TestLocomotionRealSpeedCyclesAndTickGate(t *testing.T) {
	for _, kind := range []EntityKind{EntityPlayer, EntityCompanion, EntityHostile, EntityPassive} {
		drive := func(step float32, frames, segments int) (float32, float32) {
			var e InstanceEncoder
			key := EntityKey{Kind: kind}
			e.EncodeAvatarInstances(nil, 1, []Avatar{{Key: key}})
			for frame := 1; frame <= frames; frame++ {
				for segment := 1; segment <= segments; segment++ {
					x := step * (float32(frame-1) + float32(segment)/float32(segments))
					e.EncodeAvatarInstances(nil, uint64(frame+1), []Avatar{{Key: key, Position: mgl32.Vec3{x, 0, 0}}})
				}
			}
			return e.tracks[key].distance, e.ordered[0].Swing
		}
		distance, angle := drive(.215, 40, 1)
		for _, tc := range []struct {
			step             float32
			frames, segments int
		}{{.1075, 80, 1}, {.215, 40, 3}} {
			d, a := drive(tc.step, tc.frames, tc.segments)
			if math.Abs(float64(d-distance)) > 1e-4 || math.Abs(float64(a-angle)) > 1e-4 {
				t.Fatalf("kind %d 同距相位不一致: %v/%v vs %v/%v", kind, d, a, distance, angle)
			}
		}
		var e InstanceEncoder
		key := EntityKey{Kind: kind}
		e.EncodeAvatarInstances(nil, 1, []Avatar{{Key: key}})
		e.EncodeAvatarInstances(nil, 1, []Avatar{{Key: key, Position: mgl32.Vec3{.1, 0, 0}}})
		if e.tracks[key].speed != 0 {
			t.Fatal("同 tick 修改速度门控")
		}
		e.EncodeAvatarInstances(nil, 2, []Avatar{{Key: key, Position: mgl32.Vec3{.2, 0, 0}}})
		if math.Abs(float64(e.tracks[key].speed-.2)) > 1e-6 {
			t.Fatal("tick 速度未包含前一 tick 的插值")
		}
		e.EncodeAvatarInstances(nil, 3, []Avatar{{Key: key, Position: mgl32.Vec3{.2, 0, 0}}})
		if e.ordered[0].Swing != 0 {
			t.Fatal("权威 tick 静止未回中")
		}
	}
}

func TestLocomotionVerticalOnlyNeverSteps(t *testing.T) {
	var e InstanceEncoder
	key := EntityKey{Kind: EntityPlayer}
	for tick := uint64(1); tick < 20; tick++ {
		e.EncodeAvatarInstances(nil, tick, []Avatar{{Key: key, Position: mgl32.Vec3{0, float32(tick) / 5, 0}}})
		if e.ordered[0].Swing != 0 || e.tracks[key].distance != 0 {
			t.Fatal("垂直序列踏步")
		}
	}
}
