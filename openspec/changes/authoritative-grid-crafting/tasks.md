## 1. 形状注册表与工作台方块（`internal/core`、`internal/assets`）

- [x] 1.1 在 `internal/core/recipe_test.go` 先写失败测试：裁边归一化（同一形状四角等价、外围空行列忽略、内部空洞保留）、仅水平镜像、垂直翻转失败、额外物品失败、空网格无匹配、2×2 不匹配 3×3 配方；随后在 `internal/core/recipe.go` 实现 `RecipePattern`、`MatchCraftingGrid` 与私有 `trimPattern`/`matchesPattern`（固定 9 格循环、无 map/slice 分配）。验证：`go test ./internal/core -run 'Test(RecipePattern|MatchCraftingGrid)' -count=1` 由红转绿。
- [x] 1.2 在 `internal/core/recipe_test.go` 写 18 项形状表中的 `1..13` 失败测试（含木棍 `12`、工作台 `13` 与「查询 `14..18` 稳定拒绝」），在 `internal/core/recipe.go` 把 recipe `1..11` 改为形状语义并追加 `12..13`，`Recipe` 返回 `RecipePattern`；同步删除 `CraftingRecipe` 聚合类型与 `Inventory.Craft` 并修正全部调用测试（`internal/core/inventory_test.go`、`item_test.go`），不留双重合成路径。验证：`go test ./internal/core -race -count=1`。
- [x] 1.3 在 `internal/core/inventory.go`/`inventory_test.go` 写并实现网格原子消费 `ConsumeRecipe`：对每个非空 pattern cell 恰减 1、耐久物品不参与材料、失败返回原 grid、输出不直接进入背包。验证：`go test ./internal/core -run 'TestConsumeRecipe' -count=1`。
- [x] 1.4 在 `internal/core/item.go`/`item_test.go` 与 `internal/assets/blocks.go`/`blocks_test.go` 追加 `ItemStick=37`、`ItemWorkbench=38`（可放置、采掘掉回 1 个、完整立方体碰撞、opaque、emission 0、普通 cube model、原创程序化木质顶/侧/底纹理），`internal/physics/collision_test.go` 锁定完整立方体碰撞。验证：`go test ./internal/core ./internal/assets ./internal/physics -race -count=1`；`gofmt -w internal/core internal/assets`。

## 2. 玩家瞬态网格与回收不变量（`internal/sim`）

- [x] 2.1 新建 `internal/sim/crafting_test.go` 先写失败测试：grid↔inventory、grid↔grid 移动语义（同物品按栈上限合并、不同物品不合并、空源/同格拒绝）、统一视图两端都在背包区拒绝、个人格 `4..8` 拒绝、产物只由当前匹配派生、一次取出恰消费一次并把完整产物入背包、取出容量预演失败逐格不变；随后新建 `internal/sim/crafting.go` 实现 `moveCraftingStack` 与 `takeCraftingOutput`（复用 `ItemStack.Valid`、`ItemStackLimit`、`Inventory.Slot`/`SetSlot`，局部副本试算后写回并置 dirty 标志），接入 `internal/sim/command.go` 与 `engine_step.go`。验证：`go test ./internal/sim -run 'TestCrafting' -count=1` 由红转绿。
- [x] 2.2 在 `internal/sim/crafting_test.go` 写回收可打包失败测试（36 格接近满载、grid 同类可并栈、grid 需要空格、output 恰可放与少一格不可放），实现 `canRepackCrafting` 与 `tryAddPreservingCrafting`，并把产物取出、掉落物拾取、采掘掉落、作物多掉落与初始材料包全部收口到该入口（`internal/sim/player.go`、`drop.go`、`container.go`）；破坏不变量的拾取保留世界掉落、破坏不变量的主动移动拒绝。验证：`go test ./internal/sim -run 'TestCraftingRepack' -count=1`。
- [x] 2.3 在 `internal/sim/crafting_test.go`、`death_test.go`、`drop_test.go` 写生命周期失败测试并实现：关闭/离开距离/工作台被挖先回收格 `4..8` 再降尺寸（无法回收拒绝关闭）；断线持久化前与死亡清空前无损回收全部 9 格（`internal/sim/persistence.go`、`death.go`）。验证：`go test ./internal/sim -race -count=1`；`gofmt -w internal/sim`。
- [x] 2.4 在 `internal/sim/container.go`（`openContainer`）扩展识别 `WorkbenchID`：复用权威 raycast/触及距离/loaded chunk 校验，设置网格尺寸 3 与命中位置；不占用 `world.ContainerRef` 或区块槽位；玩家离开触及距离或工作台被挖时回到尺寸 2。同时在 `internal/sim/mining.go` 的 `miningRule` 为 `WorkbenchID` 补采掘分支（木质 tier，与 `OakPlanksID` 同价 15 tick）并配测试（SPEC 评审建议 2）。验证：`go test ./internal/sim -run 'TestWorkbench' -count=1`。

