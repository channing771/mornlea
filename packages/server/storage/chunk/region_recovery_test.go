package chunk

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"io/fs"
	"math"
	"os"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage/region"
	"github.com/channing771/mornlea/packages/server/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestRegionRecoversOldPayloadAndPromotesRevision(t *testing.T) {
	path, key, chunkKey, oldHash, _ := seededRegion(t)
	r, activeEntry, _, _ := saveActiveRegionRevision(t, path, key, chunkKey, 2)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	flipRegionPayloadByte(t, path, activeEntry, 0)
	wantUnchanged := readRegionBytes(t, path)

	reopened, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Load(context.Background(), chunkKey)
	if err != nil {
		t.Fatal(err)
	}
	if got.Chunk.Hash() != oldHash {
		t.Fatalf("recovered hash = %x, want old hash %x", got.Chunk.Hash(), oldHash)
	}
	if got.Revision != 3 || got.PersistedRevision != 1 || !got.NeedsRewrite || !got.Recovered {
		t.Fatalf("recovered stored chunk = %+v, want revision 3 persisted 1 rewrite/recovered", got)
	}
	if after := readRegionBytes(t, path); !bytes.Equal(after, wantUnchanged) {
		t.Fatal("recovery load modified the region file")
	}

	if _, err := reopened.Save(context.Background(), []ChunkSave{{
		Key: chunkKey, Revision: got.Revision, Chunk: got.Chunk,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	clean, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer clean.Close()
	stored, err := clean.Load(context.Background(), chunkKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Chunk.Hash() != oldHash || stored.Revision != 3 || stored.PersistedRevision != 3 ||
		stored.NeedsRewrite || stored.Recovered {
		t.Fatalf("clean revision-3 load = %+v", stored)
	}
}

func TestRegionRecoversOldPayloadAfterActiveDecodeFailure(t *testing.T) {
	path, key, chunkKey, oldHash, _ := seededRegion(t)
	r, activeEntry, _, activeBank := saveActiveRegionRevision(t, path, key, chunkKey, 2)
	bank := r.bank
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	payload := readRegionPayload(t, path, activeEntry)
	payload[0] ^= 0xff
	writeRegionBytes(t, path, int64(activeEntry.OffsetSector)*region.SectorSize, payload)
	_, slot := region.RegionFor(chunkKey)
	bank.Entries[slot].PayloadCRC32C = crc32.Checksum(payload, region.CRCTable)
	writeRegionBank(t, path, key, activeBank, bank)

	reopened, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Load(context.Background(), chunkKey)
	if err != nil {
		t.Fatal(err)
	}
	if got.Chunk.Hash() != oldHash || got.Revision != 3 || got.PersistedRevision != 1 ||
		!got.NeedsRewrite || !got.Recovered {
		t.Fatalf("decode recovery = %+v", got)
	}
}

func TestRegionRecoversRejectsRevisionOverflowWithoutRewrite(t *testing.T) {
	path, key, chunkKey, _, _ := seededRegion(t)
	r, activeEntry, _, _ := saveActiveRegionRevision(t, path, key, chunkKey, math.MaxUint64)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	flipRegionPayloadByte(t, path, activeEntry, 0)
	wantUnchanged := readRegionBytes(t, path)

	reopened, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.Load(context.Background(), chunkKey); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("max revision recovery error = %v, want %v", err, storagedef.ErrCorrupt)
	}
	if after := readRegionBytes(t, path); !bytes.Equal(after, wantUnchanged) {
		t.Fatal("overflow recovery attempt modified the region file")
	}
}

func TestRegionRecoversRejectsFutureActivePayloadWithoutRewrite(t *testing.T) {
	path, key, chunkKey, _, _ := seededRegion(t)
	r, activeEntry, _, activeBank := saveActiveRegionRevision(t, path, key, chunkKey, 2)
	active := r.bank
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	payload := readRegionPayload(t, path, activeEntry)
	binary.LittleEndian.PutUint32(payload[8:], currentChunkSchema+1)
	writeRegionBytes(t, path, int64(activeEntry.OffsetSector)*region.SectorSize, payload)
	_, slot := region.RegionFor(chunkKey)
	active.Entries[slot].PayloadCRC32C = crc32.Checksum(payload, region.CRCTable)
	writeRegionBank(t, path, key, activeBank, active)
	wantUnchanged := readRegionBytes(t, path)

	reopened, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.Load(context.Background(), chunkKey); !errors.Is(err, storagedef.ErrFutureVersion) {
		t.Fatalf("future active payload error = %v, want %v", err, storagedef.ErrFutureVersion)
	} else if errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("future active payload error = %v, must not be classified corrupt", err)
	}
	if after := readRegionBytes(t, path); !bytes.Equal(after, wantUnchanged) {
		t.Fatal("future active payload load modified the region file")
	}
}

func TestRegionRecoversPropagatesFallbackCancellation(t *testing.T) {
	path, key, chunkKey, _, _ := seededRegion(t)
	r, activeEntry, inactiveEntry, _ := saveActiveRegionRevision(t, path, key, chunkKey, 2)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	flipRegionPayloadByte(t, path, activeEntry, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reopened, err := openRegionWithHooks(
		context.Background(), path, key,
		regionFileHooks{Open: func(name string, flag int, mode fs.FileMode) (File, error) {
			file, err := os.OpenFile(name, flag, mode)
			if err != nil {
				return nil, err
			}
			return &cancelAfterReadRegionFile{
				File:   file,
				offset: int64(inactiveEntry.OffsetSector) * region.SectorSize,
				cancel: cancel,
			}, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.Load(ctx, chunkKey); !errors.Is(err, context.Canceled) {
		t.Fatalf("fallback cancellation error = %v, want %v", err, context.Canceled)
	} else if errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("fallback cancellation error = %v, must not be classified corrupt", err)
	}
}

func TestRegionRecoversClassifiesActivePayloadReadErrors(t *testing.T) {
	path, key, chunkKey, _, _ := seededRegion(t)
	original, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	_, slot := region.RegionFor(chunkKey)
	activeOffset := int64(original.bank.Entries[slot].OffsetSector) * region.SectorSize
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		readErr     error
		wantCorrupt bool
	}{
		{name: "permission", readErr: os.ErrPermission},
		{name: "EOF", readErr: io.EOF, wantCorrupt: true},
		{name: "unexpected EOF", readErr: io.ErrUnexpectedEOF, wantCorrupt: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := openRegionFailingReadAt(t, path, key, activeOffset, tc.readErr)
			defer r.Close()
			_, err := r.Load(context.Background(), chunkKey)
			if !errors.Is(err, tc.readErr) {
				t.Fatalf("active payload read error = %v, want root %v", err, tc.readErr)
			}
			if got := errors.Is(err, storagedef.ErrCorrupt); got != tc.wantCorrupt {
				t.Fatalf("active payload read error corrupt classification = %v, want %v: %v", got, tc.wantCorrupt, err)
			}
		})
	}
}

func TestRegionRecoversPreservesFallbackReadError(t *testing.T) {
	path, key, chunkKey, _, _ := seededRegion(t)
	r, activeEntry, inactiveEntry, _ := saveActiveRegionRevision(t, path, key, chunkKey, 2)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	flipRegionPayloadByte(t, path, activeEntry, 0)

	injected := errors.New("injected fallback payload read error")
	fallbackOffset := int64(inactiveEntry.OffsetSector) * region.SectorSize
	reopened := openRegionFailingReadAt(t, path, key, fallbackOffset, injected)
	defer reopened.Close()
	got, err := reopened.Load(context.Background(), chunkKey)
	if !errors.Is(err, injected) {
		t.Fatalf("fallback payload read error = %v, want root %v", err, injected)
	}
	if !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("fallback payload read error = %v, want active corruption classification", err)
	}
	if got.Chunk != nil || got.Recovered {
		t.Fatalf("fallback read failure returned recovered chunk: %+v", got)
	}
}

func TestRegionRecoversOnlyFromEligibleInactiveEntry(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		key, chunkKey := crashRegionKeys()
		path := t.TempDir() + "/r.0.0.region"
		r, err := CreateRegion(context.Background(), path, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Save(context.Background(), []ChunkSave{changedSave(chunkKey, 2)}); err != nil {
			t.Fatal(err)
		}
		_, slot := region.RegionFor(chunkKey)
		activeEntry := r.bank.Entries[slot]
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		flipRegionPayloadByte(t, path, activeEntry, 0)
		assertRegionLoadCorrupt(t, path, key, chunkKey)
	})

	t.Run("structurally invalid bank", func(t *testing.T) {
		path, key, chunkKey, _, _ := seededRegion(t)
		r, activeEntry, _, _ := saveActiveRegionRevision(t, path, key, chunkKey, 2)
		inactiveBank := 1 - r.activeBank
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		flipRegionPayloadByte(t, path, activeEntry, 0)
		writeRegionBytes(t, path, bankOffset(inactiveBank), []byte{0xff})
		assertRegionLoadCorrupt(t, path, key, chunkKey)
	})

	t.Run("same extent", func(t *testing.T) {
		path, key, chunkKey, _, _ := seededRegion(t)
		r, activeEntry, inactiveEntry, activeBank := saveActiveRegionRevision(t, path, key, chunkKey, 2)
		active := r.bank
		inactive := r.banks[1-activeBank]
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		_, slot := region.RegionFor(chunkKey)
		active.Entries[slot].PayloadCRC32C ^= 1
		inactive.Entries[slot] = activeEntry
		if inactiveEntry == activeEntry {
			t.Fatal("fixture did not create distinct old and active extents")
		}
		writeRegionBank(t, path, key, activeBank, active)
		writeRegionBank(t, path, key, 1-activeBank, inactive)
		assertRegionLoadCorrupt(t, path, key, chunkKey)
	})

	t.Run("not older revision", func(t *testing.T) {
		path, key, chunkKey, _, _ := seededRegion(t)
		r, activeEntry, _, activeBank := saveActiveRegionRevision(t, path, key, chunkKey, 2)
		inactive := r.banks[1-activeBank]
		info, err := r.file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		payload, err := Encode(changedSave(chunkKey, 2))
		if err != nil {
			t.Fatal(err)
		}
		offset := alignSector(info.Size())
		extent := make([]byte, alignSector(int64(len(payload))))
		copy(extent, payload)
		writeRegionBytes(t, path, offset, extent)
		_, slot := region.RegionFor(chunkKey)
		inactive.Entries[slot] = region.Entry{
			OffsetSector:  uint32(offset / region.SectorSize),
			SectorCount:   uint32(len(extent) / region.SectorSize),
			PayloadLength: uint32(len(payload)),
			Revision:      2,
			PayloadCRC32C: crc32.Checksum(payload, region.CRCTable),
		}
		writeRegionBank(t, path, key, 1-activeBank, inactive)
		flipRegionPayloadByte(t, path, activeEntry, 0)
		assertRegionLoadCorrupt(t, path, key, chunkKey)
	})

	t.Run("inactive CRC failure", func(t *testing.T) {
		path, key, chunkKey, _, _ := seededRegion(t)
		r, activeEntry, inactiveEntry, _ := saveActiveRegionRevision(t, path, key, chunkKey, 2)
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		flipRegionPayloadByte(t, path, activeEntry, 0)
		flipRegionPayloadByte(t, path, inactiveEntry, 0)
		assertRegionLoadCorrupt(t, path, key, chunkKey)
	})

	t.Run("inactive decode failure", func(t *testing.T) {
		path, key, chunkKey, _, _ := seededRegion(t)
		r, activeEntry, inactiveEntry, activeBank := saveActiveRegionRevision(t, path, key, chunkKey, 2)
		inactive := r.banks[1-activeBank]
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		flipRegionPayloadByte(t, path, activeEntry, 0)
		payload := readRegionPayload(t, path, inactiveEntry)
		payload[0] ^= 0xff
		writeRegionBytes(t, path, int64(inactiveEntry.OffsetSector)*region.SectorSize, payload)
		_, slot := region.RegionFor(chunkKey)
		inactive.Entries[slot].PayloadCRC32C = crc32.Checksum(payload, region.CRCTable)
		writeRegionBank(t, path, key, 1-activeBank, inactive)
		assertRegionLoadCorrupt(t, path, key, chunkKey)
	})
}

