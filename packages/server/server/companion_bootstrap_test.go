package server

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/server/server/persistence"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestNewHostSkipsCompanionStoreWhenAIDisabled(t *testing.T) {
	store := newCompanionBootstrapStore()
	host, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	shutdownCompanionBootstrapHost(t, host)

	probes, loads, saves := store.companionCallCounts()
	if probes != 1 || loads != 0 || saves != 0 {
		t.Fatalf("AI disabled companion calls = probe %d load %d save %d，want 1,0,0", probes, loads, saves)
	}
}

func TestNewHostRestoresConfiguredBodiesAndPreservesInactiveRecords(t *testing.T) {
	activeID := companionBootstrapID(1)
	inactiveID := companionBootstrapID(2)
	active := companionBootstrapBody(activeID, 0.5)
	inactive := companionBootstrapBody(inactiveID, 20.5)
	store := newCompanionBootstrapStore()
	seedCompanionBootstrapStore(t, store, 7, []companion.Body{inactive, active}, activeID)
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: activeID, Name: "阿木"}}

	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	cleanupCompanionBootstrapHost(t, host)
	gotActive := waitForCompanionBootstrapBody(t, host, activeID)
	if gotActive != active {
		t.Fatalf("恢复 active body = %+v，want %+v", gotActive, active)
	}
	records, revision := companionBootstrapRecords(host)
	if revision != 7 || !reflect.DeepEqual(records, []companion.Body{active, inactive}) {
		t.Fatalf("merged records revision=%d records=%+v", revision, records)
	}
}

func TestNewHostAddsConfiguredIDWithoutDeletingInactiveRecords(t *testing.T) {
	configuredID := companionBootstrapID(3)
	inactive := companionBootstrapBody(companionBootstrapID(4), 20.5)
	store := newCompanionBootstrapStore()
	seedCompanionBootstrapStore(t, store, 3, []companion.Body{inactive})
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: configuredID, Name: "阿木"}}

	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	cleanupCompanionBootstrapHost(t, host)
	spawned := waitForCompanionBootstrapBody(t, host, configuredID)
	records, revision := companionBootstrapRecords(host)
	if revision != 4 || len(records) != 2 || !slices.Contains(records, inactive) || !slices.Contains(records, spawned) {
		t.Fatalf("merged records revision=%d records=%+v，want inactive+spawned", revision, records)
	}
}

func TestNewHostRejectsSixtyFifthDistinctStoredOrNewCompanion(t *testing.T) {
	records := make([]companion.Body, companion.MaxStored)
	for index := range records {
		records[index] = companionBootstrapBody(companionBootstrapID(byte(index+1)), float32(index)+0.5)
	}
	store := newCompanionBootstrapStore()
	seedCompanionBootstrapStore(t, store, 9, records)
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(65), Name: "阿木"}}

	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err == nil || host != nil {
		if host != nil {
			cleanupCompanionBootstrapHost(t, host)
		}
		t.Fatalf("NewHost = %v, %v，want 65th rejection", host, err)
	}
	_, loads, saves := store.companionCallCounts()
	if loads != 1 || saves != 0 || store.syncCount() != 0 || store.closeCount() != 0 {
		t.Fatalf("failed constructor calls load/save/sync/close=%d/%d/%d/%d", loads, saves, store.syncCount(), store.closeCount())
	}
}

func TestNewHostAcceptsSixtyFourStoredWhenConfiguredIDAlreadyExists(t *testing.T) {
	records := make([]companion.Body, companion.MaxStored)
	for index := range records {
		records[index] = companionBootstrapBody(companionBootstrapID(byte(index+1)), float32(index)+0.5)
	}
	store := newCompanionBootstrapStore()
	seedCompanionBootstrapStore(t, store, 9, records, records[0].ID)
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: records[0].ID, Name: "阿木"}}

	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	cleanupCompanionBootstrapHost(t, host)
	got, revision := companionBootstrapRecords(host)
	_, loads, saves := store.companionCallCounts()
	if revision != 9 || !reflect.DeepEqual(got, records) {
		t.Fatalf("64 条去重记录 revision=%d records=%+v", revision, got)
	}
	if loads != 1 || saves != 0 || store.syncCount() != 0 || store.closeCount() != 0 {
		t.Fatalf("constructor calls load/save/sync/close=%d/%d/%d/%d", loads, saves, store.syncCount(), store.closeCount())
	}
}

