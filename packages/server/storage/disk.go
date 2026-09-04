package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/channing771/mornlea/packages/server/storage/chunk"
	"github.com/channing771/mornlea/packages/server/storage/companion"
	"github.com/channing771/mornlea/packages/server/storage/hostile"
	"github.com/channing771/mornlea/packages/server/storage/passive"
	"github.com/channing771/mornlea/packages/server/storage/player"
	"github.com/channing771/mornlea/packages/server/storage/region"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

const maxPlayerFileLength = int64(player.EnvelopeLength) + int64(player.MaxPayload)

// DiskStore persists chunks in lazily opened region files under one locked world.
type DiskStore struct {
	mu      sync.Mutex
	files   *worldFiles
	regions map[RegionKey]*chunk.Region
	closing atomic.Bool
	closed  bool

	playerReplaceHooks    atomicReplaceHooks
	companionReplaceHooks atomicReplaceHooks
	metadataReplaceHooks  atomicReplaceHooks
	hostileReplaceHooks   atomicReplaceHooks
	passiveReplaceHooks   atomicReplaceHooks
}

func OpenDisk(ctx context.Context, root string, options OpenOptions) (*DiskStore, error) {
	files, err := openWorldFiles(ctx, root, options)
	if err != nil {
		return nil, err
	}
	return &DiskStore{
		files:   files,
		regions: make(map[RegionKey]*chunk.Region),
	}, nil
}

func (store *DiskStore) Metadata() Metadata {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.files.metadata
}

