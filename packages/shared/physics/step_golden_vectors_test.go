package physics_test

// step_golden_vectors_test.go：`physics.Step` 的位级确定性 golden 向量。
//
// 契约：重放一致性要求同一输入在所有平台上产生**逐位相同**的积分结果；e2e
// parity 只能锁同进程粗粒度行为，锁不住跨平台位漂移，源码字面向量是唯一廉价的
// 位级回归网——可评审、可 diff、零 I/O，任何一位翻转都会在此显形。
//
// 向量来源：2026-08 从当前生产路径（Go 编码 prism → Rust `mornlea_engine`
// 积分，即唯一的生产行为）经 `physics.Step` 一次性采集，人工复核合理性后固化
// 为 math.Float32bits 字面量。
//
// 场景按积分分支逐一枚举：地面加速对角行走、地面减速、地面起跳、空中重力与
// 终端速度钳制、空中行走速度钳制（超速归一化到 WalkSpeed）、水中下沉（静止/
// 终端）、水中上浮（空中）、水中水平阻力（地面）、天花板碰撞、半砖 step-up、
// unknown 格阻挡，外加负零哨兵。负零位模式（0x80000000）是「编码端不规范化
// 浮点」契约的哨兵：Go 编码、Rust 积分与输出解码任一侧把 −0 规范成 +0（或
// 反向引入）都会被钉住。
//
// tunable 显式钉在 `physics.DefaultTunables`：向量在该取值下采集，换参数即换契约。

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func TestStepGoldenVectors(t *testing.T) {
	previousTunables := physics.ActiveTunables()
	t.Cleanup(func() { physics.SetTunables(previousTunables) })
	physics.SetTunables(physics.DefaultTunables())

	floor := func() testCollisionWorld {
		world := testCollisionWorld{}
		for x := int32(-3); x <= 3; x++ {
			for z := int32(-3); z <= 3; z++ {
				world[core.BlockPos{X: x, Y: 0, Z: z}] = physics.CollisionBoxSet{Loaded: true, Count: 1, Boxes: [8]core.AABB{fullCube}}
			}
		}
		return world
	}
	negativeZeroZ := math.Float32frombits(1 << 31)

	tests := []struct {
		name       string
		state      physics.State
		input      physics.Input
		world      testCollisionWorld
		wantPos    [3]uint32
		wantVel    [3]uint32
		onGround   bool
		usedStep   bool
		hitUnknown bool
	}{
		{
			// 地面加速对角行走：首步水平速度恰为 `GroundAcceleration`·dt=2 沿对角线
			// 均分（±√2），下落触地被钳位——vy 归零、y 恒为 1。
			name:       "grounded diagonal walk",
			state:      physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			input:      physics.Input{MoveX: 1, MoveZ: 1},
			world:      floor(),
			wantPos:    [3]uint32{0x3f121a18, 0x3f800000, 0x3edbcbcf},
			wantVel:    [3]uint32{0x3fb504f3, 0x00000000, 0xbfb504f3},
			onGround:   true,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 地面减速停止：超速初值 (10,0,-3) 被 `GroundDeceleration`·dt=2.5 朝零收敛
			// 而不是瞬间截断，位置按收敛后的速度推进。
			name:       "grounded decel to stop",
			state:      physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, Velocity: mgl32.Vec3{10, 0, -3}, OnGround: true},
			input:      physics.Input{},
			world:      floor(),
			wantPos:    [3]uint32{0x3f61597d, 0x3f800000, 0x3ec5971c},
			wantVel:    [3]uint32{0x40f35fb8, 0x00000000, 0xc012063b},
			onGround:   true,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 地面起跳：vy 直接赋 `JumpSpeed`=8.4（一次冲量），同 tick 内上移
			// 0.42 且离开地面；水平仍走地面加速分支得到整 2。
			name:       "jump from ground",
			state:      physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			input:      physics.Input{Jump: true, MoveX: 1},
			world:      floor(),
			wantPos:    [3]uint32{0x3f19999a, 0x3fb5c28f, 0x3f000000},
			wantVel:    [3]uint32{0x40000000, 0x41066666, 0x00000000},
			onGround:   false,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 空中重力与终端速度钳制：vy=-78 再落一步会越过 -78.4，被钳在终端值
			// （c29ccccd），位移按钳后速度计算。
			name:       "airborne gravity terminal clamp",
			state:      physics.State{Position: mgl32.Vec3{0.5, 40, 0.5}, Velocity: mgl32.Vec3{0, -78, 0}},
			input:      physics.Input{},
			world:      floor(),
			wantPos:    [3]uint32{0x3f000000, 0x421051ec, 0x3f000000},
			wantVel:    [3]uint32{0x00000000, 0xc29ccccd, 0x00000000},
			onGround:   false,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 空中行走速度钳制：超速初值 (30,0,-20) 只被 `AirAcceleration`·dt=0.4
			// 缓慢拉向目标，水平长度仍远超 `WalkSpeed`，于是走归一化钳制分支——
			// 钳后水平模长恰为 `WalkSpeed`（√(3.575²+2.389²)=4.300），方向保留。
			name:       "airborne walk speed clamp",
			state:      physics.State{Position: mgl32.Vec3{0.5, 5, 0.5}, Velocity: mgl32.Vec3{30, 0, -20}},
			input:      physics.Input{MoveX: 1, MoveZ: 1, Yaw: 0.75},
			world:      floor(),
			wantPos:    [3]uint32{0x3f2dc2b5, 0x409d70a4, 0x3ec2d511},
			wantVel:    [3]uint32{0x4064cd87, 0xbfcccccd, 0xc018eb56},
			onGround:   false,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 跳入天花板：上升撞到 y=3 的方块底面，脚停在 3-1.8=1.2，向上速度
			// 经碰撞裁剪归零且不误报落地。
			name:  "jump into ceiling",
			state: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			input: physics.Input{Jump: true},
			world: testCollisionWorld{
				{X: 0, Y: 0, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{fullCube}},
				{X: 0, Y: 3, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{fullCube}},
			},
			wantPos:    [3]uint32{0x3f000000, 0x3f99999a, 0x3f000000},
			wantVel:    [3]uint32{0x00000000, 0x00000000, 0x00000000},
			onGround:   false,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 水中从静止下沉：水中重力 `FluidGravity`=6.4 取代空气重力，首步 vy=-0.32
			// 远未到水中终端速度，缓慢离底下沉。
			name:       "fluid sink from rest",
			state:      physics.State{Position: mgl32.Vec3{0.5, 8, 0.5}},
			input:      physics.Input{BodyInFluid: true},
			world:      floor(),
			wantPos:    [3]uint32{0x3f000000, 0x40ff7cee, 0x3f000000},
			wantVel:    [3]uint32{0x00000000, 0xbea3d70b, 0x00000000},
			onGround:   false,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 水中终端下沉：vy=-12 已越过水中终端 `FluidSinkSpeed`=3，被压到 -3
			// 而非空气终端 78.4——两条终端路径必须走各自的常量。
			name:       "fluid sink at terminal",
			state:      physics.State{Position: mgl32.Vec3{0.5, 8, 0.5}, Velocity: mgl32.Vec3{0, -12, 0}},
			input:      physics.Input{BodyInFluid: true},
			world:      floor(),
			wantPos:    [3]uint32{0x3f000000, 0x40fb3333, 0x3f000000},
			wantVel:    [3]uint32{0x00000000, 0xc0400000, 0x00000000},
			onGround:   false,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 水中上浮（空中）：`BodyInFluid`+`Jump` 是每 tick 直接赋值的持续上浮
			// `FluidAscendSpeed`=4，不看 `OnGround`；水平同时吃空中加速与水中阻力。
			name:       "fluid ascend airborne",
			state:      physics.State{Position: mgl32.Vec3{0.5, 8, 0.5}, Velocity: mgl32.Vec3{1, -2, -1}},
			input:      physics.Input{Jump: true, BodyInFluid: true, MoveX: 1, Yaw: 0.4},
			world:      floor(),
			wantPos:    [3]uint32{0x3f0e3bd2, 0x41033333, 0x3ee9b344},
			wantVel:    [3]uint32{0x3f8e562f, 0x40800000, 0xbf5eff53},
			onGround:   false,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 水中水平阻力（地面）：已达标的地速 4.3 在浸没时乘 `FluidHorizontalDrag`
			// 0.8 得稳态 3.44；z 分量的负零位模式原样穿过整条管线。
			name:       "fluid horizontal drag grounded",
			state:      physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, Velocity: mgl32.Vec3{4.3, 0, 0}, OnGround: true},
			input:      physics.Input{MoveX: 1, BodyInFluid: true},
			world:      floor(),
			wantPos:    [3]uint32{0x3f2c0832, 0x3f800000, 0x3f000000},
			wantVel:    [3]uint32{0x405c28f7, 0x00000000, 0x80000000},
			onGround:   true,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// ±0 速度哨兵：输入 vz 为负零（0x80000000），`moveToward` 的算术把它
			// 规范回 +0——钉住「哪一侧、以何种符号约定处理带符号零」，防止编码
			// 或积分改动让该约定静默漂移。
			name:       "negative zero z velocity",
			state:      physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, Velocity: mgl32.Vec3{0, 0, negativeZeroZ}, OnGround: true},
			input:      physics.Input{MoveX: 1},
			world:      floor(),
			wantPos:    [3]uint32{0x3f19999a, 0x3f800000, 0x3f000000},
			wantVel:    [3]uint32{0x40000000, 0x00000000, 0x00000000},
			onGround:   true,
			usedStep:   false,
			hitUnknown: false,
		},
		{
			// 半砖 step-up：普通移动被墙裁剪后被 step-up 路径取代——完整水平位移
			// 0.215 全额保留，落在半砖顶 y=1.5，`StepResult.UsedStep`=true。
			name:  "half block step",
			state: groundedTowardObstacle(),
			input: physics.Input{MoveX: 1},
			world: testCollisionWorld{
				{X: 0, Y: 0, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{fullCube}},
				{X: 1, Y: 0, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{fullCube}},
				{X: 1, Y: 1, Z: 0}: {Loaded: true, Count: 1, Boxes: [8]core.AABB{{Max: mgl32.Vec3{1, 0.5, 1}}}},
			},
			wantPos:    [3]uint32{0x3f370a3e, 0x3fc00000, 0x3f000000},
			wantVel:    [3]uint32{0x4089999a, 0x00000000, 0x80000000},
			onGround:   true,
			usedStep:   true,
			hitUnknown: false,
		},
		{
			// unknown 格阻挡路径：前方格未加载按实心墙处理，水平速度归零、
			// `StepResult.HitUnknown`=true 上抛给调用方判定，脚下无地面则照常悬空下落。
			name:       "unknown cell blocks path",
			state:      physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, Velocity: mgl32.Vec3{4.3, 0, 0}, OnGround: true},
			input:      physics.Input{MoveX: 1},
			world:      testCollisionWorld{{X: 1, Y: 1, Z: 0}: {}},
			wantPos:    [3]uint32{0x3f333333, 0x3f6b851f, 0x3f000000},
			wantVel:    [3]uint32{0x00000000, 0xbfcccccd, 0x80000000},
			onGround:   false,
			usedStep:   false,
			hitUnknown: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := physics.Step(test.state, test.input, test.world)
			explicit := physics.StepWithTunables(
				test.state, test.input, test.world, physics.DefaultTunables(),
			)
			if explicit != got {
				t.Fatalf("显式 tunables 路径与兼容 wrapper 不同：explicit=%+v wrapper=%+v",
					explicit, got)
			}
			for axis := range 3 {
				if bits := math.Float32bits(got.State.Position[axis]); bits != test.wantPos[axis] {
					t.Fatalf("position[%d] bits=%08x，want %08x", axis, bits, test.wantPos[axis])
				}
				if bits := math.Float32bits(got.State.Velocity[axis]); bits != test.wantVel[axis] {
					t.Fatalf("velocity[%d] bits=%08x，want %08x", axis, bits, test.wantVel[axis])
				}
			}
			if got.State.OnGround != test.onGround || got.UsedStep != test.usedStep || got.HitUnknown != test.hitUnknown {
				t.Fatalf("flags=(onGround:%t usedStep:%t hitUnknown:%t)，want (%t/%t/%t)",
					got.State.OnGround, got.UsedStep, got.HitUnknown, test.onGround, test.usedStep, test.hitUnknown)
			}
		})
	}
}
