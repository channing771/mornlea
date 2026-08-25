# Drop Go Test Oracles

## Why

`rust-engine-go-rules-design.md` §15「oracle 删除条款」预留了本任务：四个子系统（碰撞、射线、物理积分、世界生成）迁入 Rust `mornlea_engine` 并稳定后，以独立 change 删除 test-only 的旧 Go oracle 副本，使「Rust 生产切换」与「历史实现删除」分别评审、分别回退。切换已稳定多个里程碑（engine ABI v4→v6 期间差分门禁持续全绿），删除条件成熟。

保留的旧 Go 实现副本不是免费的呢：它们是与生产并行的第二份实现，每次生产侧语义演进（如 fluid 系列给物理加水中分支）都要人肉同步镜像（`motion_helpers_test.go` 的水中分支注释即为例证），并且给读者「Go 侧仍有一份可对照的实现」的错误心智模型——AGENTS.md 已明确「没有生产 Go fallback」，测试代码里的整份旧实现是该承诺之外唯一的存留体。

## What Changes

按认领范围冻结删除三切片（`internal/mesh` 的 greedy/light oracle 因 A-02 正在改 Rust mesher 而延迟，不在本 change）：

- **physics**：删除 `motion_helpers_test.go`（旧 Go 积分的逐字副本）与两个 native 对照文件中的全部差分测试；把代表性确定性用例转成位级 golden 向量；共享 fixture 与 oracle 专属代码拆分。
- **core raycast**：删除 `raycast_helpers_test.go`（旧 Go DDA 副本）与差分 fuzz；既有性质 fuzz 追加两条廉价不变量补位。
- **worldgen**：删除自包含 pointwise oracle 全套与差分门禁主体；可经生产黑盒表达的性质改写到 `GenerateChunk` 公共路径。
- **基线句同步**：`AGENTS.md`/`CLAUDE.md` 中三句「旧 Go X 只作测试 oracle」随删除失效，窄 hunk 同步更新（两文件逐字节相同），`openspec/config.yaml` context 同句一并更新；版本号句零触碰。

## Capabilities

### New Capabilities

无。本变更只删除测试基础设施并同步基线文档表述，已在 `.openspec.yaml` 中声明 `skip_specs: true`。

### Modified Capabilities

无。玩家可观察行为与现有主规格均不改变。

## Impact

- 只修改 `internal/physics`、`internal/core`、`internal/worldgen` 的 `*_test.go`、`AGENTS.md`/`CLAUDE.md`/`openspec/config.yaml` 的 oracle 表述与本 change 产物；**不修改任何生产 Go/Rust 代码**（勘察确认：旧实现早已只存在于测试副本，`generator.go` 无死代码）。
- 不改变协议、存档 schema、engine/client ABI、benchmark scenario、依赖或配置格式，也不生成视觉 golden。
- 门禁不净放宽：被删的是「生产==冻结副本」迁移差分网（§15 明文 sanction 其消亡）；行为性质网（确定性、区间、不变量、布局锁、纯性质 fuzz、e2e parity）全部保留，physics 确定性子集升级为位级 golden 向量。
- `internal/mesh` 切片延迟至 A 批次合流后另行认领；届时独立评审。

## 非目标

- 不删除 `internal/mesh` 的 greedy/light oracle 与其差分测试（A-02 独占且正在演进 Rust mesher）。
- 不触碰 companion 寻路等其它含「oracle」字样的无关代码与注释。
- 不改任何 tunable、常量、ABI 编码或生产行为；不动 benchmark/capture/golden。
- 不做 AGENTS.md 版本号段落重写（A-07 集成任务独占）；只改本 change 使失效的三句 oracle 表述。

## 用户可观察结果

游戏行为完全不变。仓库不再保有三份与生产并行的旧 Go 引擎副本；`go test` 维持全绿，physics 获得位级确定性 golden 向量与 raycast 性质 fuzz 的两条新不变量作为长期回归网。
