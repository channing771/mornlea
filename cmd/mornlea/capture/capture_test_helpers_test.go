//go:build darwin

package capture

// capture_test_helpers_test.go：capture 场景主题测试的共享夹具（场景表名称
// 查找、AI 伙伴确定性呈现状态的构造与断言）。白盒构造下沉为 app 包的导出
// 测试装配入口 `application.NewPresentationApplicationForTest`；状态断言经
// `Application` 的导出访问面读取，语义与原字段访问一致。

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// `captureSceneByName` 为 capture 场景主题测试提供唯一名称查找，并同时钉住
// 场景表不存在重名项。
func captureSceneByName(t *testing.T, name string) captureScene {
	t.Helper()
	var matches []captureScene
	for _, scene := range captureScenes {
		if scene.Name == name {
			matches = append(matches, scene)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("capture scene %q count=%d，想要 1", name, len(matches))
	}
	return matches[0]
}

// `newCaptureAICompanionState` 为场景顺序与 AI 伙伴主题测试构造共同的最小应用状态。
func newCaptureAICompanionState() *application.Application {
	return application.NewPresentationApplicationForTest()
}

// `assertCaptureAICompanionState` 为场景顺序与 AI 伙伴主题测试断言共同的确定性状态。
func assertCaptureAICompanionState(t *testing.T, app *application.Application) {
	t.Helper()
	wantCamera := client.Camera{
		Pos: mgl32.Vec3{5.5, 3.2, 9.5}, Yaw: 0, Pitch: -0.05,
		FovY: mgl32.DegToRad(70), Aspect: float32(application.CaptureWidth) / application.CaptureHeight,
		Near: 0.1, Far: 2000,
	}
	if app.WorldTimeTicks() != 6000 || *app.Camera() != wantCamera {
		t.Fatalf("固定环境 time=%d camera=%+v，想要 6000/%+v",
			app.WorldTimeTicks(), *app.Camera(), wantCamera)
	}
	presentations := app.Companions().AppendPresentations(nil)
	wantID := companion.ID{0: 0x42, 6: 0x40, 8: 0x80, 15: 0x14}
	if len(presentations) != 1 || presentations[0] != (client.CompanionPresentation{
		ID: wantID, Name: "阿木", Dimension: core.Overworld,
		Position: mgl32.Vec3{5.5, 1, 4}, Yaw: 3.1415927,
	}) {
		t.Fatalf("companion presentations=%+v", presentations)
	}
	events := app.ChatEvents().Events(nil)
	wantEvent := network.ChatEvent{
		EventID: 1, PlayerID: core.PlayerID{0: 0x23, 6: 0x40, 8: 0x80, 15: 0x11},
		PlayerName: "旅人", CompanionID: wantID, CompanionName: "阿木",
		Kind: network.ChatEventAccepted, Command: "挖石头",
	}
	if len(events) != 1 || events[0] != wantEvent {
		t.Fatalf("chat events=%+v，想要 [%+v]", events, wantEvent)
	}
	chatInput := app.ChatInput()
	if !chatInput.IsOpen() || chatInput.Text() != "@阿木 挖石头" || chatInput.Overflow() {
		t.Fatalf("chat input open=%v text=%q overflow=%v",
			chatInput.IsOpen(), chatInput.Text(), chatInput.Overflow())
	}
}
