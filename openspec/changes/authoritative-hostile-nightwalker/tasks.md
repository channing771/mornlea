# Tasks：authoritative-hostile-nightwalker

> 执行规范：每 Task 派发全新 implementer 子代理（brief 自包含）；TDD（red→green→refactor）；Task 完成后 SPEC+QUALITY 双评审，修复 ≤5 轮（R≤3 原实现者、R≥4 换新）；结论记入 `ledger.md`。2026-08-28 重定基线：批次合流模式已取消，协议 v30、client ABI v10、消息编号与 golden 由本行自带；编号实现期以注册表实占空闲位为准，撞号按 A-02 先例由后合并者重订。

## 1. 基线验证与 change 产物核对

- [x] 1.1 运行 `git status --short`（worktree 干净）并记录 `make rust`、`go test ./internal/core ./internal/companion ./internal/physics ./internal/sim ./internal/server ./internal/storage ./internal/network ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1` 输出摘要到 `ledger.md`（数值只记录）
- [x] 1.2 检查 `openspec validate --all --strict --no-interactive` 与 `git diff --check` 通过；核对 proposal/specs/design/tasks 与已批准的确认结论一致（原 A-04-q1/q2 裁决与 2026-08-28 重定基线裁决全文誊入 ledger）

## 2. 发光/衰减单一表迁入 core 与 DisplayDayPhase、腐肉食物

- [x] 2.1 核对 `core.BlockEmission`/`core.BlockLightAttenuation`（`internal/core/block_properties.go` 现行表）签名与值并记录消费结论；为 `core.BlockOpaque(block core.BlockID) bool` 写失败测试（不透明谓词与既有 `assets.Registry.Opaque` 逐值一致——registered 且非 air/glass/leaves/fluid/作物）
- [x] 2.2 实现 `core.BlockOpaque` 并把 `internal/assets`（`Registry.Opaque`）与 `internal/mesh` 的不透明判定改为委托 core；核对两表既有委托关系不回退；`go test ./internal/core ./internal/assets ./internal/mesh -race -count=1`
- [x] 2.3 在 `internal/core` 新增 overflow-safe `DisplayDayPhase(worldTime uint64, offset uint16) uint16`（先 `%24000` 再相加取模）失败测试（边界：MaxUint64、offset 23999）并实现；offset 参数是与 A-05（床与睡眠）的共享契约，本行消费恒 0；`go test ./internal/core -race -count=1`
- [x] 2.4 在 `internal/core` 登记 `ItemRottenFlesh`（堆叠 64，编号接 `ItemTorch`=44 之后、哨兵顺延）与食物表条目（饥饿 4、饱和 0 毫秒），把食物表穷举测试更新为精确五种食物（面包 5/6000、马铃薯 1/600、胡萝卜 3/3600、毒土豆 2/1200、腐肉 4/0）并覆盖进食状态机取值；`go test ./internal/core -race -count=1`；本任务组完成后提交 `feat: add core block opacity day phase and rotten flesh`

## 3. hostile_mobs.bin schema v1 与存储契约

- [x] 3.1 失败测试：文件布局（32-byte 头 magic `MHST`/envelope 1/schema 1/revision u64/count u32/payloadLen u32/CRC-32C；CRC 覆盖 `data[8:28]`+payload；≤64×72-byte 记录；总长 ≤4640 bytes；记录 ID 非零严格升序；round trip）
- [x] 3.2 失败测试：字段校验矩阵（future schema/envelope、截断、尾随、坏 CRC、count>64、重复/逆序/零 ID、未知 dimension、NaN/Inf、health 0 或 >20、非法 bool、无目标却带 PlayerID、有目标非 UUIDv4、cooldown/burn/despawn 越界、position.Y 不在 `[core.MinY, core.MaxY)`、payload 读空）
- [x] 3.3 实现固定 binary codec（复用既有 `appendU32`/`byteDecoder`/CRC helper；不使用 JSON/gob/reflection；`physics.State` 只编码 position/velocity/onGround；保留字段零）
- [x] 3.4 实现 `HostileMobStore`（`LoadHostileMobs`/`SaveHostileMobs`；missing 返回独立 `ErrHostileMobsNotFound`）与 Memory/Disk 契约（temp+fsync+rename、0600、revision 冲突与损坏保护、`hostile_mobs.bin` 路径、WorldStore 组合、backup 复制正式文件忽略 temp）
- [x] 3.5 `hostile_codec_fuzz_test.go`/故障注入（截断、bit-flip 不 panic；golden round trip）；`gofmt -w internal/storage`、`go test ./internal/storage -race -count=1`；SPEC+QUALITY 双评审后提交 `feat: persist hostile nightwalkers`（评审写入 ledger）

