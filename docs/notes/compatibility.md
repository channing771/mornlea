# 兼容性与升级

本文承接 README「兼容性与升级」一节，给出线上协议、世界/玩家/伙伴存档的版本契约、升级前后的备份与回退纪律，以及 benchmark 报告兼容规则；全部条目以当前代码与 `openspec/specs/` 为准。

## 线上协议

- 线上协议为 v29（`internal/network` 的 `ProtocolVersion`）；所有不匹配版本都会在握手阶段、进入 Play 前被稳定拒绝，不提供版本协商或降级解码；更早版本的逐版语义见 `internal/network/packet.go` 顶部注释；
- 近几版协议全部是既有 packet 尾部追加或新增单条消息，不改变任何长度上限，也不新增 `RejectReason`：
  - v26 新增 Play S→C ID 20 `PlaceBlockSucceeded(sequence)`，只回发给放置发起会话作为成功放置确认；
  - v27 新增 Play C→S ID 14 `BoneMeal`，与 `TillSoil` 同形（序号 + 朝向），目标格由权威射线决定；
  - v28 在 `PlayerInput` 尾部 `Eating` 之后追加 1 字节 `Sprinting` 疾跑意图位；
  - v29 在 `PlayerState` 尾部 `Hunger` 之后、`WorldTimeTicks` 之前追加 1 字节 `SaturationZero` 饱和度归零提示位；
- 客户端只声明按键意图，权威结算全在服务端；`SaturationZero` 是瞬态提示位，不入任何存档，饱和度与疲劳数值是纯服务端量、不占 wire 字段。

## 存档版本

- 世界 metadata 保持 v2，记录绝对世界时间；既有 v1 世界可直接打开，世界时间从 `0` 开始，并在下一次正常自动保存或关服时写为 v2；只认识旧版本的程序遇到未来 metadata 必须稳定拒绝且不得覆盖原文件；
- 玩家存档保持 schema v7：受支持的 v1..v6 沿既有迁移链读取（例如 v4 一律补为满血、v6 按新玩家初值补齐三层饥饿状态），迁移结果在下一次正常保存时改写为当前版本；未来版本必须稳定拒绝且不得覆盖；
- 区块存档保持 schema v9：v1..v8 沿既有迁移链读取，其中 v8→v9 是恒等迁移，不为旧区块追注水，已接受的代价是新旧区块之间出现干湿边界；未来版本必须稳定拒绝且不得覆盖；
- 伙伴状态独立写入世界根目录的 `companions.ai` schema v4：v1..v3 只读迁移；active 与 inactive 身体记录合计最多 64 条；active 记录可持久化当前任务、FIFO 与近期对话摘要，名称和生效 persona 始终来自当前配置；
- 列顶高度表、天空光和静态方块光只从权威方块镜像派生，不写入区块、玩家或伙伴存档，也不进入网络 payload；程序化天空只消费权威世界时间。

## 备份与回退

- 升级前必须正常关服，等待玩家、伙伴与世界存储刷写完成并备份完整世界目录，再启动新版本程序；
- 回退时必须先停服，再恢复升级前的完整备份；不承诺把 schema v9 区块、schema v7 玩家档、`companions.ai` v4 或新物品降级写回，不能让旧程序直接打开已升级目录后继续写入；
- 异常退出时玩家、伙伴与区块文件各自原子，但它们之间没有跨文件事务。

## benchmark 报告兼容

- benchmark producer 为 scenario v19，固定输入仍是七名远端玩家、零伙伴、不注入聊天，被测世界不注水且不含农业方块；scenario 版本变化记录的是被测进程本身的改变（HUD 固定上传布局、HUD 图集列数、权威 tick 工作量等），性能数值只在同 scenario 版本内可比；
- 旧 scenario 版本的报告仍可读取并做同版本比较；跨 scenario 比较只接受显式 `--allow-scenario-upgrade 18:19`，这是当前唯一显式迁移授权；
- 跨 transport 比较要求两侧 scenario 版本与 `git_commit` 都一致，否则拒绝；
- 性能数值只记录，不改变退出状态；报告结构、来源身份、真实 overflow、数据丢失和 I/O 错误仍然硬失败。
