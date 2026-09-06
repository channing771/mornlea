//go:build darwin

package app

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// TestResolveRenderCameraOpenKeepsDistance 锁定开阔地渲染相机后拉：
// 眼睛位置不动（瞄准仍用眼睛），渲染位姿为眼睛沿视线反方向固定距离。
func TestResolveRenderCameraOpenKeepsDistance(t *testing.T) {
	app := newTargetBlockApplication(t, true, core.ChunkPos{}, core.ChunkPos{Z: -1})
	app.camera.Pos = mgl32.Vec3{0.5, 10, 0.5}
	app.camera.Yaw = 0
	app.camera.Pitch = 0
	app.cameraMode = client.CameraThirdPersonBack
	got := app.resolveRenderCamera()
	want := mgl32.Vec3{0.5, 10, 0.5 + client.ThirdPersonCameraDistance}
	if got.Pos.Sub(want).Len() > 1e-4 {
		t.Fatalf("渲染相机 = %v，想要 %v", got.Pos, want)
	}
	if app.camera.Pos != (mgl32.Vec3{0.5, 10, 0.5}) {
		t.Fatalf("眼睛位置 = %v，想要保持不动（瞄准仍用眼睛）", app.camera.Pos)
	}
}

// TestResolveRenderCameraPullsInBeforeWall 锁定贴墙收缩：渲染相机停在
// 不透明墙前，不得进入墙内。
func TestResolveRenderCameraPullsInBeforeWall(t *testing.T) {
	app := newTargetBlockApplication(t, true, core.ChunkPos{}, core.ChunkPos{Z: -1})
	for _, chunkPos := range []core.ChunkPos{{X: 0, Z: 1}} {
		applyTargetMirrorChunk(t, app.mirror, world.NewChunk(chunkPos))
	}
	setTargetMirrorBlock(t, app.mirror, core.BlockPos{X: 0, Y: 10, Z: 1}, core.StoneID)
	app.camera.Pos = mgl32.Vec3{0.5, 10, 0.5}
	app.camera.Yaw = 0
	app.camera.Pitch = 0
	app.cameraMode = client.CameraThirdPersonBack
	got := app.resolveRenderCamera()
	if got.Pos.Z() <= 0.5 || got.Pos.Z() >= 1 {
		t.Fatalf("渲染相机 = %v，想要眼睛与墙面(z=1)之间", got.Pos)
	}
}

// TestCurrentBlockTargetUnchangedInThirdPerson 锁定瞄准仍用眼睛射线：
// 切第三人称后 `CurrentBlockTarget` 与第一人称命中同一方块。
func TestCurrentBlockTargetUnchangedInThirdPerson(t *testing.T) {
	app := targetBlockHitApplication(t)
	first, ok := app.CurrentBlockTarget()
	if !ok {
		t.Fatal("第一人称无命中，夹具失效")
	}
	app.cameraMode = client.CameraThirdPersonBack
	third, ok := app.CurrentBlockTarget()
	if !ok || third != first {
		t.Fatalf("第三人称瞄准 = %+v，想要与第一人称 %+v 一致（眼睛射线）", third, first)
	}
}
