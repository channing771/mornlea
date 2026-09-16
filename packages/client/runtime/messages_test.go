package runtime

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestMessagesDrainBudgetAndMirrorTranscript(t *testing.T) {
	furnace := core.FurnaceRef{Dimension: core.Overworld, Generation: 1}
	chest := core.ContainerRef{
		Dimension: core.Overworld, Kind: core.ContainerKindChest, Generation: 1,
	}
	receiver := &messageTestReceiver{messages: []network.ServerMessage{
		network.InventoryState{},
		network.FurnaceState{Furnace: furnace},
		network.ChestState{Chest: chest},
		network.CraftingState{Size: 3},
	}}
	runtime := newLoggedInRuntime(receiver, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })

	outcomes, err := runtime.DrainMessages(nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 2 || receiver.remaining() != 2 {
		t.Fatalf("first drain outcomes/remaining = %d/%d, want 2/2", len(outcomes), receiver.remaining())
	}
	if !outcomes[0].Changes.Has(MirrorChangeInventory) || outcomes[0].Container != ContainerTransitionNone {
		t.Fatalf("inventory outcome = %+v", outcomes[0])
	}
	if !outcomes[1].Changes.Has(MirrorChangeFurnace) || outcomes[1].Container != ContainerTransitionOpenFurnace {
		t.Fatalf("furnace outcome = %+v", outcomes[1])
	}

	outcomes, err = runtime.DrainMessages(outcomes[:0], 8)
	if err != nil {
		t.Fatal(err)
	}
	wantTransitions := []ContainerTransition{ContainerTransitionOpenChest, ContainerTransitionOpenCrafting}
	gotTransitions := []ContainerTransition{outcomes[0].Container, outcomes[1].Container}
	if !reflect.DeepEqual(gotTransitions, wantTransitions) {
		t.Fatalf("container transitions = %v, want %v", gotTransitions, wantTransitions)
	}
	state := runtime.MirrorState()
	if state.FurnaceOpen {
		t.Fatal("workbench open left the furnace mirror open")
	}
	if state.ChestOpen {
		t.Fatal("workbench open left the chest mirror open")
	}
	if !state.InventoryConfirmed {
		t.Fatal("inventory message did not update the composed mirror")
	}
}

func TestMessagesRoutePlayerStateIntoPrediction(t *testing.T) {
	runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
	message := network.PlayerState{ServerTick: 7, Dimension: core.Overworld}
	outcome, err := runtime.ApplyMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Handled || !outcome.PredictionChanged || outcome.Changes != 0 || outcome.Container != ContainerTransitionNone {
		t.Fatalf("PlayerState outcome = %+v, want handled prediction change", outcome)
	}
	if got, ok := outcome.Message.(network.PlayerState); !ok || got.ServerTick != message.ServerTick {
		t.Fatalf("preserved message = %#v, want %#v", outcome.Message, message)
	}
	if outcome.Prediction.Ready {
		t.Fatal("not-ready PlayerState published a ready prediction")
	}
}

func TestMessagesRejectInvalidMirrorStateAtomically(t *testing.T) {
	runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
	valid := network.InventoryState{}
	if _, err := runtime.ApplyMessage(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Inventory.Hotbar.Selected = core.HotbarSlots
	outcome, err := runtime.ApplyMessage(invalid)
	if err == nil {
		t.Fatal("invalid inventory state was accepted")
	}
	if outcome.Changes != 0 {
		t.Fatalf("failed outcome changes = %v, want zero", outcome.Changes)
	}
	state := runtime.MirrorState()
	if !state.InventoryConfirmed || state.Inventory != valid.Inventory {
		t.Fatalf("inventory changed after rejected state: %+v", state.Inventory)
	}
}

func TestMessagesDrainDoesNotPublishRejectedOutcome(t *testing.T) {
	invalid := network.InventoryState{}
	invalid.Inventory.Hotbar.Selected = core.HotbarSlots
	receiver := &messageTestReceiver{messages: []network.ServerMessage{invalid}}
	runtime := newLoggedInRuntime(receiver, nil, 0)

	outcomes, err := runtime.DrainMessages(nil, 1)
	if err == nil {
		t.Fatal("DrainMessages accepted an invalid mirror state")
	}
	if len(outcomes) != 0 {
		t.Fatalf("DrainMessages published %d rejected outcomes, want zero", len(outcomes))
	}
}

func TestMessagesWorldOutcomeDistinguishesAppliedBlockChanges(t *testing.T) {
	runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = network.SectionData{Y: int32(index), Storage: network.SectionSingle, Single: core.AirID}
	}
	if _, err := runtime.ApplyMessage(network.ChunkSnapshot{
		Dimension: core.Overworld, Revision: 3, Sections: sections,
	}); err != nil {
		t.Fatal(err)
	}
	changes := network.BlockChanges{
		Dimension: core.Overworld, BaseRevision: 3, NewRevision: 4,
		Changes: []network.BlockChange{{Position: core.BlockPos{Y: core.MinY}, Block: core.StoneID}},
	}
	outcome, err := runtime.ApplyMessage(changes)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Changes.Has(MirrorChangeWorld | MirrorChangeBlockChangesApplied) {
		t.Fatalf("applied block changes outcome = %+v", outcome)
	}
	stale, err := runtime.ApplyMessage(changes)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Changes.Has(MirrorChangeBlockChangesApplied) {
		t.Fatalf("stale block changes reported as applied: %+v", stale)
	}
}

