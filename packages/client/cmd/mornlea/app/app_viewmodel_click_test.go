//go:build darwin

package app

import (
	"bytes"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
	"testing"
	"time"
)

func TestViewmodelEffectiveAirClickImmediateAndReleaseCompletes(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemNone, core.ItemIronSword, core.ItemStone} {
		a, _ := newInteractiveTestApplication(t)
		stack := core.ItemStack{Item: item, Count: 1}
		stack.Durability, _ = core.ItemMaxDurability(item)
		if item == core.ItemNone {
			stack = core.ItemStack{}
		}
		applyViewmodelHotbar(t, a, stack, 0)
		encode := func() []byte {
			return a.viewmodelEncoder.EncodeViewmodelInstances(nil, a.deriveViewmodelInput(false, render.BlockCrack{}))
		}
		neutral := encode()
		a.applyInteractiveCursorInput(0, client.Movement{}, client.Actions{PrimaryDown: true, Mining: true}, true, false)
		if active, phase := a.viewmodelMotion.Phase(); !active || phase != 0 || !bytes.Equal(neutral, encode()) {
			t.Fatalf("item %d: click must activate at continuous neutral endpoint", item)
		}
		a.applyInteractiveCursorInput(time.Millisecond, client.Movement{}, client.Actions{}, true, false)
		if bytes.Equal(neutral, encode()) {
			t.Fatal("elapsed click did not move")
		}
		a.applyInteractiveCursorInput(100*time.Millisecond, client.Movement{}, client.Actions{}, true, false)
		stroke := encode()
		if bytes.Equal(neutral, stroke) {
			t.Fatal("release cut stroke short")
		}
		a.combatFeedback.Observe(100)
		if !bytes.Equal(stroke, encode()) {
			t.Fatal("late hit or extra render changed local motion")
		}
		a.applyInteractiveCursorInput(time.Second, client.Movement{}, client.Actions{}, true, false)
		if !bytes.Equal(neutral, encode()) {
			t.Fatal("stroke failed to recover")
		}
	}
}

func TestViewmodelClickSuppressionAndReset(t *testing.T) {
	for _, mode := range []string{"menu", "inventory", "chat", "recapture"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := newInteractiveTestApplication(t)
			applyViewmodelHotbar(t, a, core.ItemStack{}, 0)
			var input client.InputState
			update := func(down bool, elapsed time.Duration, suppressed bool) {
				actions := input.Update(down, false, 0, false, false, suppressed)
				a.applyInteractiveCursorInput(elapsed, client.Movement{}, actions, true, mode == "recapture" && suppressed)
			}
			switch mode {
			case "menu":
				a.menu.phase = MenuPhaseMenu
			case "inventory":
				a.inventoryOpen = true
			case "chat":
				a.chatInput.open = true
			}
			update(true, 0, true)
			if active, _ := a.viewmodelMotion.Phase(); active {
				t.Fatal("suppressed click started motion")
			}
			a.menu.phase = MenuPhaseGame
			a.inventoryOpen = false
			a.chatInput.open = false
			update(true, time.Second, false)
			if active, _ := a.viewmodelMotion.Phase(); active {
				t.Fatal("suppressed held click leaked on return")
			}
			update(false, 0, false)
			update(true, 0, false)
			if active, _ := a.viewmodelMotion.Phase(); !active {
				t.Fatal("new valid click did not start")
			}
			a.ResetViewmodel()
			if active, _ := a.viewmodelMotion.Phase(); active {
				t.Fatal("reset retained stroke")
			}
		})
	}
}

func TestViewmodelActualInputRatesUsePresentationElapsed(t *testing.T) {
	var expected []byte
	for _, hz := range []int{30, 60, 144} {
		a, _ := newInteractiveTestApplication(t)
		applyViewmodelHotbar(t, a, viewmodelStoneStack, 0)
		var input client.InputState
		update := func(elapsed time.Duration) {
			a.applyInteractiveCursorInput(elapsed, client.Movement{}, input.Update(true, false, 0, false, false, false), true, false)
		}
		update(0)
		total := 2137 * time.Millisecond
		for elapsed := time.Duration(0); elapsed < total; {
			step := min(time.Second/time.Duration(hz), total-elapsed)
			update(step)
			elapsed += step
		}
		data := a.viewmodelEncoder.EncodeViewmodelInstances(nil, a.deriveViewmodelInput(false, render.BlockCrack{}))
		if expected == nil {
			expected = data
		} else if !bytes.Equal(expected, data) {
			t.Fatalf("hz %d diverged", hz)
		}
		// 大间隔必须保留给呈现，不能继承预测的 100ms 截断。
		a.ResetViewmodel()
		update(0)
		update(total)
		data = a.viewmodelEncoder.EncodeViewmodelInstances(nil, a.deriveViewmodelInput(false, render.BlockCrack{}))
		if !bytes.Equal(expected, data) {
			t.Fatal("presentation elapsed clamped")
		}
	}
}
