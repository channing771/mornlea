package capture

// capture_ranged_mob.go 装配远程敌怪的无窗口夜景 capture 场景：固定夜晚
// （18000 tick，与 hostile-mob 同一相位，昼夜亮度取夜间下限）的开阔草地，
// 两只掷骨者经客户端敌怪镜像夹具固定在草地远端、正对目标玩家投掷，两名
// 事实中的「投掷结果」——骨刺——经投射物镜像夹具钉在飞行中途；目标玩家
// 经远端玩家镜像站在掷骨者与相机之间、面向威胁。场景只呈现镜像事实，
// 不依赖任何服务端模拟推进；骨刺的弹道相位经 PinVolatile 在收敛帧之后
// 重放投射物批次钉死，与机器速度无关。
//
// 构图（相机见 applyRangedMobCaptureState，从玩家身后高位向 -Z 平视）：
// 画面中部是目标玩家的背影与名牌，远端左右各一只骨白双足的掷骨者
//（火把亮池衬出轮廓），两枚骨白棱刺沿各自掷出弧线飞向玩家身前——
// 掷骨者、飞行骨刺与目标玩家三者同框，即远程敌怪战斗的可读基线。

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// captureRangedMobTargetPlayer 是目标玩家的出生事实：站在掷骨者与相机之间
// （z=4）、面向 -Z 的威胁方向（背对相机），是骨刺飞行方向的可读落点；
// 名牌 "Guard" 经远端玩家链路产生本场景唯一的一枚名牌。
var captureRangedMobTargetPlayer = network.RemotePlayerSpawn{
	// 占位 PlayerID 与仓库测试同款合法 UUIDv4 形状（avatar-nametag 先例）。
	PlayerID:    core.PlayerID{6: 0x40, 8: 0x80, 15: 7},
	DisplayName: "Guard",
	ServerTick:  1,
	Dimension:   core.Overworld,
	Position:    mgl32.Vec3{0.5, 1, 4},
	Yaw:         0,
}

// captureRangedMobSpawns 是 2 只掷骨者的出生批次（tick 1）：ID 严格升序、
// kind=1（骨白双足分支）、生命满值，各自站在草地远端并正对目标玩家
// （yaw 取「玩家位置 - 掷骨者位置」水平方向的物理朝向，与权威投掷结算
// 重建初速时的 yaw/pitch 推导同一约定）。
var captureRangedMobSpawns = []network.HostileSpawnRecord{
	{ID: 301, Dimension: core.Overworld, Position: mgl32.Vec3{-2, 1, -3}, Yaw: -2.8, Health: core.MaxHealth, Kind: network.HostileKindBoneThrower},
	{ID: 302, Dimension: core.Overworld, Position: mgl32.Vec3{3.2, 1, -4.5}, Yaw: 2.83, Health: core.MaxHealth, Kind: network.HostileKindBoneThrower},
}

// captureRangedMobShards 是 2 枚骨刺的出生批次（tick 2，ID 严格升序）：
// 各自钉在「掷骨者眼位 → 目标玩家眼位」飞行路径的中途，速度取朝玩家
// 眼位方向的骨刺初速（22 格/秒量级，与权威投掷初速同一数值契约），
// 长轴沿速度取向的骨白棱刺因此以逼近玩家的姿态入画。位置与速度都是
// 常量：镜像快照数不足 3 时呈现恒等于最新快照，批次不驱动任何模拟。
var captureRangedMobShards = []network.ProjectileSpawnRecord{
	{ID: 501, Kind: network.ProjectileKindShard, Dimension: core.Overworld,
		Position: mgl32.Vec3{-0.6, 2.45, 0.9}, Velocity: mgl32.Vec3{7.3, 1.1, 20.7}},
	{ID: 502, Kind: network.ProjectileKindShard, Dimension: core.Overworld,
		Position: mgl32.Vec3{1.6, 2.35, 0.6}, Velocity: mgl32.Vec3{-6.6, 1.7, 20.9}},
}

// 弹道批次的权威 tick 常量：出生批次在 Apply 装入（drain 之后，不会被
// 服务端消息覆盖）；PinVolatile 以更大的 tick 重放批次，把每枚骨刺的插值
// 时钟归零并重钉最新快照——收敛帧期间的帧间隔推进因此不进入最终帧。
const (
	captureRangedMobShardSpawnTick = 2
	captureRangedMobShardPinTick   = 3
)

