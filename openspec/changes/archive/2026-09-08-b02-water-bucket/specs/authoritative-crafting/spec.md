# authoritative-crafting Delta

## MODIFIED Requirements

### Requirement: 固定配方具有稳定语义

系统 SHALL 定义二十条稳定固定形状配方（既有十九条不变，追加 `20`），recipe ID、形状、原料、数量和产物 MUST 由服务端定义，客户端不得声明或覆盖这些值。形状以裁边后的宽高与非空格序列表达，匹配遵守 `authoritative-grid-crafting` 的裁边与水平镜像规则。既有 `1..19` 与既有场景全部不变，追加：

- recipe ID `20`：裁边后 3×2、左中/右中/底中 3 个铁锭（顶行空已裁掉），产出 1 个空桶。

recipe ID 只用于注册表与 UI 身份，新路径的线上消息 MUST NOT 携带 recipe ID，recipe ID MUST NOT 落盘。相同初始状态与命令序列经 Memory 和 TCP MUST 得到相同结果。

#### Scenario: 空桶配方可查询

- **WHEN** 系统读取 recipe ID `20`
- **THEN** 该配方稳定表示左中/右中/底中 3 个铁锭转换为 1 个空桶

#### Scenario: 既有配方编号不因新增而位移

- **GIVEN** 追加 recipe `20` 后的配方表
- **WHEN** 系统查询 recipe ID `1`..`19`
- **THEN** 每个配方 MUST 返回与追加前逐项一致的形状与产物
