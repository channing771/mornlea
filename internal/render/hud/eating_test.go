package hud

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

// 进度字段选型的说明（design D5 留给实现裁决）：`EatingOverlay.Progress` 采用
// 0..1 钳制后的填充比例而不是 `MiningOverlay` 的 ticks 二元组，因为进食进度是
// `client.EatingProgressTracker` 按帧间时长累积出的连续值，权威侧没有对应 wire
// 字段（design D1）；再量化回 ticks 只是丢精度。本文件按此形态断言精确几何。

// TestAppendEatingBarReusesMiningAnchorAndTrackGeometry 钉死进食条与采掘条
// 同锚点、同轨道尺寸：复用 `appendMiningBar` 的全部几何常量（`miningBarWidth`/
// `miningBarHeight`/`miningBarGap` 与 `statusBarBounds` 上方的锚定公式），
// 不新增任何几何常量，也不改变 `closedHUDHeight` 已预留的轨道高度。
func TestAppendEatingBarReusesMiningAnchorAndTrackGeometry(t *testing.T) {
	const width, height = float32(1280), float32(800)
	var mining, eating hotbarLayout
	appendMiningBar(&mining, MiningOverlay{
		Active: true, ProgressTicks: 6, RequiredTicks: 15, Harvestable: true,
	}, width, height)
	appendEatingBar(&eating, EatingOverlay{Active: true, Progress: 0.25},
		MiningOverlay{}, width, height)

	if len(mining.quads) != 3 || len(eating.quads) != eatingBarQuads {
		t.Fatalf("mining/eating quads=%d/%d，想要 3/%d（进食只有轨道与填充，无帽无缺口）",
			len(mining.quads), len(eating.quads), eatingBarQuads)
	}
	miningTrack := mining.quads[0]
	track, fill := eating.quads[0], eating.quads[1]
	// 轨道与 `appendMiningBar` 完全重合（含颜色）：这就是「同一锚点、同一呈现形状」。
	if track != miningTrack || track.Color != miningTrackColor {
		t.Fatalf("进食轨道=%+v，想要与采掘轨道重合 %+v（颜色 %v）", track, miningTrack, miningTrackColor)
	}
	// 锚定完整两行状态栈上方：与既有采掘锚点测试同一公式。
	_, hotbarY, _, scale := hotbarRowBounds(false, width, height)
	rowStep := (healthHeartSize + statusBarGap) * scale
	wantY := hotbarY - 2*rowStep - (miningBarGap+miningBarHeight)*scale
	if track.Y != wantY || track.Width != miningBarWidth*scale || track.Height != miningBarHeight*scale {
		t.Fatalf("进食轨道几何=%+v，想要 Y=%v 的 %v×%v", track, wantY, miningBarWidth*scale, miningBarHeight*scale)
	}
	// 填充与轨道同高同源，宽度按比例精确推进：0.25×240=60。
	if fill.X != track.X || fill.Y != track.Y || fill.Height != track.Height {
		t.Fatalf("进食填充未与轨道同锚: fill=%+v track=%+v", fill, track)
	}
	if fill.Width != miningBarWidth*scale*0.25 || fill.Color != eatingFillColor {
		t.Fatalf("进食填充宽度/颜色=%v/%v，想要 %v 与固定进食填充色",
			fill.Width, fill.Color, miningBarWidth*scale*0.25)
	}
}

// TestAppendEatingBarMutuallyExclusiveWithMining 见证互斥方向：采掘激活时
// 采掘优先，进食布局函数一个实例都不追加——判定在 `appendEatingBar` 内部，
// 不依赖调用方先做取舍。
func TestAppendEatingBarMutuallyExclusiveWithMining(t *testing.T) {
	mining := MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}
	var layout hotbarLayout
	appendEatingBar(&layout, EatingOverlay{Active: true, Progress: 0.9}, mining, 1280, 800)
	if len(layout.quads) != 0 {
		t.Fatalf("采掘激活时进食 quad=%d，想要 0（采掘优先、互斥）", len(layout.quads))
	}

	// 采掘未激活时进食条正常出现；互斥只看 app 镜像里同一个 `MiningOverlay.Active`
	// 标志——保守取向：退化权威态（active 而 required=0，采掘条本身不画）也让位，
	// 绝不让两条进度条在任何组合下同时出现。
	for _, test := range []struct {
		name   string
		mining MiningOverlay
		want   int
	}{
		{"采掘未激活", MiningOverlay{}, eatingBarQuads},
		{"采掘 required=0 退化态", MiningOverlay{Active: true, ProgressTicks: 6}, 0},
	} {
		var exclusive hotbarLayout
		appendEatingBar(&exclusive, EatingOverlay{Active: true, Progress: 0.5}, test.mining, 1280, 800)
		if len(exclusive.quads) != test.want {
			t.Fatalf("%s 进食 quad=%d，想要 %d", test.name, len(exclusive.quads), test.want)
		}
	}
}

