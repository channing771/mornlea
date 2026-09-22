package updates

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是随机面 `Sampler` 的已知答案测试（KAT）。锚点值全部取自
// `packages/server/sim/realm` 现行 splitmix64 哈希链实现在固定输入下的实测输出
// （临时探针测试在基线上运行取得，探针不留在仓库）：随机面搬迁导出后，任何
// 搅拌顺序、次数或常量的漂移都会让这里的逐位比对变红。已知答案与迁移前的
// 行为逐位一致，是后续把 sim/realm 换接到 `Sampler` 的等价性依据。

func TestSamplerSplitMix64KAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		x    uint64
		want uint64
	}{
		{0, 0xe220a8397b1dcdaf},
		{1, 0x910a2dec89025cc1},
		{0x0123456789abcdef, 0x157a3807a48faa9d},
		{0xfedcba9876543210, 0x7ae893b5e32fee86},
		{0xdeadbeefcafebabe, 0x0d7d93560d1929d2},
	} {
		if got := sampler.SplitMix64(v.x); got != v.want {
			t.Fatalf("SplitMix64(%#x)=%#x，want %#x", v.x, got, v.want)
		}
	}
}

func TestSamplerCropSectionHashKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed     int64
		tick     uint64
		key      core.ChunkKey
		sectionY int
		want     uint64
	}{
		{123456789, 987654321, core.ChunkKey{Dimension: 3, Pos: core.ChunkPos{X: -17, Z: 42}}, 13, 0x1049d15ff181bbfd},
		{-1234567890123456789, 1, core.ChunkKey{}, 0, 0xa556fb29da9e8687},
		{0, 0, core.ChunkKey{}, 0, 0xcbd37ad29b93b094},
		{42, 4294967296, core.ChunkKey{Dimension: 255, Pos: core.ChunkPos{X: 65535, Z: -65536}}, 23, 0xe9b84390e6a0627a},
	} {
		got := sampler.CropSectionHash(v.seed, v.tick, v.key, v.sectionY)
		if got != v.want {
			t.Fatalf("CropSectionHash(%d,%d,%+v,%d)=%#x，want %#x",
				v.seed, v.tick, v.key, v.sectionY, got, v.want)
		}
	}
}

func TestSamplerSampleCellsKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed     int64
		tick     uint64
		key      core.ChunkKey
		sectionY int
		n        int
		want     []int
	}{
		{-42, 777, core.ChunkKey{Dimension: 1, Pos: core.ChunkPos{X: 5, Z: -7}}, 9, 8,
			[]int{3835, 318, 2309, 2970, 832, 2023, 3058, 3225}},
		{99, 1000003, core.ChunkKey{Dimension: 2, Pos: core.ChunkPos{X: -100, Z: 100}}, 0, 3,
			[]int{2673, 1807, 4094}},
		{7, 8, core.ChunkKey{}, 23, 1, []int{3507}},
		{7, 8, core.ChunkKey{}, 23, 0, []int{}},
	} {
		got := sampler.SampleCells(v.seed, v.tick, v.key, v.sectionY, v.n, nil)
		if !slices.Equal(got, v.want) {
			t.Fatalf("SampleCells(%d,%d,%+v,%d,%d)=%v，want %v",
				v.seed, v.tick, v.key, v.sectionY, v.n, got, v.want)
		}
	}
}

func TestSamplerCropGrowthRollKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed   int64
		tick   uint64
		dim    core.DimensionID
		pos    core.BlockPos
		chance uint8
		want   bool
	}{
		{-42, 777, 2, core.BlockPos{X: -100, Y: 64, Z: 32000}, 35, true},
		{8675309, 1099511627776, 0, core.BlockPos{X: 300, Y: -64, Z: -300}, 50, true},
		{5, 6, 9, core.BlockPos{X: -2, Y: -3, Z: -4}, 7, false},
		{11, 22, 3, core.BlockPos{X: 0, Y: 64, Z: 0}, 50, false},
		{123, 456, 8, core.BlockPos{X: 1, Y: 1, Z: 1}, 0, false},  // 零概率短路
		{123, 456, 8, core.BlockPos{X: 1, Y: 1, Z: 1}, 100, true}, // 满概率短路
	} {
		got := sampler.CropGrowthRoll(v.seed, v.tick, v.dim, v.pos, v.chance)
		if got != v.want {
			t.Fatalf("CropGrowthRoll(%d,%d,%d,%+v,%d)=%v，want %v",
				v.seed, v.tick, v.dim, v.pos, v.chance, got, v.want)
		}
	}
}

func TestSamplerCropYieldRollsKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed           int64
		tick           uint64
		dim            core.DimensionID
		pos            core.BlockPos
		wheat, seeds   uint8
		potato, carrot uint8
		poison         bool
	}{
		{8675309, 1099511627776, 0, core.BlockPos{X: 300, Y: -64, Z: -300}, 1, 3, 1, 4, false},
		{-1, 0, 255, core.BlockPos{X: -1, Y: 320, Z: 1}, 2, 1, 3, 2, false},
		{24680, 13579, 3, core.BlockPos{X: 4096, Y: -4096, Z: 0}, 1, 1, 3, 1, false},
	} {
		wheat, seeds := sampler.CropYieldRolls(v.seed, v.tick, v.dim, v.pos)
		if wheat != v.wheat || seeds != v.seeds {
			t.Fatalf("CropYieldRolls(%d,%d,%d,%+v)=(%d,%d)，want (%d,%d)",
				v.seed, v.tick, v.dim, v.pos, wheat, seeds, v.wheat, v.seeds)
		}
		if got := sampler.CropYieldRollsPotato(v.seed, v.tick, v.dim, v.pos); got != v.potato {
			t.Fatalf("CropYieldRollsPotato(%d,%d,%d,%+v)=%d，want %d",
				v.seed, v.tick, v.dim, v.pos, got, v.potato)
		}
		if got := sampler.CropYieldRollsCarrot(v.seed, v.tick, v.dim, v.pos); got != v.carrot {
			t.Fatalf("CropYieldRollsCarrot(%d,%d,%d,%+v)=%d，want %d",
				v.seed, v.tick, v.dim, v.pos, got, v.carrot)
		}
		if got := sampler.PoisonRoll(v.seed, v.tick, v.dim, v.pos); got != v.poison {
			t.Fatalf("PoisonRoll(%d,%d,%d,%+v)=%v，want %v",
				v.seed, v.tick, v.dim, v.pos, got, v.poison)
		}
	}
	// 毒土豆判定单独补一个命中向量（命中率 1/50，上表全为未命中）。
	if !sampler.PoisonRoll(11, 22, 3, core.BlockPos{X: 1, Y: 64, Z: -1}) {
		t.Fatalf("PoisonRoll 命中向量应返回 true")
	}
}

func TestSamplerFarmlandRevertRollKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed int64
		tick uint64
		dim  core.DimensionID
		pos  core.BlockPos
		want bool
	}{
		{-1, 65535, 7, core.BlockPos{X: 1, Y: 2, Z: 3}, false},
		{24680, 13579, 3, core.BlockPos{X: 4096, Y: -4096, Z: 0}, false},
		{11, 22, 3, core.BlockPos{X: 1, Y: 64, Z: -1}, true},
	} {
		if got := sampler.FarmlandRevertRoll(v.seed, v.tick, v.dim, v.pos); got != v.want {
			t.Fatalf("FarmlandRevertRoll(%d,%d,%d,%+v)=%v，want %v",
				v.seed, v.tick, v.dim, v.pos, got, v.want)
		}
	}
}

func TestSamplerSaplingGrowthRollKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed int64
		tick uint64
		dim  core.DimensionID
		pos  core.BlockPos
		want bool
	}{
		{4611686018427387904, 9223372036854775808, 4, core.BlockPos{X: -1, Y: -2, Z: -3}, false},
		{0, 0, 0, core.BlockPos{}, false},
		{11, 22, 3, core.BlockPos{X: 10, Y: 64, Z: -10}, true},
	} {
		if got := sampler.SaplingGrowthRoll(v.seed, v.tick, v.dim, v.pos); got != v.want {
			t.Fatalf("SaplingGrowthRoll(%d,%d,%d,%+v)=%v，want %v",
				v.seed, v.tick, v.dim, v.pos, got, v.want)
		}
	}
}

