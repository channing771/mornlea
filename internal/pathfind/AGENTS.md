# Pathfind 包指南

`internal/pathfind` 是 Go 寻路的唯一所有者，唯一允许的内部包依赖是 `internal/core`。

- 输入是不可变快照；不得访问世界状态、执行 I/O 或启动 goroutine。
- 不得增加 Go/Rust fallback。
