# B-02 可搬运水源（tasks，如实勾选）

> 按 `subagent-driven-development` 执行：一任务一实现 + SPEC/QUALITY 双评审，T5/T7 各一轮修复轮，整分支终审 + 一次修复波。ledger 见 change 评审记录（PR #166）。

- [x] T1 双桶物品编号与语义（`core/item.go`，55/56/57，限 1 堆叠）— `go test ./packages/shared/core -race -count=1`
- [x] T2 空桶配方（`core/recipe.go`，`RecipeBucket=20`，裁边 3×2）— 同上；gofmt 修复轮
- [x] T3 双命令协议（ID 16/17，拒绝码 13/14，协议 v38）— `go test ./packages/shared/network/protocol -race -count=1`
- [x] T4 编解码与 golden（16/17 对称分支，pin-37→38，fuzz 种子）— `go test ./packages/shared/network/... -race -count=1`
- [x] T5 无限水 kernel（Rust `fluid_eval` + Go oracle 同步，fuzz/golden/差分补齐）— `cargo test -p mornlea_engine --locked` + `go test ./packages/server/fluid -race -count=1`；修复轮补 fuzz 不变量与向量
- [x] T6 取/放原子事务与接线（`sim/entity/bucket.go`，湿度联动，采掘互斥，伙伴结构性拒绝）— `go test ./packages/server/sim/entity ./packages/server/server -race -count=1`
- [x] T7 客户端呈现与 capture（图标 2 列、splash cue、`bucket-pond` 场景、伙伴显式 `IsFluid` 守卫、shader 层号跟进）— `go test ./packages/client/... -race -count=1` + visual-check；修复轮补 shader
- [x] T8 收尾门禁 — gofmt、`go vet` 三模块、`audit`、`openspec validate --all --strict`（101 passed）、`make dev-check`、CI 9/9
- [x] 整分支终审（Ready to merge Yes）+ 一次修复波（Y 界拒绝码、配方 9 字面量、湿度断言、发布顺序注释）+ scoped 复审

## 后续 follow-up（非阻塞，已登记）

- 重启/rescan 无限水集成测试。
- 湿度断言加强为机制锁（当前为终态锁）。
