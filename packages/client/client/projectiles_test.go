package client

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// projectileSpawnMessageOf 构造单条记录的合法投射物 spawn 消息。
func projectileSpawnMessageOf(tick, id uint64, kind uint8, position, velocity mgl32.Vec3) network.ProjectileSpawn {
	return network.ProjectileSpawn{ServerTick: tick, Spawns: []network.ProjectileSpawnRecord{{
		ID: id, Kind: kind, Dimension: core.Overworld, Position: position, Velocity: velocity,
	}}}
}

func projectileStateMessageOf(tick, id uint64, position mgl32.Vec3) network.ProjectileState {
	return network.ProjectileState{ServerTick: tick, States: []network.ProjectileStateRecord{{
		ID: id, Position: position,
	}}}
}

func TestProjectileMirrorLifecycle(t *testing.T) {
	projectiles := &Projectiles{}

	// spawn 建立镜像：呈现携带权威位置、弹种、维度与初速。
	if err := projectiles.ApplySpawn(projectileSpawnMessageOf(
		100, 7, network.ProjectileKindShard, mgl32.Vec3{1, 2, 3}, mgl32.Vec3{0, -0.5, -1},
	)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	presentations := projectiles.AppendPresentations(nil)
	if len(presentations) != 1 || presentations[0].ID != 7 ||
		presentations[0].Kind != network.ProjectileKindShard ||
		presentations[0].Dimension != core.Overworld ||
		presentations[0].Position != (mgl32.Vec3{1, 2, 3}) ||
		presentations[0].Velocity != (mgl32.Vec3{0, -0.5, -1}) {
		t.Fatalf("spawn 后呈现=%#v，想要 ID 7 骨刺在 (1,2,3) 初速 (0,-0.5,-1)", presentations)
	}

	// 重复 spawn 按稳定规则忽略：既有镜像保持不变。
	if err := projectiles.ApplySpawn(projectileSpawnMessageOf(
		999, 7, network.ProjectileKindArrow, mgl32.Vec3{9, 9, 9}, mgl32.Vec3{0, 30, 0},
	)); err != nil {
		t.Fatalf("重复 spawn 返回错误: %v", err)
	}
	presentations = projectiles.AppendPresentations(nil)
	if presentations[0].Position != (mgl32.Vec3{1, 2, 3}) || presentations[0].Kind != network.ProjectileKindShard {
		t.Fatalf("重复 spawn 改写了镜像：%#v", presentations[0])
	}

	// 从未 spawn 的 ID：state 丢弃且不隐式造实体。
	if err := projectiles.ApplyStates(projectileStateMessageOf(101, 8, mgl32.Vec3{5, 5, 5})); err != nil {
		t.Fatalf("未知 ID state 返回错误: %v", err)
	}
	if got := len(projectiles.AppendPresentations(nil)); got != 1 {
		t.Fatalf("未知 ID state 造出了实体，呈现数=%d", got)
	}

	// 过期 state（tick 100 不比镜像新）：丢弃并保持镜像不变。
	if err := projectiles.ApplyStates(projectileStateMessageOf(100, 7, mgl32.Vec3{6, 6, 6})); err != nil {
		t.Fatalf("过期 state 返回错误: %v", err)
	}
	if presentations = projectiles.AppendPresentations(nil); presentations[0].Position != (mgl32.Vec3{1, 2, 3}) {
		t.Fatalf("过期 state 改写了镜像：%#v", presentations[0])
	}

	// 更新 tick 的 state：位置前进，速度按位置差分重估（每 tick 位移 (0,-1,-2)）。
	if err := projectiles.ApplyStates(projectileStateMessageOf(101, 7, mgl32.Vec3{1, 1, 1})); err != nil {
		t.Fatalf("ApplyStates: %v", err)
	}
	presentations = projectiles.AppendPresentations(nil)
	if presentations[0].Position != (mgl32.Vec3{1, 1, 1}) ||
		presentations[0].Velocity != (mgl32.Vec3{0, -1, -2}) {
		t.Fatalf("state 后呈现=%#v，想要位置 (1,1,1) 差分速度 (0,-1,-2)", presentations[0])
	}

	// 零位移的 state：保持上一次速度估计（取向不退回未知）。
	if err := projectiles.ApplyStates(projectileStateMessageOf(102, 7, mgl32.Vec3{1, 1, 1})); err != nil {
		t.Fatalf("ApplyStates: %v", err)
	}
	presentations = projectiles.AppendPresentations(nil)
	if presentations[0].Velocity != (mgl32.Vec3{0, -1, -2}) {
		t.Fatalf("零位移 state 清掉了速度估计：%#v", presentations[0])
	}

	// 未知 ID 的 despawn 丢弃；已知 ID 的 despawn 移除镜像。
	if err := projectiles.ApplyDespawn(network.ProjectileDespawn{ServerTick: 103, IDs: []uint64{8}}); err != nil {
		t.Fatalf("未知 ID despawn 返回错误: %v", err)
	}
	if err := projectiles.ApplyDespawn(network.ProjectileDespawn{ServerTick: 103, IDs: []uint64{7}}); err != nil {
		t.Fatalf("ApplyDespawn: %v", err)
	}
	if got := len(projectiles.AppendPresentations(nil)); got != 0 {
		t.Fatalf("despawn 后仍有 %d 枚投射物", got)
	}
}

func TestProjectileMirrorCapacityIsStableAtOneHundredTwentyEight(t *testing.T) {
	projectiles := &Projectiles{}
	const overflowID = 129
	for id := uint64(1); id <= overflowID; id++ {
		if err := projectiles.ApplySpawn(projectileSpawnMessageOf(
			1, id, network.ProjectileKindArrow, mgl32.Vec3{float32(id), 0, 0}, mgl32.Vec3{1, 0, 0},
		)); err != nil {
			t.Fatalf("spawn %d: %v", id, err)
		}
	}
	presentations := projectiles.AppendPresentations(nil)
	if got := len(presentations); got != MaxProjectiles {
		t.Fatalf("镜像容量=%d，想要 %d", got, MaxProjectiles)
	}
	if MaxProjectiles != 128 {
		t.Fatalf("MaxProjectiles=%d，想要 128（与 wire 侧 record 上限同源）", MaxProjectiles)
	}
	// 满镜像拒绝的是第 129 枚（按稳定规则忽略），既有镜像不受影响。
	if !slices.ContainsFunc(presentations, func(p ProjectilePresentation) bool { return p.ID == 1 }) {
		t.Fatal("首个 spawn 的投射物被驱逐")
	}
	if slices.ContainsFunc(presentations, func(p ProjectilePresentation) bool { return p.ID == overflowID }) {
		t.Fatal("溢出的第 129 枚投射物被接纳")
	}
}

// TestProjectilePresentationsAreIDAscending 锁定呈现列表按 ID 升序：帧与帧
// 之间的绘制顺序确定，despawn/重 spawn 不改变这一纪律。
func TestProjectilePresentationsAreIDAscending(t *testing.T) {
	projectiles := &Projectiles{}
	for _, id := range []uint64{42, 3, 17} {
		if err := projectiles.ApplySpawn(projectileSpawnMessageOf(1, id, network.ProjectileKindArrow,
			mgl32.Vec3{0, 2, 0}, mgl32.Vec3{1, 0, 0})); err != nil {
			t.Fatalf("spawn %d: %v", id, err)
		}
	}
	presentations := projectiles.AppendPresentations(nil)
	if len(presentations) != 3 {
		t.Fatalf("呈现数=%d，想要 3", len(presentations))
	}
	for index := 1; index < len(presentations); index++ {
		if presentations[index-1].ID >= presentations[index].ID {
			t.Fatalf("呈现未按 ID 升序：%v", presentations)
		}
	}
}

// TestProjectilePresentationStaysInsideInterpolationWindow 复用夜行者的插值
// 窗口纪律：投射物绝不预测弹道，呈现只在已确认位置的插值窗内前进。
func TestProjectilePresentationStaysInsideInterpolationWindow(t *testing.T) {
	projectiles := &Projectiles{}
	positions := []mgl32.Vec3{{0, 2, 0}, {2, 2, 0}, {4, 2, 0}}
	if err := projectiles.ApplySpawn(projectileSpawnMessageOf(100, 7, network.ProjectileKindArrow, positions[0], mgl32.Vec3{2, 0, 0})); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	for tick := uint64(101); tick <= 102; tick++ {
		if err := projectiles.ApplyStates(projectileStateMessageOf(tick, 7, positions[tick-100])); err != nil {
			t.Fatalf("ApplyStates: %v", err)
		}
	}

	// 零推进：呈现位于插值滞后窗内（恰好是 tick 100 的已确认位置）。
	projectiles.Advance(0)
	presentations := projectiles.AppendPresentations(nil)
	if presentations[0].Position != positions[0] {
		t.Fatalf("零推进呈现=%v，想要滞后窗内的 %v", presentations[0].Position, positions[0])
	}

	// 半个 tick（25ms）：呈现落在 tick 100 与 101 的权威位置之间。
	projectiles.Advance(time.Second / 40)
	presentations = projectiles.AppendPresentations(nil)
	mid := positions[0].Add(positions[1].Sub(positions[0]).Mul(0.5))
	if presentations[0].Position != mid {
		t.Fatalf("半 tick 呈现=%v，想要插值区间内的 %v", presentations[0].Position, mid)
	}

	// 长时间推进：推进量钳制在单个 tick，绝不越过最新确认位置。
	projectiles.Advance(time.Second)
	presentations = projectiles.AppendPresentations(nil)
	if presentations[0].Position != positions[1] {
		t.Fatalf("长时间推进呈现=%v，想要钳制在已确认位置 %v", presentations[0].Position, positions[1])
	}
}

func TestProjectileMirrorRejectsInvalidMessages(t *testing.T) {
	projectiles := &Projectiles{}
	tests := []struct {
		name  string
		apply func() error
	}{
		{"spawn 非法弹种", func() error {
			return projectiles.ApplySpawn(projectileSpawnMessageOf(1, 7, 9, mgl32.Vec3{1, 1, 1}, mgl32.Vec3{}))
		}},
		{"state 非法排序", func() error {
			return projectiles.ApplyStates(network.ProjectileState{ServerTick: 1, States: []network.ProjectileStateRecord{
				{ID: 9, Position: mgl32.Vec3{1, 1, 1}},
				{ID: 7, Position: mgl32.Vec3{1, 1, 1}},
			}})
		}},
		{"despawn 空批次", func() error {
			return projectiles.ApplyDespawn(network.ProjectileDespawn{ServerTick: 1})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.apply(); !errors.Is(err, ErrProjectileProtocol) {
				t.Fatalf("错误=%v，想要 ErrProjectileProtocol", err)
			}
		})
	}
}

// TestProjectileVelocityEstimateDiffsAgainstLatestAuthoritativeSnapshot 锁定
// 速度再估计的基准：必须对最近一条权威快照位置差分，而不是呈现位置——
// 呈现位置在 ≥3 快照后落后插值滞后窗（2 tick），对它差分会系统性高估幅
// 值、把切线换成多 tick 弦。live 接线（Drain → Advance → RenderFrame）
// 在每批 state 之间都隔着 Advance，本测试按同形时序交错推进。
func TestProjectileVelocityEstimateDiffsAgainstLatestAuthoritativeSnapshot(t *testing.T) {
	projectiles := &Projectiles{}
	// 弹道位置：水平恒速 -2 格/tick，垂直受重力逐 tick 递减（-1、-2、-3、
	// -4）——切线方向逐 tick 变化，弦差分在垂直分量上必然露馅。
	positions := []mgl32.Vec3{
		{0, 64, 0}, {0, 63, -2}, {0, 61, -4}, {0, 58, -6}, {0, 54, -8},
	}
	if err := projectiles.ApplySpawn(projectileSpawnMessageOf(
		100, 7, network.ProjectileKindArrow, positions[0], mgl32.Vec3{0, -1, -2},
	)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	for tick := uint64(101); tick <= 104; tick++ {
		// 与 live 帧循环同形：上一批 state 确认后先推进插值再收下一批。
		projectiles.Advance(0)
		if err := projectiles.ApplyStates(projectileStateMessageOf(tick, 7, positions[tick-100])); err != nil {
			t.Fatalf("ApplyStates %d: %v", tick, err)
		}
	}
	presentations := projectiles.AppendPresentations(nil)
	// 末批（tick 104）的逐 tick 权威位移是 (0,-4,-2)：对快照差分即精确值，
	// 对滞后 2 tick 的呈现位置差分则得到 3 tick 弦 (0,-9,-6)/1。
	if got, want := presentations[0].Velocity, (mgl32.Vec3{0, -4, -2}); got != want {
		t.Fatalf("交错 Advance 后速度估计=%v，想要逐 tick 权威差分 %v", got, want)
	}

	// 常速直线弹道同样锁幅值：滞后基准会把 2 格/tick 高估成 6 格/tick。
	straight := &Projectiles{}
	straightPositions := []mgl32.Vec3{{0, 8, 0}, {0, 8, -2}, {0, 8, -4}, {0, 8, -6}, {0, 8, -8}}
	if err := straight.ApplySpawn(projectileSpawnMessageOf(
		100, 9, network.ProjectileKindShard, straightPositions[0], mgl32.Vec3{0, 0, -2},
	)); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	for tick := uint64(101); tick <= 104; tick++ {
		straight.Advance(0)
		if err := straight.ApplyStates(projectileStateMessageOf(tick, 9, straightPositions[tick-100])); err != nil {
			t.Fatalf("ApplyStates %d: %v", tick, err)
		}
	}
	presentations = straight.AppendPresentations(nil)
	if got, want := presentations[0].Velocity, (mgl32.Vec3{0, 0, -2}); got != want {
		t.Fatalf("常速弹道速度估计=%v，想要 %v", got, want)
	}
}

func TestProjectileResetClearsMirror(t *testing.T) {
	projectiles := &Projectiles{}
	if err := projectiles.ApplySpawn(projectileSpawnMessageOf(1, 7, network.ProjectileKindShard, mgl32.Vec3{1, 1, 1}, mgl32.Vec3{0, -1, 0})); err != nil {
		t.Fatalf("ApplySpawn: %v", err)
	}
	projectiles.Reset()
	if got := len(projectiles.AppendPresentations(nil)); got != 0 {
		t.Fatalf("Reset 后仍有 %d 枚投射物", got)
	}
	// Reset 后镜像可继续工作。
	if err := projectiles.ApplySpawn(projectileSpawnMessageOf(2, 8, network.ProjectileKindArrow, mgl32.Vec3{2, 1, 1}, mgl32.Vec3{1, 0, 0})); err != nil {
		t.Fatalf("Reset 后 ApplySpawn: %v", err)
	}
	if got := len(projectiles.AppendPresentations(nil)); got != 1 {
		t.Fatalf("Reset 后 spawn 数=%d，想要 1", got)
	}
}
