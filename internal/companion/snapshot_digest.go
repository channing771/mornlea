package companion

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/channing771/mornlea/internal/core"
)

const (
	// MaxTerrainDigestBytes 是 terrain DTO 规范 JSON 的严格独占上界。
	MaxTerrainDigestBytes = 53 << 10
	// MaxSnapshotDigestBytes 是完整快照 digest 输入的包含式上界。
	MaxSnapshotDigestBytes = 96 << 10
)

type digestBlockPosition struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
	Z int32 `json:"z"`
}

type digestTerrain struct {
	BlocksBEU16B64  string              `json:"blocks_be_u16_b64"`
	Dimensions      [3]int              `json:"dimensions"`
	HeightsBEI16B64 string              `json:"heights_be_i16_b64"`
	Origin          digestBlockPosition `json:"origin"`
	ReadyColumnsB64 string              `json:"ready_columns_b64"`
}

type digestPlayer struct {
	HasLookHit bool                 `json:"has_look_hit"`
	LookHit    *digestBlockPosition `json:"look_hit"`
	Pitch      float32              `json:"pitch"`
	PlayerID   string               `json:"player_id"`
	Position   [3]float32           `json:"position"`
	Yaw        float32              `json:"yaw"`
}

type digestInventorySlot struct {
	Count      uint8       `json:"count"`
	Durability uint16      `json:"durability"`
	ItemID     core.ItemID `json:"item_id"`
	Slot       uint8       `json:"slot"`
}

type digestCompanion struct {
	CompanionID string                `json:"companion_id"`
	Inventory   []digestInventorySlot `json:"inventory"`
	Pitch       float32               `json:"pitch"`
	Position    [3]float32            `json:"position"`
	TaskStatus  string                `json:"task_status"`
	Yaw         float32               `json:"yaw"`
}

type digestVisibleBlock struct {
	BlockID  core.BlockID        `json:"block_id"`
	Position digestBlockPosition `json:"position"`
}

type digestChunkRevision struct {
	Revision uint64 `json:"revision"`
	X        int32  `json:"x"`
	Z        int32  `json:"z"`
}

type digestSnapshot struct {
	ChunkRevisions []digestChunkRevision `json:"chunk_revisions"`
	Companion      digestCompanion       `json:"companion"`
	ExposedBlocks  []digestVisibleBlock  `json:"exposed_blocks"`
	Instruction    string                `json:"instruction"`
	Issuer         digestPlayer          `json:"issuer"`
	OnlinePlayers  []digestPlayer        `json:"online_players"`
	Terrain        digestTerrain         `json:"terrain"`
	WorldTimeTicks uint64                `json:"world_time_ticks"`
}

// CanonicalTerrainDigest 把紧凑投影编码为 digest 专用 snake_case DTO。二进制
// plane 固定使用 big-endian 与 RFC 4648 padded standard Base64。
func CanonicalTerrainDigest(projection TerrainProjection) ([]byte, error) {
	if err := projection.Validate(); err != nil {
		return nil, err
	}
	dto := buildDigestTerrain(projection)
	canonical, err := canonicalJSON(dto)
	if err != nil {
		return nil, fmt.Errorf("companion: 编码 terrain digest: %w", err)
	}
	if len(canonical) >= MaxTerrainDigestBytes {
		return nil, fmt.Errorf("companion: terrain digest %d bytes 达到上限 %d", len(canonical), MaxTerrainDigestBytes)
	}
	return canonical, nil
}

// CanonicalSnapshotDigest 返回快照专用 canonical JSON 与其小写 SHA-256。
// legacy `Heights` 不进入 DTO，dense height plane 是唯一高度表达。
func CanonicalSnapshotDigest(snapshot PlanSnapshot) ([]byte, string, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, "", err
	}
	dto := buildDigestSnapshot(snapshot)
	canonical, err := canonicalJSON(dto)
	if err != nil {
		return nil, "", fmt.Errorf("companion: 编码 snapshot digest: %w", err)
	}
	if len(canonical) > MaxSnapshotDigestBytes {
		return nil, "", fmt.Errorf("companion: snapshot digest %d bytes 超过上限 %d", len(canonical), MaxSnapshotDigestBytes)
	}
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}

