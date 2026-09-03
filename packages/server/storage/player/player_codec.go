package player

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"

	"github.com/channing771/mornlea/packages/server/storage/storagedef"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// CurrentSchema 为 v8：追加定长重生点三字段。它同时是根包读取 player
	// 文件时构造「未来 schema」故障注入的基准，故导出。
	CurrentSchema uint32 = 8
	// EnvelopeLength 是 MCPL 信封的固定头部长度；根包用它推出 player 文件的
	// 物理字节上界，故导出。
	EnvelopeLength = 44
	// MaxPayload 是单条 player 负载的解码分配上界；根包的文件读取上界由
	// EnvelopeLength + MaxPayload 组成。
	MaxPayload uint32 = 1 << 20

	playerEnvelopeVersion uint32 = 1
	// legacyPlayerHotbarBytes 是 schema v2/v3 的固定快捷栏负载长度。
	legacyPlayerHotbarBytes = 1 + core.HotbarSlots*3
	// legacyPlayerBackpackBytes 是 schema v3 的固定背包负载长度。
	legacyPlayerBackpackBytes = core.BackpackSlots * 3
	// playerHotbarBytes 是 schema v4 的固定快捷栏负载长度。
	playerHotbarBytes = 1 + core.HotbarSlots*5
	// playerBackpackBytes 是 schema v4 的固定背包负载长度。
	playerBackpackBytes = core.BackpackSlots * 5
	// playerHealthBytes 是 schema v5 起追加在负载末尾的生命值长度。
	playerHealthBytes = 1
	// playerHungerBytes 是 schema v7 起追加在生命值之后的三层饥饿状态长度：
	// 饥饿值 1 字节 + 饱和度 2 字节 + 疲劳值 2 字节（后两者小端 uint16）。
	//
	// 三字段追加在 Health **之后**而不是插进负载中段，是既有的追加纪律：解码按
	// "从末尾切走固定长度"逐层剥离（decodePlayerV8 → V7 → V5 → V4 → V1），只有
	// 末尾追加才能让旧层的切分点保持不变，冻结的旧 fixture 因此仍可解码。
	playerHungerBytes = 1 + 2 + 2
	// playerRespawnBytes 是 schema v8 起追加在饥饿状态之后的重生点长度：
	// present 1 字节 + 床尾格坐标 3×4 字节 + 维度 u32 4 字节，定长 17 字节。
	// 与 Safe 的「标志位 + 可选负载」变长编码不同：重生点恒占满 17 字节，
	// present=0 时位置与维度字节规范为零，解码无需二次定位。
	playerRespawnBytes = 1 + 12 + 4
)

var (
	playerEnvelopeMagic = [4]byte{'M', 'C', 'P', 'L'}
	playerCRCTable      = crc32.MakeTable(crc32.Castagnoli)
)

// Encode serializes one player as the stable MCPL v1 envelope.
func Encode(save PlayerSave) ([]byte, error) {
	if err := validatePlayerSave(save); err != nil {
		return nil, err
	}
	payload, err := encodePlayerV8(save)
	if err != nil {
		return nil, err
	}
	if uint64(len(payload)) > uint64(MaxPayload) {
		return nil, fmt.Errorf("%w: player payload exceeds %d bytes", storagedef.ErrCorrupt, MaxPayload)
	}
	encoded := make([]byte, 0, EnvelopeLength+len(payload))
	encoded = append(encoded, playerEnvelopeMagic[:]...)
	encoded = appendU32(encoded, playerEnvelopeVersion)
	encoded = appendU32(encoded, CurrentSchema)
	encoded = append(encoded, save.PlayerID[:]...)
	encoded = appendU64(encoded, save.Revision)
	encoded = appendU32(encoded, uint32(len(payload)))
	crcStart := len(encoded)
	encoded = appendU32(encoded, 0)
	hasher := crc32.New(playerCRCTable)
	_, _ = hasher.Write(encoded[8:crcStart])
	_, _ = hasher.Write(payload)
	binary.LittleEndian.PutUint32(encoded[crcStart:], hasher.Sum32())
	encoded = append(encoded, payload...)
	return encoded, nil
}

