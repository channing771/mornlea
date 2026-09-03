//go:build darwin

package app

// app_crack_overlay_test.go：世界空间采掘裂纹的呈现输入逐帧从最后确认的
// 权威采掘镜像（`hud.MiningOverlay.Target/HasTarget`）与选框同源派生。
// 端到端帧断言（含 Rust 侧 tag 10 段消费）由 crack pass 落地后的 capture
// 场景覆盖；本文件钉住 Go 侧派生门控与实例流编码的等价面。

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/render/hud"
)

// crackTestTarget 是 configureTargetFeedback 布置的相机准星方块：相机在
// (0.5, 3.5, 2.5) 朝 -Z 看，命中 {0,3,-3} 的砖块。
var crackTestTarget = core.BlockPos{X: 0, Y: 3, Z: -3}

// activeCrackOverlay 返回指向测试目标的权威采掘镜像：6/15 恰落在 4/10
// 阶段内（第 4 阶段），用于钉住阶段与层号派生。镜像的可采标志已随屏幕
// 采掘条退役，裂纹不再区分可采性。
func activeCrackOverlay() hud.MiningOverlay {
	return hud.MiningOverlay{
		Active: true, Target: crackTestTarget, HasTarget: true,
		ProgressTicks: 6, RequiredTicks: 15,
	}
}

// visibleCrackApplication 装配「选框可见 + 权威采掘 active」的派生前置，
// 返回经 `appendCurrentBlockTarget` 得到的本帧选框（生产 `RenderFrame` 的
// 同一组合步骤，不含 Rust 帧提交）。
func visibleCrackApplication(t *testing.T) (*Application, render.BlockOutline) {
	t.Helper()
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	configureTargetFeedback(t, app)
	app.miningOverlay = activeCrackOverlay()
	_, outline := app.appendCurrentBlockTarget(nil)
	if !outline.Visible {
		t.Fatal("测试前置没有产生可见选框")
	}
	return app, outline
}

// active + HasTarget + 选框可见：裂纹恰 1 实例、定位权威目标、阶段映射到
// LayerCrack0+4 的 atlas 层号；非 active 后同一管线输出空流（不残留）。
func TestApplicationDerivesCrackFromMiningOverlay(t *testing.T) {
	app, outline := visibleCrackApplication(t)
	crack := app.deriveBlockCrack(outline)
	if !crack.Visible || crack.Position != crackTestTarget {
		t.Fatalf("裂纹呈现=%+v，想要可见且定位 %v", crack, crackTestTarget)
	}
	if crack.Stage != 4 {
		t.Fatalf("裂纹阶段=%d，想要 4（6/15）", crack.Stage)
	}
	stream := app.entityEncoder.EncodeBlockCrackInstances(nil, crack)
	if len(stream) != 80 {
		t.Fatalf("裂纹流=%d 字节，想要 80", len(stream))
	}
	layer := math.Float32frombits(binary.LittleEndian.Uint32(stream[64:]))
	if layer != float32(int(assets.LayerCrack0)+4) {
		t.Fatalf("atlas 层号=%v，想要 %v", layer, float32(int(assets.LayerCrack0)+4))
	}

	// 非 active：下一帧裂纹整体消失且不残留位置与阶段。
	app.miningOverlay = hud.MiningOverlay{}
	if crack := app.deriveBlockCrack(outline); crack.Visible {
		t.Fatalf("非 active 后裂纹仍可见: %+v", crack)
	}
	if stream := app.entityEncoder.EncodeBlockCrackInstances(stream, render.BlockCrack{}); len(stream) != 0 {
		t.Fatalf("非 active 后裂纹流=%d 字节，想要 0", len(stream))
	}
}

// 隐藏门控与选框同源：UI 打开（背包/容器/面板）经 `appendCurrentBlockTarget`
// 使轮廓不可见，裂纹随之隐藏；reset 当帧与断线在 `RenderFrame` 里强制轮廓
// 为零值，同样落到「轮廓不可见即无裂纹」；暂停/菜单相位由游戏相位门控当帧
// 兜底隐藏（全景相位只出现在非游戏相位，故一并覆盖）。
func TestApplicationCrackGatesOnOutlineAndPhase(t *testing.T) {
	tests := []struct {
		name string
		// wantVisible 只对基准可见用例为真：其余用例全部必须隐藏裂纹。
		wantVisible bool
		// setup 在权威镜像与相机就位后运行，返回派生用的本帧选框。
		setup func(t *testing.T, app *Application) render.BlockOutline
	}{
		{"基准可见", true, func(t *testing.T, app *Application) render.BlockOutline {
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
		{"无权威目标", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.miningOverlay.HasTarget = false
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
		{"非 active", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.miningOverlay = hud.MiningOverlay{Target: crackTestTarget, HasTarget: true}
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
		{"required 为零", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.miningOverlay = hud.MiningOverlay{
				Active: true, Target: crackTestTarget, HasTarget: true,
			}
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
		{"背包打开", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.inventoryOpen = true
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
		{"熔炉打开", false, func(t *testing.T, app *Application) render.BlockOutline {
			if err := app.furnace.Apply(network.FurnaceState{Furnace: core.FurnaceRef{Generation: 1}}); err != nil {
				t.Fatal(err)
			}
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
		{"调试面板", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.panel = &panelState{visible: true}
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
		{"reset 当帧", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.blockTargetReset = true
			return render.BlockOutline{}
		}},
		{"断线", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.CloseClientSession(nil)
			return render.BlockOutline{}
		}},
		{"暂停相位", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.menu.phase = menuPhasePaused
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
		{"主菜单相位", false, func(t *testing.T, app *Application) render.BlockOutline {
			app.menu.phase = MenuPhaseMenu
			_, outline := app.appendCurrentBlockTarget(nil)
			return outline
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, _ := visibleCrackApplication(t)
			outline := test.setup(t, app)
			crack := app.deriveBlockCrack(outline)
			if crack.Visible != test.wantVisible {
				t.Fatalf("%s：裂纹可见性=%v，想要 %v（%+v）",
					test.name, crack.Visible, test.wantVisible, crack)
			}
		})
	}
}