func buildDigestTerrain(projection TerrainProjection) digestTerrain {
	heights := make([]byte, TerrainColumnCount*2)
	for index, height := range projection.heights {
		binary.BigEndian.PutUint16(heights[index*2:], uint16(height))
	}
	blocks := make([]byte, TerrainBlockCount*2)
	for index, block := range projection.blocks {
		binary.BigEndian.PutUint16(blocks[index*2:], uint16(block))
	}
	origin := projection.Origin()
	return digestTerrain{
		BlocksBEU16B64:  base64.StdEncoding.EncodeToString(blocks),
		Dimensions:      [3]int{TerrainWidth, TerrainHeight, TerrainDepth},
		HeightsBEI16B64: base64.StdEncoding.EncodeToString(heights),
		Origin:          digestPosition(origin),
		ReadyColumnsB64: base64.StdEncoding.EncodeToString(projection.readyColumns[:]),
	}
}

func buildDigestSnapshot(snapshot PlanSnapshot) digestSnapshot {
	dto := digestSnapshot{
		ChunkRevisions: make([]digestChunkRevision, len(snapshot.ChunkRevisions)),
		Companion: digestCompanion{
			CompanionID: snapshot.Companion.ID.String(),
			Inventory:   make([]digestInventorySlot, 0, core.InventorySlots),
			Pitch:       snapshot.Companion.Pitch,
			Position:    snapshot.Companion.Position,
			TaskStatus:  snapshot.Companion.TaskStatus,
			Yaw:         snapshot.Companion.Yaw,
		},
		ExposedBlocks:  make([]digestVisibleBlock, len(snapshot.ExposedBlocks)),
		Instruction:    snapshot.Command,
		Issuer:         digestPlanPlayer(snapshot.Issuer),
		OnlinePlayers:  make([]digestPlayer, len(snapshot.OnlinePlayers)),
		Terrain:        buildDigestTerrain(snapshot.Terrain),
		WorldTimeTicks: snapshot.WorldTimeTicks,
	}
	for slot := uint8(0); slot < core.InventorySlots; slot++ {
		stack, _ := snapshot.Companion.Inventory.Slot(slot)
		if stack.Item == core.ItemNone {
			continue
		}
		dto.Companion.Inventory = append(dto.Companion.Inventory, digestInventorySlot{
			Count: stack.Count, Durability: stack.Durability, ItemID: stack.Item, Slot: slot,
		})
	}
	for index, revision := range snapshot.ChunkRevisions {
		dto.ChunkRevisions[index] = digestChunkRevision{
			Revision: revision.Revision,
			X:        revision.Chunk.X,
			Z:        revision.Chunk.Z,
		}
	}
	for index, block := range snapshot.ExposedBlocks {
		dto.ExposedBlocks[index] = digestVisibleBlock{
			BlockID: block.Block, Position: digestPosition(block.Pos),
		}
	}
	for index, player := range snapshot.OnlinePlayers {
		dto.OnlinePlayers[index] = digestPlanPlayer(player)
	}
	return dto
}

func digestPlanPlayer(player PlanPlayer) digestPlayer {
	result := digestPlayer{
		HasLookHit: player.HasLookHit,
		Pitch:      player.Pitch,
		PlayerID:   player.ID.String(),
		Position:   player.Position,
		Yaw:        player.Yaw,
	}
	if player.HasLookHit {
		look := digestPosition(player.LookHit)
		result.LookHit = &look
	}
	return result
}

func digestPosition(pos core.BlockPos) digestBlockPosition {
	return digestBlockPosition{X: pos.X, Y: pos.Y, Z: pos.Z}
}

// canonicalJSON 使用标准 JSON 值模型把 object key 按字典序编码，禁用 HTML
// 转义并拒绝非有限数字；输出为紧凑 UTF-8 且不带尾随换行。
func canonicalJSON(value any) ([]byte, error) {
	initial, err := encodeCanonicalJSON(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(initial))
	decoder.UseNumber()
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return encodeCanonicalJSON(generic)
}

func encodeCanonicalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON 存在尾随值")
		}
		return err
	}
	return nil
}
