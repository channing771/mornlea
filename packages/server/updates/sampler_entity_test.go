package updates

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是随机面 `Sampler` 中 entity 家族判定流的已知答案测试（KAT）。锚点值
// 全部取自 `packages/server/sim/entity` 现行 splitmix64 哈希链实现在固定输入下的
// 实测输出（临时探针测试在基线上运行取得，探针不留在仓库）：entity 侧副本收敛
// 到本包后，任何搅拌顺序、次数或常量的漂移都会让这里的逐位比对变红。
//
// 与 realm 家族（sampler_test.go）分开成文件，是为了让两处锚点的来源各自可溯：
// 混排后无法分辨哪批向量钉的是哪侧搬迁。

func TestSamplerShortGrassSeedDropRollKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed int64
		dim  core.DimensionID
		pos  core.BlockPos
		want bool
	}{
		{0, 0, core.BlockPos{X: 0, Y: 1, Z: 5}, false},
		{-42, 7, core.BlockPos{X: -100, Y: 64, Z: 32000}, false},
		{8675309, 0, core.BlockPos{X: 300, Y: -64, Z: -300}, false},
		{5, 9, core.BlockPos{X: -2, Y: -3, Z: -4}, false},
		{11, 3, core.BlockPos{X: 1, Y: 64, Z: -1}, true},
	} {
		if got := sampler.ShortGrassSeedDropRoll(v.seed, v.dim, v.pos); got != v.want {
			t.Fatalf("ShortGrassSeedDropRoll(%d,%d,%+v)=%v，want %v",
				v.seed, v.dim, v.pos, got, v.want)
		}
	}
}

func TestSamplerLeavesSaplingDropRollKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed int64
		dim  core.DimensionID
		pos  core.BlockPos
		want bool
	}{
		{0, 0, core.BlockPos{X: 4, Y: 1, Z: 5}, false},
		{-42, 7, core.BlockPos{X: -100, Y: 64, Z: 32000}, false},
		{8675309, 0, core.BlockPos{X: 300, Y: -64, Z: -300}, false},
		{5, 9, core.BlockPos{X: -2, Y: -3, Z: -4}, true},
		{11, 3, core.BlockPos{X: 10, Y: 64, Z: -10}, false},
	} {
		if got := sampler.LeavesSaplingDropRoll(v.seed, v.dim, v.pos); got != v.want {
			t.Fatalf("LeavesSaplingDropRoll(%d,%d,%+v)=%v，want %v",
				v.seed, v.dim, v.pos, got, v.want)
		}
	}
}

func TestSamplerPassiveGrazeHitKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed int64
		tick uint64
		id   uint64
		want bool
	}{
		{0, 0, 41, false},
		{-42, 987654321, 7, false},
		{12345, 1 << 40, 999, false},
		{8675309, 13, 1 << 32, false},
		{7, 600, 0, false},
		{1000003, 12345, 777, false},
		{31, 2, 4, false},
		{-7, 99, 1, false},
		{0, 103, 41, true},
		{0, 180, 777, true},
		{0, 332, 1 << 32, true},
	} {
		if got := sampler.PassiveGrazeHit(v.seed, v.tick, v.id); got != v.want {
			t.Fatalf("PassiveGrazeHit(%d,%d,%d)=%v，want %v",
				v.seed, v.tick, v.id, got, v.want)
		}
	}
}

func TestSamplerHostileCandidateHashKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed    int64
		tick    uint64
		x, y, z int32
		want    uint64
	}{
		{0, 13001, 24, 1, 0, 0x8bcd14a8d5cf3d91},
		{-42, 23000, -24, 64, -48, 0xedf4e30235ad740a},
		{12345, 1, 100, -1, 100, 0xf4bbb82351e0ad32},
		{8675309, 1 << 40, 0, 0, 0, 0x938e0f4f216a8b27},
		{-1234567890123456789, 65535, -100, 319, 100, 0xc649b45e35a70345},
	} {
		if got := sampler.HostileCandidateHash(v.seed, v.tick, v.x, v.y, v.z); got != v.want {
			t.Fatalf("HostileCandidateHash(%d,%d,%d,%d,%d)=%#x，want %#x",
				v.seed, v.tick, v.x, v.y, v.z, got, v.want)
		}
	}
}

// TestSamplerEntitySalts 把 entity 家族的域盐值与抽选分母钉在收敛时的取值上：
// 盐值是判定流身份的一部分，任何「顺手美化」都是行为变更，必须过 KAT。
func TestSamplerEntitySalts(t *testing.T) {
	for _, v := range []struct {
		name string
		got  uint64
		want uint64
	}{
		{"ShortGrassSeedDropSalt", ShortGrassSeedDropSalt, 0x4752_4153_5353_4544},
		{"LeavesSaplingDropSalt", LeavesSaplingDropSalt, 0x5341_504C_494E_4753},
		{"PassiveGrazeRollSalt", PassiveGrazeRollSalt, 0x51ab3e4d07c3f291},
	} {
		if v.got != v.want {
			t.Fatalf("%s=%#x，want %#x", v.name, v.got, v.want)
		}
	}
	if PassiveGrazePeriodTicks != 600 {
		t.Fatalf("PassiveGrazePeriodTicks=%d，want 600", PassiveGrazePeriodTicks)
	}
}
