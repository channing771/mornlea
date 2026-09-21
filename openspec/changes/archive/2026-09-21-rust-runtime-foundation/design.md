## Context

F1 introduces contracts used by both future Rust runtime owners. The current production runtime is still Go plus the existing Rust numerical engine and renderer. See `proposal.md` and `docs/architecture-target.md`. Review baseline: `60c476645ee6dae1f6392336a7f3c593d2163ae3`; defects and reproduced failures are in [review.md](review.md).

The three foundation crates exist, but their acceptance is reopened. A passing suite of self-authored round-trips did not detect missing exported packets, migration identity loss or invalid encoder output. The current oracle hashes copied files instead of executing behavior. This design supersedes historical ledger claims of section-2 acceptance; it does not erase their implementation history.

## Goals / Non-Goals

Give future server and client owners one domain model, bounded codec interfaces, safe numerical entry points, and independent executable compatibility evidence. Preserve externally observable Go behavior while choosing Rust-native ownership and data structures.

F1 does not implement online login/session scheduling, authoritative gameplay, save workers, socket I/O or Godot features. It defines the contracts those owners consume. Full simulation replay is a later runtime prerequisite; F1 cannot label fixture-copy output as simulation replay. No default startup, live-save, C ABI symbol/layout/version, toolchain-pin or game-format-version changes are authorized here. The malformed-call publication corrections are explicitly scoped in the numerical plan.

## Decisions

### 1. Controller and worker responsibilities

The controller owns shared type placement, public API signatures, admission policy, state transitions, numeric/order rules, error precedence, bounds, corpus design and acceptance. A worker owns only the files and transformation named in its task. It must return a discrepancy to the controller instead of inventing a compatibility rule, extending a dependency whitelist, regenerating expected results from Rust, suppressing a lint, or creating a second shared registry.

Dispatch one verified node at a time along its dependencies. An isolated worker receives: baseline SHA; exact read sources and editable files; relevant sections below; required before/after examples; public signature and error categories; allocation/count bounds; failing scenarios; exact commands; forbidden scope; integration owner. The controller reserves `tasks.md`, `ledger.md`, shared public exports and architecture decisions. Workers do not edit completion status. Do not give different workers overlapping crate roots, integration test files or exports in one worktree. An isolated review may report evidence; the controller independently rules on compatibility and accepts the result.

The Superpowers revision replaces the old milestone assignments with the concrete nodes indexed in `tasks.md`. Read [execution-contract.md](execution-contract.md) and the node's linked subsystem brief. Interfaces, functional field maps, algorithms, negative cases and task dependencies are now part of this plan; dependency-blocked nodes are fully designed. A newly discovered family or contradiction returns to the main Agent for an explicit plan revision before dependent implementation.

### 2. Ownership and dependencies

| Owner | Owns | Must not own |
| --- | --- | --- |
| `mornlea_domain` | Validated identities, item/block numbering and shared value rules, semantic input/event values, session-tagged input ordering | Wire/save parsing, schema migrations, compression, I/O, trace JSON/digests, runtime state or authority |
| `mornlea_protocol` | Direction/state/ID dispatch, versioned wire representation, structural decoding, outbound validation, conversion to/from domain values | A second item registry, save migration policy, session timers or admission side effects |
| `mornlea_storage` | Historical schema DTOs, save validation/migration, checksums, format-local region/container representation | A second current item registry, file scheduling/atomic rename, client-side saves or runtime mutation |
| `mornlea_engine` | Pure bounded numerical operations and reusable caller-owned scratch; existing C ABI adapters | Go callbacks, live world access, session/replan policy or calls back through its own ABI |
| Offline evidence | Manifest, input bytes/values, normalized outcomes, provenance, differential runners | Production fallback or a second online authority |

Production dependencies stay: domain has none; protocol/storage each depend exactly on domain and the approved `zstd`; no foundation crate depends on engine/client/Godot/runtime. This round does not add dependencies. Kernel-local math views may be distinct from domain gameplay values; callers adapt them without serializing through C. A new engine-to-domain dependency is not needed for the listed numerical APIs.

Shared current values move to domain once, with private fields and checked construction: `PlayerId`, `CompanionId`, `ItemId`, `ItemStack`, `HotbarSlot`, finite `LookAngles`, block/chunk positions and validated playable dimension. `ItemStack` exposes `EMPTY`, getters and `try_new(item, count, durability)`; empty means all zero, nonempty means registered item, allowed count and durability. One domain table provides stack limits, durability maxima and smelting products. Domain tests cover every registered item and the unregistered boundary. Protocol/storage can temporarily re-export domain types during this change, but must remove their duplicate rule tables before acceptance.

Do not force every raw persisted or wire number into a stricter domain type. Historical format dimensions and sentinel identities remain format-local raw fields until their documented conversion boundary. `DropId`'s dimension validation must retain the existing Go contract; the current Go validity predicate checks slot/generation, not that dimension. An optional companion identity is `Option<CompanionId>` in semantic values; only the wire adapter maps its allowed all-zero sentinel. A validated ID must not acquire an unchecked public constructor to satisfy one packet.

