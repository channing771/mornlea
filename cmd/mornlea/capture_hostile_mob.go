package main

// capture_hostile_mob.go 装配夜行者的无窗口夜景 capture 场景：固定夜晚
//（18000 tick，与 torch-night 同一相位，昼夜亮度取夜间下限）的开阔草地，
// 三朵落地火把形成亮池，8 只夜行者经客户端镜像夹具固定在火把边缘——近处
// 个体被火把地面的亮池衬出轮廓，远处个体退入暗区。场景只呈现镜像事实，
// 不依赖任何服务端模拟推进，因此与机器速度无关。
//
// 8 只个体的 (x,z) 按相机（见 applyHostileMobCaptureState）的屏幕投影拉开
// 间距：任两只在 640×360 画面中的水平间隔都不小于约 3 具身位，深度互相
// 错开，画面因此恰好呈现 8 个互不遮挡的人形——这是 spec「图像 MUST 同时
// 显示 8 只夜行者人形」的布局前提，调整坐标前先复算投影。

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
)

// captureHostileMobSpawns 是 8 只夜行者的出生批次（tick 1）：ID 严格升序、
// 生命全部为满值 20。两具"特殊"个体刻意放在人眼一眼可辨的位置：ID 103
// （受击）在右侧火把亮池边，ID 107（追逐中）离相机最近、位居画面中部。
var captureHostileMobSpawns = []network.HostileSpawnRecord{
	{ID: 101, Dimension: core.Overworld, Position: mgl32.Vec3{-6, 1, -2}, Yaw: 2.2, Health: 20},
	{ID: 102, Dimension: core.Overworld, Position: mgl32.Vec3{-3.5, 1, -6}, Yaw: 0.6, Health: 20},
	{ID: 103, Dimension: core.Overworld, Position: mgl32.Vec3{6.5, 1, -6.5}, Yaw: -0.9, Health: 20},
	{ID: 104, Dimension: core.Overworld, Position: mgl32.Vec3{8.5, 1, -4}, Yaw: 2.8, Health: 20},
	{ID: 105, Dimension: core.Overworld, Position: mgl32.Vec3{-8, 1, -9}, Yaw: 1.2, Health: 20},
	{ID: 106, Dimension: core.Overworld, Position: mgl32.Vec3{-1, 1, -13}, Yaw: 0.3, Health: 20},
	{ID: 107, Dimension: core.Overworld, Position: mgl32.Vec3{2.8, 1, -1.5}, Yaw: 2.9, Health: 20},
	{ID: 108, Dimension: core.Overworld, Position: mgl32.Vec3{10, 1, -10}, Yaw: 3.0, Health: 20},
}

// captureHostileMobStates 是 tick 2 的状态批次（ID 严格升序）：受击个体
// （ID 103）生命从 20 掉到 13 并向左后方撤半步；追逐个体（ID 107）从出生
// 点向相机（玩家）方向推进约 3.7 格、朝向转为正对观察者的 yaw=π。两具
// 个体的夹具状态走与权威消息相同的镜像入口，镜像快照数不足 3 时呈现恒
// 等于最新快照，画面因此与帧间隔无关。
var captureHostileMobStates = []network.HostileStateRecord{
	{ID: 103, Position: mgl32.Vec3{5.8, 1, -7}, Yaw: -0.7, Health: 13},
	{ID: 107, Position: mgl32.Vec3{1.2, 1, 1.8}, Yaw: 3.1415927, Health: 20},
}

// prepareHostileMobNight 装入夜景世界夹具：一层 y=0 的草地（上表面 y=1）
// 加三朵落地火把。地面横向铺到 x=±10、纵向 z=-16..8，近端刻意延伸到相机
// 下方，画面里不出现夹具边缘的悬空断口。三朵火把都在草地方块的正上方
// （支撑是正下方实心草方块），与真实放置结果一致，且屏幕投影落在相邻
// 夜行者的空档里，不遮挡任何人形。火把间隔拉开，使地面亮池之间出现可测
// 的暗带——夜行者在亮池内、亮池边缘与暗区三种位置各有落点，「火把边缘
// 照明」因此是画面里可复核的梯度而不是摆拍术语。
func prepareHostileMobNight(app *application) error {
	if err := prepareCaptureAirNeighborhood(app); err != nil {
		return err
	}
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	setBlock := func(position core.BlockPos, block core.BlockID) {
		chunk := position.Chunk()
		if blocks[chunk] == nil {
			blocks[chunk] = make(map[core.BlockPos]core.BlockID)
		}
		blocks[chunk][position] = block
	}
	for z := int32(-16); z <= 8; z++ {
		for x := int32(-10); x <= 10; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.GrassID)
		}
	}
	setBlock(core.BlockPos{X: -3, Y: 1, Z: -1}, core.TorchStandingID)
	setBlock(core.BlockPos{X: 5, Y: 1, Z: -11}, core.TorchStandingID)
	setBlock(core.BlockPos{X: -4, Y: 1, Z: -14}, core.TorchStandingID)
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "夜行者草地")
}

// applyHostileMobCaptureState 钉死本场景的全部呈现状态并装入夜行者镜像。
// 清理复用 `resetCapturePresentation`（含夜行者镜像与呈现缓存），保证前一
// 场景留下的实体、容器、聊天与旧夜行者不渗入本场景；随后按 spawn→state
// 两批经与权威消息相同的 Apply 入口注入 8 只夜行者。注入在 Apply 中完成
// （drain 之后、收敛帧之前），不会被任何服务端消息覆盖。
func applyHostileMobCaptureState(app *application) error {
	if app.hostiles == nil {
		return fmt.Errorf("hostile-mob 需要夜行者镜像，当前为 nil")
	}
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.worldTimeTicks = 18000
	// 相机钉在草地上方平视夜行者群：近处追逐个体约占画面高度三成，
	// 远处暗区个体与火把亮池同框，受击与追逐呈现一眼可辨。
	app.camera = client.Camera{
		Pos: mgl32.Vec3{0.5, 2.8, 6.5}, Yaw: 0, Pitch: -0.12,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.center = cameraChunk(app.camera.Pos)
	app.blockTargetReset = false

	if err := app.hostiles.ApplySpawn(network.HostileSpawn{
		ServerTick: 1, Spawns: captureHostileMobSpawns,
	}); err != nil {
		return fmt.Errorf("装入夜行者出生批次: %w", err)
	}
	if err := app.hostiles.ApplyStates(network.HostileState{
		ServerTick: 2, States: captureHostileMobStates,
	}); err != nil {
		return fmt.Errorf("装入夜行者状态批次: %w", err)
	}
	// 夹具自检：批次的 ID 升序由镜像校验兜底，这里只锁数量——少装任何一只
	// 都会让 golden 少一个人形，宁可当场失败也不产出静默缺员的基线。
	if got := len(app.hostiles.AppendPresentations(nil)); got != len(captureHostileMobSpawns) {
		return fmt.Errorf("夜行者镜像数量=%d，想要 %d", got, len(captureHostileMobSpawns))
	}
	return nil
}
