package capture

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	hud "github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// captureWidth/captureHeight 是视觉场景的固定分辨率。
// 刻意远小于 benchmark 的 2560×1440：golden 图要长期入库并反复更新，
// 全尺寸会让仓库历史迅速膨胀，而 360p 足以暴露本设施要抓的问题类别。
const (
	captureWidth  = application.CaptureWidth
	captureHeight = application.CaptureHeight
)

// captureDrainMax 是抓帧期间每帧处理的服务端消息上限，取值是 app 包的
// 单一常量（`MessageDrainMax`），与 benchmark 共用同一无头帧节奏契约。
const captureDrainMax = application.MessageDrainMax

// captureGlyphSettleFrames 是 Apply 之后、真正回读之前额外渲染的帧数，
// 用来让字形图集的异步光栅化收敛。GlyphAtlas.Request 只把符文入队，
// 光栅化在后台 worker 完成，FlushUploads 每帧最多把一个结果搬上 GPU；
// 一个场景在 Apply 里第一次用到的文本（比如新出现的远端玩家昵称）如果
// 立刻回读，会读到 tofu 占位符而不是真正的字形。这里只重复渲染、不再
// drain——不会让服务端消息覆盖 Apply 设的常量——把收敛让给 worker。
// ponytail: 32 帧是 4 倍余量（每帧搬一个字形，8 个字形需要 8 帧）。
// 若后续昵称更长或多个名牌同时出现（参考 maxNameTagGlyphs），可能不够。
// 升级路径：轮询 GlyphAtlas 直到收敛，需要给它加一个导出的自省方法。
const captureGlyphSettleFrames = 32

var captureSettleTimeout = 5 * time.Minute

// captureScene 是一个视觉场景。三要素缺一不可：确定性的世界状态由固定种子、
// `WaitUntilLoaded` 与可选 Prepare 保证，固定的相机位姿与其余呈现状态由 Apply
// 设置，抓帧时机由 WarmupFrames 和收敛判据固定。任何一项随环境变化，产出的图
// 就不可比对。
//
// 潜在陷阱：app.go 把 a.serverTick 传给 itemDropRenderer.Render 作掉落物动画相位，
// 而 a.serverTick 的取值依赖 `WaitUntilLoaded` 花了多少个权威 tick 才收敛，
// 这本身取决于机器速度——不是本文件描述的三个常量之一。当前所有场景都不含
// 掉落物，因此无害；但第一个引入掉落物的场景会让基线的动画相位依赖机器速度，
// 到时需要额外想办法钉死或忽略这一相位。
type captureScene struct {
	Name string
	// WarmupFrames 是 Apply 之前空跑的帧数，用来让上传预算与网格化收敛。
	WarmupFrames int
	// Prepare 在权威消息完成最后一次 drain 后装入固定镜像夹具。
	Prepare func(SceneApplication) error
	// Apply 在最后一帧渲染前执行，是场景对呈现状态的全部干预。
	// 它跑在 drainServerMessages 之后，因此设置的值不会被当帧的服务端消息覆盖。
	Apply func(SceneApplication) error
	// Menu 可选，为真时本场景以主菜单相位呈现。菜单层已迁 WebView（client
	// ABI v12）且无头路径零参与，本场景输出无菜单 chrome 的世界底图——
	// golden 待全景场景落地时重生成。它与 Settings 互斥；每个场景都会
	// 显式重置菜单相位，避免设置页污染后续共用同一 application 的场景。
	Menu bool
	// Settings 可选，非 nil 时本场景进入正常的设置相位。它与 Menu 互斥；
	// 每个场景都会显式重置菜单相位，避免设置页污染后续共用同一 application
	// 的场景。
	Settings *application.SettingsState
	// PinVolatile 可选，在字形收敛帧之后、最后一帧渲染之前执行，用来钉住那些
	// 随机器速度变化、因而不属于场景三要素的量。
	//
	// 存在的理由：Apply 跑在收敛帧之前，而收敛帧会推进帧间隔与权威 tick，
	// 在 Apply 里设的值到最后一帧已经被覆盖。菜单全景用它钉住自转时刻，
	// 战斗场景用它把权威 marker 重新武装到 6 帧窗口的开头。
	PinVolatile func(SceneApplication) error
}

// captureContainerInventory 返回箱子和熔炉场景共用的 36 格已确认背包，刻意在
// 快捷栏与背包两段都放入物品，让两个容器画面能同时审查统一栏位与来源轮廓。
func captureContainerInventory() core.Inventory {
	inventory := core.Inventory{}
	inventory.Hotbar.Selected = 4
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 40}
	inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemChest, Count: 3}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemDirt, Count: 48}
	inventory.Backpack[2] = core.ItemStack{Item: core.ItemCoal, Count: 12}
	inventory.Backpack[3] = core.ItemStack{Item: core.ItemRawIron, Count: 8}
	inventory.Backpack[4] = core.ItemStack{Item: core.ItemIronIngot, Count: 9}
	return inventory
}

// captureSettled 判定抓帧收敛。近环半部沿用 mesher stats + pending uploads；
// lodBusy 是远环调度器的 Busy() 计数（未接线 LOD 的运行传 0）。vistaPending
// 是菜单全景管线的未完成工作量（非菜单相位恒为 0）。远环 tile 与全景区块
// 的生成与上传完全异步，若不等它们清零就回读，golden 里的远景带/全景地形
// 会随机器速度时有时无——远景像素就成了不可复现的输入，这是 5.3 把两者
// 并入收敛判据的原因。
func captureSettled(stats client.MesherStats, pending, lodBusy, vistaPending int) bool {
	return stats.DirtySections == 0 && stats.QueuedJobs == 0 &&
		stats.InFlightJobs == 0 && stats.ReadyResults == 0 && pending == 0 &&
		lodBusy == 0 && vistaPending == 0
}