func TestNewHostRejectsCorruptOrFutureCompanionStoreBeforeWorkersStart(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "corrupt", err: storage.ErrCorrupt},
		{name: "future", err: storage.ErrFutureVersion},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newCompanionBootstrapStore()
			store.loadErr = fmt.Errorf("load companions: %w", test.err)
			config := hostTestConfig()
			config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
			host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
			if host != nil || !errors.Is(err, test.err) {
				t.Fatalf("NewHost = %v, %v，want %v", host, err, test.err)
			}
			_, loads, saves := store.companionCallCounts()
			if loads != 1 || saves != 0 || store.syncCount() != 0 || store.closeCount() != 0 {
				t.Fatalf("failed constructor calls load/save/sync/close=%d/%d/%d/%d", loads, saves, store.syncCount(), store.closeCount())
			}
		})
	}
}

func TestNewHostRetiresExistingCompanionsWhenConfigEmpty(t *testing.T) {
	want := companionBootstrapBody(companionBootstrapID(1), 4.5)
	store := newCompanionBootstrapStore()
	seedCompanionBootstrapStore(t, store, 11, []companion.Body{want}, want.ID)
	host, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	shutdownCompanionBootstrapHost(t, host)

	probes, loads, saves := store.companionCallCounts()
	stored, err := store.MemoryStore.LoadCompanions(context.Background())
	if err != nil {
		t.Fatalf("LoadCompanions: %v", err)
	}
	if probes != 1 || loads != 1 || saves != 1 || stored.Revision != 12 ||
		!reflect.DeepEqual(stored.Records, []companion.Body{want}) ||
		len(stored.Lifecycles) != 1 || stored.Lifecycles[0].Active ||
		stored.Lifecycles[0].MemoryEpoch != 2 || !stored.Lifecycles[0].TombstoneOperationID.Valid() {
		t.Fatalf("AI-disabled retirement: calls=%d/%d/%d stored=%+v", probes, loads, saves, stored)
	}
	if host.world.companions != nil || host.world.companionManager != nil {
		t.Fatal("empty config retirement constructed companion runtime")
	}
}

func TestNewHostDoesNotRepeatInactiveRetirement(t *testing.T) {
	id := companionBootstrapID(1)
	body := companionBootstrapBody(id, 4.5)
	store := newCompanionBootstrapStore()
	seedCompanionBootstrapStore(t, store, 11, []companion.Body{body})
	want, err := store.MemoryStore.LoadCompanions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store)
	if err != nil {
		t.Fatal(err)
	}
	shutdownCompanionBootstrapHost(t, host)
	probes, loads, saves := store.companionCallCounts()
	got, err := store.MemoryStore.LoadCompanions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if probes != 1 || loads != 1 || saves != 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("no-op retirement calls=%d/%d/%d got=%+v want=%+v", probes, loads, saves, got, want)
	}
}

func TestNewHostPersistsCompanionIdentityBeforeRuntimeConstruction(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	next := byte(0x80)
	config.companionIdentityGenerator = func() (storage.CompanionIdentity, error) {
		identity := companionBootstrapIdentity(next)
		next++
		return identity, nil
	}
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatal(err)
	}
	cleanupCompanionBootstrapHost(t, host)
	saves := store.companionSaveSnapshot()
	if len(saves) != 1 {
		t.Fatalf("bootstrap saves=%d，想要 1", len(saves))
	}
	save := saves[0]
	metadata := store.Metadata()
	wantBody := companion.Body{
		ID: id, Dimension: metadata.SpawnDimension,
		Position: [3]float32{
			float32(metadata.SpawnAnchor.X*core.SectionSize) + 0.5,
			core.MaxY + 1,
			float32(metadata.SpawnAnchor.Z*core.SectionSize) + 0.5,
		},
	}
	if save.Revision != 1 || save.AgentNamespaceID != companionBootstrapIdentity(0x80) ||
		!reflect.DeepEqual(save.Records, []companion.Body{wantBody}) ||
		len(save.Lifecycles) != 1 || !save.Lifecycles[0].Active ||
		save.Lifecycles[0].MemoryEpoch != 1 || save.Lifecycles[0].MemoryRevision != 0 {
		t.Fatalf("identity-first bootstrap save=%+v", save)
	}
	if store.hostileLoadCount() != 1 || host.world.companions == nil || host.world.companionManager == nil {
		t.Fatal("bootstrap save 未先于后续 runtime construction 完成")
	}
}

