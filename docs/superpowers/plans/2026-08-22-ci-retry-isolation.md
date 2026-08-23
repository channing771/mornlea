# CI 去重、分片与失败隔离实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除 PR 的 push/pull_request 重复 workflow，把 macOS 单体 job 拆成可复用 Rust 产物、质量门禁、三片 race、集成/性能和最终 `test` 聚合，使真实失败只需重跑失败分片。

**Architecture:** 产品改动只在 `.github/workflows/ci.yml`。`native-macos` 一次构建同 SHA 的 engine/client dylib 并上传带 SHA 清单的短期 artifact；所有 macOS 下游 fail-closed 下载验证。`go-race` matrix 的三片并集由 workflow 内 `go list` 自检等于原 `./...`；最终轻量 `test` 用 `needs` 汇总全部门禁。

**Tech Stack:** GitHub Actions、Go 1.26、Rust 1.97.1、artifact v4、现有 Make targets/OpenSpec/benchmarks。

**Spec:** `docs/superpowers/specs/2026-08-22-five-way-parallel-wave-design.md` §7、§3、§9。

## Global Constraints

- [ ] 从共享 main 创建 `codex/ci-retry-isolation` 独立 worktree/change；除了 OpenSpec/ledger/本计划，唯一产品文件是 `.github/workflows/ci.yml`。
- [ ] 不删除测试、不减 `-count`、不缩 race 覆盖、不放宽视觉/性能/完整性/overflow/I/O/数据丢失门禁，不使用 `continue-on-error`。
- [ ] `linux-server` job 内容保持逐行不变；顶层 trigger/concurrency 和最终 aggregator 可以把它纳入依赖。
- [ ] artifact 缺失、SHA 不一致或任一 shard 失败必须让最终 `test` 失败。
- [ ] 每个 task 全新 implementer、独立 SPEC/QUALITY reviewer、最多 5 轮追加修复，证据写 ledger。

---

## Task 1: 建立 CI OpenSpec 契约并记录旧 workflow 清单

**Files:**
- Create: `openspec/changes/ci-retry-isolation/.openspec.yaml`
- Create: `openspec/changes/ci-retry-isolation/proposal.md`
- Create: `openspec/changes/ci-retry-isolation/design.md`
- Create: `openspec/changes/ci-retry-isolation/tasks.md`
- Create: `openspec/changes/ci-retry-isolation/ledger.md`
- Create: `openspec/changes/ci-retry-isolation/specs/test-timing-discipline/spec.md`

- [ ] proposal 记录现状：PR SHA 同时触发 push 与 pull_request；macOS `test` 串行执行全部步骤；任一末端错误要求整 job 重跑。
- [ ] spec 锁定一 SHA 一 workflow、取消同 PR/ref 旧 SHA、单次 Rust build、race 三片全集、50ms 探针独立、最终 `test` fail-closed、rerun failed jobs 隔离。
- [ ] design 写出 job DAG 和 artifact SHA 验证；明确 `linux-server` 不变、性能只记录。
- [ ] 把现有 `test` 的每条命令逐项抄入 ledger 的迁移对照表，目标列必须恰好有一个新 job/step，不允许遗漏。
- [ ] 验证并提交：

```bash
openspec validate ci-retry-isolation --strict --no-interactive
git diff --check
git add openspec/changes/ci-retry-isolation
git commit -m "docs(openspec): plan CI retry isolation"
```

## Task 2: 去重触发并产出同 SHA native artifact

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] 保存 `linux-server` job 的原始文本用于最终 `diff` 对照。
- [ ] 把 trigger/concurrency 改成：

```yaml
on:
  push:
    branches: [main]
  pull_request:

concurrency:
  group: ci-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
```

- [ ] 把旧 macOS `test` 的 Rust 三步提取为 `native-macos`：只做 checkout、Rust 工具链身份、`make rust-check`、`make rust`；该 job 不运行 Go/Node，禁止安装无用工具链。
- [ ] 构建后写 `engine/target/release/native-source-sha.txt`，内容严格为 `GITHUB_SHA`；上传 artifact：

```yaml
- uses: actions/upload-artifact@v4
  with:
    name: native-macos-${{ github.sha }}
    if-no-files-found: error
    retention-days: 1
    path: |
      engine/target/release/libmornlea_engine.dylib
      engine/target/release/libmornlea_client.dylib
      engine/target/release/native-source-sha.txt
```

- [ ] job 开头把 epoch 写入 `GITHUB_ENV`，末尾把总秒数和 runner OS/arch 写入 `GITHUB_STEP_SUMMARY`；不设耗时阈值。
- [ ] 本地静态核对 YAML 缩进、action 版本、两个 dylib 路径和 artifact 名都引用同一 `github.sha`。
- [ ] 提交 `git add .github/workflows/ci.yml && git commit -m "ci: build native artifacts once per SHA"`，完成双裁决。

## Task 3: 拆出 quality 与三片 race，并证明分片全集

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] 新增可重复的下游前置步骤：checkout、setup-go（cache dependency 同时包含 `go.sum`、`engine/include/mornlea_engine.h`、`engine/include/mornlea_client.h`）、download `native-macos-${{ github.sha }}` 到 `engine/target/release`，然后做下列 SHA 校验；只有运行 OpenSpec/Agent Hooks 的 `quality` 额外 setup-node，race/integration 不装 Node：

