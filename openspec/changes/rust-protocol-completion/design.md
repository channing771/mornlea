## Context

See [proposal.md](proposal.md) for the migration reason and [the delta spec](specs/rust-runtime-foundation/spec.md) for required behavior. The current Go v45 registry has 23 client and 36 server packet keys; `testdata/runtime-migration/contracts.json` lists those 59 families plus framing, but only the two framing cases execute today. Rust has individual payload modules and a copied frame codec, while the accepted `mornlea_domain` now supplies checked commands, events, item/identity values and compact sections. The existing Rust packet DTOs contain public mutable fields, duplicate some domain tables, lack direction/state dispatch, and often encode through an infallible `Vec`/`expect` path. `ContainerClosed` exists on disk but is absent from `lib.rs`.

The Go source of wire truth is `packages/shared/network/protocol/{packet,registry,message_*,snapshot}.go` plus `packages/shared/network/codec/{codec_client,codec_server,codec_primitives,chunk_codec,frame}.go`. `packages/shared/network/login.go` defines the admission decision order. These are read-only compatibility sources in this change, not target architecture. The checked semantic Rust values are authoritative for shared meaning; a wire DTO may retain raw fields only where v45 encoding or rejection requires them.

## Goals / Non-Goals

**Goals:** give both the future Rust server and client one closed v45 packet API; retain raw negotiation information until admission; make every packet encoding fallible, bounded and atomic; prove all keys with independently executed Go/Rust evidence; and keep compact snapshot representation and protocol/domain work bounds separate.

**Non-Goals:** launch a Rust server or client, move Memory/TCP transport or session timeouts, change a packet or save version, add a new gameplay rule, rewrite Go production codecs, make Godot/Python parse packets, or claim complete F1 from protocol evidence alone.

## Decisions

### 1. One Rust protocol crate owns wire shape and dispatch

`mornlea_protocol` remains a windowless rlib with only `mornlea_domain` and locked `zstd` production dependencies. It owns raw wire DTOs, canonical primitives, frame parsing, v45 key registry, codec dispatch and conversion at the wire/domain edge. `mornlea_domain` owns shared semantic input/event values and their validation; neither domain nor a future Godot adapter receives packet IDs, byte order, compression or raw wire sentinels. F2 uses the client-facing decode and server-facing encode APIs; F3 uses the inverse APIs. Both runtimes provide transport/session state, so no Memory/TCP branch or network I/O enters this crate.

The public key is `PacketKey { direction: Direction, state: State, id: u32 }`, with closed `Direction::{ClientToServer,ServerToClient}` and `State::{Handshake,Login,Play}`. `ClientPacket` and `ServerPacket` are exhaustive enums over the exact 59 keys in [the reviewed wire matrix](wire-matrix.md). `ClientPacket::key()` and `ServerPacket::key()` return complete keys. The public operations are:

```rust
pub fn decode_client(state: State, id: u32, payload: &[u8])
    -> Result<ClientPacket, ProtocolError>;
pub fn encode_client_into(packet: &ClientPacket, dst: &mut [u8])
    -> Result<usize, ProtocolError>;

pub struct ProtocolCodec { /* one owned snapshot compression/decompression context */ }
impl ProtocolCodec {
    pub fn new() -> Result<Self, ProtocolError>;
    pub fn decode_server(&mut self, state: State, id: u32, payload: &[u8])
        -> Result<ServerPacket, ProtocolError>;
    pub fn encode_server_into(&mut self, packet: &ServerPacket, dst: &mut [u8])
        -> Result<usize, ProtocolError>;
}
```

The `ProtocolCodec` owner is one caller/session; no global mutable compressor, background thread or implicit cross-session sharing is added. Unknown keys, wrong direction/state and reserved C/Play ID 1 return `ProtocolError::UnknownPacket` before a DTO is published. Every enum variant uses a real packet module, including `ContainerClosed`. IDs are not put into the domain types. A fixed packet stays inline in its enum; a variable packet owns its bounded records. A registry table proves injective complete keys and matches independent Go discovery, rather than deriving acceptance from Rust file names.

**Rejected:** using per-struct `PACKET_ID` alone (it cannot resolve direction/state collisions); moving Go codec into Rust as a runtime oracle (it would keep Go in the final hot path); dispatching from Godot/Python (violates target ownership).

### 2. Separate raw wire values from checked semantic values

