//go:build darwin

package app

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestApplicationRendersPassivesWithoutNameTags 锁定被动牛的呈现路径：镜像
// 记录进入 avatar 通道（被动身份域键），但绝不产生任何名称标签，玩家/伙伴
// 的名标不受影响。
func TestApplicationRendersPassivesWithoutNameTags(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	configureTargetFeedback(t, app)
	app.passives = &client.Passives{}
	if err := app.remotePlayers.Apply(RemoteSpawn(
		1, "Remote", 1, mgl32.Vec3{0, 2, -4},
	)); err != nil {
		t.Fatal(err)
	}
	if err := app.passives.ApplySpawn(network.PassiveSpawn{ServerTick: 1, Spawns: []network.PassiveSpawnRecord{
		{ID: 7, Dimension: core.Overworld, Position: mgl32.Vec3{-2, 1, -6}, Yaw: 0.25, Health: 10},
		{ID: 9, Dimension: core.Overworld, Position: mgl32.Vec3{4, 1, -6}, Yaw: -1.5, Health: 10},
	}}); err != nil {
		t.Fatal(err)
	}

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("renderFrame=(%v,%v)", rendered, err)
	}
	passiveKeys := map[render.EntityKey]struct{}{
		render.PassiveEntityKey(7): {}, render.PassiveEntityKey(9): {},
	}
	passiveAvatars := 0
	for _, avatar := range app.remoteAvatars {
		if _, passive := passiveKeys[avatar.Key]; passive {
			passiveAvatars++
		}
	}
	if passiveAvatars != 2 {
		t.Fatalf("avatar 通道中的被动牛=%d，想要 2", passiveAvatars)
	}
	for _, tag := range app.remoteNameTags {
		if _, passive := passiveKeys[tag.Key]; passive {
			t.Fatalf("被动牛 %v 产生了名称标签", tag.Key)
		}
	}
	// 玩家 + 目标方块的名标数量不受被动牛影响（1 具名标身体 + 1 目标）。
	if got, want := len(app.remoteNameTags), 2; got != want {
		t.Fatalf("name tags=%d，想要 %d", got, want)
	}
	// 被动牛身体进入同一实例流：3 具身体 × 6 部件。
	if got, want := len(app.avatarStream), 3*6*96; got != want {
		t.Fatalf("avatar 实例流=%d 字节，想要 %d", got, want)
	}
}

// TestAppendPassiveRenderPresentationsDyingMapsDeathPhase 锁定死亡呈现装配：
// 保留体的滚转与红闪由死亡相位函数赋值（权威 tick 派生），活体保持零值。
func TestAppendPassiveRenderPresentationsDyingMapsDeathPhase(t *testing.T) {
	presentations := []client.PassivePresentation{
		{ID: 5, Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3}, Yaw: 0.5, Health: 8},
		{ID: 6, Dimension: core.Overworld, Position: mgl32.Vec3{4, 5, 6}, Yaw: -0.5, Health: 8, Dying: true, DeathTick: 100},
	}
	avatars := AppendPassiveRenderPresentationsInto(nil, presentations, 110)
	if avatars[0].Roll != 0 || avatars[0].Flash != 0 {
		t.Fatalf("活体死亡通道=%+v，想要零值", avatars[0])
	}
	wantRoll, wantFlash := render.PassiveDeathPhase(100, 6, 110)
	if avatars[1].Roll != wantRoll || avatars[1].Flash != wantFlash {
		t.Fatalf("保留体死亡通道=(%v,%v)，想要相位 (%v,%v)", avatars[1].Roll, avatars[1].Flash, wantRoll, wantFlash)
	}
}

