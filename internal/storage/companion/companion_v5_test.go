package companion

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/storage/storagedef"
	domain "github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
)

func fixtureAgentIdentity(last byte) Identity {
	return Identity{
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x46, 0x17,
		0x88, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, last,
	}
}

func fixtureV5Lifecycle(id domain.ID, active bool, epoch uint64) StoredCompanionLifecycle {
	lifecycle := StoredCompanionLifecycle{ID: id, Active: active, MemoryEpoch: epoch}
	if !active {
		lifecycle.TombstoneOperationID = fixtureAgentIdentity(id[15] + 0x40)
	}
	return lifecycle
}

func fixtureV5Save(revision uint64, records []domain.Body, queues []StoredCompanionQueue) CompanionSave {
	lifecycles := make([]StoredCompanionLifecycle, len(records))
	for index, body := range records {
		lifecycles[index] = fixtureV5Lifecycle(body.ID, true, 1)
	}
	return CompanionSave{
		Revision:         revision,
		AgentNamespaceID: fixtureAgentIdentity(0x70),
		Records:          records,
		Lifecycles:       lifecycles,
		Queues:           queues,
	}
}

// fixtureV5SaveForTest 为历史 codec 测试补齐当前 schema 的必填 metadata。
// legacy 摘要被显式迁入 lifecycle mirror，避免测试 helper 掩盖生产边界。
func fixtureV5SaveForTest(save CompanionSave) CompanionSave {
	save.Records = append([]domain.Body(nil), save.Records...)
	save.Queues = cloneStoredQueuesForTest(save.Queues)
	if save.AgentNamespaceID == (Identity{}) {
		save.AgentNamespaceID = fixtureAgentIdentity(0x70)
	}
	if save.Lifecycles == nil {
		save.Lifecycles = make([]StoredCompanionLifecycle, len(save.Records))
		for index, body := range save.Records {
			save.Lifecycles[index] = fixtureV5Lifecycle(body.ID, index < domain.MaxActive, 1)
		}
	}
	save.Lifecycles = append([]StoredCompanionLifecycle(nil), save.Lifecycles...)
	lifecycleByID := make(map[domain.ID]int, len(save.Lifecycles))
	for index, lifecycle := range save.Lifecycles {
		lifecycleByID[lifecycle.ID] = index
	}
	for index := range save.Queues {
		if save.Queues[index].Summary == "" {
			continue
		}
		lifecycleIndex := lifecycleByID[save.Queues[index].ID]
		save.Lifecycles[lifecycleIndex].MemoryRevision = 1
		save.Lifecycles[lifecycleIndex].MemoryOperationID = fixtureAgentIdentity(save.Queues[index].ID[15] + 0x20)
		save.Lifecycles[lifecycleIndex].Summary = save.Queues[index].Summary
		save.Queues[index].Summary = ""
	}
	return save
}

func fixtureV5GoldenSave() CompanionSave {
	bodies := fixtureCompanionBodies()
	inactiveBody := bodies[0]
	inactiveBody.ID = fixtureCompanionID(3)
	inactiveBody.Position = [3]float32{24.5, 68, -17.5}
	records := []domain.Body{bodies[1], bodies[0], inactiveBody}
	nonzero := fixtureV5Lifecycle(records[0].ID, true, 7)
	nonzero.MemoryRevision = 11
	nonzero.MemoryOperationID = fixtureAgentIdentity(0x71)
	nonzero.Summary = "阿木记得北边橡树旁的小路。"
	zero := fixtureV5Lifecycle(records[1].ID, true, 3)
	inactive := fixtureV5Lifecycle(records[2].ID, false, 9)
	inactive.TombstoneOperationID = fixtureAgentIdentity(0x73)
	return CompanionSave{
		Revision:         47,
		AgentNamespaceID: fixtureAgentIdentity(0x70),
		Records:          records,
		Lifecycles:       []StoredCompanionLifecycle{nonzero, zero, inactive},
		Queues:           fixtureCompanionV3Queues(),
	}
}

