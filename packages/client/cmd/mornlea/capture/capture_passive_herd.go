package capture

// capture_passive_herd.go 装配被动牛群的无窗口昼间 capture 场景：固定正午
//（6000 tick，与夜行者夜景的 18000 tick 相对）的开阔草地，3 头牛经客户端
// 被动镜像夹具站在草地上，1 个生牛肉掉落浮在牛群前方的空位。场景只呈现镜像
// 事实，不依赖任何服务端模拟推进，因此与机器速度无关。
//
// 3 头牛的 (x,z) 按相机（见 `applyPassiveHerdCaptureState`，沿用夜行者群的
// 已验证机位）的屏幕投影拉开间距：左/中/右各一头，近处两头大、远处一头小，
// 画面因此恰好呈现 3 具互不遮挡的四足体；掉落落在相机正前方近景、中牛脚下
// 前方，纵向错开不遮挡任何牛身。调整坐标前先复算投影。
//
// 掉落动画相位由权威 tick 与稳定 ID 混合决定（见 `render` 的掉落物 pass），
// 而权威 tick 的取值取决于加载收敛花了多久——本身随机器速度漂移。`Apply`
// 只负责把夹具装入镜像，相位钉死另由 `pinPassiveHerdVolatile` 在收敛帧之后
// 完成（`PinVolatile` 语义），最终帧的掉落朝向与浮动高度因此是纯常量的确定
// 函数。

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/world"
)

// capturePassiveHerdSpawns 是 3 头牛的出生批次（tick 1）：ID 严格升序、生命
// 全部为满值。朝向各异：左牛侧对相机露出牛身斑点、中牛背对远方、右牛正对偏
// 右，四足轮廓与牛头双眼在画面里各有可辨角度。
var capturePassiveHerdSpawns = []network.PassiveSpawnRecord{
	{ID: 201, Dimension: core.Overworld, Position: mgl32.Vec3{-3.4, 1, 1.2}, Yaw: 0.7, Health: core.MaxHealth},
	{ID: 202, Dimension: core.Overworld, Position: mgl32.Vec3{0.8, 1, -0.8}, Yaw: -0.5, Health: core.MaxHealth},
	{ID: 203, Dimension: core.Overworld, Position: mgl32.Vec3{4.6, 1, 1.5}, Yaw: 2.6, Health: core.MaxHealth},
}

// capturePassiveHerdDropBlock 是生牛肉掉落的锚定方块：相机正前方近景的空气格，
// 屏幕投影落在中牛脚下前方，纵向错开不遮挡任何牛身。掉落物 pass 按方块中心
// 浮起约半格渲染，锚点本身不占画面遮挡。
var capturePassiveHerdDropBlock = core.BlockPos{X: 0, Y: 1, Z: 3}

// capturePassiveHerdServerTick 是抓帧最终帧钉死的权威 tick：掉落旋转/浮动
// 相位是该 tick 与掉落稳定 ID 的纯函数，取值与加载耗时无关，基线因此可复现。
const capturePassiveHerdServerTick = 64

// preparePassiveHerdDay 装入昼间牧场世界夹具：一层 y=0 的草地（上表面 y=1）。
// 地面横向铺到 x=±10、纵向 z=-16..8，近端刻意延伸到相机下方，画面里不出现
// 夹具边缘的悬空断口。白昼场景不需要火把：正午日光是唯一的固定光源。
func preparePassiveHerdDay(app SceneApplication) error {
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
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "牛群草地")
}

// applyPassiveHerdCaptureState 钉死本场景的全部呈现状态并装入牛群与掉落镜像。
// 清理复用 `resetCapturePresentation`（含被动牛镜像与掉落镜像），保证前一
// 场景留下的实体、容器、聊天与旧牛群不渗入本场景；随后按 spawn→upserts 两批
// 经与权威消息相同的 Apply 入口注入 3 头牛与 1 个生牛肉掉落。注入在 Apply
// 中完成（drain 之后、收敛帧之前），不会被任何服务端消息覆盖。
func applyPassiveHerdCaptureState(app SceneApplication) error {
	if app.Passives() == nil {
		return fmt.Errorf("passive-herd 需要被动牛镜像，当前为 nil")
	}
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	// 相机沿用夜行者群的已验证机位：钉在草地上方平视牛群，近处掉落与远处
	// 牛群同框，三头四足体在画面里一字排开。
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{0.5, 2.8, 6.5}, Yaw: 0, Pitch: -0.12,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetBlockTargetReset(false)

	if err := app.Passives().ApplySpawn(network.PassiveSpawn{
		ServerTick: 1, Spawns: capturePassiveHerdSpawns,
	}); err != nil {
		return fmt.Errorf("装入牛群出生批次: %w", err)
	}
	blockIndex, ok := world.ChunkBlockIndex(capturePassiveHerdDropBlock)
	if !ok {
		return fmt.Errorf("牛群掉落锚点 %+v 不在区块索引内", capturePassiveHerdDropBlock)
	}
	if err := app.ItemDrops().Apply(network.ItemDropUpserts{
		ServerTick: 2,
		Drops: []network.ItemDrop{{
			ID:         core.DropID{Dimension: core.Overworld, Chunk: capturePassiveHerdDropBlock.Chunk(), Slot: 0, Generation: 1},
			BlockIndex: blockIndex,
			Item:       core.ItemRawBeef,
			Count:      1,
		}},
	}); err != nil {
		return fmt.Errorf("装入生牛肉掉落: %w", err)
	}
	// 夹具自检：批次的 ID 升序由镜像校验兜底，这里只锁数量——少装任何一头
	// 或掉落缺席都会让 golden 缺像素，宁可当场失败也不产出静默缺员的基线。
	if got := len(app.Passives().AppendPresentations(nil)); got != len(capturePassiveHerdSpawns) {
		return fmt.Errorf("被动牛镜像数量=%d，想要 %d", got, len(capturePassiveHerdSpawns))
	}
	if got := len(app.ItemDrops().Presentations()); got != 1 {
		return fmt.Errorf("掉落物镜像数量=%d，想要 1", got)
	}
	return nil
}

// pinPassiveHerdVolatile 把掉落动画相位钉死为常量。收敛帧本身会推进权威
// tick（帧间隔、加载耗时都随机器速度变化），在 Apply 里钉死会被它们重新
// 覆盖，因此必须在收敛帧之后、最后一帧之前重钉（`PinVolatile` 语义）。
func pinPassiveHerdVolatile(app SceneApplication) error {
	app.SetServerTick(capturePassiveHerdServerTick)
	return nil
}
