//go:build darwin

package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func TestApplicationCharacterizationTranscriptParity(t *testing.T) {
	application, serverEndpoint := newInteractiveTestApplication(t)
	application.remotePlayers = client.NewRemotePlayers()
	application.loadedChunks = make(map[core.ChunkPos]struct{})
	application.frameWidth = 800
	application.frameHeight = 450
	application.menu.phase = MenuPhaseGame
	application.mesher = client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(application.mesher.Close)

	state := network.PlayerState{
		ServerTick: 10, Dimension: core.Overworld,
		Position: mgl32.Vec3{1.5, 20, 2.5}, Yaw: 0.5, Pitch: -0.25,
		OnGround: true, Ready: true, Reset: true,
		Health: 17, Oxygen: 180, Hunger: 13, SaturationZero: true,
		WorldTimeTicks: 6000, DayPhaseOffset: 250, WeatherKind: core.WeatherRain,
		Season: core.SeasonAutumn, SeasonProgress: 128, Temperature: 7, ArmorPoints: 5,
	}
	spawn := RemoteSpawn(7, "Pilot", 10, mgl32.Vec3{4, 21, -3})
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = network.SectionData{
			Y: int32(index), Storage: network.SectionSingle, Single: core.AirID,
		}
	}
	snapshot := network.ChunkSnapshot{
		Dimension: core.Overworld, Chunk: core.ChunkPos{}, Revision: 3, Sections: sections,
	}
	for _, message := range []network.ServerMessage{state, spawn, snapshot} {
		sendInteractiveServerMessage(t, serverEndpoint, message)
	}

	var transcript []string
	application.DrainServerMessages(2)
	_, chunkLoaded := application.mirror.Chunk(core.Overworld, snapshot.Chunk)
	transcript = append(transcript, fmt.Sprintf(
		"drain budget=2 tick=%d remotes=%d chunk_loaded=%t",
		application.serverTick, len(application.remotePlayers.Presentations()), chunkLoaded,
	))

	application.applyInteractiveInput(
		physics.FixedDelta,
		client.Movement{MoveX: 1, MoveZ: -1, Jump: true, Sprinting: true},
		client.Actions{Mining: true},
		true,
	)
	inputMessage := receiveInteractiveClientMessage(t, serverEndpoint)
	input, ok := inputMessage.(network.PlayerInput)
	if !ok {
		t.Fatalf("predicted input type = %T, want network.PlayerInput", inputMessage)
	}
	predicted, ready := application.predictor.State()
	transcript = append(transcript, fmt.Sprintf(
		"input ready=%t sequence=%d move=%d,%d jump=%t mining=%t sprint=%t look=%.2f,%.2f history=%d predicted=%.4f,%.4f,%.4f velocity=%.4f,%.4f,%.4f",
		ready, input.Sequence, input.MoveX, input.MoveZ, input.Jump, input.Mining, input.Sprinting,
		input.Yaw, input.Pitch, application.predictor.HistoryLen(),
		predicted.Position[0], predicted.Position[1], predicted.Position[2],
		predicted.Velocity[0], predicted.Velocity[1], predicted.Velocity[2],
	))

	application.DrainServerMessages(1)
	_, chunkLoaded = application.mirror.Chunk(core.Overworld, snapshot.Chunk)
	dirty := application.mesher.Stats().DirtySections
	firstSection := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	release := application.mesher.BlockForTest(firstSection)
	t.Cleanup(release)
	application.mesher.Schedule(application.mirror, 1)
	meshStats := application.mesher.Stats()
	transcript = append(transcript, fmt.Sprintf(
		"mesh drain_budget=1 chunk_loaded=%t dirty=%d schedule_budget=1 scheduled=%d result_capacity=%d",
		chunkLoaded, dirty, meshStats.QueuedJobs+meshStats.InFlightJobs, meshStats.ResultCapacity,
	))

	presentations := application.remotePlayers.Presentations()
	avatars, tags := RemoteRenderPresentations(presentations)
	hudState := application.assembleHUDState()
	if len(avatars) != 1 || len(tags) != 1 || hudState.Health == nil ||
		hudState.Armor == nil || hudState.Hunger == nil || hudState.Oxygen == nil {
		t.Fatalf("incomplete presentation snapshot: avatars=%d tags=%d hud=%+v", len(avatars), len(tags), hudState)
	}
	transcript = append(transcript, fmt.Sprintf(
		"presentation camera=%.2f,%.2f,%.2f look=%.2f,%.2f remote=%d avatar=%.2f,%.2f,%.2f tag=%q@%.2f hud=%dx%d/%d/%d/%d/%d/crosshair:%t environment=%d/%d/%d/%d/%d/%d",
		application.camera.Pos[0], application.camera.Pos[1], application.camera.Pos[2],
		application.camera.Yaw, application.camera.Pitch,
		len(avatars), avatars[0].Position[0], avatars[0].Position[1], avatars[0].Position[2],
		tags[0].Text, tags[0].Anchor[1],
		hudState.Viewport.Width, hudState.Viewport.Height, hudState.Health.Value,
		hudState.Armor.Points, hudState.Hunger.Value, hudState.Oxygen.Value, hudState.Crosshair,
		application.serverTick, application.worldTimeTicks, application.dayPhaseOffset,
		application.weather, application.season, application.seasonProgress,
	))

	application.inventoryOpen = true
	application.miningOverlay = hud.MiningOverlay{Active: true}
	application.combatFeedback.Observe(application.serverTick)
	application.resetSessionOwnedState()
	resetHUD := application.assembleHUDState()
	transcript = append(transcript, fmt.Sprintf(
		"reset closed=%t remotes=%d inventory_open=%t mining=%t marker=%t confirmed_hud=%t",
		application.clientSessionClosed, len(application.remotePlayers.Presentations()),
		application.inventoryOpen, application.miningOverlay.Active,
		application.combatFeedback.MarkerVisible(), resetHUD.Health != nil,
	))

	got := strings.Join(transcript, "\n")
	const want = `drain budget=2 tick=10 remotes=1 chunk_loaded=false
input ready=true sequence=1 move=1,-1 jump=true mining=true sprint=true look=0.50,-0.25 history=1 predicted=1.5960,20.2000,2.5282 velocity=1.9191,0.0000,0.5631
mesh drain_budget=1 chunk_loaded=true dirty=24 schedule_budget=1 scheduled=1 result_capacity=64
presentation camera=1.50,21.62,2.50 look=0.50,-0.25 remote=1 avatar=4.00,21.00,-3.00 tag="Pilot"@23.05 hud=800x450/17/5/13/180/crosshair:true environment=10/6000/250/1/2/128
reset closed=true remotes=0 inventory_open=false mining=false marker=false confirmed_hud=false`
	if got != want {
		t.Fatalf("characterization transcript changed:\n%s\nwant:\n%s", got, want)
	}
}
