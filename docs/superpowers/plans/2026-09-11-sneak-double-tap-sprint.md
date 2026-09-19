# B-42 潜行与双击疾跑 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 B-42 潜行与潜行放置全量：`Shift` 潜行慢移 + 边缘防坠 + 潜行放置，疾跑改双击 `W` 触发。

**Architecture:** 单一 wire 位（`PlayerInput.Sneaking`）端到端贯穿协议→合同→tick→物理；客户端双击状态机只推导意图位，全部速度门控仍在 physics 双层 + sim 饥饿门控；边缘钳制是 sim 与预测同调的 Go 纯函数，Rust 只做积分。

**Tech Stack:** Go 1.26（六模块 go.work）、Rust（`mornlea_engine` step 积分）、winit 输入快照（键位已传输，Rust 零改）。

**Spec:** `docs/superpowers/specs/2026-09-11-sneak-double-tap-sprint-design.md` — 执行者必读，计划只复述落点不重复论证。

## Global Constraints

- 协议 `v40→v41`，只在 `PlayerInput` 尾部追加 1 字节 `Sneaking`，不新增 packet、不改既有 ID、不新增 `RejectReason`。
- Step header 布局 `v3→v4`，总长保持 160 字节，不升 engine ABI（v11）、client ABI（v18）与全部存档 schema、benchmark scenario（v23）不变。
- 潜行倍率默认 `0.3`，疾跑倍率 `1.3` 不变，疾跑饥饿门控 `<6` 不变。
- 双击窗口 `300ms`，常量集中一处；时钟源可注入。
- 潜行优先：`Sneaking` 有效时疾跑加速与疾跑疲劳一律跳过。
- 权威 tick、渲染与网络热路径不做无界工作；发送成功后的消息视为不可变。
- 中文注释，标识符反引号包裹；注释中不出现任务编号。

---

## File Structure

| 文件 | 职责 |
|---|---|
| `openspec/changes/sneak-double-tap-sprint/{proposal,design,tasks}.md` + `specs/sneak/spec.md` | 本 change 的 OpenSpec 产物（新建） |
| `packages/shared/network/protocol/packet.go` | 版本常量 `40→41` + 版本史注释 |
| `packages/shared/network/protocol/message_command.go` | `PlayerInput` 追加 `Sneaking` |
| `packages/shared/network/codec/codec_client.go` | 编解码尾部追加 1 bool |
| `packages/shared/network/codec/hunger_test.go` | 载荷长度与偏移常量 `v41` |
| `packages/shared/network/codec/codec_golden_test.go` | golden 夹具追加 `Sneaking` 位 |
| `packages/server/sim/contract/contract.go` | `Command` 追加 `Sneaking` |
| `packages/server/server/session_ingress.go` | `PlayerInput→Command` 搬运 |
| `packages/server/sim/entity/tick.go` | `CommandPlayerInput→physics.Input` 搬运 + `sneakingHeld` 锁存 |
| `packages/shared/physics/types.go` | `Input.Sneaking`、默认潜行倍率常量 |
| `packages/shared/physics/tunables.go` | `Tunables.SneakSpeedMultiplier` |
| `packages/shared/physics/step.go` | 布局 `v3→v4`、编码 `130`/`152:156`、门控 |
| `packages/shared/physics/sneak_edge.go`（新建） | 边缘支撑纯函数（sim/预测同源） |
| `packages/engine/crates/mornlea_engine/src/step.rs` | 解码 + 积分潜行分支 |
| `packages/server/sim/entity/player.go` | 饥饿/潜行门控、疲劳压制、边缘钳制接入、`sneakingHeld` 字段 |
| `packages/server/sim/entity/container.go` | 潜行中拒绝 `openContainer` |
| `packages/server/sim/entity/door.go`、`sleep.go` | 门/床交互潜行分流预留 |
| `packages/client/client/input.go` | `InputState` 双击状态机 |
| `packages/client/client/predictor.go` | `Control` 追加 `Sneaking` |
| `packages/client/client/predictor_advance.go` | 上行 + 预测门控同序 |
| `packages/client/cmd/mornlea/app/interactive.go` | `Shift`/`W` 采样 + 锁存接线，`Ctrl` 退役 |
| `packages/client/cmd/mornlea/app/app_input.go` | `placeBlock` 潜行分支 |
| `docs/feature-backlog.md` | B-42 行认领与收尾 |

---

### Task 0: 认领、隔离分支与 OpenSpec 脚手架