func TestNewHostBootstrapSaveFailureStopsBeforeRuntimeConstruction(t *testing.T) {
	store := newCompanionBootstrapStore()
	wantErr := errors.New("identity archive unavailable")
	store.saveErrors = []error{wantErr}
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: companionBootstrapID(1), Name: "阿木"}}
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if host != nil || !errors.Is(err, wantErr) {
		t.Fatalf("NewHost=%v/%v，想要 bootstrap save failure", host, err)
	}
	_, loads, saves := store.companionCallCounts()
	if loads != 1 || saves != 1 || store.hostileLoadCount() != 0 {
		t.Fatalf("failure calls load/save/hostile=%d/%d/%d", loads, saves, store.hostileLoadCount())
	}
}

func TestCompanionNormalStepAutosavesAndRetriesAtTick(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	wantErr := errors.New("companion autosave failed")
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	config.AutosaveTicks = 1
	config.RetryBaseTicks = 2
	config.RetryMaxTicks = 2
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	cleanupCompanionBootstrapHost(t, host)
	bootstrap := receiveCompanionBootstrapSave(t, store)
	if bootstrap.Revision != 1 {
		t.Fatalf("bootstrap save revision=%d，想要 1", bootstrap.Revision)
	}
	store.mu.Lock()
	store.saveErrors = []error{wantErr, nil}
	store.mu.Unlock()

	body, activationTick := stepUntilCompanionBootstrapBody(t, host, id)
	first := receiveCompanionBootstrapSave(t, store)
	if first.Revision != 2 || !reflect.DeepEqual(first.Records, []companion.Body{body}) {
		t.Fatalf("autosave tick=%d save=%+v，want revision 2 body %+v", activationTick, first, body)
	}
	waitForCompanionBootstrapCompletion(t, host.world.companions)
	harvest := host.world.StepForTest()
	if harvest.Tick != activationTick+1 {
		t.Fatalf("failure harvest tick=%d，want %d", harvest.Tick, activationTick+1)
	}

	beforeRetry := host.world.StepForTest()
	if beforeRetry.Tick != harvest.Tick+1 {
		t.Fatalf("before retry tick=%d，want %d", beforeRetry.Tick, harvest.Tick+1)
	}
	assertNoCompanionBootstrapSave(t, store)
	retryTick := host.world.StepForTest()
	retry := receiveCompanionBootstrapSave(t, store)
	if retryTick.Tick != harvest.Tick+2 || !reflect.DeepEqual(retry, first) {
		t.Fatalf("retry tick/save=%d/%+v，want %d/%+v", retryTick.Tick, retry, harvest.Tick+2, first)
	}
}

