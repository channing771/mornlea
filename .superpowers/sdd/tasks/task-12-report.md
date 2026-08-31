# Task 12 report: CI、文档、版本矩阵与全量门禁

## 结果

从 Task 11 closeout `9423f067` 开始，新增独立 Python Agent 服务的固定 check gate，把真实
跨语言 integration 接入既有 macOS native-artifact CI job，并以 archcheck 钉住 Make、CI、
共享合同、服务边界和 Go 禁止 shell/FFI/embed Python 的架构约束。长期文档现在说明双进程
拓扑、Go 权威边界、loopback-only 安全模型、严格配置、失败/关服语义、companions v5
迁移以及备份/回滚。

版本矩阵保持 protocol v32、player v8、chunk v9、metadata v3、hostile v1、engine ABI v9、
client ABI v13、scenario v20；只把 `companions.ai` v4 升为 v5。实施期间没有查看、比较或合并
main，没有访问 provider/DNS/外网业务服务，也没有启动或聚焦游戏窗口。

实现提交：

- `9dfdbc69` `ci(companion): enforce agent service gates`
- `49fbc7f9` `docs(companion): document standalone agent service`
- `ee642a05` `docs(openspec): record task 12 implementation`
- `3394fc19` `test(server): restore planner seams for task fixtures`
- `3ce58189` `fix(ci): harden companion agent release gates`
- `8dac0e4c` `docs(openspec): record task 12 repair evidence`
- `72ef5580` `fix(ci): complete companion release sentinels`

本报告的最终 evidence commit 不自引用其 SHA。Task 12 当前为 Repair 2 implementation/full
gates complete、re-review pending；未修改
`openspec/changes/extract-companion-agent-service/tasks.md`，只有独立 SPEC/QUALITY 双评审
通过后才允许 closeout。

## RED / GREEN

### archcheck 与版本矩阵

先增加 code-derived version/config consistency、service/shared-contract boundary、Python source
fake 排除、Make/CI markers 与 Go no-shell/FFI/embed Python tests。RED 命令：

`go test ./internal/archcheck -run 'BaselineVersions|CompanionAgent' -count=1`

其精确暴露 `openspec/config.yaml` companions v4 与代码 v5 漂移、Make 缺少 check target、CI
缺少 Python/uv/cache/check/integration。实现后 `go test ./internal/archcheck -count=1` PASS
（package 5.532s，real 6.24s）。

### Make 与 CI

`make companion-agent-check` 固定执行 `uv sync --locked`、`ruff format --check .`、
`ruff check .`、`mypy src` 与 `pytest -q`。focused GREEN 为 403 passed（pytest 18.02s，
整组 real 19.50s）。

CI 沿用现有 macOS `integration` job 和同 SHA native artifact 校验；只在该 job 增加 Python
3.12、固定 uv 0.12.5、以 `pyproject.toml`/`uv.lock` 为依赖键的 cache，并在 artifact 校验后
运行 `make companion-agent-check` 与 `make companion-agent-integration`。required summary job 的
名称、needs 与成功语义没有改变。

### 文档

双语 README、architecture、configuration、LAN、test quickstart 与长期 progress 现在明确：

- Python Agent 是 LangChain/LangGraph/FastAPI/SQLite 独立进程，Go 仍是世界、task 与 lifecycle 权威；
- Agent HTTP 与 MCP 只绑定 loopback，不允许远程暴露、反向代理或端口转发；
- 服务启动顺序、ready/auth、PlannerUnavailable/Dialogue skip、并发/timeout/cancel 与可重试关服；
- 严格 string `config_version: v1` Python YAML、Go `ai.agentService`、env secret 与无自动拉起语义；
- companions v1..v4 只读迁移到 encoder-only v5、SQLite compact memory mirror、联合备份与回滚限制；
- `gates.sh` 已包含 `make rust`，但不包含 Python check 或真实 integration，触碰伙伴 Agent 时仍须显式运行后两者。

文档后 `git diff --check`、archcheck 与 OpenSpec strict 80/80 均 PASS。

### Round 1 独立评审 repair

Round 1 SPEC 与 QUALITY 均为 FAIL，未覆盖成 PASS。评审指出六类问题：test quickstart 的
escaped pipe 实际 no-tests；CI archcheck 依赖 substring 和下一个 job 的文本位置；Go
no-bypass 只扫两个目录而不扫生产装配的传递依赖闭包；report 必须准确写 strict string
`config_version: v1` 且 `gates.sh` 事实记录错误；configuration 仍把当前 WKWebView 说成 egui
并链接不存在的 `ui.rs`；上述门禁缺少 mutation/sentinel 承重证明。Round 1 没有要求把
Python 配置从 string `v1` 改为 YAML 数字 `1`。

RED 先真实复现：旧命令
`go test ./internal/server -run 'CrossLanguage\|CompanionAgent.*Integration' -count=1 -v`
打印 `testing: warning: no tests to run`；新增 mutation test 在实现 helper 前以 undefined symbol
编译失败。repair 后：