The protocol keeps the exact v45 raw representation where it is observable. In particular, a wire `ContainerRef` has `dimension: i32`, chunk coordinates `i32`, kind/slot `u8`, and generation `u32`; it does not narrow a foreign dimension to `u8`. The all-zero record is the one absent sentinel. `ContainerRef::to_domain_present(&self) -> Result<mornlea_domain::ContainerRef, ProtocolError>` rejects it and every invalid real reference; `to_domain_optional(&self) -> Result<Option<mornlea_domain::ContainerRef>, ProtocolError>` maps only the exact zero record to `None`. The stack-view validator requires the exact zero record for inventory/crafting and a checked present record for container view, then applies the view's slot range. These methods replace the current partial/quick/drop gap and the asymmetric `MoveContainerStack` constructor.

`mornlea_protocol` reuses `mornlea_domain::{PlayerId,CompanionId,ItemStack,DropId,PalettedSection}` and their checked tables. Wire-only exceptions stay explicit: raw inbound identity bytes, the absent companion identity in the two permitted chat rejection branches, and armor-owned broken pieces that are not ordinary `ItemStack`. Compact section conversion moves the existing palette and packed words without expanding cells, sorting the palette or recompressing data. The domain helper `trim_pinned_whitespace(&str) -> &str` exposes the already frozen Go whitespace set; protocol admission calls it before `DisplayName::try_from_canonical`, so there is one lexical rule and no Unicode-version-sensitive `str::trim` shortcut.

**Rejected:** copying current Go DTOs into domain (adds wire fields to shared semantics); narrowing raw dimensions at parse time (aliases invalid values into valid ones); accepting `CompanionId::NONE` as an ordinary domain ID (turns absence into identity); flattening every section to blocks (unbounded cross-layer work).

### 3. Parse inbound negotiation before policy

`ClientHello::decode_inbound` returns `InboundHello { protocol_version: u32 }` after canonical structural decoding and full consumption. `LoginStart::decode_inbound` returns `InboundLoginStart { player_id: [u8;16], display_name: String, view_distance: u8 }` after UTF-8, total 64 KiB payload and full-consumption checks; it does not apply the 128-byte canonical name limit before trim. The typed client dispatcher uses these inbound forms. Strict outbound `ClientHello`/`LoginStart` construction and encoding validate the current version and raw field bounds.

Pure `validate_hello(InboundHello) -> Result<(), HandshakeRejection>` maps any version other than 45 to `VersionMismatch { server_version: 45 }`. Pure `admit_login(InboundLoginStart) -> Result<AdmittedLogin, LoginAdmissionError>` checks UUID first; trims by the pinned helper and constructs a canonical display name second; checks view distance `2..=64` third. Invalid UUID/name maps to `InvalidIdentity`; an invalid distance maps to `ProtocolViolation`. `AdmittedLogin` owns a checked `PlayerId`, canonical `DisplayName` and the unmodified distance, with read-only accessors. Neither function creates a session, sends a reject, starts a deadline or clamps subscriptions. The later server chooses when to call them and owns connection lifecycle.

**Rejected:** treating an old version as a structural decode error (the peer loses the established mismatch response); validating raw login name length at 128 bytes before trim (rejects an input the Go admission accepts); putting timeout or session state into the protocol crate (would couple F1 to F2 transport).

### 4. Validate, size, reserve and publish in that order

`read_frame_ref(data: &[u8]) -> Result<FrameRef<'_>, ProtocolError>` returns `packet_id`, borrowed `payload` and exact `consumed` bytes after canonical u32 varint, nonzero body and 2 MiB bound checks. `write_frame_into(id, payload, dst) -> Result<usize, ProtocolError>` validates the body size before testing the destination. Existing allocating frame wrappers may call these functions for compatibility; they are not the F2/F3 hot-path API.

Every noncompressed concrete packet supplies `validate(&self)`, checked `encoded_len(&self)`, `encode_into(&self, dst)`, fallible allocating `encode(&self)` and `decode(payload)`. `encoded_len` includes a 64 KiB small-payload ceiling where applicable. `encode_into` validates the complete current value, computes exact length with checked arithmetic, checks capacity, then writes only `dst[..length]`; a short or invalid call leaves all destination bytes unchanged. Its private `SliceWriter` uses bounded slice writes and contains no semantic decisions. `ProtocolError::OutputTooSmall { needed, available }` and a checked-reservation error are added; the error class is stable but no source-language message text is promised. Existing constructor-only validation and `finish().expect` paths are removed from caller-reachable encoders. For variable batches, compare count to the packet maximum before scanning records or reserving their storage; domain's 4,096-record bound is never substituted for the wire maximum.

