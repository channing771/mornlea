## Context

当前服务端从世界 metadata（v5，61 字节载荷：种子、出生信息、世界时间、相位偏移、天气、维度表）恢复世界身份，再创建权威 Engine；饥饿结算与夜行者生成集中在 `packages/server/sim/entity`，但饥饿伤害、自然回血门控与敌怪生成没有世界级模式输入。玩家、区块 schema 与线上协议版本不应因本变更改变。

本 change 收编 2026-08-28 孤儿分支 `feat/B-11-authoritative-difficulty`（头 `95f733f0`）的设计裁决：该分支路径基于单元化重构前的 `internal/` 布局、其 metadata v3 槽位已被占用，代码不收编、worktree 保留不动；设计经本文件按当前基线重订并补和平档生成门控。

## Goals / Non-Goals

**Goals:**

- 用一个 `core` 领域值表达三档难度，并由 metadata v6 持久化。
- 让服务端在创建权威 Engine 时注入难度，保证 Memory/TCP 复用同一结算路径。
- 以最小规则差异实现 normal、peaceful、hard：hard 取消饥饿硬地板、peaceful 跳过饥饿伤害并取消回血门控、peaceful 禁用夜行者生成。
- 覆盖存档迁移、重启和专服启动失败边界。

**Non-Goals:**

- 不做运行时切换、客户端难度 UI、HUD 新元素或网络字段。
- 不改变敌怪伤害倍率、生成上限与密度、Rust engine/client、玩家/区块 schema 或 benchmark 场景。
- 不为兼容不受约束的外部调用者增加新的抽象层；只保留仓库现有测试构造所需的缺省 normal 语义。

## Decisions

### 1. Difficulty belongs in core

`core.Difficulty` 持有三个固定整数值、合法性、`String` 和严格的小写解析。`storage.Metadata` 只保存这个领域值，不定义第二套枚举或文本表。这样 CLI、metadata 和 sim 的边界值相同，非法值由 storage 编解码与 sim 构造边界分别守住。

否决方案：把字符串直接存进 metadata。字符串需要长度/编码边界，且会让数值 wire 格式和错误判定复杂化；固定字节更适合已有定长 payload。

### 2. Metadata v6 appends one byte

v6 保留 v5 的全部字节顺序，在 `DepthsSeedSalt` 后追加一个 `uint8` 难度值，payload 为 62 字节。v1..v5 读取时难度补 `normal`，内存中的 `FormatVersion` 规范为当前版本；打开不覆盖旧文件，下一次正常 metadata 保存才写 v6。高版本在读取版本号后立即以 `ErrFutureVersion` 拒绝，未知旧版本、错误长度、CRC 或难度值以 `ErrCorrupt` 拒绝。新世界的 `OpenOptions.Create` 必须要求当前版本，防止把旧版本结构写入新世界。

否决方案：把难度写入独立文件或复用玩家存档。独立文件会让世界锁内的权威事实出现两个提交边界；玩家存档会把世界选项错误地绑定到某个玩家且无法保证多人一致。

### 3. Simulation owns an immutable difficulty snapshot

`sim.Engine` 在构造时接收难度并在生命周期内只读；每个 tick 继续只刷新 `Tunables`，不从 storage 或 config 读取难度。保留一个缺省参数位置让现有大量测试夹具继续表达 normal，而 server 的生产装配显式传 `store.Metadata().Difficulty`。构造时对非法值 panic，避免权威 tick 内出现未定义分支。

`normal` 复用当前行为，`normal` 与 `hard` 都继续使用权威 `Tunables.RegenHungerThreshold`（默认 `18`）。`peaceful` 在 starvation 阶段跳过饥饿伤害，将回血门控阈值视为零，并在 `advanceHealthRegen` 返回实际回血后按整数状态把饥饿值设为 `core.MaxHunger`、饱和度设为该上限对应的最大值。`hard` 保留计时和 `applyDamage` 入口但取消「一点生命」硬地板，因此饥饿伤害可进入既有死亡结算。回血疲劳累积规则三档一致（peaceful 回血后饥饿即恢复完整，疲劳累积不再有下游效果）。所有三档仍使用同一个串行 `Step` 阶段和同一 `Memory/TCP` server 路径。