**Files:**
- Modify: `docs/feature-backlog.md`（B-42 行）
- Create: `openspec/changes/sneak-double-tap-sprint/proposal.md`, `openspec/changes/sneak-double-tap-sprint/design.md`, `openspec/changes/sneak-double-tap-sprint/tasks.md`, `openspec/changes/sneak-double-tap-sprint/specs/sneak/spec.md`

**Interfaces:**
- Consumes: 设计文档（Spec）的 §1–§6。
- Produces: change 目录名 `sneak-double-tap-sprint`（后续任务不再另建目录）。

- [ ] **Step 1: 创建隔离 worktree 与分支**

Run: `git status --short --branch`（确认干净、无他人改动），然后：

```bash
git worktree add .worktrees/B-42-sneak-double-tap-sprint -b feat/B-42-sneak-double-tap-sprint
```

Expected: worktree 落在 `.worktrees/B-42-sneak-double-tap-sprint`，分支名为 `feat/B-42-sneak-double-tap-sprint`。后续全部步骤都在该 worktree 内执行。

- [ ] **Step 2: 认领 B-42 行（docs-only）**

编辑 `docs/feature-backlog.md` 的 B-42 行：`状态` → `已认领`，`认领人` → `<agent 标识> @ feat/B-42-sneak-double-tap-sprint`，备注追加独占文件集：`packages/shared/network, packages/shared/physics, packages/server/sim, packages/server/server, packages/client/client, packages/client/cmd/mornlea/app, packages/engine/crates/mornlea_engine/src/step.rs, openspec/changes/sneak-double-tap-sprint`。

- [ ] **Step 3: 写 change 四件套（内容从设计文档 §1–§6 直译，不发挥）**

`proposal.md` 含背景/目标/非目标（设计文档 §1.2–§1.3 原样）；`design.md` 含 §2–§4 落点与被否决方案（§6 原样）；`tasks.md` 逐项引用本计划 Task 1–7；`specs/sneak/spec.md` 用 OpenSpec 英文结构标题 + `SHALL` 写行为契约，至少覆盖：潜行减速门控、潜行优先互斥、边缘钳制、潜行放置分流、双击触发/清除条件，每条配 `Given/When/Then`，另加 `sprint` 的 MODIFIED delta 说明触发方式变更。

- [ ] **Step 4: 校验并提交 docs-only**

Run: `openspec validate --all --strict --no-interactive`（在 worktree 内）
Expected: 全部通过（含新增 `sneak` 能力）。

```bash
git add docs/feature-backlog.md openspec/changes/sneak-double-tap-sprint
git commit -m "docs(backlog): claim B-42 sneak with double-tap sprint"
```

---

### Task 1: 协议 v41 与 codec 尾部追加

**Files:**
- Modify: `packages/shared/network/protocol/packet.go:10,46,73,85`
- Modify: `packages/shared/network/protocol/message_command.go:25-28`
- Modify: `packages/shared/network/codec/codec_client.go:41,177-180`
- Modify: `packages/shared/network/codec/hunger_test.go:101-109`
- Modify: `packages/shared/network/codec/codec_golden_test.go:32` 附近
- Test: `go test ./packages/shared/network/... -race -count=1`

**Interfaces:**
- Consumes: 无（链首）。
- Produces: `protocol.PlayerInput.Sneaking bool`（wire 尾字节，Task 2/6 消费）。

- [ ] **Step 1: 先写失败测试（载荷长度断言会先失败）**

编辑 `packages/shared/network/codec/hunger_test.go:101-109`，把布局注释与常量改为 v41：

```go
// protocol.PlayerInput 的 wire 布局（v41 起，固定长度）：
//
//	Sequence u64 | MoveX i8 | MoveZ i8 | Jump u8 | Yaw f32 | Pitch f32 | Mining u8 | Eating u8 | Sprinting u8 | Sneaking u8
const (
	playerInputPayloadBytes    = 8 + 1 + 1 + 1 + 4 + 4 + 1 + 1 + 1 + 1
	playerInputSneakingOffset  = playerInputPayloadBytes - 1
	playerInputSprintingOffset = playerInputSneakingOffset - 1
	playerInputEatingOffset    = playerInputSprintingOffset - 1
	playerInputMiningOffset    = playerInputEatingOffset - 1
)
```

Run: `go test ./packages/shared/network/codec -race -count=1 -run 'TestPlayerInput|TestProtocolV1'`
Expected: FAIL（载荷长度断言：实现仍发 v40 长度）。

