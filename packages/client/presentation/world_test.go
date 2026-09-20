package presentation

import (
	"math"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestSectionMeshUpsertAndDropValidateCurrentSectionDomain(t *testing.T) {
	payload, err := NewSectionMeshPayload([]uint64{testPackedQuad()})
	if err != nil {
		t.Fatal(err)
	}
	key := core.SectionKey{
		Dimension: core.Depths,
		Pos: core.SectionPos{
			X: math.MinInt32,
			Y: core.SectionsPerChunk - 1,
			Z: math.MaxInt32,
		},
	}
	upsert := SectionMeshUpsert{Key: key, Revision: 7, Payload: payload}
	if err := upsert.Validate(); err != nil {
		t.Fatalf("valid upsert: %v", err)
	}
	if err := (SectionDrop{Key: key, Revision: 8}).Validate(); err != nil {
		t.Fatalf("valid drop: %v", err)
	}

	for _, test := range []struct {
		name string
		key  core.SectionKey
	}{
		{"unknown dimension", core.SectionKey{Dimension: 2, Pos: key.Pos}},
		{"negative section Y", core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{Y: -1}}},
		{"section Y past ceiling", core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{Y: core.SectionsPerChunk}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := (SectionMeshUpsert{Key: test.key, Revision: 1, Payload: payload}).Validate(); err == nil {
				t.Fatal("SectionMeshUpsert.Validate() = nil")
			}
			if err := (SectionDrop{Key: test.key, Revision: 1}).Validate(); err == nil {
				t.Fatal("SectionDrop.Validate() = nil")
			}
		})
	}
	if err := (SectionMeshUpsert{Key: key, Payload: payload}).Validate(); err == nil {
		t.Fatal("zero upsert revision was accepted")
	}
	if err := (SectionDrop{Key: key}).Validate(); err == nil {
		t.Fatal("zero drop revision was accepted")
	}
}

func TestSectionMeshPayloadOwnsBoundedPackedQuads(t *testing.T) {
	packed := []uint64{testPackedQuad()}
	payload, err := NewSectionMeshPayload(packed)
	if err != nil {
		t.Fatal(err)
	}
	packed[0] = 0
	if got := payload.PackedQuads(); !reflect.DeepEqual(got, []uint64{testPackedQuad()}) {
		t.Fatalf("payload retained caller memory: %#x", got)
	}

	copyOut := payload.PackedQuads()
	copyOut[0] = 0
	if got := payload.PackedQuads(); !reflect.DeepEqual(got, []uint64{testPackedQuad()}) {
		t.Fatalf("payload returned mutable memory: %#x", got)
	}
	if _, err := NewSectionMeshPayload([]uint64{1 << 63}); err == nil {
		t.Fatal("invalid packed quad was accepted")
	}
	plant := (mesh.Quad{W: 1, H: 1, Face: mesh.FacePlantDiagA, Mat: 1}).Pack()
	if _, err := NewSectionMeshPayload([]uint64{plant | 1<<14}); err == nil {
		t.Fatal("packed quad with reserved plant bits was accepted")
	}
	if _, err := NewSectionMeshPayload(nil); err == nil {
		t.Fatal("empty upsert payload was accepted")
	}
	if _, err := NewSectionMeshPayload(make([]uint64, MaxSectionMeshQuads+1)); err == nil {
		t.Fatal("over-capacity payload was accepted")
	}
}

func TestWorldBatchCopiesAndValidatesAtomically(t *testing.T) {
	payload, err := NewSectionMeshPayload([]uint64{testPackedQuad()})
	if err != nil {
		t.Fatal(err)
	}
	upserts := []SectionMeshUpsert{{
		Key:      core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: -2, Y: 3, Z: 4}},
		Revision: 11,
		Payload:  payload,
	}}
	drops := []SectionDrop{{
		Key:      core.SectionKey{Dimension: core.Depths, Pos: core.SectionPos{X: 5, Y: 6, Z: -7}},
		Revision: 12,
	}}
	batch, err := NewWorldBatch(3, 9, upserts, drops)
	if err != nil {
		t.Fatal(err)
	}
	if err := batch.Validate(); err != nil {
		t.Fatalf("valid batch: %v", err)
	}

	upserts[0].Revision = 99
	drops[0].Revision = 99
	if got := batch.Upserts()[0].Revision; got != 11 {
		t.Fatalf("batch retained caller upserts: revision=%d", got)
	}
	if got := batch.Drops()[0].Revision; got != 12 {
		t.Fatalf("batch retained caller drops: revision=%d", got)
	}

	copyUpserts := batch.Upserts()
	copyDrops := batch.Drops()
	copyUpserts[0].Revision = 100
	copyDrops[0].Revision = 100
	if batch.Upserts()[0].Revision != 11 || batch.Drops()[0].Revision != 12 {
		t.Fatal("batch exposed mutable operation slices")
	}
}

