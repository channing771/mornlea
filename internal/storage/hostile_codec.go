package storage

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
	"slices"

	"github.com/channing771/mornlea/internal/core"
)

// 夜行者存档的 schema 演进常量。当前只写 v1；未来版本在解码入口按
// ErrFutureVersion 拒绝，绝不猜测布局。
const (
	hostileEnvelopeVersion uint32 = 1
	hostileSchemaV1        uint32 = 1
	// currentHostileSchema 是当前写出的 schema；编码端只写当前版本。
	currentHostileSchema uint32 = hostileSchemaV1
	hostileHeaderLength         = 32
	hostileRecordLength         = 72
	// maxHostileFileLength 是物理文件字节上界（spec：4640）= 32-byte 头 +
	// 64 条 72-byte 记录。解码在任何解析与分配之前按本值拒绝超长。
	maxHostileFileLength = hostileHeaderLength + MaxHostileMobs*hostileRecordLength
)

// hostileCooldownPeriodTicks 是三个冷却计时器（攻击/受击/灼烧）的共享
// 周期上限：本 change 的攻击与灼烧周期都是 20 tick，受击冷却同量级。
// 计时器落盘值必须落在周期之内，越界即损坏——上限与权威模拟侧的周期
// 常量同源，任何一侧单独调整都必须同步本值并更新 golden。
const hostileCooldownPeriodTicks uint8 = 20

// maxHostileDistantTicks 是远离 despawn 的累计 tick 稳定上限：夜行者距
// 全部 active 玩家超过 64 格累计 600 active tick 后移除，计数在回到范围
// 内时清零，因此合法落盘值是 0..600。
const maxHostileDistantTicks uint16 = 600

var (
	hostileEnvelopeMagic = [4]byte{'M', 'H', 'S', 'T'}
	hostileCRCTable      = crc32.MakeTable(crc32.Castagnoli)
)

// encodeHostileMobs 把一份夜行者集合快照编码为规范磁盘形态：记录按 ID
// 升序写出，输入顺序与字段值不得被修改。revision 为零或任何记录越界都
// 拒绝编码——编码端产出的字节必须能被解码端原样接受，绝不写出不可读
// 文件。
func encodeHostileMobs(save HostileMobsSave) ([]byte, error) {
	if save.Revision == 0 {
		return nil, fmt.Errorf("%w: zero hostile revision", ErrCorrupt)
	}
	if len(save.Records) > MaxHostileMobs {
		return nil, fmt.Errorf("%w: hostile count %d exceeds limit", ErrCorrupt, len(save.Records))
	}
	records := slices.Clone(save.Records)
	slices.SortFunc(records, func(a, b StoredHostileMob) int {
		return cmp.Compare(a.ID, b.ID)
	})
	for index, record := range records {
		if err := validateHostileRecord(record); err != nil {
			return nil, fmt.Errorf("hostile record %d: %w", index, err)
		}
		if index > 0 && records[index-1].ID == record.ID {
			return nil, fmt.Errorf("%w: duplicate hostile ID", ErrCorrupt)
		}
	}

	payloadLength := len(records) * hostileRecordLength
	encoded := make([]byte, 0, hostileHeaderLength+payloadLength)
	encoded = append(encoded, hostileEnvelopeMagic[:]...)
	encoded = appendU32(encoded, hostileEnvelopeVersion)
	encoded = appendU32(encoded, currentHostileSchema)
	encoded = appendU64(encoded, save.Revision)
	encoded = appendU32(encoded, uint32(len(records)))
	encoded = appendU32(encoded, uint32(payloadLength))
	encoded = appendU32(encoded, 0)
	for _, record := range records {
		encoded = appendHostileMob(encoded, record)
	}
	// 长度门禁的编码侧镜像：count 上界已使本分支不可达，保留它与 companion
	// codec 同构，作为「产出必须可被解码端接受」的防御断言。
	if len(encoded) > maxHostileFileLength {
		return nil, fmt.Errorf(
			"%w: hostile file length %d exceeds limit", ErrCorrupt, len(encoded),
		)
	}
	binary.LittleEndian.PutUint32(encoded[28:], hostileChecksum(encoded))
	return encoded, nil
}