// SaveMetadata 把一份 metadata 快照原子写入 world.meta。
// 失败时磁盘上保留完整旧版，内存中的值也不会前进。
func (store *DiskStore) SaveMetadata(ctx context.Context, metadata Metadata) error {
	if store.closing.Load() {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := encodeMetadata(metadata)
	if err != nil {
		return fmt.Errorf("encode world metadata: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hooks := store.metadataReplaceHooks
	hooks.beforeRename = ctx.Err
	path := filepath.Join(store.files.root, "world.meta")
	if err := replaceFileAtomicallyWithPatternAndHooks(
		path, ".world.meta.tmp-*", encoded, 0o600, hooks,
	); err != nil {
		return fmt.Errorf("save world metadata %q: %w", path, err)
	}
	store.files.metadata = metadata
	return nil
}

func (store *DiskStore) LoadChunk(
	ctx context.Context,
	key core.ChunkKey,
) (StoredChunk, error) {
	if store.closing.Load() {
		return StoredChunk{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredChunk{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return StoredChunk{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredChunk{}, err
	}

	regionKey, _ := RegionFor(key)
	opened, ok := store.regions[regionKey]
	if !ok {
		var err error
		opened, err = chunk.OpenRegion(ctx, store.regionPath(regionKey), regionKey)
		if errors.Is(err, os.ErrNotExist) {
			return StoredChunk{}, fmt.Errorf("%w: %v", ErrChunkNotFound, key)
		}
		if err != nil {
			return StoredChunk{}, err
		}
		store.regions[regionKey] = opened
	}
	return opened.Load(ctx, key)
}

func (store *DiskStore) SaveBatch(
	ctx context.Context,
	saves []ChunkSave,
) (SaveResult, error) {
	result := SaveResult{Committed: make(map[core.ChunkKey]uint64, len(saves))}
	if store.closing.Load() {
		return result, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	saves, err := validateAndNormalizeSaves(saves)
	if err != nil {
		return result, err
	}

	grouped := make(map[RegionKey][]ChunkSave)
	for _, save := range saves {
		key, _ := RegionFor(save.Key)
		grouped[key] = append(grouped[key], save)
	}
	keys := make([]RegionKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sortRegionKeys(keys)
	if store.closing.Load() {
		return result, os.ErrClosed
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return result, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		opened, err := store.regionForSave(ctx, key)
		if err != nil {
			return result, err
		}
		regionResult, err := opened.Save(ctx, grouped[key])
		for chunkKey, revision := range regionResult.Committed {
			result.Committed[chunkKey] = revision
		}
		if err != nil {
			return result, fmt.Errorf("save region %+v: %w", key, err)
		}
		if opened.ShouldCompact(region.ProductionSpacePolicy) {
			if err := opened.Compact(ctx); err != nil {
				return result, fmt.Errorf("compact region %+v: %w", key, err)
			}
		}
	}
	return result, nil
}

func (store *DiskStore) LoadPlayer(
	ctx context.Context,
	id core.PlayerID,
) (StoredPlayer, error) {
	if store.closing.Load() {
		return StoredPlayer{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredPlayer{}, err
	}
	if !id.Valid() {
		return StoredPlayer{}, fmt.Errorf("%w: invalid requested player ID", ErrCorrupt)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return StoredPlayer{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredPlayer{}, err
	}
	encoded, err := readPlayerFile(store.playerPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return StoredPlayer{}, fmt.Errorf("%w: %s", ErrPlayerNotFound, id)
	}
	if err != nil {
		return StoredPlayer{}, fmt.Errorf("read player %s: %w", id, err)
	}
	return player.Decode(id, encoded)
}

func (store *DiskStore) SavePlayer(
	ctx context.Context,
	save PlayerSave,
) (uint64, error) {
	if store.closing.Load() {
		return 0, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	encoded, err := player.Encode(save)
	if err != nil {
		return 0, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return 0, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	path := store.playerPath(save.PlayerID)
	previous, err := readPlayerFile(path)
	switch {
	case err == nil:
		stored, decodeErr := player.Decode(save.PlayerID, previous)
		if decodeErr != nil {
			return 0, fmt.Errorf("read existing player %s: %w", save.PlayerID, decodeErr)
		}
		switch {
		case save.Revision < stored.Revision:
			return stored.Revision, fmt.Errorf(
				"%w: player %s revision %d is below %d",
				ErrRevisionConflict, save.PlayerID, save.Revision, stored.Revision,
			)
		case save.Revision == stored.Revision:
			if !bytes.Equal(encoded, previous) {
				return stored.Revision, fmt.Errorf(
					"%w: player %s revision %d",
					ErrRevisionConflict, save.PlayerID, save.Revision,
				)
			}
			return stored.Revision, nil
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return 0, fmt.Errorf("read existing player %s: %w", save.PlayerID, err)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	pattern := "." + save.PlayerID.String() + ".player.tmp-*"
	hooks := store.playerReplaceHooks
	hooks.beforeRename = ctx.Err
	if err := replaceFileAtomicallyWithPatternAndHooks(
		path, pattern, encoded, 0o600, hooks,
	); err != nil {
		return 0, fmt.Errorf("save player %s: %w", save.PlayerID, err)
	}
	return save.Revision, nil
}

func (store *DiskStore) LoadCompanions(ctx context.Context) (StoredCompanions, error) {
	if store.closing.Load() {
		return StoredCompanions{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredCompanions{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return StoredCompanions{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredCompanions{}, err
	}
	encoded, err := readCompanionFile(store.companionPath())
	if errors.Is(err, os.ErrNotExist) {
		return StoredCompanions{}, ErrCompanionsNotFound
	}
	if err != nil {
		return StoredCompanions{}, fmt.Errorf("read companions: %w", err)
	}
	stored, err := companion.Decode(encoded)
	if err != nil {
		return StoredCompanions{}, fmt.Errorf("decode companions: %w", err)
	}
	return stored, nil
}

// CompanionsExist 只对固定 `companions.ai` 路径执行 metadata stat，不读取或
// 解码正文。
func (store *DiskStore) CompanionsExist(ctx context.Context) (bool, error) {
	if store.closing.Load() {
		return false, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return false, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, err := os.Stat(store.companionPath())
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat companions: %w", err)
	}
}

func (store *DiskStore) SaveCompanions(ctx context.Context, save CompanionSave) error {
	if store.closing.Load() {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := companion.Encode(save)
	if err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path := store.companionPath()
	previous, err := readCompanionFile(path)
	switch {
	case err == nil:
		stored, decodeErr := companion.Decode(previous)
		if decodeErr != nil {
			return fmt.Errorf("read existing companions: %w", decodeErr)
		}
		switch {
		case save.Revision < stored.Revision:
			return fmt.Errorf(
				"%w: companion revision %d is below %d",
				ErrRevisionConflict, save.Revision, stored.Revision,
			)
		case save.Revision == stored.Revision:
			if !bytes.Equal(encoded, previous) {
				return fmt.Errorf("%w: companion revision %d", ErrRevisionConflict, save.Revision)
			}
			return nil
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("read existing companions: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hooks := store.companionReplaceHooks
	hooks.beforeRename = ctx.Err
	if err := replaceFileAtomicallyWithPatternAndHooks(
		path, ".companions.ai.tmp-*", encoded, 0o600, hooks,
	); err != nil {
		return fmt.Errorf("save companions: %w", err)
	}
	return nil
}

func (store *DiskStore) LoadHostileMobs(ctx context.Context) (StoredHostileMobs, error) {
	if store.closing.Load() {
		return StoredHostileMobs{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredHostileMobs{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return StoredHostileMobs{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredHostileMobs{}, err
	}
	encoded, err := readHostileFile(store.hostilePath())
	if errors.Is(err, os.ErrNotExist) {
		return StoredHostileMobs{}, ErrHostileMobsNotFound
	}
	if err != nil {
		return StoredHostileMobs{}, fmt.Errorf("read hostile mobs: %w", err)
	}
	stored, err := hostile.Decode(encoded)
	if err != nil {
		return StoredHostileMobs{}, fmt.Errorf("decode hostile mobs: %w", err)
	}
	return stored, nil
}

func (store *DiskStore) SaveHostileMobs(ctx context.Context, save HostileMobsSave) error {
	if store.closing.Load() {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := hostile.Encode(save)
	if err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path := store.hostilePath()
	previous, err := readHostileFile(path)
	switch {
	case err == nil:
		stored, decodeErr := hostile.Decode(previous)
		if decodeErr != nil {
			// 正式文件损坏或为未来版本时拒绝保存并保留原文件：覆盖等于把
			// 「读不回的数据」洗成合法存档，重启会静默清怪。
			return fmt.Errorf("read existing hostile mobs: %w", decodeErr)
		}
		switch {
		case save.Revision < stored.Revision:
			return fmt.Errorf(
				"%w: hostile revision %d is below %d",
				ErrRevisionConflict, save.Revision, stored.Revision,
			)
		case save.Revision == stored.Revision:
			if !bytes.Equal(encoded, previous) {
				return fmt.Errorf("%w: hostile revision %d", ErrRevisionConflict, save.Revision)
			}
			return nil
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("read existing hostile mobs: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hooks := store.hostileReplaceHooks
	hooks.beforeRename = ctx.Err
	if err := replaceFileAtomicallyWithPatternAndHooks(
		path, ".hostile_mobs.bin.tmp-*", encoded, 0o600, hooks,
	); err != nil {
		return fmt.Errorf("save hostile mobs: %w", err)
	}
	return nil
}

func (store *DiskStore) LoadPassiveMobs(ctx context.Context) (StoredPassiveMobs, error) {
	if store.closing.Load() {
		return StoredPassiveMobs{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredPassiveMobs{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return StoredPassiveMobs{}, os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return StoredPassiveMobs{}, err
	}
	encoded, err := readPassiveFile(store.passivePath())
	if errors.Is(err, os.ErrNotExist) {
		return StoredPassiveMobs{}, ErrPassiveMobsNotFound
	}
	if err != nil {
		return StoredPassiveMobs{}, fmt.Errorf("read passive mobs: %w", err)
	}
	stored, err := passive.Decode(encoded)
	if err != nil {
		return StoredPassiveMobs{}, fmt.Errorf("decode passive mobs: %w", err)
	}
	return stored, nil
}

func (store *DiskStore) SavePassiveMobs(ctx context.Context, save PassiveMobsSave) error {
	if store.closing.Load() {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := passive.Encode(save)
	if err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return os.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path := store.passivePath()
	previous, err := readPassiveFile(path)
	switch {
	case err == nil:
		stored, decodeErr := passive.Decode(previous)
		if decodeErr != nil {
			// 正式文件损坏或为未来版本时拒绝保存并保留原文件：覆盖等于把
			// 「读不回的数据」洗成合法存档，重启会静默清牛。
			return fmt.Errorf("read existing passive mobs: %w", decodeErr)
		}
		switch {
		case save.Revision < stored.Revision:
			return fmt.Errorf(
				"%w: passive revision %d is below %d",
				ErrRevisionConflict, save.Revision, stored.Revision,
			)
		case save.Revision == stored.Revision:
			if !bytes.Equal(encoded, previous) {
				return fmt.Errorf("%w: passive revision %d", ErrRevisionConflict, save.Revision)
			}
			return nil
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("read existing passive mobs: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hooks := store.passiveReplaceHooks
	hooks.beforeRename = ctx.Err
	if err := replaceFileAtomicallyWithPatternAndHooks(
		path, ".passive_mobs.bin.tmp-*", encoded, 0o600, hooks,
	); err != nil {
		return fmt.Errorf("save passive mobs: %w", err)
	}
	return nil
}

// hashChunkFunc 是批量保存去重比对的内容哈希入口。生产路径固定为
// `world.Chunk.Hash`；探针测试注入计数包装，钉住「同批次同区块只哈希一次」。
// 返回值 `[32]byte` 即 `sha256.Size` 字节的 SHA-256 摘要（与 memory.go 的
// revision 记账同形），不为此引入 crypto/sha256 import。
type hashChunkFunc func(*world.Chunk) [32]byte

func validateAndNormalizeSaves(saves []ChunkSave) ([]ChunkSave, error) {
	return validateAndNormalizeSavesWithHash(saves, (*world.Chunk).Hash)
}

// validateAndNormalizeSavesWithHash 校验整批保存并按键收敛到最高 revision 的
// 单一保存：同键同 revision 的多个候选以内容哈希判一致，内容不同即整批拒绝。
// 内容哈希经 hashChunk 计算并按区块指针在整批内复用；单候选键没有比对需求，
// 不产生哈希计算——这是保存热路径上的纯开销消除，不改变任何接受/拒绝结果。
func validateAndNormalizeSavesWithHash(saves []ChunkSave, hashChunk hashChunkFunc) ([]ChunkSave, error) {
	maxRevisions := make(map[core.ChunkKey]uint64, len(saves))
	for _, save := range saves {
		if err := chunk.ValidateChunkSave(save); err != nil {
			return nil, err
		}
		if save.Revision > maxRevisions[save.Key] {
			maxRevisions[save.Key] = save.Revision
		}
	}

	candidates := make(map[core.ChunkKey][]ChunkSave, len(maxRevisions))
	for _, save := range saves {
		if save.Revision == maxRevisions[save.Key] {
			candidates[save.Key] = append(candidates[save.Key], save)
		}
	}
	keys := make([]core.ChunkKey, 0, len(maxRevisions))
	for key := range maxRevisions {
		keys = append(keys, key)
	}
	sortChunkKeys(keys)

	// 哈希缓存按区块指针复用：合法批次里同一指针只可能出现在同一键下，
	// 但整批一张表更简单；首个需要比对的键才建立，纯单候选批次零开销。
	var hashCache map[*world.Chunk][32]byte
	hashOf := func(c *world.Chunk) [32]byte {
		if cached, ok := hashCache[c]; ok {
			return cached
		}
		computed := hashChunk(c)
		if hashCache == nil {
			hashCache = make(map[*world.Chunk][32]byte)
		}
		hashCache[c] = computed
		return computed
	}

	normalized := make([]ChunkSave, 0, len(keys))
	for _, key := range keys {
		keyCandidates := candidates[key]
		selected := keyCandidates[0]
		if len(keyCandidates) > 1 {
			selectedHash := hashOf(selected.Chunk)
			for _, candidate := range keyCandidates[1:] {
				if hashOf(candidate.Chunk) != selectedHash {
					return nil, fmt.Errorf(
						"%w: %v revision %d", ErrRevisionConflict, key, selected.Revision,
					)
				}
			}
		}
		normalized = append(normalized, selected)
	}
	return normalized, nil
}

func sortChunkKeys(keys []core.ChunkKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Dimension != keys[j].Dimension {
			return keys[i].Dimension < keys[j].Dimension
		}
		if keys[i].Pos.X != keys[j].Pos.X {
			return keys[i].Pos.X < keys[j].Pos.X
		}
		return keys[i].Pos.Z < keys[j].Pos.Z
	})
}

func (store *DiskStore) Sync(ctx context.Context) error {
	if store.closing.Load() {
		return os.ErrClosed
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closing.Load() || store.closed {
		return os.ErrClosed
	}

	keys := store.regionKeys()
	errs := make([]error, 0, len(keys))
	for _, key := range keys {
		if err := store.regions[key].Sync(ctx); err != nil {
			errs = append(errs, fmt.Errorf("sync region %+v: %w", key, err))
		}
	}
	return errors.Join(errs...)
}

func (store *DiskStore) Close() error {
	if store == nil {
		return nil
	}
	store.closing.Store(true)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.closed = true

	keys := store.regionKeys()
	errs := make([]error, 0, len(keys))
	for _, key := range keys {
		if err := store.regions[key].Close(); err != nil {
			errs = append(errs, fmt.Errorf("close region %+v: %w", key, err))
			continue
		}
		delete(store.regions, key)
	}
	if len(errs) != 0 {
		return errors.Join(errs...)
	}
	return store.files.close()
}

func (store *DiskStore) regionPath(key RegionKey) string {
	return filepath.Join(
		store.files.root,
		"dimensions", strconv.FormatInt(int64(key.Dimension), 10),
		"regions", fmt.Sprintf("r.%d.%d.region", key.X, key.Z),
	)
}

func (store *DiskStore) playerPath(id core.PlayerID) string {
	return filepath.Join(store.files.root, "players", id.String()+".player")
}

func (store *DiskStore) companionPath() string {
	return filepath.Join(store.files.root, "companions.ai")
}

// hostilePath 是夜行者聚合存档的固定路径，与世界根目录平级（与
// companions.ai 同一布局约定）。
func (store *DiskStore) hostilePath() string {
	return filepath.Join(store.files.root, "hostile_mobs.bin")
}

// passivePath 是被动牛聚合存档的固定路径，与世界根目录平级（与
// hostile_mobs.bin 同一布局约定，文件独立）。
func (store *DiskStore) passivePath() string {
	return filepath.Join(store.files.root, "passive_mobs.bin")
}

func readPlayerFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	encoded, readErr := io.ReadAll(io.LimitReader(file, maxPlayerFileLength+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if int64(len(encoded)) > maxPlayerFileLength {
		return nil, fmt.Errorf(
			"%w: player file exceeds %d bytes", ErrCorrupt, maxPlayerFileLength,
		)
	}
	return encoded, nil
}

func readCompanionFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	encoded, readErr := io.ReadAll(io.LimitReader(file, int64(companion.MaxFileLength)+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	if len(encoded) > companion.MaxFileLength {
		return nil, fmt.Errorf(
			"%w: companion file exceeds %d bytes", ErrCorrupt, companion.MaxFileLength,
		)
	}
	return encoded, nil
}

// readHostileFile 读取夜行者存档并守住物理字节上界：先按上界截断读取再
// 拒绝超限输入，保证超大文件不会在解码前进入内存。
func readHostileFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	encoded, readErr := io.ReadAll(io.LimitReader(file, int64(hostile.MaxFileLength)+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	if len(encoded) > hostile.MaxFileLength {
		return nil, fmt.Errorf(
			"%w: hostile file exceeds %d bytes", ErrCorrupt, hostile.MaxFileLength,
		)
	}
	return encoded, nil
}

// readPassiveFile 读取被动牛存档并守住物理字节上界：先按上界截断读取再
// 拒绝超限输入，保证超大文件不会在解码前进入内存。
func readPassiveFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	encoded, readErr := io.ReadAll(io.LimitReader(file, int64(passive.MaxFileLength)+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return nil, err
	}
	if len(encoded) > passive.MaxFileLength {
		return nil, fmt.Errorf(
			"%w: passive file exceeds %d bytes", ErrCorrupt, passive.MaxFileLength,
		)
	}
	return encoded, nil
}

func (store *DiskStore) regionForSave(ctx context.Context, key RegionKey) (*chunk.Region, error) {
	if opened, ok := store.regions[key]; ok {
		return opened, nil
	}
	path := store.regionPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create region directory %q: %w", filepath.Dir(path), err)
	}
	opened, err := chunk.OpenRegion(ctx, path, key)
	if errors.Is(err, os.ErrNotExist) {
		opened, err = chunk.CreateRegion(ctx, path, key)
	}
	if err != nil {
		return nil, err
	}
	store.regions[key] = opened
	return opened, nil
}

func (store *DiskStore) regionKeys() []RegionKey {
	keys := make([]RegionKey, 0, len(store.regions))
	for key := range store.regions {
		keys = append(keys, key)
	}
	sortRegionKeys(keys)
	return keys
}

func sortRegionKeys(keys []RegionKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Dimension != keys[j].Dimension {
			return keys[i].Dimension < keys[j].Dimension
		}
		if keys[i].X != keys[j].X {
			return keys[i].X < keys[j].X
		}
		return keys[i].Z < keys[j].Z
	})
}

var _ WorldStore = (*DiskStore)(nil)