func TestCompanionShutdownFlushFailureIsRetryable(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	seedCompanionBootstrapStore(t, store, 1, []companion.Body{companionBootstrapBody(id, 0.5)}, id)
	wantErr := errors.New("companion disk full")
	store.saveErrors = []error{wantErr, nil}
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	companions := host.world.companions
	t.Cleanup(companions.Close)
	workerDone := make(chan struct{})
	go func() {
		companions.WaitGroup().Wait()
		close(workerDone)
	}()
	latest := companionBootstrapBody(id, 9.5)
	companions.Observe([]companion.Body{latest}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	err = host.Shutdown(ctx)
	cancel()
	if !errors.Is(err, wantErr) {
		t.Fatalf("first Shutdown error=%v，want %v", err, wantErr)
	}
	if store.syncCount() != 0 || store.closeCount() != 0 {
		t.Fatalf("failed Flush closed resources: sync=%d close=%d", store.syncCount(), store.closeCount())
	}
	assertCompanionBootstrapPersistenceOpen(t, companions, workerDone)
	shutdownCompanionBootstrapHost(t, host)
	assertCompanionBootstrapPersistenceClosed(t, companions, workerDone)
	saves := store.companionSaveSnapshot()
	if len(saves) != 2 || saves[0].Revision != 2 || saves[1].Revision != 2 || !reflect.DeepEqual(saves[1].Records, []companion.Body{latest}) {
		t.Fatalf("retry saves=%+v，want same revision 2 and latest body", saves)
	}
}

func TestCompanionShutdownPersistsBodyCreatedByFinalStepBeforeSync(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	host.world.StepForTest()
	waitForCompanionBootstrapChannel(t, host.world.acquired)
	host.world.StepForTest()
	waitForCompanionBootstrapChannel(t, host.world.generated)
	if bodies := host.world.engine.CompanionBodies(); len(bodies) != 0 {
		t.Fatalf("伙伴在 final step 前已激活：%+v", bodies)
	}

	shutdownCompanionBootstrapHost(t, host)
	saves := store.companionSaveSnapshot()
	if len(saves) != 2 || saves[0].Revision != 1 || saves[1].Revision != 2 ||
		len(saves[1].Records) != 1 || saves[1].Records[0].ID != id {
		t.Fatalf("final-step companion saves=%+v", saves)
	}
	wantEvents := []string{"companion-save", "companion-save", "sync", "close"}
	if events := store.eventsSnapshot(); !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("shutdown events=%v，want %v", events, wantEvents)
	}
}

func TestCompanionShutdownOrdersSaveBeforeStoreSyncAndClose(t *testing.T) {
	id := companionBootstrapID(1)
	store := newCompanionBootstrapStore()
	seedCompanionBootstrapStore(t, store, 1, []companion.Body{companionBootstrapBody(id, 0.5)}, id)
	config := hostTestConfig()
	config.Companions = []companion.Definition{{ID: id, Name: "阿木"}}
	host, err := NewHost(context.Background(), config, flatTestGenerator{}, store)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	host.world.companions.Observe([]companion.Body{companionBootstrapBody(id, 8.5)}, nil)
	shutdownCompanionBootstrapHost(t, host)
	assertCompanionBootstrapEventOrder(t, store.eventsSnapshot())
}

type companionBootstrapStore struct {
	*hostTestStore
	mu               sync.Mutex
	companionProbes  int
	companionLoads   int
	companionSaves   int
	hostileLoads     int
	loadErr          error
	saveErrors       []error
	companionSaveLog []storage.CompanionSave
	saveStarted      chan storage.CompanionSave
}

func (store *companionBootstrapStore) CompanionsExist(ctx context.Context) (bool, error) {
	store.mu.Lock()
	store.companionProbes++
	store.mu.Unlock()
	return store.MemoryStore.CompanionsExist(ctx)
}

func newCompanionBootstrapStore() *companionBootstrapStore {
	return &companionBootstrapStore{
		hostTestStore: newHostTestStore(),
		saveStarted:   make(chan storage.CompanionSave, 16),
	}
}

func (store *companionBootstrapStore) LoadCompanions(ctx context.Context) (storage.StoredCompanions, error) {
	store.mu.Lock()
	store.companionLoads++
	err := store.loadErr
	store.mu.Unlock()
	if err != nil {
		return storage.StoredCompanions{}, err
	}
	return store.MemoryStore.LoadCompanions(ctx)
}

func (store *companionBootstrapStore) SaveCompanions(ctx context.Context, save storage.CompanionSave) error {
	started := cloneCompanionBootstrapSave(save)
	store.mu.Lock()
	store.companionSaves++
	store.companionSaveLog = append(store.companionSaveLog, cloneCompanionBootstrapSave(save))
	var err error
	if len(store.saveErrors) != 0 {
		err = store.saveErrors[0]
		store.saveErrors = store.saveErrors[1:]
	}
	store.mu.Unlock()
	store.saveStarted <- started
	store.hostTestStore.mu.Lock()
	store.hostTestStore.events = append(store.hostTestStore.events, "companion-save")
	store.hostTestStore.mu.Unlock()
	if err != nil {
		return err
	}
	return store.MemoryStore.SaveCompanions(ctx, fixtureServerCompanionV5Save(save))
}