// Decode verifies the envelope and reconstructs an independent player,
// migrating older schemas up to the current one.
func Decode(wantID core.PlayerID, data []byte) (StoredPlayer, error) {
	if !wantID.Valid() {
		return StoredPlayer{}, fmt.Errorf("%w: invalid requested player ID", storagedef.ErrCorrupt)
	}
	envelope := byteDecoder{data: data}
	if err := envelope.magic(playerEnvelopeMagic); err != nil {
		return StoredPlayer{}, corrupt("player envelope magic", err)
	}
	version, err := envelope.u32()
	if err != nil {
		return StoredPlayer{}, corrupt("player envelope version", err)
	}
	if version != playerEnvelopeVersion {
		if version > playerEnvelopeVersion {
			return StoredPlayer{}, fmt.Errorf("%w: player envelope version %d", storagedef.ErrFutureVersion, version)
		}
		return StoredPlayer{}, fmt.Errorf("%w: unsupported player envelope version %d", storagedef.ErrCorrupt, version)
	}
	schema, err := envelope.u32()
	if err != nil {
		return StoredPlayer{}, corrupt("player schema", err)
	}
	if schema > CurrentSchema {
		return StoredPlayer{}, fmt.Errorf("%w: player schema %d", storagedef.ErrFutureVersion, schema)
	}
	if schema < oldestPlayerSchema {
		return StoredPlayer{}, fmt.Errorf("%w: unsupported player schema %d", storagedef.ErrCorrupt, schema)
	}
	encodedID, err := envelope.take(len(wantID))
	if err != nil {
		return StoredPlayer{}, corrupt("player ID", err)
	}
	var playerID core.PlayerID
	copy(playerID[:], encodedID)
	if !playerID.Valid() || playerID != wantID {
		return StoredPlayer{}, fmt.Errorf("%w: player ID does not match request", storagedef.ErrCorrupt)
	}
	revision, err := envelope.u64()
	if err != nil {
		return StoredPlayer{}, corrupt("player revision", err)
	}
	if revision == 0 {
		return StoredPlayer{}, fmt.Errorf("%w: zero player revision", storagedef.ErrCorrupt)
	}
	payloadLength, err := envelope.u32()
	if err != nil {
		return StoredPlayer{}, corrupt("player payload length", err)
	}
	if payloadLength > MaxPayload {
		return StoredPlayer{}, fmt.Errorf("%w: player payload length %d exceeds limit", storagedef.ErrCorrupt, payloadLength)
	}
	wantCRC, err := envelope.u32()
	if err != nil {
		return StoredPlayer{}, corrupt("player CRC32C", err)
	}
	if uint64(envelope.remaining()) != uint64(payloadLength) {
		return StoredPlayer{}, fmt.Errorf("%w: player payload length does not match envelope", storagedef.ErrCorrupt)
	}
	payload, err := envelope.take(int(payloadLength))
	if err != nil {
		return StoredPlayer{}, corrupt("player payload", err)
	}
	hasher := crc32.New(playerCRCTable)
	_, _ = hasher.Write(data[8:40])
	_, _ = hasher.Write(payload)
	if hasher.Sum32() != wantCRC {
		return StoredPlayer{}, fmt.Errorf("%w: player CRC32C", storagedef.ErrCorrupt)
	}
	dto, err := decodePlayerPayload(schema, playerID, revision, payload)
	if err != nil {
		return StoredPlayer{}, err
	}
	dto, migrated, err := migratePlayer(schema, dto)
	if err != nil {
		return StoredPlayer{}, err
	}
	if err := validatePlayerDTO(dto); err != nil {
		return StoredPlayer{}, err
	}
	stored := StoredPlayer{
		PlayerID: dto.PlayerID, Revision: dto.Revision, DisplayName: dto.DisplayName,
		Current: dto.Current, Yaw: dto.Yaw, Pitch: dto.Pitch, Inventory: dto.Inventory,
		Health:           dto.Health,
		Hunger:           dto.Hunger,
		SaturationMilli:  dto.SaturationMilli,
		ExhaustionMilli:  dto.ExhaustionMilli,
		RespawnPresent:   dto.RespawnPresent,
		RespawnPosition:  dto.RespawnPosition,
		RespawnDimension: dto.RespawnDimension,
		NeedsRewrite:     migrated,
	}
	if dto.Safe != nil {
		safe := *dto.Safe
		stored.Safe = &safe
	}
	return stored, nil
}