// decodeHostileMobs 解码一份夜行者存档。入口先守住文件与 count 上界再做
// 任何解析与分配；未来 envelope/schema 版本以 ErrFutureVersion 与损坏区分，
// 记录字段校验与编码端共用同一函数保证双向边界一致。payload 必须被恰好
// 读空：固定步长布局下任何长度错位都是损坏，静默接受等于接受非规范文件。
func decodeHostileMobs(data []byte) (StoredHostileMobs, error) {
	// 分配前门禁：任何超过物理上界的输入在解析前拒绝。
	if len(data) > maxHostileFileLength {
		return StoredHostileMobs{}, fmt.Errorf(
			"%w: hostile file length %d exceeds limit", ErrCorrupt, len(data),
		)
	}
	header := byteDecoder{data: data}
	if err := header.magic(hostileEnvelopeMagic); err != nil {
		return StoredHostileMobs{}, corrupt("hostile envelope magic", err)
	}
	version, err := header.u32()
	if err != nil {
		return StoredHostileMobs{}, corrupt("hostile envelope version", err)
	}
	if version != hostileEnvelopeVersion {
		if version > hostileEnvelopeVersion {
			return StoredHostileMobs{}, fmt.Errorf("%w: hostile envelope version %d", ErrFutureVersion, version)
		}
		return StoredHostileMobs{}, fmt.Errorf("%w: unsupported hostile envelope version %d", ErrCorrupt, version)
	}
	schema, err := header.u32()
	if err != nil {
		return StoredHostileMobs{}, corrupt("hostile schema", err)
	}
	if schema != hostileSchemaV1 {
		// 白名单只有一个成员，仍显式列出字面常量而不是 current 引用：未来
		// v2 成为 current 时，v1 文件必须仍被本入口放行（companion 同款
		// 白名单纪律）。
		if schema > currentHostileSchema {
			return StoredHostileMobs{}, fmt.Errorf("%w: hostile schema %d", ErrFutureVersion, schema)
		}
		return StoredHostileMobs{}, fmt.Errorf("%w: unsupported hostile schema %d", ErrCorrupt, schema)
	}
	revision, err := header.u64()
	if err != nil {
		return StoredHostileMobs{}, corrupt("hostile revision", err)
	}
	if revision == 0 {
		return StoredHostileMobs{}, fmt.Errorf("%w: zero hostile revision", ErrCorrupt)
	}
	count, err := header.u32()
	if err != nil {
		return StoredHostileMobs{}, corrupt("hostile count", err)
	}
	if count > MaxHostileMobs {
		return StoredHostileMobs{}, fmt.Errorf("%w: hostile count %d exceeds limit", ErrCorrupt, count)
	}
	payloadLength, err := header.u32()
	if err != nil {
		return StoredHostileMobs{}, corrupt("hostile payload length", err)
	}
	// v1 是固定步长布局：payload 长度必须恰好等于 count 条记录，任何偏差
	// 都意味着头与数据错位，继续解析没有意义。
	if payloadLength != count*hostileRecordLength {
		return StoredHostileMobs{}, fmt.Errorf("%w: hostile payload length does not match count", ErrCorrupt)
	}
	wantCRC, err := header.u32()
	if err != nil {
		return StoredHostileMobs{}, corrupt("hostile CRC32C", err)
	}
	if uint64(header.remaining()) != uint64(payloadLength) {
		return StoredHostileMobs{}, fmt.Errorf("%w: hostile payload length does not match file", ErrCorrupt)
	}
	if hostileChecksum(data) != wantCRC {
		return StoredHostileMobs{}, fmt.Errorf("%w: hostile CRC32C", ErrCorrupt)
	}

	records := make([]StoredHostileMob, int(count))
	for index := range records {
		record, err := decodeHostileMob(&header)
		if err != nil {
			return StoredHostileMobs{}, fmt.Errorf("hostile record %d: %w", index, err)
		}
		if index > 0 && records[index-1].ID >= record.ID {
			return StoredHostileMobs{}, fmt.Errorf("%w: hostile IDs are not strictly sorted", ErrCorrupt)
		}
		records[index] = record
	}
	if header.remaining() != 0 {
		return StoredHostileMobs{}, fmt.Errorf(
			"%w: hostile payload has %d trailing bytes", ErrCorrupt, header.remaining(),
		)
	}
	return StoredHostileMobs{Revision: revision, Records: records}, nil
}