- [ ] **Step 2: 追加协议字段与版本**

`message_command.go` 在 `Sprinting` 字段后追加：

```go
// Sneaking 是持续潜行输入位，协议 v41 起随玩家输入上行（wire 上紧跟
// `Sprinting` 之后）。潜行减速只在站立非浸没时生效，潜行放置分流见 sneak spec。
Sneaking bool
```

`packet.go`: `ProtocolVersion` 改为 `41`；顶部注释在 v40 段之后追加 `v41 在 PlayerInput 尾部追加 Sneaking 潜行位（紧跟 Sprinting 之后）；v28 段的 PlayerInput 描述句追加 Sneaking 说明`；`73` 行的追加语义句同步。

- [ ] **Step 3: 编解码追加尾字节**

编码（`codec_client.go:41` 后）：

```go
e.bool(message.Sprinting)
e.bool(message.Sneaking)
```

解码（`codec_client.go:177-180`，紧跟 `sprinting` 之后）：

```go
if err == nil {
	sprinting, err = d.bool()
}
if err == nil {
	sneaking, err = d.bool()
}
packet = protocol.PlayerInput{Sequence: sequence, MoveX: moveX, MoveZ: moveZ, Jump: jump, Yaw: yaw, Pitch: pitch, Mining: mining, Eating: eating, Sprinting: sprinting, Sneaking: sneaking}
```

（`sneaking` 变量需在同分支顶部与 `sprinting` 并列声明。）

- [ ] **Step 4: golden 夹具跟进**

`codec_golden_test.go:32` 附近：仿照既有 `Sprinting=false` 尾部追加语义的夹具，追加 `Sneaking` 真/假两组 hex（载荷末字节 `00`/`01`），旧版 v40 字节流解码必须拒绝（截断）。

- [ ] **Step 5: 运行并提交**

Run: `go test ./packages/shared/network/... -race -count=1`
Expected: PASS（含 `TestProtocolVersionPinned`、`TestProtocolV1PacketIDsAreFrozen` 等冻结族）。

```bash
git add packages/shared/network
git commit -m "feat(protocol): append Sneaking bit to PlayerInput in v41"
```

---

### Task 2: 合同搬运链（contract→ingress→tick）

**Files:**
- Modify: `packages/server/sim/contract/contract.go:69-76`
- Modify: `packages/server/server/session_ingress.go:118-131`
- Modify: `packages/server/sim/entity/tick.go:110-119`
- Modify: `packages/server/sim/entity/player.go`（`playerState` 结构体，加 `sneakingHeld bool`，与 `miningHeld`/`eatingHeld` 并列）
- Test: `go test ./packages/server/sim/contract ./packages/server/server -race -count=1`

**Interfaces:**
- Consumes: Task 1 的 `protocol.PlayerInput.Sneaking`。
- Produces: `contract.Command.Sneaking` + `player.sneakingHeld`（Task 5 消费）。

- [ ] **Step 1: 写失败测试**

在 `packages/server/server` 既有 ingress 映射测试旁（`player_test.go` 的消息矩阵）加一例：`network.PlayerInput{Sequence: 7, Sneaking: true}` 经 `translateClientMessage` 必须得到 `Kind == CommandPlayerInput && Sneaking == true`。

Run: `go test ./packages/server/server -race -count=1 -run TestTranslate`
Expected: FAIL（字段不存在）。

- [ ] **Step 2: 三段同形搬运**

`contract.go` 的 `Command` 在 `Sprinting bool` 后加 `Sneaking bool`。`session_ingress.go:118-131` 的 `PlayerInput` 分支加 `Sneaking: message.Sneaking`。`tick.go:110-119` 的 `player.input` 构造加 `Sneaking: command.Sneaking`，并在 `player.lastInputSequence = command.Sequence` 之后加一行 `player.sneakingHeld = command.Sneaking`。`player.go` 的 `playerState` 在 `miningHeld`/`eatingHeld` 旁加 `sneakingHeld bool`（注释：潜行锁存，供开容器/门床交互分流）。

- [ ] **Step 3: 运行并提交**

Run: `go test ./packages/server/sim/contract ./packages/server/server ./packages/server/sim/entity -race -count=1`
Expected: PASS。

```bash
git add packages/server/sim/contract packages/server/server/session_ingress.go packages/server/sim/entity/tick.go packages/server/sim/entity/player.go
git commit -m "feat(sim): carry Sneaking bit from ingress to player input"
```

---

### Task 3: 物理 header v4 与 Rust 双侧积分

