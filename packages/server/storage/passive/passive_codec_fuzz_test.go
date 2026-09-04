package passive

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

func FuzzDecodePassiveMobs(f *testing.F) {
	// golden 的全前缀截断种子：驱动头部与记录边界的每个截断位形。
	if fixture, err := os.ReadFile(filepath.Join("testdata", "passive-mobs-v1.bin")); err == nil {
		for length := range len(fixture) + 1 {
			f.Add(bytes.Clone(fixture[:length]))
		}
	}
	// 32 条满容量记录：驱动最大合法文件位形。
	records := make([]StoredPassiveMob, MaxPassiveMobs)
	for index := range records {
		records[index] = StoredPassiveMob{ID: uint64(index) + 1, Dimension: core.Overworld, Health: 1}
	}
	maximum, err := Encode(PassiveMobsSave{Revision: 1, Records: records})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(maximum)
	// 单条满字段记录：驱动位置/速度/朝向字段的深路径。
	full, err := Encode(PassiveMobsSave{
		Revision: 5, Records: fixturePassiveRecords()[:1],
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(full)
	// CRC 修复后的非法字段种子：位翻转必须命中字段校验而不是止步于校验和。
	base := passiveRecordOffset(0)
	nonFinite := bytes.Clone(full)
	binary.LittleEndian.PutUint32(
		nonFinite[base+passiveWirePosition:], math.Float32bits(float32(math.NaN())),
	)
	repairPassiveCRC(nonFinite)
	f.Add(nonFinite)
	invalidBool := bytes.Clone(full)
	invalidBool[base+passiveWireOnGround] = 2
	repairPassiveCRC(invalidBool)
	f.Add(invalidBool)
	nonzeroReserved := bytes.Clone(full)
	nonzeroReserved[base+passiveWireReserved] = 1
	repairPassiveCRC(nonzeroReserved)
	f.Add(nonzeroReserved)
	// 头声明 count=33 但文件只有 32 字节：驱动分配前 count 门禁。
	oversizedCount := make([]byte, 32)
	copy(oversizedCount, "PMST")
	binary.LittleEndian.PutUint32(oversizedCount[4:], 1)
	binary.LittleEndian.PutUint32(oversizedCount[8:], 1)
	binary.LittleEndian.PutUint64(oversizedCount[12:], 1)
	binary.LittleEndian.PutUint32(oversizedCount[20:], MaxPassiveMobs+1)
	binary.LittleEndian.PutUint32(oversizedCount[24:], (MaxPassiveMobs+1)*passiveRecordLength)
	f.Add(oversizedCount)
	f.Fuzz(func(t *testing.T, payload []byte) {
		got, err := Decode(payload)
		if err != nil {
			return
		}
		if got.Revision == 0 || len(got.Records) > MaxPassiveMobs {
			t.Fatalf("successful decode escaped bounds: %+v", got)
		}
		for index, record := range got.Records {
			if record.ID == 0 || index > 0 && got.Records[index-1].ID >= record.ID {
				t.Fatalf("successful decode returned unsorted or zero IDs: %+v", got.Records)
			}
			if err := validatePassiveRecord(record); err != nil {
				t.Fatalf("successful decode returned invalid record: %v", err)
			}
		}
		// v1 是唯一 schema 且编码无任何自由度（固定布局 + 升序 + 零保留段）：
		// 解码成功的输入必须是规范字节形态。
		encoded, err := Encode(PassiveMobsSave{
			Revision: got.Revision, Records: got.Records,
		})
		if err != nil || !bytes.Equal(encoded, payload) {
			t.Fatalf("successful decode is not canonical: encode error=%v", err)
		}
	})
}

func TestPassiveCodecGoldenRoundTrip(t *testing.T) {
	path := filepath.Join("testdata", "passive-mobs-v1.bin")
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(golden) != passiveHeaderLength+3*passiveRecordLength {
		t.Fatalf("golden 长度=%d，想要 %d", len(golden), passiveHeaderLength+3*passiveRecordLength)
	}
	if string(golden[0:4]) != "PMST" {
		t.Fatalf("golden magic=%q，想要 PMST", golden[0:4])
	}
	if schema := binary.LittleEndian.Uint32(golden[8:12]); schema != CurrentSchema {
		t.Fatalf("golden schema=%d，想要 %d", schema, CurrentSchema)
	}
	decoded, err := Decode(golden)
	if err != nil {
		t.Fatal(err)
	}
	want := StoredPassiveMobs{Revision: 11, Records: fixturePassiveRecordsSorted()}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("golden decode=%+v，想要 %+v", decoded, want)
	}
	// 同一载荷重编码必须逐字节还原 golden：磁盘形态无任何自由度。
	encoded, err := Encode(PassiveMobsSave{
		Revision: decoded.Revision, Records: decoded.Records,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, golden) {
		t.Fatal("golden 重编码与原文件不一致：编码存在未钉死的自由度")
	}
}
