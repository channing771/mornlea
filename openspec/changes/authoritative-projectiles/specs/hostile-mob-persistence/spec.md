# Spec: hostile-mob-persistence

## MODIFIED Requirements

### Requirement: 文件由固定头与最多 64 条固定记录构成

归档文件 SHALL 由 32-byte 头与最多 64 条 73-byte 记录组成，文件总长 MUST 不超过 4704 bytes。头 MUST 依次包含：magic `MHST`、envelope 版本 u32（恒为 1）、schema 版本 u32（当前写侧恒为 2）、revision u64、count u32、payload 长度 u32 与 CRC-32C；CRC 覆盖范围 MUST 按既有 `companion` 存档惯例固定（头部 `[8:28]` 段与 payload 段）。每条记录在既有 72-byte 字段之后 MUST 携带 1-byte 敌怪 kind（0=夜行者、1=掷骨者），值域 MUST 为 {0,1}。记录 MUST 按 ID 严格升序且 ID MUST 非零。编码后解码 MUST 恢复出与输入逐字段一致的全部记录。schema 1 的旧文件 MUST 仍被读入口放行并以只读迁移方式恢复（kind 恒为 0=夜行者），写侧 MUST 只写 schema 2；升级 MUST 不要求任何主动迁移动作，首次保存自然落盘 v2。

#### Scenario: 编码解码 round trip

- **GIVEN** 一个含 3 条记录（ID 非零且升序、生命/冷却/目标/kind 字段合法）的保存快照
- **WHEN** 编码后解码
- **THEN** 解码结果 MUST 与输入逐字段一致，payload MUST 被完整读取且无剩余字节

#### Scenario: 第 65 条被拒绝

- **GIVEN** 一个含 65 条记录的快照
- **WHEN** 编码或解码该文件
- **THEN** 系统 MUST 拒绝整份文件，MUST NOT 部分接受前 64 条

#### Scenario: 尾随字节被拒绝

- **GIVEN** 一个合法文件后追加若干字节
- **WHEN** 解码该文件
- **THEN** 系统 MUST 拒绝整份文件

#### Scenario: v1 旧文件只读迁移

- **GIVEN** 一份合法的 schema 1 旧存档（72-byte 记录、无 kind 字节）
- **WHEN** 服务端启动读取该文件
- **THEN** 系统 MUST 正常恢复全部记录且 kind 恒为 0，旧文件 MUST 原样保留直至下次正常保存升为 v2

### Requirement: 损坏与越界数据被完整拒绝

解码 MUST 对以下情况返回稳定错误且不部分应用：未来 schema（大于当前写侧版本的 schema 值）或未来 envelope 版本、截断数据、尾随数据、CRC 不匹配、count 超过 64、重复或逆序或零ID、未知 dimension、position/velocity/yaw 含 NaN 或 Inf、health 为 0 或大于 20、非法 bool、无目标却携带 PlayerID、有目标但 PlayerID 不是合法 UUIDv4、cooldown/burn/despawn 越界、world Y 越界、kind 不在值域内。恢复 MUST 只接受“payload 读完且无剩余”的输入。

#### Scenario: 未来 schema 与坏 CRC 被拒绝

- **GIVEN** `hostile_mobs.bin` 的 schema 版本大于当前写侧版本，或数据与 CRC 不符
- **WHEN** 服务端启动读取该文件
- **THEN** 系统 MUST 拒绝加载，服务端 MUST 以文件错误启动失败（见存储契约场景），旧文件 MUST 原样保留

#### Scenario: 逆序与零 ID 被拒绝

- **GIVEN** 记录按 ID 递减排列，或存在 ID 为 0 的记录
- **WHEN** 解码该文件
- **THEN** 系统 MUST 拒绝整份文件

#### Scenario: 非法 kind 被拒绝

- **GIVEN** 某条记录的 kind 字节为 2
- **WHEN** 解码该文件
- **THEN** 系统 MUST 拒绝整份文件，MUST NOT 部分接受其余记录