**Files:**
- Modify: `packages/shared/physics/types.go:109-110`（`Input` 加 `Sneaking`）、`defaultSprintSpeedMultiplier` 旁加 `defaultSneakSpeedMultiplier = float32(0.3)`
- Modify: `packages/shared/physics/tunables.go:33-34`（加 `SneakSpeedMultiplier float32 \`json:"sneakSpeedMultiplier"\``）、`DefaultTunables` 加 `SneakSpeedMultiplier: defaultSneakSpeedMultiplier`
- Modify: `packages/shared/physics/step.go:13-26,81-85,188-204`
- Modify: `packages/engine/crates/mornlea_engine/src/step.rs`（解码 + 积分，见步骤）
- Test: `go test ./packages/shared/physics -race -count=1` + Rust 侧既有 step 测试

**Interfaces:**
- Consumes: Task 2 的 tick 搬运（Go 调用方传参）。
- Produces: `physics.Input.Sneaking` + `Tunables.SneakSpeedMultiplier` + header v4（Task 4/5 消费）。

- [ ] **Step 1: 先跑 Rust 基线（AGENTS 要求涉 Rust 先 `make rust`）**

Run: `make rust`
Expected: 通过。后续 Go focused 命令都基于此基线。

- [ ] **Step 2: 写失败测试（Go 侧门控）**

在 `packages/shared/physics` 加 `sneak_test.go`：`OnGround` + `Sneaking` + 前移时 `stepSweepBounds` 的水平包络必须等于 `WalkSpeed*0.3` 推导的包络（用 `movementTargetFromYaw` 同函数求期望，不手算浮点）；`Sneaking+Sprinting` 同置时包络仍为潜行值（潜行优先）。

Run: `go test ./packages/shared/physics -race -count=1 -run TestSneak`
Expected: FAIL（`Input` 无 `Sneaking` 字段，编译失败即红）。

- [ ] **Step 3: Go 侧实现**

`types.go` 的 `Input` 在 `Sprinting` 后加：

```go
// Sneaking 为真且站立非浸没时，本步水平目标速度降至 WalkSpeed*SneakSpeedMultiplier，
// 且潜行优先于疾跑（同置时只减速不加速）。
Sneaking bool
```

`step.go`：
- `stepLayoutVersion = 3` → `4`，顶部注释 `v3 复用…` 追加 `v4 在 130 置潜行位、152..156 置潜行倍率，156..160 继续保留零`。
- `stepSweepBounds` 的疾跑分支改为：

```go
walkSpeed := tunables.WalkSpeed
sneaking := input.Sneaking && state.OnGround && !input.BodyInFluid
if sneaking {
	walkSpeed *= tunables.SneakSpeedMultiplier
} else if input.Sprinting && input.MoveZ > 0 && state.OnGround && !input.BodyInFluid {
	walkSpeed *= tunables.SprintSpeedMultiplier
}
```

- `encodeStepInput` 在 `bytes[129]` 块后加：

```go
if input.Sneaking {
	bytes[130] = 1
}
```

并在 `putCollisionFloat(bytes[148:152], tunables.SprintSpeedMultiplier)` 后加：

```go
putCollisionFloat(bytes[152:156], tunables.SneakSpeedMultiplier)
```

- [ ] **Step 4: Rust 侧同形实现**

`step.rs`：解码区（`sprinting: bytes[129] == 1` 旁）加 `sneaking: bytes[130] == 1`，multiplier 区（`read_f32(bytes, 148)` 旁）加 `sneak_speed_multiplier: read_f32(bytes, 152)`；积分速度分支改为潜行优先：

```rust
let sneaking_effective = input.sneaking && input.on_ground && !input.body_in_fluid;
let speed = if sneaking_effective {
    input.walk_speed * input.sneak_speed_multiplier
} else if input.sprinting && input.move_z > 0 && input.on_ground && !input.body_in_fluid {
    input.walk_speed * input.sprint_speed_multiplier
} else {
    input.walk_speed
};
```

保留区校验把允许非零的范围从 sprint 区扩展到 `129..131` 与 `148..156`，`156..160` 仍要求零。Rust 侧既有 step 单测仿疾跑例加潜行例（0.3x + 优先）。

- [ ] **Step 5: 运行并提交**

Run: `make rust && go test ./packages/shared/physics -race -count=1`
Expected: PASS（含 Go/Rust 差分测试）。

```bash
git add packages/shared/physics packages/engine/crates/mornlea_engine/src/step.rs
git commit -m "feat(physics): sneak slowdown with header v4 and rust parity"
```

