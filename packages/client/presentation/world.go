package presentation

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// `MaxSectionMeshQuads` bounds one section to the production mesher's worst case.
	MaxSectionMeshQuads = 6 * core.BlocksPerSection
	// `MaxWorldBatchOperations` bounds validation and host publication work per batch.
	MaxWorldBatchOperations = 4096
	// `MaxWorldBatchMeshBytes` bounds semantic mesh payload to 4 MiB; a later ABI
	// layout applies its fixed-header overhead inside the transport limit.
	MaxWorldBatchMeshBytes = 4 * 1024 * 1024
	// `MaxWorldBatchPackedQuads` is the corresponding packed-face capacity.
	MaxWorldBatchPackedQuads = MaxWorldBatchMeshBytes / 8
)

// `AtlasRevision` identifies the atlas and registry content required by a mesh batch.
type AtlasRevision uint64

// `SectionMeshPayload` owns the packed section faces produced by the shared mesher.
// It has no renderer resource identity and copies at each public slice boundary.
type SectionMeshPayload struct {
	packedQuads []uint64
}

// `NewSectionMeshPayload` validates and copies a bounded packed-quad payload.
func NewSectionMeshPayload(packedQuads []uint64) (SectionMeshPayload, error) {
	payload := SectionMeshPayload{packedQuads: packedQuads}
	if err := payload.Validate(); err != nil {
		return SectionMeshPayload{}, err
	}
	payload.packedQuads = append([]uint64(nil), packedQuads...)
	return payload, nil
}

// `Validate` rejects a payload that cannot be consumed by the existing mesh contract.
func (payload SectionMeshPayload) Validate() error {
	if len(payload.packedQuads) == 0 {
		return errors.New("presentation: section mesh has no quads; use a section drop")
	}
	if len(payload.packedQuads) > MaxSectionMeshQuads {
		return fmt.Errorf("presentation: section mesh has %d quads, maximum is %d", len(payload.packedQuads), MaxSectionMeshQuads)
	}
	for index, packed := range payload.packedQuads {
		if err := validatePackedQuad(packed); err != nil {
			return fmt.Errorf("presentation: section mesh quad %d is invalid: %w", index, err)
		}
	}
	return nil
}

// `PackedQuads` returns an owned copy so a receiving host cannot mutate the payload.
func (payload SectionMeshPayload) PackedQuads() []uint64 {
	return append([]uint64(nil), payload.packedQuads...)
}

// `Len` reports the number of packed faces in the payload.
func (payload SectionMeshPayload) Len() int {
	return len(payload.packedQuads)
}

// `SectionMeshUpsert` replaces one complete section at its presentation revision.
type SectionMeshUpsert struct {
	Key      core.SectionKey
	Revision uint64
	Payload  SectionMeshPayload
}

// `Validate` checks that an upsert identifies a current world section and complete mesh.
func (upsert SectionMeshUpsert) Validate() error {
	if err := validateSectionKey(upsert.Key); err != nil {
		return err
	}
	if upsert.Revision == 0 {
		return errors.New("presentation: section upsert has zero revision")
	}
	return upsert.Payload.Validate()
}

// `SectionDrop` removes one complete section at its presentation revision.
type SectionDrop struct {
	Key      core.SectionKey
	Revision uint64
}

// `Validate` checks that a drop identifies a current world section and revision.
func (drop SectionDrop) Validate() error {
	if err := validateSectionKey(drop.Key); err != nil {
		return err
	}
	if drop.Revision == 0 {
		return errors.New("presentation: section drop has zero revision")
	}
	return nil
}

// `WorldBatch` is an owned, bounded all-or-nothing set of section changes for one epoch.
type WorldBatch struct {
	Epoch         uint64
	AtlasRevision AtlasRevision
	upserts       []SectionMeshUpsert
	drops         []SectionDrop
}

