package capture

// capture_passive_graze_test.go 钉住吃草视觉场景的位置契约与夹具确定性：
// 场景必须排在 `passive-herd` 之后、`water-surface-slope` 之前；夹具经客户端
// 镜像装入 2 头牛（ID 升序：1 头放牧置位低头 + 1 头常态对照，放牧位经与权威
// 消息相同的 spawn→state 两批入口注入），世界夹具是整片草地里的一格泥土
// （吃草结算的前后对照）；被动牛只进实体通道、不产生名称标签；低头牛的
// avatar 俯仰恰为呈现侧下压角，常态牛归零；场景留下的牛必须在后续场景的公共
// 清理中被一并恢复。本场景不含掉落物与任何随机器速度变化的读数，因此无需
// PinVolatile。

import (
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestPassiveGrazeCaptureScenePosition 锁住 passive-graze 的表内位置：夹在
// `passive-herd` 与 `water-surface-slope` 之间（昼夜两片牧场之后紧跟吃草结算
// 第二、`water-underwater` 仍为唯一末场景。
func TestPassiveGrazeCaptureScenePosition(t *testing.T) {
	indexOf := func(name string) int {
		for index, scene := range captureScenes {
			if scene.Name == name {
				return index
			}
		}
		t.Fatalf("场景 %q 不存在", name)
		return -1
	}
	scene := captureSceneByName(t, "passive-graze")
	if scene.Prepare == nil || scene.Apply == nil || scene.WarmupFrames != 8 {
		t.Fatalf("passive-graze 场景不完整: %+v", scene)
	}
	// 本场景不含掉落物与任何随机器速度变化的读数：位姿在 Apply 里经镜像钉死，
	// 收敛帧内不再 drain，无需 PinVolatile 即可确定。
	if scene.PinVolatile != nil {
		t.Fatalf("passive-graze 不该有 PinVolatile：场景无易变读数")
	}
	if indexOf("passive-graze") != indexOf("passive-herd")+1 {
		t.Fatalf("passive-graze=%d 必须紧随 passive-herd=%d",
			indexOf("passive-graze"), indexOf("passive-herd"))
	}
	if indexOf("water-surface-slope") != indexOf("passive-graze")+1 {
		t.Fatalf("water-surface-slope=%d 必须紧随 passive-graze=%d",
			indexOf("water-surface-slope"), indexOf("passive-graze"))
	}
	if captureScenes[len(captureScenes)-2].Name != "far-horizon" {
		t.Fatalf("倒数第二场景=%q，想要 far-horizon",
			captureScenes[len(captureScenes)-2].Name)
	}
	if captureScenes[len(captureScenes)-1].Name != "water-underwater" {
		t.Fatalf("末场景=%q，想要 water-underwater",
			captureScenes[len(captureScenes)-1].Name)
	}
}

// TestCapturePassiveGrazeFixtureIsDeterministicAndTagFree 装入 passive-graze
// 夹具并断言五件事：固定昼间世界与相机是常量；地面是整片草地里的一格泥土
// （吃草结算对照）；镜像里恰有 2 头牛（ID 升序、1 头放牧置位 + 1 头常态，位
// 置朝向与出生一致故与插值快照数无关）；牛经呈现链路只进入实体通道，不产生
// 任何名称标签，且低头牛的 avatar 俯仰恰为呈现侧下压角、常态牛归零。最后模拟
// 后续场景的公共清理，确认临时牛群一并恢复，后续场景不继承任何夹具值。
func TestCapturePassiveGrazeFixtureIsDeterministicAndTagFree(t *testing.T) {
	scene := captureSceneByName(t, "passive-graze")
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := newCaptureAICompanionState()
	app.SetPassives(&client.Passives{})
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)

	// 预置上一场景可能留下的旧牛：场景的清理必须把它们清掉，否则前序状态会
	// 静默渗入本场景画面。
	if err := app.Passives().ApplySpawn(network.PassiveSpawn{ServerTick: 9, Spawns: []network.PassiveSpawnRecord{
		{ID: 999, Dimension: core.Overworld, Position: mgl32.Vec3{99, 1, 99}, Yaw: 1, Health: 20},
	}}); err != nil {
		t.Fatal(err)
	}
	// 夜行者镜像可能为 nil（最小测试装配）：呈现装配需要它非空，无夹具残留
	// 时跳过效果等同于空镜像。
	if app.Hostiles() == nil {
		app.SetHostiles(&client.Hostiles{})
	}

	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 passive-graze: %v", err)
	}
	// 夹具地面覆盖 3×3 区块窗口；applyCaptureBlocks 对每个窗口区块都发一次
	// BlockChanges（含空批次），因此 9 个区块 revision 全部推进到 2。
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			chunk, ok := app.Mirror().Chunk(core.Overworld, core.ChunkPos{X: x, Z: z})
			if !ok || chunk.Revision != 2 {
				t.Fatalf("chunk (%d,%d) revision/loaded=%d/%v，想要 2/true",
					x, z, chunk.Revision, ok)
			}
		}
	}
	for _, probe := range []struct {
		name     string
		position core.BlockPos
		want     core.BlockID
	}{
		{name: "吃草结算泥土", position: core.BlockPos{X: 1, Y: 0, Z: 0}, want: core.DirtID},
		{name: "泥土旁草地对照", position: core.BlockPos{X: 0, Y: 0, Z: 0}, want: core.GrassID},
		{name: "地面远角", position: core.BlockPos{X: -10, Y: 0, Z: -16}, want: core.GrassID},
		{name: "近端地面", position: core.BlockPos{X: 10, Y: 0, Z: 8}, want: core.GrassID},
		{name: "泥土上方空气", position: core.BlockPos{X: 1, Y: 1, Z: 0}, want: core.AirID},
	} {
		t.Run(probe.name, func(t *testing.T) {
			got, loaded := app.Mirror().BlockAt(core.Overworld, probe.position)
			if !loaded || got != probe.want {
				t.Fatalf("BlockAt(%+v)=%d/%v，想要 %d/true",
					probe.position, got, loaded, probe.want)
			}
		})
	}
	if mesher.Stats().DirtySections == 0 {
		t.Fatal("passive-graze 夹具没有经 mirror 标记 dirty section")
	}

	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 passive-graze: %v", err)
	}
	// 固定昼间世界时间与相机：正午相位（6000 tick）与牛群场景同机位，保证
	// 低头牛居中呈侧面、常态牛在左侧同框对照。
	wantCamera := client.Camera{
		Pos: mgl32.Vec3{0.5, 2.8, 6.5}, Yaw: 0, Pitch: -0.12,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	if app.WorldTimeTicks() != 6000 || *app.Camera() != wantCamera {
		t.Fatalf("固定环境 time=%d camera=%+v，想要 6000/%+v",
			app.WorldTimeTicks(), *app.Camera(), wantCamera)
	}
	if app.Center() != application.CameraChunk(app.Camera().Pos) {
		t.Fatalf("center=%+v 与相机区块不同步", app.Center())
	}
	// 2 头牛的呈现逐字段钉死：ID 升序，211 放牧置位（低头）、212 常态对照；
	// state 批次的位置朝向与出生一致，呈现因此与插值快照数无关，断言确定。
	presentations := app.Passives().AppendPresentations(nil)
	wantPresentations := []client.PassivePresentation{
		{ID: 211, Dimension: core.Overworld, Position: mgl32.Vec3{0.3, 1, 0.5}, Yaw: 0, Health: core.MaxHealth, Grazing: true},
		{ID: 212, Dimension: core.Overworld, Position: mgl32.Vec3{-3.4, 1, 1.2}, Yaw: 0.7, Health: core.MaxHealth, Grazing: false},
	}
	if !slices.Equal(presentations, wantPresentations) {
		t.Fatalf("被动牛呈现=%+v，想要 %+v", presentations, wantPresentations)
	}
	// 无名标断言：走与 `RenderFrame` 相同的呈现装配函数——牛只进实体通道
	//（被动身份域键），玩家/伙伴/目标名牌均不得出现；低头牛的 avatar 俯仰恰
	// 为呈现侧下压角，常态牛归零——位姿完全由镜像驱动。
	avatars, tags := application.RemoteRenderPresentationsSortedInto(
		nil, make([]render.NameTag, 0, application.MaxFrameNameTags),
		app.RemotePlayers().AppendPresentations(nil),
	)
	avatars, tags = application.AppendCompanionRenderPresentationsInto(
		avatars, tags, app.Companions().AppendPresentations(nil),
	)
	avatars = application.AppendHostileRenderPresentationsInto(
		avatars, app.Hostiles().AppendPresentations(nil),
	)
	avatars = application.AppendPassiveRenderPresentationsInto(avatars, presentations)
	if len(tags) != 0 {
		t.Fatalf("passive-graze 场景产生了名称标签: %+v", tags)
	}
	if len(avatars) != len(wantPresentations) {
		t.Fatalf("实体通道身体=%d，想要 %d", len(avatars), len(wantPresentations))
	}
	for index, avatar := range avatars {
		if avatar.Key.Kind != render.EntityPassive {
			t.Fatalf("实体通道出现非被动牛身体 %v", avatar.Key)
		}
		wantPitch := float32(0)
		if wantPresentations[index].Grazing {
			wantPitch = render.PassiveGrazeHeadPitch(true)
		}
		if avatar.Pitch != wantPitch {
			t.Fatalf("身体 %d 俯仰=%v，想要 %v（放牧位驱动）", index, avatar.Pitch, wantPitch)
		}
	}

	// 场景表没有 teardown 钩子：后续场景经公共清理恢复全部共享呈现状态，
	// 临时牛群必须一并清空。
	if err := resetCapturePresentation(app); err != nil {
		t.Fatalf("后续场景公共清理: %v", err)
	}
	if got := app.Passives().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("清理后仍有被动牛: %+v", got)
	}
}