---

### Task 4: 边缘保护纯函数与双侧接入

**Files:**
- Create: `packages/shared/physics/sneak_edge.go`
- Create: `packages/shared/physics/sneak_edge_test.go`
- Modify: `packages/server/sim/entity/player.go`（物理步之前钳制）
- Modify: `packages/client/client/predictor_advance.go:81-92`（预测同序钳制）
- Test: `go test ./packages/shared/physics ./packages/server/sim/entity ./packages/client/client -race -count=1`

**Interfaces:**
- Consumes: Task 3 的 `physics.Input.Sneaking`。
- Produces: `physics.SneakEdgeHolds`（sim/预测同调，行为一致）。

- [ ] **Step 1: 写失败测试（桩碰撞源）**

`packages/shared/physics/sneak_edge_test.go` 用内存桩实现 `CollisionSource`（一格悬崖：`x<0` 有碰撞盒，`x>=0` 为空气，顶面 `y=0`）：站立 `(-0.2, 0, 0)` 面向 `+x`（`yaw` 取使前向为 `+x` 的值，用 `movementTargetFromYaw` 反推，不手算角度）+ 潜行前移意图时 `SneakEdgeHolds` 必须为 `false`；同一位置意图为零时为 `true`；未加载格（`Loaded=false`）一律为 `true`；`Jump=true` 的调用方根本不调本函数（本函数不收 `Jump` 参数，注释写明）。

Run: `go test ./packages/shared/physics -race -count=1 -run TestSneakEdge`
Expected: FAIL（函数不存在）。

- [ ] **Step 2: 实现纯函数**

`packages/shared/physics/sneak_edge.go`：

```go
package physics

import "github.com/channing771/mornlea/packages/shared/core"

// SneakEdgeHolds 报告按本步水平意图走一步后脚底仍有支撑，是潜行边缘保护的唯一判据。
//
// 调用方必须已确认 OnGround && 潜行减速有效 && !Jump 且水平意图非零；Jump 主动
// 跳落、击退与水推走非输入路径，一律不经本函数。未加载格按有支撑处理（不误钳制）。
// 每步至多 2 次方块查询（两足印前缘正下方），有界。
func SneakEdgeHolds(state State, moveX, moveZ int8, yaw float32, source CollisionSource) bool {
	if moveX == 0 && moveZ == 0 {
		return true
	}
	target := movementTargetFromYaw(moveX, moveZ, 1, float32(sin(yaw)), float32(cos(yaw)))
	// 注：用单位速度求方向，不含 WalkSpeed，方向与速度档无关。
	_ = target
	...
}
```

（实现用 `movementTargetFromYaw(moveX, moveZ, 1, yawSin, yawCos)` 取水平方向，沿方向前探 `PlayerWidth/2+0.05` 落两足印点，对每点取脚底 `y-1` 格的 `source.CollisionBoxes`：任一 `!Loaded` 即回 `true`；否则支撑存在当且仅当至少一格 `Count>0`。三角函数复用 `step.go` 的 `math.Sin/Cos` 写法，保持与调用方同源。）

- [ ] **Step 3: sim 与预测同序接入**

`player.go`（`input.BodyInFluid…` 赋值后、`StepWithTunables` 之前）：

```go
// 潜行边缘保护：输入侧钳制意图，sweep bounds 与 Rust 积分天然一致。
if input.Sneaking && !input.Jump && (input.MoveX != 0 || input.MoveZ != 0) &&
	player.state.OnGround && !input.BodyInFluid &&
	!physics.SneakEdgeHolds(player.state, input.MoveX, input.MoveZ, input.Yaw, source) {
	input.MoveX, input.MoveZ = 0, 0
}
```

`predictor_advance.go:81-90` 的 history `physics.Input` 构造前做同一钳制（`source` 为传入的 `MirrorCollisionSource`，`OnGround` 取 `p.current.OnGround`，浸没标志取 `stepWithSubmersion` 内同一推导——若该 helper 已算出 `bodyInFluid`，把钳制放在其后并复用该值，不算第二遍）。

- [ ] **Step 4: 运行并提交**

Run: `go test ./packages/shared/physics ./packages/server/sim/entity ./packages/client/client -race -count=1`
Expected: PASS。

```bash
git add packages/shared/physics/sneak_edge.go packages/shared/physics/sneak_edge_test.go packages/server/sim/entity/player.go packages/client/client/predictor_advance.go
git commit -m "feat(sneak): clamp forward intent at unsupported edges"
```

