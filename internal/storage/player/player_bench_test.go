package player

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

var (
	benchmarkEncodedPlayer []byte
	benchmarkStoredPlayer  StoredPlayer
)

// BenchmarkPlayerCodec 只测本包编解码入口。以 DiskStore/MemoryStore 为夹具的
// player 存取基准（BenchmarkMemoryPlayerStore/BenchmarkDiskPlayerStore）的被测
// 主体是根包编排，留在根包 player_store_test.go：player 域没有可域内装配的
// 记录层，域内重装配只会改变被测主体。
func BenchmarkPlayerCodec(b *testing.B) {
	save := benchmarkPlayerSave(1)
	encoded, err := Encode(save)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			payload, err := Encode(save)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkEncodedPlayer = payload
		}
	})
	b.Run("Decode", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			player, err := Decode(save.PlayerID, encoded)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkStoredPlayer = player
		}
	})
}

func benchmarkPlayerSave(revision uint64) PlayerSave {
	safe := PlayerLocation{Dimension: core.Overworld, Position: [3]float32{4.5, 65, -6.5}}
	return PlayerSave{
		PlayerID: core.PlayerID{0x6f, 0xce, 0x82, 0x77, 0xa9, 0x33, 0x46, 0xcb, 0x9a, 0x1f, 0xda, 0x13, 0xb7, 0xee, 0x56, 0x44},
		Revision: revision, DisplayName: "Benchmark",
		Current: PlayerLocation{Dimension: core.Overworld, Position: [3]float32{8.5, 70, -9.5}},
		Yaw:     0.75, Pitch: -0.25, Safe: &safe,
	}
}
