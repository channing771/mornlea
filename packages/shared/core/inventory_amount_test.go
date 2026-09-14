package core_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestInventoryMoveStackAmountMovesExplicitHalfOfSource 锁定部分移动的数量语义：
// amount 由调用方显式传入，本测试按来源数量 n 推导半组 ceil(n/2)（这正是服务端
// sim 层将要传入的值），覆盖 1..9 的奇偶边界。空目标恰好接收 ceil(n/2) 个，
// 来源保留 floor(n/2) 个，扣空时规范化为零值空栈，两侧总量守恒。
func TestInventoryMoveStackAmountMovesExplicitHalfOfSource(t *testing.T) {
	for count := uint8(1); count <= 9; count++ {
		half := (count + 1) / 2
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: count}

		next, ok := inventory.MoveStackAmount(0, core.HotbarSlots+3, half)
		if !ok {
			t.Fatalf("来源数量 %d 的半组移动应当成功", count)
		}
		wantSource := core.ItemStack{Item: core.ItemStone, Count: count - half}
		if wantSource.Count == 0 {
			wantSource = core.ItemStack{}
		}
		if next.Hotbar.Slots[0] != wantSource {
			t.Fatalf("来源数量 %d：来源格 = %+v，想要保留 %d 个", count, next.Hotbar.Slots[0], count-half)
		}
		if next.Backpack[3] != (core.ItemStack{Item: core.ItemStone, Count: half}) {
			t.Fatalf("来源数量 %d：目标格 = %+v，想要恰收 ceil(n/2)=%d 个", count, next.Backpack[3], half)
		}
		if int(next.Hotbar.Slots[0].Count)+int(next.Backpack[3].Count) != int(count) {
			t.Fatalf("来源数量 %d：两侧总量不守恒", count)
		}
		if inventory.Hotbar.Slots[0].Count != count {
			t.Fatalf("来源数量 %d：MoveStackAmount 必须在值副本上完成", count)
		}
	}
}

// TestInventoryMoveStackAmountMovesSingleIntoMergeableTarget 覆盖单件移动：
// 目标同类 63/64 时恰并入 1 个补满，来源减 1；同类目标只剩部分空间时按
// min(amount, 空间) 截断、余量留源（「至多 amount」而非「恰 amount」）。
func TestInventoryMoveStackAmountMovesSingleIntoMergeableTarget(t *testing.T) {
	t.Run("单件补满 63/64", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemArrow, Count: 5}
		inventory.Backpack[6] = core.ItemStack{Item: core.ItemArrow, Count: core.MaxStackCount - 1}

		next, ok := inventory.MoveStackAmount(2, core.HotbarSlots+6, 1)
		if !ok {
			t.Fatal("单件并入同类未满格应当成功")
		}
		if next.Backpack[6].Count != core.MaxStackCount {
			t.Fatalf("目标格 = %+v，想要补满到 64", next.Backpack[6])
		}
		if next.Hotbar.Slots[2] != (core.ItemStack{Item: core.ItemArrow, Count: 4}) {
			t.Fatalf("来源格 = %+v，想要恰减 1", next.Hotbar.Slots[2])
		}
	})

	t.Run("合并受 stack limit 截断", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 30}
		inventory.Backpack[1] = core.ItemStack{Item: core.ItemStone, Count: 50}

		next, ok := inventory.MoveStackAmount(0, core.HotbarSlots+1, 20)
		if !ok {
			t.Fatal("目标只剩 14 空间时部分移动仍应搬入可容纳部分")
		}
		if next.Backpack[1].Count != core.MaxStackCount {
			t.Fatalf("目标格 = %+v，想要被截断补满到 64", next.Backpack[1])
		}
		if next.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 16}) {
			t.Fatalf("来源格 = %+v，想要余量 30−14=16 留源", next.Hotbar.Slots[0])
		}
	})

	t.Run("amount 超过来源时搬整堆", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Backpack[2] = core.ItemStack{Item: core.ItemDirt, Count: 7}

		next, ok := inventory.MoveStackAmount(core.HotbarSlots+2, 4, 99)
		if !ok {
			t.Fatal("amount 大于来源数量时按至多语义搬整堆应当成功")
		}
		if next.Backpack[2] != (core.ItemStack{}) {
			t.Fatalf("来源格 = %+v，想要清空", next.Backpack[2])
		}
		if next.Hotbar.Slots[4] != (core.ItemStack{Item: core.ItemDirt, Count: 7}) {
			t.Fatalf("目标格 = %+v，想要恰好 7 个（来源全部）", next.Hotbar.Slots[4])
		}
	})

	t.Run("单件工具移入空格带走耐久", func(t *testing.T) {
		var inventory core.Inventory
		tool := core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 73}
		inventory.Hotbar.Slots[0] = tool

		next, ok := inventory.MoveStackAmount(0, 1, 1)
		if !ok {
			t.Fatal("单件工具移入空格应当成功")
		}
		if next.Hotbar.Slots[1] != tool {
			t.Fatalf("目标格 = %+v，想要工具连同耐久字段一起移入", next.Hotbar.Slots[1])
		}
		if next.Hotbar.Slots[0] != (core.ItemStack{}) {
			t.Fatalf("来源格 = %+v，想要清空", next.Hotbar.Slots[0])
		}
		if !next.Valid() {
			t.Fatal("部分移动后的 Inventory 应当仍然有效")
		}
	})
}