// validateHostileRecord 校验单条夜行者记录的全部不变量：ID 非零、维度已
// 知、位置/速度/朝向有限、世界 Y 落在 [core.MinY, core.MaxY)、生命为正且
// 不超过 core.MaxHealth、三个冷却计时器不越过周期、目标字段成对一致。编码
// 与解码共用本函数，保证双向边界一致。
func validateHostileRecord(record StoredHostileMob) error {
	if record.ID == 0 {
		return fmt.Errorf("%w: zero hostile ID", ErrCorrupt)
	}
	if record.Dimension != core.Overworld {
		return fmt.Errorf("%w: unsupported hostile dimension %d", ErrCorrupt, record.Dimension)
	}
	for _, value := range record.Position {
		if !finiteHostileFloat(value) {
			return fmt.Errorf("%w: non-finite hostile position", ErrCorrupt)
		}
	}
	if record.Position[1] < float32(core.MinY) || record.Position[1] >= float32(core.MaxY) {
		return fmt.Errorf("%w: hostile position Y %v outside world", ErrCorrupt, record.Position[1])
	}
	for _, value := range record.Velocity {
		if !finiteHostileFloat(value) {
			return fmt.Errorf("%w: non-finite hostile velocity", ErrCorrupt)
		}
	}
	if !finiteHostileFloat(record.Yaw) {
		return fmt.Errorf("%w: non-finite hostile yaw", ErrCorrupt)
	}
	if record.Health == 0 || record.Health > core.MaxHealth {
		return fmt.Errorf("%w: hostile health %d outside 1..%d", ErrCorrupt, record.Health, core.MaxHealth)
	}
	for _, cooldown := range [3]uint8{record.AttackCooldown, record.HurtCooldown, record.BurnCooldown} {
		if cooldown > hostileCooldownPeriodTicks {
			return fmt.Errorf(
				"%w: hostile cooldown %d exceeds period %d",
				ErrCorrupt, cooldown, hostileCooldownPeriodTicks,
			)
		}
	}
	if record.DistantTicks > maxHostileDistantTicks {
		return fmt.Errorf(
			"%w: hostile distant ticks %d exceeds limit %d",
			ErrCorrupt, record.DistantTicks, maxHostileDistantTicks,
		)
	}
	if !record.HasTarget {
		if record.PlayerID != (core.PlayerID{}) {
			return fmt.Errorf("%w: hostile without target keeps player ID", ErrCorrupt)
		}
		return nil
	}
	if !record.PlayerID.Valid() {
		return fmt.Errorf("%w: hostile target is not a valid UUIDv4", ErrCorrupt)
	}
	return nil
}

