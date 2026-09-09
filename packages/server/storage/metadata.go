package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	currentMetadataVersion uint32 = 5
	// metadataPayloadLength 是 v5 载荷长度：v4 的 41 字节之后追加维度表
	//（维度数 u32、`Depths` 出生锚点 X/Z 各 u32、种子盐 u64，共 20 字节）。
	metadataPayloadLength uint32 = 61
	// legacyMetadataV4Version 是仍可读取的 v4；v4 只被读取和迁移，不再写出。
	legacyMetadataV4Version       uint32 = 4
	legacyMetadataV4PayloadLength uint32 = 41
	// legacyMetadataV3Version 是仍可读取的 v3；v3 只被读取和迁移，不再写出。
	legacyMetadataV3Version       uint32 = 3
	legacyMetadataV3PayloadLength uint32 = 36
	// legacyMetadataVersion 是仍可读取的 v1；v1 只被读取和迁移，不再写出。
	legacyMetadataVersion       uint32 = 1
	legacyMetadataPayloadLength uint32 = 20
	// legacyMetadataV2Version 是仍可读取的 v2；v2 只被读取和迁移，不再写出。
	legacyMetadataV2Version       uint32 = 2
	legacyMetadataV2PayloadLength uint32 = 28
	metadataHeaderLength                 = 12
	metadataChecksumLength               = 4
	// metadataDimensionCount 是 v5 维度表的维度数：主世界与 `Depths` 共两维。
	metadataDimensionCount uint32 = 2
	// depthsSeedSaltDefault 是旧档缺失维度表时的种子盐默认值：与世界生成侧
	// 派生 `Depths` 地形种子的固定盐是同一常量。
	depthsSeedSaltDefault uint64 = 0x9E3779B97F4A7C15
)

var (
	metadataMagic    = [4]byte{'M', 'C', 'G', 'M'}
	metadataCRCTable = crc32.MakeTable(crc32.Castagnoli)
)

type metadataDirectory interface {
	Sync() error
	Close() error
}

type atomicReplaceFile interface {
	Name() string
	Chmod(fs.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type atomicReplaceHooks struct {
	createTemp    func(string, string) (atomicReplaceFile, error)
	beforeRename  func() error
	rename        func(string, string) error
	openDirectory func(string) (metadataDirectory, error)
}

func encodeMetadata(metadata Metadata) ([]byte, error) {
	if metadata.FormatVersion > currentMetadataVersion {
		return nil, fmt.Errorf("%w: metadata version %d", ErrFutureVersion, metadata.FormatVersion)
	}
	if metadata.FormatVersion != currentMetadataVersion {
		return nil, fmt.Errorf("%w: unsupported metadata version %d", ErrCorrupt, metadata.FormatVersion)
	}

	encoded := make([]byte, 0, metadataHeaderLength+metadataPayloadLength+metadataChecksumLength)
	encoded = append(encoded, metadataMagic[:]...)
	encoded = binary.LittleEndian.AppendUint32(encoded, metadata.FormatVersion)
	encoded = binary.LittleEndian.AppendUint32(encoded, metadataPayloadLength)
	encoded = binary.LittleEndian.AppendUint64(encoded, uint64(metadata.Seed))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnDimension))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.X))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.SpawnAnchor.Z))
	encoded = binary.LittleEndian.AppendUint64(encoded, metadata.WorldTimeTicks)
	// 偏移是 v3 相对 v2 的纯尾部追加：v2 载荷的既有段布局一字不动。
	encoded = binary.LittleEndian.AppendUint64(encoded, metadata.DayPhaseOffset)
	// 天气是 v4 相对 v3 的纯尾部追加：种类 1 字节在前，剩余时长 u32 紧随其后。
	encoded = append(encoded, byte(metadata.WeatherKind))
	encoded = binary.LittleEndian.AppendUint32(encoded, metadata.WeatherTicksRemaining)
	// 维度表是 v5 相对 v4 的纯尾部追加：维度数固定为 2，其后是 `Depths`
	// 出生锚点 X/Z 与种子盐。
	encoded = binary.LittleEndian.AppendUint32(encoded, metadataDimensionCount)
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.DepthsSpawnAnchor.X))
	encoded = binary.LittleEndian.AppendUint32(encoded, uint32(metadata.DepthsSpawnAnchor.Z))
	encoded = binary.LittleEndian.AppendUint64(encoded, metadata.DepthsSeedSalt)
	encoded = binary.LittleEndian.AppendUint32(
		encoded, crc32.Checksum(encoded, metadataCRCTable),
	)
	return encoded, nil
}