// captureScenes 是表驱动的场景清单，新增场景即新增一行。
//
// 全部场景共用同一个 application，按本列表的顺序依次执行——两个场景之间不会
// 重置任何呈现状态。因此每个 Apply 都必须显式设定自己渲染依赖的全部字段，
// 不能依赖"没设置就是零值"，否则该场景的画面会悄悄继承前一个场景留下的状态，
// 而这份继承关系不会出现在任何一个场景自己的代码里——重排本列表、删掉某个
// 场景、或在两者之间插入新场景，都会静默改变后续场景的期望像素。新增场景应
// 追加在列表末尾；若确实需要调整顺序或插入位置，须用 --update-golden 重新
// 生成所有受影响场景的基线，并逐张人眼确认。
// 例外：`main-menu` 与紧随其后的 `settings-menu` 因 spec 排序约束（MUST 排在
// `far-horizon` 之前）共同插入表中部（water-surface-slope 与 far-horizon
// 之间），属 spec/brief 硬性例外；`mining-crack-early` 与 `mining-crack-heavy`
// 同为表中部插入（紧随 water-surface-slope），顺序由 visual-verification
// delta 的顺序 MUST 条款固定。
var captureScenes = []captureScene{
	{
		Name:         "terrain-noon",
		WarmupFrames: 8,
		Apply: func(app SceneApplication) error {
			// 6000 tick 是正午，日光与太阳高度都取到最大值，
			// 是昼夜管线上最容易看出偏差的相位。
			app.SetWorldTimeTicks(6000)
			// 登录首条权威 PlayerState 必然触发 ResetView，把 Yaw/Pitch 覆盖成
			// 服务端下发的出生朝向——那不是本场景声明的常量。这里显式钉死，
			// 避免相机姿态随出生朝向漂移；Pitch 取小幅度下俯以避免画面被天空占满。
			app.Camera().Yaw = 0
			app.Camera().Pitch = -0.25
			return nil
		},
	},
	{
		Name:         "avatar-nametag",
		WarmupFrames: 8,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(6000)
			app.Camera().Yaw = 0
			app.Camera().Pitch = -0.25
			// 本场景不关心物品栏，但场景表共用同一个 application，前序场景留在
			// app.Inventory() 里的物品状态不显式清空就会被悄悄继承。这里显式设成
			// 空物品栏，让本场景的画面只由自己的 Apply 决定，不依赖场景表的执行
			// 顺序。
			if err := app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
				return fmt.Errorf("重置物品栏: %w", err)
			}
			if app.RemotePlayers() == nil {
				return fmt.Errorf("avatar-nametag 需要远端玩家追踪器，当前为 nil")
			}
			// 昵称刻意混用 ASCII 与非 ASCII：字形 atlas 的分支在这两类上不同，
			// 只用 ASCII 会漏掉整条宽字符路径。
			spawn := network.RemotePlayerSpawn{
				// PlayerID{1} 不是合法 UUIDv4（第 6 字节高 4 位须为 4，第 8 字节
				// 高 2 位须为 10），applySpawn 会拒绝；这里改用与仓库测试同款的
				// 合法 UUIDv4 形状占位符。
				PlayerID:    core.PlayerID{6: 0x40, 8: 0x80, 15: 1},
				DisplayName: "测试Player",
				ServerTick:  1,
				Position:    app.Camera().Pos.Add(mgl32.Vec3{0, 0, -6}),
			}
			return app.RemotePlayers().Apply(spawn)
		},
	},
	{
		Name:         "inventory-crafting",
		WarmupFrames: 8,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(6000)
			app.Camera().Yaw = 0
			app.Camera().Pitch = -0.25
			app.RemotePlayers().Reset()
			app.Furnace().Reset()
			app.Chest().Reset()
			app.SetInventoryOpen(true)
			// 已选来源格取统一视图格 12（背包格 3）：来源轮廓落在背包区，
			// 与下一场景落在网格区的轮廓互为对照。
			app.SetInventorySource(12)

			inventory := core.Inventory{}
			inventory.Hotbar.Selected = 1
			inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
			inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemGrass, Count: 32}
			inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 40}
			inventory.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemFurnace, Count: 2}
			inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemChest, Count: 1}
			inventory.Backpack[0] = core.ItemStack{Item: core.ItemDirt, Count: 48}
			inventory.Backpack[1] = core.ItemStack{Item: core.ItemStoneBrick, Count: 16}
			inventory.Backpack[2] = core.ItemStack{Item: core.ItemCoal, Count: 12}
			inventory.Backpack[3] = core.ItemStack{Item: core.ItemRawIron, Count: 8}
			inventory.Backpack[4] = core.ItemStack{Item: core.ItemIronIngot, Count: 9}
			inventory.Backpack[5] = core.ItemStack{Item: core.ItemOakLog, Count: 1}
			inventory.Backpack[6] = core.ItemStack{Item: core.ItemGlass, Count: 4}
			inventory.Backpack[9] = core.ItemStack{Item: core.ItemIronBlock, Count: 1}
			if err := app.Inventory().Apply(network.InventoryState{Inventory: inventory}); err != nil {
				return err
			}
			// 个人 2×2 网格装入已匹配的真实原料形状（石砖 2×2）与非空产物格：
			// 产物直接取形状表的派生值，镜像注入因此与权威匹配器一致。
			recipe, ok := core.Recipe(core.RecipeStoneBricks)
			if !ok {
				return errors.New("石砖配方不存在")
			}
			personal := network.CraftingState{Size: 2, Output: recipe.Output}
			for slot := range 4 {
				personal.Slots[slot] = core.ItemStack{Item: core.ItemStone, Count: 1}
			}
			return app.Crafting().Apply(personal)
		},
	},
	{
		// workbench-crafting 是格子工作台的无窗口 capture 场景：已打开的 3×3
		// 网格装入一条水平镜像不对称配方的合法摆放（石锄：石头纵列在左、
		// 木棍纵列在右，Mirror 关闭，镜像摆放不匹配）与合法产物，覆盖统一
		// 凹槽风格与产物格。镐类配方整形镜像后与自身相同，覆盖不到镜像不
		// 对称语义，因此选石锄。场景不依赖前一场景留下的容器或网格状态——
		// 镜像经 Apply 全量覆盖（latest-wins）；golden PNG 由批次集成任务在
		// scenario 迁移时统一生成。
		Name:         "workbench-crafting",
		WarmupFrames: 8,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(6000)
			app.Camera().Yaw = 0
			app.Camera().Pitch = -0.25
			app.RemotePlayers().Reset()
			app.Furnace().Reset()
			app.Chest().Reset()
			app.SetInventoryOpen(true)
			// 已选来源格取统一视图格 1（网格格 1，石锄木棍列顶格）：来源轮廓
			// 落在网格区，与前一场景落在背包区的轮廓互为对照。
			app.SetInventorySource(1)

			inventory := core.Inventory{}
			inventory.Hotbar.Selected = 1
			inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
			inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStick, Count: 12}
			inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemOakLog, Count: 3}
			inventory.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemOakPlanks, Count: 24}
			inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemWorkbench, Count: 1}
			inventory.Backpack[0] = core.ItemStack{Item: core.ItemDirt, Count: 48}
			inventory.Backpack[2] = core.ItemStack{Item: core.ItemCoal, Count: 12}
			inventory.Backpack[3] = core.ItemStack{Item: core.ItemRawIron, Count: 8}
			inventory.Backpack[4] = core.ItemStack{Item: core.ItemIronIngot, Count: 9}
			inventory.Backpack[6] = core.ItemStack{Item: core.ItemGlass, Count: 4}
			if err := app.Inventory().Apply(network.InventoryState{Inventory: inventory}); err != nil {
				return err
			}
			// 3×3 石锄摆放：左列石头（格 0、3）旁接右列木棍（格 1、4），占据
			// 网格左上 2×2。产物直接取形状表的派生值（工具带满耐久），注入
			// 与权威匹配器一致。
			recipe, ok := core.Recipe(core.RecipeStoneHoe)
			if !ok {
				return errors.New("石锄配方不存在")
			}
			workbench := network.CraftingState{Size: 3, Output: recipe.Output}
			for _, slot := range []int{0, 3} {
				workbench.Slots[slot] = core.ItemStack{Item: core.ItemStone, Count: 1}
			}
			for _, slot := range []int{1, 4} {
				workbench.Slots[slot] = core.ItemStack{Item: core.ItemStick, Count: 1}
			}
			return app.Crafting().Apply(workbench)
		},
	},
	{
		Name:         "chest-container",
		WarmupFrames: 8,
		Apply: func(app SceneApplication) error {
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			app.SetWorldTimeTicks(6000)
			*app.Camera() = client.Camera{
				Pos: mgl32.Vec3{0, 110, 0}, Pitch: -0.25,
				FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
				Near: 0.1, Far: 2000,
			}
			app.SetCenter(application.CameraChunk(app.Camera().Pos))
			app.SetInventoryOpen(true)
			app.SetInventorySource(core.ChestFirstSlot)
			if err := app.Inventory().Apply(network.InventoryState{Inventory: captureContainerInventory()}); err != nil {
				return fmt.Errorf("装入箱子场景背包: %w", err)
			}
			state := network.ChestState{Chest: core.ContainerRef{
				Dimension: core.Overworld, Kind: core.ContainerKindChest, Slot: 2, Generation: 3,
			}}
			state.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
			state.Items[8] = core.ItemStack{Item: core.ItemCoal, Count: 17}
			state.Items[17] = core.ItemStack{Item: core.ItemRawIron, Count: 1}
			state.Items[26] = core.ItemStack{Item: core.ItemIronIngot, Count: 64}
			if err := app.Chest().Apply(state); err != nil {
				return fmt.Errorf("装入箱子场景镜像: %w", err)
			}
			return nil
		},
	},
	{
		Name:         "furnace-container",
		WarmupFrames: 8,
		Apply: func(app SceneApplication) error {
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			app.SetWorldTimeTicks(6000)
			*app.Camera() = client.Camera{
				Pos: mgl32.Vec3{0, 110, 0}, Pitch: -0.25,
				FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
				Near: 0.1, Far: 2000,
			}
			app.SetCenter(application.CameraChunk(app.Camera().Pos))
			app.SetInventoryOpen(true)
			app.SetInventorySource(core.FurnaceFuelSlot)
			if err := app.Inventory().Apply(network.InventoryState{Inventory: captureContainerInventory()}); err != nil {
				return fmt.Errorf("装入熔炉场景背包: %w", err)
			}
			if err := app.Furnace().Apply(network.FurnaceState{
				Furnace:       core.FurnaceRef{Dimension: core.Overworld, Slot: 3, Generation: 4},
				Input:         core.ItemStack{Item: core.ItemRawIron, Count: 8},
				Fuel:          core.ItemStack{Item: core.ItemCoal, Count: 12},
				Output:        core.ItemStack{Item: core.ItemIronIngot, Count: 5},
				ProgressTicks: 73,
				BurnTicks:     911,
			}); err != nil {
				return fmt.Errorf("装入熔炉场景镜像: %w", err)
			}
			return nil
		},
	},
	{
		// debug-panel 保留为正午世界视角回归景：调试面板的呈现（读数区、参数
		// 分组、可编辑行高亮）已整体迁 WebView 组件，无头路径的程序化面板渲染
		// 路径已删除——本场景的画面因此不含任何面板像素，golden 只覆盖相机所在
		// 高空的世界底图。面板可见态仍在这里装入，用于驱动面板状态机并验证
		// 无头路径不因面板可见而产生额外像素；面板自身的像素验收由前端组件断言
		// 与 frontend/visual 部件基线承接。读数随机器速度变化的量（帧时、权威
		// tick）不再需要钉住——没有任何像素消费它们。
		Name:         "debug-panel",
		WarmupFrames: 8,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(6000)
			app.Camera().Yaw = 0
			app.Camera().Pitch = -0.25
			// 与其余场景一样显式清空上一个场景留下的呈现状态：本列表共用同一个
			// application，不显式设置就会静默继承容器与网格场景（inventory/
			// workbench-crafting）的背包、容器或网格状态。
			app.RemotePlayers().Reset()
			app.Furnace().Reset()
			app.Chest().Reset()
			app.Crafting().Reset()
			app.SetInventoryOpen(false)
			if err := app.Inventory().Apply(
				network.InventoryState{Inventory: core.Inventory{}},
			); err != nil {
				return fmt.Errorf("重置物品栏: %w", err)
			}
			if app.Panel() == nil {
				return fmt.Errorf("debug-panel 需要面板状态，当前为 nil")
			}
			app.Panel().SetVisible(true)
			return nil
		},
	},
	{
		Name:         "skylight-tunnel",
		WarmupFrames: 8,
		Prepare:      prepareSkylightTunnel,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(6000)
			app.Camera().Pos = mgl32.Vec3{0.5, 2.8, 8.5}
			app.Camera().Yaw = 0
			app.Camera().Pitch = -0.04
			app.SetInventoryOpen(false)
			if app.Panel() != nil {
				app.Panel().SetVisible(false)
			}
			if err := app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
				return fmt.Errorf("重置物品栏: %w", err)
			}
			if app.RemotePlayers() == nil {
				return fmt.Errorf("skylight-tunnel 需要远端玩家追踪器，当前为 nil")
			}
			// 用快照逐一走合法 despawn，空列表自然成功，也不会遗漏其他玩家。
			for _, player := range app.RemotePlayers().Presentations() {
				if err := app.RemotePlayers().Apply(network.RemotePlayerDespawn{
					PlayerID: player.PlayerID,
				}); err != nil {
					return fmt.Errorf("清除远端玩家 %s: %w", player.PlayerID, err)
				}
			}
			return nil
		},
	},
	{
		Name:         "block-light-room",
		WarmupFrames: 8,
		Prepare:      prepareBlockLightRoom,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(18000)
			app.Camera().Pos = mgl32.Vec3{0.5, 2.8, 0.5}
			app.Camera().Yaw = 0
			app.Camera().Pitch = 0
			app.SetInventoryOpen(false)
			if app.Panel() != nil {
				app.Panel().SetVisible(false)
			}
			if err := app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
				return fmt.Errorf("重置物品栏: %w", err)
			}
			app.RemotePlayers().Reset()
			app.Furnace().Reset()
			app.Chest().Reset()
			app.Crafting().Reset()
			return nil
		},
	},
	{
		// torch-night 是火把的无窗口夜景 capture 场景：固定夜晚（18000 tick）
		// 的全封闭石室，室内唯一光源是火把的方块光（等级 14，与发光方块同
		// 一传播路径）。画面同时呈现落地形态与两面墙上的墙面形态（左右墙
		// 各一朵 ±X 形态）；近处地板被照亮、远角沿距离衰减变暗，火把本体
		// 是窄柄加暖色火芯的 cutout 精灵（边缘透明，透过可见背景）。
		//
		// 排序约束：紧随 block-light-room、先于 bed-night（bed-night 按
		// spec delta visual-verification 插入其后），由
		// TestTorchNightCaptureScenePosition 兜底。
		Name:         "torch-night",
		WarmupFrames: 8,
		Prepare:      prepareTorchNightRoom,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(18000)
			app.Camera().Pos = mgl32.Vec3{0.5, 2.8, 0.5}
			app.Camera().Yaw = 0
			app.Camera().Pitch = 0
			app.SetInventoryOpen(false)
			if app.Panel() != nil {
				app.Panel().SetVisible(false)
			}
			if err := app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
				return fmt.Errorf("重置物品栏: %w", err)
			}
			app.RemotePlayers().Reset()
			app.Furnace().Reset()
			app.Chest().Reset()
			app.Crafting().Reset()
			return nil
		},
	},
	{
		// bed-night 是床的无窗口夜景 capture 场景：固定夜晚（18000 tick）
		// 的全封闭石室内，四个水平朝向各一张完整床（床头与床尾成对、同框），
		// 室内亮度只来自三朵火把的方块光。床面层的枕头/毯沿亮带随朝向旋转、
		// 床体是 9/16 半高板——原创配色与半高轮廓在夜间光照下的可辨性由
		// golden 双阈值与场景内像素断言共同兜底。四张床刻意聚在相机近处的
		// 两排：床头亮带只有 3px 宽，离相机太远会在画面里薄于一个像素，
		// 多朝向可辨的像素证据随之消失。
		//
		// 排序约束：紧随 torch-night、先于 ai-companion（spec delta
		// visual-verification「bed-night 无窗口夜景场景」），由
		// TestBedNightCaptureScenePosition 兜底。床与火把夹具经 Prepare 装
		// 进客户端镜像，下一场景 materials-showcase 的 Prepare 从空气基线
		// 重装，夹具值不会泄入后续场景。
		Name:         "bed-night",
		WarmupFrames: 8,
		Prepare:      prepareBedNightRoom,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(18000)
			// 相机抬高并下俯：床面层（枕头/毯沿亮带所在的面）是水平顶面，
			// 平视只能看到一条窄边，下俯才能把四个朝向的床面同时铺进画面；
			// 姿态是常量，与登录 ResetView 的出生朝向无关。
			app.Camera().Pos = mgl32.Vec3{0.5, 3.4, 0.5}
			app.Camera().Yaw = 0
			app.Camera().Pitch = -0.3
			app.SetCenter(application.CameraChunk(app.Camera().Pos))
			app.SetInventoryOpen(false)
			if app.Panel() != nil {
				app.Panel().SetVisible(false)
			}
			if err := app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
				return fmt.Errorf("重置物品栏: %w", err)
			}
			app.RemotePlayers().Reset()
			app.Furnace().Reset()
			app.Chest().Reset()
			app.Crafting().Reset()
			return nil
		},
	},
	{
		Name:         "materials-showcase",
		WarmupFrames: 8,
		Prepare:      prepareMaterialsShowcase,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(6000)
			app.Camera().Pos = mgl32.Vec3{0.5, 5.8, 13.5}
			app.Camera().Yaw = 0
			app.Camera().Pitch = -0.12
			app.SetInventoryOpen(false)
			app.RemotePlayers().Reset()
			app.Furnace().Reset()
			app.Chest().Reset()
			app.Crafting().Reset()
			if app.Panel() != nil {
				app.Panel().SetVisible(false)
			}
			return app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}})
		},
	},
	{
		Name:         "target-block-feedback",
		WarmupFrames: 8,
		Prepare:      prepareTargetBlockFeedback,
		Apply: func(app SceneApplication) error {
			app.SetWorldTimeTicks(6000)
			app.Camera().Pos = mgl32.Vec3{0.5, 3.5, 2.5}
			app.Camera().Yaw, app.Camera().Pitch = 0, 0
			app.SetInventoryOpen(false)
			app.SetInventorySource(-1)
			app.RemotePlayers().Reset()
			app.Furnace().Reset()
			app.Chest().Reset()
			app.Crafting().Reset()
			if app.Panel() != nil {
				app.Panel().SetVisible(false)
			}
			return app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}})
		},
	},
	{
		// grass-closeup 是短草的无窗口近景 capture 场景：空气邻域基线上的
		// 一条草地支撑条，条上 3 列手工短草（短草正下方必是草地，与世界
		// 生成的不变式一致）。固定正午、固定近景机位，短草交叉面片的像素
		// 覆盖由 golden 双阈值兜底。排序：紧随 target-block-feedback、
		// 先于 oak-grove。
		Name:         "grass-closeup",
		WarmupFrames: 8,
		Prepare:      prepareGrassCloseup,
		Apply:        applyGrassCloseupCaptureState,
	},
	{
		Name:         "oak-grove",
		WarmupFrames: 8,
		Prepare:      prepareOakGrove,
		Apply:        applyOakGroveCaptureState,
	},
	{
		Name:         "ai-companion",
		WarmupFrames: 8,
		Prepare:      prepareAICompanion,
		Apply:        applyAICompanionCaptureState,
	},
	{
		Name:         "sword-combat",
		WarmupFrames: 8,
		Prepare:      prepareSwordCombat,
		Apply:        applySwordCombatCaptureState,
		PinVolatile:  pinSwordCombatVolatile,
	},
	{
		// hostile-mob 是夜行者的无窗口夜景 capture 场景：固定夜晚（18000
		// tick）的开阔草地，三朵落地火把构成亮池，8 只夜行者经客户端镜像
		// 夹具固定在火把边缘——其中一只处于受击状态（生命 13、位置后撤）、
		// 一只处于追逐中（向相机推进并正对观察者）；夜行者绝不产生名称
		// 标签。夹具与注入细节见 capture_hostile_mob.go。
		//
		// 排序约束：本场景 MUST 位于 sword-combat 之后、water-surface-slope
		// 之前，完整相邻链为 ai-companion、sword-combat、hostile-mob、
		// water-surface-slope（spec visual-verification「场景表顺序与导出」），由
		// TestHostileMobCaptureScenePosition 兜底；far-horizon 仍为倒数第二、
		// water-underwater 仍为唯一末场景。
		Name:         "hostile-mob",
		WarmupFrames: 8,
		Prepare:      prepareHostileMobNight,
		Apply:        applyHostileMobCaptureState,
	},
	{
		// 水景一：水面之上俯瞰。覆盖「水面斜坡」与「水面之下的地形」——
		// 顶层水沿 +Z（由池深处向岸边）由源方块递减到 7 级，角高度插值成连续斜面；
		// 池底的沙丘、砾石带与露出水面的圆石堆透过水面可见。
		Name:         "water-surface-slope",
		WarmupFrames: 8,
		Prepare:      prepareWaterBasin,
		Apply: func(app SceneApplication) error {
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			app.SetWorldTimeTicks(6000)
			// 3/4 角俯视：斜水面的梯度沿 z 轴，从侧后方看过去，水面高度差
			// 在画面里是一条明显倾斜的水线，正视（yaw=0）反而只能看到它退远。
			app.Camera().Pos = mgl32.Vec3{7.5, 6.4, 4.5}
			app.Camera().Yaw = 0.6
			app.Camera().Pitch = -0.3
			app.SetCenter(application.CameraChunk(app.Camera().Pos))
			return nil
		},
	},
	{
		// mining-crack-early 是采掘裂纹浅阶段的无窗口 capture 场景：复用
		// target-block-feedback 的固定世界（空气邻域中相机正前方 4.5..5.5 格、
		// 命中面 4.5 格的单块砖），权威采掘镜像钉在 6/30——按 BlockCrackStage
		// 公式映射为阶段 2 的浅裂纹。采掘镜像经 SetMiningOverlay 直装，场景
		// 结束后恢复（与 hud-survival-feedback 同一 defer 语义）；场景不含
		// 随机器速度变化的读数，位姿在 Apply 钉死，收敛帧内不再
		// drain，输出无需 PinVolatile 即确定。
		//
		// 排序约束：紧随 water-surface-slope、先于 main-menu（后者 Apply 自带
		// resetCapturePresentation，不继承本场景的呈现状态），由
		// TestCaptureSceneOrderAndAICompanionDeterminism 与裂纹夹具测试兜底。
		Name:         "mining-crack-early",
		WarmupFrames: 8,
		Prepare:      prepareTargetBlockFeedback,
		Apply: func(app SceneApplication) error {
			if err := applyMiningCrackCaptureState(app); err != nil {
				return err
			}
			// 采掘镜像经 SetMiningOverlay 直装：Target/HasTarget/进度二元组
			// 驱动世界裂纹（屏幕采掘条已退役，进度不再进入 hud 分节）。
			app.SetMiningOverlay(hud.MiningOverlay{
				Active: true, HasTarget: true,
				Target:        captureMiningCrackTarget,
				ProgressTicks: 6, RequiredTicks: 30,
			})
			return nil
		},
	},
	{
		// mining-crack-heavy 是采掘裂纹重阶段的无窗口 capture 场景：与
		// mining-crack-early 同一世界、相机与呈现链路，仅把权威进度钉到
		// 29/30——阶段 9 的最重裂纹，且刻意只到 required-1，不呈现已破坏
		// 方块的裂纹。两张基线同框对比即可判读裂纹随权威进度的离散加深。
		Name:         "mining-crack-heavy",
		WarmupFrames: 8,
		Prepare:      prepareTargetBlockFeedback,
		Apply: func(app SceneApplication) error {
			if err := applyMiningCrackCaptureState(app); err != nil {
				return err
			}
			app.SetMiningOverlay(hud.MiningOverlay{
				Active: true, HasTarget: true,
				Target:        captureMiningCrackTarget,
				ProgressTicks: 29, RequiredTicks: 30,
			})
			return nil
		},
	},
	{
		// main-menu 是主菜单相位的无窗口 capture 场景：底图由 menu-vista
		// 全景路径产出（与交互主菜单同一渲染路径——固定种子 worldgen 区块、
		// 专属镜像/mesher/远环带、正午固定世界时间与整数 tick 自转相机），
		// 菜单 chrome 由 WebView 呈现且无头零参与。菜单相位抑制准星、弹条
		// 与生存 HUD，画面是纯全景。
		//
		// Apply 里 resetCapturePresentation 清空前序场景（water-surface-slope）
		// 留下的全部共享呈现状态。相机与世界内容不再由本场景摆拍：全景接管；
		// a.center 保持出生点，后继世界场景（far-horizon）的近环收敛域不受
		// 全景锚点影响。
		//
		// 排序约束：本场景 MUST 排在 far-horizon 之前（far-horizon 仍为倒数
		// 第二、water-underwater 仍为最后），由 TestMainMenuCaptureScenePosition
		// 兜底。
		Name:         "main-menu",
		WarmupFrames: 8,
		Menu:         true,
		Apply: func(app SceneApplication) error {
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			// 全景相位钉死正午；这里同步钉一份权威世界时间，防回归时把
			// 相位门控改坏后画面悄悄变成黎明。
			app.SetWorldTimeTicks(6000)
			app.SetCenter(application.CameraChunk(mgl32.Vec3{0, 110, 0}))
			return nil
		},
		PinVolatile: func(app SceneApplication) error {
			// 收敛帧数随机器速度波动，自转 tick 在收敛后钉死：最终帧的相机
			// 姿态是纯 tick 的确定函数（spec webview-menu-ui「全景背景
			// 确定性」）。
			app.SetMenuVistaTick(captureMenuVistaTickMainMenu)
			return nil
		},
	},
	{
		// settings-menu 与 main-menu 共用同一份全景世界（同一个 application，
		// 全景惰性构建一次、两场景复用），仅以不同的自转时刻区分两张底图；
		// 设置夹具使用一组已保存的非默认值（草稿与已保存一致、不脏），因此
		// 三个控件都有明确选择，同时保持 clean/空状态。
		Name:         "settings-menu",
		WarmupFrames: 8,
		Settings: &application.SettingsState{
			Committed: application.SettingsValues{
				AudioVolume:     0.25,
				TexturePackPath: "packs/local",
				WindowSize:      config.WindowSize960x540,
			},
			Draft: application.SettingsValues{
				AudioVolume:     0.25,
				TexturePackPath: "packs/local",
				WindowSize:      config.WindowSize960x540,
			},
		},
		Apply: func(app SceneApplication) error {
			if err := resetCapturePresentation(app); err != nil {
				return err
			}
			app.SetWorldTimeTicks(6000)
			app.SetCenter(application.CameraChunk(mgl32.Vec3{0, 110, 0}))
			return nil
		},
		PinVolatile: func(app SceneApplication) error {
			app.SetMenuVistaTick(captureMenuVistaTickSettingsMenu)
			return nil
		},
	},
	{
		// far-horizon 是远环 LOD 的长期视觉门禁(spec delta「MUST 新增
		// far-horizon 视觉场景」):相机钉在近环边缘 -z 内侧的高空,朝
		// 地平线观察,单帧同时覆盖近景地形(画面底部)、远环壳带(地平线
		// 下侧到全雾)、雾过渡带(0.5×far..0.75×far,默认几何 768..1152)
		// 与天空(地平线上侧)。
		//
		// 排序协调(变基):water-underwater 的「MUST 排在场景表最后」由
		// fluid 规格固定,故本场景插在其之前(倒数第二);它与前一场景的
		// 呈现状态互相独立,不依赖场景表顺序。
		Name:         "far-horizon",
		WarmupFrames: 8,
		Apply:        applyFarHorizonCaptureState,
	},
	{
		// 水景二：水下视角。覆盖「水下视角」——相机与权威位置一起放进水体，
		// 眼睛浸没标志因此为真，画面同时呈现水色叠加、被压低的可见半径、
		// 天空光穿水衰减，以及未满的氧气条。
		//
		// Prepare 重新装一遍夹具而不是继承上一个场景：镜像的 ChunkSnapshot 会把
		// 区块 revision 复位，因此重装是幂等的，本场景的画面不依赖场景表的顺序。
		//
		// **本场景 MUST 排在场景表最后**，由 TestWaterUnderwaterCaptureSceneIsLast
		// 兜底。它注入的权威 PlayerState 带一个远大于真实值的 ServerTick，此后
		// 一切真实 PlayerState 都会被预测器的单调校验静默忽略，浸没标志因此永久
		// 停在"在水里"；排在它后面的任何场景都会带着水色叠加与被压低的可见半径
		// 出图。这不是假设——把它插在 ai-companion 之前实测过一次，ai-companion
		// 的画面 98.75% 的像素随之改变。
		Name:         "water-underwater",
		WarmupFrames: 8,
		Prepare:      prepareWaterBasin,
		Apply:        applyWaterUnderwaterCaptureState,
	},
}

