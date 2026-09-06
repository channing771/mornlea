package render

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定 viewmodel 的世界烘焙投影：`ViewmodelInput` 携带的本帧相机位姿
// 派生根变换，既有相机空间偏移经根变换烘焙为世界变换后由既有世界投影绘制；
// 双手因此恒落在屏幕左右区域，而非钉在世界原点附近。

// viewmodelProjectionFraming 是落点测试的投影帧：70° 视场、方形帧、近
// 0.1 远 100；双手恒在相机前方 0.8 米处，远平面取值不影响落点。
const (
	viewmodelProjectionFovY   = float32(70 * math.Pi / 180)
	viewmodelProjectionAspect = float32(1)
	viewmodelProjectionNear   = float32(0.1)
	viewmodelProjectionFar    = float32(100)
)

// viewmodelProjectionViewProj 逐字复用展示相机的投影口径（`Forward` 的朝向公
// 式、视图矩阵的 LookAtV 构造与 `core.Perspective` 的投影按 `ViewProj` 同序组合），
// 作为落点测试的 oracle；`render` 不得反向依赖展示相机所在包，此处只复制数
// 学公式，不引入包边。
func viewmodelProjectionViewProj(pos mgl32.Vec3, yaw, pitch float32) mgl32.Mat4 {
	cp := float32(math.Cos(float64(pitch)))
	forward := mgl32.Vec3{
		-float32(math.Sin(float64(yaw))) * cp,
		float32(math.Sin(float64(pitch))),
		-float32(math.Cos(float64(yaw))) * cp,
	}
	view := mgl32.LookAtV(pos, pos.Add(forward), mgl32.Vec3{0, 1, 0})
	return core.Perspective(
		viewmodelProjectionFovY, viewmodelProjectionAspect,
		viewmodelProjectionNear, viewmodelProjectionFar,
	).Mul4(view)
}

// decodedPartCenter 从 96 字节实例流中还原第 index 个实例的世界中心：实例
// 几何以局部原点为中心，中心即变换矩阵的平移列。
func decodedPartCenter(out []byte, index int) mgl32.Vec3 {
	base := index * avatarInstanceBytes
	return mgl32.Vec3{
		math.Float32frombits(binary.LittleEndian.Uint32(out[base+12*4:])),
		math.Float32frombits(binary.LittleEndian.Uint32(out[base+13*4:])),
		math.Float32frombits(binary.LittleEndian.Uint32(out[base+14*4:])),
	}
}

// projectToNDC 把世界点经世界投影变为归一化设备坐标，w 随返回值供调用方断
// 言点在相机前方。
func projectToNDC(viewProj mgl32.Mat4, point mgl32.Vec3) (ndc mgl32.Vec3, w float32) {
	clip := viewProj.Mul4x1(mgl32.Vec4{point[0], point[1], point[2], 1})
	w = clip[3]
	return mgl32.Vec3{clip[0] / w, clip[1] / w, clip[2] / w}, w
}

