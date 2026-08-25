package sim_test

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim"
)

// crafting_test.go：合成命令在 sim 层的语义。任务组 2 将在本文件扩展网格
// 移动与产物取出；当前唯一主题是 recipe-click 的过渡拒绝（design.md D6）。

// TestCraftRecipeCommandIsStablyRejected 锁定 recipe-click 的过渡语义：
// 即使玩家备齐了完整配方原料，`CommandCraftRecipe` 也 MUST 按既有命令拒绝
// 路径稳定回拒（`RejectInvalidInput`），不发布任何物品状态、完整物品状态
// 逐格不变——真实合成只能经格子工作台路径发生（任务组 2 接入），这里绝无
// 第二条执行路径。
//
// 夹具刻意用面包配方与 3 个小麦：这条链路曾是农业闭环「种地 → 合成 →
// 吃饭」的出口（原先由 eating_test.go 的命令级用例守着），过渡期先在这里
// 钉住「命令路径不再产出」这半边；产出半边由任务组 2 的网格取出用例接管。
func TestCraftRecipeCommandIsStablyRejected(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemWheat, Count: 3}
	engine, session := readyFlatPlayerWithInventory(t, stocked)
	engine.Enqueue(sim.Command{
		Session:  session,
		Sequence: 2,
		Kind:     sim.CommandCraftRecipe,
		Recipe:   core.RecipeBread,
	})

	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != sim.RejectInvalidInput {
		t.Fatalf("合成命令 result=%+v，想要恰一条 invalid_input 拒绝", result)
	}
	if len(result.Inventories) != 0 {
		t.Fatalf("被拒绝的合成仍发布物品状态: %+v", result.Inventories)
	}
	if got := currentInventory(t, engine, session); got != stocked {
		t.Fatalf("被拒绝的合成修改了完整物品状态: %+v，想要原值 %+v", got, stocked)
	}
}