// encodePlayerV8 在 v7 负载末尾追加定长的重生点三字段（present + 床尾格 + 维度）。
//
// present=0 时位置与维度字节一律写零：它们不携带语义，规范为零让同一份逻辑
// 状态无论调用方留下什么残值都得到逐字节相同的编码，golden fixture 因此稳定。
func encodePlayerV8(save PlayerSave) ([]byte, error) {
	payload, err := encodePlayerV7(save)
	if err != nil {
		return nil, err
	}
	if !save.RespawnPresent {
		return append(payload, make([]byte, playerRespawnBytes)...), nil
	}
	payload = append(payload, 1)
	for _, position := range save.RespawnPosition {
		payload = appendF32(payload, position)
	}
	return binary.LittleEndian.AppendUint32(payload, uint32(save.RespawnDimension)), nil
}

// encodePlayerV7 在 v5 负载末尾追加三层饥饿状态（饥饿值、饱和度、疲劳值）。
//
// 没有 encodePlayerV6：v6 与 v5 的负载布局逐字节相同，v6 只扩展了合法物品
// 注册表，因此 v7 直接叠在 v5 之上。
func encodePlayerV7(save PlayerSave) ([]byte, error) {
	payload, err := encodePlayerV5(save)
	if err != nil {
		return nil, err
	}
	payload = append(payload, save.Hunger)
	payload = binary.LittleEndian.AppendUint16(payload, save.SaturationMilli)
	return binary.LittleEndian.AppendUint16(payload, save.ExhaustionMilli), nil
}

// encodePlayerV5 在 v4 负载末尾追加 1 字节生命值。
func encodePlayerV5(save PlayerSave) ([]byte, error) {
	payload, err := encodePlayerV4(save)
	if err != nil {
		return nil, err
	}
	return append(payload, save.Health), nil
}

// encodePlayerV4 把快捷栏与背包编码为包含耐久的固定负载。
func encodePlayerV4(save PlayerSave) ([]byte, error) {
	name := []byte(save.DisplayName)
	payload := make([]byte, 0,
		4+len(name)+4+3*4+4+4+1+4+3*4+playerHotbarBytes+playerBackpackBytes)
	payload = appendU32(payload, uint32(len(name)))
	payload = append(payload, name...)
	payload = appendPlayerLocation(payload, save.Current)
	payload = appendF32(payload, save.Yaw)
	payload = appendF32(payload, save.Pitch)
	if save.Safe == nil {
		payload = append(payload, 0)
	} else {
		payload = append(payload, 1)
		payload = appendPlayerLocation(payload, *save.Safe)
	}
	payload = appendPlayerHotbar(payload, save.Inventory.Hotbar)
	for _, stack := range save.Inventory.Backpack {
		payload = appendPlayerStack(payload, stack)
	}
	return payload, nil
}

func appendPlayerHotbar(dst []byte, hotbar core.Hotbar) []byte {
	dst = append(dst, hotbar.Selected)
	for _, stack := range hotbar.Slots {
		dst = appendPlayerStack(dst, stack)
	}
	return dst
}

func appendPlayerStack(dst []byte, stack core.ItemStack) []byte {
	dst = binary.LittleEndian.AppendUint16(dst, uint16(stack.Item))
	dst = append(dst, stack.Count)
	return binary.LittleEndian.AppendUint16(dst, stack.Durability)
}

func decodePlayerStack(decoder *byteDecoder) (core.ItemStack, error) {
	item, err := decoder.u16()
	if err != nil {
		return core.ItemStack{}, err
	}
	count, err := decoder.u8()
	if err != nil {
		return core.ItemStack{}, err
	}
	durability, err := decoder.u16()
	if err != nil {
		return core.ItemStack{}, err
	}
	return core.ItemStack{Item: core.ItemID(item), Count: count, Durability: durability}, nil
}

func decodeLegacyPlayerStack(decoder *byteDecoder) (core.ItemStack, error) {
	item, err := decoder.u16()
	if err != nil {
		return core.ItemStack{}, err
	}
	count, err := decoder.u8()
	if err != nil {
		return core.ItemStack{}, err
	}
	return core.ItemStack{Item: core.ItemID(item), Count: count}, nil
}