// 两个菜单场景钉住的全景自转时刻（单位：整数 tick，均在
// `application.MenuVistaYawPeriodTicks` 一个周期内）：主菜单取 1/8 周期
// （45°），设置页取 3/8 周期（135°）。同一份全景世界因此呈现两个可区分
// 的相机时刻，两张底图不是同一张图的两份拷贝；取值与场景闭包共用本组
// 常量，由 TestMenuCaptureScenesPinDistinctVistaTicks 钉住互异与界内。
const (
	captureMenuVistaTickMainMenu     = application.MenuVistaYawPeriodTicks / 8
	captureMenuVistaTickSettingsMenu = application.MenuVistaYawPeriodTicks / 8 * 3
)

// RunCapture 依次跑完全部视觉场景。updateGolden 为真时把抓到的图写进 golden 基线；
// 为假时与已有基线比对，超阈值的场景把实拍图与差异图写进 dir 并返回错误。
func RunCapture(app SceneApplication, dir string, updateGolden bool) error {
	if err := prepareCaptureApplication(app); err != nil {
		return err
	}
	// 场景 Apply 用 SetWorldTimeTicks 钉住的昼夜值必须在收敛帧期间保持:
	// 权威状态里的服务端时间随真实时间前进,不冻结的话最终帧的天空光随
	// 进程启动漂移,逐像素 golden 门禁在天空光渗入的画面上整片翻色。
	// 直写不受冻结影响,各场景仍可换钉自己的值。
	app.SetWorldTimeFrozen(true)
	defer app.SetWorldTimeFrozen(false)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建抓帧输出目录 %s: %w", dir, err)
	}
	// 用 errors.Join 累积每个场景的错误并跑完全部场景，而不是遇错即停：
	// 着色器或格式类改动通常会让多个场景同时变红，只看到第一个红的场景
	// 会漏掉其余场景的信息，也漏跑它们各自的图像产出。
	var errs []error
	for _, scene := range captureScenes {
		if err := captureOne(app, dir, scene, updateGolden); err != nil {
			errs = append(errs, fmt.Errorf("场景 %s: %w", scene.Name, err))
		}
	}
	return errors.Join(errs...)
}