Use static protocol/version constants in hot paths; do not allocate the two Agent version strings merely to check the protocol number. Replay source revision/digest and arbitrary observation labels belong to the offline harness, not the runtime domain model. Moving them preserves offline evidence but removes the misleading claim that a digest string implements semantic events.

### 3. Semantic commands and event boundary

For the 19 sequenced intents, `CommandEnvelope<T>` carries `tick: u64`, `session: SessionId`, `sequence: u64`, `arrival_index: u64` and a typed command. Input fields are values, not authoritative results. `PlayerControl` groups move axes, finite `LookAngles` and `HeldActions`; the envelope owns sequence. This replaces ten-argument constructors without introducing wire-order coupling or lint waivers.

Within each tick, order by `(session, sequence, arrival_index)`. The arrival index is assigned by the producer at intake and must be unique within that trace's tick/session. It records the original order, not a kind-based tie breaker. The runtime session owner later applies `sequence > last_sequence`; F1 does not maintain that state. Cross-session sequence numbers are not globally comparable. The compatibility source is `packages/server/sim/runtime/engine_step.go`'s stable `(Session, Sequence)` sort and first-admitted duplicate behavior.

The domain input coverage set is the current 20 play intents other than `KeepAliveReply`; `ChatCommand` has no sequence and remains a separate FIFO `ChatIntent`, outside command sequence sorting: `PlayerInput`, `PlaceBlock`, `SelectHotbar`, `RequestChunkResync`, `DropSelectedItem`, `TillSoil`, `BoneMeal`, `CollectWater`, `PlaceWater`, `EquipArmor`, `OpenContainer`, `CloseContainer`, `MoveInventoryStack`, `MoveContainerStack`, `MoveCraftingStack`, `TakeCraftingOutput`, `MoveStackPartial`, `QuickMoveStack`, `DropStack`, `ChatCommand`. Keep handshake, login and keepalive as protocol control messages. Preserve each command's existing field domains and actor/target authority; do not add client-provided target positions to ray-based actions.

Semantic events must contain values consumable independently of bytes. Start with command outcomes (`CommandRejected`, `PlaceBlockSucceeded`, `CombatHit`, `ContainerClosed`) and the existing typed world/player/inventory/entity/chat publications mapped from the server packet registry. The packet registry is a coverage source, not permission to recreate a server authority in domain. The complete controller-selected field maps and immutable ownership are frozen in [the domain plan](plans/02-domain.md) and [the packet contract table](packet-contracts.md). The existing `domain.event` row pointing only to a transcript test source is not evidence for any such event. Do not check the domain milestone complete with only the first group.

### 4. Protocol admission is asymmetric

Represent structural requests separately from admitted values:

- `InboundHello { protocol_version: u32 }` decodes any structurally valid canonical uvarint. A pure compatibility check reports the existing handshake mismatch category; F2 owns sending the rejection and closing/timing the session.
- `InboundLoginStart { player_id: [u8; 16], display_name: String, view_distance: u8 }` decodes the existing bounded wire string and all u8 distances. Apply the Go codec's 64 KiB small-payload bound before allocation, not the 128-byte admitted-name bound during decoding.
- `admit_login(&InboundLoginStart)` performs UUIDv4/name validation and canonical trimming first, then the `2..=64` distance check, returning `AdmittedLogin` or the existing invalid-identity/protocol-violation category. An invalid identity takes precedence when both identity and distance are invalid. No network operations or session creation occur here.
- Outbound `ClientHello` and `LoginStart` remain strict: current version, valid identity/name/distance. No public infallible outbound path may bypass these checks. Canonical display-name behavior follows `core.NormalizeDisplayName`, including Unicode/control-character and byte/rune boundaries; generic Rust `trim` equivalence must be tested on the Go corpus rather than assumed.

Malformed lengths, invalid UTF-8 where the Go codec rejects it, truncation and trailing bytes remain structural errors. Unknown save versions still fail in storage. An inbound login exception must not leak into play packets, save decoding or accepted domain values. Both future Memory and TCP paths use the same structural/admission split.

Freeze a compiled registry keyed by `(Direction, State, packet_id)`, never ID alone: ID 0 is reused across states/directions. Use typed packet enums and match-based encode/decode dispatch. A test registry row contains the callable constructor/decode/encode adapter, bounds and family identity; it must not be a list of source filenames. `ContainerClosed` must be reachable as server/play/14. Tests reject wrong state/direction, duplicate keys, unknown IDs and an inventory row without an executable case.

### 5. Fallible encoding, buffers and publication

Public packet DTOs may remain editable where useful, but every public outbound `encode`/`encode_into` is fallible and revalidates the whole value. Private validated domain fields remove many invalid states; they do not remove aggregate validation. Eliminate caller-reachable `.expect("validated ...")`. Do not use `catch_unwind` as validation. A codec must not silently emit data its decoder rejects.

