## Why

`findSpawnInColumn` 目前把任何有碰撞体的方块都视为满高方块，固定把脚底放在 `y+1`。耕地的碰撞顶面实际为 `15/16`，因此出生实体会悬空 `1/16` 后再沉降，并与登录恢复和 safe 点记录已经使用的完整支撑口径不一致。

## What Changes

- 出生列扫描从方块的全部碰撞盒读取真实顶面，并按由高到低的顺序考察脚底候选。
- 候选必须同时满足身体无碰撞和既有完整支撑判定，才进入原有干燥度分档。
- 玩家与伙伴继续复用同一出生扫描；玩家首次出生、登录恢复和 safe 点更新因此使用同一支撑定义。
- 增加自动化测试，覆盖耕地 `15/16` 顶面上的出生、恢复与 safe 点记录。

## Capabilities

### New Capabilities

- `authoritative-spawn-support`: 定义出生脚底取真实碰撞盒顶面，并与恢复和 safe 点的完整支撑语义保持一致。

### Modified Capabilities

- 无。

## Impact

- 受影响代码限于 `internal/sim/spawn.go` 与同主题的 `internal/sim` 测试。
- 不改变协议、任何存档 schema、engine/client ABI、benchmark scenario、配置格式或资源上限。
- 不新增依赖、goroutine、锁或持久化数据；出生扫描仍受既有候选列、世界高度与每格最多 8 个碰撞盒上限约束。
- 回退方式为整支 revert；旧存档无需迁移。

## Non-Goals

- 不修改碰撞盒注册表、物理积分、流体出生分档或候选列顺序。
- 不为未来方块形状新增通用模型、缓存或配置项。
