package passive

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
	"slices"

	"github.com/channing771/mornlea/packages/server/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 被动牛存档的 schema 演进常量。当前只写 v1；未来版本在解码入口按
// storagedef.ErrFutureVersion 拒绝，绝不猜测布局。
const (
	passiveEnvelopeVersion uint32 = 1
	passiveSchemaV1        uint32 = 1
	// CurrentSchema 是当前写出的 schema；编码端只写当前版本。留根的根包
	// store 测试以它构造未来 schema 故障注入（跟随权威常量而非字面量），
	// 故导出。
	CurrentSchema       uint32 = passiveSchemaV1
	passiveHeaderLength        = 32
	passiveRecordLength        = 72
	// MaxFileLength 是物理文件字节上界（spec：2336）= 32-byte 头 +
	// 32 条 72-byte 记录。解码在任何解析与分配之前按本值拒绝超长；根包
	// 编排读取 passive_mobs.bin 时按同一上界截断读取，故导出。
	MaxFileLength = passiveHeaderLength + MaxPassiveMobs*passiveRecordLength
)

// passiveReservedLength 是单条记录尾段的保留字节数：42 字节身体字段之后
// 的 30 字节是 schema v1 的保留段，编码恒写零、解码遇非零即损坏。被动牛
// 没有夜行者侧的冷却/目标/重规划字段（sim 侧刻意不设，逃跑与出生区块是
// 运行时派生物），保留段为未来字段演进占位而不破坏 72-byte 固定步长。
const passiveReservedLength = passiveRecordLength - 42

var (
	passiveEnvelopeMagic = [4]byte{'P', 'M', 'S', 'T'}
	passiveCRCTable      = crc32.MakeTable(crc32.Castagnoli)
)

// Encode 把一份被动牛集合快照编码为规范磁盘形态：记录按 ID 升序写出，
// 输入顺序与字段值不得被修改。revision 为零或任何记录越界都拒绝编码——
// 编码端产出的字节必须能被 Decode 原样接受，绝不写出不可读文件。
func Encode(save PassiveMobsSave) ([]byte, error) {
	if save.Revision == 0 {
		return nil, fmt.Errorf("%w: zero passive revision", storagedef.ErrCorrupt)
	}
	if len(save.Records) > MaxPassiveMobs {
		return nil, fmt.Errorf("%w: passive count %d exceeds limit", storagedef.ErrCorrupt, len(save.Records))
	}
	records := slices.Clone(save.Records)
	slices.SortFunc(records, func(a, b StoredPassiveMob) int {
		return cmp.Compare(a.ID, b.ID)
	})
	for index, record := range records {
		if err := validatePassiveRecord(record); err != nil {
			return nil, fmt.Errorf("passive record %d: %w", index, err)
		}
		if index > 0 && records[index-1].ID == record.ID {
			return nil, fmt.Errorf("%w: duplicate passive ID", storagedef.ErrCorrupt)
		}
	}

	payloadLength := len(records) * passiveRecordLength
	encoded := make([]byte, 0, passiveHeaderLength+payloadLength)
	encoded = append(encoded, passiveEnvelopeMagic[:]...)
	encoded = appendU32(encoded, passiveEnvelopeVersion)
	encoded = appendU32(encoded, CurrentSchema)
	encoded = appendU64(encoded, save.Revision)
	encoded = appendU32(encoded, uint32(len(records)))
	encoded = appendU32(encoded, uint32(payloadLength))
	encoded = appendU32(encoded, 0)
	for _, record := range records {
		encoded = appendPassiveMob(encoded, record)
	}
	// 长度门禁的编码侧镜像：count 上界已使本分支不可达，保留它与 hostile
	// codec 同构，作为「产出必须可被解码端接受」的防御断言。
	if len(encoded) > MaxFileLength {
		return nil, fmt.Errorf(
			"%w: passive file length %d exceeds limit", storagedef.ErrCorrupt, len(encoded),
		)
	}
	binary.LittleEndian.PutUint32(encoded[28:], passiveChecksum(encoded))
	return encoded, nil
}

