//go:build darwin

package render

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestEncodeWeatherInstancesSteadyFrameZeroAlloc 生产入口稳定天气帧零分配：
// `EncodeWeatherInstances`（含 `e.parts` 复用与 `growEncodeBuffer`）在预热后
// 不分配。跨平台的部件级断言见 `TestBuildWeatherPartsSteadyFrameZeroAlloc`
// （`weather_test.go`），此处锁定生产实际调用的路径。
func TestEncodeWeatherInstancesSteadyFrameZeroAlloc(t *testing.T) {
	var encoder InstanceEncoder
	cam := mgl32.Vec3{0, 40, 0}
	dst := encoder.EncodeWeatherInstances(nil, cam, 0, 99, core.WeatherRain, 0, 6000)
	if len(dst) != WeatherMaxParticles*avatarInstanceBytes {
		t.Fatalf("雨天字节数 = %d，想要 %d", len(dst), WeatherMaxParticles*avatarInstanceBytes)
	}
	allocs := testing.AllocsPerRun(20, func() {
		encoder.EncodeWeatherInstances(dst[:0], cam, 0, 99, core.WeatherRain, 0, 6000)
	})
	if allocs != 0 {
		t.Fatalf("生产入口稳定天气帧分配 = %v，想要 0", allocs)
	}
}

// TestEncodeWeatherStateSteadyFrameZeroAlloc 状态段稳定复用零分配：预热后
// 同一 `dst` 的雨天编码不分配；晴天截断同样不分配。
func TestEncodeWeatherStateSteadyFrameZeroAlloc(t *testing.T) {
	dst := EncodeWeatherState(nil, core.WeatherRain)
	if len(dst) != weatherStateBytes {
		t.Fatalf("雨天状态段长度 = %d，想要 %d", len(dst), weatherStateBytes)
	}
	allocs := testing.AllocsPerRun(20, func() {
		EncodeWeatherState(dst[:weatherStateBytes], core.WeatherRain)
	})
	if allocs != 0 {
		t.Fatalf("状态段稳定复用分配 = %v，想要 0", allocs)
	}
	allocs = testing.AllocsPerRun(20, func() {
		EncodeWeatherState(dst[:weatherStateBytes], core.WeatherClear)
	})
	if allocs != 0 {
		t.Fatalf("晴天状态截断分配 = %v，想要 0", allocs)
	}
}
