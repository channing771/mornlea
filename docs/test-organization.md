# 测试文件编排规范（细则）

本文件是 `AGENTS.md`「实现约定」中两条测试编排条文的操作细则：判定判据、拆分步骤、
helper 落位规则与验收清单都在这里。条文是原则，本文件是可执行的步骤。

## 原则与边界

- **零行为变化**：测试文件的拆分与 helper 迁移是纯重组。测试函数名、子测试
  （`t.Run` 标签）是测试入口，一律不改——这同时是 openspec 主规格
  `repository-code-organization` 的要求（重构不改测试入口，`-run` 过滤器保持兼容）。
- 测试与被测代码**同目录**（`go test` 只认同目录 `_test.go`，白盒断言依赖同包），
  不做集中式 `tests/` 镜像目录。
- 单包的测试文件重组按本文件直接执行；跨多包的批量推广是跨组件改动，走 OpenSpec
  change。

## 目录与包形态

- 新测试文件跟随包内既有形态：包内以白盒同包为主就写同包，以 `foo_test` 外部包为
  主就写外部包。既有分布：`internal/core`、`internal/physics`、`internal/world`、
  `internal/mesh` 偏外部黑盒；`internal/server`、`internal/sim` 偏白盒。
- 白盒与外部包不得为了重组而互换——那是行为可见性变化，超出纯重组范围。

## 测试文件命名

- 基本式 `<主题>_test.go`，主题名取自**被测源文件名**（`rules.go` →
  `rules_test.go`）或**被证性质**。
- 前缀与后缀语义（文件名纯组织性，`-run`/`-list` 只认函数名）：

| 名字段 | 语义 | 例 |
|---|---|---|
| `property_` 前缀 | 被证性质（决策验证关口性质测试集） | `property_rescan_test.go` |
| `_integration` 后缀 | 跨组件集成 | `farming_integration_test.go` |
| `_restart` 后缀 | 重启/持久化恢复 | `drop_restart_test.go` |
| `_e2e` 后缀 | 端到端 | `hunger_loop_e2e_test.go` |
| `_parity` 后缀 | 双实现对照（Memory/TCP、Go/Rust oracle） | `parity_test.go` |
| `_oracle` 后缀 | 以测试 oracle 为基准 | `oracle_test.go` |
| `_fuzz` 后缀 | Fuzz 入口 | `codec_fuzz_test.go` |
| `_bench`/`_benchmark`/`_perf` 后缀 | Benchmark 入口 | `bench_test.go` |
| `_external` 后缀 | `foo_test` 外部包专属视角 | `deadline_external_test.go` |
| `_golden` 后缀 | golden 逐字节断言 | `codec_golden_test.go` |

- 前缀表「被证性质」、后缀表「测试种类」，语义不同**不得叠加**（不写
  `property_x_integration_test.go`）。

## 何时拆分（判据）

- **主题混装是唯一硬判据**：一个测试文件装了 ≥2 个不相干主题（判定方法：`grep
  '^func Test' <file>` 后按函数名前缀聚类——前缀分属不同功能域即混装；或测试对应
  的被测源文件多于一个且无性质关联），就是拆分候选。
- 行数只是检视信号，不是判据：文件超过约 800 行时检查是否混装；**单主题大文件
  不拆**（如 `internal/sim/mining_test.go` 千余行全部是采掘，保持原样）。
- 拆分时机：该文件因任何改动被触碰时，顺势拆分；不为拆而拆地发起独立任务（批量
  推广走 OpenSpec）。

## 拆分操作步骤（SOP）

1. **快照**：`go test ./<pkg> -list '.*' | sort > /tmp/<pkg>-before.txt`。
2. **helper 落位判定**：对文件内每个非 `Test`/`Benchmark`/`Fuzz` 的声明，grep 其
   引用文件集合（`grep -ln '<名字>' <pkg>/*_test.go`）；被多于一个测试文件引用 →
   迁入 helper 中心；单文件私有 → 随其唯一消费者走。
3. **建目标文件并逐字搬运**：按上文命名规范建文件，测试函数连同其注释、专属
   类型与常量整体搬运；不合并、不改写、不重排语句。
4. **自指注释修复**：搬运后注释里「本文件」指称失真的，把指称改为点名文件（如
   「本文件用它当 oracle」→「queue_bounded_test.go 用它当 oracle」）。这是唯一
   允许的注释改写之一。
5. **文件头注释**：每个新文件写中文头注释，说明本文件职责（被证性质/主题、属哪
   组测试集）；被拆文件的原头说明按内容分摊到各新文件头——**任何一句不得整体
   消失**。
6. **import 最小化**：每个新文件只保留自己用到的 import。
7. **验收**：执行下节清单。
8. **提交**：单聚焦 commit，`test(<pkg>): ...` 前缀，说明拆分映射。

## helper 中心规则

- 每包**最多一个**共享 helper 中心：优先扩展包内既有 `*_helpers_test.go`；没有才
  新建 `helpers_test.go`；不得另立并行中心（如 `cmd/mornlea` 已有
  `app_test_helpers_test.go` 与 `benchmark_helpers_test.go`，扩展它们，不要再建）。
- 纯 helper 文件（不含任何 `Test`/`Benchmark`/`Fuzz` 函数）必须以
  `*_helpers_test.go` 命名，不得顶着普通测试文件名。
- 中心体量上限：超过约 500 行、或横跨两个以上不相干域（如测试替身 + 地形夹具 +
  白盒断言三域并存）时，按域再拆（如 `fixtures_test.go`）。
- helper 必须有中文注释说明用途；跨文件准入的 helper 注明它的消费者范围。

## 验收清单（零行为变化）

- [ ] 测试函数名逐一不变（`diff` 改造前后两份 `-list` 快照，排序后**集合一致**
      即可——声明顺序随文件拆分必然改变，这是预期行为不是回归）；
- [ ] 子测试标签（`t.Run` 的首个参数）逐一不变；
- [ ] 生产文件、`testdata/`、golden 零改动（`git diff --stat` 只含 `*_test.go`）；
- [ ] 注释零信息损失（被拆文件头说明的每句话都在某个新文件里有落点）；
- [ ] `gofmt -l <pkg>` 无输出、`go vet ./<pkg>`、`go test ./<pkg> -race -count=1`；
- [ ] 改动 Go 文件后 `go test ./internal/archcheck -count=1`。

## 范例

`internal/fluid`（2026-08 首个试点，commit `fa12c56`）：

| 改造前 | 改造后 |
|---|---|
| `property_test.go`（805 行，混 340 行共享工具 + 四条性质） | `property_rescan_test.go`（168）、`property_budget_test.go`（66）、`property_order_test.go`（162）、`property_converge_test.go`（148） |
| `memworld_test.go`（33 行纯 helper，无测试函数） | 并入 `helpers_test.go`（393 行） |
| `queue_bounded_test.go` 顶部跨文件 helper | `sortItems`/`queuedDueTick` 入 `helpers_test.go`；`boundedPos` 单文件私有，留原处 |

混装识别范例：`cmd/mornlea/app_input_test.go`（约 1300 行、39 个测试，前缀横跨输入
预测门控、挖掘 overlay、快捷栏放置、熔炉 UI、箱子 UI、合成、丢弃/进食/使用键七个
功能域）是当前最典型的拆分候选（此例为识别示范，若该文件已被拆分则自然过时，以
判据为准）。
