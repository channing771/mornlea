## 1. Core and Metadata

- [x] 1.1 在 `packages/shared/core` 增加 `Difficulty` 的三个固定值、合法性、严格小写解析和格式化，并在同包测试覆盖完整值域与非法文本；验证：`go test ./packages/shared/core -race -count=1`
- [x] 1.2 在 `packages/server/storage` 给 `Metadata` 增加难度字段，将 metadata codec 的当前版本与 payload 升至 v6（62 字节，`DepthsSeedSalt` 后纯尾部追加 1 字节），在 `WorldStore` 创建和保存路径使用当前版本；验证：`go test ./packages/server/storage -race -count=1`
- [x] 1.3 为 metadata v6 增加 deterministic/golden、v1..v5 只读迁移（难度恒 normal）、非法难度、CRC/长度、未来版本和原子保存失败测试，并让现有 v1..v5 测试 fixture 继续覆盖旧字节；验证：`go test ./packages/server/storage -race -count=1`
- [x] 1.4 更新 `packages/audit` 基线版本钉值（`TestBaselineVersionsMatchCode`）与 `AGENTS.md`、`openspec/config.yaml` 版本矩阵文本为 metadata v6，不改变协议、玩家/区块 schema、ABI 或 benchmark 版本；验证：`go test ./packages/audit -count=1`

## 2. Authoritative Simulation

- [x] 2.1 让 `sim.Engine` 在构造时接收并保存不可变难度，缺省构造保持 normal，非法难度稳定失败；为三档构造行为写失败测试；验证：`go test ./packages/server/sim/entity -race -count=1`
- [x] 2.2 修改饥饿结算（先写失败测试）：normal 保持一点生命硬地板，hard 允许饥饿伤害进入既有死亡结算，peaceful 跳过饥饿伤害且不重置回血计时；覆盖间隔边界、回血计时重置和死亡反馈；验证：`go test ./packages/server/sim/entity -race -count=1`
- [x] 2.3 修改自然回血（先写失败测试）：normal/hard 继续使用权威回血门控（默认值 18），peaceful 取消该门控，并仅在实际回血后将饥饿与饱和度恢复到完整值；覆盖未回血和满血 no-op；验证：`go test ./packages/server/sim/entity -race -count=1`
- [x] 2.4 修改夜行者生成门控（先写失败测试）：peaceful 在 `advanceHostileSpawn` 入口短路，不派生候选、不消耗验证预算；normal/hard 生成行为逐位不变；验证：`go test ./packages/server/sim/entity -race -count=1`

## 3. Server Wiring

- [x] 3.1 从 `store.Metadata()` 单次快照把难度显式传入 `sim.NewEngine`，保持 Memory 与磁盘 World 的同一装配路径；验证：`go test ./packages/server/server -race -count=1`
- [x] 3.2 增加重启和跨传输集成测试，证明保存的难度优先于构造默认值或客户端输入；验证：`go test ./packages/server/server -race -count=1`

## 4. Dedicated Server CLI

- [x] 4.1 在 `packages/server/cmd/mornlea-server` 增加可选 `--difficulty`，区分省略与显式 `normal`，仅接受三个小写值；新世界省略使用 normal，已有世界省略使用 metadata；验证：`go test ./packages/server/cmd/mornlea-server -race -count=1`
- [x] 4.2 在 listener 创建前校验已有世界的显式难度冲突；parse/open 失败不得产生后续副作用，store 打开后的冲突、listen 和 host 构造错误路径必须关闭已拥有资源；增加 listener 未创建和 close 计数测试；验证：`go test ./packages/server/cmd/mornlea-server -race -count=1`

## 5. Graphical Client Guardrails

- [x] 5.1 保持图形客户端没有难度覆盖 flag，误传参数在解析阶段失败；覆盖 `--connect`、benchmark、capture 和普通本地路径，不修改 HUD、协议、capture 或 benchmark 数据；验证：`go test ./packages/client/... -count=1`（含 `cmd/mornlea` 相关选项解析测试）

## 6. Integration and Contract Closure

- [ ] 6.1 复读 proposal、delta specs、design 和 tasks，校正实现/测试与行为契约的偏差；验证：`openspec validate --all --strict --no-interactive`
- [ ] 6.2 运行跨层 metadata v6 重启、CLI 一致性、peaceful 生成门控和难度结果集成测试，确认不存在第二套难度规则或每 tick 磁盘读取；验证：`go test ./packages/audit -count=1`、`go vet ./...`（六模块）

## 7. Final Gates

- [ ] 7.1 运行 `gofmt` 检查、六模块 `go vet`、`make test-race` 和 `openspec validate --all --strict --no-interactive`；验证：以上命令全部成功
- [ ] 7.2 运行 `make dev-check`、`make test-race-changed`，记录性能结果但不放宽真实错误门禁；验证：以上命令全部成功
- [ ] 7.3 完成整分支规格/质量评审，修复或按 SDD 规则记录所有 findings，并在 ledger 写入最终结果；验证：`git diff --check` 与整分支 review package 完成
