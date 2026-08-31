// companions.ai 摘要测试同时锁定冻结 v4 fixture 的只读兼容与 v5 memory
// mirror 的严格布局。当前编码器只产出 v5；历史摘要只在 migration 时从
// queue 搬入 lifecycle，此后 queue 不再承载摘要。
package companion

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	domain "github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/storage/storagedef"
)

// updateStorageFixtures 只允许显式重写当前 companion golden；普通测试永不
// 修改仓库 fixture。
var updateStorageFixtures = flag.Bool(
	"update-storage-fixtures", false, "rewrite committed storage fixtures",
)

// fixtureCompanionSummary 是冻结 v4 golden 中的最近对话摘要。
const fixtureCompanionSummary = "阿木记得玩家常在橡树旁停留，上次一起修好了北边的小路。"

// fixtureCompanionV4Queues 描述冻结 v4 fixture 的 legacy queue 载荷。
func fixtureCompanionV4Queues() []StoredCompanionQueue {
	queues := cloneStoredQueuesForTest(fixtureCompanionV3Queues())
	queues[0].Summary = fixtureCompanionSummary
	queues = append(queues, StoredCompanionQueue{
		ID:      fixtureCompanionID(2),
		Pending: []string{"v4仅排队甲", "v4仅排队乙"},
	})
	return queues
}

func deterministicIdentityGenerator(start byte) (IdentityGenerator, *int) {
	calls := 0
	return func() (Identity, error) {
		identity := fixtureAgentIdentity(start + byte(calls))
		calls++
		return identity, nil
	}, &calls
}

func TestCompanionCodecV4GoldenReadOnlyAndV5Rewrite(t *testing.T) {
	if CurrentSchema != 5 || MaxFileLength != 393904 {
		t.Fatalf("current schema/max=%d/%d，想要 5/393904", CurrentSchema, MaxFileLength)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "companions-v4.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if schema := binary.LittleEndian.Uint32(golden[8:12]); schema != 4 {
		t.Fatalf("v4 golden schema=%d，想要 4", schema)
	}
	decoded, err := Decode(golden)
	if err != nil {
		t.Fatal(err)
	}
	wantRecords := []domain.Body{fixtureCompanionBodies()[1], fixtureCompanionBodies()[0]}
	wantQueues := fixtureCompanionV4Queues()
	if decoded.SourceSchema != 4 || decoded.Revision != 47 ||
		!reflect.DeepEqual(decoded.Records, wantRecords) ||
		!reflect.DeepEqual(decoded.Queues, wantQueues) {
		t.Fatalf("v4 golden decode=%+v", decoded)
	}

	generate, calls := deterministicIdentityGenerator(0x80)
	migrated, changed, err := MergeV5(decoded, decoded.Records, generate)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || *calls != 2 || migrated.SourceSchema != 5 || migrated.Revision != 48 {
		t.Fatalf("v4 migration changed/calls/schema/revision=%v/%d/%d/%d", changed, *calls, migrated.SourceSchema, migrated.Revision)
	}
	if migrated.AgentNamespaceID != fixtureAgentIdentity(0x80) ||
		migrated.Lifecycles[0].MemoryRevision != 1 ||
		migrated.Lifecycles[0].MemoryOperationID != fixtureAgentIdentity(0x81) ||
		migrated.Lifecycles[0].Summary != fixtureCompanionSummary ||
		migrated.Lifecycles[1].MemoryRevision != 0 {
		t.Fatalf("v4 migration metadata=%+v", migrated)
	}
	for _, queue := range migrated.Queues {
		if queue.Summary != "" {
			t.Fatalf("v5 queue 仍携带 legacy summary=%q", queue.Summary)
		}
	}

	encoded, err := Encode(CompanionSave{
		Revision:         migrated.Revision,
		AgentNamespaceID: migrated.AgentNamespaceID,
		Records:          migrated.Records,
		Lifecycles:       migrated.Lifecycles,
		Queues:           migrated.Queues,
	})
	if err != nil {
		t.Fatal(err)
	}
	if schema := binary.LittleEndian.Uint32(encoded[8:12]); schema != 5 {
		t.Fatalf("migration rewrite schema=%d，想要 5", schema)
	}
	reloaded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded, migrated) {
		t.Fatalf("v5 rewrite=%+v，想要 %+v", reloaded, migrated)
	}
	clear(golden)
	if migrated.Lifecycles[0].Summary != fixtureCompanionSummary {
		t.Fatal("decode/migration 结果引用了输入 bytes")
	}
}