func TestMessagesContainerCloseOnlyTransitionsMatchingView(t *testing.T) {
	runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
	chest := core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKindChest, Generation: 1}
	other := core.FurnaceRef{Dimension: core.Overworld, Generation: 1}
	if _, err := runtime.ApplyMessage(network.ChestState{Chest: chest}); err != nil {
		t.Fatal(err)
	}
	mismatch, err := runtime.ApplyMessage(network.ContainerClosed{Container: other})
	if err != nil {
		t.Fatal(err)
	}
	if mismatch.Changes != 0 || mismatch.Container != ContainerTransitionNone || !runtime.MirrorState().ChestOpen {
		t.Fatalf("mismatched close changed the active chest: outcome=%+v state=%+v", mismatch, runtime.MirrorState())
	}
	matched, err := runtime.ApplyMessage(network.ContainerClosed{Container: chest})
	if err != nil {
		t.Fatal(err)
	}
	if matched.Changes != MirrorChangeChest || matched.Container != ContainerTransitionClose || runtime.MirrorState().ChestOpen {
		t.Fatalf("matching close outcome/state = %+v/%+v", matched, runtime.MirrorState())
	}
}

func TestMirrorStateReturnsDetachedSlices(t *testing.T) {
	runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
	playerID := testRemoteIdentity(t).PlayerID
	message := network.ChatEvent{
		EventID: 1, PlayerID: playerID, PlayerName: "Pilot",
		Kind: network.ChatEventRejected, RejectReason: network.ChatRejectInvalidFormat,
	}
	if _, err := runtime.ApplyMessage(message); err != nil {
		t.Fatal(err)
	}
	state := runtime.MirrorState()
	state.Chat[0].PlayerName = "mutated"
	got := runtime.MirrorState()
	if got.Chat[0].PlayerName != "Pilot" {
		t.Fatalf("retained snapshot mutation reached runtime mirror: %+v", got.Chat[0])
	}
}