// decodePlayerPayload 按 schema 解析 payload；v1 没有快捷栏字段。
func decodePlayerPayload(
	schema uint32,
	playerID core.PlayerID,
	revision uint64,
	data []byte,
) (playerDTO, error) {
	switch schema {
	case 1:
		return decodePlayerV1(playerID, revision, data)
	case 2:
		return decodePlayerV2(playerID, revision, data)
	case 3:
		return decodePlayerV3(playerID, revision, data)
	case 4:
		return decodePlayerV4(playerID, revision, data)
	case 5, 6:
		// v6 与 v5 的负载布局相同（v6 只扩展了合法物品注册表），共用一个解码器；
		// 两者都没有饥饿字段，缺口由 migratePlayer 的 v6 迁移补初值。
		return decodePlayerV5(playerID, revision, data)
	case 7:
		return decodePlayerV7(playerID, revision, data)
	case 8:
		return decodePlayerV8(playerID, revision, data)
	default:
		return playerDTO{}, fmt.Errorf("%w: unsupported player schema %d", storagedef.ErrCorrupt, schema)
	}
}

// decodePlayerV8 在 v7 解析结果之上剥离定长的重生点尾巴（present + 床尾格 + 维度）。
//
// present=0 时位置与维度字节不携带语义，读入即规范为零；只有 present=1 才
// 校验维度与坐标的合法性——越界值由 validatePlayerDTO 统一拒绝。
func decodePlayerV8(playerID core.PlayerID, revision uint64, data []byte) (playerDTO, error) {
	if len(data) < playerRespawnBytes {
		return playerDTO{}, fmt.Errorf("%w: player payload is shorter than the respawn point", storagedef.ErrCorrupt)
	}
	split := len(data) - playerRespawnBytes
	dto, err := decodePlayerV7(playerID, revision, data[:split])
	if err != nil {
		return playerDTO{}, err
	}
	// 长度已在上面校验，这里直接按固定偏移读；越界不可能发生。
	tail := data[split:]
	switch tail[0] {
	case 0:
	case 1:
		dto.RespawnPresent = true
		for index := range dto.RespawnPosition {
			dto.RespawnPosition[index] = math.Float32frombits(
				binary.LittleEndian.Uint32(tail[1+4*index:]),
			)
		}
		dto.RespawnDimension = core.DimensionID(int32(binary.LittleEndian.Uint32(tail[13:])))
	default:
		return playerDTO{}, fmt.Errorf("%w: invalid player respawn flag %d", storagedef.ErrCorrupt, tail[0])
	}
	return dto, nil
}

// decodePlayerV7 在 v5 解析结果之上追加三层饥饿状态。
func decodePlayerV7(playerID core.PlayerID, revision uint64, data []byte) (playerDTO, error) {
	if len(data) < playerHungerBytes {
		return playerDTO{}, fmt.Errorf("%w: player payload is shorter than the hunger state", storagedef.ErrCorrupt)
	}
	split := len(data) - playerHungerBytes
	dto, err := decodePlayerV5(playerID, revision, data[:split])
	if err != nil {
		return playerDTO{}, err
	}
	// 长度已在上面校验，这里直接按固定偏移读；越界不可能发生。
	tail := data[split:]
	dto.Hunger = tail[0]
	dto.SaturationMilli = binary.LittleEndian.Uint16(tail[1:])
	dto.ExhaustionMilli = binary.LittleEndian.Uint16(tail[3:])
	return dto, nil
}

// decodePlayerV5 在 v4 解析结果之上追加 1 字节生命值。
func decodePlayerV5(playerID core.PlayerID, revision uint64, data []byte) (playerDTO, error) {
	if len(data) < playerHealthBytes {
		return playerDTO{}, fmt.Errorf("%w: player payload is shorter than health", storagedef.ErrCorrupt)
	}
	split := len(data) - playerHealthBytes
	dto, err := decodePlayerV4(playerID, revision, data[:split])
	if err != nil {
		return playerDTO{}, err
	}
	dto.Health = data[split]
	return dto, nil
}

func decodePlayerV4(playerID core.PlayerID, revision uint64, data []byte) (playerDTO, error) {
	inventoryBytes := playerHotbarBytes + playerBackpackBytes
	if len(data) < inventoryBytes {
		return playerDTO{}, fmt.Errorf("%w: player payload is shorter than the inventory", storagedef.ErrCorrupt)
	}
	split := len(data) - inventoryBytes
	dto, err := decodePlayerV1(playerID, revision, data[:split])
	if err != nil {
		return playerDTO{}, err
	}
	decoder := byteDecoder{data: data[split:]}
	selected, err := decoder.u8()
	if err != nil {
		return playerDTO{}, corrupt("player hotbar selection", err)
	}
	dto.Inventory.Hotbar.Selected = selected
	for index := range dto.Inventory.Hotbar.Slots {
		stack, err := decodePlayerStack(&decoder)
		if err != nil {
			return playerDTO{}, corrupt("player hotbar slot", err)
		}
		dto.Inventory.Hotbar.Slots[index] = stack
	}
	for index := range dto.Inventory.Backpack {
		stack, err := decodePlayerStack(&decoder)
		if err != nil {
			return playerDTO{}, corrupt("player backpack slot", err)
		}
		dto.Inventory.Backpack[index] = stack
	}
	return dto, nil
}