// prepareRangedMobNight 装入夜景世界夹具：一层 y=0 的草地（上表面 y=1）
// 加三朵落地火把。地面横向铺到 x=±10、纵向 z=-16..10，近端刻意延伸到相机
// 下方，画面里不出现夹具边缘的悬空断口。火把都在草地方块正上方（支撑是
// 正下方实心草方块），两朵分立掷骨者侧照亮投手轮廓、一朵靠近目标玩家
// 照亮落点区，屏幕投影都落在人物的空档里，不遮挡任何身体或骨刺。
func prepareRangedMobNight(app SceneApplication) error {
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
	for z := int32(-16); z <= 10; z++ {
		for x := int32(-10); x <= 10; x++ {
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.GrassID)
		}
	}
	setBlock(core.BlockPos{X: -4, Y: 1, Z: 0}, core.TorchStandingID)
	setBlock(core.BlockPos{X: 4, Y: 1, Z: -2}, core.TorchStandingID)
	setBlock(core.BlockPos{X: 1, Y: 1, Z: 6}, core.TorchStandingID)
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "掷骨者草地")
}

// applyRangedMobCaptureState 钉死本场景的全部呈现状态并装入敌怪、投射物
// 与目标玩家镜像。清理复用 `resetCapturePresentation`（含敌怪、投射物、
// 远端玩家镜像与呈现缓存），保证前一场景留下的实体、弹道、容器、聊天与
// 旧夜行者不渗入本场景；随后按玩家→掷骨者→骨刺的顺序经与权威消息相同
// 的 Apply 入口注入夹具。注入在 Apply 中完成（drain 之后、收敛帧之前），
// 不会被任何服务端消息覆盖。
func applyRangedMobCaptureState(app SceneApplication) error {
	if app.Hostiles() == nil {
		return fmt.Errorf("ranged-mob 需要敌怪镜像，当前为 nil")
	}
	if app.Projectiles() == nil {
		return fmt.Errorf("ranged-mob 需要投射物镜像，当前为 nil")
	}
	if app.RemotePlayers() == nil {
		return fmt.Errorf("ranged-mob 需要远端玩家追踪器，当前为 nil")
	}
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(18000)
	// 相机钉在目标玩家身后的高位、向 -Z 平视：玩家背影占画面下部，掷骨者
	// 与飞行骨刺同框在中远景，火把亮池衬出人物轮廓。
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{0.5, 2.6, 8.5}, Yaw: 0, Pitch: -0.1,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetBlockTargetReset(false)

	if err := app.RemotePlayers().Apply(captureRangedMobTargetPlayer); err != nil {
		return fmt.Errorf("装入目标玩家: %w", err)
	}
	if err := app.Hostiles().ApplySpawn(network.HostileSpawn{
		ServerTick: 1, Spawns: captureRangedMobSpawns,
	}); err != nil {
		return fmt.Errorf("装入掷骨者出生批次: %w", err)
	}
	if err := app.Projectiles().ApplySpawn(network.ProjectileSpawn{
		ServerTick: captureRangedMobShardSpawnTick, Spawns: captureRangedMobShards,
	}); err != nil {
		return fmt.Errorf("装入骨刺出生批次: %w", err)
	}
	// 夹具自检：少装任何一只掷骨者或一枚骨刺都会让 golden 缺主体，宁可
	// 当场失败也不产出静默缺员的基线。
	if got := len(app.Hostiles().AppendPresentations(nil)); got != len(captureRangedMobSpawns) {
		return fmt.Errorf("掷骨者镜像数量=%d，想要 %d", got, len(captureRangedMobSpawns))
	}
	if got := len(app.Projectiles().AppendPresentations(nil)); got != len(captureRangedMobShards) {
		return fmt.Errorf("骨刺镜像数量=%d，想要 %d", got, len(captureRangedMobShards))
	}
	return nil
}

// pinRangedMobVolatile 在收敛帧之后、最终帧之前重放投射物批次：先按夹具
// ID despawn 再以更大的权威 tick 重新 spawn。重放把每枚骨刺的插值时钟
// 归零（快照环重置为单一夹具快照），最终帧的骨刺位置因此恒等于夹具值，
// 与收敛帧数、帧间隔等机器速度相关的量无关——这是「钉弹道相位」的落点。
// 掷骨者与目标玩家各只有单一快照（呈现恒等于该快照），无需重钉。
func pinRangedMobVolatile(app SceneApplication) error {
	if app.Projectiles() == nil {
		return fmt.Errorf("ranged-mob 钉弹道相位需要投射物镜像，当前为 nil")
	}
	ids := make([]uint64, 0, len(captureRangedMobShards))
	for _, shard := range captureRangedMobShards {
		ids = append(ids, shard.ID)
	}
	if err := app.Projectiles().ApplyDespawn(network.ProjectileDespawn{
		ServerTick: captureRangedMobShardPinTick, IDs: ids,
	}); err != nil {
		return fmt.Errorf("重钉骨刺（despawn）: %w", err)
	}
	if err := app.Projectiles().ApplySpawn(network.ProjectileSpawn{
		ServerTick: captureRangedMobShardPinTick, Spawns: captureRangedMobShards,
	}); err != nil {
		return fmt.Errorf("重钉骨刺（spawn）: %w", err)
	}
	return nil
}
