package capture

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// passiveDeathGIFScripts 是四条 GIF 剧本的注册表：与 `captureScenes` 解耦
// （独立目录、独立顺序），覆盖吃草前后/持麦靠近/击杀/牛肉掉落。
var passiveDeathGIFScripts = []gifScript{
	gifGrazeScript(),
	gifLureScript(),
	gifKillScript(),
	gifBeefDropScript(),
}

// gifGrazeScript 吃草前后：211 号牛前 6 帧常态、后 6 帧低头（放牧位翻转即
// 动作），212 号常态对照；泥土格静态预置（吃草结算的既有对照）。
func gifGrazeScript() gifScript {
	const base = 1000
	standing := mgl32.Vec3{0.3, 1, 0.5}
	reference := mgl32.Vec3{-3.4, 1, 1.2}
	return gifScript{
		Name:   "graze",
		Frames: 12,
		Setup: func(app SceneApplication) error {
			if app.Passives() == nil {
				return fmt.Errorf("gif-graze 需要被动牛镜像，当前为 nil")
			}
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			app.SetWorldTimeTicks(6000)
			applyGIFPastureCamera(app)
			if err := preparePassiveGrazeDay(app); err != nil {
				return err
			}
			return app.Passives().ApplySpawn(network.PassiveSpawn{
				ServerTick: base,
				Spawns: []network.PassiveSpawnRecord{
					{ID: 211, Dimension: core.Overworld, Position: standing, Yaw: 0, Health: core.MaxHealth},
					{ID: 212, Dimension: core.Overworld, Position: reference, Yaw: 0.7, Health: core.MaxHealth},
				},
			})
		},
		Step: func(app SceneApplication, frame int) error {
			tick := uint64(base + 1 + frame)
			var grazing uint8
			if frame >= 6 {
				grazing = 1
			}
			app.SetServerTick(tick)
			return app.Passives().ApplyStates(network.PassiveState{
				ServerTick: tick,
				States: []network.PassiveStateRecord{
					{ID: 211, Position: standing, Yaw: 0, Health: core.MaxHealth, Grazing: grazing},
					{ID: 212, Position: reference, Yaw: 0.7, Health: core.MaxHealth},
				},
			})
		},
	}
}

// gifLureScript 持麦靠近：220 号牛静立，小麦掉落置于牛身前（被递出的麦），
// 远端玩家逐帧 deterministic 靠近（位置是帧号的纯函数，步进固定 elapsed）。
func gifLureScript() gifScript {
	const base = 2000
	cow := mgl32.Vec3{0.8, 1, -0.8}
	wheatBlock := core.BlockPos{X: 0, Y: 1, Z: 1}
	player := core.PlayerID{6: 0x40, 8: 0x80, 15: 0x31}
	start := mgl32.Vec3{-5, 1, 4.5}
	end := mgl32.Vec3{-0.6, 1, 1.6}
	return gifScript{
		Name:   "lure",
		Frames: 16,
		Setup: func(app SceneApplication) error {
			if app.Passives() == nil || app.RemotePlayers() == nil || app.ItemDrops() == nil {
				return fmt.Errorf("gif-lure 需要被动牛/远端玩家/掉落镜像")
			}
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			app.SetWorldTimeTicks(6000)
			applyGIFPastureCamera(app)
			if err := preparePassiveHerdDay(app); err != nil {
				return err
			}
			if err := app.Passives().ApplySpawn(network.PassiveSpawn{
				ServerTick: base,
				Spawns: []network.PassiveSpawnRecord{
					{ID: 220, Dimension: core.Overworld, Position: cow, Yaw: -0.5, Health: core.MaxHealth},
				},
			}); err != nil {
				return err
			}
			upsert, err := gifDropUpsert(base+1, wheatBlock, core.ItemWheat)
			if err != nil {
				return err
			}
			return app.ItemDrops().Apply(upsert)
		},
		Step: func(app SceneApplication, frame int) error {
			tick := uint64(base + 1 + frame)
			blend := float32(frame) / 15
			position := start.Add(end.Sub(start).Mul(blend))
			app.SetServerTick(tick)
			if frame == 0 {
				if err := app.RemotePlayers().Apply(network.RemotePlayerSpawn{
					PlayerID: player, DisplayName: "持麦人",
					ServerTick: tick, Position: position,
				}); err != nil {
					return err
				}
				return nil
			}
			return app.RemotePlayers().Apply(network.RemotePlayerStates{
				ServerTick: tick,
				Players: []network.RemotePlayerState{{
					PlayerID: player, Dimension: core.Overworld,
					Position: position, Yaw: 0.8,
				}},
			})
		},
	}
}

