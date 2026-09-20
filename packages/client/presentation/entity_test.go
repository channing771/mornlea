package presentation

import (
	"math"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestEntityKindAndRecordValidateRemotePlayerSemantics(t *testing.T) {
	record := validRemotePlayerEntity(1)
	if !record.Kind.Valid() {
		t.Fatalf("valid entity kind %d is invalid", record.Kind)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("valid entity record: %v", err)
	}
	if EntityKind(255).Valid() {
		t.Fatal("unknown entity kind is valid")
	}

	for _, test := range []struct {
		name   string
		mutate func(*EntityRecord)
	}{
		{"unknown kind", func(record *EntityRecord) { record.Kind = EntityKind(255) }},
		{"invalid player identity", func(record *EntityRecord) { record.PlayerID = core.PlayerID{} }},
		{"unknown dimension", func(record *EntityRecord) { record.Dimension = core.DimensionID(2) }},
		{"non-finite position", func(record *EntityRecord) { record.Position[1] = float32(math.NaN()) }},
		{"non-finite yaw", func(record *EntityRecord) { record.Yaw = float32(math.Inf(1)) }},
		{"non-finite pitch", func(record *EntityRecord) { record.Pitch = float32(math.Inf(-1)) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := record
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("EntityRecord.Validate() = nil for %+v", candidate)
			}
		})
	}
}

func TestEntityBatchCopiesRecordsAndValidatesAtomically(t *testing.T) {
	records := []EntityRecord{validRemotePlayerEntity(1), validRemotePlayerEntity(2)}
	batch, err := NewEntityBatch(17, records)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.Validate(); err != nil {
		t.Fatalf("valid entity batch: %v", err)
	}

	records[0].Position[0] = 99
	if got := batch.Records()[0].Position[0]; got != 1 {
		t.Fatalf("batch retained caller records: position=%v", got)
	}
	copyOut := batch.Records()
	copyOut[0].Position[0] = 100
	if got := batch.Records()[0].Position[0]; got != 1 {
		t.Fatalf("batch exposed mutable record slice: position=%v", got)
	}

	invalid := append([]EntityRecord(nil), records...)
	invalid[1].Kind = EntityKind(255)
	before := append([]EntityRecord(nil), invalid...)
	failed, err := NewEntityBatch(18, invalid)
	if err == nil {
		t.Fatal("NewEntityBatch() = nil error for an invalid record")
	}
	if failed.ServerTick != 0 || len(failed.Records()) != 0 {
		t.Fatalf("failed entity batch published state: %+v", failed)
	}
	if !reflect.DeepEqual(invalid, before) {
		t.Fatal("failed validation mutated caller records")
	}
}

func TestEntityBatchRejectsDuplicateIdentitiesAndOverflow(t *testing.T) {
	duplicate := validRemotePlayerEntity(1)
	if _, err := NewEntityBatch(1, []EntityRecord{duplicate, duplicate}); err == nil {
		t.Fatal("duplicate remote-player identity was accepted")
	}

	records := make([]EntityRecord, MaxEntityBatchRecords)
	for index := range records {
		records[index] = validRemotePlayerEntity(byte(index + 1))
	}
	if _, err := NewEntityBatch(1, records); err != nil {
		t.Fatalf("entity batch at capacity: %v", err)
	}
	if MaxEntityBatchRecords != 7 {
		t.Fatalf("MaxEntityBatchRecords=%d, want protocol-aligned remote-player capacity 7", MaxEntityBatchRecords)
	}
	if _, err := NewEntityBatch(1, append(records, validRemotePlayerEntity(8))); err == nil {
		t.Fatal("over-capacity entity batch was accepted")
	}
}

func TestEntityBatchRejectsOutOfOrderTicksWithoutPublishing(t *testing.T) {
	batch, err := NewEntityBatch(12, []EntityRecord{validRemotePlayerEntity(1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.ValidateAfter(11); err != nil {
		t.Fatalf("newer entity batch: %v", err)
	}
	if err := batch.ValidateAfter(12); err == nil {
		t.Fatal("equal entity batch tick was accepted")
	}
	if err := batch.ValidateAfter(13); err == nil {
		t.Fatal("older entity batch tick was accepted")
	}
	if batch.ServerTick != 12 || len(batch.Records()) != 1 {
		t.Fatalf("receiver validation mutated batch: %+v", batch)
	}
}

func TestEntityRecordsContainNoMutableReferences(t *testing.T) {
	assertNoMutableReferences(t, reflect.TypeOf(EntityKind(0)), "EntityKind")
	assertNoMutableReferences(t, reflect.TypeOf(EntityRecord{}), "EntityRecord")
}

func validRemotePlayerEntity(id byte) EntityRecord {
	return EntityRecord{
		Kind:      EntityKindRemotePlayer,
		PlayerID:  validEntityPlayerID(id),
		Dimension: core.Depths,
		Position:  [3]float32{float32(id), 2, 3},
		Yaw:       0.25,
		Pitch:     -0.5,
	}
}

func validEntityPlayerID(id byte) core.PlayerID {
	return core.PlayerID{0: id, 6: 0x40, 8: 0x80}
}
