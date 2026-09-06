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

// viewmodelProjectionCamera 是落点测试的固定相机：远离世界原点，朝向与原点
// 无关；旧行为（根为单位阵、偏移直作世界坐标）下双手留在原点附近，恒在视锥
// 之外，测试必红。
var viewmodelProjectionCamera = struct {
	pos          mgl32.Vec3
	yaw, pitch   float32
	fovY, aspect float32
	near, far    float32
}{
	pos:    mgl32.Vec3{10, 3, 10},
	yaw:    0,
	pitch:  -0.1,
	fovY:   mgl32.DegToRad(70),
	aspect: 1,
	near:   0.1,
	far:    100,
}

// viewmodelProjectionViewProj 逐字复用展示相机的投影口径（`Forward` 的朝向公
// 式、视图矩阵的 LookAtV 构造与 `core.Perspective` 的投影按 `ViewProj` 同序组合），
// 作为落点测试的 oracle；`render` 不得反向依赖展示相机所在包，此处只复制数
// 学公式，不引入包边。
func viewmodelProjectionViewProj() mgl32.Mat4 {
	cam := viewmodelProjectionCamera
	cp := float32(math.Cos(float64(cam.pitch)))
	forward := mgl32.Vec3{
		-float32(math.Sin(float64(cam.yaw))) * cp,
		float32(math.Sin(float64(cam.pitch))),
		-float32(math.Cos(float64(cam.yaw))) * cp,
	}
	view := mgl32.LookAtV(cam.pos, cam.pos.Add(forward), mgl32.Vec3{0, 1, 0})
	return core.Perspective(cam.fovY, cam.aspect, cam.near, cam.far).Mul4(view)
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

// TestViewmodelLandingProjectionOnScreen 锁定世界烘焙的落点：固定相机下中立
// 双手经世界投影落在屏幕内，左手在左半屏、右手在右半屏；旧行为下双手钉在世
// 界原点附近（本相机视锥之外），本测试必红。
func TestViewmodelLandingProjectionOnScreen(t *testing.T) {
	cam := viewmodelProjectionCamera
	input := viewmodelTestInput(core.PlayerID{41}, core.ItemStack{}, 10)
	input.CamPos, input.CamYaw, input.CamPitch = cam.pos, cam.yaw, cam.pitch
	out := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)
	if len(out) != 2*avatarInstanceBytes {
		t.Fatalf("中立实例数 = %d，想要 2（左右手）", len(out)/avatarInstanceBytes)
	}
	viewProj := viewmodelProjectionViewProj()
	const framePixels = 480
	var ndc [2]mgl32.Vec3
	for index := range 2 {
		center := decodedPartCenter(out, index)
		if distance := center.Sub(cam.pos).Len(); distance > 1.5 {
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
	cam := viewmodelProjectionCamera
	input := viewmodelTestInput(core.PlayerID{43}, core.ItemStack{Item: core.ItemStone, Count: 1}, 20)
	input.CamPos, input.CamYaw, input.CamPitch = cam.pos, cam.yaw, cam.pitch
	first := append([]byte(nil), (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, input)...)
	replay := viewmodelTestInput(core.PlayerID{43}, core.ItemStack{Item: core.ItemStone, Count: 1}, 20)
	replay.CamPos, replay.CamYaw, replay.CamPitch = cam.pos, cam.yaw, cam.pitch
	if second := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, replay); string(second) != string(first) {
		t.Fatal("同位姿同输入两次编码不一致，想要逐字节相同")
	}
	moved := viewmodelTestInput(core.PlayerID{43}, core.ItemStack{Item: core.ItemStone, Count: 1}, 20)
	moved.CamPos, moved.CamYaw, moved.CamPitch = cam.pos.Add(mgl32.Vec3{1, 0, 0}), cam.yaw, cam.pitch
	if shifted := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, moved); string(shifted) == string(first) {
		t.Fatal("相机平移后编码不变，想要位姿进入烘焙字节")
	}
}
