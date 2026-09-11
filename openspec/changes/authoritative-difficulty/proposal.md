## Why

Mornlea 当前只有固定的普通生存规则，服务端无法为同一个世界选择可持久化、可重放且由服务端统一解释的难度。饥饿与自然回血的权威结算已合入，敌怪生成也有稳定门控，难度是把这些既有规则按世界分档的最小上层输入；本 change 同时是生存闭环收尾战役（`docs/superpowers/specs/2026-09-11-survival-loop-completion-design.md`）阶段一的首行。

## What Changes

- 新增三档世界难度：`normal`、`peaceful`、`hard`，由世界 metadata 保存并由服务端注入权威模拟。
- metadata 升版 v5→v6：纯尾部追加 1 字节难度；v1..v5 旧世界只读迁移为 `normal`，未来版本继续稳定拒绝。
- 饥饿伤害、自然回血饥饿门控与回血后的饥饿恢复按难度解释：`hard` 取消「一点生命」硬地板使饥饿可致死，`peaceful` 跳过饥饿伤害、取消回血门控并在实际回血后把饥饿与饱和度恢复到完整值，`normal` 保持现状。
- 和平难度禁用夜行者生成：`peaceful` 世界不生成任何夜行者；难度建域即固定且旧档迁移恒为 `normal`，因此不存在需要清除的既有夜行者。
- 专用服务端增加可选 `--difficulty`：显式值与已有世界不一致时在监听前失败并关闭已打开的存储。
- 图形客户端、线上协议、benchmark、capture 不新增难度参数或字段。

## Capabilities

### New Capabilities

- `authoritative-difficulty`: 世界难度的领域值、metadata v6 持久化与迁移、权威模拟注入语义、和平档生成门控与专服启动一致性校验。

### Modified Capabilities

- `authoritative-hunger`: 饥饿归零扣血与自然回血门控由世界难度条件化（重写两条既有需求）。
- `authoritative-hostile-nightwalker`: 夜间生成条件追加「世界难度非和平」。

## Impact

- 受影响包：`packages/shared/core`、`packages/server/storage`、`packages/server/sim/entity`、`packages/server/sim/runtime`（构造尾参注入）、`packages/server/server`、`packages/server/cmd/mornlea-server`、`packages/client`（生产 Create 点随 v6 机械清扫与旗标拒绝测试）、`packages/audit`（基线版本钉值与难度域守卫）。
- 版本矩阵：世界 metadata v5→v6；协议、玩家 schema、区块 schema、engine/client ABI、benchmark scenario 均不变。
- 既有 v1..v5 世界只读迁移为普通难度；新保存写出 v6。新增字段只影响世界存档兼容性，不增加网络依赖。

## 延期与放弃

- 延期：`packages/audit/difficulty_domain_test.go` 的守卫缺常驻坏样本自检（沿 `TestCommentIdentifierScannerCatchesKnownBadSamples` 先例抽纯函数内核 + 内嵌样本），组 6 评审 minor；当前断言逻辑简单，留待该文件出现第二位消费者或判定规则演进时补齐。
- 延期：和平档敌怪生成门控未覆盖「已持久化夜行者在加载后的清除」——经裁决这是不可能状态（难度建域固定、旧档迁移恒 normal），设计上显式不做，非遗漏。
- 免责：`packages/server/server` 的 `TestDualDimensionReloadAfterRestart` 与 `TestWarpParityMemoryVsTCP` 在本分支基线 `d4cccb6c` 即失败（F-11 在案 flake 族），与本 change 无关，移交 F-11 行处置。