The common interfaces are `encoded_len(&self) -> Result<usize, ProtocolError>` and `encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError>`, plus a fallible owned convenience wrapper. Validate all fields and checked size arithmetic before writing the destination. A short buffer leaves its bytes unchanged. For variable compression whose final size is not known until execution, use caller-owned scratch and publish to the destination only on success. Storage exposes the analogous fallible operation with `StorageError` and the same all-or-nothing result contract.

A borrowed frame view returns packet ID, payload slice and bytes consumed. It borrows only while its input is held; an asynchronous runtime owner must retain an immutable owned buffer or copy before sending across a task/channel boundary. The owned convenience reader may copy explicitly. Varints write to a five-byte stack array or the caller's destination; fixed array decoding uses stack/fixed storage, not an intermediate `Vec`. Prefix/ID/payload do not each get their own allocation.

Hard allocation/work contracts:

| Operation | Bound and verification |
| --- | --- |
| Borrowed framing; fixed packet `PlayerInput`, `InventoryState` encode/decode | Zero heap allocations after caller setup; fixed arrays remain fixed; reject before writes on invalid value/short destination |
| Variable record/string decoding | Check family count, byte limit, minimum bytes per record and checked multiplication before `reserve`/copy; one owned allocation per necessary variable field or collection, no per-primitive allocation |
| Compressed chunk/snapshot | Check declared compressed and decoded maxima first; decompression stops at the decoded limit even if the frame lies; reuse an exclusively owned codec context and scratch on a background worker; no global mutex or per-call context construction on the intended hot API |
| Save aggregate validation | Bound collection sizes before cloning/sorting/compressing; every accepted value re-decodes and preserves associations |
| Numerical operation | Explicit input/output capacities, deterministic work budget and reusable scratch; no I/O or unbounded callback |

The existing owned convenience methods can allocate and must be described as such. Timing/throughput results are informational; allocation-bound, panic, overflow, data-loss and I/O failures are acceptance failures. Record cold and warm allocation/copy counts separately. Do not claim that Rust syntax alone improves performance or invent a hardware-specific latency threshold without measurement.

### 6. Storage repair contracts

Keep schema-specific parsing separate from current-value validation and migration. Decode into private temporary state, validate the entire value/relationships, then return it. Encode uses the same aggregate invariants before producing a logical payload or compression output. Historical migration may intentionally synthesize documented defaults; it may not silently pad malformed current inputs or discard associations.

| Node | Required implementation | Required counterexamples |
| --- | --- | --- |
| Legacy companion queues | In v2/v3/v4 decoding, bind each parsed queue to its containing body's ID before deciding whether to retain it; validate membership after migration | Every queue in each committed v2/v3/v4 fixture maps to exactly one body and preserves task/FIFO/summary; compare exact IDs and contents with Go, not merely record counts |
| Companion cardinality | Reject more than 64 bodies, lifecycle-count mismatch and impossible queue count before any clone/sort; preserve existing 4-active and queue-membership rules | 64 inactive valid records succeed; 65 fail; 4/5 active boundary; duplicate/orphan/inactive queues; rejection does not clone the oversized collection |
| Chunk containers | Factor a shared aggregate validator used by encode and decode; validate shape, section values, drop slots, active furnace/chest index bounds, per-kind uniqueness and actual block match | Active furnace on air, chest on air, duplicate block indices, maximum valid/first invalid index, inactive canonical slots, and valid mixed containers; rejection happens before compression |
| Region bank | Store entries as `Box<[Entry; REGION_SLOTS]>` with `REGION_SLOTS = 1024`; a checked `try_from_entries(Vec<Entry>)` reports wrong cardinality; retain all extent/revision/checksum validation | Lengths 0, 1023, 1024, 1025 and 1200; the invalid lengths return errors without panic/padding; last slot and fixed bank-padding bytes remain exact |

A fixed-size domain collection must not become a freely sized `Vec` merely because Go arrays were inconvenient to translate. Conversely do not introduce a huge stack frame for the region table; construct the boxed fixed-size storage safely from a bounded vector. No unchecked indexing or blanket panic catch is a substitute for the cardinality invariant.

Compression ruling remains unchanged: `save.chunk` and `protocol.server.ChunkSnapshot` compare exact logical payloads and normalized values, verify checksums/length limits, and cross-decode Go and Rust outputs. Compressed block bytes need not match. Noncompressed current wire/save records retain their specified byte parity. Older schema decode followed by current encode is compared to the Go migration result, not automatically to the old input bytes.

### 7. Executable corpus and trace identity

Separate inventory provenance from behavioral inputs. A source/header/test filename is provenance; a binary fixture or explicit value/error case is executable evidence. Give each case a stable `case_id`, family, operation, direction/state/ID when applicable, supported source version, input content digest, expected Go result/error category, and Rust consumer. Every supported family/version needs positive and applicable negative cases; not every packet shares the same two test source files as evidence.

