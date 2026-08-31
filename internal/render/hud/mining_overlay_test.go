package hud

// mining_overlay_test.go：`MiningOverlay.Target/HasTarget` 是世界空间裂纹
// 呈现（`internal/render.BlockCrack`）的定位字段，HUD 进度条布局不得消费
// 它们；字段零值（capture 既有 fixture 不设置）时进度条行为与既有语义一致。

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// HUD 布局不消费 Target/HasTarget：同进度下携带权威目标与不携带目标的
// overlay 产生完全相同的进度条 quad。
func TestMiningOverlayTargetFieldsNotConsumedByHUDLayout(t *testing.T) {
	base := MiningOverlay{Active: true, ProgressTicks: 9, RequiredTicks: 30, Harvestable: true}
	targeted := base
	targeted.Target = core.BlockPos{X: 7, Y: 8, Z: 9}
	targeted.HasTarget = true
	var plain, withTarget hotbarLayout
	appendMiningBar(&plain, base, 1280, 800)
	appendMiningBar(&withTarget, targeted, 1280, 800)
	if len(plain.quads) == 0 {
		t.Fatal("基准 overlay 没有产生进度条 quad，测试前置失效")
	}
	if !reflect.DeepEqual(plain, withTarget) {
		t.Fatalf("Target/HasTarget 改变了 HUD 进度条布局: %+v vs %+v", plain, withTarget)
	}
}

// Target/HasTarget 零值安全：非 active 不产生 quad（裂纹字段不得唤醒进度
// 条），active 时进度条照常出现。
func TestMiningOverlayZeroTargetFieldsKeepBarBehavior(t *testing.T) {
	var inactive hotbarLayout
	appendMiningBar(&inactive, MiningOverlay{Target: core.BlockPos{X: 1, Y: 2, Z: 3}}, 1280, 800)
	if len(inactive.quads) != 0 {
		t.Fatalf("非 active overlay 产生了 %d 个 quad", len(inactive.quads))
	}
	var active hotbarLayout
	appendMiningBar(&active, MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}, 1280, 800)
	if len(active.quads) == 0 {
		t.Fatal("active overlay 没有产生进度条 quad")
	}
}