func TestMirrorResetOnCloseCannotBeRepopulated(t *testing.T) {
	runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
	if _, err := runtime.ApplyMessage(network.InventoryState{}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if runtime.MirrorState().InventoryConfirmed {
		t.Fatal("Close retained the inventory mirror")
	}
	if _, err := runtime.ApplyMessage(network.InventoryState{}); err == nil {
		t.Fatal("closed runtime accepted a message into its reset mirrors")
	}
}

func TestMirrorResetClearsEverySessionOwnedMirror(t *testing.T) {
	playerID := testRemoteIdentity(t).PlayerID
	companionID, err := companion.ParseID("10112233-4455-4677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	chunk := core.ChunkPos{X: 2, Z: -3}
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = network.SectionData{Y: int32(index), Storage: network.SectionSingle, Single: core.AirID}
	}
	furnace := core.FurnaceRef{Dimension: core.Overworld, Generation: 1}
	chest := core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKindChest, Generation: 1}
	dropID := core.DropID{Dimension: core.Overworld, Generation: 1}
	chat := network.ChatEvent{
		EventID: 1, PlayerID: playerID, PlayerName: "Pilot",
		Kind: network.ChatEventRejected, RejectReason: network.ChatRejectInvalidFormat,
	}

	tests := []struct {
		name    string
		message network.ServerMessage
		changes MirrorChanges
		present func(*Runtime) bool
	}{
		{"world", network.ChunkSnapshot{Dimension: core.Overworld, Chunk: chunk, Revision: 3, Sections: sections}, MirrorChangeWorld, func(runtime *Runtime) bool {
			_, loaded := runtime.WorldChunkState(core.Overworld, chunk)
			return loaded
		}},
		{"inventory", network.InventoryState{}, MirrorChangeInventory, func(runtime *Runtime) bool { return runtime.MirrorState().InventoryConfirmed }},
		{"crafting", network.CraftingState{Size: 2}, MirrorChangeCrafting, func(runtime *Runtime) bool { return runtime.MirrorState().CraftingConfirmed }},
		{"chest", network.ChestState{Chest: chest}, MirrorChangeChest, func(runtime *Runtime) bool { return runtime.MirrorState().ChestOpen }},
		{"furnace", network.FurnaceState{Furnace: furnace}, MirrorChangeFurnace, func(runtime *Runtime) bool { return runtime.MirrorState().FurnaceOpen }},
		{"chat", chat, MirrorChangeChat, func(runtime *Runtime) bool { return len(runtime.MirrorState().Chat) == 1 }},
		{"item drops", network.ItemDropUpserts{ServerTick: 1, Drops: []network.ItemDrop{{ID: dropID, Item: core.ItemStone, Count: 1}}}, MirrorChangeItemDrops, func(runtime *Runtime) bool {
			return len(runtime.MirrorState().ItemDrops) == 1
		}},
		{"remote players", network.RemotePlayerSpawn{PlayerID: playerID, DisplayName: "Pilot", ServerTick: 1, Dimension: core.Overworld}, MirrorChangeRemotePlayers, func(runtime *Runtime) bool {
			return len(runtime.MirrorState().RemotePlayers) == 1
		}},
		{"companions", network.CompanionSpawn{ID: companionID, Name: "Buddy", Tick: 1, Dimension: core.Overworld}, MirrorChangeCompanions, func(runtime *Runtime) bool {
			return len(runtime.MirrorState().Companions) == 1
		}},
		{"hostiles", network.HostileSpawn{ServerTick: 1, Spawns: []network.HostileSpawnRecord{{ID: 1, Dimension: core.Overworld, Health: 1, Kind: network.HostileKindNightwalker}}}, MirrorChangeHostiles, func(runtime *Runtime) bool {
			return len(runtime.MirrorState().Hostiles) == 1
		}},
		{"passives", network.PassiveSpawn{ServerTick: 1, Spawns: []network.PassiveSpawnRecord{{ID: 1, Dimension: core.Overworld, Health: 1}}}, MirrorChangePassives, func(runtime *Runtime) bool {
			return len(runtime.MirrorState().Passives) == 1
		}},
		{"projectiles", network.ProjectileSpawn{ServerTick: 1, Spawns: []network.ProjectileSpawnRecord{{ID: 1, Kind: network.ProjectileKindShard, Dimension: core.Overworld}}}, MirrorChangeProjectiles, func(runtime *Runtime) bool {
			return len(runtime.MirrorState().Projectiles) == 1
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
			outcome, err := runtime.ApplyMessage(test.message)
			if err != nil {
				t.Fatalf("ApplyMessage(%T): %v", test.message, err)
			}
			if outcome.Changes != test.changes {
				t.Fatalf("message changes = %v, want %v", outcome.Changes, test.changes)
			}
			if !test.present(runtime) {
				t.Fatal("seed message did not populate its mirror")
			}
			runtime.ResetMirrors()
			if test.present(runtime) {
				t.Fatal("mirror survived reset")
			}
		})
	}
}

type messageTestReceiver struct {
	messages []network.ServerMessage
}

func (receiver *messageTestReceiver) TryRecv() (network.ServerMessage, bool) {
	if len(receiver.messages) == 0 {
		return nil, false
	}
	message := receiver.messages[0]
	receiver.messages = receiver.messages[1:]
	return message, true
}

func (receiver *messageTestReceiver) Err() error   { return nil }
func (receiver *messageTestReceiver) Close() error { return nil }
func (receiver *messageTestReceiver) remaining() int {
	return len(receiver.messages)
}
