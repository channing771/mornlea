# Tasks：authoritative-hostile-nightwalker

> 执行规范：每 Task 派发全新 implementer 子代理（brief 自包含）；TDD（red→green→refactor）；Task 完成后 SPEC+QUALITY 双评审，修复 ≤5 轮（R≤3 原实现者、R≥4 换新）；结论记入 `ledger.md`。分支内编号均为临时值，终值由 A-06 集成锁定；golden PNG 由 A-07 独占生成，本分支只写场景构造与不变量测试。

## 1. 基线验证与 change 产物核对

- [ ] 1.1 运行 `git status --short`（worktree 干净）并记录 `make rust`、`go test ./internal/core ./internal/companion ./internal/physics ./internal/sim ./internal/server ./internal/storage ./internal/network ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1` 输出摘要到 `ledger.md`（数值只记录）
- [ ] 1.2 检查 `openspec validate --all --strict --no-interactive` 与 `git diff --check` 通过；核对 proposal/specs/design/tasks 与已批准的确认结论一致（brainstorming 批准轮与 A-04-q1/q2 裁决全文誊入 ledger）

## 2. 发光/衰减单一表迁入 core 与 DisplayDayPhase、腐肉食物

- [ ] 2.1 在 `internal/core` 新增 `BlockEmission(block core.BlockID) uint8` 与 `BlockLightAttenuation(block core.BlockID) uint8` 失败测试（light block 15/其余 0；流体 1/其余 0；未来 torch 条目由 A-02 追加）；若 A-02 契约已落地（signature 与值一致）则直接消费不重复创建并记录
- [ ] 2.2 实现 core 单一表并让 `internal/assets`（`Registry.Emission`）与 `internal/mesh`（`LightAttenuation`）改为委托 core；`go test ./internal/core ./internal/assets ./internal/mesh -race -count=1`
- [ ] 2.3 在 `internal/core` 新增 overflow-safe `DisplayDayPhase(worldTime uint64, offset uint16) uint16`（先 `%24000` 再相加取模）失败测试（边界：MaxUint64、offset 23999）并实现；`go test ./internal/core -race -count=1`
- [ ] 2.4 在 `internal/core` 登记 `ItemRottenFlesh`（堆叠 64）与食物表条目（饥饿 4、饱和 0），更新「只有面包」穷举测试为精确两种食物；`go test ./internal/core -race -count=1`

## 3. hostile_mobs.bin schema v1 与存储契约

- [ ] 3.1 失败测试：文件布局（32-byte 头 magic `MHST`/envelope 1/schema 1/revision u64/count u16/payloadLen u32/CRC-32C；≤64×72-byte 记录；总长 ≤4640 bytes；记录 ID 非零严格升序；round trip）
- [ ] 3.2 失败测试：字段校验矩阵（future schema/envelope、截断、尾随、坏 CRC、count>64、重复/逆序/零 ID、未知 dimension、NaN/Inf、health 0 或 >20、非法 bool、无目标却带 PlayerID、有目标非 UUIDv4、cooldown/burn/despawn 越界、world Y 越界、payload 读空）
- [ ] 3.3 实现固定 binary codec（复用既有 `appendU32`/`byteDecoder`/CRC helper；不使用 JSON/gob/reflection；`physics.State` 只编码 position/velocity/onGround；保留字段零）
- [ ] 3.4 实现 `HostileMobStore`（`LoadHostileMobs`/`SaveHostileMobs`；missing 返回独立 `ErrHostileMobsNotFound`）与 Memory/Disk 契约（temp+fsync+rename、0600、revision 冲突与损坏保护、`hostile_mobs.bin` 路径、WorldStore 组合、backup 复制正式文件忽略 temp）
- [ ] 3.5 `hostile_codec_fuzz_test.go`/故障注入（截断、bit-flip 不 panic；golden round trip）；`gofmt -w internal/storage`、`go test ./internal/storage -race -count=1`；SPEC+QUALITY 双评审后提交 `feat: persist hostile nightwalkers`（评审写入 ledger）

