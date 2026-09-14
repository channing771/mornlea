package capture

// capture_ranged_mob_test.go 钉住远程敌怪视觉场景的位置契约与夹具确定性：
// 场景必须紧随 `hostile-mob`（两片敌怪夜景相邻）、先于 `passive-herd`；夹具
// 经客户端镜像装入 2 只掷骨者（kind=1，骨白双足）与 1 名目标玩家，骨刺经
// 投射物镜像（spawn 直读初速）固定在飞行中途，弹道相位经 PinVolatile 重钉
// 后与机器速度无关；场景留下的掷骨者、投射物与目标玩家必须在后续场景的
// 公共清理中被一并恢复。

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

// TestRangedMobCaptureScenePosition 锁住 ranged-mob 的表内位置：紧随
// `hostile-mob`（近战与远程两片敌怪夜景相邻，链尾由 passive-herd 的位置
// 测试兜底），同时确认既有尾段不变量未被本场景移动——`far-horizon` 仍为
// 倒数第二、`water-underwater` 仍为唯一末场景。
// 断言写「相邻关系」而不是「在表里」：后者是存在性断言，插到别的位置也
// 照样通过，正是要挡的那种改动。
func TestRangedMobCaptureScenePosition(t *testing.T) {
	indexOf := func(name string) int {
		for index, scene := range captureScenes {
			if scene.Name == name {
				return index
			}
		}
		t.Fatalf("场景 %q 不存在", name)
		return -1
	}
	scene := captureSceneByName(t, "ranged-mob")
	if scene.Prepare == nil || scene.Apply == nil || scene.WarmupFrames != 8 {
		t.Fatalf("ranged-mob 场景不完整: %+v", scene)
	}
	// 弹道相位依赖收敛帧期间推进的插值时钟，必须经 PinVolatile 重钉后才
	// 与机器速度无关。
	if scene.PinVolatile == nil {
		t.Fatal("ranged-mob 缺少 PinVolatile：弹道相位必须钉死")
	}
	if indexOf("ranged-mob") != indexOf("hostile-mob")+1 {
		t.Fatalf("ranged-mob=%d 必须紧随 hostile-mob=%d",
			indexOf("ranged-mob"), indexOf("hostile-mob"))
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

// TestCaptureRangedMobFixtureIsDeterministicAndTagFree 装入 ranged-mob 夹具并
// 断言四件事：固定夜间世界与相机是常量；镜像里恰有 2 只掷骨者（kind=1、
// 面向目标玩家）与 2 枚飞行中的骨刺（位置与初速钉死在夹具值上，镜像快照
// 数不足 3 时呈现恒等于最新快照，因此与机器速度无关）；目标玩家只产生
// 自己的一枚名牌，掷骨者与骨刺经呈现链路只进入实体通道与投射物段、不产
// 生名称标签。最后模拟后续场景的公共清理，确认临时掷骨者、投射物与目标
// 玩家一并恢复，后续场景不继承任何夹具值。
func TestCaptureRangedMobFixtureIsDeterministicAndTagFree(t *testing.T) {
	scene := captureSceneByName(t, "ranged-mob")
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := newCaptureAICompanionState()
	app.SetHostiles(&client.Hostiles{})
	app.SetProjectiles(&client.Projectiles{})
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)

	// 预置上一场景可能留下的旧敌怪与旧投射物：场景的清理必须把它们清掉，
	// 否则前序状态会静默渗入本场景画面。
	if err := app.Hostiles().ApplySpawn(network.HostileSpawn{ServerTick: 9, Spawns: []network.HostileSpawnRecord{
		{ID: 999, Dimension: core.Overworld, Position: mgl32.Vec3{99, 1, 99}, Yaw: 1, Health: 20},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Projectiles().ApplySpawn(network.ProjectileSpawn{ServerTick: 9, Spawns: []network.ProjectileSpawnRecord{
		{ID: 900, Kind: network.ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{99, 2, 99}, Velocity: mgl32.Vec3{0, 0, 1}},
	}}); err != nil {
		t.Fatal(err)
	}

	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 ranged-mob: %v", err)
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
		{name: "近端地面", position: core.BlockPos{X: 10, Y: 0, Z: 10}, want: core.GrassID},
		{name: "火把一", position: core.BlockPos{X: -4, Y: 1, Z: 0}, want: core.TorchStandingID},
		{name: "火把二", position: core.BlockPos{X: 4, Y: 1, Z: -2}, want: core.TorchStandingID},
		{name: "火把三", position: core.BlockPos{X: 1, Y: 1, Z: 6}, want: core.TorchStandingID},
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
		t.Fatal("ranged-mob 夹具没有经 mirror 标记 dirty section")
	}

	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 ranged-mob: %v", err)
	}
	// 固定夜间世界时间与相机：与 hostile-mob 同一夜晚相位（18000 tick，
	// 昼夜亮度取夜间下限），相机钉在目标玩家身后高位平视掷骨者。
	wantCamera := client.Camera{
		Pos: mgl32.Vec3{0.5, 2.6, 8.5}, Yaw: 0, Pitch: -0.1,
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
	// 目标玩家位姿钉死：站在掷骨者与相机之间、面向掷骨者（背对相机），
	// 是骨刺飞行方向的可读落点。
	players := app.RemotePlayers().AppendPresentations(nil)
	if len(players) != 1 || players[0].DisplayName != "Guard" ||
		players[0].Position != (mgl32.Vec3{0.5, 1, 4}) || players[0].Yaw != 0 {
		t.Fatalf("目标玩家呈现=%+v，想要唯一 Guard 于 (0.5,1,4) 朝向掷骨者", players)
	}
	// 2 只掷骨者的呈现逐字段钉死：kind=1（骨白双足分支）、面向目标玩家、
	// 生命满值。镜像快照数不足 3 时呈现恒等于最新快照，与帧间隔无关。
	hostilePresentations := app.Hostiles().AppendPresentations(nil)
	wantHostiles := []client.HostilePresentation{
		{ID: 301, Dimension: core.Overworld, Position: mgl32.Vec3{-2, 1, -3}, Yaw: -2.8, Health: 20, Kind: 1},
		{ID: 302, Dimension: core.Overworld, Position: mgl32.Vec3{3.2, 1, -4.5}, Yaw: 2.83, Health: 20, Kind: 1},
	}
	if !slices.Equal(hostilePresentations, wantHostiles) {
		t.Fatalf("掷骨者呈现=%+v，想要 %+v", hostilePresentations, wantHostiles)
	}
	// 2 枚骨刺钉在飞行中途：位置在掷骨者眼位与目标玩家眼位之间、速度是
	// 朝玩家方向的骨刺初速（22 格/秒量级），spawn 直读初速不经差分。
	projectilePresentations := app.Projectiles().AppendPresentations(nil)
	wantProjectiles := []client.ProjectilePresentation{
		{ID: 501, Kind: network.ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{-0.6, 2.45, 0.9}, Velocity: mgl32.Vec3{7.3, 1.1, 20.7}},
		{ID: 502, Kind: network.ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{1.6, 2.35, 0.6}, Velocity: mgl32.Vec3{-6.6, 1.7, 20.9}},
	}
	if !slices.Equal(projectilePresentations, wantProjectiles) {
		t.Fatalf("骨刺呈现=%+v，想要 %+v", projectilePresentations, wantProjectiles)
	}
	// 呈现链路分派：目标玩家进实体通道并产生唯一一枚名牌；掷骨者只进
	// 实体通道（敌怪身份域键、kind=1 骨白分支）；骨刺既不进 avatar 通道
	// 也不产生名牌，走独立投射物段。
	avatars, tags := application.RemoteRenderPresentationsSortedInto(
		nil, make([]render.NameTag, 0, application.MaxFrameNameTags),
		app.RemotePlayers().AppendPresentations(nil),
	)
	avatars, tags = application.AppendCompanionRenderPresentationsInto(
		avatars, tags, app.Companions().AppendPresentations(nil),
	)
	avatars = application.AppendHostileRenderPresentationsInto(avatars, hostilePresentations)
	if len(tags) != 1 || tags[0].Text != "Guard" {
		t.Fatalf("ranged-mob 场景名牌=%+v，想要唯一目标玩家名牌", tags)
	}
	if len(avatars) != 3 {
		t.Fatalf("实体通道身体=%d，想要 3（1 玩家 + 2 掷骨者）", len(avatars))
	}
	hostileBodies := 0
	for _, avatar := range avatars {
		switch avatar.Key.Kind {
		case render.EntityHostile:
			hostileBodies++
			if avatar.HostileKind != network.HostileKindBoneThrower {
				t.Fatalf("敌怪身体类别=%d，想要掷骨者 1", avatar.HostileKind)
			}
		case render.EntityPlayer:
		default:
			t.Fatalf("实体通道出现非玩家非敌怪身体 %v", avatar.Key)
		}
	}
	if hostileBodies != 2 {
		t.Fatalf("掷骨者身体=%d，想要 2", hostileBodies)
	}
	shardInstances := application.AppendProjectileRenderPresentationsInto(nil, projectilePresentations)
	if len(shardInstances) != 2 {
		t.Fatalf("投射物段实例=%d，想要 2", len(shardInstances))
	}
	for _, shard := range shardInstances {
		if shard.Kind != render.ProjectileKindShard {
			t.Fatalf("投射物段弹种=%d，想要骨刺 0", shard.Kind)
		}
	}

	// 弹道相位重钉：PinVolatile 重放投射物批次（despawn→spawn），把插值
	// 时钟归零、呈现钉回夹具快照；重钉后位置仍是夹具值。
	if err := scene.PinVolatile(app); err != nil {
		t.Fatalf("钉住 ranged-mob: %v", err)
	}
	if got := app.Projectiles().AppendPresentations(nil); !slices.Equal(got, wantProjectiles) {
		t.Fatalf("重钉后骨刺呈现=%+v，想要 %+v", got, wantProjectiles)
	}

	// 场景表没有 teardown 钩子：后续场景（passive-herd）经公共清理恢复全部
	// 共享呈现状态，临时掷骨者、骨刺与目标玩家必须一并清空。
	if err := resetCapturePresentation(app); err != nil {
		t.Fatalf("后续场景公共清理: %v", err)
	}
	if got := app.Hostiles().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("清理后仍有敌怪: %+v", got)
	}
	if got := app.Projectiles().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("清理后仍有投射物: %+v", got)
	}
	if got := app.RemotePlayers().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("清理后仍有远端玩家: %+v", got)
	}
	if len(app.HostilePresentations()) != 0 {
		t.Fatalf("清理后仍有敌怪呈现缓存: %+v", app.HostilePresentations())
	}
}
