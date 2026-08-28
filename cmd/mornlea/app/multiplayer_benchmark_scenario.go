//go:build darwin

package app

// multiplayer_benchmark_scenario.go：多人 benchmark 场景的确定性输入集合
// （本地玩家、七名远端玩家出生消息与名牌）。它由 benchmark 观察者装配路径
// 消费，也被 capture 的呈现转换测试用作固定排序输入，因此随共享夹具下沉
// 本包；取值与顺序是场景身份的一部分，不得单侧改动。

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
)

type MultiplayerBenchmarkScenario struct {
	LocalPlayerID core.PlayerID
	Spawns        []network.RemotePlayerSpawn
	Tags          []render.NameTag
}

func NewMultiplayerBenchmarkScenario() MultiplayerBenchmarkScenario {
	local := benchmarkPlayerID(0)
	names := [...]string{"星野", "月河", "云山", "海界", "星河", "月海", "云野"}
	spawns := make([]network.RemotePlayerSpawn, len(names))
	tags := make([]render.NameTag, len(names), MaxFrameNameTags)
	for index, name := range names {
		angle := float64(index) * 2 * math.Pi / float64(len(names))
		position := mgl32.Vec3{float32(math.Cos(angle)) * 4, 80, float32(math.Sin(angle))*4 - 8}
		playerID := benchmarkPlayerID(index + 1)
		spawns[index] = network.RemotePlayerSpawn{
			PlayerID: playerID, DisplayName: name, ServerTick: 1,
			Dimension: core.Overworld, Position: position,
		}
		tags[index] = render.NameTag{
			Key:  render.EntityKey{Kind: render.EntityPlayer, ID: [16]byte(playerID)},
			Text: name, Anchor: position.Add(mgl32.Vec3{0, 2.05, 0}),
		}
	}
	return MultiplayerBenchmarkScenario{LocalPlayerID: local, Spawns: spawns, Tags: tags}
}

func (scenario MultiplayerBenchmarkScenario) States(tick uint64) network.RemotePlayerStates {
	states := make([]network.RemotePlayerState, len(scenario.Spawns))
	for index, spawn := range scenario.Spawns {
		phase := float64(tick)*0.035 + float64(index)*2*math.Pi/float64(len(scenario.Spawns))
		position := spawn.Position.Add(mgl32.Vec3{
			float32(math.Sin(phase)) * 1.5,
			0,
			float32(math.Cos(phase)) * 1.5,
		})
		states[index] = network.RemotePlayerState{
			PlayerID: spawn.PlayerID, Dimension: core.Overworld, Position: position,
			Yaw:   float32(math.Atan2(math.Sin(phase), math.Cos(phase))),
			Pitch: float32(math.Sin(phase*0.5)) * 0.15,
		}
	}
	return network.RemotePlayerStates{ServerTick: tick, Players: states}
}
