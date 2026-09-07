//go:build darwin

package app

// app_capture_weather_test.go：抓帧路径天气注入的访问面回归。`SetCaptureWeather`
// 是 capture-only 的呈现天气写口：值域口径与 `Predictor` 和解一致，
// 越界值拒绝且不污染已钉住的值；生产帧循环不消费它（权威天气只走
// `DrainServerMessages` 的更新 tick 纪律）。

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestCaptureWeatherSetterFollowsPredictorCaliber 钉住抓帧天气写口的值域口径：
// 初值晴天，雨天可写，越界值拒绝且保持原值。
func TestCaptureWeatherSetterFollowsPredictorCaliber(t *testing.T) {
	app := NewPresentationApplicationForTest()
	if app.Weather() != core.WeatherClear {
		t.Fatalf("初始呈现天气 = %d，想要晴天 %d", app.Weather(), core.WeatherClear)
	}
	if err := app.SetCaptureWeather(core.WeatherRain); err != nil {
		t.Fatalf("写入雨天: %v", err)
	}
	if app.Weather() != core.WeatherRain {
		t.Fatalf("呈现天气 = %d，想要雨天 %d", app.Weather(), core.WeatherRain)
	}
	if err := app.SetCaptureWeather(core.WeatherThunder + 1); err == nil {
		t.Fatal("越界天气被接受")
	}
	if app.Weather() != core.WeatherRain {
		t.Fatalf("越界写入后呈现天气 = %d，想要保持雨天 %d", app.Weather(), core.WeatherRain)
	}
}
