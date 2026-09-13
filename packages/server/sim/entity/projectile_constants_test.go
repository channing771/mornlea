package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/physics"
)

// 本文件钉住投射物域的固定数值契约：重力、两类弹种的初速与伤害、寿命、
// 集合上限与弹种编号。这些值是跨语言/跨层数字契约（协议 v43 与后续发布侧
// 按同值消费），任何一侧单独调整都必须全链同步并更新测试。

func TestProjectileNumericContractIsFrozen(t *testing.T) {
	if projectileGravity != 18 {
		t.Fatalf("projectileGravity=%v，想要 18 格/秒²", projectileGravity)
	}
	if projectileArrowShortSpeed != 16 {
		t.Fatalf("projectileArrowShortSpeed=%v，想要 16 格/秒", projectileArrowShortSpeed)
	}
	if projectileArrowFullSpeed != 30 {
		t.Fatalf("projectileArrowFullSpeed=%v，想要 30 格/秒", projectileArrowFullSpeed)
	}
	if projectileShardSpeed != 22 {
		t.Fatalf("projectileShardSpeed=%v，想要 22 格/秒", projectileShardSpeed)
	}
	if projectileShardDamage != 3 {
		t.Fatalf("projectileShardDamage=%d，想要 3", projectileShardDamage)
	}
	if projectileArrowShortDamage != 2 {
		t.Fatalf("projectileArrowShortDamage=%d，想要 2", projectileArrowShortDamage)
	}
	if projectileArrowFullDamage != 5 {
		t.Fatalf("projectileArrowFullDamage=%d，想要 5", projectileArrowFullDamage)
	}
	if projectileMaxLifetimeTicks != 100 {
		t.Fatalf("projectileMaxLifetimeTicks=%d，想要 100", projectileMaxLifetimeTicks)
	}
	if maxProjectiles != 128 {
		t.Fatalf("maxProjectiles=%d，想要 128", maxProjectiles)
	}
	// 弹种编号与协议 v43 的 kind 字节同值：0=骨刺（远程敌怪）、1=箭（玩家弓）。
	if projectileKindShard != 0 || projectileKindArrow != 1 {
		t.Fatalf("弹种编号漂移：shard=%d arrow=%d，想要 0/1", projectileKindShard, projectileKindArrow)
	}
	// 积分步长与玩家/敌怪物理同源：同一 FixedDelta 语义。
	if physics.FixedDeltaSeconds != 0.05 {
		t.Fatalf("physics.FixedDeltaSeconds=%v，想要 0.05", physics.FixedDeltaSeconds)
	}
}
