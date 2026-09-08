package capture

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// captureSaplingGrowthSeed 是运行时树形几何的世界种子。取与抓帧世界默认种子
// 同值的 42 只是沿用既有场景的取值习惯：`worldgen.TreeBlocks` 的几何在世界
// 生成的候选格网格之外由独立冻结 salt 从 (种子, 根坐标) 派生，与种子 42 的
// 世界生成结果无关，换任何种子都只改变树形参数。
const captureSaplingGrowthSeed int64 = 42

// captureSaplingGrowthRoot 是运行时橡树的根格（树干底）。取在草地方块之上，
// 与服务端生长语义一致：`advanceSaplingCell` 把树苗所在格当作根坐标，树干底
// 替换的正是树苗那一格，因此成树的树干底比地面高一格。
var captureSaplingGrowthRoot = core.BlockPos{X: 4, Y: 1, Z: -11}

// captureSaplingGrowthSapling 是手工摆放的树苗格。正下方是草方块——树苗与
// 短草同属无碰撞植物，只能立在泥土或草地上，摆空即与放置契约不符。
var captureSaplingGrowthSapling = core.BlockPos{X: -1, Y: 1, Z: -1}

// prepareSaplingGrowth 装入树苗与橡树的确定性夹具：空气邻域基线上的一片
// 草地支撑面（y=0 草、y=-1 石），面上立一株树苗，另一侧立一棵由运行时
// 树形几何长成的橡树。
//
// 橡树刻意不手写方块表：`worldgen.TreeBlocks` 是树形的唯一真源，夹具逐条
// 展开它的记录，本场景因此同时把新 ABI 放进真实客户端进程里跑一遍——几何
// 一旦与 engine 脱钩或记录越界，抓帧会在 Prepare 阶段直接失败，而不是悄悄
// 画出一棵手工树。树形参数（高度、蓬松）由 (种子, 根坐标) 派生且冻结，
// 同一份夹具在任何机器上产出同一组方块。
//
// 支撑面刻意铺到区块窗口的边界之外：抓帧世界在 y=0 以下是实体地形，窗口内
// 不铺地相机就悬在空腔里、画面下缘会露出窗口边界；铺满后窗口自身的地形成为
// 画面主体，与既有近景场景（短草近景、水桶池塘）同口径。窗口为区块 -1..1，
// 即 x/z ∈ [-16,31]，支撑面 x=-5..9、z=-14..4 完全落在窗口内。
func prepareSaplingGrowth(app SceneApplication) error {
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
	for z := int32(-14); z <= 4; z++ {
		for x := int32(-5); x <= 9; x++ {
			setBlock(core.BlockPos{X: x, Y: -1, Z: z}, core.StoneID)
			setBlock(core.BlockPos{X: x, Y: 0, Z: z}, core.GrassID)
		}
	}
	setBlock(captureSaplingGrowthSapling, core.SaplingID)
	for _, record := range worldgen.TreeBlocks(captureSaplingGrowthSeed, captureSaplingGrowthRoot) {
		setBlock(core.BlockPos{
			X: captureSaplingGrowthRoot.X + int32(record.DX),
			Y: captureSaplingGrowthRoot.Y + int32(record.DY),
			Z: captureSaplingGrowthRoot.Z + int32(record.DZ),
		}, record.Block)
	}
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "树苗生长")
}

// applySaplingGrowthCaptureState 钉死树苗生长场景的全部呈现状态：固定正午、
// 固定机位（同时框住近处树苗与远处橡树），前序场景留下的共享呈现状态经公共
// 清场统一清空，不依赖场景表顺序。
//
// 机位取景依据：相机在 (0.5,2.2,3.5) 平视略俯，树苗（x=-1）在画面左侧约
// 4.6 格外、占约 56px 高，橡树（x=4）在右侧约 14.5 格外、整树约 146px 高且
// 树顶离画面上缘留有余量。中心视线落在地面三十余格之外，超出方块交互的目标
// 距离，因此画面里不会出现命中高亮框与方块名牌——本场景只验收树苗与橡树的
// 呈现，不掺入交互反馈像素。
func applySaplingGrowthCaptureState(app SceneApplication) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{0.5, 2.2, 3.5}, Yaw: 0, Pitch: -0.06,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	return nil
}
