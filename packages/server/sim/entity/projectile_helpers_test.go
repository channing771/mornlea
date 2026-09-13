package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是投射物域测试的共享夹具：白盒生成入口、视图刷新包装。各行为主题
// 分布在同目录其余 projectile_*_test.go 中，一个文件一个主题。

// fixtureViewRadius 是实体夹具为「未声明视距」的会话填充的订阅半径：与
// runtime 生产装配的缺省视界同值（server config 默认 ViewRadius）。夹具没有
// 引擎视界可钳制，直接以该值代表生效半径；需要精确控制订阅范围的用例经
// setSessionViewForTest 覆盖。
const fixtureViewRadius = 33

// refreshViewsForTest 重建传给 production 阶段的只读订阅快照：直接推进
// 投射物阶段的用例不经 beginTick，需要显式刷新一次让 despawn 判定看到会话。
func refreshViewsForTest(engine *Engine) {
	engine.engineContext.views = engine.viewSnapshot()
}

// spawnTestProjectile 是 spawnProjectile 的断言包装：生成失败即测试失败，
// 成功返回派生 ID。
func spawnTestProjectile(
	t *testing.T,
	engine *Engine,
	kind uint8,
	dimension core.DimensionID,
	position mgl32.Vec3,
	velocity mgl32.Vec3,
	owner uint64,
	damage int32,
) uint64 {
	t.Helper()
	id, ok := engine.spawnProjectile(kind, dimension, position, velocity, owner, damage)
	if !ok || id == 0 {
		t.Fatalf("生成测试投射物失败：ok=%v id=%d", ok, id)
	}
	return id
}

// projectileAt 返回指定 ID 的投射物条目与存在性，供存在性与字段断言复用。
func projectileAt(engine *Engine, id uint64) (projectileState, bool) {
	index := engine.projectiles.findIndex(id)
	if index < 0 {
		return projectileState{}, false
	}
	return engine.projectiles.entries[index], true
}
