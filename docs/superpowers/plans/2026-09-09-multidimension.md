# Multidimension Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Boot a second authoritative dimension `Depths` with warp round-trip and isolated persistence.

**Architecture:** Reuse existing half-built multidim plumbing (`ChunkKey{Dimension,Pos}`, `dimensions/{dim}/regions/`, `realm.State.dimensions`, mirror multidim map); tick stays serial `for dim in [0,1]`; teleport is a single-tick atomic transaction triggered by `ChatCommand`; protocol v38->v39 only relaxes validation, no new packet IDs; Rust engine untouched.

**Tech Stack:** Go 1.26 workspaces (`packages/shared`, `packages/server`), existing `nativeabi` worldgen bridge, zstd chunk codec, `MCGM` metadata envelope.

**Spec:** `docs/superpowers/specs/2026-09-09-multidimension-design.md`

## Global Constraints

- 服务端是唯一权威，客户端只跟随 `PlayerState{Dimension,Reset:true}`，不做 UI/渲染改动。
- 权威 tick 保持串行，不引入并行 tick；tick 内不阻塞磁盘/网络，生成与落盘走有界队列与 worker。
- 协议 `ProtocolVersion` 由 38 升至 39，只放宽维度校验，不新增 packet ID，不改既有编号。
- 存档 `metadata` 由 v4 升至 v5（尾部追加），读 v1..v5，写只写 v5；chunk schema 保持 v9，engine ABI 保持 v10。
- 任何 Go 包不得导入 WebGPU 绑定；只有 `packages/shared/nativeabi` 接触 engine ABI。
- 跨 goroutine 发送成功后的消息及其 slice 视为不可变。
- 伙伴/敌怪/被动生物本期不跨维度，其消息继续只允许 `Overworld`。

---

## File Map

- `packages/shared/core/block.go`: 新增 `Depths DimensionID = 1`，维度值域注释。
- `packages/server/sim/runtime/engine.go`: `NewEngine` 启动 `EnsureDimension(Depths)`，tick 双维串行。
- `packages/server/sim/realm/state.go`: 双维记账已就绪，本计划只补越界守卫与测试。
- `packages/shared/worldgen/generator.go`: `GenerateChunk(dim,pos)` + `dimSeed` 派生，保持 `MGW1` 布局。
- `packages/server/server/generator.go`: `Generator` 接口加维度参数，`runGeneration` 透传。
- `packages/server/storage/metadata.go` + `packages/server/storage/types.go`: metadata v5 编解码与迁移。
- `packages/server/storage/chunk/chunk_codec.go`: 允许 `dimension 0/1`（信封字段已存在）。
- `packages/shared/network/protocol/packet.go`: `ProtocolVersion 38->39` + 版本历史注释。
- `packages/shared/network/protocol/message_*.go` + `packages/shared/network/codec/codec_server.go`: 放行玩家/区块类 `0/1`，伙伴系继续拒非 0。
- `packages/server/sim/entity/sleep.go` + `spawn*`: 新维出生扫描与床重生回落（旧维锚点）。
- `packages/server/server/` 传送装配（复用 `ChatCommand` 解析）：新文件 `warp.go` + 测试 `warp_test.go`。

---

### Task 1: Core second dimension constant and engine boot

**Files:**
- Modify: `packages/shared/core/block.go`
- Modify: `packages/server/sim/runtime/engine.go`
- Test: `packages/server/sim/runtime/runtime_test.go` (add, do not rewrite)

**Interfaces:**
- Consumes: `realm.NewState(core.Overworld)`, `core.ChunkKey{Dimension,Pos}`
- Produces: `core.Depths DimensionID` (=1), `Engine` boots dimensions `[Overworld,Depths]`

- [ ] **Step 1: Write the failing test**

```go
func TestEngineBootsTwoDimensions(t *testing.T) {
    engine := NewEngine(2, 0, 1234)
    if engine.dimension(core.Overworld) == nil {
        t.Fatal("overworld missing")
    }
    if engine.dimension(core.Depths) == nil {
        t.Fatal("depths missing")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/server/sim/runtime -race -count=1 -run TestEngineBootsTwoDimensions`
Expected: FAIL (`Depths` undefined / nil dimension)

- [ ] **Step 3: Write minimal implementation**

