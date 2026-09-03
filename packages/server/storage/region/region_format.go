package region

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
	"sort"

	"github.com/channing771/mornlea/packages/server/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// SectorSize 是 region 文件的扇区粒度，所有 extent 偏移与长度都以它对齐。
	SectorSize  = 4096
	bankSectors = 7
	// DataStartSector 是固定头部（superblock 与两个 bank）之后的第一个数据扇区。
	DataStartSector = 15
	regionSlots     = 32 * 32
	// MaxCompressedChunk 是单个 region entry 允许的最大压缩 chunk 载荷字节数。
	// 它定义在 region 包而非 chunk 包：validateRegionBank 在解码时按它拒绝
	// 超限 entry，而 region 包不得反向依赖 chunk 包。
	MaxCompressedChunk          = 1 << 20
	currentRegionVersion uint32 = 1

	BankAStartSector = 1
	BankBStartSector = 8

	BankSize             = bankSectors * SectorSize
	regionBankHeaderSize = 64
	regionEntrySize      = 24

	superMagicOffset       = 0
	superVersionOffset     = 4
	superSectorSizeOffset  = 8
	superDimensionOffset   = 12
	superRegionXOffset     = 16
	superRegionZOffset     = 20
	superBankAStartOffset  = 24
	superBankBStartOffset  = 28
	superBankSectorsOffset = 32
	superDataStartOffset   = 36
	superReservedOffset    = 40
	superCRCOffset         = SectorSize - 4

	bankMagicOffset      = 0
	bankVersionOffset    = 4
	bankSectorSizeOffset = 8
	bankDimensionOffset  = 12
	bankRegionXOffset    = 16
	bankRegionZOffset    = 20
	bankGenerationOffset = 24
	bankEntryCountOffset = 32
	bankEntrySizeOffset  = 36
	bankSectorsOffset    = 40
	bankDataStartOffset  = 44
	bankReservedOffset   = 48
	bankCRCOffset        = regionBankHeaderSize - 4
	bankEntriesOffset    = regionBankHeaderSize
	bankPaddingOffset    = bankEntriesOffset + regionSlots*regionEntrySize

	entryOffsetSectorOffset  = 0
	entrySectorCountOffset   = 4
	entryPayloadLengthOffset = 8
	entryRevisionOffset      = 12
	entryPayloadCRCOffset    = 20
)

var (
	regionSuperblockMagic = [4]byte{'M', 'C', 'G', 'R'}
	regionBankMagic       = [4]byte{'M', 'C', 'G', 'B'}
	// CRCTable 是 region 格式约定的 Castagnoli CRC32 表，chunk 载荷校验与
	// bank 校验共用同一张表。
	CRCTable = crc32.MakeTable(crc32.Castagnoli)
)

// Entry 是 region bank 内单个区块槽位的落盘索引：载荷起始扇区、扇区数、
// 有效字节数、修订号与载荷 CRC32C。OffsetSector 为零表示槽位为空。
type Entry struct {
	OffsetSector  uint32
	SectorCount   uint32
	PayloadLength uint32
	Revision      uint64
	PayloadCRC32C uint32
}

// Bank 是 region 文件的双 bank 索引之一：Generation 递增用于崩溃后选择
// 最新已提交的索引副本。
type Bank struct {
	Generation uint64
	Entries    [regionSlots]Entry
}

type sectorRange struct {
	first uint64
	end   uint64
}

func EncodeSuperblock(key RegionKey) [SectorSize]byte {
	var encoded [SectorSize]byte
	copy(encoded[superMagicOffset:], regionSuperblockMagic[:])
	putRegionU32(encoded[:], superVersionOffset, currentRegionVersion)
	putRegionU32(encoded[:], superSectorSizeOffset, SectorSize)
	putRegionU32(encoded[:], superDimensionOffset, uint32(key.Dimension))
	putRegionU32(encoded[:], superRegionXOffset, uint32(key.X))
	putRegionU32(encoded[:], superRegionZOffset, uint32(key.Z))
	putRegionU32(encoded[:], superBankAStartOffset, BankAStartSector)
	putRegionU32(encoded[:], superBankBStartOffset, BankBStartSector)
	putRegionU32(encoded[:], superBankSectorsOffset, bankSectors)
	putRegionU32(encoded[:], superDataStartOffset, DataStartSector)
	putRegionU32(
		encoded[:], superCRCOffset,
		crc32.Checksum(encoded[:superCRCOffset], CRCTable),
	)
	return encoded
}

