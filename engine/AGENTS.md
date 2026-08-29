# Rust Workspace 指南

## 作用域

进入 `engine/crates/mornlea_engine/` 或 `engine/crates/mornlea_client/` 前，分别读取对应 crate 内的 `AGENTS.md`。

## 工具链与职责

- `engine/rust-toolchain.toml` 固定 Rust 1.97.1；不要单独升级 compiler、Cargo lock 或 wgpu 依赖线。
- `mornlea_engine` 是无窗口数值内核，提供 mesh/light、collision、raycast、physics 与 worldgen 的 engine ABI。
- `mornlea_client` 是 Darwin 窗口、事件、egui 和 GPU 后端，提供独立的 client ABI。两个 ABI 独立演进，不共享版本号。

## FFI

- `extern "C"` 入口不得让 panic unwind 穿过 FFI；在构造 slice、解引用或写输出前校验 ABI、pointer、length、alignment、overlap 与容量。
- 校验失败不留下部分输出。固定容量和 overflow 状态属于跨语言契约，不得为通过测试静默截断。
- Rust 导出项使用中文 `///` doc comment，说明安全前提、所有权、失败语义和 ABI 同步面。

## 测试组织

Rust 测试按 `docs/test-organization.md` 的主题子模块与 helper 中心规则组织；单主题测试留在对应模块树，不新建平行集成测试镜像。

## 验证与入口

- 构建 cdylib：`make rust`。
- Makefile 为 `make rust` 默认设置当前 worktree 的 `CARGO_TARGET_DIR=engine/target/cargo`；`make rust` 将 release dylibs 复制到 `engine/target/release`。直接调用 cargo 不读取 Makefile，需要该目录或其他目标时须显式设置 `CARGO_TARGET_DIR`；CI 也显式设置，传给 `make rust` 的值可覆盖默认值。
- workspace 门禁：`make rust-check`。
- crate 定点：`cd engine && cargo test -p mornlea_engine --locked`、`cd engine && cargo test -p mornlea_client --locked`。
- 当前文档入口：`docs/notes/go-rust-division.md`、`docs/test-organization.md`。