## 4. sim 夜行者身体、spawn、局部暗度与生命周期

- [ ] 4.1 失败测试：固定集合（插入/恢复按 ID 排序、重复/第 65 只拒绝、`hostileState` 容量 64）；与玩家/伙伴同一 per-actor `physics.Step`（顺序玩家→伙伴→夜行者 ID 序、复用 `SubmersionFlags` 与同一 tunables）
- [ ] 4.2 实现最小 `hostileState`（排序 slice + 复用 scratch + restore/snapshot/测试入口；不建 map/ECS；AABB 与玩家相同、输入仅水平+jump）
- [ ] 4.3 失败测试：预分配 29³ 16-bucket BFS 局部区块光（半径 14、发射值、每步 −1 −`BlockLightAttenuation`、opaque 阻挡、fluid 额外 −1、unknown/unloaded 当阻挡；与既有客户端/Rust 光 oracle 小夹具一致；重复调用 allocations=0）并实现 `block_light_query.go`
- [ ] 4.4 失败测试：spawn 决定（active sessions 排序后 `WorldTimeTicks % n` 选锚点；splitmix64 派生半径/轴向/坐标；hash 低 8 位 <13 才尝试；24..48 距离窗；双格空气/下方 solid/非流体/loaded；night/day；light 7/8；global64；nearby 8；相同输入重放；每 tick 读取候选 ≤1）并实现 spawn 与稳定 ID（ID 同一 hash 非零；冲突重散列 ≤64 次）
- [ ] 4.5 失败测试：灼烧（露天白昼每 20 tick 扣 1、遮顶/夜间重置）、despawn（>64 格累计 600、回 50 格清零）、死亡掉落（同 tick 移除、死亡 chunk 环形尝试放 1 腐肉、全满确定性省略）；扩展 `core.FoodValue` 两食物更新后实现
- [ ] 4.6 `engine_step.go` 新增 hostile 阶段并把 spawn/burn/despawn/drop 接入；`gofmt -w internal/core internal/sim`、`make rust`、`go test ./internal/core ./internal/sim -race -count=1`；双评审后提交 `feat: simulate hostile nightwalkers`

## 5. server 有界追逐 worker 与路径执行

- [ ] 5.1 失败测试：目标选择（最近 active 同维 live player、等距按 `PlayerID` 字节序）；每 tick 至多 2 份快照（ID 最小且到期），其余顺延；快照覆盖 33×9×33 与 revisions；第三份不得读取世界
- [ ] 5.2 实现两槽 worker（复用 `companion.NewPathGrid`/`FindPath`；channel cap 2；结果按 ID 序在 tick 边界应用；旧 generation/target/revision 变化丢弃；权威 tick 只发快照不等待 A*）
- [ ] 5.3 失败测试：路径执行（目标超窗口钳到朝玩家方向窗缘可站立格；每 waypoint 前重验 revisions 与当前 cell；失效清 path 且 `NextRepathTick` 下一 tick；到 1.8 停移并冻结一次攻击意图；无路径不穿墙直线移动）并实现 `HostileAction{MoveX,MoveZ,Jump,AttackTarget}` 消费
- [ ] 5.4 极简 damage seam：sim 内 test-only 通道验证 3 伤害/20 tick 冷却（同 tick 意图冻结、按 ID 升序、既有 `applyDamage`）；`gofmt -w internal/server internal/sim`、`go test ./internal/companion ./internal/server ./internal/sim -race -count=1`；双评审后提交 `feat: drive bounded nightwalker AI`（seam 与 A-03 统一 combat 的接通交由 A-06，删除主体也归集成）

## 6. 持久化装配、启动恢复与错误路径

