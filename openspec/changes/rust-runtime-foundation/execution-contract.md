# Foundation worker execution contract

This document and the linked subsystem briefs are the controller's Superpowers implementation plan for the unstarted work. Baseline is `60c476645ee6dae1f6392336a7f3c593d2163ae3`. Existing runtime implementations remain in use. Only `tasks.md` contains task-status checkboxes. A task's dependency wait is not an unresolved design decision.

## Goal and architecture

Build one validated domain model, bounded fallible wire/save adapters, native safe kernel APIs and executable offline Go/Rust evidence. Preserve protocol 45, player/chunk schema 9, world metadata 6, companions 5, hostile 2, passive 1, engine ABI 11, client ABI 19 and existing historical schema support. The old Go runtime remains the authority until later changes.

Use Rust 1.97.1 / edition 2024, std, and the already approved zstd dependencies. Foundation production dependency sets remain unchanged. The corpus support node may add **dev-dependencies only**: the already locked `serde_json` version and `sha2` 0.10 with a narrowly reviewed lock addition; no production serialization derives, no runtime dependency cycle and no upgrades to existing locked packages. The controller owns manifest/lock integration. These are planned test dependencies, not installed by this planning round.

## Shared protocol API

Add `Direction { ClientToServer, ServerToClient }`, `State { Handshake, Login, Play }` and `PacketKey { direction: Direction, state: State, id: u32 }` in protocol `registry.rs`. Derive Copy/Eq/Ord. Packet IDs and their accepted keys come from `packet-contracts.md`. The registry dispatches existing concrete packet structs; domain values do not carry packet IDs.

```rust
pub struct FrameRef<'a> {
    pub packet_id: u32,
    pub payload: &'a [u8],
    pub consumed: usize,
}
pub fn read_frame_ref(data: &[u8]) -> Result<FrameRef<'_>, ProtocolError>;
pub fn write_frame_into(id: u32, payload: &[u8], dst: &mut [u8])
    -> Result<usize, ProtocolError>;
```

Each noncompressed concrete packet `P` gets these inherent methods; keep its existing `decode` result type except the two inbound admission exceptions:

```rust
impl P {
    pub fn validate(&self) -> Result<(), ProtocolError>;
    pub fn encoded_len(&self) -> Result<usize, ProtocolError>;
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError>;
    pub fn encode(&self) -> Result<Vec<u8>, ProtocolError>;
    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError>;
}
```

`P` is the concrete packet name in each task, not an undefined generic base class. The common implementation helper is private:

```rust
pub(crate) fn publish_packet(
    length: usize,
    dst: &mut [u8],
    write: impl FnOnce(&mut SliceWriter<'_>),
) -> Result<usize, ProtocolError>;
```

`SliceWriter` in `bytes.rs` owns a mutable slice and cursor. After preflight it writes fixed-width primitive bytes with checked slices; its primitive methods are private/internal and never perform semantic validation or allocate. `publish_packet` checks `dst.len() >= length` before calling the closure, restricts the writer to `dst[..length]`, and verifies the final cursor in a debug assertion. All content validation and checked length computation precede it. An implementation mismatch in a debug assertion is a test failure, not an input error policy. Eliminate caller-reachable encoder `expect` and unchecked public writers.

Add `ProtocolError::OutputTooSmall { needed: usize, available: usize }`; retain existing structural/semantic variants. Validation precedence: complete value validation, checked format length/max limit, destination capacity, write. An invalid value beats a short-output error. `encode` calls `encoded_len`, reserves exactly that size, then `encode_into`; it never publishes a partially encoded vector. Wire layout remains little-endian and canonical u32 varint, maximum five bytes. Framing combined ID+payload limit remains `2 << 20`; the prefix is not included in that limit.

For a fixed packet with known size, the central test is:

```rust
let packet = valid_packet(); // The node gives its exact constructor or corpus case.
let n = packet.encoded_len().unwrap();
let mut dst = vec![0xa5; n + 3];
assert_eq!(packet.encode_into(&mut dst).unwrap(), n);
assert_eq!(&dst[n..], &[0xa5; 3]);
let mut short = vec![0xa5; n.saturating_sub(1)];
let before = short.clone();
assert!(packet.encode_into(&mut short).is_err());
assert_eq!(short, before);
```

`valid_packet` in this example is expanded to a named frozen case by every packet node, through the corpus helper defined below. The test is not usable as an unexplained placeholder in a dispatched brief.

### Admission signatures and exact order

