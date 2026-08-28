package capture

// capture_hostile_mob_test.go 钉住夜行者视觉场景的位置契约与夹具确定性：
// 场景必须排在 `ai-companion` 之后、`water-surface-slope` 之前；夹具经客户端
// 镜像装入 8 只夜行者（1 只受击、1 只追逐中）且绝不产生名称标签；场景留下
// 的临时夜行者必须在后续场景的公共清理中被一并恢复。

import (
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
)

// TestHostileMobCaptureScenePosition 锁住 hostile-mob 的表内位置：夹在
// `ai-companion` 与 `water-surface-slope` 之间（spec visual-verification
// delta「场景表顺序与导出」），同时确认既有尾段不变量未被本场景移动——
// `far-horizon` 仍为倒数第二、`water-underwater` 仍为唯一末场景。
// 断言写「相邻关系」而不是「在表里」：后者是存在性断言，插到别的位置也
// 照样通过，正是要挡的那种改动。
func TestHostileMobCaptureScenePosition(t *testing.T) {
	indexOf := func(name string) int {
		for index, scene := range captureScenes {
			if scene.Name == name {
				return index
			}
		}
		t.Fatalf("场景 %q 不存在", name)
		return -1
	}
	scene := captureSceneByName(t, "hostile-mob")
	if scene.Prepare == nil || scene.Apply == nil || scene.WarmupFrames != 8 {
		t.Fatalf("hostile-mob 场景不完整: %+v", scene)
	}
	if indexOf("hostile-mob") != indexOf("ai-companion")+1 {
		t.Fatalf("hostile-mob=%d 必须紧随 ai-companion=%d",
			indexOf("hostile-mob"), indexOf("ai-companion"))
	}
	if indexOf("water-surface-slope") != indexOf("hostile-mob")+1 {
		t.Fatalf("water-surface-slope=%d 必须紧随 hostile-mob=%d",
			indexOf("water-surface-slope"), indexOf("hostile-mob"))
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

// TestCaptureHostileMobFixtureIsDeterministicAndTagFree 装入 hostile-mob 夹具并
// 断言三件事：固定夜间世界与相机是常量；镜像里恰有 8 只夜行者、其中受击个体的
// 生命与追逐个体的推进位置/朝向都钉死在夹具值上（镜像在快照数不足 3 时把呈现
// 钉在最新快照，因此这些值与机器速度无关）；夜行者经呈现链路只进入实体通道，
// 不产生任何名称标签。最后模拟后续场景的公共清理，确认临时夜行者与受击/追逐
// 状态一并恢复，后续场景不继承任何夹具值。
func TestCaptureHostileMobFixtureIsDeterministicAndTagFree(t *testing.T) {
	scene := captureSceneByName(t, "hostile-mob")
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := newCaptureAICompanionState()
	app.SetHostiles(&client.Hostiles{})
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)

	// 预置上一场景可能留下的旧实体与旧夜行者：场景的清理必须把它们清掉，
	// 否则前序状态会静默渗入本场景画面。
	if err := app.Hostiles().ApplySpawn(network.HostileSpawn{ServerTick: 9, Spawns: []network.HostileSpawnRecord{
		{ID: 999, Dimension: core.Overworld, Position: mgl32.Vec3{99, 1, 99}, Yaw: 1, Health: 20},
	}}); err != nil {
		t.Fatal(err)
	}

	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 hostile-mob: %v", err)
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
		{name: "草地地面", position: core.BlockPos{X: 0, Y: 0, Z: 0}, want: core.GrassID},
		{name: "地面远角", position: core.BlockPos{X: -10, Y: 0, Z: -16}, want: core.GrassID},
		{name: "近端地面", position: core.BlockPos{X: 10, Y: 0, Z: 8}, want: core.GrassID},
		{name: "火把一", position: core.BlockPos{X: -3, Y: 1, Z: -1}, want: core.TorchStandingID},
		{name: "火把二", position: core.BlockPos{X: 5, Y: 1, Z: -11}, want: core.TorchStandingID},
		{name: "火把三", position: core.BlockPos{X: -4, Y: 1, Z: -14}, want: core.TorchStandingID},
		{name: "地面上方空气", position: core.BlockPos{X: 0, Y: 2, Z: 0}, want: core.AirID},
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
		t.Fatal("hostile-mob 夹具没有经 mirror 标记 dirty section")
	}

	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 hostile-mob: %v", err)
	}
	// 固定夜间世界时间与相机：与 torch-night 同一夜晚相位（18000 tick，
	// 昼夜亮度取夜间下限），相机钉在高位平视火把边缘的夜行者群。
	wantCamera := client.Camera{
		Pos: mgl32.Vec3{0.5, 2.8, 6.5}, Yaw: 0, Pitch: -0.12,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	if app.WorldTimeTicks() != 18000 || *app.Camera() != wantCamera {
		t.Fatalf("固定环境 time=%d camera=%+v，想要 18000/%+v",
			app.WorldTimeTicks(), *app.Camera(), wantCamera)
	}
	if app.Center() != application.CameraChunk(app.Camera().Pos) {
		t.Fatalf("center=%+v 与相机区块不同步", app.Center())
	}
	// 8 只夜行者的呈现逐字段钉死：受击个体（ID 103）生命 13、位置后撤；
	// 追逐个体（ID 107）从出生点 (2.8,1,-1.5) 推进到 (1.2,1,1.8)、朝向相机。
	// 镜像快照数不足 3 时呈现恒等于最新快照，与帧间隔无关，断言因此确定。
	presentations := app.Hostiles().AppendPresentations(nil)
	wantPresentations := []client.HostilePresentation{
		{ID: 101, Dimension: core.Overworld, Position: mgl32.Vec3{-6, 1, -2}, Yaw: 2.2, Health: 20},
		{ID: 102, Dimension: core.Overworld, Position: mgl32.Vec3{-3.5, 1, -6}, Yaw: 0.6, Health: 20},
		{ID: 103, Dimension: core.Overworld, Position: mgl32.Vec3{5.8, 1, -7}, Yaw: -0.7, Health: 13},
		{ID: 104, Dimension: core.Overworld, Position: mgl32.Vec3{8.5, 1, -4}, Yaw: 2.8, Health: 20},
		{ID: 105, Dimension: core.Overworld, Position: mgl32.Vec3{-8, 1, -9}, Yaw: 1.2, Health: 20},
		{ID: 106, Dimension: core.Overworld, Position: mgl32.Vec3{-1, 1, -13}, Yaw: 0.3, Health: 20},
		{ID: 107, Dimension: core.Overworld, Position: mgl32.Vec3{1.2, 1, 1.8}, Yaw: 3.1415927, Health: 20},
		{ID: 108, Dimension: core.Overworld, Position: mgl32.Vec3{10, 1, -10}, Yaw: 3.0, Health: 20},
	}
	if !slices.Equal(presentations, wantPresentations) {
		t.Fatalf("夜行者呈现=%+v，想要 %+v", presentations, wantPresentations)
	}
	// 无名标断言：走与 `RenderFrame` 相同的呈现装配函数——夜行者只进实体通道
	//（敌怪身份域键），玩家/伙伴/目标名牌均不得出现。呈现缓存由 renderFrame
	// 每帧重建，这里直接喂镜像导出的同一份呈现值。
	avatars, tags := application.RemoteRenderPresentationsSortedInto(
		nil, make([]render.NameTag, 0, application.MaxFrameNameTags),
		app.RemotePlayers().AppendPresentations(nil),
	)
	avatars, tags = application.AppendCompanionRenderPresentationsInto(
		avatars, tags, app.Companions().AppendPresentations(nil),
	)
	avatars = application.AppendHostileRenderPresentationsInto(avatars, presentations)
	if len(tags) != 0 {
		t.Fatalf("hostile-mob 场景产生了名称标签: %+v", tags)
	}
	if len(avatars) != len(wantPresentations) {
		t.Fatalf("实体通道身体=%d，想要 %d", len(avatars), len(wantPresentations))
	}
	for _, avatar := range avatars {
		if avatar.Key.Kind != render.EntityHostile {
			t.Fatalf("实体通道出现非敌怪身体 %v", avatar.Key)
		}
	}

	// 场景表没有 teardown 钩子：后续场景（water-surface-slope）经公共清理
	// 恢复全部共享呈现状态，临时夜行者、受击与追逐值必须一并清空。
	if err := resetCapturePresentation(app); err != nil {
		t.Fatalf("后续场景公共清理: %v", err)
	}
	if got := app.Hostiles().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("清理后仍有夜行者: %+v", got)
	}
	if len(app.HostilePresentations()) != 0 {
		t.Fatalf("清理后仍有夜行者呈现缓存: %+v", app.HostilePresentations())
	}
}