- [ ] 6.1 失败测试：启动错误矩阵（missing→空集合；valid→首 tick 前恢复；corrupt/future/read error→Host 构造失败不启动 tick/path worker；重复/超 64 不截断恢复）
- [ ] 6.2 实现 hostile persistence worker（jobs/completions 容量 1、revision、dirty/inFlight/retry、autosave tick、`Flush`/`Close`；保存输入是 sim 排序值快照；不抽通用 generic）
- [ ] 6.3 失败测试：非阻塞与故障注入（慢 Save 不持锁不阻 tick；失败按既有 retry；in-flight 时新快照合并 latest；shutdown flush 最新；context cancel/Sync/rename 错误保留旧正式文件并返回错误）
- [ ] 6.4 Memory/Disk 重启端到端（位置/速度/生命/冷却/目标/DistantTicks 恢复；path 不恢复且首 tick 重算；重启不清怪）；`gofmt -w internal/server`、`go test ./internal/storage ./internal/server -race -count=1`；双评审后提交 `feat: restore hostile nightwalkers`

## 7. 协议、客户端镜像插值、75-body avatar 与 client ABI v9

- [ ] 7.1 失败测试：wire 值域（三类消息各 `<ServerTick u64 + count u8 + ≤64 records>`、ID 严格升序；spawn/state/despawn 字段；拒绝重复/逆序/零 ID、NaN/Inf、非法 health/dimension、count 65、截断/尾随；Memory/TCP round trip + fuzz seed）
- [ ] 7.2 registry/packet/codec 接线（S→C 21/22/23 预留；`ValidateServerPacket`、codec_server 分支、`registry_test`）与 per-session publication（只发已订阅 chunk；进入发 spawn、逐 tick state、离开/死亡 despawn；每类每 tick ≤1 包 64 条 ID 升序；Memory/TCP transcript 一致）
- [ ] 7.3 客户端 latest-wins 镜像（固定 64 records；spawn 建立、state 只接受更新 tick、despawn 删除；未知 state 请求下一 spawn 不隐式造实体；插值复用远端时间边界）与 `presentation_conversion_test`
- [ ] 7.4 avatar 容量 11→75/66→450（`internal/render/avatar.go`、Go/Rust 上传大小、indirect offset、容量错误；`EntityHostile` kind、16-byte key 写 ID u64、6-cuboid 原创暗青/灰紫调色与不同头身比例；nametag ≤12 且永不加入 hostile）+ Rust `AVATAR_MAX_INSTANCES` 同步
- [ ] 7.5 client ABI v8→v9（`engine/include/mornlea_client.h`、`mornlea_client` ffi/lib、Go 侧校验；旧 ABI 早期拒绝）与两份基线文档仅 client ABI 版本行最小同步（`cmp -s`）+ `TestBaselineVersionsMatchCode`
- [ ] 7.6 `gofmt -w internal/network internal/server internal/client internal/render cmd/mornlea`、`make rust`、`go test ./internal/network ./internal/server ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1`；双评审后提交 `feat: present hostile nightwalkers`

## 8. 视觉场景构造与功能线终审

- [ ] 8.1 `hostile-mob` 场景构造（夜间火把边缘 8 只夜行者、1 只受击 1 只追逐；无 nametag 断言；插入 `ai-companion` 与 `water-surface-slope` 之间；不写 golden——缺失 golden 时分支模式跳过该场景逐图比对、顺序与不变量测试照常）
- [ ] 8.2 功能线验证：`make rust`、`go test ./internal/core ./internal/companion ./internal/network ./internal/storage ./internal/sim ./internal/server ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./...`、`gofmt -l .` 无输出、`openspec validate --all --strict --no-interactive`、`git diff --check`
- [ ] 8.3 独立整分支终审（规格合规 + 上限 + worker 不阻 tick + spawn 重放 + 暗度 oracle + schema 错误矩阵 + 重启 + wire 订阅 + 75-body ABI + 无 nametag + 无版权资源）；终审结论与验证输出摘要写入 `ledger.md`；提交后交集成控制器（不自行合入 main、不更新 golden）
