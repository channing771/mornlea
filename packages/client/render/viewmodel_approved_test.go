package render

import (
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestViewmodelApprovedForearmAndFist(t *testing.T) {
	input := &ViewmodelInput{ViewportWidth: 1280, ViewportHeight: 720}
	parts := buildViewmodelParts(nil, input, 0)
	if len(parts) != 8 {
		t.Fatalf("hand parts=%d, want sleeve cuff palm thumb and four fingers", len(parts))
	}
	out := make([]byte, len(parts)*96)
	encodeAvatarPartsInto(out, parts)
	size := decodedPartSize(out, 0)
	if size[1] > .8 {
		t.Fatalf("long sleeve %v", size)
	}
	degrees := viewmodelSlantDegreesOf(out, 0)
	if degrees < 30 || degrees > 55 {
		t.Fatalf("forearm angle=%v, want diagonal", degrees)
	}
	projection := core.Perspective(viewmodelProjectionFovY, 1280.0/720, .1, 100)
	p, _ := projectToNDC(projection, decodedPartCenter(out, 2))
	x, y := (p[0]+1)/2, (1-p[1])/2
	if x < .85 || x > .89 || y < .75 || y > .83 {
		t.Fatalf("fist center=%v,%v", x, y)
	}
	if parts[2].color == parts[0].color || parts[3].color == parts[0].color {
		t.Fatal("fist/thumb still sleeve colored")
	}
	inv := parts[2].transform.Inv()
	thumb := inv.Mul4x1(parts[3].transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}))
	if abs32(thumb[0]) < .35 {
		t.Fatal("thumb silhouette absent")
	}
}

func TestViewmodelResizeDoesNotJumpAtLayoutBoundary(t *testing.T) {
	a := viewmodelGripRoot(&ViewmodelInput{ViewportWidth: 999, ViewportHeight: 720}, 0)
	b := viewmodelGripRoot(&ViewmodelInput{ViewportWidth: 1000, ViewportHeight: 720}, 0)
	if a.Col(3).Sub(b.Col(3)).Len() > .01 {
		t.Fatal("one-pixel resize visibly jumps hand")
	}
}