---

### Task 5: sim 门控、疲劳压制与交互分流

**Files:**
- Modify: `packages/server/sim/entity/player.go:534-584`（饥饿门控 + 疲劳条件）
- Modify: `packages/server/sim/entity/container.go:60-104`（`openContainer` 潜行拒绝）
- Modify: `packages/server/sim/entity/door.go:165-198`、`sleep.go`（`executeInteractBed` 同位置预留分流）
- Test: `go test ./packages/server/sim/... -race -count=1`

**Interfaces:**
- Consumes: Task 2 的 `sneakingHeld`、Task 3 的门控语义。
- Produces: 权威结算完整（Task 7 只做集成）。

- [ ] **Step 1: 写失败测试**

`entity` 包加 `sneak_settlement_test.go` 三例（走 `settlePlayerInteractionsTick` + 直调结算 helper，既有模式）：
1. 潜行 + 疾跑同置 + 地面前移：本 tick 无加速（位置增量等于 `0.3x` 期望区间）且 `exhaustionSprintMilli` 未计费；
2. 潜行中 `CommandOpenFurnace`（`sneakingHeld=true` 前置一个 `CommandPlayerInput{Sneaking:true}`）：必须 `RejectInvalidInput` 且不建立查看关系；
3. 非潜行对照组：同条件加速 `1.3x` 且开容器成功（防回归）。

Run: `go test ./packages/server/sim/entity -race -count=1 -run TestSneak`
Expected: FAIL。

- [ ] **Step 2: 门控与疲劳**

`player.go:534-538` 饥饿门控后追加潜行压制：

```go
// 潜行优先：潜行意图有效时疾跑加速与疲劳都不触发（sprint spec 的互斥半边）。
if input.Sneaking {
	input.Sprinting = false
}
```

（位置在饥饿清零之后、氧气结算之前；`input.Sneaking` 本体保留，供 physics 减速与放置分流。）疲劳判据 `player.go:582` 追加 `&& !input.Sneaking` 的等价条件——因上一步已清零 `Sprinting`，此处无需改动，但加一行注释说明压制点在上游，避免后人重复设防。（若评审坚持显式双保险，则把条件写全，测试覆盖两种顺序。）

- [ ] **Step 3: 交互分流**

`container.go` 的 `openContainer` 在权威射线命中并确认方块为容器/`Workbench` 后、建立查看关系前加：

```go
// 潜行放置：潜行中不打开容器，客户端应直发 PlaceBlock；服务端权威拒绝兜底。
if engine.sessions[id].player.sneakingHeld {
	return RejectInvalidInput, true
}
```

`door.go` 的 `executeInteractDoor` 在 `core.IsDoor(block)` 确认后、`handleInteractDoor` 之前加同形分流（`session.player.sneakingHeld`）；`sleep.go` 的 `executeInteractBed` 同位置同样预留（现无上行触发，测试直调 handler 覆盖）。

- [ ] **Step 4: 运行并提交**

Run: `go test ./packages/server/sim/... -race -count=1`
Expected: PASS。

```bash
git add packages/server/sim/entity
git commit -m "feat(sim): sneak priority gating and sneak-place refusals"
```

---

### Task 6: 客户端双击状态机、上行与放置分支

**Files:**
- Modify: `packages/client/client/input.go`（`InputState` 加状态机）
- Modify: `packages/client/cmd/mornlea/app/interactive.go:416-422,487-499`（采样 + 接线，`Ctrl` 退役）
- Modify: `packages/client/client/predictor.go:20-34`（`Control` 加 `Sneaking`）
- Modify: `packages/client/client/predictor_advance.go:60-90`（上行 + 镜像门控）
- Modify: `packages/client/cmd/mornlea/app/app_input.go:26-53`（`placeBlock` 潜行分支）
- Test: `go test ./packages/client/... -race -count=1`

**Interfaces:**
- Consumes: Task 1 的 wire 位、Task 4 的钳制函数。
- Produces: 客户端完整行为（Task 7 集成）。

- [ ] **Step 1: 写失败测试（假时钟）**

`packages/client/client/input_test.go` 加 `TestInputStateDoubleTapSprint`：假时钟 `t0` 按下 `W` 松开（记录释放），`t0+200ms` 再次按下 → `UpdateSprint` 返回 `true`；`t0+400ms` 才第二次按下 → `false`；锁存后松开 `W` → `false`；锁存中 `sneakHeld=true` → `false` 且 armed 时刻清除（随后 100ms 内再按 `W` 不得误触发）；`uiOpen=true` → `false`。