The compressed `ChunkSnapshot` is the sole exception to precomputed payload length: the owned `ProtocolCodec` compresses one validated logical snapshot into bounded scratch, verifies its 1 MiB compressed and 2 MiB decoded limits, then checks the destination and copies the complete envelope/payload once. Decoding checks envelope and expansion limits before decompression; one codec instance reuses its context. Cross-encoder compatibility compares canonical logical snapshot bytes and mutual decode, not compressed block identity. `zstd` scratch/compression errors never publish partial caller output.

**Rejected:** keeping the current `Vec`/`expect` encoder as the shared runtime API (mutable public fields can invalidate construction and panic or publish bad bytes); changing the Go compressed golden to match Rust's encoder (the two compatible encoders legitimately differ); claiming zero allocation for all snapshot operations (compression needs bounded scratch).

### 5. Semantic adapters are exhaustive but do not create authority metadata

`PlayIntent` is a protocol-edge enum with `Sequenced { sequence: u64, command: mornlea_domain::Command }`, `Chat(mornlea_domain::ChatIntent)` and `KeepAliveReply { token: u64 }`. `TryFrom<ClientPacket> for PlayIntent` accepts the 19 sequenced Play commands, chat and keepalive reply, and rejects Handshake/Login or any future control value. `TryFrom<ServerPacket> for mornlea_domain::Event` accepts exactly the 30 publication packets; it rejects server hello/reject, login success/reject, keepalive and disconnect. `TryFrom<mornlea_domain::Event> for ServerPacket` applies v45 wire batch limits and converts the matching 30 events without truncation. `TryFrom<PlayIntent> for ClientPacket` is the F3 output path; it consumes only an explicit `PlayIntent` and never invents `tick`, `session` or `arrival_index`; F2 creates `CommandEnvelope` after validated ingress. Rejection reason IDs use a closed match, not a numeric cast. Chat recipient routing remains in `RoutedEvent`, outside the packet itself.

**Rejected:** making `CommandEnvelope` from wire bytes in F1 (session/tick/arrival are server-owned); turning keepalive or login into domain `Event`; silently splitting a 4,096-record domain batch into multiple wire packets without a caller policy.

### 6. Evidence is per key and per real operation

The reviewed [wire matrix](wire-matrix.md) is the implementation field map; `tasks.md` is the sole status source. Test-only Go producers use actual `protocol` validation and `codec` encode/decode paths, not a copy of Rust rules. Rust corpus consumers load `testdata/runtime-migration/contracts.json` through the existing strict helper and execute actual decode/encode paths; paired package-local Go login-driver and Rust admission tests pin rejection order, while direct Rust adapter tests pin semantic conversion. Each v45 packet family gets at least one valid decode case, one valid encode case and one invalid or boundary case; every applicable count, enum, UTF-8, float, sentinel and truncation edge receives an explicit table row in its owning task packet. Frame evidence gains an encode route. Each selected case executes once, and a field/key mutation must fail comparison. `ReconcileWorking` may continue to report uncovered storage/kernel families; a protocol-only closure assertion requires zero uncovered `protocol.*` points and nonempty executed cases for all 60 protocol families. It does not claim complete F1.

Each producer writes candidate assets only under a harness-owned temporary directory or an external create-exclusive export directory. The controller reviews actual Go outcomes, case identity, source hashes, per-input and aggregate digests, then commits only reviewed assets. `source_revision` and `BaselineSourceRevision` are refreshed together once at corpus closure to an implementation baseline SHA that contains the relevant Go source and producer tests; existing case sources stay bound by their own hashes. Rust-only passing tests do not prove Go-source provenance, and Go-only case generation does not prove Rust execution. The ledger records both gates, discovered/executed counts, corpus digest, source/result SHAs and rollback evidence per node.

**Rejected:** naming a Rust consumer without an executable route; comparing only file counts or hex length; regenerating expectations from Rust output; letting producers rewrite tracked `testdata` directly; treating a protocol-only closure as the final F1 acceptance.

## Risks / Trade-offs

- **Corpus breadth can hide untested fields** → each task packet enumerates fields, a valid case, rejection/boundary cases and at least one field mutation; the final registry/semantic closure audits all 59 keys and 60 families.
- **Mutable DTOs can bypass constructor checks** → every public encoding validates the current value before capacity or writes; regression cases mutate a constructed packet.
- **Raw and semantic representations can drift** → consume checked domain types where the wire does not require raw fidelity, and pin the explicit exceptions in a conversion table.
- **Go and Rust zstd blocks differ** → compare logical bytes and mutual decoder acceptance while pinning envelope, checksum and expansion bounds separately.
- **A manifest refresh can obscure existing evidence** → review candidate diffs and source hashes, refresh one source revision at closure, rerun all existing domain consumers, and reject a changed unrelated fixture.
- **Planning baseline can drift before execution** → the first implementation node rechecks the live v45 Go registry, version matrix and corpus counts; any mismatch returns to the controller for an OpenSpec revision before a worker proceeds.