```go
// packages/shared/core/block.go
const Depths DimensionID = 1
```

```go
// packages/server/sim/runtime/engine.go
realmState := realm.NewState(core.Overworld)
realmState.EnsureDimension(core.Depths)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./packages/server/sim/runtime -race -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add packages/shared/core/block.go packages/server/sim/runtime/engine.go packages/server/sim/runtime/runtime_test.go
git commit -m "feat(sim): boot depths dimension alongside overworld"
```

---

### Task 2: Dimension-aware worldgen and server Generator

**Files:**
- Modify: `packages/shared/worldgen/generator.go`
- Modify: `packages/server/server/generator.go`
- Test: `packages/shared/worldgen/generator_test.go` (add)

**Interfaces:**
- Consumes: `core.Depths`, `worldgen.New(seed, fluidEnabled)` header builder
- Produces: `func (g *Generator) GenerateChunk(dim core.DimensionID, pos core.ChunkPos) *world.Chunk`, `type Generator interface { GenerateChunk(core.DimensionID, core.ChunkPos) *world.Chunk }`

- [ ] **Step 1: Write the failing test**

```go
func TestGenerateChunkDimensionSaltDiverges(t *testing.T) {
    g := New(42, false)
    overworld := g.GenerateChunk(core.Overworld, core.ChunkPos{X: 0, Z: 0})
    depths := g.GenerateChunk(core.Depths, core.ChunkPos{X: 0, Z: 0})
    if overworld == nil || depths == nil {
        t.Fatal("nil chunk")
    }
    if overworld.Pos != depths.Pos {
        t.Fatal("pos must match, dimension differs")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/shared/worldgen -race -count=1 -run TestGenerateChunkDimensionSaltDiverges`
Expected: FAIL (method signature mismatch)

- [ ] **Step 3: Write minimal implementation**

```go
func dimSeed(base int64, dim core.DimensionID) int64 {
    if dim == core.Depths {
        return base ^ 0x9E3779B97F4A7C15
    }
    return base
}
```

