//go:build darwin

package app

// multiplayer_benchmark_scenario.go：多人 benchmark 场景的确定性输入集合
// （本地玩家、七名远端玩家出生消息与名牌）。它由 benchmark 观察者装配路径
// 消费，也被 capture 的呈现转换测试用作固定排序输入，因此随共享夹具下沉
// 本包；取值与顺序是场景身份的一部分，不得单侧改动。

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// benchmarkSessionViewDistances 是多人 benchmark 场景按会话登录顺序声明的
// 视距梯度：本地玩家在前、七名远端依次在后，2/4/6/8 循环两轮。取值与
// 顺序是场景身份的一部分——它决定各会话的订阅方形与服务端实际加载的区块
// 并集（scenario v23 起经 v40 登录协商逐会话生效），不得单侧改动。
//
// 上端取 8 而不是更大的值，是探针预算的硬约束而非偏好：服务端存档 job 是
// 单 FIFO，缺失区块的生成 job 排在全部装载 job 之后，探针服务端
// （Workers=1、job 通道每 tick 只补有限个）在既有 20+200 tick 窗口内只能
// 消化约数百个区块的双趟 job。视距 8 的会话订阅方形（半径 9 → 361 区块）
// 是能在窗口内持续产出 Ready 的最大规格；更大的上端（如 32 → 4489 区块）
// 会让首个 Ready 落在窗口之外，探针的 streaming 完整性门禁必然失败。
var benchmarkSessionViewDistances = [8]uint8{2, 4, 6, 8, 2, 4, 6, 8}

type MultiplayerBenchmarkScenario struct {
	LocalPlayerID core.PlayerID
	Spawns        []network.RemotePlayerSpawn
	Tags          []render.NameTag
	// ViewDistances 是各会话在 v40 登录消息中声明的期望视距，顺序与探针
	// 会话登录顺序一致（本地玩家索引 0，远端按 Spawns 顺序）。渲染侧消费
	// 的是 Spawns/Tags；本字段只喂给登录协商，不进任何 wire 呈现夹具。
	ViewDistances []uint8
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
	return MultiplayerBenchmarkScenario{
		LocalPlayerID: local,
		Spawns:        spawns,
		Tags:          tags,
		ViewDistances: append([]uint8(nil), benchmarkSessionViewDistances[:]...),
	}
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
