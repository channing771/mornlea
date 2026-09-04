# passive-mob-persistence Specification

## Purpose

定义被动牛存档 `passive_mobs.bin` 的稳定格式、校验错误矩阵、存储契约与重启恢复语义。牛的路径与寻路派生物不落盘，与夜行者存档纪律对齐但文件独立。

## Requirements

### Requirement: 文件由固定头与最多 32 条固定记录构成

归档文件 SHALL 由 32-byte 头与最多 32 条 72-byte 记录组成，文件总长 MUST 不超过 2336 bytes。头 MUST 依次包含：magic `PMST`、envelope 版本 u32（恒为 1）、schema 版本 u32（恒为 1）、revision u64、count u32、payload 长度 u32 与 CRC-32C；CRC 覆盖范围 MUST 与 `hostile_mobs.bin` 同惯例固定（头部 `[8:28]` 段与 payload 段）。记录 MUST 按 ID 严格升序且 ID MUST 非零。编码后解码 MUST 恢复出与输入逐字段一致的全部记录。

#### Scenario: 编码解码 round trip

- **GIVEN** 一个含 3 条记录（ID 非零且升序、生命/位置/目标字段合法）的保存快照
- **WHEN** 编码后解码
- **THEN** 解码结果 MUST 与输入逐字段一致，payload MUST 被完整读取且无剩余字节

#### Scenario: 第 33 条被拒绝

- **GIVEN** 一个含 33 条记录的快照
- **WHEN** 编码或解码该文件
- **THEN** 系统 MUST 拒绝整份文件，MUST NOT 部分接受前 32 条

#### Scenario: 尾随字节被拒绝

- **GIVEN** 一个合法文件后追加若干字节
- **WHEN** 解码该文件
- **THEN** 系统 MUST 拒绝整份文件

### Requirement: 损坏与越界数据被完整拒绝

解码 MUST 对以下情况返回稳定错误且不部分应用：未来 schema 或 envelope 版本、截断数据、尾随数据、CRC 不匹配、count 超过 32、重复或逆序或零 ID、未知 dimension、position/velocity/yaw 含 NaN 或 Inf、health 为 0 或大于满值、非法 bool、world Y 越界。恢复 MUST 只接受“payload 读完且无剩余”的输入。

#### Scenario: 未来 schema 与坏 CRC 被拒绝

- **GIVEN** `passive_mobs.bin` 的 schema 版本为 2，或数据与 CRC 不符
- **WHEN** 服务端启动读取该文件
- **THEN** 系统 MUST 拒绝加载，服务端 MUST 以文件错误启动失败，旧文件 MUST 原样保留

#### Scenario: 逆序与零 ID 被拒绝

- **GIVEN** 记录按 ID 递减排列，或存在 ID 为 0 的记录
- **WHEN** 解码
- **THEN** 系统 MUST 拒绝整份文件

#### Scenario: 非法生命与坐标被拒绝

- **GIVEN** 某记录 health 为 0 或超满值，或其 position 含 NaN/Inf
- **WHEN** 解码
- **THEN** 系统 MUST 拒绝整份文件

### Requirement: Memory 与 Disk 同构的存储契约

存储层 SHALL 提供加载与保存牛集合的契约；文件缺失 MUST 返回独立哨兵错误，调用方视同空集合。磁盘写入 MUST 使用同目录临时文件 + fsync + rename + 父目录 fsync，权限 0600；保存时 revision 冲突或正式文件损坏 MUST 保留旧文件并返回错误。内存实现 MUST 与磁盘实现返回相同哨兵与冲突语义。世界备份 MUST 精确复制正式 `passive_mobs.bin` 并忽略临时文件。

#### Scenario: 缺失文件视为空集合

- **GIVEN** 世界目录中没有 `passive_mobs.bin`
- **WHEN** 服务端启动加载
- **THEN** 牛 MUST 为空集合，服务端 MUST 正常启动

#### Scenario: 写入失败保留旧文件

- **GIVEN** 磁盘上存在一份合法旧 `passive_mobs.bin`
- **WHEN** 一次保存因临时文件创建、sync、rename 或目录 fsync 失败
- **THEN** 旧文件 MUST 仍为原内容，保存 MUST 返回错误，MUST NOT 声称成功

#### Scenario: 备份包含正式文件并忽略临时文件

- **GIVEN** 世界目录中有 `passive_mobs.bin` 与一个按既有临时文件命名约定的临时文件
- **WHEN** 执行世界备份
- **THEN** 备份目录 MUST 含正式 `passive_mobs.bin`，MUST NOT 含临时文件

### Requirement: 重启恢复正常路线且重启不可能清牛

服务端启动 MUST 在第一个权威 tick 前恢复牛集合；正常关服重启后全部牛的位置、速度、生命 MUST 逐字段保留，路径 MUST 为空且首 tick 排入重算。启动读取到缺失、损坏或未来版本文件时，服务端 MUST 以错误启动失败且不得把已有记录当空集合覆盖回写——重启 MUST NOT 成为清牛手段。

#### Scenario: 重启恢复全部记录

- **GIVEN** 关服时存在 3 头牛（含位置与生命状态）
- **WHEN** 服务端重新启动并恢复
- **THEN** 首 tick 前 MUST 恢复 3 头，逐字段与关服一致（路径除外，首 tick 重算）

#### Scenario: 损坏文件不能以空集合覆盖

- **GIVEN** 磁盘上 `passive_mobs.bin` 损坏或为未来版本
- **WHEN** 服务端启动
- **THEN** 启动 MUST 失败且 MUST NOT 写入任何覆盖旧文件的保存