func decodePlayerV3(playerID core.PlayerID, revision uint64, data []byte) (playerDTO, error) {
	if len(data) < legacyPlayerBackpackBytes {
		return playerDTO{}, fmt.Errorf("%w: player payload is shorter than the backpack", storagedef.ErrCorrupt)
	}
	split := len(data) - legacyPlayerBackpackBytes
	dto, err := decodePlayerV2(playerID, revision, data[:split])
	if err != nil {
		return playerDTO{}, err
	}
	decoder := byteDecoder{data: data[split:]}
	for index := range dto.Inventory.Backpack {
		stack, err := decodeLegacyPlayerStack(&decoder)
		if err != nil {
			return playerDTO{}, corrupt("player backpack slot", err)
		}
		dto.Inventory.Backpack[index] = stack
	}
	return dto, nil
}

func decodePlayerV2(playerID core.PlayerID, revision uint64, data []byte) (playerDTO, error) {
	if len(data) < legacyPlayerHotbarBytes {
		return playerDTO{}, fmt.Errorf("%w: player payload is shorter than the hotbar", storagedef.ErrCorrupt)
	}
	split := len(data) - legacyPlayerHotbarBytes
	dto, err := decodePlayerV1(playerID, revision, data[:split])
	if err != nil {
		return playerDTO{}, err
	}
	decoder := byteDecoder{data: data[split:]}
	selected, err := decoder.u8()
	if err != nil {
		return playerDTO{}, corrupt("player hotbar selection", err)
	}
	dto.Inventory.Hotbar.Selected = selected
	for index := range dto.Inventory.Hotbar.Slots {
		stack, err := decodeLegacyPlayerStack(&decoder)
		if err != nil {
			return playerDTO{}, corrupt("player hotbar slot", err)
		}
		dto.Inventory.Hotbar.Slots[index] = stack
	}
	return dto, nil
}

func decodePlayerV1(playerID core.PlayerID, revision uint64, data []byte) (playerDTO, error) {
	decoder := byteDecoder{data: data}
	nameLength, err := decoder.u32()
	if err != nil {
		return playerDTO{}, corrupt("player name length", err)
	}
	if uint64(nameLength) > uint64(decoder.remaining()) {
		return playerDTO{}, fmt.Errorf("%w: player name length does not match payload", storagedef.ErrCorrupt)
	}
	name, err := decoder.take(int(nameLength))
	if err != nil {
		return playerDTO{}, corrupt("player name", err)
	}
	current, err := decodePlayerLocation(&decoder)
	if err != nil {
		return playerDTO{}, corrupt("player current location", err)
	}
	yaw, err := decodeF32(&decoder)
	if err != nil {
		return playerDTO{}, corrupt("player yaw", err)
	}
	pitch, err := decodeF32(&decoder)
	if err != nil {
		return playerDTO{}, corrupt("player pitch", err)
	}
	hasSafe, err := decoder.u8()
	if err != nil {
		return playerDTO{}, corrupt("player safe flag", err)
	}
	dto := playerDTO{PlayerID: playerID, Revision: revision, DisplayName: string(name), Current: current, Yaw: yaw, Pitch: pitch}
	switch hasSafe {
	case 0:
	case 1:
		safe, err := decodePlayerLocation(&decoder)
		if err != nil {
			return playerDTO{}, corrupt("player safe location", err)
		}
		dto.Safe = &safe
	default:
		return playerDTO{}, fmt.Errorf("%w: invalid player safe flag %d", storagedef.ErrCorrupt, hasSafe)
	}
	if decoder.remaining() != 0 {
		return playerDTO{}, fmt.Errorf("%w: trailing player payload bytes", storagedef.ErrCorrupt)
	}
	return dto, nil
}

func appendPlayerLocation(dst []byte, location PlayerLocation) []byte {
	dst = appendU32(dst, uint32(location.Dimension))
	for _, position := range location.Position {
		dst = appendF32(dst, position)
	}
	return dst
}