func prepareCaptureApplication(app SceneApplication) error {
	if err := validateCaptureApplication(app); err != nil {
		return err
	}
	// 加载等待与 benchmark 域共用 app 包的同一套判据（`WaitUntilLoaded` 函数族）：
	// 同样的视距、同样的收敛判据。抓帧不另设视距，否则图里所见与真实客户端
	// 所见就会分歧，golden 随之失去意义。
	if _, err := application.WaitUntilLoaded(app, 5*time.Minute); err != nil {
		return fmt.Errorf("固定场景加载: %w", err)
	}
	return nil
}

// validateCaptureApplication 检查无头 capture 的固定 framebuffer 契约。单
// application 与 LOD on/off control 都必须在开始消费服务端快照前通过它。
func validateCaptureApplication(app SceneApplication) error {
	if width, height := app.FramebufferSize(); width != captureWidth || height != captureHeight {
		return fmt.Errorf("capture framebuffer=%dx%d，要求精确 %dx%d",
			width, height, captureWidth, captureHeight)
	}
	if app.Window() != nil {
		return fmt.Errorf("capture 需要无头 offscreen 渲染器,当前为窗口模式")
	}
	return nil
}

// prepareGoldenUpdateControls 在同一 goroutine 中交错推进两个 control 的
// 初始加载。二者在构造后 Host 已开始发送完整快照，故不能先完整加载其中
// 一个；否则另一侧的 bounded receiver 会在闲置时溢出。交错仅调用现有帧
// 路径，不并发使用任何 renderer。
func prepareGoldenUpdateControls(lodOn, lodOff SceneApplication) error {
	for _, control := range []struct {
		name string
		app  SceneApplication
	}{
		{name: "LOD-on", app: lodOn},
		{name: "LOD-off", app: lodOff},
	} {
		if err := validateCaptureApplication(control.app); err != nil {
			return fmt.Errorf("%s control: %w", control.name, err)
		}
	}
	if err := application.WaitUntilLoadedPair(lodOn, lodOff, 5*time.Minute); err != nil {
		return fmt.Errorf("固定场景加载: %w", err)
	}
	return nil
}

