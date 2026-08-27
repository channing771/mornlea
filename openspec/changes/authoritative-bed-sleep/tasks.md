# Tasks：authoritative-bed-sleep

> 执行规范：每 Task 派发全新 implementer 子代理（brief 自包含）；TDD（red→green→refactor）；Task 完成后 SPEC+QUALITY 双评审，修复 ≤5 轮（R≤3 原实现者、R≥4 换新）；结论记入 `ledger.md`。协议版本号与编号终值以实现期 `main` 实占为准，撞号按 A-02 先例由后合并者重订。**前置检查（2026-08-28 并行裁决）：`core.DisplayDayPhase(worldTime uint64, offset uint16) uint16` 允许本行与夜行者行各自交付——若实现期该函数尚不在本分支，按此钉定签名与「先 `%24000` 再相加取模」语义自带（含边界测试），rebase 合并时与先行行去重（保留一份）。**

## 1. 基线验证与契约核对

- [x] 1.1 运行 `git status --short`（worktree 干净）并记录 `make rust`、`go test ./internal/core ./internal/sim ./internal/storage ./internal/network ./internal/client ./internal/render ./internal/assets ./internal/mesh ./cmd/mornlea -race -count=1` 输出摘要到 `ledger.md`（数值只记录）
- [x] 1.2 核对 `core.DisplayDayPhase` 是否已存在：已存在则核对其签名为 `(worldTime uint64, offset uint16) uint16` 并直接消费；不存在则按头部前置检查的钉定签名记入本行自带清单（rebase 去重）；核对 `core.BlockEmission`/`BlockLightAttenuation`/`BlockOpaque` 可用性；核对方块/物品/配方段末常量与 S→C 实占编号，记录本行将取用的编号于 `ledger.md`；`openspec validate --all --strict --no-interactive` 与 `git diff --check` 通过

## 2. 床方块、物品、配方与碰撞

- [x] 2.1 失败测试：8 个床 `BlockID`（床尾/床头 × 4 朝向）与 `ItemBed`（堆叠 64）登记、`BlockIDMax`/物品段顺延、放置/采掘/朝向映射的纯函数表；`go test ./internal/core -race -count=1` 后实现
- [x] 2.2 失败测试：`RecipeBed`（3×3 顶排 3 小麦 + 下排 3 橡木木板 → 床 ×1，镜像位自身等价；与门 2×3 形状互不匹配；裁边语义不受摆放位置影响）后实现；`go test ./internal/core -race -count=1`
- [x] 2.3 失败测试：床半高 9/16 碰撞体（`physics.BlockCollisionBoxes`）、raycast 命中两格、流体视床为占据格；`go test ./internal/physics ./internal/core ./internal/fluid -race -count=1` 后实现
- [x] 2.4 `internal/assets` 程序化纹理层（原创橡木配色）与 `internal/mesh` model tag 追加（容量内，不升 engine ABI，门先例；容量断言测试）；`gofmt -w internal/core internal/physics internal/fluid internal/assets internal/mesh`、`make rust`、`go test ./internal/assets ./internal/mesh ./internal/engine -race -count=1`；SPEC+QUALITY 双评审后提交 `feat: add bed blocks and recipe`（评审写入 ledger）

## 3. 放置、采掘与支撑（门先例同构）

- [ ] 3.1 失败测试：`tryPlaceBed` 同区块原子双格写入（床尾 + 朝向侧床头；两格空气、各自下方 `isSolidSupport`；跨区块未就绪整单拒绝不消耗；`pending` 合并先例）
- [ ] 3.2 失败测试：采掘任一半双清两格、恰好掉落 1 个 `ItemBed`（含耐久/掉落路径无交互）；支撑失效清除（比照门）；接 `executePlace`/采掘分派
- [ ] 3.3 `gofmt -w internal/sim`、`go test ./internal/sim -race -count=1`；SPEC+QUALITY 双评审后提交 `feat: place and break beds atomically`

## 4. 入睡、跳夜与个人重生点（sim 权威）

