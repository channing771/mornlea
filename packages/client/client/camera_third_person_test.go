package client_test

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
)

// openSolid 是开阔地夹具：沿线无完整不透明方块，相机应保持固定距离。
func openSolid(core.BlockPos) (bool, error) { return false, nil }

// TestResolveThirdPersonCameraOpenKeepsFixedDistance 锁定 spec 开阔场景：
// 背面位于眼睛沿视线反方向固定距离，正面位于正方向同等距离。
func TestResolveThirdPersonCameraOpenKeepsFixedDistance(t *testing.T) {
	eye := mgl32.Vec3{0.5, 10, 0.5}
	base := client.Camera{Pos: eye, Yaw: 0, Pitch: 0, FovY: 1.2, Aspect: 1.6, Near: 0.1, Far: 100}

	back := client.ResolveThirdPersonCamera(base, client.CameraThirdPersonBack, openSolid)
	wantBack := eye.Add(mgl32.Vec3{0, 0, client.ThirdPersonCameraDistance})
	if back.Pos.Sub(wantBack).Len() > 1e-4 {
		t.Fatalf("背面相机 = %v，想要眼睛后方固定距离 %v", back.Pos, wantBack)
	}
	if back.Yaw != 0 || back.Pitch != 0 {
		t.Fatalf("背面朝向 yaw/pitch = %v/%v，想要与眼睛一致 0/0", back.Yaw, back.Pitch)
	}

	front := client.ResolveThirdPersonCamera(base, client.CameraThirdPersonFront, openSolid)
	wantFront := eye.Add(mgl32.Vec3{0, 0, -client.ThirdPersonCameraDistance})
	if front.Pos.Sub(wantFront).Len() > 1e-4 {
		t.Fatalf("正面相机 = %v，想要眼睛前方固定距离 %v", front.Pos, wantFront)
	}
	if math.Abs(float64(front.Yaw-math.Pi)) > 1e-4 {
		t.Fatalf("正面 yaw = %v，想要眼睛 yaw+Pi（回望玩家以便看到脸部）", front.Yaw)
	}
}

// TestResolveThirdPersonCameraPullsInBeforeWall 锁定 spec 贴墙场景：
// 身后 1 格有不透明墙时相机收缩到墙前，不得进入墙内。
func TestResolveThirdPersonCameraPullsInBeforeWall(t *testing.T) {
	eye := mgl32.Vec3{0.5, 10, 0.5}
	base := client.Camera{Pos: eye, Yaw: 0, Pitch: 0, FovY: 1.2, Aspect: 1.6, Near: 0.1, Far: 100}
	wall := func(pos core.BlockPos) (bool, error) {
		return pos == (core.BlockPos{X: 0, Y: 10, Z: 1}), nil
	}
	got := client.ResolveThirdPersonCamera(base, client.CameraThirdPersonBack, wall)
	if got.Pos.Z() <= eye.Z() || got.Pos.Z() >= 1 {
		t.Fatalf("收缩后相机 = %v，想要眼睛(%v)与墙面(z=1)之间", got.Pos, eye)
	}
	if got.Pos.X() != eye.X() || got.Pos.Y() != eye.Y() {
		t.Fatalf("收缩后相机 = %v，想要只沿视线轴收缩", got.Pos)
	}
}

// TestResolveThirdPersonCameraFullyBlockedCollapsesToEye 锁定完全遮挡：
// 阻挡点比最小贴脸距离更近时收至眼睛处，不得穿墙。
func TestResolveThirdPersonCameraFullyBlockedCollapsesToEye(t *testing.T) {
	eye := mgl32.Vec3{0.5, 10, 0.9}
	base := client.Camera{Pos: eye, Yaw: 0, Pitch: 0, FovY: 1.2, Aspect: 1.6, Near: 0.1, Far: 100}
	wall := func(pos core.BlockPos) (bool, error) {
		return pos == (core.BlockPos{X: 0, Y: 10, Z: 1}), nil
	}
	got := client.ResolveThirdPersonCamera(base, client.CameraThirdPersonBack, wall)
	if got.Pos != eye {
		t.Fatalf("完全遮挡后相机 = %v，想要收至眼睛处 %v", got.Pos, eye)
	}
}

// TestResolveThirdPersonCameraFrontOpposesEyeForwardAtNonZeroPitch 锁定正面
// 非零俯仰下的回望语义：渲染前向恰为眼睛前向的反方向（点积约 -1），
// 俯仰取反（仅 yaw+Pi 不够，俯仰非零时会偏）。
func TestResolveThirdPersonCameraFrontOpposesEyeForwardAtNonZeroPitch(t *testing.T) {
	base := client.Camera{Pos: mgl32.Vec3{1.5, 65.25, -3.75}, Yaw: 0.7, Pitch: -0.25, FovY: 1.22173, Aspect: 16.0 / 9.0, Near: 0.1, Far: 1536}
	front := client.ResolveThirdPersonCamera(base, client.CameraThirdPersonFront, openSolid)
	if front.Pitch != -base.Pitch {
		t.Fatalf("正面 pitch = %v，想要眼睛 pitch 取反 %v", front.Pitch, -base.Pitch)
	}
	sum := front.Forward().Add(base.Forward())
	if sum.Len() > 1e-5 {
		t.Fatalf("正面与眼睛前向之和 = %v（模 %v），想要约零向量（点积约 -1）", sum, sum.Len())
	}
}

// TestResolveThirdPersonCameraIgnoresTransparentBlocks 锁定透明方块不触发
// 后拉：树叶/玻璃按 `core.BlockOpaque` 非完全不透明，相机保持完整的 4 格距离。
func TestResolveThirdPersonCameraIgnoresTransparentBlocks(t *testing.T) {
	eye := mgl32.Vec3{0.5, 10, 0.5}
	base := client.Camera{Pos: eye, Yaw: 0, Pitch: 0, FovY: 1.2, Aspect: 1.6, Near: 0.1, Far: 100}
	for _, id := range []core.BlockID{core.LeavesID, core.GlassID} {
		blocks := map[core.BlockPos]core.BlockID{{X: 0, Y: 10, Z: 1}: id}
		solid := func(pos core.BlockPos) (bool, error) {
			block, ok := blocks[pos]
			if !ok {
				return false, nil
			}
			return core.BlockOpaque(block), nil
		}
		got := client.ResolveThirdPersonCamera(base, client.CameraThirdPersonBack, solid)
		want := eye.Add(mgl32.Vec3{0, 0, client.ThirdPersonCameraDistance})
		if got.Pos.Sub(want).Len() > 1e-4 {
			t.Fatalf("透明方块 %d 后相机 = %v，想要完整距离 %v（不后拉）", id, got.Pos, want)
		}
	}
}

// TestResolveThirdPersonCameraFirstPersonKeepsEye 锁定第一人称直通：
// 相机即眼睛，不做后拉与射线查询（solid 为 nil 也不触碰）。
func TestResolveThirdPersonCameraFirstPersonKeepsEye(t *testing.T) {
	base := client.Camera{Pos: mgl32.Vec3{1, 2, 3}, Yaw: 0.4, Pitch: -0.2, FovY: 1.2, Aspect: 1.6, Near: 0.1, Far: 100}
	got := client.ResolveThirdPersonCamera(base, client.CameraFirstPerson, nil)
	if got != base {
		t.Fatalf("第一人称相机 = %+v，想要原样直通 %+v", got, base)
	}
}
