package assets

import (
	"bytes"
	"testing"
)

func TestOriginalHumanFaceLayers(t *testing.T) {
	r := NewRegistry()
	if r.LayerCount() != 160 {
		t.Fatalf("人物分面层缺失: %d", r.LayerCount())
	}
	for _, base := range []int{112, 136} {
		front, back := r.LayerRGBA(base+5), r.LayerRGBA(base+4)
		if bytes.Equal(front, back) {
			t.Fatal("面部复制到后脑")
		}
		colors := map[[4]byte]bool{}
		for i := 0; i < len(front); i += 4 {
			colors[[4]byte(front[i:i+4])] = true
		}
		if len(colors) < 6 {
			t.Fatal("面部缺乏肤色/额发/眼眉鼻细节")
		}
		for part := 0; part < 4; part++ {
			for face := 0; face < 6; face++ {
				pixels := r.LayerRGBA(base + part*6 + face)
				if len(pixels) != 16*16*4 {
					t.Fatal("人物层大小错误")
				}
				for i := 3; i < len(pixels); i += 4 {
					if pixels[i] != 255 {
						t.Fatal("人物层不是不透明")
					}
				}
			}
		}
	}
}
