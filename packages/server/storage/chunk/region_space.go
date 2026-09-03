package chunk

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"

	"github.com/channing771/mornlea/packages/server/storage/region"
	"github.com/channing771/mornlea/packages/server/storage/storagedef"
)

// shouldCompact 按 policy 判定 region 文件浪费是否达到整文件重写的阈值；
// nil 接收者与已关闭容器一律视为不需要压缩。
func (r *Region) shouldCompact(policy region.SpacePolicy) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.file == nil {
		return false
	}
	info, err := r.file.Stat()
	if err != nil {
		return false
	}
	dataSize := info.Size() - int64(region.DataStartSector*region.SectorSize)
	if dataSize <= 0 {
		return false
	}
	var liveSize int64
	for _, entry := range r.bank.Entries {
		liveSize += int64(entry.SectorCount) * region.SectorSize
	}
	if liveSize > dataSize {
		return false
	}
	waste := dataSize - liveSize
	return waste >= policy.MinWaste && float64(waste)/float64(dataSize) >= policy.WasteRatio
}

// ShouldCompact 是 shouldCompact 的导出入口，供根包 DiskStore 在保存批次后
// 决定是否触发压缩。
func (r *Region) ShouldCompact(policy region.SpacePolicy) bool {
	return r.shouldCompact(policy)
}

func (r *Region) writeCompactedFile(ctx context.Context, temporary *os.File) (region.Bank, error) {
	if err := ctx.Err(); err != nil {
		return region.Bank{}, err
	}
	if temporary == nil {
		return region.Bank{}, fmt.Errorf("write compacted region %q: nil temporary file", r.path)
	}

	next := region.Bank{Generation: r.bank.Generation}
	nextSector := uint64(region.DataStartSector)
	for slot, entry := range r.bank.Entries {
		if entry.OffsetSector == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return region.Bank{}, err
		}
		if entry.PayloadLength == 0 {
			return region.Bank{}, fmt.Errorf("%w: region entry %d has empty payload", storagedef.ErrCorrupt, slot)
		}
		payload := make([]byte, int(entry.PayloadLength))
		if err := readFullAt(r.file, payload, int64(entry.OffsetSector)*region.SectorSize); err != nil {
			return region.Bank{}, fmt.Errorf("read compacted region slot %d: %w", slot, err)
		}
		if crc32.Checksum(payload, region.CRCTable) != entry.PayloadCRC32C {
			return region.Bank{}, fmt.Errorf("%w: region entry %d payload CRC32C", storagedef.ErrCorrupt, slot)
		}

		sectorCount := uint64((entry.PayloadLength + region.SectorSize - 1) / region.SectorSize)
		endSector := nextSector + sectorCount
		if sectorCount == 0 || endSector > math.MaxUint32 {
			return region.Bank{}, fmt.Errorf("%w: compacted region extent overflows", storagedef.ErrCorrupt)
		}
		extent := make([]byte, int(sectorCount)*region.SectorSize)
		copy(extent, payload)
		if err := writeFullAt(temporary, extent, int64(nextSector)*region.SectorSize); err != nil {
			return region.Bank{}, fmt.Errorf("write compacted region slot %d: %w", slot, err)
		}
		entry.OffsetSector = uint32(nextSector)
		entry.SectorCount = uint32(sectorCount)
		next.Entries[slot] = entry
		nextSector = endSector
	}

	bankA, err := region.EncodeRegionBank(r.key, next)
	if err != nil {
		return region.Bank{}, err
	}
	bankB, err := region.EncodeRegionBank(r.key, region.Bank{})
	if err != nil {
		return region.Bank{}, err
	}
	header := make([]byte, region.DataStartSector*region.SectorSize)
	superblock := region.EncodeSuperblock(r.key)
	copy(header, superblock[:])
	copy(header[bankOffset(0):], bankA[:])
	copy(header[bankOffset(1):], bankB[:])
	if err := writeFullAt(temporary, header, 0); err != nil {
		return region.Bank{}, fmt.Errorf("write compacted region header: %w", err)
	}
	return next, nil
}

func (r *Region) reopenCanonical() error {
	hooks := r.fileHooks
	if hooks.Open == nil {
		hooks.Open = openRegionFile
	}
	reopened, err := openRegionWithHooks(context.Background(), r.path, r.key, hooks)
	if err != nil {
		return err
	}

	old := r.file
	r.file = reopened.file
	r.activeBank = reopened.activeBank
	r.bank = reopened.bank
	r.banks = reopened.banks
	r.bankValid = reopened.bankValid
	r.fileHooks = hooks
	reopened.file = nil
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (r *Region) compact(ctx context.Context) error {
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

	parent := filepath.Dir(r.path)
	temporary, err := os.CreateTemp(parent, "."+filepath.Base(r.path)+".compact-*")
	if err != nil {
		return fmt.Errorf("create compacted region %q: %w", r.path, err)
	}
	temporaryPath := temporary.Name()
	temporaryOpen := true
	removeTemporary := true
	defer func() {
		if temporaryOpen {
			_ = temporary.Close()
		}
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	next, err := r.writeCompactedFile(ctx, temporary)
	if err != nil {
		return err
	}
	hooks := r.compactionHooks
	if hooks.BeforeTempSync != nil {
		if err := hooks.BeforeTempSync(); err != nil {
			return err
		}
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync compacted region %q: %w", r.path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close compacted region %q: %w", r.path, err)
	}
	temporaryOpen = false
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.file.Sync(); err != nil {
		return fmt.Errorf("sync canonical region %q: %w", r.path, err)
	}
	if err := r.file.Close(); err != nil {
		return errors.Join(
			fmt.Errorf("close canonical region %q: %w", r.path, err),
			r.reopenCanonical(),
		)
	}
	r.file = nil

	rename := hooks.Rename
	if rename == nil {
		rename = os.Rename
	}
	if err := rename(temporaryPath, r.path); err != nil {
		return errors.Join(
			fmt.Errorf("replace canonical region %q: %w", r.path, err),
			r.reopenCanonical(),
		)
	}
	removeTemporary = false

	syncDir := hooks.SyncDirectory
	if syncDir == nil {
		syncDir = syncDirectory
	}
	if err := syncDir(parent); err != nil {
		return errors.Join(
			fmt.Errorf("sync compacted region directory %q: %w", parent, err),
			r.reopenCanonical(),
		)
	}
	if err := r.reopenCanonical(); err != nil {
		return err
	}
	if r.activeBank != 0 || r.bank != next {
		return fmt.Errorf("%w: reopened compacted region does not match replacement", storagedef.ErrCorrupt)
	}
	r.bank, r.activeBank = next, 0
	return nil
}

// Compact 是 compact 的导出入口，供根包 DiskStore 在保存批次后执行整文件
// 压缩替换。
func (r *Region) Compact(ctx context.Context) error {
	return r.compact(ctx)
}