- [ ] 4.1 失败测试：入睡判定（夜间相位经 `core.DisplayDayPhase(ticks, offset)` ∈ 13000..23000；白天右键 `CommandRejected` 不消耗；入睡记录床尾重生点）；移动输入或受击取消 sleeping
- [ ] 4.2 失败测试：全员跳夜（全部活跃玩家 sleeping → tick 边界设 offset 使相位到 0 并清全部 sleeping；单机即一人；有玩家未入睡则相位不变；offset 覆盖旧值；`WorldTimeTicks` 推进不受影响——作物/流体随机 tick 节奏不变的对拍用例）
- [ ] 4.3 失败测试：死亡重生（重生点两格仍为同属一床 → 回床尾格；床已破坏/半破坏 → 回出生锚点且 present 清 0；`beginReset` 路径复用）
- [ ] 4.4 引擎 `dayPhaseOffset` 原子单值接入 `DisplayDayPhase` 全部消费点（本行与 A-04 敌怪判夜自动一致的对拍用例）；`gofmt -w internal/sim`、`go test ./internal/sim -race -count=1`；SPEC+QUALITY 双评审后提交 `feat: sleep through the night`

## 5. 持久化：玩家 schema v8 与 metadata v3

- [ ] 5.1 失败测试：玩家 record v8 追加 `respawnPresent/respawnPosition/respawnDimension` 尾部布局、v7 旧档迁移 present=0、round trip、fuzz 不 panic；`go test ./internal/storage -race -count=1` 后实现
- [ ] 5.2 失败测试：metadata v3 追加 `DayPhaseOffset` u64、v2/v1 迁移 offset=0、重启恢复 offset、CRC 与损坏拒绝矩阵（沿用既有错误矩阵先例）；`go test ./internal/storage -race -count=1` 后实现
- [ ] 5.3 `gofmt -w internal/storage`、`go test ./internal/storage ./internal/sim -race -count=1`；SPEC+QUALITY 双评审后提交 `feat: persist respawn point and day offset`

## 6. 协议、客户端显示相位与版本基线

- [ ] 6.1 失败测试：`PlayerState` 尾部追加 `DayPhaseOffset` u16（0..23999，>23999 拒绝；Memory/TCP round trip + transcript 一致；协议版本号取实现期 `main` 下一空闲并更新 `packet.go` 版本注释）
- [ ] 6.2 失败测试：客户端显示相位 `(WorldTimeTicks + offset) % 24000`（`render/daylight.go`；offset 变更后天空/光照随下一份权威状态切换；旧值不回退）后实现；`go test ./internal/network ./internal/client ./internal/render -race -count=1`
- [ ] 6.3 两份基线文档协议版本行、玩家 schema v8、metadata v3 版本行同步（逐字节相同，`cmp -s`）+ `TestBaselineVersionsMatchCode`；`gofmt -w internal/network internal/client internal/render`、`make rust`；SPEC+QUALITY 双评审后提交 `feat: deliver day phase offset to clients`

## 7. 视觉场景与功能线终审

- [ ] 7.1 `bed-night` 场景构造（固定夜间卧室、多朝向床形态、夜空呈现；插入 `torch-night` 之后、`ai-companion` 之前；不创建/聚焦前台窗口）并生成 golden（合并时基于届时基线口径顺延），`visual-check` 全表比对全绿
- [ ] 7.2 功能线验证：`make rust`、`go test ./... -race`（合并前全量）、`go test ./internal/archcheck -count=1`、`go vet ./...`、`gofmt -l .` 无输出、`openspec validate --all --strict --no-interactive`、`git diff --check`；benchmark/perfcheck 输出摘要记录（数值只记录）
- [ ] 7.3 独立整分支终审（规格合规 + 原子放置矩阵 + 跳夜全员语义 + 重生点校验矩阵 + v7→v8/v2→v3 迁移 + wire 尾部追加 + 绝对时间不受影响 + 无版权资源）；终审结论与验证摘要写入 `ledger.md`；推送分支开 PR，评审通过后合并 main