// `NewWorldBatch` validates all operations before returning an owned batch.
func NewWorldBatch(
	epoch uint64,
	atlasRevision AtlasRevision,
	upserts []SectionMeshUpsert,
	drops []SectionDrop,
) (WorldBatch, error) {
	batch := WorldBatch{
		Epoch:         epoch,
		AtlasRevision: atlasRevision,
		upserts:       upserts,
		drops:         drops,
	}
	if err := batch.Validate(); err != nil {
		return WorldBatch{}, err
	}
	batch.upserts = append([]SectionMeshUpsert(nil), upserts...)
	batch.drops = append([]SectionDrop(nil), drops...)
	return batch, nil
}

// `Validate` rejects incomplete, conflicting, or over-capacity operations before publication.
func (batch WorldBatch) Validate() error {
	if batch.Epoch == 0 {
		return errors.New("presentation: world batch has zero epoch")
	}
	if batch.AtlasRevision == 0 {
		return errors.New("presentation: world batch has zero atlas revision")
	}
	operationCount := len(batch.upserts) + len(batch.drops)
	if operationCount == 0 || operationCount > MaxWorldBatchOperations {
		return fmt.Errorf("presentation: world batch operation count %d is invalid", operationCount)
	}
	seen := make(map[core.SectionKey]struct{}, operationCount)
	packedQuadCount := 0
	for index, upsert := range batch.upserts {
		if err := upsert.Validate(); err != nil {
			return fmt.Errorf("presentation: world batch upsert %d: %w", index, err)
		}
		packedQuadCount += upsert.Payload.Len()
		if packedQuadCount > MaxWorldBatchPackedQuads {
			return fmt.Errorf("presentation: world batch has %d packed quads, maximum is %d", packedQuadCount, MaxWorldBatchPackedQuads)
		}
		if _, exists := seen[upsert.Key]; exists {
			return fmt.Errorf("presentation: world batch has duplicate section %v", upsert.Key)
		}
		seen[upsert.Key] = struct{}{}
	}
	for index, drop := range batch.drops {
		if err := drop.Validate(); err != nil {
			return fmt.Errorf("presentation: world batch drop %d: %w", index, err)
		}
		if _, exists := seen[drop.Key]; exists {
			return fmt.Errorf("presentation: world batch has duplicate section %v", drop.Key)
		}
		seen[drop.Key] = struct{}{}
	}
	return nil
}

// `ValidateAfter` rejects batches from another epoch, an older atlas, or stale sections.
// The lookup is read-only; successful validation does not publish or mutate any caller state.
func (batch WorldBatch) ValidateAfter(
	epoch uint64,
	atlasRevision AtlasRevision,
	lookup func(core.SectionKey) (uint64, bool),
) error {
	if err := batch.Validate(); err != nil {
		return err
	}
	if batch.Epoch != epoch {
		return errors.New("presentation: world batch epoch does not match receiver")
	}
	if batch.AtlasRevision < atlasRevision {
		return errors.New("presentation: world batch atlas revision is stale")
	}
	if lookup == nil {
		return nil
	}
	for _, upsert := range batch.upserts {
		if revision, exists := lookup(upsert.Key); exists && upsert.Revision <= revision {
			return fmt.Errorf("presentation: section upsert revision %d is stale", upsert.Revision)
		}
	}
	for _, drop := range batch.drops {
		if revision, exists := lookup(drop.Key); exists && drop.Revision <= revision {
			return fmt.Errorf("presentation: section drop revision %d is stale", drop.Revision)
		}
	}
	return nil
}

// `Upserts` returns an owned copy of the batch's upsert records.
func (batch WorldBatch) Upserts() []SectionMeshUpsert {
	return append([]SectionMeshUpsert(nil), batch.upserts...)
}

// `Drops` returns an owned copy of the batch's drop records.
func (batch WorldBatch) Drops() []SectionDrop {
	return append([]SectionDrop(nil), batch.drops...)
}

func validateSectionKey(key core.SectionKey) error {
	if key.Dimension != core.Overworld && key.Dimension != core.Depths {
		return fmt.Errorf("presentation: section has unknown dimension %d", key.Dimension)
	}
	if key.Pos.Y < 0 || key.Pos.Y >= core.SectionsPerChunk {
		return fmt.Errorf("presentation: section Y %d is outside the world", key.Pos.Y)
	}
	return nil
}

func validatePackedQuad(packed uint64) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	mesh.UnpackQuad(packed)
	return nil
}
