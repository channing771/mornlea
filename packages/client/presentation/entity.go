package presentation

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// `MaxEntityBatchRecords` is the protocol-aligned maximum number of remote
	// players visible to one client. Keeping it here bounds host work without
	// exposing any renderer instance layout.
	MaxEntityBatchRecords = 7
)

// `EntityKind` identifies a presentation entity semantic, not a renderer resource.
type EntityKind uint8

const (
	// `EntityKindRemotePlayer` is the pilot's only remote entity semantic. Its
	// state remains a server-confirmed client mirror rather than gameplay authority.
	EntityKindRemotePlayer EntityKind = 1
)

// `Valid` reports whether the kind belongs to the current presentation contract.
func (kind EntityKind) Valid() bool {
	return kind == EntityKindRemotePlayer
}

// `EntityRecord` is one renderer-independent remote-player pose at a server tick.
// Coordinates and orientation remain Mornlea values; an adapter performs any host
// transform conversion only when it presents this confirmed mirror state.
type EntityRecord struct {
	Kind      EntityKind
	PlayerID  core.PlayerID
	Dimension core.DimensionID
	Position  [3]float32
	Yaw       float32
	Pitch     float32
}

// `Validate` rejects records that cannot represent a current remote-player mirror.
func (record EntityRecord) Validate() error {
	if !record.Kind.Valid() {
		return fmt.Errorf("presentation: entity has unknown kind %d", record.Kind)
	}
	if !record.PlayerID.Valid() {
		return errors.New("presentation: entity has invalid player identity")
	}
	if record.Dimension != core.Overworld && record.Dimension != core.Depths {
		return fmt.Errorf("presentation: entity has unknown dimension %d", record.Dimension)
	}
	for _, component := range record.Position {
		if !finiteFloat32(component) {
			return errors.New("presentation: entity position is not finite")
		}
	}
	if !finiteFloat32(record.Yaw) || !finiteFloat32(record.Pitch) {
		return errors.New("presentation: entity orientation is not finite")
	}
	return nil
}

// `EntityBatch` is an owned all-or-nothing remote-player snapshot for one server tick.
// It deliberately stores semantic records instead of the legacy 96-byte GPU instance.
type EntityBatch struct {
	ServerTick uint64
	records    []EntityRecord
}

// `NewEntityBatch` validates every record before copying the batch into owned storage.
func NewEntityBatch(serverTick uint64, records []EntityRecord) (EntityBatch, error) {
	batch := EntityBatch{ServerTick: serverTick, records: records}
	if err := batch.Validate(); err != nil {
		return EntityBatch{}, err
	}
	batch.records = append([]EntityRecord(nil), records...)
	return batch, nil
}

// `Validate` rejects invalid, duplicate, or over-capacity records before publication.
func (batch EntityBatch) Validate() error {
	if len(batch.records) > MaxEntityBatchRecords {
		return fmt.Errorf("presentation: entity batch has %d records, maximum is %d", len(batch.records), MaxEntityBatchRecords)
	}
	seen := make(map[core.PlayerID]struct{}, len(batch.records))
	for index, record := range batch.records {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("presentation: entity batch record %d: %w", index, err)
		}
		if _, exists := seen[record.PlayerID]; exists {
			return fmt.Errorf("presentation: entity batch has duplicate player identity %s", record.PlayerID)
		}
		seen[record.PlayerID] = struct{}{}
	}
	return nil
}

// `ValidateAfter` rejects batches that would move a receiver backward or repeat a tick.
// It is read-only so callers can reject the whole batch without changing published state.
func (batch EntityBatch) ValidateAfter(lastServerTick uint64) error {
	if err := batch.Validate(); err != nil {
		return err
	}
	if batch.ServerTick <= lastServerTick {
		return fmt.Errorf("presentation: entity batch tick %d is not newer than %d", batch.ServerTick, lastServerTick)
	}
	return nil
}

// `Records` returns an owned copy so a receiving host cannot mutate the batch.
func (batch EntityBatch) Records() []EntityRecord {
	return append([]EntityRecord(nil), batch.records...)
}