## 3. 协议消息、服务端接线与客户端镜像（`internal/network`、`internal/server`、`internal/client`）

- [x] 3.1 在 `internal/network/codec_inventory_test.go`、`registry_test.go` 先写失败测试：`MoveCraftingStack` 值域（统一格 `0..44`、`From≠To`、至少一端 <9、个人扩展格拒绝——落 sim 权威层，网络层不知网格尺寸不重复校验，见 ledger 裁决）、`TakeCraftingOutput` 非零 Sequence、`CraftingState`（Size 仅 2/3、Size=2 时格 `4..8` 空、全部栈与 Output 合法、固定 9 格编码、拒绝截断/尾随/未知 size）；随后在 `internal/network/message_inventory.go`、`codec_client.go`、`codec_server.go`、`packet.go` 实现三条消息（临时编号 C→S 14/15、S→C 21，版本保持 v26），完成 registry、Validate 与 Memory/TCP round trip，并在 `codec_fuzz_test.go` 追加 seed。验证：`go test ./internal/network -race -count=1`。
- [x] 3.2 在 `internal/server/session_ingress.go` 把两条 C→S 消息映射到 sim 命令、`CraftRecipe` 继续映射既有命令 kind、由 sim 稳定拒绝（观察等价于 D6，见 ledger 裁决）；在 `internal/server/publication.go` 只在网格 dirty、尺寸切换或产物变化时向所属 session 发完整 `CraftingState`（不广播）；`transport_parity_integration_test.go` 追加网格命令序列的 Memory/TCP 逐字段一致测试。验证：`go test ./internal/server -race -count=1`。
- [x] 3.3 在 `internal/client/inventory.go`/`inventory_mirror_test.go` 保存最后合法完整网格状态（latest-wins、断线清空、不预测），测试同一命令序列经 Memory 与 TCP 得到相同 grid、output、inventory 与拒绝结果。验证：`go test ./internal/client -race -count=1`；`gofmt -w internal/network internal/server internal/client`。

## 4. HUD 容器界面与输入路径（`internal/render/hud`、`cmd/mornlea`）

- [x] 4.1 在 `internal/render/hud/container_test.go` 先写布局/命中失败测试：个人面板恰好 2×2、工作台恰好 3×3、产物格独立且不是普通移动目标、36 格栏位保持原位、全部格在受支持最窄 framebuffer 内可见且命中矩形与绘制矩形一致、最大打开态精确 quad 数锁定且 ≤267、glyph/offset/总容量不变；随后在 `internal/render/hud/container.go` 以 grid size 参数与产物格替换十条配方行（复用 `appendItemTile` 与既有 atlas）。验证：`go test ./internal/render/hud -race -count=1`。
- [x] 4.2 在 `cmd/mornlea/app_inventory_crafting_test.go` 先写失败测试：格点击组 `MoveCraftingStack`、产物格点击发 `TakeCraftingOutput`、确认前不本地改写、recipe-click 发送路径删除；随后改 `cmd/mornlea/app.go`、`app_input.go`、`app_messages.go` 接通输入与权威镜像。验证：`go test ./cmd/mornlea -run 'TestInventory|TestCrafting' -count=1`。
- [x] 4.3 更新 `cmd/mornlea/capture_scene.go`/`capture_scene_test.go`：`inventory-crafting` 改为 2×2 实物配方与非空产物格，新增 `workbench-crafting` 构造（3×3、镜像不对称配方、合法产物）插在其后、`chest-container` 之前；只加场景与像素不变量测试，不写 golden PNG。验证：`go test ./cmd/mornlea -run 'TestCaptureScene' -count=1`；`gofmt -w internal/render/hud cmd/mornlea`。

## 5. 功能线终审与移交

- [x] 5.1 运行功能线验证并把结果记入 `ledger.md`：`make rust`；`go test ./internal/core ./internal/network ./internal/storage ./internal/sim ./internal/server ./internal/client ./internal/render/hud ./cmd/mornlea -race -count=1`；`go test ./internal/archcheck -count=1`；`openspec validate --all --strict --no-interactive`；`git diff --check`。
- [x] 5.2 独立 SPEC/QUALITY 双评审通过后勾选全部任务、在 `ledger.md` 记录 commit SHA 与评审结论，移交批次集成（A-06）；不自行合入 `main`、不更新 golden 与基线文档。
