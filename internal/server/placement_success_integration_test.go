package server

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/storage"
)

type placementSuccessTranscript struct {
	Successes []uint64
	Rejected  network.CommandRejected
}

func TestPlaceBlockSucceededMemoryTCPParity(t *testing.T) {
	memory := runPlacementSuccessScript(t, "memory")
	tcp := runPlacementSuccessScript(t, "tcp")
	if !reflect.DeepEqual(memory, tcp) {
		t.Fatalf("放置成功应答 Memory/TCP 不一致\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
	if want := []uint64{1, 2, 3, 4}; !reflect.DeepEqual(memory.Successes, want) {
		t.Fatalf("放置成功序列=%v，想要 %v", memory.Successes, want)
	}
	if memory.Rejected != (network.CommandRejected{Sequence: 5, Reason: network.RejectInvalidBlock}) {
		t.Fatalf("放置拒绝=%+v", memory.Rejected)
	}
}

func runPlacementSuccessScript(t *testing.T, transport string) placementSuccessTranscript {
	t.Helper()
	identity := integrationIdentity(0xa6, "PlacementAck")
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 2, Seed: 42, SpawnDimension: core.Overworld,
	})
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 8}
	location := storage.PlayerLocation{
		Dimension: core.Overworld,
		Position:  [3]float32{0.5, 1.001, 0.5},
	}
	if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
		PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
		Current: location, Safe: &location, Inventory: inventory,
	})); err != nil {
		t.Fatal(err)
	}

	config := hostTestConfig()
	config.ViewRadius = 1
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, flatGenerator{}, store)
	endpoint, acceptDone, closeTransport := openParityTransport(t, host, transport, identity)
	t.Cleanup(func() {
		_ = endpoint.Close()
		closeTransport()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		select {
		case err := <-acceptDone:
			if err != nil && !errors.Is(err, network.ErrClosed) {
				t.Errorf("%s placement accept worker: %v", transport, err)
			}
		case <-ctx.Done():
			t.Errorf("%s placement accept worker 未退出: %v", transport, ctx.Err())
		}
		if err := host.Shutdown(ctx); err != nil {
			t.Errorf("%s placement Host.Shutdown: %v", transport, err)
		}
	})

	mirror := client.NewMirror()
	ready, inventoryReady := false, false
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s placement success", transport),
		func() bool { return ready && inventoryReady && parityViewLoaded(mirror) },
		func() string {
			return fmt.Sprintf("ready=%v inventoryReady=%v viewLoaded=%v", ready, inventoryReady, parityViewLoaded(mirror))
		},
		func() {
			_, messages := parityStep(t, host, endpoint, mirror)
			for _, message := range messages {
				switch message := message.(type) {
				case network.PlayerState:
					ready = ready || message.Ready
				case network.InventoryState:
					inventoryReady = inventoryReady || message.Inventory == inventory
				}
			}
		},
	)

	transcript := placementSuccessTranscript{Successes: make([]uint64, 0, 4)}
	step := func() {
		t.Helper()
		_, messages := parityStep(t, host, endpoint, mirror)
		for _, message := range messages {
			switch message := message.(type) {
			case network.PlaceBlockSucceeded:
				transcript.Successes = append(transcript.Successes, message.Sequence)
			case network.CommandRejected:
				transcript.Rejected = message
			}
		}
	}
	send := func(command network.ClientMessage) {
		t.Helper()
		sendIntegration(t, endpoint, command)
	}
	waitQueued := func(count int) {
		t.Helper()
		waitIntegrationCondition(t, fmt.Sprintf("%s placement commands queued", transport), func() bool {
			return len(host.world.incoming) >= count
		})
	}

	// 前两个命令在一次权威 Step 内结算，后两个各跨一个 tick。
	for _, sequence := range []uint64{1, 2} {
		send(network.PlaceBlock{Sequence: sequence, Pitch: -0.2, Slot: 0})
	}
	waitQueued(2)
	step()
	for _, sequence := range []uint64{3, 4} {
		send(network.PlaceBlock{Sequence: sequence, Pitch: -0.2, Slot: 0})
		waitQueued(1)
		step()
	}
	// 第 8 格是空栏位；命令线上合法，但权威放置必须拒绝且不发成功应答。
	send(network.PlaceBlock{Sequence: 5, Pitch: -0.2, Slot: 8})
	waitQueued(1)
	step()
	return transcript
}