Keep the inventory scanner stdlib-only. Add Go test-only adapters under `packages/tools/cmd/runtime-oracle/` that call the established codec/storage/domain/pathfind APIs. Existing numerical kernels are compared through the established `nativeabi` bridge and Rust safe API; do not add direct C linking to oracle tooling. Adjust `packages/audit` narrowly for these test imports and read-only header provenance, with negative tests that continue to forbid production authority imports and ABI linking outside `nativeabi`. Update the oracle's scoped guide in that same node. The read-only CLI may load/validate evidence; actual Go fixture generation is an explicit test operation writing only to an isolated temporary directory. Committing generated corpus data is a separate reviewed controller action.

For each case, Go executes decode/encode/migration/admission or a numerical function and emits both normalized outcome and encoded bytes where applicable. Rust independently executes the corresponding consumer. No expected values are copied from Rust results. Float observations use IEEE-754 bit patterns unless that family already specifies a named tolerance; reject non-finite values where required. Large values can be summarized by a digest of a canonical typed representation, with the full payload retained for failure diagnosis. Use explicit sorted lists for maps/sets, preserve meaningful order, and represent 64-bit integers losslessly.

The corrected trace uses tooling schema 2; schema 1 fixture-copy traces are not accepted as behavioral evidence. A run binds source Git SHA, contract identities, manifest schema, seed, case list, operation inputs and checkpoint schedule. The corpus digest includes canonical manifest bytes plus the sorted `(case_id, input digest, expected-output digest)` set; changing input/output content changes identity. A fake nonempty digest or source string is insufficient. Compare claimed identities to the frozen manifest and actual input content. Provenance hashes of sources are recorded separately and are not observations.

A schedule is strictly increasing and duplicate-free. Every expected `(tick, case_id)` has exactly one observation; no extra family, unknown tick, missing case or duplicate input index is tolerated. Stateless codec cases explicitly declare a single checkpoint; stateful numerical sequences carry actual inputs and resulting values. A seed need not alter seed-independent codecs, but seed-dependent worldgen cases must exercise it. Inputs record the session/sequence/arrival semantics from section 3. Validate before copying files or creating output directories.

Use a fresh harness-created temporary workspace. Caller-selected export paths must resolve the deepest existing ancestor, append all missing components, and reject repository/live-save destinations or symlink traversal before creation. The harness has exclusive ownership of its work directory; it must not follow copied fixture symlinks or mutate source fixtures. Use component-aware containment checks and refuse ambiguous path-resolution errors. Tests cover symlink ancestors with multiple nonexistent descendants, preexisting output files and I/O failure. A successful run atomically publishes its report after validation, never a partial success report.

The Rust `differential_replay` integration target uses test-only protocol/storage/kernel dependencies; production domain remains dependency-free. Its coverage must execute, not count names. Mutation checks deliberately remove a compiled packet adapter, change one decoded scalar/queue ID, omit a checkpoint, or corrupt a content digest; each must fail acceptance. Agent HTTP/MCP fixtures stay language-neutral contract validation only, with no service startup or expansion of Python authority.

### 8. Safe Rust numerical boundary

Do not expose private byte parsers unchecked or make Rust callers construct ABI byte blobs. The structure is `validated typed request -> existing numerical core -> typed result`; C entry points perform pointer/layout conversion and then call the same validated core. Pointer/alignment/alias and panic-containment duties stay in `ffi.rs`; value/range validation must also be reachable by native Rust callers. ABI v11 symbols, lengths, successful values and status meanings stay unchanged. Plan05 explicitly repairs metadata-alias publication and mesh partial payload writes; former invariant panics map to the existing failure status without escaping the safe API. Native fluid-eval batching is capped at4096 while the old ABI adapter retains its validated larger-count behavior.

| API family | Native representation and boundary | Reuse/evidence |
| --- | --- | --- |
| Collision / physics | Finite vector/AABB/control values plus bounded borrowed voxel view; typed position/velocity/contact result; validate cell count and all used scalars before the core | `collision.rs`, `step.rs`; physics cell cap 4096; compare safe call and existing ABI including step-height and malformed input |
| Raycast | Finite ray request, opaque validated continuation cursor, fixed maximum 64 typed records and completion/continuation result | `raycast.rs`; preserve batch continuation and boundary/tie behavior, not just first batch |
| Worldgen chunk/probe/tree | Validated `WorldgenParams`, integer request positions, caller output buffers; probe at most 64 records, tree at most 128 blocks, chunk exactly 24 × 4096 block IDs | `worldgen.rs`; preserve integer overflow/rounding policy and ordering; ABI's 566-byte params header stays only at the adapter |
| Fluid evaluation/rescan | Seven-cell evaluation items and typed changes; validated immutable rescan view, caller-owned result/scratch and explicit output limit | `fluid_eval.rs`, `fluid_rescan.rs`; preserve deterministic record order and overflow result, never silently truncate |
| Mesh / LOD | Validated neighbor/light/palette view and reusable scratch; typed mesh/quad output with checked capacity | `greedy/`, `lod.rs`; retain format packing only at consumer/ABI boundary and preserve zero/short-capacity semantics |