func TestCompanionCodecV5SummaryBoundariesAndSeparation(t *testing.T) {
	maxSummary := strings.Repeat("木", 682) + "ab"
	if len(maxSummary) != MaxCompanionSummaryBytes {
		t.Fatalf("边界摘要长度=%d，想要 %d", len(maxSummary), MaxCompanionSummaryBytes)
	}
	body := fixtureCompanionBodies()[0]
	lifecycle := fixtureV5Lifecycle(body.ID, true, 3)
	lifecycle.MemoryRevision = 9
	lifecycle.MemoryOperationID = fixtureAgentIdentity(0x91)
	lifecycle.Summary = maxSummary
	save := CompanionSave{
		Revision:         11,
		AgentNamespaceID: fixtureAgentIdentity(0x90),
		Records:          []domain.Body{body},
		Lifecycles:       []StoredCompanionLifecycle{lifecycle},
		Queues: []StoredCompanionQueue{{
			ID: body.ID, Pending: []string{"排队"},
		}},
	}
	encoded, err := Encode(save)
	if err != nil {
		t.Fatalf("2,048-byte summary encode: %v", err)
	}
	got, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lifecycles[0].Summary != maxSummary || got.Queues[0].Summary != "" {
		t.Fatalf("summary lifecycle/queue=%d/%q", len(got.Lifecycles[0].Summary), got.Queues[0].Summary)
	}

	for name, summary := range map[string]string{
		"over limit":   maxSummary + "c",
		"contains NUL": "记忆\x00注入",
		"invalid UTF8": "记忆\xff碎片",
	} {
		t.Run(name, func(t *testing.T) {
			bad := save
			bad.Lifecycles = append([]StoredCompanionLifecycle(nil), save.Lifecycles...)
			bad.Lifecycles[0].Summary = summary
			if _, err := Encode(bad); !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("encode error=%v，想要 ErrCorrupt", err)
			}
		})
	}

	legacyQueue := save
	legacyQueue.Queues = cloneStoredQueuesForTest(save.Queues)
	legacyQueue.Queues[0].Summary = "不得写入 v5 queue"
	if _, err := Encode(legacyQueue); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("legacy queue summary encode error=%v，想要 ErrCorrupt", err)
	}

	canonicalZero := save
	canonicalZero.Lifecycles = append([]StoredCompanionLifecycle(nil), save.Lifecycles...)
	canonicalZero.Lifecycles[0].MemoryRevision = 0
	canonicalZero.Lifecycles[0].MemoryOperationID = Identity{}
	canonicalZero.Lifecycles[0].Summary = ""
	if _, err := Encode(canonicalZero); err != nil {
		t.Fatalf("canonical-zero mirror encode: %v", err)
	}
}