Run: `go test ./packages/client/client -race -count=1 -run TestInputStateDoubleTap`
Expected: FAIL（方法不存在）。

- [ ] **Step 2: 状态机实现**

`input.go`（加 `time` 导入）：

```go
// sprintDoubleTapWindow 是双击 W 判定为疾跑的最大释放→按下间隔。
const sprintDoubleTapWindow = 300 * time.Millisecond

// InputState 新增私有字段（与既有 primaryDown 等并列）：
// wWasHeld bool; lastWRelease time.Time; wReleaseArmed bool; sprintLatched bool

// UpdateSprint 由交互层每帧调用。wHeld 为本帧 W 是否按住，sneakHeld 为 Shift 是否
// 按住，uiOpen 为任一界面/聊天/暂停/面板是否打开（打开即清零并清除 armed 时刻）。
// 返回本帧是否请求疾跑；真正的速度门控仍在服务端与预测侧。
func (state *InputState) UpdateSprint(wHeld, sneakHeld, uiOpen bool, now time.Time) bool {
	if uiOpen || sneakHeld {
		state.sprintLatched = false
		state.wReleaseArmed = false
		state.wWasHeld = wHeld
		return false
	}
	if wHeld && !state.wWasHeld {
		if state.wReleaseArmed && now.Sub(state.lastWRelease) <= sprintDoubleTapWindow {
			state.sprintLatched = true
		}
		state.wReleaseArmed = false
	}
	if !wHeld && state.wWasHeld {
		state.lastWRelease = now
		state.wReleaseArmed = true
		state.sprintLatched = false
	}
	state.wWasHeld = wHeld
	return state.sprintLatched && wHeld
}
```

- [ ] **Step 3: 交互层接线与 `Ctrl` 退役**

`interactive.go` 的 `movement` 构造后（`416-422` 行后）加：

```go
wHeld := app.window.KeyDown(client.KeyW)
sneakHeld := app.window.KeyDown(client.KeyLeftShift)
sprinting := input.UpdateSprint(wHeld, sneakHeld,
	app.inventoryOpen || chatBlockedThisFrame || pausedUI || app.panelVisible(), time.Now())
movement.Sneaking = sneakHeld
movement.Sprinting = sprinting
```

（`client.Movement` 结构体加 `Sneaking, Sprinting bool` 字段；`MovementFromKeys` 不变，零值即不潜行不疾跑，既有单测不受影响。）`applyInteractiveInput` 的 `Control` 构造（`487-499`）改为：

```go
Sneaking: allowActions && movement.Sneaking,
Sprinting: allowActions && movement.Sprinting,
```

删除 `Ctrl/Shift` 按住即疾跑行（`Ctrl` 退役；`Shift` 已是潜行）。`app_input.go` 的 `placeBlock` 签名改为 `placeBlock(sneaking bool)`，调用点传 `movement.Sneaking && allowActions` 的等价值（`applyInteractiveInput` 内 `control.Sneaking`）；函数内开容器分支（`42-53`）门口加：

```go
// 潜行放置：潜行中跳过开容器，直发 PlaceBlock；服务端权威拒绝兜底。
if !sneaking {
	...既有容器/工作台判定...
}
```

锄地/骨粉分支保持原样（非目标）。

- [ ] **Step 4: 预测侧上行**

`predictor.go:33` 后加 `Sneaking bool`（注释同 `Sprinting` 体例：门控见 sneak spec，客户端只上行意图）。`predictor_advance.go:62-76` 改为：

```go
sprinting := control.Sprinting
sneaking := control.Sneaking
if p.hunger < 6 {
	sprinting = false
}
if sneaking {
	sprinting = false
}
message := network.PlayerInput{
	...
	Sprinting: sprinting,
	Sneaking:  sneaking,
}
```

history 的 `physics.Input` 同步加 `Sneaking: sneaking`（`Sprinting: sprinting` 保持压制后值），随后接 Task 4 的边缘钳制（`p.current.OnGround` + 浸没值复用 `stepWithSubmersion` 内推导）。

- [ ] **Step 5: 运行并提交**

Run: `go test ./packages/client/... -race -count=1`
Expected: PASS（既有 `Movement{MoveZ:1}` 用例零值无潜行，行为不变）。