// gifKillScript 击杀：230 号牛前 4 帧存活，第 4 帧死亡 despawn（原因位死亡）
// 并在牛身下刷出生牛肉掉落，随后 20 帧完整覆盖红闪侧倒保留期（T+19 仍在、
// T+20 移除），231 号活牛对照。
func gifKillScript() gifScript {
	const base = 3000
	victim := mgl32.Vec3{0.8, 1, -0.8}
	witness := mgl32.Vec3{-3.4, 1, 1.2}
	dropBlock := core.BlockPos{X: 0, Y: 1, Z: -1}
	return gifScript{
		Name:   "kill",
		Frames: 25,
		Setup: func(app SceneApplication) error {
			if app.Passives() == nil || app.ItemDrops() == nil {
				return fmt.Errorf("gif-kill 需要被动牛/掉落镜像")
			}
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			app.SetWorldTimeTicks(6000)
			applyGIFPastureCamera(app)
			if err := preparePassiveHerdDay(app); err != nil {
				return err
			}
			return app.Passives().ApplySpawn(network.PassiveSpawn{
				ServerTick: base,
				Spawns: []network.PassiveSpawnRecord{
					{ID: 230, Dimension: core.Overworld, Position: victim, Yaw: -0.5, Health: core.MaxHealth},
					{ID: 231, Dimension: core.Overworld, Position: witness, Yaw: 0.7, Health: core.MaxHealth},
				},
			})
		},
		Step: func(app SceneApplication, frame int) error {
			tick := uint64(base + 1 + frame)
			app.SetServerTick(tick)
			if frame < 4 {
				return app.Passives().ApplyStates(network.PassiveState{
					ServerTick: tick,
					States: []network.PassiveStateRecord{
						{ID: 230, Position: victim, Yaw: -0.5, Health: core.MaxHealth},
						{ID: 231, Position: witness, Yaw: 0.7, Health: core.MaxHealth},
					},
				})
			}
			if frame == 4 {
				if err := app.Passives().ApplyDespawn(network.PassiveDespawn{
					ServerTick: tick,
					Despawns:   []network.PassiveDespawnRecord{{ID: 230, Reason: network.PassiveDespawnDied}},
				}); err != nil {
					return err
				}
				upsert, err := gifDropUpsert(tick, dropBlock, core.ItemRawBeef)
				if err != nil {
					return err
				}
				if err := app.ItemDrops().Apply(upsert); err != nil {
					return err
				}
			}
			// 死亡后不再有该 ID 的 state（服务端语义）：只推进见证牛。
			return app.Passives().ApplyStates(network.PassiveState{
				ServerTick: tick,
				States: []network.PassiveStateRecord{
					{ID: 231, Position: witness, Yaw: 0.7, Health: core.MaxHealth},
				},
			})
		},
	}
}

// gifBeefDropScript 牛肉掉落：单个生牛肉掉落的浮动与旋转（权威 tick 派生，
// 每帧 2 tick 步进，转相更明显），无牛无玩家。
func gifBeefDropScript() gifScript {
	const base = 4000
	dropBlock := core.BlockPos{X: 0, Y: 1, Z: 3}
	return gifScript{
		Name:   "beef-drop",
		Frames: 12,
		Setup: func(app SceneApplication) error {
			if app.ItemDrops() == nil {
				return fmt.Errorf("gif-beef-drop 需要掉落镜像")
			}
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			app.SetWorldTimeTicks(6000)
			applyGIFPastureCamera(app)
			if err := preparePassiveHerdDay(app); err != nil {
				return err
			}
			upsert, err := gifDropUpsert(base+1, dropBlock, core.ItemRawBeef)
			if err != nil {
				return err
			}
			return app.ItemDrops().Apply(upsert)
		},
		Step: func(app SceneApplication, frame int) error {
			app.SetServerTick(uint64(base + 1 + 2*frame))
			return nil
		},
	}
}