## 4. sim 夜行者身体、spawn、局部暗度与生命周期

- [x] 4.1 失败测试：固定集合（插入/恢复按 ID 排序、重复/第 65 只拒绝、`hostileState` 容量 64）；与玩家/伙伴同一 per-actor `physics.Step`（顺序玩家→伙伴→夜行者 ID 序、复用 `SubmersionFlags` 与同一 tunables）
- [x] 4.2 实现最小 `hostileState`（排序 slice + 复用 scratch + restore/snapshot/测试入口；不建 map/ECS；AABB 与玩家相同、输入仅水平+jump）
- [x] 4.3 失败测试：预分配 29³ 16-bucket BFS 局部区块光（半径 14、发射值、每步 −1 −`BlockLightAttenuation`、`core.BlockOpaque` 阻挡、fluid 额外 −1、unknown/unloaded 当阻挡；与既有客户端/Rust 光 oracle 小夹具逐位一致——真实差异必须记录并裁决，不得静默采用两套规则；重复调用 allocations=0）并实现 `block_light_query.go`
- [x] 4.4 失败测试：spawn 决定（active sessions 排序后 `WorldTimeTicks % n` 选锚点；splitmix64 派生半径/轴向/坐标；hash 低 8 位 <13 才尝试；24..48 距离窗；双格空气/下方 solid/非流体/loaded；night/day；light 7/8；global64；nearby 8；相同输入重放；每 tick 读取候选 ≤1）并实现 spawn 与稳定 ID（ID 同一 hash 非零；冲突重散列 ≤64 次）
- [x] 4.5 失败测试：灼烧（露天白昼每 20 tick 扣 1、遮顶/夜间重置）、despawn（>64 格累计 600、回 50 格清零）、死亡掉落（同 tick 移除、死亡 chunk 环形尝试放 1 腐肉、全满确定性省略）；扩展 `core.FoodValue` 两食物更新后实现
- [x] 4.6 `engine_step.go` 新增 hostile 阶段并把 spawn/burn/despawn/drop 接入；`gofmt -w internal/core internal/sim`、`make rust`、`go test ./internal/core ./internal/sim -race -count=1`；双评审后提交 `feat: simulate hostile nightwalkers`

## 5. server 有界追逐 worker 与路径执行

- [ ] 5.1 失败测试：目标选择（最近 active 同维 live player、等距按 `PlayerID` 字节序）；每 tick 至多 2 份快照（ID 最小且到期），其余顺延；快照覆盖 33×9×33 与 revisions；第三份不得读取世界
- [ ] 5.2 实现两槽 worker（复用 `companion.NewPathGrid`/`FindPath`；channel cap 2；**投递 MUST 非阻塞 select**——满槽时该 mob 本次顺延、下一 tick 重规划；结果按 ID 序在 tick 边界应用；已过期（generation/target/revision 变化）的结果丢弃；权威 tick 只发快照不等待 A*；补满槽非阻塞用例）
- [ ] 5.3 失败测试：路径执行（目标超窗口钳到朝玩家方向窗缘可站立格；每 waypoint 前重验 revisions 与当前 cell；失效清 path 且 `NextRepathTick` 下一 tick；到 1.8 停移并冻结一次攻击意图；无路径不穿墙直线移动）并实现 `HostileAction{MoveX,MoveZ,Jump,AttackTarget}` 消费
- [ ] 5.4 极简 damage seam：sim 内 test-only 通道验证 3 伤害/20 tick 冷却（同 tick 意图冻结、按 ID 升序、既有 `applyDamage`）；`gofmt -w internal/server internal/sim`、`go test ./internal/companion ./internal/server ./internal/sim -race -count=1`；双评审后提交 `feat: drive bounded nightwalker AI`（seam 待 A-03 统一战斗落地后收编删除）

## 6. 持久化装配、启动恢复与错误路径