```bash
test -f engine/target/release/libmornlea_engine.dylib
test -f engine/target/release/libmornlea_client.dylib
test "$(cat engine/target/release/native-source-sha.txt)" = "$GITHUB_SHA"
```

任一失败自然退出非零；不要加 fallback build。

- [ ] `quality` 搬入且只搬入：OpenSpec strict、agent hooks、`internal/archcheck storage network physics`、`go vet ./...`、`gofmt -l .`。
- [ ] 在 quality 增加 race 分片全集自检：

```bash
go list ./... | sort -u > /tmp/all-packages.txt
{
  go list ./cmd/...
  printf '%s\n' github.com/channing771/mornlea/internal/server
  go list ./internal/... | grep -v '/internal/server$'
} | sort -u > /tmp/sharded-packages.txt
diff -u /tmp/all-packages.txt /tmp/sharded-packages.txt
```

- [ ] 新增 `go-race` matrix：

```yaml
strategy:
  fail-fast: false
  matrix:
    include:
      - shard: cmd
        packages: ./cmd/...
        extra: -skip '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$'
      - shard: internal-server
        packages: ./internal/server
        extra: ''
      - shard: internal-rest
        packages: "$(go list ./internal/... | grep -v '/internal/server$')"
        extra: ''
steps:
  # checkout/setup/download/SHA 验证同上
  - name: Race tests
    run: go test ${{ matrix.packages }} -race -p=1 ${{ matrix.extra }}
```

- [ ] 每个 matrix child 记录自己的墙钟秒数和 runner OS/arch 到 summary，不设阈值。`fail-fast:false` 保证一个 shard 失败时其余证据仍完整。
- [ ] 对照旧命令证明三片并集恰为 `go test ./... -race -p=1`，且只有 50ms 探针从 cmd shard 排除。
- [ ] 提交 `git commit -am "ci: shard race tests without dropping coverage"`，完成双裁决。

## Task 4: 拆 integration、保留全部性能门禁并建立最终 test

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] `integration` 使用同一 artifact/SHA 前置，按原参数和次数依次搬入：
  1. 50ms server probe `-count=1`；
  2. TCP restart/Memory-TCP parity `-race -count=10`；
  3. M3C v6/性能报告门禁 `-count=1`；
  4. 全仓微基准 `-bench=. -benchtime=1x -run='^$'`；
  5. 多人微基准 `-benchmem -benchtime=100x -count=1`。
- [ ] `quality` 与 `integration` 都记录总墙钟时间和 runner OS/arch；不要建立性能退出阈值。
- [ ] 保留 `linux-server` 原内容；用保存的文本逐行对照，除 YAML 因插入其他 job 导致的位置变化外内容必须相同。
- [ ] 新增最终 job，名字精确为 `test`：

```yaml
test:
  if: ${{ always() }}
  needs: [native-macos, quality, go-race, integration, linux-server]
  runs-on: ubuntu-latest
  steps:
    - name: 汇总全部必需门禁
      run: |
        test "${{ needs.native-macos.result }}" = success
        test "${{ needs.quality.result }}" = success
        test "${{ needs['go-race'].result }}" = success
        test "${{ needs.integration.result }}" = success
        test "${{ needs.linux-server.result }}" = success
```

- [ ] 检查任何 upstream 失败/取消/跳过都会让 `test` 的一个断言失败；不得用只检查 failure 而放过 skipped 的条件。
- [ ] 提交 `git commit -am "ci: isolate integration and aggregate required checks"`，完成双裁决。

## Task 5: PR 实机验收和整分支终审

**Files:**
- Modify: `openspec/changes/ci-retry-isolation/tasks.md`
- Modify: `openspec/changes/ci-retry-isolation/ledger.md`

- [ ] 本地运行 workflow 所覆盖的非重复门禁，证明拆分未掩盖代码失败：

```bash
make rust
make rust-check
go test ./... -race
go vet ./...
gofmt -l .
openspec validate --all --strict --no-interactive
git diff --check
```

- [ ] 正常 push 独立 PR，针对同一 PR SHA 检查 Actions：只有一个 CI workflow；job graph 含 native/quality/3 个 race child/integration/linux-server/test；两个下游成功下载同 SHA artifact。
- [ ] 若任一真实 shard 失败，确认最终 `test` 失败；使用 GitHub “Re-run failed jobs” 后，确认只重跑失败 matrix child（或失败的独立 job）与 `test`，成功的 native/其他 shard 不重跑。不要为了演示而放宽或伪造测试。
- [ ] 连续 push 新提交，确认旧 SHA workflow 被 concurrency 取消，新 SHA 只留一份活动 workflow。
- [ ] 把各 job wall time 记入 ledger，仅作记录；不与旧时长比较退出状态。
- [ ] 生成 `BASE..HEAD` committed review package/SHA-256，独立终审逐项对照旧 test 命令迁移表、artifact fail-closed、分片全集和 linux job；修复最多 5 轮。
- [ ] 提交 ledger/tasks；PR 不自行归档。