func TestCompanionCodecV5RejectsCorruptSummaryBytes(t *testing.T) {
	body := fixtureCompanionBodies()[0]
	lifecycle := fixtureV5Lifecycle(body.ID, true, 1)
	lifecycle.MemoryRevision = 1
	lifecycle.MemoryOperationID = fixtureAgentIdentity(0xa1)
	lifecycle.Summary = "summarytext"
	valid, err := Encode(CompanionSave{
		Revision:         21,
		AgentNamespaceID: fixtureAgentIdentity(0xa0),
		Records:          []domain.Body{body},
		Lifecycles:       []StoredCompanionLifecycle{lifecycle},
	})
	if err != nil {
		t.Fatal(err)
	}
	const summaryPrefixOffset = companionHeaderLength + 16 + companionRecordLength + 1 + 8 + 8 + 16
	tests := []struct {
		name   string
		mutate func([]byte)
		want   error
	}{
		{"summary length exceeds limit", func(payload []byte) {
			binary.LittleEndian.PutUint16(payload[summaryPrefixOffset:], MaxCompanionSummaryBytes+1)
		}, storagedef.ErrCorrupt},
		{"summary length shrunk misaligns", func(payload []byte) {
			binary.LittleEndian.PutUint16(payload[summaryPrefixOffset:], uint16(len(lifecycle.Summary)-1))
		}, storagedef.ErrCorrupt},
		{"summary length grown truncates", func(payload []byte) {
			binary.LittleEndian.PutUint16(payload[summaryPrefixOffset:], uint16(len(lifecycle.Summary)+1))
		}, storagedef.ErrCorrupt},
		{"summary contains NUL", func(payload []byte) { payload[summaryPrefixOffset+2] = 0 }, storagedef.ErrCorrupt},
		{"summary invalid UTF-8", func(payload []byte) { payload[summaryPrefixOffset+2] = 0xff }, storagedef.ErrCorrupt},
		{"future schema v6", func(payload []byte) {
			binary.LittleEndian.PutUint32(payload[8:], CurrentSchema+1)
		}, storagedef.ErrFutureVersion},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := bytes.Clone(valid)
			tc.mutate(payload)
			if tc.name != "future schema v6" {
				repairCompanionCRC(payload)
			}
			if _, err := Decode(payload); !errors.Is(err, tc.want) {
				t.Fatalf("decode error=%v，想要 %v", err, tc.want)
			}
		})
	}

	v3Golden, err := os.ReadFile(filepath.Join("testdata", "companions-v3.bin"))
	if err != nil {
		t.Fatal(err)
	}
	reserved := bytes.Clone(v3Golden)
	reserved[companionHeaderLength+companionRecordLength] |= companionLegacyFlagHasSummary
	repairCompanionCRC(reserved)
	if _, err := Decode(reserved); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("v3 summary reserved bit error=%v，想要 ErrCorrupt", err)
	}

	oversized := bytes.Repeat([]byte{0x5a}, MaxFileLength+1)
	if _, err := Decode(oversized); !errors.Is(err, storagedef.ErrCorrupt) ||
		!strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("oversized error=%v，想要分配前长度门禁", err)
	}
}

func TestCompanionLegacySummariesMigrateOnlyThroughMergeV5(t *testing.T) {
	for _, fixture := range []struct {
		name string
		file string
	}{
		{"v1", "companions-v1.bin"},
		{"v2", "companions-v2.bin"},
		{"v3", "companions-v3.bin"},
		{"v4", "companions-v4.bin"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			golden, err := os.ReadFile(filepath.Join("testdata", fixture.file))
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := Decode(golden)
			if err != nil {
				t.Fatal(err)
			}
			generate, _ := deterministicIdentityGenerator(0xb0)
			migrated, changed, err := MergeV5(legacy, legacy.Records, generate)
			if err != nil {
				t.Fatal(err)
			}
			if !changed || migrated.SourceSchema != 5 {
				t.Fatalf("changed/schema=%v/%d", changed, migrated.SourceSchema)
			}
			for _, queue := range migrated.Queues {
				if queue.Summary != "" {
					t.Fatalf("v5 queue retained summary=%q", queue.Summary)
				}
			}
			for _, lifecycle := range migrated.Lifecycles {
				if fixture.name != "v4" && (lifecycle.MemoryRevision != 0 || lifecycle.Summary != "") {
					t.Fatalf("%s created nonzero mirror=%+v", fixture.name, lifecycle)
				}
			}
		})
	}
}
