package protocol

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// smeltingOutputImage 遍历整个 `ItemID` 值域推导 `core.SmeltingOutput` 的
// 全部产物，而不是手写输入清单：未来扩展熔炼映射时，新产物自动进入本
// 守卫的期望集合，漏改输出白名单会让测试立刻失败。
func smeltingOutputImage() map[core.ItemID]bool {
	image := make(map[core.ItemID]bool)
	for id := range uint32(math.MaxUint16) + 1 {
		if output, ok := core.SmeltingOutput(core.ItemID(id)); ok {
			image[output] = true
		}
	}
	return image
}

func testFurnaceRef() core.FurnaceRef {
	return core.FurnaceRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: 2, Z: -5},
		Kind:       core.ContainerKindFurnace,
		Slot:       3,
		Generation: 7,
	}
}

// TestFurnaceStateOutputWhitelistMatchesSmeltingImage 锁定协议层
// `validFurnaceOutput` 的白名单恰好等于 `core.SmeltingOutput` 的全部产物
// ∪ {ItemNone}：少一项时，权威 tick 把该产物写进输出格的同一个 tick，
// 服务端发送侧校验失败会杀掉全部查看者会话，客户端解码同样拒绝，
// 熔炉界面整体不可用；多一项则放宽了固定产物约束。
func TestFurnaceStateOutputWhitelistMatchesSmeltingImage(t *testing.T) {
	image := smeltingOutputImage()
	if len(image) == 0 {
		t.Fatal("熔炼产物表为空，守卫失去意义")
	}
	for id := range uint32(math.MaxUint16) + 1 {
		item := core.ItemID(id)
		state := FurnaceState{Furnace: testFurnaceRef()}
		if item != core.ItemNone {
			state.Output = core.ItemStack{Item: item, Count: 1}
		}
		err := state.Validate()
		want := item == core.ItemNone || image[item]
		if want && err != nil {
			t.Errorf("FurnaceState 输出格装熔炼产物 %d 被拒绝: %v", item, err)
		}
		if !want && err == nil {
			t.Errorf("FurnaceState 输出格装非产物物品 %d 被接受", item)
		}
	}
}

// TestValidateServerPacketAcceptsCookedBeefFurnaceState 跨发送侧校验边界
// 回归锁定：输出格装熟牛肉的完整熔炉状态必须能通过 `ValidateServerPacket`
// 放行——否则熟牛肉入炉的同一个 tick，每名查看者的会话都会被发送侧
// 校验失败杀掉。
func TestValidateServerPacketAcceptsCookedBeefFurnaceState(t *testing.T) {
	state := FurnaceState{
		Furnace:       testFurnaceRef(),
		Input:         core.ItemStack{Item: core.ItemRawBeef, Count: 2},
		Fuel:          core.ItemStack{Item: core.ItemCoal, Count: 1},
		Output:        core.ItemStack{Item: core.ItemCookedBeef, Count: 3},
		ProgressTicks: 137,
		BurnTicks:     1463,
	}
	if err := ValidateServerPacket(StatePlay, state); err != nil {
		t.Fatalf("输出格装熟牛肉的 FurnaceState 被发送侧校验拒绝: %v", err)
	}
}
