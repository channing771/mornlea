package presentation

import (
	"math"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestSessionPhaseDomainIncludesSafeZero(t *testing.T) {
	for _, phase := range []SessionPhase{
		SessionPhaseNotReady,
		SessionPhaseConnecting,
		SessionPhaseLogin,
		SessionPhaseLoading,
		SessionPhasePlay,
		SessionPhaseDisconnected,
	} {
		if !phase.Valid() {
			t.Fatalf("SessionPhase(%d).Valid() = false", phase)
		}
	}
	if SessionPhase(255).Valid() {
		t.Fatal("out-of-range session phase is valid")
	}
	var phase SessionPhase
	if phase != SessionPhaseNotReady {
		t.Fatalf("zero session phase = %d, want not-ready", phase)
	}
}

func TestCameraSnapshotDomainAndZero(t *testing.T) {
	var zero CameraSnapshot
	if zero.Ready || zero.Validate() != nil {
		t.Fatalf("zero camera = %+v, want valid not-ready value", zero)
	}

	valid := CameraSnapshot{
		Ready:    true,
		Position: [3]float32{-12.5, 64.25, 9},
		Yaw:      1.25,
		Pitch:    -0.5,
		FOVY:     float32(math.Pi / 3),
		Aspect:   16.0 / 9.0,
		Near:     0.1,
		Far:      512,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid camera: %v", err)
	}
	for _, pitch := range []float32{-cameraPitchLimit, cameraPitchLimit} {
		boundary := valid
		boundary.Pitch = pitch
		if err := boundary.Validate(); err != nil {
			t.Fatalf("camera pitch boundary %v: %v", pitch, err)
		}
	}

	tests := []struct {
		name   string
		mutate func(*CameraSnapshot)
	}{
		{"not-ready payload", func(snapshot *CameraSnapshot) { snapshot.Ready = false }},
		{"non-finite position", func(snapshot *CameraSnapshot) { snapshot.Position[1] = float32(math.NaN()) }},
		{"non-finite yaw", func(snapshot *CameraSnapshot) { snapshot.Yaw = float32(math.Inf(1)) }},
		{"non-finite pitch", func(snapshot *CameraSnapshot) { snapshot.Pitch = float32(math.NaN()) }},
		{"pitch below domain", func(snapshot *CameraSnapshot) { snapshot.Pitch = -float32(math.Pi / 2) }},
		{"pitch above domain", func(snapshot *CameraSnapshot) { snapshot.Pitch = float32(math.Pi / 2) }},
		{"non-finite field of view", func(snapshot *CameraSnapshot) { snapshot.FOVY = float32(math.Inf(1)) }},
		{"zero field of view", func(snapshot *CameraSnapshot) { snapshot.FOVY = 0 }},
		{"field of view at pi", func(snapshot *CameraSnapshot) { snapshot.FOVY = float32(math.Pi) }},
		{"non-finite aspect", func(snapshot *CameraSnapshot) { snapshot.Aspect = float32(math.NaN()) }},
		{"zero aspect", func(snapshot *CameraSnapshot) { snapshot.Aspect = 0 }},
		{"non-finite near plane", func(snapshot *CameraSnapshot) { snapshot.Near = float32(math.Inf(1)) }},
		{"zero near plane", func(snapshot *CameraSnapshot) { snapshot.Near = 0 }},
		{"non-finite far plane", func(snapshot *CameraSnapshot) { snapshot.Far = float32(math.Inf(1)) }},
		{"far plane not beyond near", func(snapshot *CameraSnapshot) { snapshot.Far = snapshot.Near }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("CameraSnapshot.Validate() = nil for %+v", candidate)
			}
		})
	}
}

func TestEnvironmentSnapshotDomainAndZero(t *testing.T) {
	var zero EnvironmentSnapshot
	if zero.Ready || zero.Validate() != nil {
		t.Fatalf("zero environment = %+v, want valid not-ready value", zero)
	}

	valid := EnvironmentSnapshot{
		Ready:          true,
		ServerTick:     17,
		WorldTimeTicks: 48_000,
		DayPhaseOffset: 23_999,
		Weather:        core.WeatherThunder,
		Season:         core.SeasonWinter,
		SeasonProgress: 255,
		Temperature:    -40,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid environment: %v", err)
	}
	maximumTemperature := valid
	maximumTemperature.Temperature = int8(core.TemperatureMax)
	if err := maximumTemperature.Validate(); err != nil {
		t.Fatalf("maximum environment temperature: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*EnvironmentSnapshot)
	}{
		{"not-ready payload", func(snapshot *EnvironmentSnapshot) { snapshot.Ready = false }},
		{"day offset at cycle length", func(snapshot *EnvironmentSnapshot) { snapshot.DayPhaseOffset = uint16(core.DayLengthTicks) }},
		{"unknown weather", func(snapshot *EnvironmentSnapshot) { snapshot.Weather = core.WeatherThunder + 1 }},
		{"unknown season", func(snapshot *EnvironmentSnapshot) { snapshot.Season = core.SeasonWinter + 1 }},
		{"temperature below minimum", func(snapshot *EnvironmentSnapshot) { snapshot.Temperature = int8(core.TemperatureMin) - 1 }},
		{"temperature above maximum", func(snapshot *EnvironmentSnapshot) { snapshot.Temperature = int8(core.TemperatureMax) + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("EnvironmentSnapshot.Validate() = nil for %+v", candidate)
			}
		})
	}
}

