//go:build darwin

package app

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestSeasonalYearPhaseReconstructionAnchors 量化重建锚点：各季季首落在年相位
// 的 1/4 整分点、季中为半季、冬末 255 不越过年末，量化步长恰 1/1024（进度
// u8 的一个台阶在年相位上的宽度）。
func TestSeasonalYearPhaseReconstructionAnchors(t *testing.T) {
	tests := []struct {
		name     string
		season   core.Season
		progress uint8
		want     float64
	}{
		{"春始", core.SeasonSpring, 0, 0},
		{"春半", core.SeasonSpring, 128, 0.125},
		{"夏始", core.SeasonSummer, 0, 0.25},
		{"秋始", core.SeasonAutumn, 0, 0.5},
		{"冬始", core.SeasonWinter, 0, 0.75},
		{"冬末", core.SeasonWinter, 255, (3 + 255.0/256) / 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := seasonalYearPhase(tc.season, tc.progress); math.Abs(got-tc.want) > 1e-12 {
				t.Fatalf("seasonalYearPhase(%v,%d) = %v，想要 %v", tc.season, tc.progress, got, tc.want)
			}
		})
	}
	step := seasonalYearPhase(core.SeasonSpring, 1) - seasonalYearPhase(core.SeasonSpring, 0)
	if math.Abs(step-1.0/1024) > 1e-12 {
		t.Fatalf("量化步长 = %v，想要 1/1024", step)
	}
}

// TestApplicationYearPhaseReadsMirrorAndCapturePin 呈现侧年相位读镜像两字段：
// 零值即春始分点（warp 恒等基线），capture 写口可直写钉住（含冬末深冬），
// 越界季节被拒绝且不污染已钉住的值。
func TestApplicationYearPhaseReadsMirrorAndCapturePin(t *testing.T) {
	app := NewPresentationApplicationForTest()
	if got := app.YearPhase(); got != 0 {
		t.Fatalf("零值镜像的 yearPhase = %v，想要 0（春始分点）", got)
	}
	if err := app.SetCaptureSeason(core.SeasonWinter, 128); err != nil {
		t.Fatalf("capture 季节直写失败: %v", err)
	}
	want := (3 + 128.0/256) / 4
	if got := app.YearPhase(); math.Abs(got-want) > 1e-12 {
		t.Fatalf("深冬镜像的 yearPhase = %v，想要 %v", got, want)
	}
	if err := app.SetCaptureSeason(core.Season(4), 0); err == nil {
		t.Fatal("越界季节未被拒绝")
	}
	if got := app.YearPhase(); math.Abs(got-want) > 1e-12 {
		t.Fatalf("越界写入污染了已钉值：yearPhase = %v，想要保持 %v", got, want)
	}
}