func (store *companionBootstrapStore) LoadHostileMobs(ctx context.Context) (storage.StoredHostileMobs, error) {
	store.mu.Lock()
	store.hostileLoads++
	store.mu.Unlock()
	return store.MemoryStore.LoadHostileMobs(ctx)
}

func (store *companionBootstrapStore) companionCallCounts() (int, int, int) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.companionProbes, store.companionLoads, store.companionSaves
}

func (store *companionBootstrapStore) hostileLoadCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.hostileLoads
}

func (store *companionBootstrapStore) companionSaveSnapshot() []storage.CompanionSave {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]storage.CompanionSave, len(store.companionSaveLog))
	for index, save := range store.companionSaveLog {
		result[index] = cloneCompanionBootstrapSave(save)
	}
	return result
}

func companionBootstrapID(number byte) companion.ID {
	return companion.ID{0, 0, 0, 0, 0, 0, 0x40, 0, 0x80, 0, 0, 0, 0, 0, 0, number}
}

func companionBootstrapBody(id companion.ID, x float32) companion.Body {
	return companion.Body{ID: id, Dimension: core.Overworld, Position: [3]float32{x, 1, 0.5}}
}

func companionBootstrapIdentity(last byte) storage.CompanionIdentity {
	return storage.CompanionIdentity{
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x46, 0x17,
		0x88, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, last,
	}
}

func seedCompanionBootstrapStore(
	t *testing.T,
	store *companionBootstrapStore,
	revision uint64,
	records []companion.Body,
	activeIDs ...companion.ID,
) {
	t.Helper()
	active := make(map[companion.ID]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = struct{}{}
	}
	lifecycles := make([]storage.StoredCompanionLifecycle, len(records))
	for index, body := range records {
		_, isActive := active[body.ID]
		lifecycles[index] = storage.StoredCompanionLifecycle{
			ID: body.ID, Active: isActive, MemoryEpoch: 1,
		}
		if !isActive {
			lifecycles[index].TombstoneOperationID = companionBootstrapIdentity(byte(0xc0 + index))
		}
	}
	if err := store.MemoryStore.SaveCompanions(context.Background(), fixtureServerCompanionV5Save(storage.CompanionSave{
		Revision:         revision,
		AgentNamespaceID: companionBootstrapIdentity(0x70),
		Records:          records,
		Lifecycles:       lifecycles,
	})); err != nil {
		t.Fatalf("seed SaveCompanions: %v", err)
	}
}

func cloneCompanionBootstrapSave(save storage.CompanionSave) storage.CompanionSave {
	save.Records = slices.Clone(save.Records)
	save.Lifecycles = slices.Clone(save.Lifecycles)
	save.Queues = slices.Clone(save.Queues)
	for index := range save.Queues {
		save.Queues[index].Current.PlanSteps = slices.Clone(save.Queues[index].Current.PlanSteps)
		save.Queues[index].Pending = slices.Clone(save.Queues[index].Pending)
	}
	return save
}

// fixtureServerCompanionV5Save 为 server 包历史集成测试的直接种子补齐 v5
// metadata。它只用于测试装配；生产启动迁移始终走 `MergeV5`。
func fixtureServerCompanionV5Save(save storage.CompanionSave) storage.CompanionSave {
	save = cloneCompanionBootstrapSave(save)
	if save.AgentNamespaceID == (storage.CompanionIdentity{}) {
		save.AgentNamespaceID = companionBootstrapIdentity(0x70)
	}
	if save.Lifecycles == nil {
		save.Lifecycles = make([]storage.StoredCompanionLifecycle, len(save.Records))
		for index, body := range save.Records {
			save.Lifecycles[index] = storage.StoredCompanionLifecycle{
				ID: body.ID, Active: true, MemoryEpoch: 1,
			}
		}
	}
	byID := make(map[companion.ID]int, len(save.Lifecycles))
	for index, lifecycle := range save.Lifecycles {
		byID[lifecycle.ID] = index
	}
	for index := range save.Queues {
		if save.Queues[index].Summary == "" {
			continue
		}
		lifecycle := &save.Lifecycles[byID[save.Queues[index].ID]]
		lifecycle.MemoryRevision = 1
		lifecycle.MemoryOperationID = companionBootstrapIdentity(byte(0x90 + index))
		lifecycle.Summary = save.Queues[index].Summary
		save.Queues[index].Summary = ""
	}
	return save
}