func TestWorldBatchRejectsInvalidOrConflictingOperationsWithoutPublication(t *testing.T) {
	payload, err := NewSectionMeshPayload([]uint64{testPackedQuad()})
	if err != nil {
		t.Fatal(err)
	}
	key := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: 1, Y: 2, Z: 3}}
	validUpsert := SectionMeshUpsert{Key: key, Revision: 7, Payload: payload}
	validDrop := SectionDrop{Key: core.SectionKey{Dimension: core.Depths}, Revision: 8}

	for _, test := range []struct {
		name    string
		epoch   uint64
		atlas   AtlasRevision
		upserts []SectionMeshUpsert
		drops   []SectionDrop
	}{
		{"zero epoch", 0, 1, []SectionMeshUpsert{validUpsert}, nil},
		{"zero atlas revision", 1, 0, []SectionMeshUpsert{validUpsert}, nil},
		{"empty operations", 1, 1, nil, nil},
		{"invalid middle operation", 1, 1, []SectionMeshUpsert{validUpsert, {Key: key, Payload: payload}}, []SectionDrop{validDrop}},
		{"duplicate upsert", 1, 1, []SectionMeshUpsert{validUpsert, validUpsert}, nil},
		{"duplicate drop", 1, 1, nil, []SectionDrop{validDrop, validDrop}},
		{"conflicting upsert and drop", 1, 1, []SectionMeshUpsert{validUpsert}, []SectionDrop{{Key: key, Revision: 8}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			beforeUpserts := append([]SectionMeshUpsert(nil), test.upserts...)
			beforeDrops := append([]SectionDrop(nil), test.drops...)
			batch, err := NewWorldBatch(test.epoch, test.atlas, test.upserts, test.drops)
			if err == nil {
				t.Fatal("NewWorldBatch() = nil error")
			}
			if batch.Epoch != 0 || batch.AtlasRevision != 0 || len(batch.Upserts()) != 0 || len(batch.Drops()) != 0 {
				t.Fatalf("failed batch published state: %+v", batch)
			}
			if !reflect.DeepEqual(test.upserts, beforeUpserts) || !reflect.DeepEqual(test.drops, beforeDrops) {
				t.Fatal("validation mutated caller operations")
			}
		})
	}
}

func TestWorldBatchOperationCapacityBoundary(t *testing.T) {
	drops := sectionDrops(MaxWorldBatchOperations)
	if _, err := NewWorldBatch(1, 1, nil, drops); err != nil {
		t.Fatalf("world batch at operation capacity: %v", err)
	}
	drops = sectionDrops(MaxWorldBatchOperations + 1)
	if _, err := NewWorldBatch(1, 1, nil, drops); err == nil {
		t.Fatal("world batch above operation capacity was accepted")
	}
}

func TestWorldBatchPackedQuadCapacityBoundary(t *testing.T) {
	upserts := sectionUpsertsWithPackedQuads(t, MaxWorldBatchPackedQuads)
	if _, err := NewWorldBatch(1, 1, upserts, nil); err != nil {
		t.Fatalf("world batch at packed-quad capacity: %v", err)
	}
	upserts = sectionUpsertsWithPackedQuads(t, MaxWorldBatchPackedQuads+1)
	if _, err := NewWorldBatch(1, 1, upserts, nil); err == nil {
		t.Fatal("world batch above packed-quad capacity was accepted")
	}
}

func TestWorldBatchRejectsStaleRevisionsWithoutMutatingLookup(t *testing.T) {
	payload, err := NewSectionMeshPayload([]uint64{testPackedQuad()})
	if err != nil {
		t.Fatal(err)
	}
	key := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{Y: 2}}
	batch, err := NewWorldBatch(4, 9, []SectionMeshUpsert{{Key: key, Revision: 7, Payload: payload}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	revisions := map[core.SectionKey]uint64{key: 7}
	if err := batch.ValidateAfter(4, 9, func(lookup core.SectionKey) (uint64, bool) {
		value, ok := revisions[lookup]
		return value, ok
	}); err == nil {
		t.Fatal("stale section revision was accepted")
	}
	if got := revisions[key]; got != 7 {
		t.Fatalf("validation mutated revision lookup: %d", got)
	}
	if err := batch.ValidateAfter(4, 10, nil); err == nil {
		t.Fatal("stale atlas revision was accepted")
	}
	if err := batch.ValidateAfter(3, 1, nil); err == nil {
		t.Fatal("epoch mismatch was accepted")
	}
	if err := batch.ValidateAfter(4, 9, func(lookup core.SectionKey) (uint64, bool) {
		return 6, lookup == key
	}); err != nil {
		t.Fatalf("new section revision with unchanged atlas rejected: %v", err)
	}

	dropBatch, err := NewWorldBatch(4, 9, nil, []SectionDrop{{Key: key, Revision: 7}})
	if err != nil {
		t.Fatal(err)
	}
	if err := dropBatch.ValidateAfter(4, 9, func(lookup core.SectionKey) (uint64, bool) {
		return 7, lookup == key
	}); err == nil {
		t.Fatal("stale section drop revision was accepted")
	}
}

func testPackedQuad() uint64 {
	return (mesh.Quad{W: 1, H: 1, Face: mesh.FacePosY, Mat: 1}).Pack()
}

func sectionDrops(count int) []SectionDrop {
	drops := make([]SectionDrop, count)
	for index := range drops {
		drops[index] = SectionDrop{
			Key:      core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: int32(index), Y: int32(index % core.SectionsPerChunk)}},
			Revision: uint64(index + 1),
		}
	}
	return drops
}

func sectionUpsertsWithPackedQuads(t *testing.T, count int) []SectionMeshUpsert {
	t.Helper()
	upserts := make([]SectionMeshUpsert, 0, (count+MaxSectionMeshQuads-1)/MaxSectionMeshQuads)
	for remaining, index := count, 0; remaining > 0; index++ {
		quadCount := min(remaining, MaxSectionMeshQuads)
		packed := make([]uint64, quadCount)
		for quad := range packed {
			packed[quad] = testPackedQuad()
		}
		payload, err := NewSectionMeshPayload(packed)
		if err != nil {
			t.Fatal(err)
		}
		upserts = append(upserts, SectionMeshUpsert{
			Key:      core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: int32(index), Y: int32(index % core.SectionsPerChunk)}},
			Revision: uint64(index + 1),
			Payload:  payload,
		})
		remaining -= quadCount
	}
	return upserts
}