The exact native signatures, value validators, output bounds, ABI adaptation and tests are frozen in [the kernel plan](plans/05-kernel.md). Its nodes wait only for their stated implementation prerequisites; they do not assign interface design to workers. Existing Rust algorithms are reused, not duplicated as a new foundation kernel.

### 9. Pathfinding design

Use `PathGrid` with immutable owned block snapshot, checked origin/extents, injected passability table and normalized revision list; `PathScratch` belongs exclusively to a caller/worker and can be reused; `find_path(&PathGrid, start, goal, &mut PathScratch)` returns an independently owned path/revision result or a typed invalid-input/unreachable/budget failure. Runtime cooldown, stale-result rejection, task retry policy and cancellation of queued work stay outside the kernel.

Freeze Go semantics: positive dimensions, at most `1 << 17` cells, Y-fast index `((x * size_z) + z) * size_y + y`, at most nine revision entries before deduplication, revisions sorted by `(X,Z)`, equal duplicates collapsed and conflicts rejected. Unknown blocks are blocked. Feet/head must be passable and supporting cell in-bounds/nonpassable. Validate dimensions and revisions before reading/copying grid content. Production 33 × 9 × 33 windows have 9801 cells; maximum-size cases remain tested.

Expand directions `-X,+X,-Z,+Z`; within each direction independently test flat, jump-up, fall-one and gap-two in that order and emit every admissible transition. Costs are 1,2,1,2. Jump requires the current cell's upper head clearance; gap requires intermediate feet/head clearance. Heuristic is horizontal Manhattan distance ignoring Y. Compare heap keys `(f, first_insertion_ordinal)`. Only strictly smaller `g` replaces parent/cost; equal `g` retains the first parent; a decrease-key preserves insertion ordinal. Closed nodes are not reopened under this consistent heuristic.

Use dense cell-indexed node state and an indexed min-heap with one open entry per cell. This replaces Go's hash map plus repeated linear open-list scan while preserving tie-breaking. Preallocate bounded scratch from validated cell count; no per-neighbor heap allocation. Reset touched state or generation stamps safely between calls; handle generation wrap by clearing without affecting results. Space is O(cells), heap work O(discovered nodes × log(cells)) plus O(cells) bounded setup; at most 4096 expansions and 16 neighbor attempts per expansion. Check the budget before expanding the 4097th node. No-path is unreachable; nonempty frontier at the exhausted budget is budget failure. Start equals goal succeeds only after standing validation and returns one waypoint.

Use widened checked arithmetic for dimensions, coordinates and Manhattan distance, then validated conversion to i32 indices/positions. Do not reproduce accidental Go overflow as intended gameplay. Extreme inputs outside safely representable bounds must be recorded as rejected boundary cases; any difference for an input the supported Go contract considers valid requires a controller compatibility ruling before release. Scratch/output failure returns no partially published path. Time-based cancellation must not change the deterministic result for an executed case.

Acceptance cases: flat corridor, each jump/fall/gap rule and blocked-clearance counterpart, diamond/tie cases, decrease-key preserving first insertion, equal-g parent retention, start=goal, blocked endpoints, unreachable, 4096/4097 boundary, unknown block, grid/count overflow, revision duplicate/conflict, scratch reuse across success/failure and path/result ownership independence. Compare exact path and revisions with Go for valid inputs; report runtime and allocations on fixed small/production/max/budget corpora without turning noisy timing into a hard gate.

## Risks / Trade-offs

- Broad API edits can hide parity changes → repair independently reproduced storage defects first; migrate shared values in separately reviewed nodes with Go-derived cases.
- Strict type validation can change legacy behavior → keep raw wire/save DTOs distinct and pin structural/admission asymmetry and migration rules.
- Independent handwritten goldens can repeat implementation mistakes → Go emits expected values; Rust round-trip alone cannot close a task.
- Buffer reuse introduces lifetime hazards → exclusive codec/scratch ownership and immutable publication; no global lock or borrowed packet outliving its input.
- Existing audit failures obscure new failures → retain baseline evidence and fix the oracle-induced boundary violation without granting an ABI exception. Baseline failures still block full closeout until resolved; they are not silently waived.

## Migration Plan

First repair the four concrete storage defects and protocol admission/export/encode defects with failing counterexamples. Establish the domain/value and buffer interfaces before further packet work. Replace inventory-as-evidence with actual corpus execution and repair trace validation/isolation. After domain/protocol/storage acceptance and trace infrastructure/case execution are repaired, the controller may release individually specified kernel API/pathfinding nodes. Whole-inventory and whole-trace acceptance close after the kernel cases; they are not prerequisites for starting those kernel nodes. Complete differential coverage and all stage gates before F2/F3 may consume F1 as accepted.

