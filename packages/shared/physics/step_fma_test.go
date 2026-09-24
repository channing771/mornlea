package physics

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
)

type emptyLoadedStepSource struct{}

func (emptyLoadedStepSource) CollisionBoxes(core.BlockPos) CollisionBoxSet {
	return CollisionBoxSet{Loaded: true}
}

func TestStepVectorLengthMatchesRustFusedOrder(t *testing.T) {
	tests := []struct {
		name     string
		vector   mgl32.Vec3
		wantBits uint32
	}{
		{
			name:     "hosted displacement witness",
			vector:   mgl32.Vec3{math.Float32frombits(0x3f048e4c), 0, math.Float32frombits(0x4081842c)},
			wantBits: 0x40829268,
		},
		{name: "zero", vector: mgl32.Vec3{}, wantBits: 0x00000000},
		{name: "mixed signs", vector: mgl32.Vec3{-3, 4, -12}, wantBits: 0x41500000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := math.Float32bits(stepVectorLength(test.vector)); got != test.wantBits {
				t.Fatalf("length bits=%08x, want %08x", got, test.wantBits)
			}
		})
	}
}

func TestStepWithTunablesWitnessDelta(t *testing.T) {
	state := State{
		Position: mgl32.Vec3{0.5, 5, 0.5},
		Velocity: mgl32.Vec3{-math.Float32frombits(0x3f048e4c), 0, -math.Float32frombits(0x4081842c)},
		OnGround: true,
	}
	got := StepWithTunables(state, Input{}, emptyLoadedStepSource{}, DefaultTunables()).State
	wantPosition := [3]uint32{0x3efaddb0, 0x409d70a4, 0x3ed7de9b}
	wantVelocity := [3]uint32{0xbe4d5c78, 0xbfcccccd, 0xbfc8a6f8}
	for index := range 3 {
		if bits := math.Float32bits(got.Position[index]); bits != wantPosition[index] {
			t.Errorf("position[%d] bits=%08x, want %08x", index, bits, wantPosition[index])
		}
		if bits := math.Float32bits(got.Velocity[index]); bits != wantVelocity[index] {
			t.Errorf("velocity[%d] bits=%08x, want %08x", index, bits, wantVelocity[index])
		}
	}
}
