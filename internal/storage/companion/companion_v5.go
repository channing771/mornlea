package companion

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math"
	"slices"

	domain "github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/storage/storagedef"
)

// IdentityGenerator 为 bootstrap 提供可失败、可注入的 UUIDv4 entropy。
// MergeV5 只按规范顺序调用它，便于证明失败时没有半迁移状态。
type IdentityGenerator func() (Identity, error)

// GenerateIdentity 从系统随机源生成一个 canonical UUIDv4 identity。
func GenerateIdentity() (Identity, error) {
	var identity Identity
	if _, err := rand.Read(identity[:]); err != nil {
		return Identity{}, fmt.Errorf("generate companion identity: %w", err)
	}
	identity[6] = identity[6]&0x0f | 0x40
	identity[8] = identity[8]&0x3f | 0x80
	return identity, nil
}

type pendingIdentityKind uint8

const (
	pendingIdentityNone pendingIdentityKind = iota
	pendingIdentityMirror
	pendingIdentityTombstone
)

type mergedLifecycle struct {
	lifecycle StoredCompanionLifecycle
	pending   pendingIdentityKind
}

// MergeV5 把 missing/legacy/v5 聚合与当前 active 身体集合做一次纯配置合并。
// active 中已有 ID 的 Body 只用于识别配置集合，已存身体保持不变；新 ID 使用
// 调用方提供的 provisional Body。任一校验、overflow 或 entropy 错误都不会
// 修改输入，也不会返回可保存的部分结果。
func MergeV5(
	loaded StoredCompanions,
	active []domain.Body,
	generate IdentityGenerator,
) (StoredCompanions, bool, error) {
	input, err := validateMergeInput(loaded)
	if err != nil {
		return StoredCompanions{}, false, err
	}
	activeByID, err := validateActiveBodies(active)
	if err != nil {
		return StoredCompanions{}, false, err
	}

	bodyByID := make(map[domain.ID]domain.Body, len(input.Records)+len(activeByID))
	for _, body := range input.Records {
		bodyByID[body.ID] = body
	}
	for id, body := range activeByID {
		if _, exists := bodyByID[id]; !exists {
			bodyByID[id] = body
		}
	}
	if len(bodyByID) > domain.MaxStored {
		return StoredCompanions{}, false, fmt.Errorf(
			"%w: companion active+inactive count %d exceeds limit",
			storagedef.ErrCorrupt, len(bodyByID),
		)
	}

	records := make([]domain.Body, 0, len(bodyByID))
	for _, body := range bodyByID {
		records = append(records, body)
	}
	slices.SortFunc(records, compareCompanionBodies)
	queueByID := make(map[domain.ID]StoredCompanionQueue, len(input.Queues))
	for _, queue := range input.Queues {
		queueByID[queue.ID] = cloneStoredCompanionQueue(queue)
	}
	oldLifecycleByID := make(map[domain.ID]StoredCompanionLifecycle, len(input.Lifecycles))
	for _, lifecycle := range input.Lifecycles {
		oldLifecycleByID[lifecycle.ID] = lifecycle
	}

	changed := input.SourceSchema != companionSchemaV5
	merged := make([]mergedLifecycle, 0, len(records))
	var queues []StoredCompanionQueue
	for _, body := range records {
		_, shouldBeActive := activeByID[body.ID]
		oldLifecycle, hasLifecycle := oldLifecycleByID[body.ID]
		var next mergedLifecycle
		switch {
		case input.SourceSchema != companionSchemaV5:
			next.lifecycle = StoredCompanionLifecycle{
				ID: body.ID, Active: shouldBeActive, MemoryEpoch: 1,
			}
			if shouldBeActive {
				if queue, exists := queueByID[body.ID]; exists {
					if queue.Summary != "" {
						next.lifecycle.MemoryRevision = 1
						next.lifecycle.Summary = queue.Summary
						next.pending = pendingIdentityMirror
					}
					queue.Summary = ""
					if queue.HasCurrent || len(queue.Pending) != 0 {
						queues = append(queues, queue)
					}
				}
			} else {
				next.pending = pendingIdentityTombstone
			}
		case !hasLifecycle:
			changed = true
			next.lifecycle = StoredCompanionLifecycle{
				ID: body.ID, Active: true, MemoryEpoch: 1,
			}
		case oldLifecycle.Active == shouldBeActive:
			next.lifecycle = oldLifecycle
			if shouldBeActive {
				if queue, exists := queueByID[body.ID]; exists {
					queues = append(queues, queue)
				}
			}
		case oldLifecycle.MemoryEpoch == math.MaxUint64:
			return StoredCompanions{}, false, fmt.Errorf(
				"%w: companion %s memory epoch overflow", storagedef.ErrCorrupt, body.ID,
			)
		case shouldBeActive:
			changed = true
			next.lifecycle = StoredCompanionLifecycle{
				ID: body.ID, Active: true, MemoryEpoch: oldLifecycle.MemoryEpoch + 1,
			}
		default:
			changed = true
			next.lifecycle = StoredCompanionLifecycle{
				ID: body.ID, Active: false, MemoryEpoch: oldLifecycle.MemoryEpoch + 1,
			}
			next.pending = pendingIdentityTombstone
		}
		merged = append(merged, next)
	}

	if !changed {
		return cloneStoredCompanions(input), false, nil
	}
	if input.Revision == math.MaxUint64 {
		return StoredCompanions{}, false, fmt.Errorf(
			"%w: companion aggregate revision overflow", storagedef.ErrCorrupt,
		)
	}
	if generate == nil {
		return StoredCompanions{}, false, fmt.Errorf("%w: nil companion identity generator", storagedef.ErrCorrupt)
	}

	namespace := input.AgentNamespaceID
	if !namespace.Valid() {
		namespace, err = generateCanonicalIdentity(generate, "namespace")
		if err != nil {
			return StoredCompanions{}, false, err
		}
	}
	for index := range merged {
		switch merged[index].pending {
		case pendingIdentityMirror:
			identity, err := generateCanonicalIdentity(generate, "memory operation")
			if err != nil {
				return StoredCompanions{}, false, err
			}
			merged[index].lifecycle.MemoryOperationID = identity
		case pendingIdentityTombstone:
			identity, err := generateCanonicalIdentity(generate, "tombstone operation")
			if err != nil {
				return StoredCompanions{}, false, err
			}
			merged[index].lifecycle.TombstoneOperationID = identity
		}
	}

	lifecycles := make([]StoredCompanionLifecycle, len(merged))
	for index := range merged {
		lifecycles[index] = merged[index].lifecycle
	}
	revision := input.Revision + 1
	if input.SourceSchema == 0 {
		revision = 1
	}
	result := StoredCompanions{
		SourceSchema:     companionSchemaV5,
		Revision:         revision,
		AgentNamespaceID: namespace,
		Records:          records,
		Lifecycles:       lifecycles,
		Queues:           queues,
	}
	if _, _, _, err := canonicalV5Parts(CompanionSave{
		Revision:         result.Revision,
		AgentNamespaceID: result.AgentNamespaceID,
		Records:          result.Records,
		Lifecycles:       result.Lifecycles,
		Queues:           result.Queues,
	}); err != nil {
		return StoredCompanions{}, false, err
	}
	return result, true, nil
}