// TestAppendEatingBarClampsProgressAndSkipsInactive 覆盖非激活零实例、零进度
// 只画轨道与超额进度钳制三条边界。
func TestAppendEatingBarClampsProgressAndSkipsInactive(t *testing.T) {
	var inactive hotbarLayout
	appendEatingBar(&inactive, EatingOverlay{Progress: 0.7}, MiningOverlay{}, 1280, 800)
	if len(inactive.quads) != 0 {
		t.Fatalf("非激活 quad=%d，想要 0", len(inactive.quads))
	}

	var zero hotbarLayout
	appendEatingBar(&zero, EatingOverlay{Active: true}, MiningOverlay{}, 1280, 800)
	if len(zero.quads) != 1 {
		t.Fatalf("零进度 quad=%d，想要只有轨道 1 个", len(zero.quads))
	}

	var clamped hotbarLayout
	appendEatingBar(&clamped, EatingOverlay{Active: true, Progress: 1.5}, MiningOverlay{}, 1280, 800)
	if len(clamped.quads) != eatingBarQuads {
		t.Fatalf("超额进度 quad=%d，想要 %d", len(clamped.quads), eatingBarQuads)
	}
	if fill := clamped.quads[1]; fill.Width != clamped.quads[0].Width {
		t.Fatalf("超额进度填充宽=%v，想要钳制到轨道宽 %v", fill.Width, clamped.quads[0].Width)
	}
}

// TestHotbarPrepareEatingFrameQuadsNotExceedMiningFrame 是容量红线：同一关闭态
// 布局里进食激活帧的 quad 数必须不超过采掘激活帧（进食 2 ≤ 采掘轨道+填充+三缺口
// 5），固定上传容量 `maxHotbarQuads` 不因此增加。
func TestHotbarPrepareEatingFrameQuadsNotExceedMiningFrame(t *testing.T) {
	renderer := newTestHotbarRenderer()
	budget := render.NewUploadBudget(1024)
	miningFrame := MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}
	if err := renderer.Prepare(core.Inventory{}, true, false, -1, nil, nil, nil,
		miningFrame, EatingOverlay{}, HealthOverlay{}, OxygenOverlay{}, HungerOverlay{},
		ChatOverlay{}, false, 1280, 800, budget); err != nil {
		t.Fatalf("采掘激活帧 Prepare: %v", err)
	}
	miningQuads := len(renderer.layout.quads)
	if err := renderer.Prepare(core.Inventory{}, true, false, -1, nil, nil, nil,
		MiningOverlay{}, EatingOverlay{Active: true, Progress: 1},
		HealthOverlay{}, OxygenOverlay{}, HungerOverlay{},
		ChatOverlay{}, false, 1280, 800, budget); err != nil {
		t.Fatalf("进食激活帧 Prepare: %v", err)
	}
	eatingQuads := len(renderer.layout.quads)
	if eatingQuads > miningQuads {
		t.Fatalf("进食激活帧 quad=%d 超过采掘激活帧 %d，容量红线失守", eatingQuads, miningQuads)
	}
	if want := miningQuads - (miningBarQuads + miningWarningNotches) + eatingBarQuads; eatingQuads != want {
		t.Fatalf("进食激活帧 quad=%d，想要精确值 %d（基线 + %d）", eatingQuads, want, eatingBarQuads)
	}
	if eatingQuads > maxHotbarQuads {
		t.Fatalf("进食激活帧 quad=%d 超过固定上传容量 %d", eatingQuads, maxHotbarQuads)
	}
}

// TestLayoutInventoryDrawsEatingBarOnlyWhenClosed 见证 `layoutInventory` 的接线：
// 关闭态在采掘条之后追加进食条；打开容器时进食条与采掘条一样不出现（开箱本身
// 已使进食输入位归零，这里钉死布局侧不再多画一份）。
func TestLayoutInventoryDrawsEatingBarOnlyWhenClosed(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	eating := EatingOverlay{Active: true, Progress: 0.5}
	var layout hotbarLayout
	closed := layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{}, eating, 1280, 800)
	// 关闭态基线 = 双层面板 + 双层选中框 + 九格（与 `appendEatingBar` 无关的
	// 既有常量显式列出，裸数字会让后续样式微调时悄悄失真）。
	closedBase := closedHotbarPanelQuads + closedHotbarSelectionQuads + core.HotbarSlots
	if got := len(closed.quads); got != closedBase+eatingBarQuads {
		t.Fatalf("关闭态进食 quads=%d，想要基线 %d + %d", got, closedBase, eatingBarQuads)
	}
	var opened hotbarLayout
	open := layoutInventory(&opened, atlas, core.Inventory{}, true, -1, nil, nil, nil,
		MiningOverlay{}, eating, 1280, 800)
	base := layoutInventory(&opened, atlas, core.Inventory{}, true, -1, nil, nil, nil,
		MiningOverlay{}, EatingOverlay{}, 1280, 800)
	if len(open.quads) != len(base.quads) {
		t.Fatalf("打开态仍绘制进食条: %d vs 基线 %d", len(open.quads), len(base.quads))
	}
}
