//go:build darwin

package app

import (
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

// TestAppendPassiveRenderPresentationsIntoKeysPositions 锁定被动呈现到
// avatar 记录的键与位姿映射：键为被动身份域，位置与朝向直通，俯仰归零。
func TestAppendPassiveRenderPresentationsIntoKeysPositions(t *testing.T) {
	presentations := []client.PassivePresentation{
		{ID: 5, Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3}, Yaw: 0.5, Health: 8},
	}
	avatars := AppendPassiveRenderPresentationsInto(nil, presentations)
	if len(avatars) != 1 {
		t.Fatalf("avatars=%d，想要 1", len(avatars))
	}
	if avatars[0].Key != render.PassiveEntityKey(5) {
		t.Fatalf("avatar 键=%v，想要被动牛 ID 5 的键", avatars[0].Key)
	}
	if avatars[0].Position != presentations[0].Position || avatars[0].Yaw != 0.5 || avatars[0].Pitch != 0 {
		t.Fatalf("avatar 位姿=%+v，想要位置与朝向直通、俯仰归零", avatars[0])
	}
}