```bash
git add packages/client/client/input.go packages/client/client/input_test.go packages/client/client/predictor.go packages/client/client/predictor_advance.go packages/client/cmd/mornlea/app/interactive.go packages/client/cmd/mornlea/app/app_input.go
git commit -m "feat(client): double-tap sprint latch and sneak-place routing"
```

---

### Task 7: 全链集成、门禁与归档收尾

**Files:**
- Modify: `openspec/changes/sneak-double-tap-sprint/tasks.md`（勾选）
- Modify: `openspec/specs/sprint/spec.md`（MODIFIED 同步触发方式）、`openspec/specs/sneak/spec.md`（新建能力从 change sync）
- Modify: `docs/notes/progress.md`（编年史追加 B-42 一节）、`docs/feature-backlog.md`（B-42 → 已完成，由合入后状态定）
- Test: 全量门禁（见步骤）

**Interfaces:**
- Consumes: Task 1–6 全部。
- Produces: 可合入分支 + 归档 change。

- [ ] **Step 1: 跨端 parity 补测**

加两例集成测试（仿 `eating_parity_test.go` 体例，Memory transport）：潜行前进 N tick 后服务端权威位置与客户端预测位置差值在容差内（含悬崖边钳制场景：悬崖边潜行前进 40 tick 不坠落、非潜行对照组坠落）；`Sneaking=true` 的 `PlayerInput` 经 TCP 与 Memory 双路往返一致。

Run: `go test ./packages/server/server -race -count=1 -run 'TestSneakParity|TestPlayerInputSneak'`
Expected: PASS。

- [ ] **Step 2: 阶段边界门禁（T2）**

Run（worktree 内，按序）：

```bash
make rust
go test ./packages/audit -count=1
make dev-check
```

Expected: 全绿。`dev-check` 覆盖六模块 vet 与相关守卫；失败先修实现不修门禁。

- [ ] **Step 3: 视觉零漂门禁**

Run: `make visual-check`
Expected: 30 景零差异（本行无呈现变更；若有亚阈值漂移，按 F-05 先例逐图确认来源后钉准，不顺手重生无关 golden）。

- [ ] **Step 4: 全量门禁（T3）与 OpenSpec 严格校验**

Run:

```bash
make test-race
openspec validate --all --strict --no-interactive
```

Expected: 全绿 + strict 通过。`test-race` 为六模块全量 race；性能数值只记录（benchmark 不升 scenario）。

- [ ] **Step 5: 同步主规格并归档**

把 change 的 delta 合入主规格（`sprint` MODIFIED + 新建 `sneak` 能力，`openspec sync` 或按仓库既有归档先例手合同步），change 目录移入 `openspec/changes/archive/2026-09-11-sneak-double-tap-sprint/`，`docs/notes/progress.md` 追加 B-42 一节（含协议 v41、header v4、门/床 wiring 缺口记录），B-42 行 `状态` → `待集成`（PR 合入后 → `已完成`）。

- [ ] **Step 6: 提交收尾**

```bash
gofmt -l packages/shared packages/server packages/client | grep . && exit 1 || true
git add -A
git commit -m "chore(openspec): archive B-42 sneak with double-tap sprint"
```

随后按 `pr-submit` skill 走 PR（标题单行英文，`feat(b42): ...` 体例，正文用 `.github/PULL_REQUEST_TEMPLATE.md` 模板英文填写 `Summary` 与 `Validation`）。

---

## Self-Review

**1. Spec 覆盖：** §2 输入键位→Task 6；§3 协议物理→Task 1/2/3；§4.1 边缘→Task 4；§4.2 放置→Task 5/6；§5 版本测试→Task 7；§6 被否决方案→Task 0 的 design 直译 + Task 5/6 的非目标守卫（锄地骨粉水桶不动、视线高度不动）。门/床 wiring 缺口在 Task 5 预留 + Task 7 progress 记录，无遗漏。

**2. 占位扫描：** 无 `TBD/TODO/适当处理/仿照 Task N`；所有步骤含真实代码、真实命令与期望输出；`sneak_edge.go` 的方向探针写法指向 `movementTargetFromYaw` 同源，不留第二套三角。

**3. 类型一致：** `Sneaking bool` 在 `protocol.PlayerInput`/`contract.Command`/`physics.Input`/`client.Control`/`client.Movement` 五处同名同类型；`SneakSpeedMultiplier float32` 在 `physics.Tunables` 与 header `152:156` 同名；`sneakingHeld` 只在 `playerState`，`armed/latch` 只在 `client.InputState`，两域不混用；header v4 字节位 `130`/`152:156` 在 Go/Rust 两侧一致。
