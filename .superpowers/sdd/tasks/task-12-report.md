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

本报告与 ledger/progress 证据提交会在门禁完成后补入最终提交列表。Task 12 当前仍为
Review pending；未修改 `openspec/changes/extract-companion-agent-service/tasks.md`，只有独立
SPEC/QUALITY 双评审通过后才允许 closeout。

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
- 严格 `config_version: 1` Python YAML、Go `ai.agentService`、env secret 与无自动拉起语义；
- companions v1..v4 只读迁移到 encoder-only v5、SQLite compact memory mirror、联合备份与回滚限制；
- `gates.sh` 不隐含 `make rust`、Python check 或真实 integration，收尾必须显式运行。

文档后 `git diff --check`、archcheck 与 OpenSpec strict 80/80 均 PASS。

## 全量门禁

待在 evidence baseline commit 上逐项真实运行并记录 brief 指定命令；任何失败必须修复根因，
不使用 skip、豁免变量或降级。门禁结果写入本节后，仅追加 evidence commit，并在该最终提交上
复跑文档/架构/OpenSpec/diff 门禁。

## 范围与风险

- Python unit/integration 都使用 deterministic fake model 或 checked-in fixtures；没有 provider 调用。
- CI 仍消费既有 native artifacts，没有新建平行 native build 或改变 required-job 语义。
- 不升 engine/client ABI，不改变游戏 wire、HTTP application contract v1 或 MCP tool contract v1。
- 独立 SPEC/QUALITY 尚未执行，因此 ledger 必须保持 Review pending。
