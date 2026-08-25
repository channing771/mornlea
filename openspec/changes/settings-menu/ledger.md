# settings-menu 执行 ledger

## 领取与基线

- 任务：`D-01 设置菜单`
- 分支：`codex/D-01-settings-menu`
- 基线：`origin/main` `345f6077`
- 领取提交：`bfb91297`（已推送）
- 领取讨论：https://github.com/channing771/mornlea/discussions/71#discussioncomment-18147961
- 工作树：`/Users/chen/work/mornlea/.worktrees/D-01-settings-menu`
- 规划调查：只读子代理 `d01_design_investigation`；未修改实现。

## 开工前验证

| 命令 | 结果 |
| --- | --- |
| `make rust` | PASS |
| `go test ./internal/config ./internal/client ./cmd/mornlea -race -count=1` | PASS（`internal/config` 2.588s；`internal/client` 4.318s；`cmd/mornlea` 270.709s） |

## 子任务裁决

每个实现任务由全新的 implementer 完成，并由互相独立的 SPEC 与 QUALITY reviewer 双裁决。任何修复轮次、候选提交、验证结果和控制会话 ruling 均在本表追加；未完成双裁决前不得勾选 `tasks.md`。

| 任务 | Implementer | 候选/修复提交 | SPEC reviewer | QUALITY reviewer | 修复轮次 | 验证 | 控制会话 ruling |
| --- | --- | --- | --- | --- | ---: | --- | --- |
| 1.1 配置契约 | `d01_t1_config_impl` | `e1bfaa89` | `d01_t1_spec_review`：PASS | `d01_t1_quality_review`：PASS | 0 | RED 编译失败；`make rust`、`go test ./internal/config -race -count=1`、`go vet ./internal/config`、archcheck、gofmt、diff-check PASS | ACCEPTED |
| 2.1 client ABI v9 | `d01_t2_abi_impl` | `79623956`；修复 `e0e08099`、`c0b5e5d0` | `d01_t2_spec_review`：FAIL→PASS→PASS | `d01_t2_quality_review`：FAIL→FAIL→PASS | 2 | RED 编译失败；Rust 105 tests、clippy/fmt、`make rust`、client race、cmd compile/action routing、diff-check PASS；Rust 测试名集合不变 | ACCEPTED |
| 3.1 Rust egui 设置页 | 待派发 | — | — | — | 0 | — | PENDING |
| 4.1 Go 事务与接线 | 待派发 | — | — | — | 0 | — | PENDING |
| 5.1 视觉基线 | 待派发 | — | — | — | 0 | — | PENDING |
| 5.2 长期文档 | 待派发 | — | — | — | 0 | — | PENDING |
| 6.1 整分支收尾 | 待派发 | — | — | — | 0 | — | PENDING |

## 整分支终审与发布

- 独立终审：PENDING
- 最终门禁：PENDING
- OpenSpec 同步/归档：PENDING
- PR / CI / 合并：PENDING
- Discussion 完成回报：PENDING

## Rulings

- R-001：设置页只纳入 `audioVolume`、`texturePackPath` 和固定 `windowSize` 三项；不扩展在线协议、世界/玩家存档、benchmark scenario 或引擎 ABI。
- R-002：材质包保存时允许在世界装配前做候选校验，但活动材质注册表不热替换；新路径下次启动生效。
- R-003：持久化成功是运行时音频/窗口应用的前置条件；保存失败时磁盘、committed 状态与运行时均保持不变。
- R-004：client ABI v9 使用整批结构化事件与原子容量门禁；不得以旧 raw button ID 通道夹带设置值。
- R-005：接受非法直接构造的 `WindowSize.Dimensions()` 返回 `(0, 0)`；所有配置与 UI 输入边界负责先拒绝非法枚举，合法三预设映射已由测试锁定。
- R-006：client ABI 升级任务必须让现存 `cmd/mornlea` 保持可编译；在设置业务接入前，typed action 继续路由，settings-changed 只进入显式 deferred 分支且不得误触按钮。
- R-007：因本任务大量触碰既有混装 Rust UI 测试模块，按项目硬规范在同一任务拆成关注点文件并保留测试名集合；漏写主题头注释按阻断项修复，不以“仅测试”降级。