func TestCompanionCodecV5GoldenRoundTrip(t *testing.T) {
	save := fixtureV5GoldenSave()
	encoded, err := Encode(save)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "companions-v5.bin")
	if *updateStorageFixtures {
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if schema := binary.LittleEndian.Uint32(golden[8:12]); schema != companionSchemaV5 {
		t.Fatalf("v5 golden schema=%d，想要 5", schema)
	}
	if !bytes.Equal(encoded, golden) {
		t.Fatal("companions v5 fixture drift；需要显式 -update-storage-fixtures 重生成并评审字节")
	}
	decoded, err := Decode(golden)
	if err != nil {
		t.Fatal(err)
	}
	want := StoredCompanions{
		SourceSchema:     companionSchemaV5,
		Revision:         save.Revision,
		AgentNamespaceID: save.AgentNamespaceID,
		Records:          save.Records,
		Lifecycles:       save.Lifecycles,
		Queues:           save.Queues,
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("v5 golden decode=%+v，想要 %+v", decoded, want)
	}
	if decoded.Lifecycles[0].MemoryRevision == 0 ||
		decoded.Lifecycles[1].MemoryRevision != 0 || decoded.Lifecycles[1].Summary != "" ||
		decoded.Lifecycles[2].Active || !decoded.Lifecycles[2].TombstoneOperationID.Valid() ||
		len(decoded.Queues) != 1 || !decoded.Queues[0].HasCurrent || len(decoded.Queues[0].Pending) == 0 {
		t.Fatalf("v5 golden 未覆盖 nonzero/zero/tombstone/task/FIFO：%+v", decoded)
	}
}