// TestPassiveDeathRetentionMatchesRenderPhase 钉住两处“20”的行为一致：客户
// 端镜像的死亡保留时长与渲染侧相位函数的保留时长必须同为 20 tick——一侧改
// 数值不同步另一侧，本测试即红。
func TestPassiveDeathRetentionMatchesRenderPhase(t *testing.T) {
	passives := &client.Passives{}
	position := mgl32.Vec3{1, 2, 3}
	if err := passives.ApplySpawn(network.PassiveSpawn{ServerTick: 100, Spawns: []network.PassiveSpawnRecord{
		{ID: 7, Dimension: core.Overworld, Position: position, Yaw: 0.5, Health: 9},
	}}); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	if err := passives.ApplySpawn(network.PassiveSpawn{ServerTick: 100, Spawns: []network.PassiveSpawnRecord{
		{ID: 8, Dimension: core.Overworld, Position: mgl32.Vec3{9, 1, 9}, Yaw: 0, Health: 20},
	}}); err != nil {
		t.Fatalf("ApplySpawn 活牛: %v", err)
	}
	if err := passives.ApplyDespawn(network.PassiveDespawn{ServerTick: 100, Despawns: []network.PassiveDespawnRecord{
		{ID: 7, Reason: network.PassiveDespawnDied},
	}}); err != nil {
		t.Fatalf("死亡 ApplyDespawn: %v", err)
	}
	advanceTo := func(tick uint64) {
		t.Helper()
		if err := passives.ApplyStates(network.PassiveState{ServerTick: tick, States: []network.PassiveStateRecord{
			{ID: 8, Position: mgl32.Vec3{9, 1, 9}, Yaw: 0, Health: 20},
		}}); err != nil {
			t.Fatalf("推进 tick=%d: %v", tick, err)
		}
	}
	advanceTo(119)
	presentations := passives.AppendPresentations(nil)
	dying := false
	for _, presentation := range presentations {
		if presentation.ID == 7 {
			dying = true
		}
	}
	if !dying {
		t.Fatal("T+19 保留体已消失，想要仍在")
	}
	if roll, _ := render.PassiveDeathPhase(100, 7, 119); roll >= float32(1.5707964) {
		t.Fatalf("T+19 侧倒=%v，想要未满 90°", roll)
	}
	advanceTo(120)
	for _, presentation := range passives.AppendPresentations(nil) {
		if presentation.ID == 7 {
			t.Fatal("T+20 保留体仍在，想要已移除")
		}
	}
}

// TestAppendPassiveRenderPresentationsIntoKeysPositions 锁定被动呈现到
// avatar 记录的键与位姿映射：键为被动身份域，位置直通，朝向经装配层 +π/2
// 对齐映射（牛模型面朝 +X、物理 yaw=0 面向 -Z，详见
// `TestAppendPassiveRenderPresentationsAlignCowHeadingWithPhysicsYaw`），
// 俯仰由放牧位经呈现侧映射直通（放牧下压、常态归零），位姿完全由权威镜像
// 驱动。
func TestAppendPassiveRenderPresentationsIntoKeysPositions(t *testing.T) {
	presentations := []client.PassivePresentation{
		{ID: 5, Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3}, Yaw: 0.5, Health: 8},
		{ID: 6, Dimension: core.Overworld, Position: mgl32.Vec3{4, 5, 6}, Yaw: -0.5, Health: 8, Grazing: true},
	}
	avatars := AppendPassiveRenderPresentationsInto(nil, presentations, 2)
	if len(avatars) != 2 {
		t.Fatalf("avatars=%d，想要 2", len(avatars))
	}
	if avatars[0].Key != render.PassiveEntityKey(5) {
		t.Fatalf("avatar 键=%v，想要被动牛 ID 5 的键", avatars[0].Key)
	}
	mappedYaw := avatars[0].Yaw - 0.5
	if avatars[0].Position != presentations[0].Position || mappedYaw < math.Pi/2-1e-5 || mappedYaw > math.Pi/2+1e-5 ||
		avatars[0].Pitch != render.PassiveIdleNodPitch(2, 5) {
		t.Fatalf("avatar 位姿=%+v，想要位置直通、朝向为权威 yaw+π/2、俯仰为闲时点头", avatars[0])
	}
	if avatars[1].Key != render.PassiveEntityKey(6) {
		t.Fatalf("avatar 键=%v，想要被动牛 ID 6 的键", avatars[1].Key)
	}
	if avatars[1].Pitch != render.PassiveGrazeHeadPitch(true) {
		t.Fatalf("放牧 avatar 俯仰=%v，想要呈现侧下压角 %v", avatars[1].Pitch, render.PassiveGrazeHeadPitch(true))
	}
}