func captureOne(app SceneApplication, dir string, scene captureScene, updateGolden bool) error {
	img, err := captureSceneImage(app, scene)
	if err != nil {
		return err
	}
	// 无条件把场景图写进 dir——不管比对通不通过、要不要更新基线。
	// spec 要求 dir 里必须为每个场景产出一份与场景名同名的图像文件；
	// 之前只在比对失败或更新基线时才写，比对通过的正常路径反而拿不到图看。
	if err := writePNG(filepath.Join(dir, scene.Name+".png"), img); err != nil {
		return fmt.Errorf("写出场景图 %s: %w", scene.Name, err)
	}
	if updateGolden {
		goldenPath := filepath.Join(captureGoldenDir, scene.Name+".png")
		if err := os.MkdirAll(captureGoldenDir, 0o755); err != nil {
			return fmt.Errorf("创建 golden 基线目录 %s: %w", captureGoldenDir, err)
		}
		if err := writePNG(goldenPath, img); err != nil {
			return err
		}
		fmt.Printf("已抓取场景 %s(写入基线)\n", scene.Name)
		return nil
	}
	diff, err := compareAgainstGolden(captureGoldenDir, dir, scene.Name, img, captureThresholds)
	fmt.Printf("已抓取场景 %s: %s\n", scene.Name, diff)
	return err
}