// Decode 解码一份被动牛存档。入口先守住文件与 count 上界再做
// 任何解析与分配；未来 envelope/schema 版本以 storagedef.ErrFutureVersion 与损坏区分，
// 记录字段校验与编码端共用同一函数保证双向边界一致。payload 必须被恰好
// 读空：固定步长布局下任何长度错位都是损坏，静默接受等于接受非规范文件。
func Decode(data []byte) (StoredPassiveMobs, error) {
	// 分配前门禁：任何超过物理上界的输入在解析前拒绝。
	if len(data) > MaxFileLength {
		return StoredPassiveMobs{}, fmt.Errorf(
			"%w: passive file length %d exceeds limit", storagedef.ErrCorrupt, len(data),
		)
	}
	header := byteDecoder{data: data}
	if err := header.magic(passiveEnvelopeMagic); err != nil {
		return StoredPassiveMobs{}, corrupt("passive envelope magic", err)
	}
	version, err := header.u32()
	if err != nil {
		return StoredPassiveMobs{}, corrupt("passive envelope version", err)
	}
	if version != passiveEnvelopeVersion {
		if version > passiveEnvelopeVersion {
			return StoredPassiveMobs{}, fmt.Errorf("%w: passive envelope version %d", storagedef.ErrFutureVersion, version)
		}
		return StoredPassiveMobs{}, fmt.Errorf("%w: unsupported passive envelope version %d", storagedef.ErrCorrupt, version)
	}
	schema, err := header.u32()
	if err != nil {
		return StoredPassiveMobs{}, corrupt("passive schema", err)
	}
	if schema != passiveSchemaV1 {
		// 白名单只有一个成员，仍显式列出字面常量而不是 current 引用：未来
		// v2 成为 current 时，v1 文件必须仍被本入口放行（hostile 同款
		// 白名单纪律）。
		if schema > CurrentSchema {
			return StoredPassiveMobs{}, fmt.Errorf("%w: passive schema %d", storagedef.ErrFutureVersion, schema)
		}
		return StoredPassiveMobs{}, fmt.Errorf("%w: unsupported passive schema %d", storagedef.ErrCorrupt, schema)
	}
	revision, err := header.u64()
	if err != nil {
		return StoredPassiveMobs{}, corrupt("passive revision", err)
	}
	if revision == 0 {
		return StoredPassiveMobs{}, fmt.Errorf("%w: zero passive revision", storagedef.ErrCorrupt)
	}
	count, err := header.u32()
	if err != nil {
		return StoredPassiveMobs{}, corrupt("passive count", err)
	}
	if count > MaxPassiveMobs {
		return StoredPassiveMobs{}, fmt.Errorf("%w: passive count %d exceeds limit", storagedef.ErrCorrupt, count)
	}
	payloadLength, err := header.u32()
	if err != nil {
		return StoredPassiveMobs{}, corrupt("passive payload length", err)
	}
	// v1 是固定步长布局：payload 长度必须恰好等于 count 条记录，任何偏差
	// 都意味着头与数据错位，继续解析没有意义。
	if payloadLength != count*passiveRecordLength {
		return StoredPassiveMobs{}, fmt.Errorf("%w: passive payload length does not match count", storagedef.ErrCorrupt)
	}
	wantCRC, err := header.u32()
	if err != nil {
		return StoredPassiveMobs{}, corrupt("passive CRC32C", err)
	}
	if uint64(header.remaining()) != uint64(payloadLength) {
		return StoredPassiveMobs{}, fmt.Errorf("%w: passive payload length does not match file", storagedef.ErrCorrupt)
	}
	if passiveChecksum(data) != wantCRC {
		return StoredPassiveMobs{}, fmt.Errorf("%w: passive CRC32C", storagedef.ErrCorrupt)
	}

	records := make([]StoredPassiveMob, int(count))
	for index := range records {
		record, err := decodePassiveMob(&header)
		if err != nil {
			return StoredPassiveMobs{}, fmt.Errorf("passive record %d: %w", index, err)
		}
		if index > 0 && records[index-1].ID >= record.ID {
			return StoredPassiveMobs{}, fmt.Errorf("%w: passive IDs are not strictly sorted", storagedef.ErrCorrupt)
		}
		records[index] = record
	}
	if header.remaining() != 0 {
		return StoredPassiveMobs{}, fmt.Errorf(
			"%w: passive payload has %d trailing bytes", storagedef.ErrCorrupt, header.remaining(),
		)
	}
	return StoredPassiveMobs{Revision: revision, Records: records}, nil
}

