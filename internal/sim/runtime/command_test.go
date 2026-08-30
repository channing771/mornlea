package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func TestLookDirection(t *testing.T) {
	if got, want := LookDirection(0, 0), (mgl32.Vec3{0, 0, -1}); got != want {
		t.Fatalf("look direction = %v, want %v", got, want)
	}
}