// TestAppendPassiveRenderPresentationsIdleNodGating 锁定点头门控：闲时点头只
// 叠非常态非死亡体，放牧与死亡体的俯仰不受点头污染。
func TestAppendPassiveRenderPresentationsIdleNodGating(t *testing.T) {
	presentations := []client.PassivePresentation{
		{ID: 5, Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3}, Yaw: 0.5, Health: 8},
		{ID: 6, Dimension: core.Overworld, Position: mgl32.Vec3{4, 5, 6}, Yaw: -0.5, Health: 8, Grazing: true},
		{ID: 7, Dimension: core.Overworld, Position: mgl32.Vec3{7, 8, 9}, Yaw: 0, Health: 8, Dying: true, DeathTick: 100},
	}
	avatars := AppendPassiveRenderPresentationsInto(nil, presentations, 110)
	if avatars[0].Pitch != render.PassiveIdleNodPitch(110, 5) {
		t.Fatalf("闲时 avatar 俯仰=%v，想要点头相位 %v", avatars[0].Pitch, render.PassiveIdleNodPitch(110, 5))
	}
	if avatars[1].Pitch != render.PassiveGrazeHeadPitch(true) {
		t.Fatalf("放牧 avatar 俯仰=%v，想要下压角", avatars[1].Pitch)
	}
	if want, _ := render.PassiveDeathPhase(100, 7, 110); avatars[2].Roll != want {
		t.Fatalf("死亡 avatar 滚转=%v，想要相位 %v", avatars[2].Roll, want)
	}
}

// TestAppendPassiveRenderPresentationsAlignCowHeadingWithPhysicsYaw 锁定装配层
// 朝向对齐：牛模型静止姿态面朝局部 +X（躯干长轴 X、头在 +X），而物理约定
// yaw=0 面向 -Z、前进方向为 (-sin yaw, 0, -cos yaw)（见 shared/physics 的
// movementTargetFromYaw）；装配把权威 yaw 加 π/2 后归一化到 [-π, π) 作为渲染
// yaw——渲染根矩阵为 Translate·HomogRotate3DY(θ)（mathgl 列主序），θ 把模型
// +X 头部基映到 (cos θ, 0, -sin θ)，θ = yaw + π/2 时恰为物理前进方向。牛头
// 因此指向行进方向而非身体侧对的横行姿态。该映射只作用于被动牛装配路径，
// 玩家/伙伴/夜行者的 yaw 仍直传。
func TestAppendPassiveRenderPresentationsAlignCowHeadingWithPhysicsYaw(t *testing.T) {
	for _, yaw := range []float32{0, math.Pi / 2, math.Pi, -math.Pi / 4, 3 * math.Pi / 4, -math.Pi, 5.5} {
		presentations := []client.PassivePresentation{
			{ID: 11, Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3}, Yaw: yaw, Health: 8},
		}
		avatars := AppendPassiveRenderPresentationsInto(nil, presentations, 2)
		if len(avatars) != 1 {
			t.Fatalf("avatars=%d，想要 1", len(avatars))
		}
		renderYaw := avatars[0].Yaw
		// 归一化边界：装配输出必须落在 [-π, π)，含 +π/2 偏移后的回卷值。
		if renderYaw < -math.Pi || renderYaw >= math.Pi {
			t.Fatalf("权威 yaw=%v 的渲染 yaw=%v，未归一化到 [-π, π)", yaw, renderYaw)
		}
		// 渲染 yaw θ 下模型头部基向量：+X 基经 HomogRotate3DY(θ) 的像。
		head := mgl32.Vec3{
			float32(math.Cos(float64(renderYaw))),
			0,
			float32(-math.Sin(float64(renderYaw))),
		}
		forward := mgl32.Vec3{
			float32(-math.Sin(float64(yaw))),
			0,
			float32(-math.Cos(float64(yaw))),
		}
		// 用绝对距离比较：`ApproxEqualThreshold` 对近零分量走相对语义
		// （ε² 与相对除法），正交轴残差 ~1e-8 会被误判为不等。
		if delta := head.Sub(forward).Len(); delta > 1e-5 {
			t.Fatalf("权威 yaw=%v：渲染 yaw=%v 的头部基=%v，想要物理前进方向 %v（偏差 %v）",
				yaw, renderYaw, head, forward, delta)
		}
	}
}