func DecodeSuperblock(key RegionKey, encoded []byte) error {
	if len(encoded) != SectorSize {
		return fmt.Errorf("%w: superblock length %d, want %d", storagedef.ErrCorrupt, len(encoded), SectorSize)
	}
	if !bytes.Equal(encoded[superMagicOffset:superVersionOffset], regionSuperblockMagic[:]) {
		return fmt.Errorf("%w: superblock magic", storagedef.ErrCorrupt)
	}
	version := regionU32(encoded, superVersionOffset)
	if version > currentRegionVersion {
		return fmt.Errorf("%w: region version %d", storagedef.ErrFutureVersion, version)
	}
	if version != currentRegionVersion {
		return fmt.Errorf("%w: unsupported region version %d", storagedef.ErrCorrupt, version)
	}
	wantCRC := regionU32(encoded, superCRCOffset)
	gotCRC := crc32.Checksum(encoded[:superCRCOffset], CRCTable)
	if gotCRC != wantCRC {
		return fmt.Errorf("%w: superblock CRC32C", storagedef.ErrCorrupt)
	}
	if regionU32(encoded, superSectorSizeOffset) != SectorSize ||
		regionU32(encoded, superBankAStartOffset) != BankAStartSector ||
		regionU32(encoded, superBankBStartOffset) != BankBStartSector ||
		regionU32(encoded, superBankSectorsOffset) != bankSectors ||
		regionU32(encoded, superDataStartOffset) != DataStartSector {
		return fmt.Errorf("%w: superblock fixed geometry", storagedef.ErrCorrupt)
	}
	if core.DimensionID(int32(regionU32(encoded, superDimensionOffset))) != key.Dimension ||
		int32(regionU32(encoded, superRegionXOffset)) != key.X ||
		int32(regionU32(encoded, superRegionZOffset)) != key.Z {
		return fmt.Errorf("%w: superblock region key mismatch", storagedef.ErrCorrupt)
	}
	if !regionBytesZero(encoded[superReservedOffset:superCRCOffset]) {
		return fmt.Errorf("%w: superblock reserved bytes", storagedef.ErrCorrupt)
	}
	return nil
}

func EncodeRegionBank(key RegionKey, bank Bank) ([BankSize]byte, error) {
	if err := validateRegionBank(bank, 0, false); err != nil {
		return [BankSize]byte{}, err
	}

	var encoded [BankSize]byte
	copy(encoded[bankMagicOffset:], regionBankMagic[:])
	putRegionU32(encoded[:], bankVersionOffset, currentRegionVersion)
	putRegionU32(encoded[:], bankSectorSizeOffset, SectorSize)
	putRegionU32(encoded[:], bankDimensionOffset, uint32(key.Dimension))
	putRegionU32(encoded[:], bankRegionXOffset, uint32(key.X))
	putRegionU32(encoded[:], bankRegionZOffset, uint32(key.Z))
	binary.LittleEndian.PutUint64(encoded[bankGenerationOffset:], bank.Generation)
	putRegionU32(encoded[:], bankEntryCountOffset, regionSlots)
	putRegionU32(encoded[:], bankEntrySizeOffset, regionEntrySize)
	putRegionU32(encoded[:], bankSectorsOffset, bankSectors)
	putRegionU32(encoded[:], bankDataStartOffset, DataStartSector)

	for slot, entry := range bank.Entries {
		offset := bankEntriesOffset + slot*regionEntrySize
		putRegionU32(encoded[:], offset+entryOffsetSectorOffset, entry.OffsetSector)
		putRegionU32(encoded[:], offset+entrySectorCountOffset, entry.SectorCount)
		putRegionU32(encoded[:], offset+entryPayloadLengthOffset, entry.PayloadLength)
		binary.LittleEndian.PutUint64(encoded[offset+entryRevisionOffset:], entry.Revision)
		putRegionU32(encoded[:], offset+entryPayloadCRCOffset, entry.PayloadCRC32C)
	}
	putRegionU32(encoded[:], bankCRCOffset, regionBankChecksum(encoded[:]))
	return encoded, nil
}

