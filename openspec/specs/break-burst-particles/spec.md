# break-burst-particles Specification

## Purpose
为图形客户端定义方块被破坏后在原位置呈现的短暂破碎粒子反馈：以新出现的权威掉落物为唯一触发来源，粒子轨迹与寿命由掉落物 ID 与 tick 确定性推导，不引入协议变更与本地随机源。
## Requirements
### Requirement: 方块破坏后在原位置呈现确定性破碎粒子

图形客户端 SHALL 在镜像中首次出现某权威掉落物 ID 时，在该掉落物的方块位置生成一次破碎 burst：8 粒 solid 小方块，颜色 MUST 为该掉落物品的既有物品基色；粒子位置 MUST 由掉落物 ID 整数散列派生的初速与 tick 年龄按固定重力公式逐帧推导，客户端 MUST NOT 引入 `math/rand` 全局源、时间或帧计数；粒子高度 MUST 以同一掉落物的支撑高度为地板钳制；burst 年龄达到 20 tick 时 MUST 停止编码。burst 跟踪表 MUST 定界（最近 16 个掉落物 ID，环形淘汰），掉落物消失即删除对应条目；待编码粒子总数 MUST 以 64 实例为上限，超出时淘汰最老的 burst。无掉落物产生的方块移除 MUST NOT 产生粒子。

#### Scenario: 新掉落物触发同色 burst

- **GIVEN** 镜像中首次出现掉落物 ID 为 D、物品为泥土、位置为 P
- **WHEN** 客户端编码当前帧
- **THEN** P 处 MUST 出现 8 粒泥土基色小方块
- **AND** 8 粒的初速方向 MUST 互不相同且以下半球向外为主（含水平外向分量），重力作用下全程以下坠为主
- **AND** 8 粒的初始位置 MUST 分布在原方块体积内（非同一点），破裂首帧即在原方块处可见

#### Scenario: 粒子年龄到期消失

- **GIVEN** 同一掉落物 ID 的 burst 已持续 19 tick
- **WHEN** tick 再推进 1
- **THEN** 该 burst MUST 不再编码任何粒子

#### Scenario: 同输入逐帧一致

- **GIVEN** 相同的掉落物 ID 集合与 tick
- **WHEN** 重复编码两帧
- **THEN** 全部粒子实例字节 MUST 完全一致

#### Scenario: 容量超限淘汰最老 burst

- **GIVEN** 跟踪表已满 16 个 burst 且有第 17 个新掉落物出现
- **WHEN** 客户端编码当前帧
- **THEN** 最老的 burst MUST 被淘汰且其粒子不再编码
- **AND** 编码的粒子总数 MUST NOT 超过 64 实例
