//go:build darwin

package capture

// capture_season_pin_test.go：抓帧季节基线默认锚的钉死回归。抓帧管线的
// `pinCaptureSeasonBaseline` 把呈现侧季节输入钉回无季节基线——镜像季节钉
// 春始分点（SeasonSpring/0）且显示相位偏移清零，使 yearPhase=0、昼弧 12000
// warp 恒等、冷色 tint 权重 0，golden 不随真实世界（seed 派生、随加载时长
// 漂移）的季节相位翻色；rain-noon 改钉的夏至与相位补偿也由它在下一场景
// 复位。

import (
	"testing"

	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestPinCaptureSeasonBaselineAnchorsIdentity 钉住默认锚并验证其恒等性：深冬
// 镜像 + 非零相位补偿被钉回分点零偏移后，yearPhase=0、昼弧 12000，显式
// yearPhase 的昼夜状态与省略参数（无季节基线）逐值一致。
func TestPinCaptureSeasonBaselineAnchorsIdentity(t *testing.T) {
	app := application.NewPresentationApplicationForTest()
	// 预置非分点季节与相位补偿（模拟 rain-noon 场景的改钉残留）：钉前呈现侧
	// yearPhase 应显著偏离 0。
	if err := app.SetCaptureSeason(core.SeasonWinter, 200); err != nil {
		t.Fatalf("预置深冬镜像: %v", err)
	}
	if err := app.SetCaptureDayPhaseOffset(1800); err != nil {
		t.Fatalf("预置相位补偿: %v", err)
	}
	if got := app.YearPhase(); got <= 0.9 {
		t.Fatalf("夹具无效：深冬 yearPhase = %v，想要 > 0.9", got)
	}
	if err := pinCaptureSeasonBaseline(app); err != nil {
		t.Fatalf("钉住抓帧季节基线: %v", err)
	}
	if got := app.YearPhase(); got != 0 {
		t.Fatalf("钉住后的 yearPhase = %v，想要 0（春始分点）", got)
	}
	if got := app.DayPhaseOffset(); got != 0 {
		t.Fatalf("钉住后的显示相位偏移 = %d，想要 0（相位补偿必须复位）", got)
	}
	if arc := core.DayArcTicks(app.YearPhase()); arc != 12000 {
		t.Fatalf("钉住后的昼弧 = %d，想要 12000（warp 恒等）", arc)
	}
	for _, worldTime := range []uint64{0, 6000, 13000, 18000, 23999} {
		if got, want := render.DayNightAt(worldTime, 0, app.YearPhase()), render.DayNightAt(worldTime, 0); got != want {
			t.Fatalf("世界时间 %d 的钉住昼夜状态 = %+v，想要与无季节基线一致 %+v", worldTime, got, want)
		}
	}
}
