package hostile

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func FuzzDecodeHostileMobs(f *testing.F) {
	// golden 的全前缀截断种子：驱动头部与记录边界的每个截断位形。
	if fixture, err := os.ReadFile(filepath.Join("testdata", "hostile-mobs-v1.bin")); err == nil {
		for length := range len(fixture) + 1 {
			f.Add(bytes.Clone(fixture[:length]))
		}
	}
	// 64 条满容量记录：驱动最大合法文件位形。
	records := make([]StoredHostileMob, MaxHostileMobs)
	for index := range records {
		records[index] = StoredHostileMob{ID: uint64(index) + 1, Dimension: core.Overworld, Health: 1}
	}
	maximum, err := Encode(HostileMobsSave{Revision: 1, Records: records})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(maximum)
	// 单条满字段记录：驱动目标/冷却/distant 字段的深路径。
	full, err := Encode(HostileMobsSave{
		Revision: 5, Records: fixtureHostileRecords()[:1],
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(full)
	// CRC 修复后的非法字段种子：位翻转必须命中字段校验而不是止步于校验和。
	base := hostileRecordOffset(0)
	nonFinite := bytes.Clone(full)
	binary.LittleEndian.PutUint32(
		nonFinite[base+hostileWirePosition:], math.Float32bits(float32(math.NaN())),
	)
	repairHostileCRC(nonFinite)
	f.Add(nonFinite)
	invalidBool := bytes.Clone(full)
	invalidBool[base+hostileWireOnGround] = 2
	repairHostileCRC(invalidBool)
	f.Add(invalidBool)
	oversizedDistant := bytes.Clone(full)
	binary.LittleEndian.PutUint16(oversizedDistant[base+hostileWireDistant:], maxHostileDistantTicks+1)
	repairHostileCRC(oversizedDistant)
	f.Add(oversizedDistant)
	// 头声明 count=65 但文件只有 32 字节：驱动分配前 count 门禁。
	oversizedCount := make([]byte, 32)
	copy(oversizedCount, "MHST")
	binary.LittleEndian.PutUint32(oversizedCount[4:], 1)
	binary.LittleEndian.PutUint32(oversizedCount[8:], 1)
	binary.LittleEndian.PutUint64(oversizedCount[12:], 1)
	binary.LittleEndian.PutUint32(oversizedCount[20:], MaxHostileMobs+1)
	binary.LittleEndian.PutUint32(oversizedCount[24:], (MaxHostileMobs+1)*hostileRecordLength)
	f.Add(oversizedCount)
	f.Fuzz(func(t *testing.T, payload []byte) {
		got, err := Decode(payload)
		if err != nil {
			return
		}
		if got.Revision == 0 || len(got.Records) > MaxHostileMobs {
			t.Fatalf("successful decode escaped bounds: %+v", got)
		}
		for index, record := range got.Records {
			if record.ID == 0 || index > 0 && got.Records[index-1].ID >= record.ID {
				t.Fatalf("successful decode returned unsorted or zero IDs: %+v", got.Records)
			}
			if err := validateHostileRecord(record); err != nil {
				t.Fatalf("successful decode returned invalid record: %v", err)
			}
		}
		// v1 是唯一 schema 且编码无任何自由度（固定布局 + 升序）：解码成功
		// 的输入必须是规范字节形态。
		encoded, err := Encode(HostileMobsSave{
			Revision: got.Revision, Records: got.Records,
		})
		if err != nil || !bytes.Equal(encoded, payload) {
			t.Fatalf("successful decode is not canonical: encode error=%v", err)
		}
	})
}

func TestHostileCodecGoldenRoundTrip(t *testing.T) {
	path := filepath.Join("testdata", "hostile-mobs-v1.bin")
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(golden) != hostileHeaderLength+3*hostileRecordLength {
		t.Fatalf("golden 长度=%d，想要 %d", len(golden), hostileHeaderLength+3*hostileRecordLength)
	}
	if string(golden[0:4]) != "MHST" {
		t.Fatalf("golden magic=%q，想要 MHST", golden[0:4])
	}
	if schema := binary.LittleEndian.Uint32(golden[8:12]); schema != CurrentSchema {
		t.Fatalf("golden schema=%d，想要 %d", schema, CurrentSchema)
	}
	decoded, err := Decode(golden)
	if err != nil {
		t.Fatal(err)
	}
	want := StoredHostileMobs{Revision: 19, Records: fixtureHostileRecordsSorted()}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("golden decode=%+v，想要 %+v", decoded, want)
	}
	// 同一载荷重编码必须逐字节还原 golden：磁盘形态无任何自由度。
	encoded, err := Encode(HostileMobsSave{
		Revision: decoded.Revision, Records: decoded.Records,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, golden) {
		t.Fatal("golden 重编码与原文件不一致：编码存在未钉死的自由度")
	}
}
