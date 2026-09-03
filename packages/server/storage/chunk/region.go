package chunk

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/channing771/mornlea/packages/server/storage/region"
	"github.com/channing771/mornlea/packages/server/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

// Region 是单个 region 文件的记录层容器（原根包局部类型 `*region`）：持有
// 当前生效 bank 与双 bank 副本，负责 chunk 载荷的落盘、读取、崩溃恢复回退
// 与整文件压缩替换。根包 DiskStore 以 `map[region.RegionKey]*Region` 缓存编排。
type Region struct {
	mu         sync.RWMutex
	key        region.RegionKey
	path       string
	file       File
	activeBank int
	bank       region.Bank
	banks      [2]region.Bank
	bankValid  [2]bool
	fileHooks  regionFileHooks

	compactionHooks region.CompactionHooks
}

type preparedRegionSave struct {
	save    ChunkSave
	slot    int
	hash    [32]byte
	payload []byte
}

var errRegionPayloadInvalid = errors.New("region payload integrity failure")

func CreateRegion(ctx context.Context, path string, key region.RegionKey) (*Region, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	bankA, err := region.EncodeRegionBank(key, region.Bank{Generation: 1})
	if err != nil {
		return nil, err
	}
	bankB, err := region.EncodeRegionBank(key, region.Bank{})
	if err != nil {
		return nil, err
	}
	header := make([]byte, region.DataStartSector*region.SectorSize)
	superblock := region.EncodeSuperblock(key)
	copy(header, superblock[:])
	copy(header[bankOffset(0):], bankA[:])
	copy(header[bankOffset(1):], bankB[:])

	parent := filepath.Dir(path)
	temporary, err := os.CreateTemp(parent, "."+filepath.Base(path)+".create-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary region %q: %w", path, err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := writeFullAt(temporary, header, 0); err != nil {
		return nil, fmt.Errorf("write temporary region %q: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		return nil, fmt.Errorf("sync temporary region %q: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("close temporary region %q: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return nil, fmt.Errorf("rename temporary region %q: %w", path, err)
	}
	removeTemporary = false
	if err := syncDirectory(parent); err != nil {
		return nil, fmt.Errorf("sync region directory %q: %w", parent, err)
	}

	return OpenRegion(ctx, path, key)
}

func OpenRegion(ctx context.Context, path string, key region.RegionKey) (*Region, error) {
	return openRegionWithHooks(ctx, path, key, regionFileHooks{Open: openRegionFile})
}

func openRegionFile(name string, flag int, mode os.FileMode) (File, error) {
	return os.OpenFile(name, flag, mode)
}

func openRegionWithHooks(
	ctx context.Context,
	path string,
	key region.RegionKey,
	hooks regionFileHooks,
) (*Region, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hooks.Open == nil {
		return nil, fmt.Errorf("open region %q: nil file hook", path)
	}
	file, err := hooks.Open(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open region %q: %w", path, err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = file.Close()
		}
	}()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat region %q: %w", path, err)
	}
	header := make([]byte, region.DataStartSector*region.SectorSize)
	if err := readFullAt(file, header, 0); err != nil {
		return nil, fmt.Errorf("%w: read region header %q: %v", storagedef.ErrCorrupt, path, err)
	}
	if err := region.DecodeSuperblock(key, header[:region.SectorSize]); err != nil {
		return nil, fmt.Errorf("decode region superblock %q: %w", path, err)
	}
	bankA, errA := region.DecodeRegionBank(
		key,
		header[bankOffset(0):bankOffset(0)+region.BankSize],
		info.Size(),
	)
	bankB, errB := region.DecodeRegionBank(
		key,
		header[bankOffset(1):bankOffset(1)+region.BankSize],
		info.Size(),
	)
	bank, activeBank, err := region.SelectRegionBank(bankA, errA, bankB, errB)
	if err != nil {
		return nil, fmt.Errorf("select region bank %q: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	closeOnError = false
	return &Region{
		key:        key,
		path:       path,
		file:       file,
		activeBank: activeBank,
		bank:       bank,
		banks:      [2]region.Bank{bankA, bankB},
		bankValid:  [2]bool{errA == nil, errB == nil},
		fileHooks:  hooks,
	}, nil
}

// File 返回容器当前持有的底层文件；已关闭时为 nil。仅供根包编排测试断言
// 文件所有权（如失败 close 后不残留已消费的描述符）。
func (r *Region) File() File {
	return r.file
}

// ReplaceFile 以给定文件替换容器当前持有的底层文件并返回旧文件。生产路径
// 不经此方法；根包编排测试用它注入 Sync/Close 观察桩以断言跨 region 顺序。
func (r *Region) ReplaceFile(file File) File {
	old := r.file
	r.file = file
	return old
}

// SetCompactionHooks 注入压缩替换路径的故障钩子。生产路径不注入；根包
// 编排测试用它复现压缩中途失败并断言提交结果保留。
func (r *Region) SetCompactionHooks(hooks region.CompactionHooks) {
	r.compactionHooks = hooks
}

// Bank 返回当前生效 bank 的副本，供根包 `ChunkKeys` 编排枚举已落盘槽位，
// 不暴露容器内部状态的可变引用。
func (r *Region) Bank() region.Bank {
	return r.bank
}

func (r *Region) Load(ctx context.Context, key core.ChunkKey) (StoredChunk, error) {
	if err := ctx.Err(); err != nil {
		return StoredChunk{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return StoredChunk{}, err
	}
	if r.file == nil {
		return StoredChunk{}, os.ErrClosed
	}

	stored, err := r.loadLocked(ctx, key)
	if err != nil {
		return StoredChunk{}, err
	}
	stored.Chunk = stored.Chunk.Clone()
	return stored, nil
}

func (r *Region) Save(ctx context.Context, saves []ChunkSave) (SaveResult, error) {
	result := SaveResult{Committed: make(map[core.ChunkKey]uint64, len(saves))}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if r.file == nil {
		return result, os.ErrClosed
	}

	prepared := make([]preparedRegionSave, 0, len(saves))
	for _, save := range saves {
		if err := ValidateChunkSave(save); err != nil {
			return result, err
		}
		regionKey, slot := region.RegionFor(save.Key)
		if regionKey != r.key {
			return result, fmt.Errorf(
				"%w: chunk %v belongs to region %+v, not %+v",
				storagedef.ErrCorrupt, save.Key, regionKey, r.key,
			)
		}
		payload, err := Encode(save)
		if err != nil {
			return result, err
		}
		prepared = append(prepared, preparedRegionSave{
			save: save, slot: slot, hash: save.Chunk.Hash(), payload: payload,
		})
		if err := ctx.Err(); err != nil {
			return result, err
		}
	}
	sort.SliceStable(prepared, func(i, j int) bool {
		return prepared[i].slot < prepared[j].slot
	})

	pending := make(map[core.ChunkKey]preparedRegionSave, len(prepared))
	for _, candidate := range prepared {
		if current, ok := pending[candidate.save.Key]; ok {
			if candidate.save.Revision == current.save.Revision && candidate.hash != current.hash {
				return result, fmt.Errorf(
					"%w: %v revision %d",
					ErrRevisionConflict, candidate.save.Key, candidate.save.Revision,
				)
			}
			if candidate.save.Revision > current.save.Revision {
				pending[candidate.save.Key] = candidate
			}
			continue
		}

		entry := r.bank.Entries[candidate.slot]
		if entry.OffsetSector != 0 {
			if candidate.save.Revision < entry.Revision {
				result.Committed[candidate.save.Key] = entry.Revision
				continue
			}
			if candidate.save.Revision == entry.Revision {
				stored, err := r.loadLocked(ctx, candidate.save.Key)
				if err != nil {
					return result, err
				}
				if candidate.hash != stored.Chunk.Hash() {
					return result, fmt.Errorf(
						"%w: %v revision %d",
						ErrRevisionConflict, candidate.save.Key, candidate.save.Revision,
					)
				}
				result.Committed[candidate.save.Key] = entry.Revision
				continue
			}
		}
		pending[candidate.save.Key] = candidate
	}
	if len(pending) == 0 {
		return result, nil
	}
	if r.bank.Generation == math.MaxUint64 {
		return result, fmt.Errorf("%w: region generation overflow", storagedef.ErrCorrupt)
	}

	ordered := make([]preparedRegionSave, 0, len(pending))
	for _, candidate := range pending {
		ordered = append(ordered, candidate)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].slot < ordered[j].slot })

	info, err := r.file.Stat()
	if err != nil {
		return result, fmt.Errorf("stat region %q: %w", r.path, err)
	}
	fileSize := info.Size()
	free, err := region.FreeSectorExtents(r.bank, fileSize)
	if err != nil {
		return result, err
	}
	appendSector := uint64(fileSize / region.SectorSize)
	if fileSize%region.SectorSize != 0 {
		appendSector++
	}
	if appendSector > math.MaxUint32 {
		return result, fmt.Errorf("%w: region file exceeds uint32 sectors", storagedef.ErrCorrupt)
	}
	next := r.bank
	for _, candidate := range ordered {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		sectorCount := (len(candidate.payload) + region.SectorSize - 1) / region.SectorSize
		allocation, remaining := region.AllocateExtent(free, uint32(sectorCount), uint32(appendSector))
		if allocation.Count == 0 {
			return result, fmt.Errorf("%w: region extent exceeds uint32 sectors", storagedef.ErrCorrupt)
		}
		free = remaining
		endSector := uint64(allocation.First) + uint64(allocation.Count)
		if endSector > math.MaxUint32 {
			return result, fmt.Errorf("%w: region extent exceeds uint32 sectors", storagedef.ErrCorrupt)
		}
		if endSector > appendSector {
			appendSector = endSector
		}
		offset := int64(allocation.First) * region.SectorSize
		extent := make([]byte, sectorCount*region.SectorSize)
		copy(extent, candidate.payload)
		if err := writeFullAt(r.file, extent, offset); err != nil {
			return result, fmt.Errorf("write chunk payload %v: %w", candidate.save.Key, err)
		}
		next.Entries[candidate.slot] = region.Entry{
			OffsetSector:  uint32(offset / region.SectorSize),
			SectorCount:   uint32(sectorCount),
			PayloadLength: uint32(len(candidate.payload)),
			Revision:      candidate.save.Revision,
			PayloadCRC32C: crc32.Checksum(candidate.payload, region.CRCTable),
		}
	}
	if err := r.file.Sync(); err != nil {
		return result, fmt.Errorf("sync payloads: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	next.Generation = r.bank.Generation + 1
	encoded, err := region.EncodeRegionBank(r.key, next)
	if err != nil {
		return result, err
	}
	inactiveBank := 1 - r.activeBank
	if err := writeFullAt(r.file, encoded[:], bankOffset(inactiveBank)); err != nil {
		return result, err
	}
	if err := r.file.Sync(); err != nil {
		return result, fmt.Errorf("sync index bank: %w", err)
	}
	r.bank, r.activeBank = next, inactiveBank
	r.banks[inactiveBank] = next
	r.bankValid[inactiveBank] = true
	for _, candidate := range ordered {
		result.Committed[candidate.save.Key] = candidate.save.Revision
	}
	return result, nil
}

func (r *Region) Sync(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.file == nil {
		return os.ErrClosed
	}
	return r.file.Sync()
}

func (r *Region) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

func (r *Region) loadLocked(ctx context.Context, key core.ChunkKey) (StoredChunk, error) {
	regionKey, slot := region.RegionFor(key)
	if regionKey != r.key {
		return StoredChunk{}, fmt.Errorf(
			"%w: chunk %v belongs to region %+v, not %+v",
			storagedef.ErrCorrupt, key, regionKey, r.key,
		)
	}
	activeEntry := r.bank.Entries[slot]
	if activeEntry.OffsetSector == 0 {
		return StoredChunk{}, fmt.Errorf("%w: %v", ErrChunkNotFound, key)
	}
	active, activeErr := r.loadEntry(ctx, key, slot, activeEntry)
	if activeErr == nil {
		return StoredChunk{
			Key:               key,
			Revision:          active.Revision,
			PersistedRevision: activeEntry.Revision,
			NeedsRewrite:      active.Migrated,
			Chunk:             active.Chunk,
		}, nil
	}
	if !errors.Is(activeErr, errRegionPayloadInvalid) {
		return StoredChunk{}, activeErr
	}

	inactiveBank := 1 - r.activeBank
	inactiveEntry := r.banks[inactiveBank].Entries[slot]
	var old DecodedPayload
	oldErr := fmt.Errorf("%w: inactive bank entry is ineligible", storagedef.ErrCorrupt)
	if r.bankValid[inactiveBank] &&
		inactiveEntry.OffsetSector != 0 &&
		inactiveEntry.Revision < activeEntry.Revision &&
		!regionExtentsOverlap(activeEntry, inactiveEntry) {
		old, oldErr = r.loadEntry(ctx, key, slot, inactiveEntry)
	}
	if err := ctx.Err(); err != nil {
		return StoredChunk{}, err
	}
	if oldErr != nil {
		return StoredChunk{}, fmt.Errorf(
			"%w: active=%w fallback=%w", storagedef.ErrCorrupt, activeErr, oldErr,
		)
	}
	if activeEntry.Revision == math.MaxUint64 {
		return StoredChunk{}, fmt.Errorf(
			"%w: active=%w fallback revision would overflow", storagedef.ErrCorrupt, activeErr,
		)
	}
	return StoredChunk{
		Key:               key,
		Revision:          activeEntry.Revision + 1,
		PersistedRevision: old.Revision,
		NeedsRewrite:      true,
		Recovered:         true,
		Chunk:             old.Chunk,
	}, nil
}

func (r *Region) loadEntry(
	ctx context.Context,
	key core.ChunkKey,
	slot int,
	entry region.Entry,
) (DecodedPayload, error) {
	if err := ctx.Err(); err != nil {
		return DecodedPayload{}, err
	}
	if entry.OffsetSector == 0 {
		return DecodedPayload{}, fmt.Errorf("%w: absent region entry %d", storagedef.ErrCorrupt, slot)
	}
	payload := make([]byte, int(entry.PayloadLength))
	if err := readFullAt(r.file, payload, int64(entry.OffsetSector)*region.SectorSize); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return DecodedPayload{}, fmt.Errorf(
				"%w: read chunk payload %v: %w", storagedef.ErrCorrupt, key, err,
			)
		}
		return DecodedPayload{}, fmt.Errorf("read chunk payload %v: %w", key, err)
	}
	if err := ctx.Err(); err != nil {
		return DecodedPayload{}, err
	}
	if crc32.Checksum(payload, region.CRCTable) != entry.PayloadCRC32C {
		return DecodedPayload{}, fmt.Errorf(
			"%w: %w: chunk payload CRC32C for %v",
			storagedef.ErrCorrupt, errRegionPayloadInvalid, key,
		)
	}
	decoded, err := Decode(key, entry.Revision, payload)
	if err != nil {
		if errors.Is(err, storagedef.ErrFutureVersion) {
			return DecodedPayload{}, err
		}
		return DecodedPayload{}, fmt.Errorf("%w: %w", errRegionPayloadInvalid, err)
	}
	if err := ctx.Err(); err != nil {
		return DecodedPayload{}, err
	}
	return decoded, nil
}

func regionExtentsOverlap(a, b region.Entry) bool {
	aStart := uint64(a.OffsetSector)
	aEnd := aStart + uint64(a.SectorCount)
	bStart := uint64(b.OffsetSector)
	bEnd := bStart + uint64(b.SectorCount)
	return aStart < bEnd && bStart < aEnd
}

func bankOffset(index int) int64 {
	if index == 0 {
		return int64(region.BankAStartSector * region.SectorSize)
	}
	return int64(region.BankBStartSector * region.SectorSize)
}

func alignSector(offset int64) int64 {
	return (offset + region.SectorSize - 1) / region.SectorSize * region.SectorSize
}

func writeFullAt(file File, data []byte, offset int64) error {
	for len(data) > 0 {
		written, err := file.WriteAt(data, offset)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
		offset += int64(written)
	}
	return nil
}

func readFullAt(file File, data []byte, offset int64) error {
	for len(data) > 0 {
		read, err := file.ReadAt(data, offset)
		if read > 0 {
			data = data[read:]
			offset += int64(read)
		}
		if err != nil {
			return err
		}
		if read == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}

// syncDirectory 与根包 world_files.go 的同名助手保持同语义（fsync 目录以
// 固化 rename），因 chunk 包不得反向依赖根包而在此持有一份最小副本。
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