`New` keeps signature; add `NewForDimension(seed, fluidEnabled, dim)` building the same 566B `MGW1` header with `dimSeed`, and `GenerateChunk(dim, pos)` dispatching to the per-dimension header. `runGeneration` passes `key.Dimension` into `generator.GenerateChunk(dim, pos)` instead of only `key.Pos`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./packages/shared/worldgen -race -count=1`
Expected: PASS

- [ ] **Step 5: Fix server call sites to new signature**

Run: `go test ./packages/server/server -race -count=1 -run TestGenerator`
Expected: PASS (update `flatTestGenerator` and siblings to `GenerateChunk(dim, pos)`, ignoring `dim` where the test is single-dimension)

- [ ] **Step 6: Commit**

```bash
git add packages/shared/worldgen/generator.go packages/shared/worldgen/generator_test.go packages/server/server/generator.go packages/server/server/*_test.go
git commit -m "feat(worldgen): derive depths terrain from salted seed"
```

---

### Task 3: Storage metadata v5 and chunk dimension allowlist

**Files:**
- Modify: `packages/server/storage/metadata.go`
- Modify: `packages/server/storage/types.go`
- Modify: `packages/server/storage/chunk/chunk_codec.go`
- Test: `packages/server/storage/metadata_test.go` (add round-trip)

**Interfaces:**
- Consumes: `Metadata{Seed, SpawnDimension, SpawnAnchor, ...}`
- Produces: metadata v5 read v1..v5 / write v5 only; chunk decode accepts `dimension 0/1`

- [ ] **Step 1: Write the failing test**

```go
func TestMetadataV4MigratesToV5DepthsDefault(t *testing.T) {
    v4 := Metadata{FormatVersion: 4, Seed: 42, SpawnDimension: core.Overworld}
    raw := mustEncodeForTest(t, v4)
    loaded := mustDecodeForTest(t, raw)
    if loaded.FormatVersion != 5 {
        t.Fatalf("got version %d", loaded.FormatVersion)
    }
    if loaded.DepthsSpawnAnchor != loaded.SpawnAnchor {
        t.Fatal("depths anchor must default to overworld anchor")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/server/storage -race -count=1 -run TestMetadataV4MigratesToV5DepthsDefault`
Expected: FAIL (v5 unknown)

- [ ] **Step 3: Write minimal implementation**

v5 payload = v4 41B + `dimCount u32 (=2)` + `depthsSpawnX/Z u32/u32` + `depthsSeedSalt u64`. `decodeMetadata` switches v1/v2/v3/v4/v5, missing tail zeroes to defaults (anchor = overworld anchor, salt = `0x9E3779B97F4A7C15`). `encodeMetadata` writes v5 only. Chunk codec dimension check `dimension <= 1` instead of `== Overworld`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./packages/server/storage/... -race -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add packages/server/storage/metadata.go packages/server/storage/types.go packages/server/storage/chunk/chunk_codec.go packages/server/storage/metadata_test.go
git commit -m "feat(storage): migrate world metadata to v5 for depths"
```

---

### Task 4: Protocol v39 dimension validation relaxation

**Files:**
- Modify: `packages/shared/network/protocol/packet.go`
- Modify: `packages/shared/network/protocol/message_player.go`
- Modify: `packages/shared/network/protocol/message_command.go`
- Modify: `packages/shared/network/codec/codec_server.go`
- Test: `packages/shared/network/protocol/message_test.go` (add matrix rows)

**Interfaces:**
- Consumes: existing `Dimension int32` wire fields on `ChunkSnapshot/BlockChanges/ForgetChunks/RequestChunkResync/PlayerState/RemotePlayer*`
- Produces: `ProtocolVersion=39`; player/chunk classes accept `0/1`, companion/hostile/passive still reject non-zero

- [ ] **Step 1: Write the failing test**

```go
func TestPlayerChunkMessagesAcceptDepths(t *testing.T) {
    if err := (PlayerState{Dimension: core.Depths}).Validate(); err != nil {
        t.Fatalf("depths playerstate must validate: %v", err)
    }
    if err := (PlayerState{Dimension: core.DimensionID(2)}).Validate(); err == nil {
        t.Fatal("dimension 2 must reject")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/shared/network/protocol -race -count=1 -run TestPlayerChunkMessagesAcceptDepths`
Expected: FAIL (depths rejected)

- [ ] **Step 3: Write minimal implementation**

Bump `ProtocolVersion` 38->39 with history comment. Replace `Dimension != Overworld` guards with `Dimension != Overworld && Dimension != Depths` on: `RequestChunkResync`, `ChunkSnapshot`, `BlockChanges`, `ForgetChunks`, `PlayerState`, `RemotePlayer*`. Keep companion/hostile/passive guards unchanged. Mirror the same allowlist in `codec_server.go validateServerWirePacket` and `validateSnapshotDimension`. Update `TestProtocolVersionPinned` expectation.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./packages/shared/network/... -race -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add packages/shared/network/protocol/packet.go packages/shared/network/protocol/message_player.go packages/shared/network/protocol/message_command.go packages/shared/network/codec/codec_server.go packages/shared/network/protocol/message_test.go
git commit -m "feat(network): allow depths dimension on player and chunk packets"
```

---

### Task 5: Authoritative warp transaction via ChatCommand

**Files:**
- Create: `packages/server/server/warp.go`
- Create: `packages/server/server/warp_test.go`
- Modify: `packages/server/sim/entity/sleep.go` (respawn fallback per dimension)
- Test: `packages/server/server/warp_test.go`

**Interfaces:**
- Consumes: `realm.State` dim搬运, per-dim spawn scan, `contract.GeneratedChunk{Dimension,...}`
- Produces: `func parseWarpCommand(text string) (core.DimensionID, bool)`; tick-atomic `warpPlayer` emitting `PlayerState{Dimension,Reset:true}` + `ForgetChunks` + `ChunkSnapshot` stream

- [ ] **Step 1: Write the failing test**

```go
func TestWarpOverworldToDepthsRoundTrip(t *testing.T) {
    host := newWarpTestHost(t, 42)
    player := host.SpawnPlayer(t, core.Overworld)
    host.SendChat(t, player, "/warp depths")
    host.Step(t)
    state := host.PlayerState(t, player)
    if state.Dimension != core.Depths || !state.Reset {
        t.Fatalf("got dim=%d reset=%v", state.Dimension, state.Reset)
    }
    host.SendChat(t, player, "/warp overworld")
    host.Step(t)
    back := host.PlayerState(t, player)
    if back.Dimension != core.Overworld {
        t.Fatalf("did not return: %d", back.Dimension)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/server/server -race -count=1 -run TestWarpOverworldToDepthsRoundTrip`
Expected: FAIL (`parseWarpCommand` undefined)

- [ ] **Step 3: Write minimal implementation**

`parseWarpCommand` accepts `/warp depths|overworld` (case-sensitive, no args). `warpPlayer` runs in-tick: validate alive + `Active` + target != current; `SavePlayer` current dim; `realm`搬运 (`oldDim`删/`newDim`插); per-dim spawn scan; emit `PlayerState{Dimension:target,Reset:true}`; old-dim companions emit `Despawn`; new-dim chunks enqueue via existing `wanted` set. Any failure emits `CommandRejected` and leaves the player unmoved. Bed respawn: `bedRespawnCandidate` requires same-dimension bed, else falls back to that dimension's spawn anchor.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./packages/server/server -race -count=1 -run TestWarp`
Expected: PASS

- [ ] **Step 5: Run sim subtree to catch phase-order drift**

Run: `go test ./packages/server/sim/... -race -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add packages/server/server/warp.go packages/server/server/warp_test.go packages/server/sim/entity/sleep.go
git commit -m "feat(server): add authoritative warp between overworld and depths"
```

---

### Task 6: Parity, restart recovery and gates

**Files:**
- Modify: `packages/server/server/transport_parity_integration_test.go` (add warp parity case)
- Modify: `packages/server/server/tcp_restart_integration_test.go` (add dual-dim reload case)

**Interfaces:**
- Consumes: Tasks 1-5 outputs
- Produces: Memory/TCP identical warp streams; restart reloads both `dimensions/{0,1}/regions/`

- [ ] **Step 1: Write the failing parity test**

```go
func TestWarpParityMemoryVsTCP(t *testing.T) {
    memoryStates := runWarpSequenceOverMemory(t, 42, []string{"/warp depths", "/warp overworld"})
    tcpStates := runWarpSequenceOverTCP(t, 42, []string{"/warp depths", "/warp overworld"})
    if len(memoryStates) != len(tcpStates) {
        t.Fatalf("memory=%d tcp=%d", len(memoryStates), len(tcpStates))
    }
    for i := range memoryStates {
        if memoryStates[i].Dimension != tcpStates[i].Dimension || memoryStates[i].Reset != tcpStates[i].Reset {
            t.Fatalf("step %d: memory=%+v tcp=%+v", i, memoryStates[i], tcpStates[i])
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/server/server -race -count=1 -run TestWarpParityMemoryVsTCP`
Expected: FAIL (`runWarpSequenceOverMemory` undefined)

- [ ] **Step 3: Implement the two sequence helpers plus restart case**

```go
func runWarpSequenceOverMemory(t *testing.T, seed int64, cmds []string) []network.PlayerState {
    t.Helper()
    host := newWarpTestHost(t, seed)
    player := host.SpawnPlayer(t, core.Overworld)
    var out []network.PlayerState
    for _, cmd := range cmds {
        host.SendChat(t, player, cmd)
        host.Step(t)
        out = append(out, host.PlayerState(t, player))
    }
    return out
}
```

`runWarpSequenceOverTCP` 同形，只是 host 改走现有 TCP harness（复用 `tcp_restart_integration_test.go` 的 dial + login helpers）。Restart case 新增 `TestDualDimensionReloadAfterRestart`：warp 到 depths 后 `SaveBatch` 脏区块，重启 host，断言 `dimensions/0/regions` 与 `dimensions/1/regions` 均可 `LoadChunk` 且 revision 单调。

- [ ] **Step 4: Run the gates**

Run: `go test ./packages/shared/worldgen -race -count=1`
Run: `go test ./packages/server/storage/... -race -count=1`
Run: `go test ./packages/shared/network/... -race -count=1`
Run: `go test ./packages/server/sim/... -race -count=1`
Run: `go test ./packages/server/server -race -count=1`
Run: `go test ./packages/audit -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add packages/server/server/transport_parity_integration_test.go packages/server/server/tcp_restart_integration_test.go
git commit -m "test(server): cover warp parity and dual-dimension reload"
```