func finiteHostileFloat(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// appendHostileMob 按固定布局追加一条 72-byte 记录：ID u64、dimension
// u32、position 与 velocity 各 3×f32、onGround u8、yaw f32、health 与三个
// 冷却各 u8、hasTarget u8、目标玩家 ID 16 字节、下一重规划 tick u64、
// distant u16——恰好无填充；两个布尔各占独立字节（D7 字段切分），非 0/1
// 的字节由解码端拒绝。调用前必须已通过 validateHostileRecord。
func appendHostileMob(dst []byte, record StoredHostileMob) []byte {
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
	dst = append(dst, record.Health, record.AttackCooldown, record.HurtCooldown, record.BurnCooldown)
	var hasTarget byte
	if record.HasTarget {
		hasTarget = 1
	}
	dst = append(dst, hasTarget)
	dst = append(dst, record.PlayerID[:]...)
	dst = appendU64(dst, record.NextRepathTicks)
	return binary.LittleEndian.AppendUint16(dst, record.DistantTicks)
}

func decodeHostileMob(decoder *byteDecoder) (StoredHostileMob, error) {
	var record StoredHostileMob
	id, err := decoder.u64()
	if err != nil {
		return StoredHostileMob{}, corrupt("hostile ID", err)
	}
	record.ID = id
	dimension, err := decoder.u32()
	if err != nil {
		return StoredHostileMob{}, corrupt("hostile dimension", err)
	}
	record.Dimension = core.DimensionID(int32(dimension))
	for index := range record.Position {
		if record.Position[index], err = decodeF32(decoder); err != nil {
			return StoredHostileMob{}, corrupt("hostile position", err)
		}
	}
	for index := range record.Velocity {
		if record.Velocity[index], err = decodeF32(decoder); err != nil {
			return StoredHostileMob{}, corrupt("hostile velocity", err)
		}
	}
	onGround, err := decoder.u8()
	if err != nil {
		return StoredHostileMob{}, corrupt("hostile onGround", err)
	}
	if onGround > 1 {
		return StoredHostileMob{}, fmt.Errorf("%w: hostile onGround %d is not a bool", ErrCorrupt, onGround)
	}
	record.OnGround = onGround == 1
	if record.Yaw, err = decodeF32(decoder); err != nil {
		return StoredHostileMob{}, corrupt("hostile yaw", err)
	}
	if record.Health, err = decoder.u8(); err != nil {
		return StoredHostileMob{}, corrupt("hostile health", err)
	}
	if record.AttackCooldown, err = decoder.u8(); err != nil {
		return StoredHostileMob{}, corrupt("hostile attack cooldown", err)
	}
	if record.HurtCooldown, err = decoder.u8(); err != nil {
		return StoredHostileMob{}, corrupt("hostile hurt cooldown", err)
	}
	if record.BurnCooldown, err = decoder.u8(); err != nil {
		return StoredHostileMob{}, corrupt("hostile burn cooldown", err)
	}
	hasTarget, err := decoder.u8()
	if err != nil {
		return StoredHostileMob{}, corrupt("hostile hasTarget", err)
	}
	if hasTarget > 1 {
		return StoredHostileMob{}, fmt.Errorf("%w: hostile hasTarget %d is not a bool", ErrCorrupt, hasTarget)
	}
	record.HasTarget = hasTarget == 1
	playerID, err := decoder.take(len(core.PlayerID{}))
	if err != nil {
		return StoredHostileMob{}, corrupt("hostile player ID", err)
	}
	copy(record.PlayerID[:], playerID)
	if record.NextRepathTicks, err = decoder.u64(); err != nil {
		return StoredHostileMob{}, corrupt("hostile next repath", err)
	}
	if record.DistantTicks, err = decoder.u16(); err != nil {
		return StoredHostileMob{}, corrupt("hostile distant ticks", err)
	}
	if err := validateHostileRecord(record); err != nil {
		return StoredHostileMob{}, err
	}
	return record, nil
}

// hostileChecksum 沿用 companion 存档惯例：CRC-32C 覆盖头部的 [8:28] 段
// （schema..revision..count..payloadLen）与 payload 段，magic 与 CRC 字节
// 自身不参与。
func hostileChecksum(data []byte) uint32 {
	hasher := crc32.New(hostileCRCTable)
	_, _ = hasher.Write(data[8:28])
	_, _ = hasher.Write(data[hostileHeaderLength:])
	return hasher.Sum32()
}