func TestCompanionCodecV5WireAndStrictRoundTrip(t *testing.T) {
	records := fixtureCompanionBodies()
	queues := fixtureCompanionV3Queues()
	active := fixtureV5Lifecycle(fixtureCompanionID(1), true, 7)
	active.MemoryRevision = 11
	active.MemoryOperationID = fixtureAgentIdentity(0x71)
	active.Summary = "保留的恢复摘要"
	inactive := fixtureV5Lifecycle(fixtureCompanionID(2), false, 9)
	save := CompanionSave{
		Revision:         47,
		AgentNamespaceID: fixtureAgentIdentity(0x70),
		Records:          records,
		Lifecycles:       []StoredCompanionLifecycle{inactive, active},
		Queues:           queues,
	}

	encoded, err := Encode(save)
	if err != nil {
		t.Fatal(err)
	}
	if CurrentSchema != 5 || binary.LittleEndian.Uint32(encoded[8:12]) != 5 {
		t.Fatalf("current/schema=%d/%d，想要 5/5", CurrentSchema, binary.LittleEndian.Uint32(encoded[8:12]))
	}
	if !bytes.Equal(encoded[companionHeaderLength:companionHeaderLength+16], save.AgentNamespaceID[:]) {
		t.Fatal("v5 payload 未以 namespace 的 16 bytes 开头")
	}
	firstFlagsOffset := companionHeaderLength + 16 + companionRecordLength
	if got := encoded[firstFlagsOffset]; got != companionFlagActive|companionFlagHasTask|companionFlagHasFIFO {
		t.Fatalf("首条 active flags=%#x，想要 %#x", got, companionFlagActive|companionFlagHasTask|companionFlagHasFIFO)
	}
	if got := binary.LittleEndian.Uint64(encoded[firstFlagsOffset+1:]); got != 7 {
		t.Fatalf("首条 epoch=%d，想要 7", got)
	}

	got, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	wantRecords := []domain.Body{records[1], records[0]}
	wantLifecycles := []StoredCompanionLifecycle{active, inactive}
	if got.SourceSchema != 5 || got.Revision != 47 || got.AgentNamespaceID != save.AgentNamespaceID ||
		!reflect.DeepEqual(got.Records, wantRecords) ||
		!reflect.DeepEqual(got.Lifecycles, wantLifecycles) ||
		!reflect.DeepEqual(got.Queues, queues) {
		t.Fatalf("v5 round-trip=%+v", got)
	}
	reencoded, err := Encode(CompanionSave{
		Revision:         got.Revision,
		AgentNamespaceID: got.AgentNamespaceID,
		Records:          got.Records,
		Lifecycles:       got.Lifecycles,
		Queues:           got.Queues,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reencoded, encoded) {
		t.Fatal("v5 decode 后重编码字节不规范")
	}
}

func TestCompanionCodecV5MaximumReachableLength(t *testing.T) {
	if MaxFileLength != 393904 {
		t.Fatalf("MaxFileLength=%d，想要 393904", MaxFileLength)
	}
	records := make([]domain.Body, domain.MaxStored)
	lifecycles := make([]StoredCompanionLifecycle, domain.MaxStored)
	queues := make([]StoredCompanionQueue, domain.MaxActive)
	for index := range records {
		records[index] = fixtureCompanionBodies()[0]
		records[index].ID = fixtureCompanionID(byte(index + 1))
		active := index < domain.MaxActive
		lifecycles[index] = fixtureV5Lifecycle(records[index].ID, active, uint64(index+1))
		if !active {
			continue
		}
		lifecycles[index].MemoryRevision = uint64(index + 1)
		lifecycles[index].MemoryOperationID = fixtureAgentIdentity(byte(0x80 + index))
		lifecycles[index].Summary = strings.Repeat("s", MaxCompanionSummaryBytes)
		steps := make([]domain.PlanStep, MaxCompanionPlanSteps)
		for step := 0; step < MaxCompanionPlanSteps-1; step++ {
			steps[step] = domain.PlanStep{
				Kind: domain.PlanStepPlace, X: int32(step), Y: 64, Z: -int32(step),
				Block: core.OakPlanksID,
			}
		}
		steps[len(steps)-1] = domain.PlanStep{Kind: domain.PlanStepFollow, PlayerID: fixtureFollowPlayerID()}
		queues[index] = StoredCompanionQueue{
			ID:         records[index].ID,
			HasCurrent: true,
			Current: StoredCompanionTask{
				Command:   strings.Repeat("c", MaxCompanionTaskCommandBytes),
				PlanSteps: steps,
				StepIndex: len(steps) - 1,
				State:     domain.TaskRunning,
				StartTick: 1,
			},
			Pending: make([]string, MaxCompanionFIFOEntries),
		}
		for pending := range queues[index].Pending {
			queues[index].Pending[pending] = strings.Repeat("p", MaxCompanionTaskCommandBytes)
		}
	}
	encoded, err := Encode(CompanionSave{
		Revision:         1,
		AgentNamespaceID: fixtureAgentIdentity(0x70),
		Records:          records,
		Lifecycles:       lifecycles,
		Queues:           queues,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != MaxFileLength {
		t.Fatalf("最大合法 v5 长度=%d，想要 %d", len(encoded), MaxFileLength)
	}
	if _, err := Decode(encoded); err != nil {
		t.Fatalf("最大合法 v5 decode: %v", err)
	}
	oversized := append(bytes.Clone(encoded), 0)
	if _, err := Decode(oversized); !errors.Is(err, storagedef.ErrCorrupt) ||
		!strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("超一字节 decode error=%v，想要分配前 ErrCorrupt", err)
	}
}

func TestCompanionCodecV5RejectsIdentityLifecycleMatrix(t *testing.T) {
	body := fixtureCompanionBodies()[0]
	valid := fixtureV5Save(1, []domain.Body{body}, nil)
	tests := []struct {
		name   string
		mutate func(*CompanionSave)
	}{
		{"zero namespace", func(save *CompanionSave) { save.AgentNamespaceID = Identity{} }},
		{"bad namespace version", func(save *CompanionSave) { save.AgentNamespaceID[6] = 0x30 }},
		{"zero epoch", func(save *CompanionSave) { save.Lifecycles[0].MemoryEpoch = 0 }},
		{"missing lifecycle", func(save *CompanionSave) { save.Lifecycles = nil }},
		{"orphan lifecycle", func(save *CompanionSave) { save.Lifecycles[0].ID = fixtureCompanionID(9) }},
		{"zero revision with operation", func(save *CompanionSave) {
			save.Lifecycles[0].MemoryOperationID = fixtureAgentIdentity(0x72)
		}},
		{"zero revision with summary", func(save *CompanionSave) { save.Lifecycles[0].Summary = "非法" }},
		{"nonzero revision without operation", func(save *CompanionSave) {
			save.Lifecycles[0].MemoryRevision = 1
		}},
		{"active with tombstone", func(save *CompanionSave) {
			save.Lifecycles[0].TombstoneOperationID = fixtureAgentIdentity(0x73)
		}},
		{"inactive without tombstone", func(save *CompanionSave) {
			save.Lifecycles[0].Active = false
		}},
		{"inactive with queue", func(save *CompanionSave) {
			save.Lifecycles[0].Active = false
			save.Lifecycles[0].TombstoneOperationID = fixtureAgentIdentity(0x73)
			save.Queues = []StoredCompanionQueue{{ID: body.ID, Pending: []string{"排队"}}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			save := valid
			save.Records = append([]domain.Body(nil), valid.Records...)
			save.Lifecycles = append([]StoredCompanionLifecycle(nil), valid.Lifecycles...)
			tc.mutate(&save)
			if _, err := Encode(save); !errors.Is(err, storagedef.ErrCorrupt) {
				t.Fatalf("Encode error=%v，想要 ErrCorrupt", err)
			}
		})
	}
}

func TestCompanionMergeV5ConsumesIdentityInCanonicalOrder(t *testing.T) {
	records := make([]domain.Body, 3)
	for index := range records {
		records[index] = fixtureCompanionBodies()[0]
		records[index].ID = fixtureCompanionID(byte(index + 1))
	}
	legacy := StoredCompanions{
		SourceSchema: 4,
		Revision:     9,
		Records:      []domain.Body{records[2], records[0], records[1]},
		Queues: []StoredCompanionQueue{
			{ID: records[0].ID, Summary: "迁移摘要"},
			{ID: records[1].ID, Pending: []string{"保留任务"}},
		},
	}
	values := []Identity{
		fixtureAgentIdentity(0x01),
		fixtureAgentIdentity(0x02),
		fixtureAgentIdentity(0x03),
	}
	consumed := 0
	generate := func() (Identity, error) {
		value := values[consumed]
		consumed++
		return value, nil
	}
	merged, changed, err := MergeV5(legacy, []domain.Body{records[1], records[0]}, generate)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || consumed != 3 || merged.AgentNamespaceID != values[0] || merged.Revision != 10 ||
		merged.SourceSchema != 5 {
		t.Fatalf("merge identity/revision=%+v consumed=%d changed=%v", merged, consumed, changed)
	}
	wantLifecycles := []StoredCompanionLifecycle{
		{
			ID: records[0].ID, Active: true, MemoryEpoch: 1, MemoryRevision: 1,
			MemoryOperationID: values[1], Summary: "迁移摘要",
		},
		{ID: records[1].ID, Active: true, MemoryEpoch: 1},
		{
			ID: records[2].ID, Active: false, MemoryEpoch: 1,
			TombstoneOperationID: values[2],
		},
	}
	if !reflect.DeepEqual(merged.Lifecycles, wantLifecycles) {
		t.Fatalf("迁移 lifecycle=%+v，想要 %+v", merged.Lifecycles, wantLifecycles)
	}
	if len(merged.Queues) != 1 || merged.Queues[0].ID != records[1].ID ||
		merged.Queues[0].Summary != "" {
		t.Fatalf("迁移 queues=%+v，想要仅 active 任务且裸 summary 已清除", merged.Queues)
	}
}

func TestCompanionMergeV5TransitionsOverflowAndAtomicFailure(t *testing.T) {
	records := fixtureCompanionBodies()
	active := fixtureV5Lifecycle(fixtureCompanionID(1), true, 5)
	active.MemoryRevision = 7
	active.MemoryOperationID = fixtureAgentIdentity(0x21)
	active.Summary = "旧摘要"
	inactive := fixtureV5Lifecycle(fixtureCompanionID(2), false, 8)
	loaded := StoredCompanions{
		SourceSchema:     5,
		Revision:         12,
		AgentNamespaceID: fixtureAgentIdentity(0x20),
		Records:          []domain.Body{records[1], records[0]},
		Lifecycles:       []StoredCompanionLifecycle{active, inactive},
		Queues:           []StoredCompanionQueue{{ID: active.ID, Pending: []string{"旧队列"}}},
	}
	before := cloneStoredCompanionsForV5Test(loaded)
	generated := []Identity{fixtureAgentIdentity(0x22)}
	calls := 0
	merged, changed, err := MergeV5(loaded, []domain.Body{records[0]}, func() (Identity, error) {
		calls++
		return generated[calls-1], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || calls != 1 || merged.Revision != 13 {
		t.Fatalf("transition changed=%v calls=%d revision=%d", changed, calls, merged.Revision)
	}
	if merged.Lifecycles[0].Active || merged.Lifecycles[0].MemoryEpoch != 6 ||
		merged.Lifecycles[0].TombstoneOperationID != generated[0] || len(merged.Queues) != 0 {
		t.Fatalf("active→inactive=%+v queues=%+v", merged.Lifecycles[0], merged.Queues)
	}
	if !reflect.DeepEqual(loaded, before) {
		t.Fatal("MergeV5 修改了调用方输入")
	}

	allInactive := cloneStoredCompanionsForV5Test(merged)
	for index := range allInactive.Lifecycles {
		allInactive.Lifecycles[index] = StoredCompanionLifecycle{
			ID: allInactive.Lifecycles[index].ID, Active: false,
			MemoryEpoch:          allInactive.Lifecycles[index].MemoryEpoch,
			TombstoneOperationID: fixtureAgentIdentity(byte(0x30 + index)),
		}
	}
	allInactive.Queues = nil
	allInactive.Revision = math.MaxUint64
	noOp, noChange, err := MergeV5(allInactive, nil, func() (Identity, error) {
		t.Fatal("全 inactive no-op 不得消费 identity")
		return Identity{}, nil
	})
	if err != nil || noChange || !reflect.DeepEqual(noOp, allInactive) {
		t.Fatalf("全 inactive MaxUint64 no-op=%+v changed=%v err=%v", noOp, noChange, err)
	}

	overflow := cloneStoredCompanionsForV5Test(loaded)
	overflow.Revision = math.MaxUint64
	if _, _, err := MergeV5(overflow, nil, func() (Identity, error) {
		t.Fatal("aggregate overflow 必须先于 identity")
		return Identity{}, nil
	}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("aggregate overflow error=%v，想要 ErrCorrupt", err)
	}
	if !reflect.DeepEqual(loaded, before) {
		t.Fatal("overflow 路径修改了原输入")
	}
}

func TestCompanionMergeV5RejectsCapacityAndEntropyFailureAtomically(t *testing.T) {
	records := make([]domain.Body, domain.MaxStored)
	for index := range records {
		records[index] = fixtureCompanionBodies()[0]
		records[index].ID = fixtureCompanionID(byte(index + 1))
	}
	legacy := StoredCompanions{SourceSchema: 1, Revision: 1, Records: records}
	before := cloneStoredCompanionsForV5Test(legacy)
	extra := fixtureCompanionBodies()[0]
	extra.ID = domain.ID{0x22, 0, 0, 0, 0, 0, 0x40, 0, 0x80, 0, 0, 0, 0, 0, 0, 0x7f}
	if _, _, err := MergeV5(legacy, []domain.Body{extra}, func() (Identity, error) {
		t.Fatal("容量失败必须先于 identity")
		return Identity{}, nil
	}); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("capacity error=%v，想要 ErrCorrupt", err)
	}
	if !reflect.DeepEqual(legacy, before) {
		t.Fatal("容量失败修改了输入")
	}

	failed := errors.New("entropy unavailable")
	calls := 0
	if _, _, err := MergeV5(StoredCompanions{
		SourceSchema: 4,
		Revision:     3,
		Records:      fixtureCompanionBodies(),
	}, nil, func() (Identity, error) {
		calls++
		if calls == 2 {
			return Identity{}, failed
		}
		return fixtureAgentIdentity(byte(calls)), nil
	}); !errors.Is(err, failed) {
		t.Fatalf("entropy failure=%v，想要 injected error", err)
	}
	if calls != 2 {
		t.Fatalf("entropy calls=%d，想要在第二个 identity 立即停止", calls)
	}
}

func cloneStoredCompanionsForV5Test(stored StoredCompanions) StoredCompanions {
	return cloneStoredCompanions(stored)
}