func decodeMetadata(encoded []byte) (Metadata, error) {
	if len(encoded) < metadataHeaderLength {
		return Metadata{}, fmt.Errorf("%w: metadata header is short", ErrCorrupt)
	}
	if string(encoded[:len(metadataMagic)]) != string(metadataMagic[:]) {
		return Metadata{}, fmt.Errorf("%w: metadata magic", ErrCorrupt)
	}

	version := binary.LittleEndian.Uint32(encoded[4:8])
	if version > currentMetadataVersion {
		return Metadata{}, fmt.Errorf("%w: metadata version %d", ErrFutureVersion, version)
	}
	// v1、v2、v3、v4 与 v5 各自有固定 payload 长度；旧版本读取后在内存中规范为当前
	// 版本：v1 世界时间与偏移均为零，v2 偏移为零，v1/v2/v3 天气均为晴天、
	// 剩余时长均为零（零表示旧档未记录，恢复时按新世界默认值掷骰），v1..v4 的
	// `Depths` 出生锚点默认取主世界锚点、种子盐取固定盐。
	var wantPayloadLength uint32
	switch version {
	case currentMetadataVersion:
		wantPayloadLength = metadataPayloadLength
	case legacyMetadataV4Version:
		wantPayloadLength = legacyMetadataV4PayloadLength
	case legacyMetadataV3Version:
		wantPayloadLength = legacyMetadataV3PayloadLength
	case legacyMetadataV2Version:
		wantPayloadLength = legacyMetadataV2PayloadLength
	case legacyMetadataVersion:
		wantPayloadLength = legacyMetadataPayloadLength
	default:
		return Metadata{}, fmt.Errorf("%w: unsupported metadata version %d", ErrCorrupt, version)
	}

	payloadLength := binary.LittleEndian.Uint32(encoded[8:12])
	if payloadLength != wantPayloadLength {
		return Metadata{}, fmt.Errorf("%w: metadata payload length %d", ErrCorrupt, payloadLength)
	}
	wantLength := metadataHeaderLength + int(payloadLength) + metadataChecksumLength
	if len(encoded) != wantLength {
		return Metadata{}, fmt.Errorf(
			"%w: metadata length %d, want %d", ErrCorrupt, len(encoded), wantLength,
		)
	}

	checksumOffset := wantLength - metadataChecksumLength
	wantChecksum := binary.LittleEndian.Uint32(encoded[checksumOffset:])
	gotChecksum := crc32.Checksum(encoded[:checksumOffset], metadataCRCTable)
	if gotChecksum != wantChecksum {
		return Metadata{}, fmt.Errorf("%w: metadata CRC32C", ErrCorrupt)
	}

	payload := encoded[metadataHeaderLength:checksumOffset]
	metadata := Metadata{
		FormatVersion:  currentMetadataVersion,
		Seed:           int64(binary.LittleEndian.Uint64(payload[0:8])),
		SpawnDimension: core.DimensionID(int32(binary.LittleEndian.Uint32(payload[8:12]))),
		SpawnAnchor: core.ChunkPos{
			X: int32(binary.LittleEndian.Uint32(payload[12:16])),
			Z: int32(binary.LittleEndian.Uint32(payload[16:20])),
		},
	}
	// 世界时间自 v2 起持久化，偏移自 v3 起持久化，天气自 v4 起持久化，
	// 维度表自 v5 起持久化：旧版本读入即升级，缺失的尾部字段按零值迁移
	// （天气为晴天、剩余时长为零，`Depths` 出生锚点默认取主世界锚点、种子盐
	// 取固定盐），行为与升级前完全一致。
	if version >= legacyMetadataV2Version {
		metadata.WorldTimeTicks = binary.LittleEndian.Uint64(payload[20:28])
	}
	if version >= legacyMetadataV3Version {
		metadata.DayPhaseOffset = binary.LittleEndian.Uint64(payload[28:36])
	}
	if version >= legacyMetadataV4Version {
		metadata.WeatherKind = core.WeatherKind(payload[36])
		metadata.WeatherTicksRemaining = binary.LittleEndian.Uint32(payload[37:41])
	}
	if version == currentMetadataVersion {
		if dimCount := binary.LittleEndian.Uint32(payload[41:45]); dimCount != metadataDimensionCount {
			return Metadata{}, fmt.Errorf("%w: metadata dimension count %d", ErrCorrupt, dimCount)
		}
		metadata.DepthsSpawnAnchor = core.ChunkPos{
			X: int32(binary.LittleEndian.Uint32(payload[45:49])),
			Z: int32(binary.LittleEndian.Uint32(payload[49:53])),
		}
		metadata.DepthsSeedSalt = binary.LittleEndian.Uint64(payload[53:61])
	} else {
		metadata.DepthsSpawnAnchor = metadata.SpawnAnchor
		metadata.DepthsSeedSalt = depthsSeedSaltDefault
	}
	return metadata, nil
}

func replaceFileAtomically(
	path, pattern string,
	data []byte,
	mode fs.FileMode,
) error {
	return replaceFileAtomicallyWithPatternAndHooks(
		path, pattern, data, mode, atomicReplaceHooks{},
	)
}

func replaceFileAtomicallyWithHooks(
	path string,
	data []byte,
	mode fs.FileMode,
	hooks atomicReplaceHooks,
) error {
	return replaceFileAtomicallyWithPatternAndHooks(
		path, ".world.meta.tmp-*", data, mode, hooks,
	)
}

func replaceFileAtomicallyWithPatternAndHooks(
	path, pattern string,
	data []byte,
	mode fs.FileMode,
	hooks atomicReplaceHooks,
) error {
	if hooks.createTemp == nil {
		hooks.createTemp = func(directory, pattern string) (atomicReplaceFile, error) {
			return os.CreateTemp(directory, pattern)
		}
	}
	if hooks.rename == nil {
		hooks.rename = os.Rename
	}
	if hooks.openDirectory == nil {
		hooks.openDirectory = openMetadataDirectory
	}

	parent := filepath.Dir(path)
	temporary, err := hooks.createTemp(parent, pattern)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temporary file: %w", err)
	}
	for remaining := data; len(remaining) > 0; {
		written, err := temporary.Write(remaining)
		if err != nil {
			return fmt.Errorf("write temporary file: %w", err)
		}
		if written == 0 {
			return fmt.Errorf("write temporary file: %w", io.ErrShortWrite)
		}
		remaining = remaining[written:]
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if hooks.beforeRename != nil {
		if err := hooks.beforeRename(); err != nil {
			return fmt.Errorf("before replacing file: %w", err)
		}
	}
	if err := hooks.rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	removeTemporary = false

	directory, err := hooks.openDirectory(parent)
	if err != nil {
		return fmt.Errorf("open containing directory: %w", err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return fmt.Errorf("sync containing directory: %w", err)
	}

	return nil
}

func openMetadataDirectory(path string) (metadataDirectory, error) {
	return os.Open(path)
}