No online writer is introduced. Rollback disables/removes unused foundation artifacts and test tooling, preserving Go startup, source fixtures and existing engine ABI. No save downgrade or shadow authority is part of rollback.

## Validation and completion evidence

Each node records baseline/result SHA, exact failing counterexample, Go result, Rust before/after result, changed files, discovered/executed tests, corpus identity, remaining coverage and controller review. Compilation failure from a missing module is useful registration evidence but does not replace a behavioral red test for a repair. The controller checks public paths and invalid constructed values, not only constructors and self-round-trips.

Use the named commands linked from `tasks.md` and its task briefs, then formatting, `make rust-check`, `make dev-check` (six-module vet), `make test-race`, `go test ./packages/audit -count=1`, and strict OpenSpec validation at closeout. Do not bypass warnings/gates or launch a foreground game window. Planning validation establishes artifact coherence only, not implementation correctness.

## Superpowers design revision

The user requested a complete redesign of all unstarted nodes using Superpowers. The main Agent used installed `brainstorming` and `writing-plans`, verified legacy behavior and selected the target interfaces; isolated readers supplied source facts only. The goal is an executable specification for workers with no architectural decisions left implicit. Runtime implementation is still unstarted for these nodes.

Alternatives considered: (1) keep broad family tasks and let each worker fill gaps, which repeats the reviewed ownership and validation defects; (2) rewrite the entire runtime model in one node, which cannot be reviewed or rolled back independently; (3) freeze common contracts first and provide bounded family-specific implementation nodes with a shared oracle. Choose (3). The resulting plans explicitly separate preserved behavior from Rust-native memory ownership and algorithms.

The single status index is `tasks.md`. Detailed execution lives in `plans/01-evidence.md` through `plans/06-acceptance.md`; `execution-contract.md` defines shared signatures and `packet-contracts.md` pins packet field and boundary cases. No parallel plan store is created under `docs/superpowers/`. The old task numbering in historical ledger entries refers to the pre-Superpowers revision; the new index includes a migration map.


### Final planning decisions and source exceptions

The fully specified work is indexed by117 nodes (one retained accepted registration and116 unchecked implementation/acceptance nodes), including59 individual packet nodes. Read the exact numeric dependency list in tasks.md; historical IDs in the ledger refer to the prior revision. Detailed contracts are in [evidence](plans/01-evidence.md), [domain](plans/02-domain.md), [protocol](plans/03-protocol.md), [storage](plans/04-storage.md), [kernels](plans/05-kernel.md) and [acceptance](plans/06-acceptance.md). The [storage source table](storage-contracts.md) adds exact supported versions, private Go codec seams and migration exceptions.

- Current metadata codec preserves arbitrary weather bytes, raw spawn dimension and full day-phase value; runtime restoration owns normalization. Rust's current extra weather check is a newly source-inspected compatibility defect assigned4.7. Player armor is raw format fidelity, including values invalid as ordinary ItemStack.
- Domain command migration2.4 includes the one existing protocol validation call that depends on its old ten-argument constructor, so producer and consumer compile together without a temporary wrapper or clippy waiver. Packet constructors with more than7 parameters become validated field-record constructors within their family node.
- Command ordering uses caller-owned reusable scratch to reject duplicate arrival identities before mutating input. Chat has no sequence and remains a separate intent/FIFO.
- Native kernel signatures include complete request/result/scratch/error ownership. Worldgen signed X/Z fringe arithmetic is made explicitly wrapping to match deployed release behavior for every accepted i32 coordinate. Pathfinding uses checked/widened coordinates and four independent transition predicates.
- Native rescan accepts only interior1..16 ranges with a guaranteed halo; its legacy adapter preserves successful0..17 requests and maps a missing-neighbor failure to its previous caught-panic status. Mesh uses a40960-quad native staging bound that covers accepted custom registries, while retaining the24576 minimum-capacity legacy ABI rule; production registries still fit24576.
- Agent HTTP/MCP evidence reuses existing pure Go schema/DTO/tool validators in package-local tests. These externally owned contracts are labeled as such, without pretending that F1 implements a Rust or Python Agent runtime.
- Dev-only corpus dependencies are serde_json at the already locked version and sha2 0.10, with a narrow lock review. No foundation production dependency or game version changes.

The main Agent chose these policies. Workers may report contrary source evidence, but cannot silently change them, tighten compatibility, generate expectations from Rust output or start additional runtime ownership work.

## Reviewed baseline extraction and successor changes

### Intent and status

The original change is too large to remain one worker delivery: it contains 117
nodes spanning executable evidence, domain values, 59 packet families, seven
save families, numerical APIs, pathfinding and final acceptance. The first 16
checked nodes (1.1–1.6, 2.1–2.10) form a useful baseline, but independent
closeout reviews found that their checked status overstates executable evidence
and resource-bound coverage. They MUST NOT be archived as complete without the
repairs below.

The selected strategy is extract-and-retire:

1. create `rust-runtime-foundation-baseline` for only the implemented evidence
   and domain surface;