// TestInventoryMoveStackAmountRejectsInvalidRequests 锁定全部拒绝路径的原子性：
// 同格、来源/目标越界、空来源、amount 为零、异类非空目标与同类满目标一律
// 返回原值和 false，整个 Inventory 结构逐字段不变（零改动）。
func TestInventoryMoveStackAmountRejectsInvalidRequests(t *testing.T) {
	base := core.Inventory{}
	base.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 10}
	base.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemGrass, Count: 4}
	base.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}
	base.Backpack[8] = core.ItemStack{Item: core.ItemArrow, Count: 9}

	cases := []struct {
		name         string
		from, to     uint8
		amount       uint8
		rejectReason string
	}{
		{"同格", 0, 0, 4, "部分移动没有交换语义，同格必须拒绝"},
		{"amount 为零", 0, 3, 0, "零数量是空操作，必须显式拒绝"},
		{"来源越界", core.InventorySlots, 0, 4, "越界索引"},
		{"目标越界", 0, core.InventorySlots, 4, "越界索引"},
		{"空来源", 5, 0, 4, "空来源无可搬运之物"},
		{"异类非空目标", 0, 1, 4, "部分移动不做交换"},
		{"同类目标已满", 0, 2, 4, "可移动量为零"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, ok := base.MoveStackAmount(tc.from, tc.to, tc.amount)
			if ok {
				t.Fatalf("%s：非法部分移动被接受为 %+v", tc.rejectReason, next)
			}
			if next != base {
				t.Fatalf("%s：拒绝路径必须整单零改动: %+v", tc.rejectReason, next)
			}
		})
	}
}

// TestInventoryMoveStackAmountContrastsMoveStack 锁定与整堆移动 `MoveStack` 的
// 行为差异表：异类非空目标上整堆移动交换、部分移动拒绝；空目标上整堆移动搬
// 全部、部分移动只搬 amount 个；可移动量为零时两者同形拒绝。
func TestInventoryMoveStackAmountContrastsMoveStack(t *testing.T) {
	t.Run("异类目标：整堆交换，部分移动拒绝", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
		inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemGrass, Count: 5}

		moved, ok := inventory.MoveStack(0, 1)
		if !ok || moved.Hotbar.Slots[0].Item != core.ItemGrass || moved.Hotbar.Slots[1].Item != core.ItemStone {
			t.Fatalf("MoveStack 异类交换 = %+v, %v，前提不成立", moved, ok)
		}
		next, ok := inventory.MoveStackAmount(0, 1, 3)
		if ok || next != inventory {
			t.Fatalf("异类目标部分移动 = %+v, %v，想要原值和 false（不交换）", next, ok)
		}
	})

	t.Run("空目标：整堆搬全部，部分移动只搬 amount 个", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 9}

		moved, ok := inventory.MoveStack(0, core.HotbarSlots)
		if !ok || moved.Backpack[0].Count != 9 {
			t.Fatalf("MoveStack 整堆移动 = %+v, %v，前提不成立", moved, ok)
		}
		next, ok := inventory.MoveStackAmount(0, core.HotbarSlots, 4)
		if !ok {
			t.Fatal("部分移动空目标应当成功")
		}
		if next.Backpack[0].Count != 4 || next.Hotbar.Slots[0].Count != 5 {
			t.Fatalf("部分移动 = %+v / %+v，想要目标 4 个、来源余 5 个", next.Backpack[0], next.Hotbar.Slots[0])
		}
	})

	t.Run("同类满目标：两者同形拒绝", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
		inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}

		if moved, ok := inventory.MoveStack(0, 1); ok || moved != inventory {
			t.Fatalf("MoveStack 同类满目标 = %+v, %v，前提不成立", moved, ok)
		}
		if next, ok := inventory.MoveStackAmount(0, 1, 2); ok || next != inventory {
			t.Fatalf("MoveStackAmount 同类满目标 = %+v, %v，想要与 MoveStack 同形拒绝", next, ok)
		}
	})
}
