//go:build darwin

package capture

// capture_scene_hygiene_test.go：跨场景 hygiene 的钉死回归。场景表共用同一个
// application 且没有 teardown 钩子，清场落点只能在后一个场景：雨天场景注入
// 的雨天（呈现侧）必须在后继双机位场景入口回到晴天，双机位场景切走的第三
// 人称必须在后继菜单场景入口回到第一人称。两者都由公共清场
// `resetCapturePresentation` 承载，本文件经真实场景 `Apply` 链路锁定。

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestCaptureRainAndCameraHygieneAcrossScenes 按真实表序走一遍雨天→双机位→
// 雪景的场景链：雨天场景之后后继场景入口必须回晴，双机位场景之后菜单场景入口
// （同样经公共清场）必须回到第一人称，雪景场景二次注入的雨（冬季雪形）必须
// 在菜单场景入口再次回晴；后续正式场景不得继承雨天或自身体。
func TestCaptureRainAndCameraHygieneAcrossScenes(t *testing.T) {
	app := newRainNoonTestApplication(t)

	if err := applyRainNoonCaptureState(app); err != nil {
		t.Fatalf("应用 rain-noon: %v", err)
	}
	if app.Weather() != core.WeatherRain {
		t.Fatalf("rain-noon 后呈现天气 = %d，想要雨天 %d（前置条件不成立）",
			app.Weather(), core.WeatherRain)
	}

	if err := applyCameraThirdBackCaptureState(app); err != nil {
		t.Fatalf("应用 camera-third-back: %v", err)
	}
	if app.Weather() != core.WeatherClear {
		t.Fatalf("camera-third-back 后呈现天气 = %d，想要晴天 %d（雨天泄入后继场景）",
			app.Weather(), core.WeatherClear)
	}
	if app.CameraMode() != client.CameraThirdPersonBack {
		t.Fatalf("camera-third-back 后机位 = %d，想要背面 %d",
			app.CameraMode(), client.CameraThirdPersonBack)
	}

	if err := applyCameraThirdFrontCaptureState(app); err != nil {
		t.Fatalf("应用 camera-third-front: %v", err)
	}
	if app.Weather() != core.WeatherClear {
		t.Fatalf("camera-third-front 后呈现天气 = %d，想要晴天 %d（雨天泄入后继场景）",
			app.Weather(), core.WeatherClear)
	}

	// snow-cover 在双机位之后二次注入雨（冬季形态为雪）：切走的第三人称必须
	// 已由雪景自身的公共清场复位为第一人称，注入的雨随后必须在菜单场景入口
	// 再次回晴。
	if err := applySnowCoverCaptureState(app); err != nil {
		t.Fatalf("应用 snow-cover: %v", err)
	}
	if app.CameraMode() != client.CameraFirstPerson {
		t.Fatalf("snow-cover 入口机位 = %d，想要第一人称 %d（第三人称泄入雪景）",
			app.CameraMode(), client.CameraFirstPerson)
	}
	if app.Weather() != core.WeatherRain {
		t.Fatalf("snow-cover 后呈现天气 = %d，想要雨天 %d（雪形降水的前置不成立）",
			app.Weather(), core.WeatherRain)
	}

	// main-menu 的 `Apply` 经同一公共清场：双机位之后必须回到第一人称，
	// 自身体不泄入后续场景。这里走真实菜单场景的 `Apply`（与抓帧管线同一
	// 落点），菜单场景改坏清场时本测试即失败。
	if err := captureSceneByName(t, "main-menu").Apply(app); err != nil {
		t.Fatalf("应用 main-menu: %v", err)
	}
	if app.CameraMode() != client.CameraFirstPerson {
		t.Fatalf("菜单场景入口机位 = %d，想要第一人称 %d（自身体泄入后继场景）",
			app.CameraMode(), client.CameraFirstPerson)
	}
	if app.Weather() != core.WeatherClear {
		t.Fatalf("菜单场景入口呈现天气 = %d，想要晴天 %d", app.Weather(), core.WeatherClear)
	}
}
