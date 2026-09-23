package assets

// atlasMips is the number of material atlas mip levels and must match the Rust renderer's ATLAS_MIPS.
const atlasMips = 5

// layerMipChain generates one complete layer-major mip chain, including mip zero.
func (r *Registry) layerMipChain(layer int) [][]byte {
	chain := make([][]byte, 0, atlasMips)
	px := r.LayerRGBA(layer)
	size := texSize
	chain = append(chain, px)
	for mip := 1; mip < atlasMips; mip++ {
		// 小麦与树叶、玻璃同属 cutout 类：普通盒式降采样会把细麦秆的 alpha
		// 平均到 0.5 以下，远处整片作物被 `c.a < 0.5` 的 discard 抹掉。
		if isCutoutLayer(layer) {
			px = downsampleCutout(px, size)
		} else {
			px = downsample(px, size)
		}
		size /= 2
		chain = append(chain, px)
	}
	return chain
}

// AtlasPixels returns layer-major, mip-major RGBA bytes shared by native renderer upload and deterministic asset generation.
func (r *Registry) AtlasPixels() (int, []byte) {
	var out []byte
	for layer := 0; layer < r.LayerCount(); layer++ {
		for _, px := range r.layerMipChain(layer) {
			out = append(out, px...)
		}
	}
	return r.LayerCount(), out
}

func downsampleCutout(src []byte, size int) []byte {
	dst := downsample(src, size)
	half := size / 2
	for y := 0; y < half; y++ {
		for x := 0; x < half; x++ {
			a := byte(0)
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					a = max(a, src[((y*2+dy)*size+x*2+dx)*4+3])
				}
			}
			dst[(y*half+x)*4+3] = a
		}
	}
	return dst
}

func downsample(src []byte, size int) []byte {
	half := size / 2
	dst := make([]byte, half*half*4)
	for y := 0; y < half; y++ {
		for x := 0; x < half; x++ {
			for c := 0; c < 4; c++ {
				sum := int(src[((y*2)*size+x*2)*4+c]) +
					int(src[((y*2)*size+x*2+1)*4+c]) +
					int(src[((y*2+1)*size+x*2)*4+c]) +
					int(src[((y*2+1)*size+x*2+1)*4+c])
				dst[(y*half+x)*4+c] = byte(sum / 4)
			}
		}
	}
	return dst
}