func saveActiveRegionRevision(
	t *testing.T,
	path string,
	key region.RegionKey,
	chunkKey core.ChunkKey,
	revision uint64,
) (*Region, region.Entry, region.Entry, int) {
	t.Helper()
	r, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Save(context.Background(), []ChunkSave{changedSave(chunkKey, revision)}); err != nil {
		r.Close()
		t.Fatal(err)
	}
	_, slot := region.RegionFor(chunkKey)
	return r, r.bank.Entries[slot], r.banks[1-r.activeBank].Entries[slot], r.activeBank
}

func flipRegionPayloadByte(t *testing.T, path string, entry region.Entry, relativeOffset int64) {
	t.Helper()
	offset := int64(entry.OffsetSector)*region.SectorSize + relativeOffset
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var value [1]byte
	if _, err := file.ReadAt(value[:], offset); err != nil {
		t.Fatal(err)
	}
	value[0] ^= 0xff
	if _, err := file.WriteAt(value[:], offset); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
}

func readRegionPayload(t *testing.T, path string, entry region.Entry) []byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	payload := make([]byte, int(entry.PayloadLength))
	if _, err := file.ReadAt(payload, int64(entry.OffsetSector)*region.SectorSize); err != nil {
		t.Fatal(err)
	}
	return payload
}

func readRegionBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeRegionBytes(t *testing.T, path string, offset int64, data []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteAt(data, offset); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
}

func writeRegionBank(t *testing.T, path string, key region.RegionKey, bankIndex int, bank region.Bank) {
	t.Helper()
	encoded, err := region.EncodeRegionBank(key, bank)
	if err != nil {
		t.Fatal(err)
	}
	writeRegionBytes(t, path, bankOffset(bankIndex), encoded[:])
}

func assertRegionLoadCorrupt(t *testing.T, path string, key region.RegionKey, chunkKey core.ChunkKey) {
	t.Helper()
	r, err := OpenRegion(context.Background(), path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.Load(context.Background(), chunkKey); !errors.Is(err, storagedef.ErrCorrupt) {
		t.Fatalf("load error = %v, want %v", err, storagedef.ErrCorrupt)
	}
}

type cancelAfterReadRegionFile struct {
	File
	offset int64
	cancel context.CancelFunc
}

func (f *cancelAfterReadRegionFile) ReadAt(data []byte, offset int64) (int, error) {
	read, err := f.File.ReadAt(data, offset)
	if offset == f.offset {
		f.cancel()
	}
	return read, err
}

type failingReadAtRegionFile struct {
	File
	offset int64
	err    error
}

func (f *failingReadAtRegionFile) ReadAt(data []byte, offset int64) (int, error) {
	if offset == f.offset {
		return 0, f.err
	}
	return f.File.ReadAt(data, offset)
}

func openRegionFailingReadAt(
	t *testing.T,
	path string,
	key region.RegionKey,
	offset int64,
	readErr error,
) *Region {
	t.Helper()
	r, err := openRegionWithHooks(
		context.Background(), path, key,
		regionFileHooks{Open: func(name string, flag int, mode fs.FileMode) (File, error) {
			file, err := os.OpenFile(name, flag, mode)
			if err != nil {
				return nil, err
			}
			return &failingReadAtRegionFile{File: file, offset: offset, err: readErr}, nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