func validateMergeInput(loaded StoredCompanions) (StoredCompanions, error) {
	switch loaded.SourceSchema {
	case 0:
		if loaded.Revision != 0 || loaded.AgentNamespaceID != (Identity{}) ||
			len(loaded.Records) != 0 || len(loaded.Lifecycles) != 0 || len(loaded.Queues) != 0 {
			return StoredCompanions{}, fmt.Errorf("%w: missing companion aggregate has data", storagedef.ErrCorrupt)
		}
		return StoredCompanions{}, nil
	case companionSchemaV1, companionSchemaV2, companionSchemaV3, companionSchemaV4:
		if loaded.Revision == 0 || loaded.AgentNamespaceID != (Identity{}) || len(loaded.Lifecycles) != 0 {
			return StoredCompanions{}, fmt.Errorf("%w: legacy companion metadata invalid", storagedef.ErrCorrupt)
		}
		records, err := canonicalRecords(loaded.Records)
		if err != nil {
			return StoredCompanions{}, err
		}
		if loaded.SourceSchema == companionSchemaV1 && len(loaded.Queues) != 0 {
			return StoredCompanions{}, fmt.Errorf("%w: schema v1 companion queue", storagedef.ErrCorrupt)
		}
		if err := validateStoredCompanionQueues(loaded.Queues, records, loaded.SourceSchema); err != nil {
			return StoredCompanions{}, err
		}
		if loaded.SourceSchema < companionSchemaV4 {
			for _, queue := range loaded.Queues {
				if queue.Summary != "" {
					return StoredCompanions{}, fmt.Errorf("%w: legacy companion summary before v4", storagedef.ErrCorrupt)
				}
			}
		}
		result := cloneStoredCompanions(loaded)
		result.Records = records
		result.Queues = sortStoredCompanionQueues(result.Queues)
		return result, nil
	case companionSchemaV5:
		records, lifecycles, queues, err := canonicalV5Parts(CompanionSave{
			Revision:         loaded.Revision,
			AgentNamespaceID: loaded.AgentNamespaceID,
			Records:          loaded.Records,
			Lifecycles:       loaded.Lifecycles,
			Queues:           loaded.Queues,
		})
		if err != nil {
			return StoredCompanions{}, err
		}
		return StoredCompanions{
			SourceSchema: companionSchemaV5, Revision: loaded.Revision,
			AgentNamespaceID: loaded.AgentNamespaceID,
			Records:          records, Lifecycles: lifecycles, Queues: queues,
		}, nil
	default:
		if loaded.SourceSchema > companionSchemaV5 {
			return StoredCompanions{}, fmt.Errorf("%w: companion schema %d", storagedef.ErrFutureVersion, loaded.SourceSchema)
		}
		return StoredCompanions{}, fmt.Errorf("%w: unsupported companion schema %d", storagedef.ErrCorrupt, loaded.SourceSchema)
	}
}

