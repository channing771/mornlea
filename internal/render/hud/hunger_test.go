package hud

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

// 杀死变异：忽略 Confirmed 标记或画出预测值，会让 HUD 在收到权威状态前显示猜测值。
// 饥饿值与生命值、氧气同属权威镜像，客户端一侧没有任何推算。
func TestAppendHungerBarDrawsOnlyConfirmedValues(t *testing.T) {
	var unconfirmed hotbarLayout
	appendHungerBar(&unconfirmed, HungerOverlay{Confirmed: false, Value: 12}, false, 1280, 720)
	if len(unconfirmed.quads) != 0 || len(unconfirmed.glyphs) != 0 {
		t.Fatalf("未确认饥饿值 quads=%d glyphs=%d，想要都为 0",
			len(unconfirmed.quads), len(unconfirmed.glyphs))
	}
	var degenerate hotbarLayout
	appendHungerBar(&degenerate, HungerOverlay{Confirmed: true, Value: 12}, false, 0, 720)
	if len(degenerate.quads) != 0 {
		t.Fatalf("零宽 framebuffer quads=%d，想要 0", len(degenerate.quads))
	}
}

// TestHungerBarUsesConfirmedClampedTwoPointSegments 逐 quad 断言鸡腿的**填充列**、
// 上限钳制与半格几何。
//
// 断言的是「第几个 quad 采哪一列纹理、宽多少、画在哪」，不是「quad 非空」或
// 「两个值的 quad 数不同」：一个把填充比例写死（例如恒画 10 满鸡腿）的实现会让
// 后两者全绿。半格粒度也必须逐个核对——奇数饥饿值必须给出恰好一个半宽 quad，
// 且因为饥饿条是**右下镜像**，半格露出的是鸡腿的右半边（U0 取中点、X 右移半格）。
func TestHungerBarUsesConfirmedClampedTwoPointSegments(t *testing.T) {
	emptyUV := hotbarTextureUV(hotbarEmptyDrumstickColumn)
	fullUV := hotbarTextureUV(hotbarFullDrumstickColumn)
	halfU0 := (fullUV[0] + fullUV[2]) * 0.5
	for _, test := range []struct {
		name   string
		value  uint8
		full   int
		half   bool
		wantUV [][4]float32
	}{
		{"饥饿全空", 0, 0, false, nil},
		{"一点饥饿给出右半格", 1, 0, true, nil},
		{"两点饥饿给出一满格", 2, 1, false, nil},
		{"九点饥饿给出四满一半", 9, 4, true, nil},
		{"十九点饥饿给出九满一半", 19, 9, true, nil},
		{"饥饿满仍然显示十满", core.MaxHunger, 10, false, nil},
		{"越界值钳制到满饥饿", 255, 10, false, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: test.value}, false, 1280, 720)
			wantQuads := healthSegmentCount + test.full
			if test.half {
				wantQuads++
			}
			if len(layout.quads) != wantQuads || len(layout.glyphs) != 0 {
				t.Fatalf("quads/glyphs=%d/%d，想要 %d/0",
					len(layout.quads), len(layout.glyphs), wantQuads)
			}
			// 前 10 个是常驻的空鸡腿底：饥饿满时也必须仍然画出（与氧气条
			// 「未满才出现」相反，饥饿是常态资源，条本身永远在）。
			for index, quad := range layout.quads[:healthSegmentCount] {
				got := [4]float32{quad.U0, quad.V0, quad.U1, quad.V1}
				if got != emptyUV {
					t.Fatalf("空鸡腿 %d UV=%v，想要空列 %v", index, got, emptyUV)
				}
				if quad.Width != healthHeartSize || quad.Height != healthHeartSize {
					t.Fatalf("空鸡腿 %d 尺寸=%v×%v", index, quad.Width, quad.Height)
				}
			}
			filled := layout.quads[healthSegmentCount:]
			for index, quad := range filled {
				got := [4]float32{quad.U0, quad.V0, quad.U1, quad.V1}
				wantHalf := test.half && index == len(filled)-1
				want := [4]float32{fullUV[0], fullUV[1], fullUV[2], fullUV[3]}
				wantWidth := healthHeartSize
				if wantHalf {
					want[0] = halfU0
					wantWidth = healthHeartSize / 2
				}
				if got != want {
					t.Fatalf("填充鸡腿 %d UV=%v，想要 %v（半格=%v）", index, got, want, wantHalf)
				}
				if quad.Width != wantWidth {
					t.Fatalf("填充鸡腿 %d 宽=%v，想要 %v", index, quad.Width, wantWidth)
				}
				// 半格露出右半边：X 相对同序号的空鸡腿右移半格，其余与之重合。
				base := layout.quads[index]
				wantX := base.X
				if wantHalf {
					wantX += healthHeartSize / 2
				}
				if quad.X != wantX || quad.Y != base.Y {
					t.Fatalf("填充鸡腿 %d 位置=(%v,%v)，想要 (%v,%v)",
						index, quad.X, quad.Y, wantX, base.Y)
				}
			}
		})
	}
}