- [ ] 6.1 失败测试：启动错误矩阵（missing→空集合；valid→首 tick 前恢复；corrupt/future/read error→Host 构造失败不启动 tick/path worker；重复/超 64 不截断恢复）
- [ ] 6.2 实现 hostile persistence worker（jobs/completions 容量 1、revision、dirty/inFlight/retry、autosave tick、`Flush`/`Close`；保存输入是 sim 排序值快照；不抽通用 generic）
- [ ] 6.3 失败测试：非阻塞与故障注入（慢 Save 不持锁不阻 tick；失败按既有 retry；in-flight 时新快照合并 latest；shutdown flush 最新；context cancel/Sync/rename 错误保留旧正式文件并返回错误）
- [ ] 6.4 Memory/Disk 重启端到端（位置/速度/生命/冷却/目标/DistantTicks 恢复；path 不恢复且首 tick 重算；重启不清怪）；`gofmt -w internal/server`、`go test ./internal/storage ./internal/server -race -count=1`；双评审后提交 `feat: restore hostile nightwalkers`

## 7. 协议、客户端镜像插值、75-body avatar 与 client ABI v10

- [ ] 7.1 失败测试：wire 值域（三类消息各 `<ServerTick u64 + count u8 + ≤64 records>`、ID 严格升序；spawn/state/despawn 字段；拒绝重复/逆序/零 ID、NaN/Inf、非法 health/dimension、count 65、截断/尾随；Memory/TCP round trip + fuzz seed）
- [ ] 7.2 registry/packet/codec 接线（S→C 22/23/24，实现期以注册表实占空闲位为准；`ProtocolVersion` 29→30；`ValidateServerPacket`、codec_server 分支、`registry_test`）与 per-session publication（只发已订阅 chunk；进入发 spawn、逐 tick state、离开/死亡 despawn；每类每 tick ≤1 包 64 条 ID 升序；Memory/TCP transcript 一致）
- [ ] 7.3 客户端 latest-wins 镜像（固定 64 records；spawn 建立、state 只接受更新 tick、despawn 删除；未知 state 请求下一 spawn 不隐式造实体；插值复用远端时间边界）与 `presentation_conversion_test`
- [ ] 7.4 avatar 容量 11→75/66→450（`internal/render/avatar.go`、Go/Rust 上传大小、indirect offset、容量错误；`EntityHostile` kind、16-byte key 写 ID u64、6-cuboid 原创暗青/灰紫调色与不同头身比例；nametag ≤12 且永不加入 hostile）+ Rust `AVATAR_MAX_INSTANCES` 同步
- [ ] 7.5 client ABI v9→v10（`engine/include/mornlea_client.h`、`mornlea_client` ffi/lib、Go 侧校验；旧 ABI 早期拒绝）与两份基线文档协议 v30、client ABI v10、`hostile_mobs` v1 版本行同步（两份逐字节相同，`cmp -s`）+ `TestBaselineVersionsMatchCode`
- [ ] 7.6 `gofmt -w internal/network internal/server internal/client internal/render cmd/mornlea`、`make rust`、`go test ./internal/network ./internal/server ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1`；双评审后提交 `feat: present hostile nightwalkers`

## 8. 视觉场景构造与功能线终审

- [ ] 8.1 `hostile-mob` 场景构造（夜间火把边缘 8 只夜行者、1 只受击 1 只追逐；无 nametag 断言；插入 `ai-companion` 与 `water-surface-slope` 之间）并生成本场景 golden PNG（torch-night 先例；场景总数 21→22 口径），`visual-check` 对全表比对全绿
- [ ] 8.2 功能线验证：`make rust`、`go test ./internal/core ./internal/companion ./internal/network ./internal/storage ./internal/sim ./internal/server ./internal/client ./internal/render ./internal/nativeabi ./cmd/mornlea -race -count=1`、`go test ./... -race`（合并前全量）、`go test ./internal/archcheck -count=1`、`go vet ./...`、`gofmt -l .` 无输出、`openspec validate --all --strict --no-interactive`、`git diff --check`；另记录 benchmark scenario v19 或 `cmd/perfcheck` 输出摘要（tick 热路径变更，数值只记录、不改基线）
- [ ] 8.3 独立整分支终审（规格合规 + 上限 + worker 不阻 tick + spawn 重放 + 暗度 oracle + schema 错误矩阵 + 重启 + wire 订阅 + 75-body ABI + 无 nametag + 无版权资源）；终审结论与验证输出摘要写入 `ledger.md`；推送分支开 PR，评审通过后合并 main（含 golden 与基线文档版本行）
