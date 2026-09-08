package capture

import (
	"bytes"
	"testing"

	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestHeldItemsCatalogueCoversRegisteredItemsAndValidStacks(t *testing.T) {
	seen := map[core.ItemID]bool{}
	for _, stack := range heldItemsCatalogue() {
		if seen[stack.Item] || !stack.Valid() {
			t.Fatalf("invalid or duplicate stack: %+v", stack)
		}
		seen[stack.Item] = true
	}
	for id := core.ItemNone; id < core.ItemIDMax; id++ {
		if (id == core.ItemNone || core.RegisteredItem(id)) && !seen[id] {
			t.Fatalf("missing item %d", id)
		}
	}
}

func TestHeldItemsSequenceProducesNeutralMiningAttackAndRecovery(t *testing.T) {
	for _, id := range []core.ItemID{core.ItemNone, core.ItemStone, core.ItemIronPickaxe, core.ItemIronSword, core.ItemBrokenIronHoe} {
		app := &heldItemsAttackObserver{SceneApplication: application.NewPresentationApplicationForTest()}
		stack := core.ItemStack{Item: id, Count: 1}
		if id == core.ItemNone {
			stack = core.ItemStack{}
		}
		stack.Durability, _ = core.ItemMaxDurability(id)
		if err := confirmHandCaptureBackpack(app, stack); err != nil {
			t.Fatal(err)
		}
		var encoder render.ViewmodelEncoder
		var neutral []byte
		miningChanged, attackChanged := false, false
		for frame := 0; frame < heldItemsFrameCount(stack); frame++ {
			if err := applyHeldItemsFrame(app, stack, frame); err != nil {
				t.Fatal(err)
			}
			if app.ServerTick() != uint64(frame+1) {
				t.Fatalf("tick drift at %d", frame)
			}
			input := render.ViewmodelInput{Selected: stack, Tick: app.ServerTick(), Mining: app.MiningOverlay().Active, AttackTick: app.attackTick}
			_, period := render.ViewmodelSwingParams(render.ViewmodelTierOf(stack))
			attackFrame := 8 + int(period)
			if frame < attackFrame && app.attackTick != 0 {
				t.Fatal("attack edge before neutral gap")
			}
			if frame >= attackFrame && (app.attackTick != uint64(attackFrame+1) || app.attacks != 1) {
				t.Fatal("missing or repeated attack confirmation")
			}
			data := encoder.EncodeViewmodelInstances(nil, &input)
			if frame == 0 {
				neutral = append([]byte(nil), data...)
			}
			if frame >= 4 && frame < 4+int(period) && !bytes.Equal(neutral, data) {
				miningChanged = true
			}
			if frame >= attackFrame && !bytes.Equal(neutral, data) {
				attackChanged = true
			}
			if (frame < 4 || frame == heldItemsFrameCount(stack)-1) && !bytes.Equal(neutral, data) {
				t.Fatalf("item %d frame %d not neutral", id, frame)
			}
		}
		if !miningChanged || !attackChanged {
			t.Fatalf("item %d missing full actions: mining=%v attack=%v", id, miningChanged, attackChanged)
		}
	}
}

func TestHeldItemsDispatchRejectsMissingOutput(t *testing.T) {
	err := RunMotion(nil, "", "held-items")
	if err == nil || err.Error() == `未知 motion 场景 "held-items"` {
		t.Fatalf("held-items dispatch: %v", err)
	}
}

// heldItemsAttackObserver 记录真实应用消费的确认沿，让漏接或重复触发直接暴露在编码时序中。
type heldItemsAttackObserver struct {
	SceneApplication
	attackTick uint64
	attacks    int
}

func (a *heldItemsAttackObserver) ObserveCombatHitForCapture(tick uint64) {
	a.attackTick = tick
	a.attacks++
	a.SceneApplication.ObserveCombatHitForCapture(tick)
}

func TestHeldItemsSequenceRejectsOutOfRangeBeforeMutatingApplication(t *testing.T) {
	app := application.NewPresentationApplicationForTest()
	app.SetServerTick(99)
	for _, frame := range []int{-1, heldItemsFrameCount(core.ItemStack{})} {
		if err := applyHeldItemsFrame(app, core.ItemStack{}, frame); err == nil || app.ServerTick() != 99 {
			t.Fatalf("invalid frame mutated state: %d", frame)
		}
	}
	for _, stack := range heldItemsCatalogue() {
		if heldItemsFrameCount(stack) > motionMaxFrames {
			t.Fatalf("item %d exceeds recorder budget", stack.Item)
		}
	}
}