func cleanupCompanionBootstrapHost(t *testing.T, host *Host) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		if err := host.Shutdown(ctx); err != nil {
			t.Errorf("Host cleanup Shutdown: %v", err)
		}
	})
}

func waitForCompanionBootstrapBody(t *testing.T, host *Host, id companion.ID) companion.Body {
	t.Helper()
	body, _ := stepUntilCompanionBootstrapBody(t, host, id)
	return body
}

func stepUntilCompanionBootstrapBody(t *testing.T, host *Host, id companion.ID) (companion.Body, uint64) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for time.Now().Before(deadline) {
		result := host.world.StepForTest()
		bodies := host.world.engine.CompanionBodies()
		for _, body := range bodies {
			if body.ID == id {
				return body, result.Tick
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("伙伴 %s 未激活", id)
	return companion.Body{}, 0
}

func receiveCompanionBootstrapSave(t *testing.T, store *companionBootstrapStore) storage.CompanionSave {
	t.Helper()
	select {
	case save := <-store.saveStarted:
		return save
	case <-time.After(shortWaitDeadline):
		t.Fatal("正常 server tick 未启动伙伴保存")
		return storage.CompanionSave{}
	}
}

func assertNoCompanionBootstrapSave(t *testing.T, store *companionBootstrapStore) {
	t.Helper()
	select {
	case save := <-store.saveStarted:
		t.Fatalf("retry deadline 前启动伙伴保存：%+v", save)
	case <-time.After(50 * time.Millisecond):
	}
}

func waitForCompanionBootstrapCompletion(t *testing.T, persistence *persistence.Companions) {
	t.Helper()
	deadline := time.Now().Add(shortWaitDeadline)
	for !persistence.HasPendingCompletion() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !persistence.HasPendingCompletion() {
		t.Fatal("伙伴保存 completion 未返回")
	}
}

func assertCompanionBootstrapPersistenceOpen(
	t *testing.T,
	persistence *persistence.Companions,
	workerDone <-chan struct{},
) {
	t.Helper()
	if persistence.IsClosed() {
		t.Fatal("失败 Shutdown 关闭了伙伴 persistence")
	}
	select {
	case <-persistence.Context().Done():
		t.Fatal("失败 Shutdown 取消了伙伴 worker context")
	default:
	}
	select {
	case <-workerDone:
		t.Fatal("失败 Shutdown 退出了伙伴 worker")
	default:
	}
}

func assertCompanionBootstrapPersistenceClosed(
	t *testing.T,
	persistence *persistence.Companions,
	workerDone <-chan struct{},
) {
	t.Helper()
	if !persistence.IsClosed() {
		t.Fatal("成功 Shutdown 未关闭伙伴 persistence")
	}
	select {
	case <-persistence.Context().Done():
	default:
		t.Fatal("成功 Shutdown 未取消伙伴 worker context")
	}
	select {
	case <-workerDone:
	case <-time.After(shortWaitDeadline):
		t.Fatal("成功 Shutdown 未等待伙伴 worker 退出")
	}
}

func companionBootstrapRecords(host *Host) ([]companion.Body, uint64) {
	return host.world.companions.RecordsAndRevision()
}

func waitForCompanionBootstrapChannel[T any](t *testing.T, channel <-chan T) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for len(channel) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(channel) == 0 {
		t.Fatal("等待伙伴区块 worker 超时")
	}
}

func assertCompanionBootstrapEventOrder(t *testing.T, events []string) {
	t.Helper()
	want := []string{"companion-save", "sync", "close"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("shutdown events=%v，want %v", events, want)
	}
}

func shutdownCompanionBootstrapHost(t *testing.T, host *Host) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := host.Shutdown(ctx); err != nil {
		t.Fatalf("Host Shutdown: %v", err)
	}
}