2. complete and independently review its six repair nodes;
3. sync its deliberately narrow delta into the canonical
   `rust-runtime-foundation` specification and archive it;
4. create the bounded successor changes listed below; and
5. archive this original change without syncing its superseded broad delta once
   every successor artifact is present and strictly valid.

Until the written successor plans are approved, this file records the selected
design only. The existing `tasks.md` remains the status source and no old task
is reinterpreted as completing a successor change.

### Alternatives rejected

- Keep this change as a parent tracker while successor changes also own task
  state: rejected because the same work would have two checkbox/status sources.
- Keep one change and only split worker briefs: rejected because planning,
  validation and archive scope would remain the same oversized rollback unit.
- Archive the 16 checked nodes without repairs: rejected because passing focused
  tests do not prove that the Go-produced corpus was executed by Rust or that
  coverage and trace validation fail closed.

### Baseline scope

`rust-runtime-foundation-baseline` owns only the behavior already intended by
nodes 1.1–1.6 and 2.1–2.10:

- the schema-2 offline corpus representation, isolated trace workspace, narrow
  audit allowance, framing seed case and service-free Agent contract cases;
- registration and dependency direction for `mornlea_domain`,
  `mornlea_protocol` and `mornlea_storage`;
- domain identities, text and scalar values, item/drop/container values, the 19
  sequenced command payloads, separate chat intent, deterministic command
  ordering, and player/world/inventory/remote-player/companion publications.

It explicitly excludes the unfinished hostile/passive, projectile/drop and
chat event families, the 30-variant event closure, complete packet/save
coverage, numerical APIs, pathfinding and final differential acceptance. The
baseline delta MUST describe partial executable infrastructure rather than
claim complete foundation coverage.

### Baseline repair nodes

Each repair is an independently rejectable test-first node. A worker receives
the frozen interface and file ownership from the later implementation plan and
escalates a contract conflict rather than changing these decisions.

1. **Working versus complete inventory validation.** Keep an explicitly named
   partial validation path for in-progress manifests. Add a separate complete
   acceptance path that binds every live family and supported version to
   executable positive and applicable negative cases and resolves each
   `rust_consumer` through a closed callable registry. The external Agent
   consumer is an explicit typed exception, not an arbitrary nonempty string.
2. **Executed traces and fail-closed expected values.** `RunTrace` MUST consume
   observations returned by registered operations and MUST NOT read expected
   files to manufacture observations. Trace validation receives an explicit
   corpus root and returns an error for root resolution, read, digest or JSON
   failures before accepting an outcome.
3. **Isolated corpus generation.** Remove CLI and test flags that rewrite the
   tracked manifest or frozen case directories in place. Generation publishes
   only into a fresh external harness directory through the existing
   containment and no-replace path; controller-reviewed import is a separate
   step. Tests construct repository-shaped fixtures under `t.TempDir()` and
   never rename a tracked corpus file.
4. **Rust corpus-loader parity.** Walk every path component without following
   symlinks, enforce resolved containment, reject duplicate JSON keys, unknown
   input formats and unknown consumer identities, and enforce the same byte and
   digest rules as the Go validator.
5. **Executed domain corpus.** Add a manifest-driven Rust consumer for all 376
   frozen cases genuinely owned by the implemented domain families. Dispatch
   through the real constructors and compare normalized values; accepted
   command cases omit only the Go codec's `fields.wire`. Correct the sole
   `domain.input` engine-step case to a closed
   `external:runtime-authority` consumer because its expectation contains Go
   authority admission and world effects that the domain orderer cannot
   produce. Keep focused Rust command-order tests for the pure ordering
   contract, require nonzero corpus discovery, and prove that a one-field
   semantic mutation fails.
6. **Bounded domain construction.** Check the 128-byte display-name bound before
   Unicode scalar iteration. Variable semantic batches use a shared maximum of
   4096 records before scanning, sorting or cloning; this is a domain work bound
   rather than the smaller packet caps, so existing multi-frame semantics remain
   representable. `ForgetChunks` uses fallible pre-reservation before copying
   its bounded uniqueness scratch. Allocation failure and an over-budget batch
   return typed domain errors without partial publication. Vector validation
   uses a neutral non-finite-value error rather than a rotation-only label.

The value 4096 reuses the established bounded world/publication work scale,
preserves the accepted tests that domain batches may exceed the remote-player
and companion wire caps, and prevents an arbitrary safe Rust caller from
triggering unbounded validation work. Protocol adapters retain their tighter
per-packet limits.

### Successor changes

Every successor is one independent OpenSpec status and rollback unit. Detailed
task packets will copy, not merely link to, the exact relevant contracts and
tests from this change so an isolated worker does not need the controller
conversation or a superseded task number to invent behavior.