// TestHungerBarDistinguishesValues 把「两个不同的权威饥饿值必须给出不同的填充」
// 钉成 quad 序列的直接比较：只比 quad 数量的话，13 与 14 都是 17 个 quad，
// 一个把最后一格恒画整格的实现照样全绿。
func TestHungerBarDistinguishesValues(t *testing.T) {
	layoutFor := func(value uint8) []hotbarInstance {
		var layout hotbarLayout
		appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: value}, false, 1280, 720)
		return layout.quads
	}
	full, mid, odd := layoutFor(core.MaxHunger), layoutFor(12), layoutFor(13)
	if reflect.DeepEqual(full, mid) {
		t.Fatal("饥饿 20 与 12 画出了相同的鸡腿序列")
	}
	if reflect.DeepEqual(mid, odd) {
		t.Fatal("饥饿 12 与 13 画出了相同的鸡腿序列：半格粒度丢失")
	}
	if len(full) != healthSegmentCount+10 || len(mid) != healthSegmentCount+6 ||
		len(odd) != healthSegmentCount+7 {
		t.Fatalf("quad 数=%d/%d/%d，想要 20/16/17", len(full), len(mid), len(odd))
	}
}

// TestHungerBarMirrorsHealthBarOnTheRight 钉死鸡腿从右向左排列且使用心形同款
// 16×16 槽尺寸；快捷栏右边缘锚点与 open/closed 行位由状态栈几何测试单独覆盖。
func TestHungerBarMirrorsHealthBarOnTheRight(t *testing.T) {
	const width, height = 1280, 720
	var hunger hotbarLayout
	appendHungerBar(&hunger, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, false, width, height)
	if len(hunger.quads) != hungerQuads {
		t.Fatalf("饥饿 quad 数=%d，想要 %d", len(hunger.quads), hungerQuads)
	}
	for index := range healthSegmentCount {
		quad := hunger.quads[index]
		if quad.Width != healthHeartSize || quad.Height != healthHeartSize {
			t.Fatalf("第 %d 格尺寸=%v×%v，想要 %v×%v",
				index, quad.Width, quad.Height, healthHeartSize, healthHeartSize)
		}
		if index > 0 && quad.X >= hunger.quads[index-1].X {
			t.Fatalf("第 %d 格 X=%v，没有位于前一格 X=%v 的左侧",
				index, quad.X, hunger.quads[index-1].X)
		}
	}
}

// TestHotbarPrepareDrawsHungerBar 钉死 `Prepare` 确实调用了 `appendHungerBar`。
//
// 没有这条断言，把那一行从 `Prepare` 里删掉不会让任何测试变红：quad 容量、
// offset 与总容量都是编译期常量，`appendHungerBar` 自己的用例又直接调它。
// 界面上饥饿条整条消失，门禁却全绿——这正是「链路接线无人断言」的经典形态。
func TestHotbarPrepareDrawsHungerBar(t *testing.T) {
	renderer := &HotbarRenderer{
		atlas: &allocationGlyphSource{},
		layout: hotbarLayout{
			quads:  make([]hotbarInstance, 0, maxHotbarQuads),
			glyphs: make([]hotbarInstance, 0, maxHotbarGlyphs),
		},
		upload: make([]byte, hotbarUploadBytes),
	}
	budget := render.NewUploadBudget(1024)
	countDrumsticks := func(hunger HungerOverlay) (empty, full int) {
		t.Helper()
		if err := renderer.Prepare(
			core.Inventory{}, false, false, -1, nil, nil, MiningOverlay{},
			HealthOverlay{}, OxygenOverlay{}, hunger, ChatOverlay{}, 1280, 720, budget,
		); err != nil {
			t.Fatal(err)
		}
		emptyUV := hotbarTextureUV(hotbarEmptyDrumstickColumn)
		fullUV := hotbarTextureUV(hotbarFullDrumstickColumn)
		for _, quad := range renderer.layout.quads {
			switch {
			case quad.U0 == emptyUV[0] && quad.U1 == emptyUV[2]:
				empty++
			case quad.U1 == fullUV[2] && quad.U0 >= fullUV[0] && quad.U0 < fullUV[2]:
				full++
			}
		}
		return empty, full
	}
	if empty, full := countDrumsticks(HungerOverlay{Confirmed: true, Value: 13}); empty != 10 || full != 7 {
		t.Fatalf("Prepare 后空/填充鸡腿=%d/%d，想要 10/7", empty, full)
	}
	if empty, full := countDrumsticks(HungerOverlay{Confirmed: false, Value: 13}); empty != 0 || full != 0 {
		t.Fatalf("未确认饥饿值仍画出 %d/%d 个鸡腿", empty, full)
	}
}