## Migration Plan

1. Freeze the actual Go registry and current Rust/domain API against the matrix, then establish a real packet corpus route and red case.
2. Land bounded framing/writer and raw-versus-domain conversion primitives; qualify pure negotiation before packet families.
3. Port and verify small coherent packet groups, with one focused test cycle and scoped commit per group. Shared exports, corpus manifest and source revision are controller-owned serial integration points.
4. Close typed registry and semantic adapters only after their producer packet groups pass; run the protocol-only zero-gap evidence gate and full stage gates.
5. Keep the existing Go runtime/default unchanged. If the protocol change fails, revert the Rust protocol/corpus group; no save migration or live authority rollback is needed. F2 remains blocked until storage, numerical API, pathfinding and final F1 acceptance separately pass.

## Validation

Use the exact focused commands in each task packet, inspect test discovery with `-- --list`, and reject zero executed cases. Final gates include Rust format and `make rust-check`, the full Go runtime-oracle race suite, `go test ./packages/shared/network/... -race -count=1`, `go test ./packages/audit -count=1`, `make dev-check`, `make test-race`, and `openspec validate --all --strict --no-interactive`. `make rust` precedes focused Go commands on a clean checkout when Rust artifacts are involved. No automated test may focus a game window or write a live save.

## Final whole-branch review remediation

The final independent review found four gaps in the completed branch: frozen Rust cases discard their packet key; no Go decoder consumes a Rust-produced zstd frame; public snapshot helpers can be called outside the envelope ceilings; and two strict outbound negotiation encoders still trust construction-time validation. These are corrections to the existing protocol contract, not new packet families or version changes. Nodes 5.1–5.5 in the closure packet own the repair and final integrated acceptance.

The test-only corpus loader retains an optional `FrozenPacketKey { direction, state, id }` from the manifest. All 433 packet cases require it; the three frame cases do not. The Rust corpus test resolves each family's key from its successful frozen decode case, runs every successful decode payload through `decode_client` or `ProtocolCodec::decode_server`, asserts the returned typed packet's key, and rejects a mismatched key on every case (including invalid and encode cases). A mutation of direction, state or ID on a loaded case must now fail a Rust assertion while its payload and expected fields remain unchanged. The existing concrete codec comparison still checks each field and error category, and the Go manifest/registry reconciliation remains the independent key oracle. This avoids a second 59-entry family map in the test.

`SnapshotEnvelope` will keep its two fields private and expose read-only accessors. The decompression helper rechecks the 2 MiB decoded and 1 MiB compressed ceilings before calling zstd, even if an internal caller constructs an envelope. The public one-shot `compress_logical` and the shared compression helper refuse a logical slice over 2 MiB and a compression-bound scratch request over 1 MiB before reserving. The existing expansion-bomb tests may use test-only zstd directly to construct an invalid frame; production and public protocol helpers remain bounded. A committed Rust-produced fixture of the canonical overworld snapshot is pinned against the Rust encoder and fed to the real Go `DecodeServer`; Go compares it to its independently encoded fixture's semantic record and rejects a corrupted Rust frame. The two encoders' compressed blocks remain free to differ.

Strict outbound `ClientHello` and `LoginStart` will use the same fallible `validate` → `encoded_len` → `encode_into` → `encode` chain as other concrete packets. `ClientHello` rechecks version 45. `LoginStart` rechecks the canonical display name and distance in constructor order, computes length with checked arithmetic, and leaves a short or invalid destination unchanged. The raw inbound negotiation types stay structurally permissive. `BlockChanges` will validate all field rules first and then check adjacent block indices in a second allocation-free pass, preserving its existing error precedence.

The integration branch merges the current `main`, preserves main's updated `mornlea_engine/src/step.rs` provenance hash in the shared manifest while retaining the protocol Go-source revision `b6043f00`, and removes the active `rust-region-format-completion` planning copy already archived on main. The protocol delta is synchronized into the main `rust-runtime-foundation` spec and archived only after the merged result passes all stage gates. No live authority, default runtime, save, protocol version, ABI or Cargo dependency changes are authorized. The original user-modified checkout is left intact.