func DecodeRegionBank(key RegionKey, encoded []byte, fileSize int64) (Bank, error) {
	if len(encoded) != BankSize {
		return Bank{}, fmt.Errorf("%w: region bank length %d, want %d", storagedef.ErrCorrupt, len(encoded), BankSize)
	}
	if !bytes.Equal(encoded[bankMagicOffset:bankVersionOffset], regionBankMagic[:]) {
		return Bank{}, fmt.Errorf("%w: region bank magic", storagedef.ErrCorrupt)
	}
	version := regionU32(encoded, bankVersionOffset)
	if version > currentRegionVersion {
		return Bank{}, fmt.Errorf("%w: region bank version %d", storagedef.ErrFutureVersion, version)
	}
	if version != currentRegionVersion {
		return Bank{}, fmt.Errorf("%w: unsupported region bank version %d", storagedef.ErrCorrupt, version)
	}
	if regionBankChecksum(encoded) != regionU32(encoded, bankCRCOffset) {
		return Bank{}, fmt.Errorf("%w: region bank CRC32C", storagedef.ErrCorrupt)
	}
	if regionU32(encoded, bankSectorSizeOffset) != SectorSize ||
		regionU32(encoded, bankEntryCountOffset) != regionSlots ||
		regionU32(encoded, bankEntrySizeOffset) != regionEntrySize ||
		regionU32(encoded, bankSectorsOffset) != bankSectors ||
		regionU32(encoded, bankDataStartOffset) != DataStartSector {
		return Bank{}, fmt.Errorf("%w: region bank fixed geometry", storagedef.ErrCorrupt)
	}
	if core.DimensionID(int32(regionU32(encoded, bankDimensionOffset))) != key.Dimension ||
		int32(regionU32(encoded, bankRegionXOffset)) != key.X ||
		int32(regionU32(encoded, bankRegionZOffset)) != key.Z {
		return Bank{}, fmt.Errorf("%w: region bank key mismatch", storagedef.ErrCorrupt)
	}
	if !regionBytesZero(encoded[bankReservedOffset:bankCRCOffset]) {
		return Bank{}, fmt.Errorf("%w: region bank reserved bytes", storagedef.ErrCorrupt)
	}
	if !regionBytesZero(encoded[bankPaddingOffset:]) {
		return Bank{}, fmt.Errorf("%w: region bank padding", storagedef.ErrCorrupt)
	}

	bank := Bank{Generation: binary.LittleEndian.Uint64(encoded[bankGenerationOffset:])}
	for slot := range bank.Entries {
		offset := bankEntriesOffset + slot*regionEntrySize
		bank.Entries[slot] = Entry{
			OffsetSector:  regionU32(encoded, offset+entryOffsetSectorOffset),
			SectorCount:   regionU32(encoded, offset+entrySectorCountOffset),
			PayloadLength: regionU32(encoded, offset+entryPayloadLengthOffset),
			Revision:      binary.LittleEndian.Uint64(encoded[offset+entryRevisionOffset:]),
			PayloadCRC32C: regionU32(encoded, offset+entryPayloadCRCOffset),
		}
	}
	if err := validateRegionBank(bank, fileSize, true); err != nil {
		return Bank{}, err
	}
	return bank, nil
}