func decodePlayerLocation(decoder *byteDecoder) (PlayerLocation, error) {
	dimension, err := decoder.u32()
	if err != nil {
		return PlayerLocation{}, err
	}
	location := PlayerLocation{Dimension: core.DimensionID(int32(dimension))}
	for index := range location.Position {
		position, err := decodeF32(decoder)
		if err != nil {
			return PlayerLocation{}, err
		}
		location.Position[index] = position
	}
	return location, nil
}

func appendF32(dst []byte, value float32) []byte {
	return binary.LittleEndian.AppendUint32(dst, math.Float32bits(value))
}

func decodeF32(decoder *byteDecoder) (float32, error) {
	bits, err := decoder.u32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(bits), nil
}

func validatePlayerSave(save PlayerSave) error {
	return validatePlayerDTO(playerDTO{
		PlayerID: save.PlayerID, Revision: save.Revision, DisplayName: save.DisplayName,
		Current: save.Current, Yaw: save.Yaw, Pitch: save.Pitch, Safe: save.Safe,
		Inventory: save.Inventory, Health: save.Health,
		Hunger: save.Hunger, SaturationMilli: save.SaturationMilli,
		ExhaustionMilli:  save.ExhaustionMilli,
		RespawnPresent:   save.RespawnPresent,
		RespawnPosition:  save.RespawnPosition,
		RespawnDimension: save.RespawnDimension,
	})
}

func validatePlayerDTO(dto playerDTO) error {
	if !dto.PlayerID.Valid() {
		return fmt.Errorf("%w: invalid player ID", storagedef.ErrCorrupt)
	}
	if dto.Revision == 0 {
		return fmt.Errorf("%w: zero player revision", storagedef.ErrCorrupt)
	}
	name, err := core.NormalizeDisplayName(dto.DisplayName)
	if err != nil || name != dto.DisplayName {
		return fmt.Errorf("%w: invalid player display name", storagedef.ErrCorrupt)
	}
	if err := validatePlayerLocation(dto.Current); err != nil {
		return err
	}
	if !finitePlayerFloat(dto.Yaw) {
		return fmt.Errorf("%w: non-finite player yaw", storagedef.ErrCorrupt)
	}
	if !finitePlayerFloat(dto.Pitch) || dto.Pitch < -math.Pi/2 || dto.Pitch > math.Pi/2 {
		return fmt.Errorf("%w: invalid player pitch", storagedef.ErrCorrupt)
	}
	if dto.Safe != nil {
		if err := validatePlayerLocation(*dto.Safe); err != nil {
			return err
		}
	}
	if !dto.Inventory.Valid() {
		return fmt.Errorf("%w: invalid player inventory", storagedef.ErrCorrupt)
	}
	if !core.ValidHealth(dto.Health) {
		return fmt.Errorf("%w: invalid player health", storagedef.ErrCorrupt)
	}
	if !core.ValidHunger(dto.Hunger) {
		return fmt.Errorf("%w: invalid player hunger", storagedef.ErrCorrupt)
	}
	// 饱和度是饥饿值之上的缓冲，上界就是饥饿值本身（千分位）。乘积至多
	// 20×1000，远在 uint16 之内，不会溢出。疲劳值没有静态上界：阈值是 tunable，
	// uint16 的全域都是合法取值。
	if dto.SaturationMilli > uint16(dto.Hunger)*core.SaturationMilliPerPoint {
		return fmt.Errorf("%w: player saturation exceeds hunger", storagedef.ErrCorrupt)
	}
	// 重生点只在 present=1 时校验：位置与维度复用 PlayerLocation 的同一条
	// 规则（维度受支持、坐标有限），present=0 时三个字段都不携带语义。
	if dto.RespawnPresent {
		if err := validatePlayerLocation(PlayerLocation{
			Dimension: dto.RespawnDimension,
			Position:  dto.RespawnPosition,
		}); err != nil {
			return err
		}
	}
	return nil
}

func validatePlayerLocation(location PlayerLocation) error {
	if location.Dimension != core.Overworld {
		return fmt.Errorf("%w: unsupported player dimension %d", storagedef.ErrCorrupt, location.Dimension)
	}
	for _, position := range location.Position {
		if !finitePlayerFloat(position) {
			return fmt.Errorf("%w: non-finite player position", storagedef.ErrCorrupt)
		}
	}
	return nil
}

func finitePlayerFloat(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