// `TestSamplerGrassSpreadRollKAT` pins measured outputs for fixed inputs. Its
// anchors capture the `hash&3 == 0` mask and independent salt chain, so mixing
// order, mask width, or salt drift changes the bitwise comparison.
func TestSamplerGrassSpreadRollKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed int64
		tick uint64
		dim  core.DimensionID
		pos  core.BlockPos
		want bool
	}{
		{4611686018427387904, 9223372036854775808, 4, core.BlockPos{X: -1, Y: -2, Z: -3}, false},
		{0, 0, 0, core.BlockPos{}, false},
		{11, 22, 3, core.BlockPos{X: 10, Y: 64, Z: -10}, true},
		{-42, 777, 2, core.BlockPos{X: -100, Y: 64, Z: 32000}, false},
		{8675309, 1099511627776, 0, core.BlockPos{X: 300, Y: -64, Z: -300}, false},
	} {
		if got := sampler.GrassSpreadRoll(v.seed, v.tick, v.dim, v.pos); got != v.want {
			t.Fatalf("GrassSpreadRoll(%d,%d,%d,%+v)=%v，want %v",
				v.seed, v.tick, v.dim, v.pos, got, v.want)
		}
	}
}

// TestSamplerSaltConstants 把全部域盐值与概率常量钉在搬迁时的取值上：盐值是
// 判定流身份的一部分，任何「顺手美化」都是行为变更，必须过 KAT。
func TestSamplerSaltConstants(t *testing.T) {
	for _, v := range []struct {
		name string
		got  uint64
		want uint64
	}{
		{"CropGrowthRollSalt", CropGrowthRollSalt, 0xc0ffee5eedca11ed},
		{"CropYieldRollSalt", CropYieldRollSalt, 0x5eedfeedfaceface},
		{"CropYieldPotatoSalt", CropYieldPotatoSalt, 0x70a70a515eedface},
		{"CropYieldCarrotSalt", CropYieldCarrotSalt, 0xca7707701ace5eed},
		{"PoisonPotatoSalt", PoisonPotatoSalt, 0xdeadbeefcafe1234},
		{"FarmlandRevertRollSalt", FarmlandRevertRollSalt, 0xfa1abb1edeadc0de},
		{"SaplingGrowthRollSalt", SaplingGrowthRollSalt, 0x5341_504C_4752_4F57},
		{"GrassSpreadRollSalt", GrassSpreadRollSalt, 0x4752_5350_5245_4144},
		{"ProjectileSpawnSalt", ProjectileSpawnSalt, 0x5052_4F4A_4543_5449},
		{"HostileShotSpreadSalt", HostileShotSpreadSalt, 0x5348_4F54_5350_5244},
		{"HostileHurlerDropSalt", HostileHurlerDropSalt, 0x4855_524C_4452_4F50},
	} {
		if v.got != v.want {
			t.Fatalf("%s=%#x，want %#x", v.name, v.got, v.want)
		}
	}
	if FarmlandRevertChancePercent != 30 {
		t.Fatalf("FarmlandRevertChancePercent=%d，want 30", FarmlandRevertChancePercent)
	}
}