// `captureSceneImage` 只完成既有场景的预热、状态装入、收敛和回读，不写文件。
// update control 与正式 `captureOne` 共用它，保证两条路径没有第二套场景渲染逻辑。
func captureSceneImage(app SceneApplication, scene captureScene) (*image.NRGBA, error) {
	for i := 0; i < scene.WarmupFrames; i++ {
		if _, err := app.Frame(captureDrainMax, captureDrainMax, physics.FixedDelta); err != nil {
			return nil, fmt.Errorf("预热第 %d 帧: %w", i, err)
		}
	}
	// 最后一帧手工拆开 frame()：先收消息，再装入夹具并覆盖呈现状态，最后渲染。
	// 顺序不能变；从 Prepare 开始不再 drain，固定夹具不会被权威消息覆盖。
	app.DrainServerMessages(captureDrainMax)
	if scene.Prepare != nil {
		if err := scene.Prepare(app); err != nil {
			return nil, fmt.Errorf("准备场景夹具: %w", err)
		}
	}
	if err := scene.Apply(app); err != nil {
		return nil, fmt.Errorf("应用场景状态: %w", err)
	}
	// 菜单相位在 Apply 之后、收敛循环之前装入。无条件重置相位是刻意而为：
	// 场景共用同一个 application，若不显式清除，上一场景的主菜单或设置页
	// 会静默留在后续画面上。多数场景恢复 game 相位;菜单相位抑制准星与弹条
	// (无头路径 WebView 零参与,不产生任何桥推送)。
	if scene.Menu && scene.Settings != nil {
		return nil, fmt.Errorf("场景 %s 同时设置 Menu 与 Settings", scene.Name)
	}
	switch {
	case scene.Settings != nil:
		app.SetMenuPhase(application.MenuPhaseSettings)
		app.SetSettings(*scene.Settings)
	case scene.Menu:
		app.SetMenuPhase(application.MenuPhaseMenu)
	default:
		app.SetMenuPhase(application.MenuPhaseGame)
	}
	settleDeadline := time.Now().Add(captureSettleTimeout)
	for i := 0; ; i++ {
		if _, err := app.RenderFrame(captureDrainMax); err != nil {
			return nil, fmt.Errorf("场景收敛第 %d 帧: %w", i, err)
		}
		stats, pending := app.Mesher().Stats(), app.Scheduler().PendingUploads()
		// 远环收敛判据与近环同源：pending==0 且 worker 空闲（Busy 归零）。
		// 禁用路径 lodScheduler 为 nil，传 0 即与旧语义一致。全景 pending
		// 只在菜单相位场景非零（世界内容与远环带由全景管线异步装配）。
		lodBusy := 0
		if app.LODScheduler() != nil {
			lodBusy = app.LODScheduler().Busy()
		}
		vistaPending := app.MenuVistaPending()
		if i+1 >= captureGlyphSettleFrames && captureSettled(stats, pending, lodBusy, vistaPending) {
			break
		}
		if time.Now().After(settleDeadline) {
			return nil, fmt.Errorf("场景 %s 在 %s 内未收敛：mesher=%+v pending=%d lodBusy=%d vistaPending=%d",
				scene.Name, captureSettleTimeout, stats, pending, lodBusy, vistaPending)
		}
	}
	// PinVolatile 必须在收敛帧之后、最后一帧之前：收敛帧本身会推进那些随机器
	// 速度变化的量（帧间隔、权威 tick），在 Apply 里钉死会被它们重新覆盖。
	if scene.PinVolatile != nil {
		if err := scene.PinVolatile(app); err != nil {
			return nil, fmt.Errorf("钉住易变读数: %w", err)
		}
	}
	// 菜单全景场景的相机自转在收敛帧里持续推进：渲染器的高精度遮挡剔除
	// （HiZ）以「相机与上一帧一致」为启用前提，最终帧的剔除态因此会随
	// 收敛帧数的奇偶巧合漂移。这里先渲染一帧钉住位姿的预热帧（HiZ 用同
	// 位姿深度图重建），再重钉一次并渲染真正回读的最终帧——最终帧的相机
	// 稳定态与 HiZ 内容都与收敛历史无关，两次抓帧逐字节一致。
	if scene.Menu || scene.Settings != nil {
		if _, err := app.RenderFrame(captureDrainMax); err != nil {
			return nil, fmt.Errorf("全景位姿预热帧: %w", err)
		}
		if scene.PinVolatile != nil {
			if err := scene.PinVolatile(app); err != nil {
				return nil, fmt.Errorf("重钉易变读数: %w", err)
			}
		}
	}
	if _, err := app.RenderFrame(captureDrainMax); err != nil {
		return nil, fmt.Errorf("渲染抓帧: %w", err)
	}
	pixels := app.Renderer().Readback()
	return bgraToNRGBA(pixels, captureWidth, captureHeight), nil
}

