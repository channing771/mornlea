package capture

import (
	"fmt"
	"math"

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

// gifGrazeTriggerBlock 是吃草事件的触发格：211 号牛站位 (0.3,1,0.5) 的脚下
// 支撑格（脚底 Y 减探针后下取整），结算只写这一格。
var gifGrazeTriggerBlock = core.BlockPos{X: 0, Y: 0, Z: 0}

// gifGrazeScript 吃草前后：211 号牛前 4 帧常态、4 帧起低头，212 号常态对照；
// 触发格前 8 帧为草地、第 8 帧起为泥土（capture-only 写块口只允许切换触发格
// 一格；单格 remesh 由 worker 异步落盘，Step 内泵送收敛后才交出，保证泥土恰
// 在第 8 帧出现、不随机器速度漂移），同镜呈现草→泥。
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
			if err := prepareGIFGrazeDay(app); err != nil {
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
			if frame >= 4 {
				grazing = 1
			}
			if frame == 8 {
				if err := app.SetCaptureBlock(gifGrazeTriggerBlock, core.DirtID); err != nil {
					return fmt.Errorf("gif-graze 切换触发格: %w", err)
				}
				if err := settleGIFMesher(app); err != nil {
					return fmt.Errorf("gif-graze 等待泥土落盘: %w", err)
				}
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

// prepareGIFGrazeDay 装入 GIF 吃草剧本的世界夹具：与 PNG 吃草场景同 footprint
// 的纯草地（上表面 y=1），不预置泥土——泥土由剧本在结算帧经写块口切换。
func prepareGIFGrazeDay(app SceneApplication) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	for z := int32(-16); z <= 8; z++ {
		for x := int32(-10); x <= 10; x++ {
			position := core.BlockPos{X: x, Y: 0, Z: z}
			chunk := position.Chunk()
			if blocks[chunk] == nil {
				blocks[chunk] = make(map[core.BlockPos]core.BlockID)
			}
			blocks[chunk][position] = core.GrassID
		}
	}
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "GIF 吃草草地")
}

// gifLureScript 持麦靠近：持麦玩家逐帧走近，220 号牛逐帧向玩家移动并止步
// （末帧距玩家约止步距离）、朝向玩家；小麦掉落随玩家同行（被递出的麦），玩
// 家全程在帧内可见。跟随逻辑由 sim 单测兜底，GIF 只验呈现。
func gifLureScript() gifScript {
	const base = 2000
	cowStart := mgl32.Vec3{2.5, 1, -1.5}
	cowEnd := mgl32.Vec3{1.0, 1, -0.2}
	player := core.PlayerID{6: 0x40, 8: 0x80, 15: 0x31}
	playerStart := mgl32.Vec3{-2.6, 1, 3.0}
	playerEnd := mgl32.Vec3{-0.6, 1, 1.6}
	wheatID := core.DropID{Dimension: core.Overworld, Chunk: core.ChunkPos{X: -1, Z: 0}, Slot: 0, Generation: 1}
	cowYawToward := func(cow, target mgl32.Vec3) float32 {
		dx := target.X() - cow.X()
		dz := target.Z() - cow.Z()
		return float32(math.Atan2(float64(-dx), float64(-dz)))
	}
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
					{ID: 220, Dimension: core.Overworld, Position: cowStart, Yaw: cowYawToward(cowStart, playerStart), Health: core.MaxHealth},
				},
			}); err != nil {
				return err
			}
			// 持麦人出生在 Setup（而非第 0 帧）：收敛循环的 extra 字形帧把名牌
			// 光栅化完成，第 0 帧即是成品字形而非 tofu 占位符。
			if err := app.RemotePlayers().Apply(network.RemotePlayerSpawn{
				PlayerID: player, DisplayName: "持麦人",
				ServerTick: base, Position: playerStart,
			}); err != nil {
				return err
			}
			upsert, err := gifDropUpsert(base+1, gifBlockOf(playerStart), core.ItemWheat)
			if err != nil {
				return err
			}
			return app.ItemDrops().Apply(upsert)
		},
		Step: func(app SceneApplication, frame int) error {
			tick := uint64(base + 1 + frame)
			blend := float32(frame) / 15
			playerPos := playerStart.Add(playerEnd.Sub(playerStart).Mul(blend))
			cowPos := cowStart.Add(cowEnd.Sub(cowStart).Mul(blend))
			cowYaw := cowYawToward(cowPos, playerPos)
			app.SetServerTick(tick)
			if err := app.Passives().ApplyStates(network.PassiveState{
				ServerTick: tick,
				States: []network.PassiveStateRecord{
					{ID: 220, Position: cowPos, Yaw: cowYaw, Health: core.MaxHealth},
				},
			}); err != nil {
				return err
			}
			if err := app.RemotePlayers().Apply(network.RemotePlayerStates{
				ServerTick: tick,
				Players: []network.RemotePlayerState{{
					PlayerID: player, Dimension: core.Overworld,
					Position: playerPos, Yaw: 0.8,
				}},
			}); err != nil {
				return err
			}
			upsert, err := gifDropUpsert(tick, gifBlockOf(playerPos), core.ItemWheat)
			if err != nil {
				return err
			}
			// 小麦随人同行：同一掉落 ID 逐帧改写锚点（被递出的麦）。
			upsert.Drops[0].ID = wheatID
			return app.ItemDrops().Apply(upsert)
		},
	}
}

// gifBlockOf 返回位置脚底所在的方块坐标（GIF 剧本内的小麦跟随定位）。
func gifBlockOf(position mgl32.Vec3) core.BlockPos {
	return core.BlockPos{
		X: int32(math.Floor(float64(position.X()))),
		Y: int32(math.Floor(float64(position.Y()))),
		Z: int32(math.Floor(float64(position.Z()))),
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
