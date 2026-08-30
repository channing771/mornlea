# Execution Ledger: rust-render-world-cache

## Task 1：change 设立与基线

- 起始提交：`2344ca8551277317a31d4c1614c6086acd4ce328`。
- 起始工作区：`openspec/changes/rust-render-world-cache/` 是唯一未跟踪路径；其中
  `.openspec.yaml` 预置为 `schema: spec-driven`、`created: 2026-08-30`。没有已跟踪的
  生产代码或无关 OpenSpec 改动。
- 工具链：`rustc 1.97.1 (8bab26f4f 2026-07-14)`；
  `cargo 1.97.1 (c980f4866 2026-06-30)`；`go version go1.26.0 darwin/arm64`。

### 基线命令

| 命令 | 结果摘要 |
| --- | --- |
| `make rust` | 退出 0；`rustup run 1.97.1 cargo build --locked --release` 在 0.27s 完成，并更新两个 dylib 签名。 |
| `cd engine && cargo test -p mornlea_client --locked` | 退出 0；186 passed、0 failed、0 ignored，3.83s；doc-tests 0 passed、0 failed。 |
| `go test ./internal/client -race -count=1` | 退出 0；`ok github.com/channing771/mornlea/internal/client`，4.242s。 |
| `go test ./internal/archcheck -count=1` | 退出 0；`ok github.com/channing771/mornlea/internal/archcheck`，10.024s。 |

### 裁决与范围

- MRW1 v1 的 24 字节 batch header、32 字节 record header、4 MiB/4096 上限、
  `ContainerSnapshot` 三态、world reset 首 record、epoch/revision/tombstone 和坐标裁决
  以 Task 1 binding brief 为唯一固定契约；不得以网络 packet 或未列出的兼容格式替代。
- `RenderWorld` 是 renderer 独占、可丢弃的派生缓存；Go Mirror 仍为逻辑真相来源。
  Task 1 不允许它接入 app、mesh、visibility、upload 或 draw。
- client ABI 升至 v12 并拒绝 v11 混装；engine ABI 保持 v8。流体源码、流体语义与
  benchmark scenario v20 均不在范围内。
- Task 1 是文档/bootstrap 工作，没有代码 RED/GREEN 循环。控制器明确要求本任务不派发
  subagent；后续 Tasks 2–5 必须遵循 `tasks.md` 的 fresh implementer 与双独立评审流程。

## Task 1 验证与自检

- `openspec status --change rust-render-world-cache --json`：所有 proposal、specs、design 与
  tasks artifact 均为 `done`，`isComplete` 为 true。
- `openspec validate --all --strict --no-interactive`：退出 0，78 passed、0 failed。
- `git diff --check -- openspec/changes/rust-render-world-cache`：退出 0 且无输出。
- 产物限于本 change；未修改 Rust/Go 生产代码、fluid-aware engine 源码、hook 配置或
  无关 OpenSpec change。