`InboundHello { protocol_version: u32 }` and `InboundLoginStart { player_id: [u8;16], display_name: String, view_distance: u8 }` live in protocol and remain structural values. `ClientHello::decode_inbound` and `LoginStart::decode_inbound` return these types. The typed client dispatcher uses them. Existing `decode` may remain a strict outbound-record convenience, explicitly named/documented as such, but cannot serve inbound login dispatch.

`validate_hello(InboundHello) -> Result<(), HandshakeRejection>` uses the current version constant and returns `VersionMismatch { server_version: 45 }`. `admit_login(InboundLoginStart) -> Result<AdmittedLogin, LoginAdmissionError>` checks UUID then canonical name, then distance; errors are `InvalidIdentity` and `ProtocolViolation`. `AdmittedLogin` has private `player_id: domain::PlayerId`, `display_name: String`, `view_distance: u8`, with getters. The canonical name is trim-space output using the frozen Go whitespace/control predicate in plan02, 1..=32 Unicode scalars, <=128 UTF-8 bytes, no Unicode control code points. The raw wire field is bounded by total small payload <=65536 bytes; names longer than 128 before trimming may still admit if the trimmed name meets the rule. Do not put the admitted-name bound into structural parsing. Outbound LoginStart requires raw name length<=128 bytes and a valid normalized name, preserves original edge spaces in emitted bytes, and validates distance2..64; structural inbound parsing does not apply that raw-name bound.

Identity invalidity wins over distance invalidity. No session, timeout, network send or clamping belongs here. `ContainerRef::NONE` is raw wire data; a semantic optional container uses `Option<domain::ContainerRef>`. Raw nonzero container views must validate overworld dimension 0, kind furnace/chest, slot <32/<16 and generation >0, including split/quick-move/drop commands.

## Domain type and representation contracts

All common valid values live in domain with private fields and getters. Constructors use `try_new` or retain an already defined equivalent name. `PlayerId::try_from_bytes([u8;16])` and `CompanionId::try_from_bytes([u8;16])` require nonzero UUIDv4/variant; these are distinct newtypes over the same checked bits. No unchecked public constructor or NONE valid-ID value. Historical storage raw IDs use a format DTO and validate at conversion. Player-save armor is a current format-level pure-fidelity exception too: preserve its four raw `(item,count,durability)` triples, including zero durability or other data the Go codec passes through; do not run ordinary ItemStack admission on that field. Gameplay equipment validation belongs to its consuming authority. `ItemStack::try_new(u16,u8,u16)` / `EMPTY` and getters `item()`, `count()`, `durability()` are the single current stack rule. Item IDs 0..65 are defined; 66 is unregistered. Max ordinary stack 64; per-item durability/smelting tables are copied once from verified Go core and exhaustively compared by item ID.

`LookAngles::try_new(yaw: f32,pitch: f32)` requires finite floats and does not clamp/reduce angles. `BlockPos { x:i32,y:i32,z:i32 }` and `ChunkPos { x:i32,z:i32 }` are coordinates, not world access. `Dimension` accepts the current playable values 0/1. Raw region dimensions stay i32; DropId's raw dimension validity stays as in Go. No position normalization or authority settlement occurs in these values.

`CommandEnvelope { tick:u64, session:u64, sequence:u64, arrival_index:u64, command:Command }` owns intake metadata for the 19 sequenced play intents. `ChatIntent { text: CommandText }` is the twentieth intent and stays outside this envelope: chat has no wire sequence and is consumed through its own FIFO. Do not fabricate a sequence or globally merge chat with sequenced commands. Each `Command` variant's payload and exact source map are fixed in the domain plan. Sequence is not duplicated inside the payload. `order_commands(&mut [CommandEnvelope], &mut CommandOrderScratch) -> Result<(), DomainError>` uses reusable key scratch specified in plan02, first rejects duplicate `(tick,session,arrival_index)` keys, then orders `(tick,session,sequence,arrival_index)` without kind comparisons. The caller supplies an already bounded intake batch; F1 does not invent a global client-count limit. Duplicate sequence filtering is runtime policy, not this function.

Events own immutable semantic values, with no packet ID, direction, wire length, endian methods or digest string. Their field map follows `packet-contracts.md`; wire-only layout/sentinel fields are converted at the protocol adapter. Domain `Event` excludes hello/login/reject/keepalive/disconnect transport control. Domain `Observation` and replay identity JSON move into test support. Player/entity arrays preserve source order; identifiers and enum values are validated on construction. Batch maxima are protocol-specific and stay in protocol, while domain single-record invariants are shared.