否决方案：在 server 每 tick 修改全局 `Tunables`。这会把世界选项和调参配置混在一起，且多个世界/测试引擎之间会互相污染；难度是世界身份，不是调试旋钮。

### 4. Peaceful gates hostile spawning at the entry

`advanceHostileSpawn`（`packages/server/sim/entity/hostile_spawn.go`）入口处以难度快照短路：`peaceful` 直接返回，不派生候选、不消耗验证预算。不需要清除既有夜行者：难度在创建世界时固定，v1..v5 旧档迁移恒为 `normal`，因此 `peaceful` 世界的 `hostile_mobs` 持久化天然为空；防御性地不依赖该事实（加载路径不变，门控只管生成）。

否决方案：加载持久化夜行者后按难度清除。它为「不可能存在的状态」引入新的清除边界与存档写放大；生成门控已足够。

### 5. CLI checks metadata before listener

专服 `--difficulty` 使用 `flag.Visit` 区分省略与显式 `normal`。省略时，新世界 Create metadata 为 normal，已有世界完全使用 metadata；显式时，新世界使用该值，已有世界在 listener 创建前比较并返回错误。store 已打开时所有后续错误路径都通过既有 `Close` 汇总返回。

图形客户端不提供该旗标。若未来参数误传，普通选项解析应拒绝，而不是悄悄改变本地或远程世界。`--connect`、benchmark、capture 继续分别走现有远程/确定性路径。

否决方案：先监听再验证难度。监听成功后再失败会短暂暴露错误世界，且测试和脚本会观察到不应存在的端口；先比较 metadata 更安全，也不增加网络工作。

## Data Ownership and Concurrency

- `storage.DiskStore` 在 world lock 下拥有 metadata 内存副本和磁盘提交；metadata 编解码是纯函数，原子替换沿用已有临时文件、sync、rename、目录 sync 路径。
- `server.newWorld` 构造时只读取一次 metadata 快照，把难度与既有世界身份一起传给 `sim.Engine`；权威 tick 不读取磁盘。
- `sim.Engine` 的难度和玩家规则状态由 simulation owner 串行读写；网络 reader 只入队命令，不能直接修改难度。
- 客户端不拥有或预测难度，不新增 packet、客户端状态或渲染资源。

## Risks / Trade-offs

- [旧 metadata 被打开后直到下一次保存仍是旧字节] -> 这是只读迁移纪律；启动失败不会覆盖旧文件，正常自动保存/关服才发布 v6。
- [显式难度与世界事实不一致导致启动失败] -> 这是避免运行错误世界的安全边界；用户可省略参数使用 metadata，或在明确操作后新建独立世界目录。
- [hard 改变死亡可达性] -> 只改变 starvation damage 的硬地板，继续复用既有 `applyDamage`、死亡、快照和持久化路径，并由单测钉住一条间隔边界。
- [peaceful 回血规则与现有饥饿状态互动复杂] -> 只在实际回血返回 true 后执行固定整数恢复；满血和门控失败不改变状态，避免隐式每 tick 进食。
- [peaceful 生成门控与既有每 tick 预算交互] -> 入口短路发生在候选派生之前，不占用「每 tick 至多验证一个候选」的预算语义。

## Migration Plan

1. 读取 v1..v5 metadata，校验完整 payload 和 CRC，内存迁移为当前版本、normal 难度。
2. 以迁移后的 metadata 创建 Engine；世界时间、偏移、天气与维度表保持原有语义。
3. 自动保存或正常关服通过现有异步/原子路径写出 v6；写入失败保持旧文件并返回原错误。
4. 回滚代码时，v6 会按未来版本拒绝；恢复到备份或继续使用更新后的程序，不尝试把 v6 静默降写为 v5。

## Verification

覆盖 core 值域、metadata v1..v6 golden、CRC/长度/非法难度/未来版本、三种饥饿规则、peaceful 生成门控、server 重启、CLI 关闭顺序和图形选项拒绝；`packages/audit` 的 `TestBaselineVersionsMatchCode` 同步钉住 metadata v6。完成后运行 focused race、archcheck、vet、OpenSpec strict、仓库 gate 和全量 race；不启动前台游戏窗口。
