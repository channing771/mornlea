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
	// 两代 golden 的全前缀截断种子：驱动头部与记录边界的每个截断位形，
	// v2 种子额外覆盖 kind 尾字节的截断与越界位形。
	for _, name := range []string{"hostile-mobs-v1.bin", "hostile-mobs-v2.bin"} {
		if fixture, err := os.ReadFile(filepath.Join("testdata", name)); err == nil {
			for length := range len(fixture) + 1 {
				f.Add(bytes.Clone(fixture[:length]))
			}
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
	// CRC 修复后的非法 kind 种子：单条记录文件以 kind 字节收尾，值域 {0,1}
	// 越界必须被字段校验拒绝而不是止步于校验和。
	illegalKind := bytes.Clone(full)
	illegalKind[len(illegalKind)-1] = 2
	repairHostileCRC(illegalKind)
	f.Add(illegalKind)
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
		// 当前 schema 编码无任何自由度（固定布局 + 升序）：解码成功的 v2 输入
		// 必须重编码逐字节还原。v1 输入经只读迁移读入后重编码会升级为 v2
		// 字节形态，逐字节相等不再成立，改为断言升级路径确定性：重编码再
		// 解码必须恢复同一集合（含恒 0 的 kind）。
		schema := binary.LittleEndian.Uint32(payload[8:12])
		encoded, err := Encode(HostileMobsSave{
			Revision: got.Revision, Records: got.Records,
		})
		if err != nil {
			t.Fatalf("successful decode failed to re-encode: %v", err)
		}
		if schema == CurrentSchema {
			if !bytes.Equal(encoded, payload) {
				t.Fatal("successful decode is not canonical")
			}
			return
		}
		again, err := Decode(encoded)
		if err != nil || !reflect.DeepEqual(again, got) {
			t.Fatalf("migrated decode does not survive the current-schema rewrite: %v", err)
		}
	})
}

// TestHostileCodecGoldenRoundTrip 冻结当前 schema 的编码结果，防止字节布局
// 无声漂移。夹具取三条 kind 覆盖 {0,1} 两端的记录，让 73-byte 记录的 kind
// 尾字节承重。
//
// 冻结的 v1 golden（testdata/hostile-mobs-v1.bin）刻意保留在原处不再生成：
// 它是"旧存档仍然可读"的唯一真实证据，见 hostile_migration_test.go 的
// TestHostileCodecV1FixtureMigratesToKindZero。
func TestHostileCodecGoldenRoundTrip(t *testing.T) {
	path := filepath.Join("testdata", "hostile-mobs-v2.bin")
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(golden) != hostileHeaderLength+3*hostileRecordLengthV2 {
		t.Fatalf("golden 长度=%d，想要 %d", len(golden), hostileHeaderLength+3*hostileRecordLengthV2)
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