func validateActiveBodies(active []domain.Body) (map[domain.ID]domain.Body, error) {
	if len(active) > domain.MaxActive {
		return nil, fmt.Errorf("%w: active companion count %d exceeds limit", storagedef.ErrCorrupt, len(active))
	}
	result := make(map[domain.ID]domain.Body, len(active))
	for index, body := range active {
		if err := validateCompanionBody(body); err != nil {
			return nil, fmt.Errorf("active companion %d: %w", index, err)
		}
		if _, duplicate := result[body.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate active companion ID", storagedef.ErrCorrupt)
		}
		result[body.ID] = body
	}
	return result, nil
}

func canonicalV5Parts(save CompanionSave) (
	[]domain.Body,
	[]StoredCompanionLifecycle,
	[]StoredCompanionQueue,
	error,
) {
	if save.Revision == 0 {
		return nil, nil, nil, fmt.Errorf("%w: zero companion revision", storagedef.ErrCorrupt)
	}
	if !save.AgentNamespaceID.Valid() {
		return nil, nil, nil, fmt.Errorf("%w: invalid companion agent namespace", storagedef.ErrCorrupt)
	}
	records, err := canonicalRecords(save.Records)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(save.Lifecycles) != len(records) {
		return nil, nil, nil, fmt.Errorf("%w: companion lifecycle set does not match records", storagedef.ErrCorrupt)
	}
	lifecycles := append([]StoredCompanionLifecycle(nil), save.Lifecycles...)
	slices.SortFunc(lifecycles, func(left, right StoredCompanionLifecycle) int {
		return bytes.Compare(left.ID[:], right.ID[:])
	})
	active := make(map[domain.ID]struct{}, domain.MaxActive)
	for index, lifecycle := range lifecycles {
		if index > 0 && lifecycles[index-1].ID == lifecycle.ID {
			return nil, nil, nil, fmt.Errorf("%w: duplicate companion lifecycle ID", storagedef.ErrCorrupt)
		}
		if lifecycle.ID != records[index].ID {
			return nil, nil, nil, fmt.Errorf("%w: companion lifecycle set does not match records", storagedef.ErrCorrupt)
		}
		if err := validateV5Lifecycle(lifecycle); err != nil {
			return nil, nil, nil, fmt.Errorf("companion lifecycle %d: %w", index, err)
		}
		if lifecycle.Active {
			active[lifecycle.ID] = struct{}{}
		}
	}
	if len(active) > domain.MaxActive {
		return nil, nil, nil, fmt.Errorf("%w: active companion count %d exceeds limit", storagedef.ErrCorrupt, len(active))
	}
	if err := validateStoredCompanionQueues(save.Queues, records, companionSchemaV5); err != nil {
		return nil, nil, nil, err
	}
	queues := sortStoredCompanionQueues(save.Queues)
	for _, queue := range queues {
		if queue.Summary != "" {
			return nil, nil, nil, fmt.Errorf("%w: v5 queue carries legacy summary", storagedef.ErrCorrupt)
		}
		if _, exists := active[queue.ID]; !exists {
			return nil, nil, nil, fmt.Errorf("%w: inactive companion carries task or FIFO", storagedef.ErrCorrupt)
		}
	}
	return records, lifecycles, queues, nil
}