Paletted sections remain compact; do not expand every snapshot into 98304 block IDs at the contract boundary. Domain `PalettedSection` is a private validated enum-backed value: Single(block), Indexed4/Indexed8 { palette: Box<[u16]>, words: Box<[u64]> }, Direct15 { words: Box<[u64]> }. It provides `block_at(index: usize) -> Option<u16>` and read-only variant accessors. Validate 4096 cells, indexed palette uniqueness/registered IDs/count <=2^bits, exact words `ceil(4096/floor(64/bits))`, all slots in palette, and unused high bits/unused final slots using existing Go/Rust section rules. Preserve the existing representation; do not recompress/reorder a palette during codec conversion. Both save and protocol adapters use this one current section value while historical schema decoding remains storage-local.

## Corpus contract and helper ownership

Go test adapters emit cases using the actual supported APIs. Inventory schema 2 separates `sources` from `cases`. Case IDs are `<family>/<version>/<label>` and all paths are repository-relative slash paths with no `..`, absolute prefix or symlinks. The frozen manifest lists source Git SHA, identities, case operation, packet key if applicable, input/output paths and sha256 digests, expected category and normalized fields. Maximum manifest 4 MiB, case JSON 256 KiB, binary case input/output each 4 MiB, cases 8192, observations 32768. These are offline harness budgets; game-family bounds remain tighter and mandatory. Reject file metadata exceeding these budgets before `ReadFile`/allocation.

Normalized JSON uses sorted object keys, ordered arrays, decimal strings for i64/u64 and hexadecimal 8-digit IEEE-754 f32 bit strings. Integer fields <=32 bits are JSON integers. Outcomes have `kind: "ok"|"error"`, `category`, `fields`, and optional encoded/logical payload digests. Structural errors distinguish truncated, trailing, invalid-varint, capacity, unsupported-version, invalid-enum, invalid-identity, invalid-value and integrity. Login admission categories are handshake-version-mismatch, login-invalid-identity and login-protocol-violation. Storage normalizes Go sentinel identity to corrupt/future-version, with field-scoped detail retained diagnostically; do not compare localized error prose.

`packages/engine/tests/runtime_corpus.rs` is test-only shared support, included by the three foundation integration targets. It defines:

```rust
pub struct FrozenCase {
    pub id: String,
    pub family: String,
    pub input: Vec<u8>,
    pub normalized: serde_json::Value,
    pub encoded: Option<Vec<u8>>,
    pub category: String,
}
pub fn load_case(id: &str) -> FrozenCase;
pub fn assert_normalized(case: &FrozenCase, actual: serde_json::Value);
pub fn assert_rejected_unchanged<T: PartialEq + std::fmt::Debug>(
    before: &T, after: &T, error: bool,
);
```

`load_case` reads the frozen index via paths rooted from `CARGO_MANIFEST_DIR`, enforces the above byte limits, hashes actual bytes with sha2, checks claimed identity and fails on a missing/duplicate ID. No Rust test launches Go. Go generation writes only a harness-created temporary tree; the controller reviews generated corpus differences before committing. Shared helper changes have exclusive ownership in node 1.1.

Each packet-family task defines its `valid` and negative cases concretely from the packet table. The Go adapter generates the valid value using the exact fields shown there; it never takes expected output from Rust. Reusable test procedures are instantiated with a concrete packet type and named case, not a filename count. Storage legacy fixtures remain untouched. The full Rust replay integration target is the only place with test dependencies on all three foundation crates plus engine; no production cycle is introduced.

## Worker steps and closure

Every node's brief spells exact files, predecessors, contract, regression body/data and algorithm. Execute these steps in order: (1) add the specified regression to the named topic test file; (2) run its exact filter and confirm the intended assertion fails; (3) implement only the frozen transformation; (4) run the focused suite plus listed dependent integration checks; (5) submit changed files, before/after outputs and risks for controller review, then make the scoped commit. Test helper setup belongs with its first real behavioral case. Use English comments at new ownership/lifecycle/error boundaries.

Controller review checks exact source/result SHA, actual test discovery and execution, oracle identity, invalid constructed values, no partial publication and the declared allocation bounds. Workers never regenerate expected values to match their implementation or check off their own task. No stash/reset/cleanup, broad staging, foreground game, live saves or default runtime change. If current file contents diverge from the brief, return the discrepancy to the main Agent before the dependent edit.

Rollback for a node is a scoped reversal of its implementation plus its new test/helper contribution, retaining fixtures and already accepted prerequisites. A coordinated type migration rolls back its producer and consumers together; do not leave old/new registries both authoritative. No save downgrade is needed because none of these nodes switches production ownership.