- `TestCompanionAgentCIGates` 用 `yaml.v3` typed scalar/map 解码，不依赖 job 文本顺序，结构化锁定
  `integration` 的 `native-macos`、macOS runner、Python 3.12、uv 0.12.5/cache、同 SHA artifact
  下载和 manifest 校验顺序、两条 Make 门禁，以及最终 `test` 的 dependency/result assertion；
- 15 项 CI mutation 逐项破坏 dependency、runner、类型、版本、cache、artifact、顺序、Make 和
  summary，合法 job 重排 fixture 保持 GREEN；
- no-bypass 从 `cmd/mornlea`、`cmd/mornlea/app`、`cmd/mornlea-server`、`internal/server`、
  `internal/companion` 构造 production import closure，只对实际 benchmark 和既有 native bridge
  做 exact package/import 窄豁免；三项 helper mutation 均能抓住间接 `os/exec`、`C`、
  `runtime/cgo`，闭包外 agent-board 不误杀；
- quickstart 命令改为单引号内 `CrossLanguage|CompanionAgent.*Integration`。永久哨兵从 server
  顶层测试源码证明它精确选中 3 条，旧 escaped-pipe mutation 选中 0 条；实际 `-list` 和 `-v`
  命令也只运行并通过三条目标测试（package 6.332s，real 6.90s）；
- configuration 改为真实的 Rust-hosted WKWebView、Go typed state/transaction 与 React rendering
  文件链接；但 Repair 1 错误地把权威 string `config_version: v1` 改成数字 `1`。report/progress
  的 `gates.sh` 事实已正确更正。

focused mutation/CI/closure/quickstart 集合 PASS（package 0.901s，real 1.62s），完整 archcheck
PASS（package 4.979s，real 5.20s）。Repair 1 仍是 re-review pending；最终实现 SHA `3ce58189`
的完整 mandatory gate 结果见下文。

### Repair 1 scoped re-review 与 Repair 2 RED

Repair 1 scoped SPEC 与 QUALITY 仍均为 FAIL，未覆盖成 PASS。明确剩余项是：数字
`config_version: 1` 被生产 `Literal["v1"]` 与 `tests/test_config.py` 拒绝；CI gate 未证明
`validate_artifact` 实际分别调用 engine/client dylib、未锁定函数内 size 校验，也未锁定最终
summary 的 `if: ${{ always() }}`；正式 change ledger 仍停在旧 Review pending；配置文档
没有直接联动 strict loader 的 sentinel。

Repair 2 RED 已复现：把文档 YAML 示例直接交给真实 `load_config` 时以
`invalid configuration fields: config_version` 失败；新增 CI mutations 删除 size 校验、engine/
client validator 调用或删除/改写 summary `always()` 时，旧 validator 全部返回零违规而失败。

Repair 2 已把 configuration 示例恢复为 string `v1`，并让既有 strict config 测试直接抽取该
fenced YAML 后调用真实 `load_config`；`tests/test_config.py` 111 passed。typed CI gate 现在同时锁定
`validate_artifact` 函数内 size/digest 校验、engine/client 两次实际调用，以及 summary 精确
`if: ${{ always() }}`；20 项 mutation 与真实 workflow gate 全部 PASS（package 0.766s，real
1.56s），完整 archcheck PASS（package 5.307s，real 5.61s）。正式 change ledger 已补齐两轮
FAIL 与历史 full-gate 证据。`go mod tidy -diff`、Ruff focused、diff-check 与 OpenSpec strict
80/80 均 PASS。Repair 2 实现提交为 `72ef5580`；其 mandatory full gates 见下文，重新独立双评审
仍 pending。

## 全量门禁

### 首轮失败与根因修复

evidence baseline `ee642a05` 的第一轮 `go test ./... -race` 真实运行 392.59s 后 FAIL；唯一
失败包为 `internal/server`（371.169s），其余包全部 PASS。九个旧 mine/place tests 从预期
Accepted→Started→terminal 变为 Accepted→Failed，两条 restore tests 各在 60s timeout。

focused 复现确认不是全仓并发 flaky：两个交互用例立即以 `[1 6]` 失败，两条 restore 用例继续
60s timeout（real 122.72s）。根因是 Task 9 删除 direct-model endpoint 时，
`newInteractionHost` 与 `restoredCompanionHost` 的 `if model != nil` 留成空块；fake planner
从未注入，生产缺省 Agent planner 正确返回 unavailable。`3394fc19` 让两个 helper 复用已有
typed `replacePlannerForTest` seam，不改生产路径。原失败的 11 个用例全部 GREEN：package
12.281s，real 16.29s。

没有使用 skip、豁免变量、timeout 放宽或测试删减。以下是在 clean `3394fc19` 从头重跑的
历史 mandatory gate 清单；Round 1 repair 已改变实现树，最终证据必须由 repair commit SHA
重新运行并追加：

