package capture

// capture_passive_herd_test.go 钉住被动牛群视觉场景的位置契约与夹具确定性：
// 场景必须排在 `hostile-mob` 之后、`water-surface-slope` 之前；夹具经客户端
// 镜像装入 3 头牛（ID 升序、不同朝向/位置）与 1 个生牛肉掉落，被动牛只进实体
// 通道、不产生名称标签；掉落动画相位经固定权威 tick 钉死，与机器速度无关；
// 场景留下的牛与掉落必须在后续场景的公共清理中被一并恢复。

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

// TestPassiveHerdCaptureScenePosition 锁住 passive-herd 的表内位置：夹在
// `hostile-mob` 与 `water-surface-slope` 之间，同时确认既有尾段不变量未被
// 本场景移动——`far-horizon` 仍为倒数第二、`water-underwater` 仍为唯一末场景。
func TestPassiveHerdCaptureScenePosition(t *testing.T) {
	indexOf := func(name string) int {
		for index, scene := range captureScenes {
			if scene.Name == name {
				return index
			}
		}
		t.Fatalf("场景 %q 不存在", name)
		return -1
	}
	scene := captureSceneByName(t, "passive-herd")
	if scene.Prepare == nil || scene.Apply == nil || scene.PinVolatile == nil || scene.WarmupFrames != 8 {
		t.Fatalf("passive-herd 场景不完整: %+v", scene)
	}
	if indexOf("passive-herd") != indexOf("hostile-mob")+1 {
		t.Fatalf("passive-herd=%d 必须紧随 hostile-mob=%d",
			indexOf("passive-herd"), indexOf("hostile-mob"))
	}
	if indexOf("water-surface-slope") != indexOf("passive-herd")+1 {
		t.Fatalf("water-surface-slope=%d 必须紧随 passive-herd=%d",
			indexOf("water-surface-slope"), indexOf("passive-herd"))
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

// TestCapturePassiveHerdFixtureIsDeterministicAndTagFree 装入 passive-herd
// 夹具并断言四件事：固定昼间世界与相机是常量；镜像里恰有 3 头牛（ID 升序、
// 朝向各异）与 1 个生牛肉掉落（镜像快照数不足 3 时呈现恒等于最新快照，与帧
// 间隔无关）；牛与掉落经呈现链路只进入实体通道，不产生任何名称标签；掉落
// 动画相位钉死在固定权威 tick，与机器速度无关。最后模拟后续场景的公共清理，
// 确认临时牛群与掉落一并恢复，后续场景不继承任何夹具值。
func TestCapturePassiveHerdFixtureIsDeterministicAndTagFree(t *testing.T) {
	scene := captureSceneByName(t, "passive-herd")
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := newCaptureAICompanionState()
	app.SetPassives(&client.Passives{})
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)

	// 预置上一场景可能留下的旧夜行者与旧牛：场景的清理必须把它们清掉，
	// 否则前序状态会静默渗入本场景画面。
	if app.Hostiles() == nil {
		app.SetHostiles(&client.Hostiles{})
	}
	if err := app.Hostiles().ApplySpawn(network.HostileSpawn{ServerTick: 9, Spawns: []network.HostileSpawnRecord{
		{ID: 999, Dimension: core.Overworld, Position: mgl32.Vec3{99, 1, 99}, Yaw: 1, Health: 20},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Passives().ApplySpawn(network.PassiveSpawn{ServerTick: 9, Spawns: []network.PassiveSpawnRecord{
		{ID: 999, Dimension: core.Overworld, Position: mgl32.Vec3{99, 1, 99}, Yaw: 1, Health: 20},
	}}); err != nil {
		t.Fatal(err)
	}

	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 passive-herd: %v", err)
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
		t.Fatal("passive-herd 夹具没有经 mirror 标记 dirty section")
	}

	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 passive-herd: %v", err)
	}
	// 固定昼间世界时间与相机：正午相位（6000 tick）与夜行者夜景相对，
	// 相机沿用夜行者群的已验证机位，保证 3 头牛同框。
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
	// 3 头牛的呈现逐字段钉死：ID 升序、朝向各异（左牛侧对、右牛近正对，
	// 中牛侧对远方）；近处两头约占画面高度两成，远处一头退后拉开深度。
	// 镜像快照数不足 3 时呈现恒等于最新快照，与帧间隔无关，断言因此确定。
	presentations := app.Passives().AppendPresentations(nil)
	wantPresentations := []client.PassivePresentation{
		{ID: 201, Dimension: core.Overworld, Position: mgl32.Vec3{-3.4, 1, 1.2}, Yaw: 0.7, Health: core.MaxHealth},
		{ID: 202, Dimension: core.Overworld, Position: mgl32.Vec3{0.8, 1, -0.8}, Yaw: -0.5, Health: core.MaxHealth},
		{ID: 203, Dimension: core.Overworld, Position: mgl32.Vec3{4.6, 1, 1.5}, Yaw: 2.6, Health: core.MaxHealth},
	}
	if !slices.Equal(presentations, wantPresentations) {
		t.Fatalf("被动牛呈现=%+v，想要 %+v", presentations, wantPresentations)
	}
	// 1 个生牛肉掉落钉死在相机正前方的近景空位：物品与数量是夹具值，方块
	// 位置经区块索引往返还原；投影落在中牛脚下前方，纵向错开不遮挡任何牛身。
	drops := app.ItemDrops().Presentations()
	if len(drops) != 1 {
		t.Fatalf("掉落物数量=%d，想要 1", len(drops))
	}
	if drops[0].Item != core.ItemRawBeef || drops[0].Count != 1 {
		t.Fatalf("掉落物=%+v，想要 1 个生牛肉", drops[0])
	}
	wantDropBlock := core.BlockPos{X: 0, Y: 1, Z: 3}
	if block, ok := render.ItemDropBlock(drops[0].ID.Chunk, drops[0].BlockIndex); !ok || block != wantDropBlock {
		t.Fatalf("掉落物方块=%+v/%v，想要 %+v/true", block, ok, wantDropBlock)
	}
	// 无名标断言：走与 `RenderFrame` 相同的呈现装配函数——牛只进实体通道
	//（被动身份域键），玩家/伙伴/目标名牌均不得出现。
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
		t.Fatalf("passive-herd 场景产生了名称标签: %+v", tags)
	}
	if len(avatars) != len(wantPresentations) {
		t.Fatalf("实体通道身体=%d，想要 %d", len(avatars), len(wantPresentations))
	}
	for _, avatar := range avatars {
		if avatar.Key.Kind != render.EntityPassive {
			t.Fatalf("实体通道出现非被动牛身体 %v", avatar.Key)
		}
	}
	// 掉落动画相位钉死：PinVolatile 把权威 tick 固定为常量，掉落旋转/浮动
	// 相位因此是 tick 的纯函数，与加载耗时等机器速度因素无关。
	if err := scene.PinVolatile(app); err != nil {
		t.Fatalf("钉住 passive-herd 易变读数: %v", err)
	}
	if app.ServerTick() != capturePassiveHerdServerTick {
		t.Fatalf("权威 tick=%d，想要 %d", app.ServerTick(), capturePassiveHerdServerTick)
	}

	// 场景表没有 teardown 钩子：后续场景经公共清理恢复全部共享呈现状态，
	// 临时牛群与掉落必须一并清空。
	if err := resetCapturePresentation(app); err != nil {
		t.Fatalf("后续场景公共清理: %v", err)
	}
	if got := app.Passives().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("清理后仍有被动牛: %+v", got)
	}
	if got := app.ItemDrops().Presentations(); len(got) != 0 {
		t.Fatalf("清理后仍有掉落物: %+v", got)
	}
	if got := app.Hostiles().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("清理后仍有夜行者: %+v", got)
	}
}