// `RunGoldenUpdateControl` 在两个 disposable application 上只抓取 far-horizon，
// 并在调用方可能写入任一 golden 前完成当前 LOD on/off 帧的近环比较。
func RunGoldenUpdateControl(lodOn, lodOff SceneApplication, dir string) error {
	if err := prepareGoldenUpdateControls(lodOn, lodOff); err != nil {
		return err
	}
	return runGoldenUpdateControlWithCapture(
		lodOn, lodOff, dir,
		func(app SceneApplication, scene captureScene) (*image.NRGBA, error) {
			return captureSceneImage(app, scene)
		},
	)
}

// `runGoldenUpdateControlWithCapture` 保留最小的抓帧 seam，让测试用合成当前帧
// 覆盖 fail-closed guard；生产调用仍由 `RunGoldenUpdateControl` 走真实完整链路。
func runGoldenUpdateControlWithCapture(
	lodOn, lodOff SceneApplication,
	dir string,
	capture func(SceneApplication, captureScene) (*image.NRGBA, error),
) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建抓帧输出目录 %s: %w", dir, err)
	}
	var farHorizon *captureScene
	for index := range captureScenes {
		if captureScenes[index].Name == "far-horizon" {
			farHorizon = &captureScenes[index]
			break
		}
	}
	if farHorizon == nil {
		return errors.New("视觉基线近环 control 缺少 far-horizon 场景")
	}
	lodOnFrame, err := capture(lodOn, *farHorizon)
	if err != nil {
		return fmt.Errorf("抓取 LOD-on control: %w", err)
	}
	if err := writePNG(filepath.Join(dir, "far-horizon-lod-on-control.png"), lodOnFrame); err != nil {
		return fmt.Errorf("写出 LOD-on control: %w", err)
	}
	lodOffFrame, err := capture(lodOff, *farHorizon)
	if err != nil {
		return fmt.Errorf("抓取 LOD-off control: %w", err)
	}
	if err := writePNG(filepath.Join(dir, "far-horizon-lod-off-control.png"), lodOffFrame); err != nil {
		return fmt.Errorf("写出 LOD-off control: %w", err)
	}
	guard := newNearBandGuard(
		*lodOn.Camera(), lodOn.LODTileCenter(),
		application.LodNearTileRadius(lodOn.Render().ViewDistance),
		application.LodFarTileRadius(lodOn.Render().ViewDistance, lodOn.Render().LodFarMultiplier),
		lodOn.LODScheduler() != nil,
	)
	if err := guard.assertUnchanged(farHorizon.Name, lodOffFrame, lodOnFrame); err != nil {
		return err
	}
	fmt.Println("LOD on/off 近环 control 已执行并通过")
	return nil
}