// `TestSamplerSaltsPairwiseDistinct` pins all 14 salts as pairwise distinct:
// eight realm streams, three entity streams, one projectile-ID derivation, and
// two hostile-hurler decisions. Denominators and probability constants are not salts.
// 各判定流只靠盐值区分身份，未来追加新盐值若与既有任何一个撞值，两条流会在
// 相同 `(种子, tick, 维度, 位置)` 输入下系统性同源——这条两两互异断言让撞盐
// 在新增当刻变红，无须等到分布级测试以统计方式偶然暴露。
func TestSamplerSaltsPairwiseDistinct(t *testing.T) {
	salts := []struct {
		name  string
		value uint64
	}{
		{"CropGrowthRollSalt", CropGrowthRollSalt},
		{"CropYieldRollSalt", CropYieldRollSalt},
		{"CropYieldPotatoSalt", CropYieldPotatoSalt},
		{"CropYieldCarrotSalt", CropYieldCarrotSalt},
		{"PoisonPotatoSalt", PoisonPotatoSalt},
		{"FarmlandRevertRollSalt", FarmlandRevertRollSalt},
		{"SaplingGrowthRollSalt", SaplingGrowthRollSalt},
		{"GrassSpreadRollSalt", GrassSpreadRollSalt},
		{"ShortGrassSeedDropSalt", ShortGrassSeedDropSalt},
		{"LeavesSaplingDropSalt", LeavesSaplingDropSalt},
		{"PassiveGrazeRollSalt", PassiveGrazeRollSalt},
		{"ProjectileSpawnSalt", ProjectileSpawnSalt},
		{"HostileShotSpreadSalt", HostileShotSpreadSalt},
		{"HostileHurlerDropSalt", HostileHurlerDropSalt},
	}
	if len(salts) != 14 {
		t.Fatalf("盐值清单长度 %d，想要 14——新增盐值后必须把本断言的清单同步扩容", len(salts))
	}
	for i := range salts {
		for j := i + 1; j < len(salts); j++ {
			if salts[i].value == salts[j].value {
				t.Fatalf("%s 与 %s 撞盐 %#x：两条判定流会同源", salts[i].name, salts[j].name, salts[i].value)
			}
		}
	}
}

// TestSamplerReplayBitIdentical 钉住「相同输入重放逐位一致」：随机面是纯整数
// 哈希，不允许引入任何进程级随机源或隐藏状态——同一批输入执行两次，全部判定
// 逐位一致。
func TestSamplerReplayBitIdentical(t *testing.T) {
	run := func() []bool {
		sampler := Sampler{}
		var out []bool
		for i := range 512 {
			seed := int64(i)*7919 - 12345
			tick := uint64(i) * 1_000_003
			dim := core.DimensionID(i % 5)
			pos := spreadPos(i)
			out = append(out,
				sampler.CropGrowthRoll(seed, tick, dim, pos, 40),
				sampler.FarmlandRevertRoll(seed, tick, dim, pos),
				sampler.SaplingGrowthRoll(seed, tick, dim, pos),
				sampler.PoisonRoll(seed, tick, dim, pos),
			)
		}
		return out
	}
	first, second := run(), run()
	if !slices.Equal(first, second) {
		t.Fatalf("相同输入两次执行的判定不一致：随机面引入了隐藏状态")
	}
}

// TestSamplerDomainSaltsIndependent 钉住「不同域盐值相互独立」：两个共享全部
// `(种子, tick, 维度, 位置)` 输入的判定，在大量输入上四种命中组合都必须出现——
// 任一判定可由另一判定推出（恒等、恒反）时必有组合缺席。
func TestSamplerDomainSaltsIndependent(t *testing.T) {
	sampler := Sampler{}
	var combos [4]int
	for i := range 20000 {
		pos := spreadPos(i % 4096)
		growth := sampler.CropGrowthRoll(7, 11, 1, pos, 50)
		revert := sampler.FarmlandRevertRoll(7, 11, 1, pos)
		combos[boolToInt(growth)+2*boolToInt(revert)]++
	}
	for i, count := range combos {
		if count == 0 {
			t.Fatalf("两域判定在 2 万个共享输入上组合 %d 从未出现：判定不再相互独立，分布 %v", i, combos)
		}
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestHostileShotSpreadKAT 把掷骨者射击散布钉在固定输入的实测输出上：散布是
// 命中判定的数值契约输入，任何搅拌顺序或映射漂移都会让这里的逐位比对变红。
func TestHostileShotSpreadKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed      int64
		tick      uint64
		id        uint64
		wantYaw   float32
		wantPitch float32
	}{
		{0, 1, 1, 0.0020711517, -0.0060293195},
		{42, 13000, 0xdeadbeefcafebabe, -0.049812812, 0.047607422},
		{-7, 987654321, 7777, 0.055351753, -0.058046035},
	} {
		gotYaw, gotPitch := sampler.HostileShotSpread(v.seed, v.tick, v.id)
		if gotYaw != v.wantYaw || gotPitch != v.wantPitch {
			t.Fatalf("HostileShotSpread(%d,%d,%d)=(%v,%v)，want (%v,%v)",
				v.seed, v.tick, v.id, gotYaw, gotPitch, v.wantYaw, v.wantPitch)
		}
	}
}

