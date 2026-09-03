package companion

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestSnapshotDigestTerrainWireAndDeterminism(t *testing.T) {
	snapshot := testSnapshot()
	canonical, digest, err := CanonicalSnapshotDigest(snapshot)
	if err != nil {
		t.Fatalf("CanonicalSnapshotDigest: %v", err)
	}
	again, againDigest, err := CanonicalSnapshotDigest(snapshot)
	if err != nil || !bytes.Equal(again, canonical) || againDigest != digest {
		t.Fatalf("相同快照的 canonical/digest 不确定: err=%v digest=%q/%q", err, digest, againDigest)
	}
	if len(canonical) > MaxSnapshotDigestBytes {
		t.Fatalf("完整 digest 输入=%d，超过 %d", len(canonical), MaxSnapshotDigestBytes)
	}
	if len(digest) != 64 {
		t.Fatalf("digest 长度=%d，want 64", len(digest))
	}
	const wantSnapshotDigest = "b1bcb780f32b59233983c3bc06fb662ffd42260cba9d6c7cf964c146fd7a8a50"
	if digest != wantSnapshotDigest {
		t.Fatalf("snapshot canonical golden digest=%q，want %q", digest, wantSnapshotDigest)
	}
	if bytes.Contains(canonical, []byte(`"heights":`)) {
		t.Fatalf("digest 重复编码 legacy Heights: %s", canonical)
	}
	if !bytes.Contains(canonical, []byte(`"heights_be_i16_b64":`)) {
		t.Fatalf("digest 缺少 dense height plane")
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &root); err != nil {
		t.Fatalf("解析 canonical digest: %v", err)
	}
	var terrain struct {
		Blocks     string        `json:"blocks_be_u16_b64"`
		Dimensions [3]int        `json:"dimensions"`
		Heights    string        `json:"heights_be_i16_b64"`
		Origin     core.BlockPos `json:"origin"`
		Ready      string        `json:"ready_columns_b64"`
	}
	if err := json.Unmarshal(root["terrain"], &terrain); err != nil {
		t.Fatalf("解析 terrain digest: %v", err)
	}
	if terrain.Dimensions != [3]int{TerrainWidth, TerrainHeight, TerrainDepth} ||
		terrain.Origin != snapshot.Terrain.Origin() {
		t.Fatalf("terrain metadata=%+v/%+v", terrain.Dimensions, terrain.Origin)
	}
	ready := decodeDigestPlane(t, terrain.Ready, TerrainReadyBitmapBytes)
	heights := decodeDigestPlane(t, terrain.Heights, TerrainColumnCount*2)
	blocks := decodeDigestPlane(t, terrain.Blocks, TerrainBlockCount*2)
	if ready[len(ready)-1]&0xfe != 0 {
		t.Fatalf("ready bitmap unused bits=%08b", ready[len(ready)-1])
	}
	if got := int16(binary.BigEndian.Uint16(heights[:2])); got != 64 {
		t.Fatalf("首列 height=%d，want 64", got)
	}
	blockOffset := ((18*TerrainHeight+6)*TerrainDepth + 14) * 2
	if got := core.BlockID(binary.BigEndian.Uint16(blocks[blockOffset : blockOffset+2])); got != core.GrassID {
		t.Fatalf("(8,63,-2) block=%d，want grass", got)
	}
	terrainCanonical, err := CanonicalTerrainDigest(snapshot.Terrain)
	if err != nil {
		t.Fatalf("CanonicalTerrainDigest: %v", err)
	}
	if len(terrainCanonical) >= MaxTerrainDigestBytes {
		t.Fatalf("terrain canonical=%d，必须小于 %d", len(terrainCanonical), MaxTerrainDigestBytes)
	}
	terrainSum := sha256.Sum256(terrainCanonical)
	const wantTerrainDigest = "2942dddd4743b1c783b153a3b4e2f5e7026584844bb741bff2a6135ca350f363"
	if got := hex.EncodeToString(terrainSum[:]); got != wantTerrainDigest {
		t.Fatalf("terrain canonical golden digest=%q，want %q", got, wantTerrainDigest)
	}

	legacyOnly := snapshot
	legacyOnly.Heights = []PlanHeight{{X: 123, Z: 456, Height: core.MinY - 1}}
	_, legacyDigest, err := CanonicalSnapshotDigest(legacyOnly)
	if err != nil || legacyDigest != digest {
		t.Fatalf("legacy Heights 不应改变 digest: err=%v got=%q want=%q", err, legacyDigest, digest)
	}
	changed := snapshot
	if !changed.Terrain.SetBlock(core.BlockPos{X: 6, Y: 64, Z: 0}, core.DirtID) {
		t.Fatal("修改 terrain 失败")
	}
	_, changedDigest, err := CanonicalSnapshotDigest(changed)
	if err != nil || changedDigest == digest {
		t.Fatalf("terrain 变化未改变 digest: err=%v digest=%q", err, changedDigest)
	}
}

func TestSnapshotDigestCanonicalKeyOrderAndBounds(t *testing.T) {
	canonical, _, err := CanonicalSnapshotDigest(testSnapshot())
	if err != nil {
		t.Fatalf("CanonicalSnapshotDigest: %v", err)
	}
	if !strings.HasPrefix(string(canonical), `{"chunk_revisions":`) {
		t.Fatalf("canonical object 未按 key 排序: %.80s", canonical)
	}
	var decoded any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatalf("canonical JSON 非法: %v", err)
	}
	remarshaled, err := canonicalJSON(decoded)
	if err != nil || !bytes.Equal(remarshaled, canonical) {
		t.Fatalf("canonical JSON 非幂等: err=%v", err)
	}
}

func decodeDigestPlane(t *testing.T, encoded string, want int) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 plane: %v", err)
	}
	if len(decoded) != want {
		t.Fatalf("plane bytes=%d，want %d", len(decoded), want)
	}
	if base64.StdEncoding.EncodeToString(decoded) != encoded {
		t.Fatal("plane 不是 RFC 4648 padded standard Base64")
	}
	return decoded
}