| Successor change | Migrated nodes | Deliverable |
| --- | --- | --- |
| `rust-domain-event-completion` | 2.11–2.14 | Remaining entity/chat values and the complete 30-variant semantic event surface |
| `rust-protocol-foundation-and-login` | 3.1–3.3, 3.10–3.15 | Caller-buffer primitives, shared values, structural admission and handshake/login packets |
| `rust-protocol-client-world-intents` | 3.16–3.20, 3.23, 3.27–3.29, 3.31–3.32 | Client control, session and world-interaction packets |
| `rust-protocol-client-inventory-intents` | 3.21–3.22, 3.24–3.26, 3.30, 3.33–3.36 | Client inventory, crafting, container, armor and stack packets |
| `rust-protocol-server-world-session` | 3.37–3.43, 3.57 | Server chunk/player/session outcomes and placement success |
| `rust-protocol-server-player-inventory` | 3.44–3.47, 3.50–3.52, 3.58 | Remote-player, inventory, container and crafting publications |
| `rust-protocol-server-chat-companion` | 3.53–3.56 | Chat and companion publications |
| `rust-protocol-server-mobs-combat` | 3.59–3.65 | Hostile/passive and combat publications |
| `rust-protocol-server-object-drops` | 3.48–3.49, 3.66–3.68 | Item-drop and projectile publications |
| `rust-protocol-registry-closure` | 3.4–3.5, 3.69 | Complete typed registry, semantic adapters and allocation audit |
| `rust-storage-safety-repairs` | 4.1–4.5 | Queue/cardinality/chunk/region repairs and shared current values |
| `rust-storage-codec-closure` | 4.6–4.12 | Versioned player, metadata, entity, chunk, companion and region codecs |
| `rust-runtime-contract-gate` | 6.1–6.2 | Integrated domain/protocol/storage evidence and the pre-kernel gate |
| `rust-kernel-native-core` | 5.1–5.4 | Common safe surface, collision, physics and ray continuation |
| `rust-kernel-native-world` | 5.5–5.11 | Worldgen, probes, trees, LOD, fluids and mesh/light |
| `rust-kernel-pathfinding-and-corpus` | 5.12–5.14 | Immutable pathfinding plus complete numerical corpus |
| `rust-runtime-foundation-acceptance` | 6.3–6.6 | Differential replay, mutation gates, governance review and final stage gates |

### Dependency and integration order

The archived baseline is the first prerequisite. After it closes,
`rust-domain-event-completion`, `rust-protocol-foundation-and-login` and
`rust-storage-safety-repairs` may proceed independently. Both client protocol
changes and all five server packet changes depend on the protocol foundation.
The protocol registry closure depends on every packet change plus domain event
completion. Storage codec closure depends on storage safety repairs.

`rust-runtime-contract-gate` depends on domain completion, protocol registry
closure and storage codec closure. `rust-kernel-native-core` depends on that
contract gate. Kernel work is then serialized at its shared `api.rs`/`ffi.rs`
integration seam: `rust-kernel-native-world` depends on native core, and
`rust-kernel-pathfinding-and-corpus` depends on native world. Final
`rust-runtime-foundation-acceptance` depends explicitly on both the contract
gate and the complete pathfinding/numerical corpus, preserving the original
6.3 prerequisites even though the gate is also transitive through native core.
No successor may begin its implementation before these direct prerequisites or
claim F1 acceptance early.

The controller creates changes in dependency order, checks exact producer and
consumer signatures before dispatch, and keeps shared export/manifest edits
serial. Parallel workers never own overlapping crate roots, manifests, corpus
indexes or integration tests in the same worktree.

### Mandatory Superpowers lifecycle

Every successor proposal and every materially revised task packet uses
`superpowers:brainstorming` before design and `superpowers:writing-plans` before
implementation. Each behavior or defect node uses
`superpowers:test-driven-development`; review repairs also use
`superpowers:systematic-debugging`. Delegated execution uses
`superpowers:subagent-driven-development`, each independently verified node is
reviewed through `superpowers:requesting-code-review`, and no completion or
archive claim is made before `superpowers:verification-before-completion` has
fresh evidence.

These are task-entry requirements, not optional guidance. Each successor's
`tasks.md` carries the applicable skill list, exact red/green commands, files,
interfaces, exclusions, commit and rollback. The controller, not a worker,
updates task status and the append-only ledger after independently checking the
reported diff and commands.

### Archive and rollback policy

The baseline archives only after all six repairs, independent re-review,
focused Rust/Go/audit gates, stage-boundary gates and strict OpenSpec validation
pass at one recorded result SHA. Its delta is then synced because it describes
implemented behavior. This original change is archived later with its spec sync
explicitly skipped: its broad delta is superseded by the canonical baseline and
the successor deltas, and syncing it would claim unfinished behavior.

Before archiving the original change, verify every old unchecked node appears
exactly once in the successor map and every old checked node is either present
in the archived baseline or named by a baseline repair. The supersession ledger
records the mapping and validation evidence. Rollback restores this original
change as the planning source and removes only unimplemented successor
artifacts; it never reverts accepted runtime commits, rewrites frozen fixtures,
changes a game format or enables another online authority.
