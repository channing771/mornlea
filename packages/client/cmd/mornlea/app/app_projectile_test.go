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

// hostileIDOfKey 从敌怪实体键解码 little-endian u64 ID（键槽低 8 字节）。
func hostileIDOfKey(key render.EntityKey) uint64 {
	var id uint64
	for index := 0; index < 8; index++ {
		id |= uint64(key.ID[index]) << (8 * index)
	}
	return id
}

// TestApplicationRendersProjectilesInDedicatedStream 锁定投射物的帧装配：
// 镜像 spawn 后投射物进入独立的 tag 14 实例流（avatar 通道与名标集合不
// 变），despawn 后该流回到空，帧边界无残留。
func TestApplicationRendersProjectilesInDedicatedStream(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	app.companions = &client.Companions{}
	configureTargetFeedback(t, app)

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("初始 renderFrame=(%v,%v)", rendered, err)
	}
	if got := len(app.projectileStream); got != 0 {
		t.Fatalf("无投射物时实例流=%d 字节，想要空", got)
	}

	if err := app.projectiles.ApplySpawn(network.ProjectileSpawn{ServerTick: 1, Spawns: []network.ProjectileSpawnRecord{
		{ID: 5, Kind: network.ProjectileKindShard, Dimension: core.Overworld,
			Position: mgl32.Vec3{0, 2, -4}, Velocity: mgl32.Vec3{0, 0, -22}},
		{ID: 6, Kind: network.ProjectileKindArrow, Dimension: core.Overworld,
			Position: mgl32.Vec3{1, 2, -4}, Velocity: mgl32.Vec3{1, 0, -30}},
	}}); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("spawn 后 renderFrame=(%v,%v)", rendered, err)
	}
	if got, want := len(app.projectileStream), 2*96; got != want {
		t.Fatalf("投射物实例流=%d 字节，想要 %d", got, want)
	}
	// 投射物不进入 avatar 通道与名标集合。
	for _, avatar := range app.remoteAvatars {
		if avatar.Key.Kind == render.EntityHostile || avatar.Key.Kind == render.EntityPassive {
			t.Fatalf("投射物误入 avatar 通道：%v", avatar.Key)
		}
	}

	if err := app.projectiles.ApplyDespawn(network.ProjectileDespawn{ServerTick: 2, IDs: []uint64{5, 6}}); err != nil {
		t.Fatalf("ApplyDespawn: %v", err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("despawn 后 renderFrame=(%v,%v)", rendered, err)
	}
	if got := len(app.projectileStream); got != 0 {
		t.Fatalf("despawn 后实例流=%d 字节，想要空", got)
	}
}

// TestApplicationPassesHostileKindToAvatarAssembly 锁定敌怪类别的装配通路：
// spawn record 的 kind 字节直达 avatar 部件装配（夜行者零值与掷骨者各归
// 其分支），名标与 avatar 计数不受影响。
func TestApplicationPassesHostileKindToAvatarAssembly(t *testing.T) {
	glyphs := &IntegrationGlyphSource{}
	app := newRemoteRenderApplication(t, glyphs)
	app.companions = &client.Companions{}
	configureTargetFeedback(t, app)
	if err := app.hostiles.ApplySpawn(network.HostileSpawn{ServerTick: 1, Spawns: []network.HostileSpawnRecord{
		{ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{-2, 1, -6}, Yaw: 0.25, Health: 13,
			Kind: network.HostileKindNightwalker},
		{ID: 2, Dimension: core.Overworld, Position: mgl32.Vec3{4, 1, -6}, Yaw: -1.5, Health: 20,
			Kind: network.HostileKindBoneThrower},
	}}); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}

	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("renderFrame=(%v,%v)", rendered, err)
	}
	kinds := map[uint64]uint8{}
	for _, avatar := range app.remoteAvatars {
		if avatar.Key.Kind == render.EntityHostile {
			kinds[hostileIDOfKey(avatar.Key)] = avatar.HostileKind
		}
	}
	if len(kinds) != 2 {
		t.Fatalf("avatar 通道中的敌怪=%d，想要 2", len(kinds))
	}
	if kinds[1] != render.HostileKindNightwalker {
		t.Fatalf("夜行者 kind=%d，想要 %d", kinds[1], render.HostileKindNightwalker)
	}
	if kinds[2] != render.HostileKindBoneThrower {
		t.Fatalf("掷骨者 kind=%d，想要 %d", kinds[2], render.HostileKindBoneThrower)
	}
}