// TestHostileShotSpreadBoundedAndVaried 钉住散布的形状契约：偏移以 0.06 rad
// 为界（固定数值契约）、不同输入覆盖多种偏移（不是常量）、相同输入重放
// 逐位一致。
func TestHostileShotSpreadBoundedAndVaried(t *testing.T) {
	sampler := Sampler{}
	limit := HostileShotSpreadMaxRadians
	seen := make(map[[2]float32]struct{})
	for i := range 20000 {
		seed := int64(i)*7919 - 12345
		tick := uint64(i) * 1_000_003
		yaw, pitch := sampler.HostileShotSpread(seed, tick, uint64(i)*31+1)
		if yaw > limit || yaw < -limit || pitch > limit || pitch < -limit {
			t.Fatalf("散布偏移 (%v,%v) 越出 ±%v", yaw, pitch, limit)
		}
		seen[[2]float32{yaw, pitch}] = struct{}{}
	}
	if len(seen) < 10000 {
		t.Fatalf("2 万个输入只覆盖 %d 种偏移，散布映射退化", len(seen))
	}
	aYaw, aPitch := sampler.HostileShotSpread(3, 100, 55)
	bYaw, bPitch := sampler.HostileShotSpread(3, 100, 55)
	if aYaw != bYaw || aPitch != bPitch {
		t.Fatalf("相同输入重放不一致：(%v,%v) vs (%v,%v)", aYaw, aPitch, bYaw, bPitch)
	}
}

// TestHostileHurlerDropRollsKAT 把掷骨者死亡掉落判定钉在固定输入的实测输出
// 上：骨头数量 0..2 与弓的 1/8 判定共用一条盐值链，任何漂移都在此变红。
func TestHostileHurlerDropRollsKAT(t *testing.T) {
	sampler := Sampler{}
	for _, v := range []struct {
		seed  int64
		tick  uint64
		id    uint64
		bones uint8
		bow   bool
	}{
		{0, 4000, 21, 2, true},
		{42, 4000, 0xdeadbeefcafebabe, 0, false},
		{-7, 987654321, 7777, 0, false},
	} {
		gotBones, gotBow := sampler.HostileHurlerDropRolls(v.seed, v.tick, v.id)
		if gotBones != v.bones || gotBow != v.bow {
			t.Fatalf("HostileHurlerDropRolls(%d,%d,%d)=(%d,%v)，want (%d,%v)",
				v.seed, v.tick, v.id, gotBones, gotBow, v.bones, v.bow)
		}
	}
}

// TestHostileHurlerDropRollsDistribution 钉住掉落判定的分布契约：骨头 0..2
// 三档都要出现（比例约 1:1:1），弓命中率约 1/8（宽松边界），重放逐位一致。
func TestHostileHurlerDropRollsDistribution(t *testing.T) {
	sampler := Sampler{}
	var boneCounts [3]int
	bowHits := 0
	const samples = 60000
	for i := range samples {
		bones, bow := sampler.HostileHurlerDropRolls(int64(i)*7919-12345, uint64(i)*1_000_003, uint64(i)*31+1)
		boneCounts[bones]++
		if bow {
			bowHits++
		}
	}
	for count, hit := range boneCounts {
		ratio := float64(hit) / samples
		if ratio < 0.28 || ratio > 0.39 {
			t.Fatalf("骨头 %d 根的比例=%v，越出均匀三档的宽松边界 [0.28, 0.39]", count, ratio)
		}
	}
	bowRatio := float64(bowHits) / samples
	if bowRatio < 0.1 || bowRatio > 0.16 {
		t.Fatalf("弓命中率=%v，越出 1/8 的宽松边界 [0.10, 0.16]", bowRatio)
	}
	firstBones, firstBow := sampler.HostileHurlerDropRolls(9, 9, 9)
	secondBones, secondBow := sampler.HostileHurlerDropRolls(9, 9, 9)
	if firstBones != secondBones || firstBow != secondBow {
		t.Fatal("相同输入重放不一致：掉落判定引入了隐藏状态")
	}
}