func TestHUDSnapshotDomainAndZero(t *testing.T) {
	var zero HUDSnapshot
	if zero.Ready || zero.Validate() != nil {
		t.Fatalf("zero HUD = %+v, want valid not-ready value", zero)
	}

	valid := HUDSnapshot{
		Ready:  true,
		Health: core.MaxHealth,
		Hunger: core.MaxHunger,
		Oxygen: core.MaxOxygenTicks,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid HUD: %v", err)
	}
	if err := (HUDSnapshot{Ready: true}).Validate(); err != nil {
		t.Fatalf("zero confirmed HUD values: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*HUDSnapshot)
	}{
		{"not-ready payload", func(snapshot *HUDSnapshot) { snapshot.Ready = false }},
		{"health above maximum", func(snapshot *HUDSnapshot) { snapshot.Health = core.MaxHealth + 1 }},
		{"hunger above maximum", func(snapshot *HUDSnapshot) { snapshot.Hunger = core.MaxHunger + 1 }},
		{"oxygen above maximum", func(snapshot *HUDSnapshot) { snapshot.Oxygen = core.MaxOxygenTicks + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("HUDSnapshot.Validate() = nil for %+v", candidate)
			}
		})
	}
}

func TestTargetSnapshotDomainAndZero(t *testing.T) {
	var zero TargetSnapshot
	if zero.Visible || zero.Validate() != nil {
		t.Fatalf("zero target = %+v, want valid hidden value", zero)
	}

	valid := TargetSnapshot{
		Visible:  true,
		Position: core.BlockPos{X: -9, Y: core.MinY, Z: 12},
		Name:     "砖块",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid target: %v", err)
	}
	ceiling := valid
	ceiling.Position.Y = core.MaxY - 1
	if err := ceiling.Validate(); err != nil {
		t.Fatalf("target at maximum world block: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*TargetSnapshot)
	}{
		{"hidden payload", func(snapshot *TargetSnapshot) { snapshot.Visible = false }},
		{"below world", func(snapshot *TargetSnapshot) { snapshot.Position.Y = core.MinY - 1 }},
		{"at world ceiling", func(snapshot *TargetSnapshot) { snapshot.Position.Y = core.MaxY }},
		{"empty name", func(snapshot *TargetSnapshot) { snapshot.Name = "" }},
		{"invalid UTF-8 name", func(snapshot *TargetSnapshot) { snapshot.Name = string([]byte{0xff}) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("TargetSnapshot.Validate() = nil for %+v", candidate)
			}
		})
	}
}

func TestErrorStateDomainAndZero(t *testing.T) {
	var zero ErrorState
	if zero.Present() || zero.Validate() != nil || zero.Text() != "" {
		t.Fatalf("zero error state = %+v, want no stable error", zero)
	}

	for _, code := range []ErrorCode{
		ErrorConnection,
		ErrorLogin,
		ErrorProtocol,
		ErrorOverflow,
		ErrorInternal,
	} {
		state := ErrorState{Code: code}
		if !state.Present() || state.Validate() != nil || state.Text() == "" {
			t.Fatalf("error state %+v is not a valid present error", state)
		}
	}
	invalid := ErrorState{Code: ErrorCode(255)}
	if !invalid.Present() || invalid.Validate() == nil || invalid.Text() != "" {
		t.Fatalf("invalid error state = %+v, want rejected code without text", invalid)
	}
}

func TestSnapshotsContainNoMutableReferences(t *testing.T) {
	for _, value := range []any{
		SessionPhase(0),
		CameraSnapshot{},
		EnvironmentSnapshot{},
		HUDSnapshot{},
		TargetSnapshot{},
		ErrorState{},
	} {
		assertNoMutableReferences(t, reflect.TypeOf(value), reflect.TypeOf(value).String())
	}

	originalCamera := CameraSnapshot{Position: [3]float32{1, 2, 3}}
	copiedCamera := originalCamera
	copiedCamera.Position[0] = 99
	if originalCamera.Position[0] != 1 {
		t.Fatalf("camera copy mutated original: %+v", originalCamera)
	}

	originalTarget := TargetSnapshot{Name: "Stone"}
	copiedTarget := originalTarget
	copiedTarget.Name = "Brick"
	if originalTarget.Name != "Stone" {
		t.Fatalf("target copy mutated original: %+v", originalTarget)
	}
}

func assertNoMutableReferences(t *testing.T, valueType reflect.Type, path string) {
	t.Helper()
	switch valueType.Kind() {
	case reflect.Array:
		assertNoMutableReferences(t, valueType.Elem(), path+"[]")
	case reflect.Struct:
		for index := 0; index < valueType.NumField(); index++ {
			field := valueType.Field(index)
			assertNoMutableReferences(t, field.Type, path+"."+field.Name)
		}
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer,
		reflect.Slice, reflect.UnsafePointer:
		t.Fatalf("%s has mutable reference kind %s", path, valueType.Kind())
	}
}
