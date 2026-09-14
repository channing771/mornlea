package render

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定 viewmodel 编码：双手与第三人称手臂同源三值、单帧实例有界、
// 调用方缓冲零分配、无输入输出为空。

func partSize(transform [16]float32) [3]float32 {
	column := func(base int) float32 {
		return float32(math.Sqrt(float64(
			transform[base]*transform[base] +
				transform[base+1]*transform[base+1] +
				transform[base+2]*transform[base+2])))
	}
	return [3]float32{column(0), column(4), column(8)}
}

// decodedPartSize 从 96 字节实例流中还原第 index 个实例的缩放三轴：旋转
// 保持列范数，故挥动中同样可读尺寸。
func decodedPartSize(out []byte, index int) [3]float32 {
	var transform [16]float32
	for element := range 16 {
		transform[element] = math.Float32frombits(binary.LittleEndian.Uint32(out[index*avatarInstanceBytes+element*4:]))
	}
	return partSize(transform)
}

func decodedPartColor(out []byte, index int) [4]float32 {
	var color [4]float32
	for channel := range 4 {
		color[channel] = math.Float32frombits(binary.LittleEndian.Uint32(out[index*avatarInstanceBytes+64+channel*4:]))
	}
	return color
}

func decodedPartMaterial(out []byte, index int) uint32 {
	return binary.LittleEndian.Uint32(out[index*avatarInstanceBytes+80:])
}

func approxEqual(left, right float32) bool {
	diff := left - right
	return diff > -1e-5 && diff < 1e-5
}

func TestViewmodelHandsMatchAvatarArmStyle(t *testing.T) {
	player := core.PlayerID{21, 22, 23}
	encoder := &ViewmodelEncoder{}
	out := append([]byte(nil), encoder.EncodeViewmodelInstances(nil, viewmodelTestInput(player, core.ItemStack{}, 10))...)
	if len(out) != 8*avatarInstanceBytes {
		t.Fatalf("空手实例数 = %d，想要 8", len(out)/avatarInstanceBytes)
	}
	key := EntityKey{Kind: EntityPlayer, ID: [16]byte(player)}

	headMaterial := uint32(assets.LayerHumanSageHead)
	if swingPhaseID(key)%2 != 0 {
		headMaterial = uint32(assets.LayerHumanClayHead)
	}
	coatPixels := viewmodelDefaultRegistry.LayerRGBA(int(headMaterial) + 12)
	offset := (6*16 + 8) * 4
	wantColor := [4]float32{float32(coatPixels[offset]) / 255, float32(coatPixels[offset+1]) / 255, float32(coatPixels[offset+2]) / 255, 1}
	for index := range 1 {
		if color := decodedPartColor(out, index); color != wantColor {
			t.Fatalf("第 %d 只手颜色 = %v，想要 %v（与同身份袖子布料同源）", index, color, wantColor)
		}
		if material := decodedPartMaterial(out, index); material != avatarMaterialSolid {
			t.Fatalf("第 %d 只手材质 = %d，想要当前身份布料色的独立分面", index, material)
		}
		if size := decodedPartSize(out, index); !approxEqual(size[0], 0.17) || size[1] > .8 || !approxEqual(size[2], 0.185) {
			t.Fatalf("第 %d 只手尺寸 = %v，想要短前臂截面 0.17×0.185", index, size)
		}
	}
}

func TestViewmodelFrameInstanceBound(t *testing.T) {
	if ViewmodelMaxInstances != 264 {
		t.Fatalf("单帧实例上限 = %d，想要 264（主手与图标棱柱）", ViewmodelMaxInstances)
	}
	player := core.PlayerID{25}
	stacks := []core.ItemStack{
		{},
		{Item: core.ItemStone, Count: 1},
		{Item: core.ItemIronSword, Count: 1},
		{Item: core.ItemBread, Count: 1},
	}
	for _, stack := range stacks {
		encoder := &ViewmodelEncoder{}
		input := viewmodelTestInput(player, stack, 30)
		input.SwingActive = true
		input.SwingPhase = .35
		out := encoder.EncodeViewmodelInstances(nil, input)
		if count := len(out) / avatarInstanceBytes; count > ViewmodelMaxInstances {
			t.Fatalf("持物(%d)实例数 = %d，超出单帧上限", stack.Item, count)
		}
	}
}

func TestViewmodelNilInputEncodesEmpty(t *testing.T) {
	encoder := &ViewmodelEncoder{}
	live := viewmodelTestInput(core.PlayerID{27}, core.ItemStack{Item: core.ItemStone, Count: 1}, 40)
	live.SwingActive = true
	live.SwingPhase = .35
	before := append([]byte(nil), encoder.EncodeViewmodelInstances(nil, live)...)
	if len(before) == 0 {
		t.Fatalf("前置条件崩了：有输入时编码为空")
	}
	if got := encoder.EncodeViewmodelInstances(nil, nil); len(got) != 0 {
		t.Fatalf("无输入编码长度 = %d，想要空", len(got))
	}
	// 空输入不改变显式动作相位；恢复后与同输入直接编码一致。
	next := viewmodelTestInput(core.PlayerID{27}, core.ItemStack{Item: core.ItemStone, Count: 1}, 41)
	next.SwingActive = true
	next.SwingPhase = .35
	encoder.EncodeViewmodelInstances(nil, nil)
	got := append([]byte(nil), encoder.EncodeViewmodelInstances(nil, next)...)
	replay := &ViewmodelEncoder{}
	replayLive := viewmodelTestInput(core.PlayerID{27}, core.ItemStack{Item: core.ItemStone, Count: 1}, 40)
	replayLive.SwingActive = true
	replayLive.SwingPhase = .35
	replay.EncodeViewmodelInstances(nil, replayLive)
	want := append([]byte(nil), replay.EncodeViewmodelInstances(nil, next)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("空输入扰动了编码器状态，想要后一帧与无空隙序列一致")
	}
}

func TestViewmodelEncodeZeroAlloc(t *testing.T) {
	encoder := &ViewmodelEncoder{}
	input := viewmodelTestInput(core.PlayerID{29}, core.ItemStack{Item: core.ItemStone, Count: 1}, 60)
	input.SwingActive = true
	input.SwingPhase = .35
	dst := make([]byte, 0, ViewmodelMaxInstances*avatarInstanceBytes)
	encoder.EncodeViewmodelInstances(dst, input)
	allocs := testing.AllocsPerRun(20, func() {
		encoder.EncodeViewmodelInstances(dst, input)
	})
	if allocs != 0 {
		t.Fatalf("热路径分配 = %v，想要零分配（复用调用方缓冲）", allocs)
	}
}