// validatePassiveRecord 校验单条被动牛记录的全部不变量：ID 非零、维度已
// 知、位置/速度/朝向有限、世界 Y 落在 [core.MinY, core.MaxY)、生命为正且
// 不超过 core.MaxHealth。编码与解码共用本函数，保证双向边界一致。
func validatePassiveRecord(record StoredPassiveMob) error {
	if record.ID == 0 {
		return fmt.Errorf("%w: zero passive ID", storagedef.ErrCorrupt)
	}
	if record.Dimension != core.Overworld {
		return fmt.Errorf("%w: unsupported passive dimension %d", storagedef.ErrCorrupt, record.Dimension)
	}
	for _, value := range record.Position {
		if !finitePassiveFloat(value) {
			return fmt.Errorf("%w: non-finite passive position", storagedef.ErrCorrupt)
		}
	}
	if record.Position[1] < float32(core.MinY) || record.Position[1] >= float32(core.MaxY) {
		return fmt.Errorf("%w: passive position Y %v outside world", storagedef.ErrCorrupt, record.Position[1])
	}
	for _, value := range record.Velocity {
		if !finitePassiveFloat(value) {
			return fmt.Errorf("%w: non-finite passive velocity", storagedef.ErrCorrupt)
		}
	}
	if !finitePassiveFloat(record.Yaw) {
		return fmt.Errorf("%w: non-finite passive yaw", storagedef.ErrCorrupt)
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return fmt.Errorf("%w: passive health %d outside 1..%d", storagedef.ErrCorrupt, record.Health, core.MaxHealth)
	}
	return nil
}

func finitePassiveFloat(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// appendPassiveMob 按固定布局追加一条 72-byte 记录：ID u64、dimension
// u32、position 与 velocity 各 3×f32、onGround u8、yaw f32、health u8，
// 随后是 30 字节全零保留段——恰好无填充；onGround 非 0/1 的字节由解码端
// 拒绝。调用前必须已通过 validatePassiveRecord。
func appendPassiveMob(dst []byte, record StoredPassiveMob) []byte {
	dst = appendU64(dst, record.ID)
	dst = appendU32(dst, uint32(record.Dimension))
	for _, value := range record.Position {
		dst = appendF32(dst, value)
	}
	for _, value := range record.Velocity {
		dst = appendF32(dst, value)
	}
	var onGround byte
	if record.OnGround {
		onGround = 1
	}
	dst = append(dst, onGround)
	dst = appendF32(dst, record.Yaw)
	dst = append(dst, record.Health)
	return append(dst, make([]byte, passiveReservedLength)...)
}

func decodePassiveMob(decoder *byteDecoder) (StoredPassiveMob, error) {
	var record StoredPassiveMob
	id, err := decoder.u64()
	if err != nil {
		return StoredPassiveMob{}, corrupt("passive ID", err)
	}
	record.ID = id
	dimension, err := decoder.u32()
	if err != nil {
		return StoredPassiveMob{}, corrupt("passive dimension", err)
	}
	record.Dimension = core.DimensionID(int32(dimension))
	for index := range record.Position {
		if record.Position[index], err = decodeF32(decoder); err != nil {
			return StoredPassiveMob{}, corrupt("passive position", err)
		}
	}
	for index := range record.Velocity {
		if record.Velocity[index], err = decodeF32(decoder); err != nil {
			return StoredPassiveMob{}, corrupt("passive velocity", err)
		}
	}
	onGround, err := decoder.u8()
	if err != nil {
		return StoredPassiveMob{}, corrupt("passive onGround", err)
	}
	if onGround > 1 {
		return StoredPassiveMob{}, fmt.Errorf("%w: passive onGround %d is not a bool", storagedef.ErrCorrupt, onGround)
	}
	record.OnGround = onGround == 1
	if record.Yaw, err = decodeF32(decoder); err != nil {
		return StoredPassiveMob{}, corrupt("passive yaw", err)
	}
	if record.Health, err = decoder.u8(); err != nil {
		return StoredPassiveMob{}, corrupt("passive health", err)
	}
	reserved, err := decoder.take(passiveReservedLength)
	if err != nil {
		return StoredPassiveMob{}, corrupt("passive reserved", err)
	}
	for index, value := range reserved {
		if value != 0 {
			return StoredPassiveMob{}, fmt.Errorf(
				"%w: passive reserved byte %d is not zero", storagedef.ErrCorrupt, index,
			)
		}
	}
	if err := validatePassiveRecord(record); err != nil {
		return StoredPassiveMob{}, err
	}
	return record, nil
}

// passiveChecksum 沿用 hostile 存档惯例：CRC-32C 覆盖头部的 [8:28] 段
// （schema..revision..count..payloadLen）与 payload 段，magic 与 CRC 字节
// 自身不参与。
func passiveChecksum(data []byte) uint32 {
	hasher := crc32.New(passiveCRCTable)
	_, _ = hasher.Write(data[8:28])
	_, _ = hasher.Write(data[passiveHeaderLength:])
	return hasher.Sum32()
}
