package tuning

import (
	"reflect"
	"sync"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestConcurrentSetAndActiveTunablesReturnWholeSnapshots 证明并发读写只能观察到
// 某次完整发布的值，不能把两个配置的字段拼成撕裂快照。
func TestConcurrentSetAndActiveTunablesReturnWholeSnapshots(t *testing.T) {
	t.Cleanup(func() { SetTunables(DefaultTunables()) })

	first := wholeSnapshotFixture(1)
	second := wholeSnapshotFixture(2)
	firstValue := reflect.ValueOf(first)
	secondValue := reflect.ValueOf(second)
	for index := range firstValue.NumField() {
		if reflect.DeepEqual(firstValue.Field(index).Interface(), secondValue.Field(index).Interface()) {
			t.Fatalf("A/B 快照字段 %s 未取不同值", firstValue.Type().Field(index).Name)
		}
	}
	SetTunables(first)

	const readerCount = 8
	readersReady := make(chan struct{}, readerCount)
	startWriter := make(chan struct{})
	writerStarted := make(chan struct{})
	readersEntered := make(chan struct{}, readerCount)
	startReads := make(chan struct{})
	stopWriter := make(chan struct{})
	writerDone := make(chan struct{})
	unexpected := make(chan Tunables, 1)
	var group sync.WaitGroup
	group.Add(readerCount)
	go func() {
		defer close(writerDone)
		<-startWriter
		SetTunables(second)
		close(writerStarted)
		for range readerCount {
			<-readersEntered
		}
		close(startReads)
		for {
			select {
			case <-stopWriter:
				return
			default:
				SetTunables(first)
				SetTunables(second)
			}
		}
	}()
	for range readerCount {
		go func() {
			defer group.Done()
			readersReady <- struct{}{}
			<-writerStarted
			readersEntered <- struct{}{}
			<-startReads
			for range 5000 {
				got := ActiveTunables()
				if got == first || got == second {
					continue
				}
				select {
				case unexpected <- got:
				default:
				}
				return
			}
		}()
	}
	for range readerCount {
		<-readersReady
	}
	close(startWriter)
	group.Wait()
	close(stopWriter)
	<-writerDone
	select {
	case got := <-unexpected:
		t.Fatalf("并发读取观察到撕裂快照：%+v", got)
	default:
	}
}

func wholeSnapshotFixture(value uint32) Tunables {
	return Tunables{
		InteractionReach:              float32(value),
		RegenDelayTicks:               value,
		RegenIntervalTicks:            value,
		DrownDamageIntervalTicks:      value,
		DropPickupDelayTicks:          uint8(value),
		PlayerDropPickupDelayTicks:    uint8(value),
		DropLifetimeTicks:             value,
		DropPickupRange:               float32(value),
		SpawnRadius:                   int32(value),
		FurnaceSmeltTicks:             uint8(value),
		FurnaceBurnTicks:              uint16(value),
		FluidFlowDelayTicks:           value,
		FluidUpdatesPerTick:           value,
		FluidRescanCellsPerTick:       value,
		RandomTicksPerSection:         uint8(value),
		CropGrowthChancePercent:       uint8(value),
		StarvationDamageIntervalTicks: value,
		ExhaustionThresholdMilli:      uint16(value),
		RegenHungerThreshold:          uint8(value),
		EatingTicks:                   uint16(value),
	}
}

func TestDefaultTunablesMatchLegacyConstants(t *testing.T) {
	tunables := DefaultTunables()
	for _, check := range []struct {
		name      string
		got, want float64
	}{
		{"InteractionReach", float64(tunables.InteractionReach), 6},
		{"RegenDelayTicks", float64(tunables.RegenDelayTicks), 100},
		{"RegenIntervalTicks", float64(tunables.RegenIntervalTicks), 40},
		{"DrownDamageIntervalTicks", float64(tunables.DrownDamageIntervalTicks), 20},
		{"DropPickupDelayTicks", float64(tunables.DropPickupDelayTicks), 10},
		{"PlayerDropPickupDelayTicks", float64(tunables.PlayerDropPickupDelayTicks), 40},
		{"DropLifetimeTicks", float64(tunables.DropLifetimeTicks), 6000},
		{"DropPickupRange", float64(tunables.DropPickupRange), 1.25},
		{"SpawnRadius", float64(tunables.SpawnRadius), 16},
		{"FurnaceSmeltTicks", float64(tunables.FurnaceSmeltTicks), float64(core.FurnaceSmeltTicks)},
		{"FurnaceBurnTicks", float64(tunables.FurnaceBurnTicks), float64(core.FurnaceBurnTicks)},
		{"FluidFlowDelayTicks", float64(tunables.FluidFlowDelayTicks), 5},
		{"FluidUpdatesPerTick", float64(tunables.FluidUpdatesPerTick), 512},
		{"FluidRescanCellsPerTick", float64(tunables.FluidRescanCellsPerTick), 65536},
		{"RandomTicksPerSection", float64(tunables.RandomTicksPerSection), 3},
		{"CropGrowthChancePercent", float64(tunables.CropGrowthChancePercent), 50},
		{"StarvationDamageIntervalTicks", float64(tunables.StarvationDamageIntervalTicks), 80},
		{"ExhaustionThresholdMilli", float64(tunables.ExhaustionThresholdMilli), 4000},
		{"RegenHungerThreshold", float64(tunables.RegenHungerThreshold), 18},
		{"EatingTicks", float64(tunables.EatingTicks), 32},
	} {
		if check.got != check.want {
			t.Errorf("%s = %v，want %v", check.name, check.got, check.want)
		}
	}
}

func TestActiveTunablesDefaultsToDefaultTunables(t *testing.T) {
	if ActiveTunables() != DefaultTunables() {
		t.Fatal("未经设置时生效参数必须等于默认参数")
	}
}

// TestSetTunablesClampsAuthorityTickInvariants 证明 SetTunables 兜住了两条
// 直接决定权威 tick 安全的不变量：RegenIntervalTicks 是取模除数（0 会 panic），
// SpawnRadius 决定一次平方级分配（不钳制会触发巨额分配）。
//
// 这两条区间在 packages/shared/config 里也有一份，靠约定隔着一个包维持不变量是不够的。
func TestSetTunablesClampsAuthorityTickInvariants(t *testing.T) {
	t.Cleanup(func() { SetTunables(DefaultTunables()) })

	unsafe := DefaultTunables()
	unsafe.RegenIntervalTicks = 0
	unsafe.SpawnRadius = 100000
	SetTunables(unsafe)
	if got := ActiveTunables().RegenIntervalTicks; got < 1 {
		t.Errorf("RegenIntervalTicks = %d，必须钳到 >= 1（否则取模除零 panic）", got)
	}
	if got := ActiveTunables().SpawnRadius; got != maxSpawnRadius {
		t.Errorf("SpawnRadius = %d，必须钳到上界 %d", got, maxSpawnRadius)
	}

	unsafe.SpawnRadius = -5
	SetTunables(unsafe)
	if got := ActiveTunables().SpawnRadius; got != minSpawnRadius {
		t.Errorf("SpawnRadius = %d，必须钳到下界 %d", got, minSpawnRadius)
	}
}

// TestSetTunablesRoundTripsFluidFields 证明 FluidFlowDelayTicks 与
// FluidUpdatesPerTick 已按既有 tunable 约定接入 SetTunables/ActiveTunables
// 快照机制——本组只定义这两个值，尚无消费方读取它们（见字段 GoDoc），但快照
// 写入与读出本身必须已经生效，供后续调用方直接消费。
func TestSetTunablesRoundTripsFluidFields(t *testing.T) {
	t.Cleanup(func() { SetTunables(DefaultTunables()) })

	custom := DefaultTunables()
	custom.FluidFlowDelayTicks = 9
	custom.FluidUpdatesPerTick = 1024
	SetTunables(custom)

	if got := ActiveTunables().FluidFlowDelayTicks; got != 9 {
		t.Errorf("FluidFlowDelayTicks = %d，want 9", got)
	}
	if got := ActiveTunables().FluidUpdatesPerTick; got != 1024 {
		t.Errorf("FluidUpdatesPerTick = %d，want 1024", got)
	}
}
