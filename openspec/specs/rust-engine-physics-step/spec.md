# rust-engine-physics-step Specification

## Purpose

把物理固定步的积分（移动目标、加速/摩擦、跳跃、重力、终端速度裁剪）与碰撞解析一并交由 Rust engine 唯一生产实现，Go 只保留领域 API 与编码，并以冻结的位级 golden 向量保证任意平台 float32 逐位一致。
## Requirements
### Requirement: 物理 tick 积分由 Rust engine 独占生产

物理固定步的积分（移动目标、加速/摩擦、跳跃、重力、终端速度裁剪）与碰撞解析、速度裁剪
MUST 由 Rust engine 独占生产；Go 生产路径与测试 MUST 都不保留旧 Go 积分副本，跨平台
float32 逐位一致契约由冻结的位级 golden 向量把守。

#### Scenario: 生产 Step 与冻结 golden 向量逐位一致

- GIVEN 冻结 golden 向量的固定 State、Input、CollisionSource 与运行时 Tunables
- WHEN 在任意平台（含 arm64）调用 physics.Step
- THEN 结果 State（Position/Velocity/OnGround）、UsedStep、HitUnknown MUST 与源码固化
  的 float32 bit 字面量逐位一致——13 条向量覆盖地面行走/减速停止、起跳、空中重力与
  终端速度钳制、空中行走速度钳制、水中下沉/上浮/水平阻力、天花板碰撞、半砖 step-up、
  unknown 格阻挡与负零哨兵（packages/shared/physics/step_golden_vectors_test.go），任何一位
  漂移都使测试失败

#### Scenario: golden 向量在非 arm64 平台同样逐位成立

- GIVEN 非 arm64 平台上的合法物理输入
- WHEN 运行物理测试与生产 Step
- THEN Rust engine 仍执行生产积分与行为测试，冻结 golden 向量期望是平台无关的源码
  字面量，测试 MUST NOT 依赖任何随平台变化的参考实现

#### Scenario: 对角输入无斜向加速增益

- GIVEN 地面玩家，OnGround=true，MoveX=1，MoveZ=1，默认 tunables
- WHEN 执行一个固定步
- THEN 水平速度模长 ‖v‖ 满足 |‖v‖ − 2.0| < 1e-5（acceleration=40、dt=0.05）

#### Scenario: 跳跃与重力使用固定常量

- GIVEN 地面玩家，Jump=true，默认 tunables
- WHEN 执行一个固定步
- THEN 垂直速度等于 JumpSpeed 且 OnGround=false
- GIVEN 空中玩家垂直速度 −78
- WHEN 执行一个固定步
- THEN 垂直速度不低于 −TerminalFallSpeed

### Requirement: 运行时 tunables 每步生效

SetTunables 之后的下一次 Step MUST 使用新参数。

#### Scenario: SetTunables 后下一步生效

- GIVEN 任意合法状态与碰撞源
- WHEN SetTunables(增大 StepHeight) 后调用 physics.Step
- THEN 该步 prism 尺寸与结果反映新 StepHeight（4096-cell 上限用例保持通过）

### Requirement: sweep bounds 违约拒绝

当输入携带的位移界与积分位移相差超过 1 ulp 时，调用 MUST 被拒绝并报告稳定中文 panic
文案，且 MUST 不产出静默漂移结果；位移在界内或界外至多 1 ulp 时 MUST 通过。

#### Scenario: 位移越界超过 1 ulp 被拒绝

- GIVEN 合法 state/input 但输入 sweep bounds 与该步积分位移相差超过 1 ulp
- WHEN 调用 native physics_step
- THEN 返回 StatusInput，且输出缓冲不被修改

#### Scenario: 位移在界内或界外至多 1 ulp 通过

- GIVEN 合法 state/input，且该步积分位移在 sweep bounds 界内或界外至多 1 ulp
- WHEN 调用 native physics_step
- THEN 正常返回积分与碰撞结果，不返回 StatusInput

### Requirement: 碰撞差分入口保留

`mornlea_collision_resolve` MUST 继续可用且行为不变，供行为锁测试直接驱动碰撞解析出口；
生产路径只调用 `mornlea_physics_step`。

#### Scenario: 碰撞差分测试继续通过

- GIVEN 采集自生产路径并固化为字面量期望的碰撞级行为语料
- WHEN 调用 nativeabi.CollisionResolve
- THEN 输出位置、clipped mask、OnGround、UsedStep 与 HitUnknown MUST 与冻结字面量
  逐位一致；并发调用 MUST 与串行基准一致，常规桥 MUST 保持零 Go heap allocation
  （见 packages/shared/physics/collision_native_test.go）

