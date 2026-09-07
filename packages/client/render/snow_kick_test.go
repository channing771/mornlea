package render

import (
	"bytes"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// snowKickCommonInput 是粒子测试的公共驱动输入：脚位取整格中心附近，
// 事件沿显式给定。
func snowKickCommonInput(kick bool) SnowKickInput {
	return SnowKickInput{Kick: kick, Feet: mgl32.Vec3{12.5, 40, -7.5}}
}

// TestSnowKickPartsFollowEventLifetime 证踢雪尘的事件寿命：事件沿建锚后恰
// `SnowKickParticles` 粒，随 tick 老化仍存续并在寿命到期后归零；无事件的帧
// 恒为零粒子。
func TestSnowKickPartsFollowEventLifetime(t *testing.T) {
	if SnowKickParticles != 8 {
		t.Fatalf("SnowKickParticles = %d，想要 8（design ≤8 粒/事件）", SnowKickParticles)
	}
	var kicks SnowKicks
	if parts := kicks.BuildParts(nil, 10, snowKickCommonInput(false)); len(parts) != 0 {
		t.Fatalf("无事件帧粒子数 = %d，想要 0", len(parts))
	}
	parts := kicks.BuildParts(nil, 10, snowKickCommonInput(true))
	if len(parts) != SnowKickParticles {
		t.Fatalf("事件帧粒子数 = %d，想要 %d", len(parts), SnowKickParticles)
	}
	if aging := kicks.BuildParts(nil, 11, snowKickCommonInput(false)); len(aging) != SnowKickParticles {
		t.Fatalf("老化帧粒子数 = %d，想要 %d", len(aging), SnowKickParticles)
	}
	if expired := kicks.BuildParts(nil, 10+snowKickLifetimeTicks, snowKickCommonInput(false)); len(expired) != 0 {
		t.Fatalf("到期帧粒子数 = %d，想要 0", len(expired))
	}
}

// TestSnowKickPartsDeterministicAndLocal 证确定性派生与脚部邻域：同锚点同
// tick 两次装配逐字段一致；粒子以事件脚位为原点小半径散布、纵向只向上扬起，
// 尺寸随年龄收缩、颜色与材质走纯色哨兵分支。
func TestSnowKickPartsDeterministicAndLocal(t *testing.T) {
	first := (&SnowKicks{}).BuildParts(nil, 20, snowKickCommonInput(true))
	second := (&SnowKicks{}).BuildParts(nil, 20, snowKickCommonInput(true))
	if len(first) != len(second) {
		t.Fatalf("同输入两次装配粒子数 %d != %d", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("粒子 %d 两次装配不一致：%v != %v", index, first[index], second[index])
		}
	}
	feet := snowKickCommonInput(true).Feet
	agingKicks := &SnowKicks{}
	agingKicks.BuildParts(nil, 20, snowKickCommonInput(true))
	older := agingKicks.BuildParts(nil, 20+snowKickLifetimeTicks/2, snowKickCommonInput(false))
	if len(older) != SnowKickParticles {
		t.Fatalf("半寿命帧粒子数 = %d，想要 %d", len(older), SnowKickParticles)
	}
	for index := range first {
		if first[index].material != avatarMaterialSolid || first[index].color != weatherSnowColor {
			t.Fatalf("粒子 %d 材质/颜色 %+v 不走纯色雪白分支", index, first[index])
		}
		// 事件帧尺寸取首帧边长；半寿命帧同粒序号尺寸严格更小（随年龄收缩）。
		if !(snowKickPartSize(older[index]) < snowKickPartSize(first[index])) {
			t.Fatalf("粒子 %d 半寿命尺寸 %v 未小于事件帧 %v", index,
				snowKickPartSize(older[index]), snowKickPartSize(first[index]))
		}
		dx := snowKickPartCenter(first[index]).X() - feet.X()
		dy := snowKickPartCenter(first[index]).Y() - feet.Y()
		dz := snowKickPartCenter(first[index]).Z() - feet.Z()
		if dx < -1 || dx > 1 || dz < -1 || dz > 1 {
			t.Fatalf("粒子 %d 水平散布 (%v,%v) 超出脚部邻域", index, dx, dz)
		}
		if dy < 0 {
			t.Fatalf("粒子 %d 纵向偏移 %v 不应低于脚位", index, dy)
		}
	}
}

// TestSnowKickPartsBytesStableAcrossFrames 证同输入两帧输出逐字节一致（装配
// 纯函数纪律）：事件后固定 tick 重放，字节流不因缓冲复用而漂移。
func TestSnowKickPartsBytesStableAcrossFrames(t *testing.T) {
	kicks := &SnowKicks{}
	encode := func(tick uint64, input SnowKickInput) []byte {
		parts := kicks.BuildParts(nil, tick, input)
		out := make([]byte, len(parts)*avatarInstanceBytes)
		encodeAvatarPartsInto(out, parts)
		return out
	}
	if stream := encode(30, snowKickCommonInput(true)); len(stream) != SnowKickParticles*avatarInstanceBytes {
		t.Fatalf("事件帧流 = %d 字节，想要 %d", len(stream), SnowKickParticles*avatarInstanceBytes)
	}
	aging := encode(31, snowKickCommonInput(false))
	replay := encode(31, snowKickCommonInput(false))
	if !bytes.Equal(aging, replay) {
		t.Fatal("同输入两帧编码字节不一致")
	}
}

// snowKickPartCenter 从实例变换取平移分量：avatar 布局是列主序 mat4，平移落在
// 第 4 列（下标 12..14）。
func snowKickPartCenter(part avatarPart) mgl32.Vec3 {
	return mgl32.Vec3{part.transform[12], part.transform[13], part.transform[14]}
}

// snowKickPartSize 从实例变换取均匀缩放：取 X 轴基向量长度（Scale3D 对角元）。
func snowKickPartSize(part avatarPart) float32 {
	return part.transform[0]
}