func SelectRegionBank(
	bankA Bank,
	errA error,
	bankB Bank,
	errB error,
) (Bank, int, error) {
	if errA == nil && bankA.Generation == 0 {
		errA = fmt.Errorf("%w: bank A is an uncommitted standby", storagedef.ErrCorrupt)
	}
	if errB == nil && bankB.Generation == 0 {
		errB = fmt.Errorf("%w: bank B is an uncommitted standby", storagedef.ErrCorrupt)
	}
	if errA != nil && errB != nil {
		return Bank{}, -1, fmt.Errorf(
			"%w: both region banks invalid: bank A: %v; bank B: %v",
			storagedef.ErrCorrupt, errA, errB,
		)
	}
	if errA != nil {
		return bankB, 1, nil
	}
	if errB != nil {
		return bankA, 0, nil
	}
	if bankA.Generation > bankB.Generation {
		return bankA, 0, nil
	}
	if bankB.Generation > bankA.Generation {
		return bankB, 1, nil
	}

	// Decoding accepts only canonical zero-filled headers and padding, so equal
	// decoded values represent byte-identical banks for a shared RegionKey.
	if bankA == bankB {
		return bankA, 0, nil
	}
	return Bank{}, -1, fmt.Errorf(
		"%w: divergent region banks at generation %d", storagedef.ErrCorrupt, bankA.Generation,
	)
}

func validateRegionBank(bank Bank, fileSize int64, checkFileSize bool) error {
	if checkFileSize && fileSize < int64(DataStartSector*SectorSize) {
		return fmt.Errorf("%w: region file is shorter than fixed headers", storagedef.ErrCorrupt)
	}

	ranges := make([]sectorRange, 0)
	for slot, entry := range bank.Entries {
		if entry.OffsetSector == 0 {
			if entry != (Entry{}) {
				return fmt.Errorf("%w: absent region entry %d has nonzero fields", storagedef.ErrCorrupt, slot)
			}
			continue
		}
		if bank.Generation == 0 {
			return fmt.Errorf("%w: generation zero region bank is not empty", storagedef.ErrCorrupt)
		}
		if entry.SectorCount == 0 {
			return fmt.Errorf("%w: region entry %d has zero sector count", storagedef.ErrCorrupt, slot)
		}
		if entry.PayloadLength > MaxCompressedChunk {
			return fmt.Errorf("%w: region entry %d payload exceeds limit", storagedef.ErrCorrupt, slot)
		}
		if uint64(entry.PayloadLength) > uint64(entry.SectorCount)*SectorSize {
			return fmt.Errorf("%w: region entry %d payload exceeds extent", storagedef.ErrCorrupt, slot)
		}
		if entry.Revision == 0 {
			return fmt.Errorf("%w: region entry %d has zero revision", storagedef.ErrCorrupt, slot)
		}

		first := uint64(entry.OffsetSector)
		end := first + uint64(entry.SectorCount)
		if end > math.MaxUint32 {
			return fmt.Errorf("%w: region entry %d extent overflows uint32", storagedef.ErrCorrupt, slot)
		}
		ranges = append(ranges, sectorRange{first: first, end: end})
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].first < ranges[j].first })
	for index, regionRange := range ranges {
		if regionRange.first < DataStartSector || regionRange.first >= regionRange.end {
			return fmt.Errorf("%w: invalid extent", storagedef.ErrCorrupt)
		}
		if checkFileSize && regionRange.end > uint64(fileSize/SectorSize) {
			return fmt.Errorf("%w: invalid extent", storagedef.ErrCorrupt)
		}
		if index > 0 && ranges[index-1].end > regionRange.first {
			return fmt.Errorf("%w: overlapping extents", storagedef.ErrCorrupt)
		}
	}
	return nil
}

func regionBankChecksum(encoded []byte) uint32 {
	hasher := crc32.New(CRCTable)
	_, _ = hasher.Write(encoded[:bankCRCOffset])
	_, _ = hasher.Write([]byte{0, 0, 0, 0})
	_, _ = hasher.Write(encoded[bankCRCOffset+4:])
	return hasher.Sum32()
}

func putRegionU32(encoded []byte, offset int, value uint32) {
	binary.LittleEndian.PutUint32(encoded[offset:], value)
}

func regionU32(encoded []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(encoded[offset:])
}

func regionBytesZero(encoded []byte) bool {
	for _, value := range encoded {
		if value != 0 {
			return false
		}
	}
	return true
}