// assertHandsLandedOnScreen 是落点断言的唯一落点：给定相机位姿下中立双手
// 经世界投影落在屏幕内，左手在左半屏、右手在右半屏。双手定义在相机空间，落
// 点 NDC 与世界位姿无关；根的顺序/符号一旦写错，烘焙出的世界点经世界投影即
// 偏离相机空间原位，断言变红。
func assertHandsLandedOnScreen(t *testing.T, pos mgl32.Vec3, yaw, pitch float32) {
	t.Helper()
	input := viewmodelTestInput(core.PlayerID{41}, core.ItemStack{}, 10)
	input.CamPos, input.CamYaw, input.CamPitch = pos, yaw, pitch
	out := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)
	if len(out) != 2*avatarInstanceBytes {
		t.Fatalf("中立实例数 = %d，想要 2（左右手）", len(out)/avatarInstanceBytes)
	}
	viewProj := viewmodelProjectionViewProj(pos, yaw, pitch)
	const framePixels = 480
	var ndc [2]mgl32.Vec3
	for index := range 2 {
		center := decodedPartCenter(out, index)
		if distance := center.Sub(pos).Len(); distance > 1.5 {
			t.Fatalf("第 %d 只手距相机 %.2f 米，想要 1.5 米内（烘焙到本帧相机处）", index, distance)
		}
		point, w := projectToNDC(viewProj, center)
		if w <= 0 {
			t.Fatalf("第 %d 只手 w=%.2f，想要在相机前方（w>0）", index, w)
		}
		if point[0] < -1 || point[0] > 1 || point[1] < -1 || point[1] > 1 {
			t.Fatalf("第 %d 只手 NDC=(%.2f,%.2f)，想要落在屏幕内", index, point[0], point[1])
		}
		ndc[index] = point
	}
	if ndc[0][0] >= 0 {
		t.Fatalf("左手 NDC x=%.2f，想要在左半屏（<0）", ndc[0][0])
	}
	if ndc[1][0] <= 0 {
		t.Fatalf("右手 NDC x=%.2f，想要在右半屏（>0）", ndc[1][0])
	}
	leftPx := (ndc[0][0]*0.5 + 0.5) * framePixels
	rightPx := (ndc[1][0]*0.5 + 0.5) * framePixels
	if leftPx < 0 || leftPx >= framePixels/2 || rightPx <= framePixels/2 || rightPx > framePixels {
		t.Fatalf("双手像素 x=%.0f/%.0f，想要分居 %d 像素帧的左右两半", leftPx, rightPx, framePixels)
	}
}

// TestViewmodelLandingProjectionOnScreen 锁定世界烘焙的落点：远离原点的固定
// 相机下双手经世界投影落在屏幕左右区域；旧行为下双手钉在世界原点附近（本相
// 机视锥之外），本测试必红。
func TestViewmodelLandingProjectionOnScreen(t *testing.T) {
	assertHandsLandedOnScreen(t, mgl32.Vec3{10, 3, 10}, 0, -0.1)
}

// TestViewmodelLandingProjectionYawedCamera 锁定非零偏航/俯仰下的根顺序与符
// 号：同一断言换一组偏航 0.6、俯仰 -0.25 的位姿重跑；`T·Ry·Rx` 之外的顺序或
// 符号会把双手烘焙到视锥外或左右互换，断言变红。
func TestViewmodelLandingProjectionYawedCamera(t *testing.T) {
	assertHandsLandedOnScreen(t, mgl32.Vec3{-3, 4, 7}, 0.6, -0.25)
}

// TestViewmodelZeroPoseRootIsIdentity 锁定零位姿的根变换为单位阵：既有零位
// 姿输入（全部历史测试与 Task 1/2 回归口径）走旧链，逐字节行为不动。
func TestViewmodelZeroPoseRootIsIdentity(t *testing.T) {
	if root := viewmodelRootFromCameraPose(mgl32.Vec3{}, 0, 0); root != mgl32.Ident4() {
		t.Fatalf("零位姿根变换 = %v，想要单位阵", root)
	}
}

// TestViewmodelPoseEntersReplayBytes 锁定重放语义含相机位姿：同位姿同输入序
// 列逐字节一致，换位姿字节必变（位姿若死亡，烘焙即丢失而本测试变红）。
func TestViewmodelPoseEntersReplayBytes(t *testing.T) {
	pose := mgl32.Vec3{10, 3, 10}
	input := viewmodelTestInput(core.PlayerID{43}, core.ItemStack{Item: core.ItemStone, Count: 1}, 20)
	input.CamPos, input.CamYaw, input.CamPitch = pose, 0, -0.1
	first := append([]byte(nil), (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)...)
	replay := viewmodelTestInput(core.PlayerID{43}, core.ItemStack{Item: core.ItemStone, Count: 1}, 20)
	replay.CamPos, replay.CamYaw, replay.CamPitch = pose, 0, -0.1
	if second := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, replay); string(second) != string(first) {
		t.Fatal("同位姿同输入两次编码不一致，想要逐字节相同")
	}
	moved := viewmodelTestInput(core.PlayerID{43}, core.ItemStack{Item: core.ItemStone, Count: 1}, 20)
	moved.CamPos, moved.CamYaw, moved.CamPitch = pose.Add(mgl32.Vec3{1, 0, 0}), 0, -0.1
	if shifted := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, moved); string(shifted) == string(first) {
		t.Fatal("相机平移后编码不变，想要位姿进入烘焙字节")
	}
}
