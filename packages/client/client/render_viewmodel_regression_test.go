//go:build darwin

package client

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// 本文件锁定任务 1.2 的帧级回归：无 viewmodel 输入的帧，其 `EncodeRenderFrame`
// 输出 MUST 与本 change 前逐字节一致。以下 golden 是 viewmodel 接线前
// （client ABI v16）的帧字节快照；后续 change 新增 viewmodel TLV 段时，空段
// 不得写入任何字节（沿空负载跳过的既有纪律），三组 golden 必须全绿。

func viewmodelRegressionFill(n int, seed byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte((int(seed) + i*31) % 251)
	}
	return out
}

func viewmodelRegressionFullFrame() RenderFrame {
	frame := RenderFrame{
		Daylight:       0.5,
		StarVisibility: 0.25,
		CloudMacroX:    7,
		CloudLocal:     0.125,
		Pos:            [3]float32{1.5, 64, -2.5},
		SunDirection:   [3]float32{0.3, 0.8, 0.1},
		SkyColor:       [4]float32{0.2, 0.4, 1, 1},
		Visible:        [][3]int32{{1, 2, 3}, {-1, 0, -3}},
	}
	for i := 0; i < 4; i++ {
		frame.ViewProj[i*4+i] = 1
		frame.ViewProjInv[i*4+i] = 1
	}
	frame.AvatarInstances = viewmodelRegressionFill(96, 11)
	frame.DropInstances = viewmodelRegressionFill(96, 23)
	frame.OutlineInstances = viewmodelRegressionFill(192, 37)
	frame.CrackInstances = viewmodelRegressionFill(80, 53)
	frame.OverlayStrength = 0.5
	frame.WaterTint = [4]float32{0.12, 0.34, 0.52, 0.45}
	frame.NameTagSegment = EncodeQuadSegment(viewmodelRegressionFill(16, 67), viewmodelRegressionFill(96, 71), viewmodelRegressionFill(48, 73), 48)
	frame.HUDSegment = EncodeQuadSegment(viewmodelRegressionFill(8, 79), viewmodelRegressionFill(48, 83), nil, 48)
	return frame
}

func TestEncodeRenderFrameWithoutViewmodelMatchesBaseline(t *testing.T) {
	avatar := RenderFrame{
		Daylight:        0.5,
		AvatarInstances: viewmodelRegressionFill(96, 11),
	}
	frames := map[string]RenderFrame{
		"empty":  {},
		"avatar": avatar,
		"full":   viewmodelRegressionFullFrame(),
	}
	goldens := map[string]struct {
		hex    string
		length int
	}{

		"empty": {
			hex: "0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000000000000",
			length: 192,
		},
		"avatar": {
			hex: "0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000000000000" +
				"0000000000000000000000000000003f00000000000000000000000000000000" +
				"0000000000000000000000000000000000000000000000000000000002000000" +
				"01000000600000000b2a496887a6c5e40827466584a3c2e10524436281a0bfde" +
				"0221405f7e9dbcdbfa1e3d5c7b9ab9d8f71b3a597897b6d5f41837567594b3d2" +
				"f11534537291b0cfee1231506f8eadcceb0f2e4d6c8baac9e80c2b4a6988a7c6" +
				"e50928476685a4c3",
			length: 296,
		},
		"full": {
			hex: "0000803f000000000000000000000000000000000000803f0000000000000000" +
				"00000000000000000000803f000000000000000000000000000000000000803f" +
				"0000803f000000000000000000000000000000000000803f0000000000000000" +
				"00000000000000000000803f000000000000000000000000000000000000803f" +
				"0000c03f00008042000020c00000003f9a99993ecdcc4c3fcdcccc3d0000803e" +
				"cdcc4c3ecdcccc3e0000803f0000803f070000000000003e0200000002000000" +
				"010000000200000003000000ffffffff00000000fdffffff0100000060000000" +
				"0b2a496887a6c5e40827466584a3c2e10524436281a0bfde0221405f7e9dbcdb" +
				"fa1e3d5c7b9ab9d8f71b3a597897b6d5f41837567594b3d2f11534537291b0cf" +
				"ee1231506f8eadcceb0f2e4d6c8baac9e80c2b4a6988a7c6e50928476685a4c3" +
				"02000000600000001736557493b2d1f01433527190afceed11304f6e8daccbea" +
				"0e2d4c6b8aa9c8e70b2a496887a6c5e40827466584a3c2e10524436281a0bfde" +
				"0221405f7e9dbcdbfa1e3d5c7b9ab9d8f71b3a597897b6d5f41837567594b3d2" +
				"f11534537291b0cf03000000c000000025446382a1c0df032241607f9ebddc00" +
				"1f3e5d7c9bbad9f81c3b5a7998b7d6f51938577695b4d3f21635547392b1d0ef" +
				"133251708faecdec102f4e6d8cabcae90d2c4b6a89a8c7e60a29486786a5c4e3" +
				"0726456483a2c1e004234261809fbedd01203f5e7d9cbbdaf91d3c5b7a99b8d7" +
				"f61a39587796b5d4f31736557493b2d1f01433527190afceed11304f6e8daccb" +
				"ea0e2d4c6b8aa9c8e70b2a496887a6c5e40827466584a3c2e10524436281a0bf" +
				"de0221405f7e9dbcdbfa1e3d5c7b9ab904000000040000000000003f05000000" +
				"a8000000436281a0bfde0221405f7e9dbcdbfa1e0200000001000000476685a4" +
				"c3e20625446382a1c0df032241607f9ebddc001f3e5d7c9bbad9f81c3b5a7998" +
				"b7d6f51938577695b4d3f21635547392b1d0ef133251708faecdec102f4e6d8c" +
				"abcae90d2c4b6a89a8c7e60a29486786a5c4e30726456483a2c1e004496887a6" +
				"c5e40827466584a3c2e10524436281a0bfde0221405f7e9dbcdbfa1e3d5c7b9a" +
				"b9d8f71b3a597897b6d5f41806000000400000004f6e8daccbea0e2d01000000" +
				"00000000537291b0cfee1231506f8eadcceb0f2e4d6c8baac9e80c2b4a6988a7" +
				"c6e50928476685a4c3e20625446382a1c0df032208000000100000008fc2f53d" +
				"7b14ae3eb81e053f6666e63e0a0000005000000035547392b1d0ef133251708f" +
				"aecdec102f4e6d8cabcae90d2c4b6a89a8c7e60a29486786a5c4e30726456483" +
				"a2c1e004234261809fbedd01203f5e7d9cbbdaf91d3c5b7a99b8d7f61a395877" +
				"96b5d4f3",
			length: 996,
		},
	}
	for name, frame := range frames {
		raw := strings.ReplaceAll(goldens[name].hex, " ", "")
		raw = strings.ReplaceAll(raw, "\n", "")
		want, err := hex.DecodeString(raw)
		if err != nil {
			t.Fatalf("%s golden 解码失败：%v", name, err)
		}
		if len(want) != goldens[name].length {
			t.Fatalf("%s golden 长度 = %d，想要 %d", name, len(want), goldens[name].length)
		}
		if got := EncodeRenderFrame(frame); !bytes.Equal(got, want) {
			t.Fatalf("%s 帧字节与基线不一致（长度 %d vs %d），无 viewmodel 输入的帧 MUST 逐字节一致", name, len(got), len(want))
		}
	}
}
