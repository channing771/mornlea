package capture

import (
	"bytes"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

func TestMotionRecorderRejectsUnboundedBudgets(t *testing.T) {
	for _, count := range []int{0, -1, motionMaxFrames + 1} {
		calls := 0
		if _, err := captureBoundedMotionFrames(count, func(int) (*image.NRGBA, error) { calls++; return image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil }); err == nil || calls != 0 {
			t.Fatalf("预算 %d err=%v calls=%d", count, err, calls)
		}
	}
}

func TestAvatarWalkUsesEqualDistanceAtTwentyHz(t *testing.T) {
	slow := avatarWalkDistance(59) - avatarWalkDistance(19)
	fast := avatarWalkDistance(79) - avatarWalkDistance(59)
	if slow != fast || slow < 4.29 || slow > 4.31 {
		t.Fatalf("慢走=%v 快走=%v", slow, fast)
	}
	if avatarWalkDistance(0) != 0 || avatarWalkDistance(99) != avatarWalkDistance(79) {
		t.Fatal("静止和停稳阶段仍移动")
	}
}

func TestDropDensityCoversBirthGrowthAndRemoval(t *testing.T) {
	for _, point := range []struct{ frame, count int }{{0, 0}, {9, 0}, {10, 1}, {30, 4}, {50, 9}, {70, 16}, {90, 32}, {129, 32}, {130, 16}, {159, 16}} {
		if got := motionDropCount("drop-density", point.frame); got != point.count {
			t.Fatalf("帧 %d 数量=%d，想要 %d", point.frame, got, point.count)
		}
	}
}

func TestMotionPaletteKeepsFlatColorsWithoutDitherNoise(t *testing.T) {
	frame := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			frame.SetNRGBA(x, y, color.NRGBA{R: 127, G: 179, B: 153, A: 255})
		}
	}
	data, err := encodeMotionGIF([]*image.NRGBA{frame}, 5)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	first := decoded.Image[0].At(0, 0)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if decoded.Image[0].At(x, y) != first {
				t.Fatal("纯色区域出现调色抖动噪声")
			}
		}
	}
}

func TestDropMotionInjectsValidMirrorLifecycle(t *testing.T) {
	app := application.NewPresentationApplicationForTest()
	for frame := 0; frame < 160; frame++ {
		if err := applyExperienceMotionFrame(app, "drop-density", frame); err != nil {
			t.Fatalf("帧 %d: %v", frame, err)
		}
		if got, want := len(app.ItemDrops().Presentations()), motionDropCount("drop-density", frame); got != want {
			t.Fatalf("帧 %d 镜像数量=%d，想要%d", frame, got, want)
		}
	}
}