func canonicalRecords(records []domain.Body) ([]domain.Body, error) {
	if len(records) > domain.MaxStored {
		return nil, fmt.Errorf("%w: companion count %d exceeds limit", storagedef.ErrCorrupt, len(records))
	}
	result := append([]domain.Body(nil), records...)
	slices.SortFunc(result, compareCompanionBodies)
	for index, body := range result {
		if err := validateCompanionBody(body); err != nil {
			return nil, fmt.Errorf("companion record %d: %w", index, err)
		}
		if index > 0 && result[index-1].ID == body.ID {
			return nil, fmt.Errorf("%w: duplicate companion ID", storagedef.ErrCorrupt)
		}
	}
	return result, nil
}

func validateV5Lifecycle(lifecycle StoredCompanionLifecycle) error {
	if !lifecycle.ID.Valid() {
		return fmt.Errorf("%w: invalid companion lifecycle ID", storagedef.ErrCorrupt)
	}
	if lifecycle.MemoryEpoch == 0 {
		return fmt.Errorf("%w: zero companion memory epoch", storagedef.ErrCorrupt)
	}
	if lifecycle.Active {
		if lifecycle.TombstoneOperationID != (Identity{}) {
			return fmt.Errorf("%w: active companion carries tombstone", storagedef.ErrCorrupt)
		}
		if lifecycle.MemoryRevision == 0 {
			if lifecycle.MemoryOperationID != (Identity{}) || lifecycle.Summary != "" {
				return fmt.Errorf("%w: invalid canonical-zero companion memory", storagedef.ErrCorrupt)
			}
			return nil
		}
		if !lifecycle.MemoryOperationID.Valid() {
			return fmt.Errorf("%w: invalid companion memory operation", storagedef.ErrCorrupt)
		}
		if err := validateStoredCompanionSummary(lifecycle.Summary); err != nil {
			return err
		}
		return nil
	}
	if lifecycle.MemoryRevision != 0 || lifecycle.MemoryOperationID != (Identity{}) || lifecycle.Summary != "" {
		return fmt.Errorf("%w: inactive companion carries memory mirror", storagedef.ErrCorrupt)
	}
	if !lifecycle.TombstoneOperationID.Valid() {
		return fmt.Errorf("%w: invalid companion tombstone operation", storagedef.ErrCorrupt)
	}
	return nil
}

func generateCanonicalIdentity(generate IdentityGenerator, purpose string) (Identity, error) {
	identity, err := generate()
	if err != nil {
		return Identity{}, fmt.Errorf("generate companion %s: %w", purpose, err)
	}
	if !identity.Valid() {
		return Identity{}, fmt.Errorf("%w: generated companion %s is not UUIDv4", storagedef.ErrCorrupt, purpose)
	}
	return identity, nil
}

func compareCompanionBodies(left, right domain.Body) int {
	return bytes.Compare(left.ID[:], right.ID[:])
}

func sortStoredCompanionQueues(queues []StoredCompanionQueue) []StoredCompanionQueue {
	if queues == nil {
		return nil
	}
	result := make([]StoredCompanionQueue, len(queues))
	for index := range queues {
		result[index] = cloneStoredCompanionQueue(queues[index])
	}
	slices.SortFunc(result, func(left, right StoredCompanionQueue) int {
		return bytes.Compare(left.ID[:], right.ID[:])
	})
	return result
}

func cloneStoredCompanionQueue(queue StoredCompanionQueue) StoredCompanionQueue {
	queue.Current.PlanSteps = slices.Clone(queue.Current.PlanSteps)
	queue.Pending = slices.Clone(queue.Pending)
	return queue
}

func cloneStoredCompanions(stored StoredCompanions) StoredCompanions {
	stored.Records = slices.Clone(stored.Records)
	stored.Lifecycles = slices.Clone(stored.Lifecycles)
	stored.Queues = sortStoredCompanionQueues(stored.Queues)
	return stored
}