- `cd services/companion-agent && uv sync --locked`：PASS，80 packages resolved、77 checked，real 0.10s。
- `uv run ruff format --check .`：PASS，38 files already formatted，real 0.06s。
- `uv run ruff check .`：PASS，real 0.03s。
- `uv run mypy src`：PASS，23 source files，real 0.53s。
- `uv run pytest -q`：PASS，403 passed in 17.97s，real 18.65s。
- `gofmt -l .`：PASS，无输出，real 0.21s。
- `go vet ./...`：PASS，无输出，real 1.98s。
- `go test ./... -race`：PASS；改动相关 server 247.013s、整组 real 248.75s；其余未变包由 Go 标准 test cache 验证。
- `make rust`：PASS，Rust 1.97.1 locked release，real 0.42s；同 SHA clean preflight 亦 PASS（0.41s）。
- `make companion-agent-check`：PASS，403 passed in 18.01s，real 19.43s。
- `make companion-agent-integration`：PASS；companion 2.745s、server 9.332s，real 10.73s。
- `git diff --check`：PASS，无输出，real 0.01s。
- `openspec validate --all --strict --no-interactive`：PASS，80 passed/0 failed，real 1.57s。

### Repair 1 最终实现 SHA 全量门禁

Repair 1 提交后先确认 clean `3ce58189`，按 clean-checkout Rust 纪律执行 preflight
`make rust`，再逐项从头运行 brief 的 mandatory 清单：

- clean preflight `make rust`：PASS，Rust 1.97.1 locked release，real 0.42s。
- `cd services/companion-agent && uv sync --locked`：PASS，80 packages resolved、77 checked，real 0.11s。
- `uv run ruff format --check .`：PASS，38 files already formatted，real 0.05s。
- `uv run ruff check .`：PASS，real 0.02s。
- `uv run mypy src`：PASS，23 source files，real 0.55s。
- `uv run pytest -q`：PASS，403 passed in 17.98s，real 18.64s。
- `gofmt -l .`：PASS，无文件输出，real 0.20s。
- `go vet ./...`：PASS，无输出，real 0.58s。
- `go test ./... -race`：PASS；repair 影响的 archcheck 37.396s，总 real 39.35s；其余未改包由 Go 标准 test cache 复核。
- mandatory `make rust`：PASS，Rust 1.97.1 locked release，real 0.38s。
- `make companion-agent-check`：PASS，403 passed in 17.98s，real 19.34s。
- `make companion-agent-integration`：PASS；companion 2.933s（该 package 无匹配测试）、server 9.853s，real 11.34s。
- `git diff --check`：PASS，无输出，real 0.01s。
- `openspec validate --all --strict --no-interactive`：PASS，80 passed/0 failed，real 1.47s。

所有命令 exit 0。运行期间只使用 deterministic fake/check-in fixtures 和 loopback 真进程，
没有 provider、DNS 或外网业务访问，也没有启动/聚焦游戏窗口。运行结束时 `git status --short`
无输出；本 evidence-only 更新后只需复跑受影响的 archcheck、OpenSpec、diff/status。

### Repair 2 最终实现 SHA 全量门禁

Repair 2 提交后确认 clean `72ef5580`，按 clean-checkout Rust 纪律执行 preflight `make rust`，
再逐项运行 brief 的 mandatory 清单：

- clean preflight `make rust`：PASS，Rust 1.97.1 locked release，real 0.44s。
- `cd services/companion-agent && uv sync --locked`：PASS，80 packages resolved、77 checked，real 0.02s。
- `uv run ruff format --check .`：PASS，38 files already formatted，real 0.03s。
- `uv run ruff check .`：PASS，real 0.03s。
- `uv run mypy src`：PASS，23 source files，real 0.58s。
- `uv run pytest -q`：PASS，403 passed in 18.08s，real 18.78s。
- `gofmt -l .`：PASS，无文件输出。
- `go vet ./...`：PASS，无输出，real 0.68s。
- `go test ./... -race`：PASS；repair 影响的 archcheck 37.162s，总 real 38.87s；其余未改包由 Go 标准 test cache 复核。
- mandatory `make rust`：PASS，Rust 1.97.1 locked release，real 0.44s。
- `make companion-agent-check`：PASS，403 passed in 17.94s，real 19.33s。
- `make companion-agent-integration`：PASS；companion 2.941s（该 package 无匹配测试）、server 9.825s，real 11.29s。
- `git diff --check`：PASS，无输出；`git status --short` 同样无输出。
- `openspec validate --all --strict --no-interactive`：PASS，80 passed/0 failed，real 1.44s。

所有命令 exit 0。运行期间只使用 deterministic fake/check-in fixtures 与 loopback 真进程，
没有 provider、DNS 或外网业务访问，也没有启动/聚焦游戏窗口。版本矩阵仍保持 engine ABI v9、
client ABI v13，只有 `companions.ai` 为 v5。

## 范围与风险

- Python unit/integration 都使用 deterministic fake model 或 checked-in fixtures；没有 provider 调用。
- CI 仍消费既有 native artifacts，没有新建平行 native build 或改变 required-job 语义。
- 不升 engine/client ABI，不改变游戏 wire、HTTP application contract v1 或 MCP tool contract v1。
- Round 1 与 Repair 1 scoped SPEC/QUALITY 均 FAIL；Repair 2 implementation/full gates 已完成，但
  重新独立双评审尚未执行，因此 ledger 必须保持 re-review pending。
