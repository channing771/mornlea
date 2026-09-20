# Foundation review — 2026-09-20

Baseline: `60c476645ee6dae1f6392336a7f3c593d2163ae3`, branch `codex/align-runtime-migration-plans`. This is a review and planning correction, not an implementation repair. The foundation is not yet the production runtime, so the defects below are migration/acceptance blockers rather than evidence of an already-deployed Rust save failure.

## Findings

### R1 — P1: The oracle does not execute the behavior it claims to verify

`packages/tools/cmd/runtime-oracle/trace.go:131` creates one synthetic `checkpoint` input per tick and repeats the copied fixture digests as observations. No codec, storage migration, kernel or state transition executes. `discover.go:205` assigns the same two Go test source files to almost every packet as fixtures. A wrong Rust implementation can pass this evidence pipeline while every source file remains discoverable. This invalidates replay acceptance; it does not mean the individual Rust codecs do no work.

Separate provenance scanning from executable Go/Rust cases. The redesigned corpus must bind actual inputs and normalized outputs/errors to callable consumers and fail a deliberate behavioral mutation. The tooling import boundary currently forbids the adapters required for such execution; amend test-only adapter imports with corresponding negative architecture tests, not a production fallback. See design section 7 and tasks 1.3–1.8, 3.3 and 3.10.

### R2 — P1: Legacy companion decoding drops queue ownership

`packages/engine/crates/mornlea_storage/src/companion.rs:381` retains the result of `decode_queue_sections` without assigning the containing body's ID. The Go decoder in `packages/server/storage/companion/companion_codec.go:297` explicitly does that assignment. Committed v2/v3/v4 fixtures decode to queues with all-zero IDs in Rust; no body has that ID. Queued tasks/FIFO/summary therefore lose their entity association during migration.

An external regression test requiring each queue's ID to match a decoded record fails. The same Go fixtures return the expected UUIDs, including both bodies in v4. Bind identity before publication and compare exact queue contents, not only migrated schema/count. Task 2.5.

### R3 — P1: Chunk encoding accepts aggregates its decoder rejects

`packages/engine/crates/mornlea_storage/src/chunk.rs:711` validates each furnace/chest slot in isolation, while `validate_chunk` at line 783 additionally checks block index bounds, duplicate positions and matching blocks only on decode. An all-air chunk with a valid active furnace at index 0 successfully encodes; Rust decoding that output rejects it because index 0 is not a furnace block. Go rejects the same value at encode time.

Use one aggregate invariant in both directions, including the existing drop validations, before compression. Cover furnaces and chests, duplicates and out-of-range positions. Task 2.7.

### R4 — P1: Encoding can panic or emit invalid packets after ordinary public mutation

`packages/engine/crates/mornlea_protocol/src/player_input.rs:56` exposes infallible encoding that calls `expect`, despite all fields being public. Construct a valid input and set `yaw = f32::NAN`: `encode` panics with `InvalidFloat`. `inventory_state.rs:42` accepts a public `selected = 255` and emits bytes that its own decoder rejects. Validating a constructor once does not establish an invariant on a mutable public DTO.

Use private checked domain values and fallible aggregate encoding with validation before output publication. Review every public encoder, not only these two examples. Tasks 2.15–2.16. This cannot be resolved by adding a panic catch or changing tests to avoid public-field mutation.

### R5 — P1: Handshake/login semantic validation occurs before the required rejection path

`packages/engine/crates/mornlea_protocol/src/client_hello.rs:26` rejects a structurally valid version-44 hello during decode. `login_start.rs:54` rejects raw invalid identities and semantically invalid names/distances. Go explicitly permits structurally complete hello/login requests in `packages/shared/network/protocol/packet.go:506`; `packages/shared/network/login.go:74` and line 100 produce the frozen rejection responses. Rust would prevent a future login driver from observing these requests and returning the correct response.

External regressions for `[44]` hello and a complete login payload containing a zero UUID both fail in Rust. Existing Rust tests assert the incorrect strict-decode behavior, demonstrating why green tests are insufficient. Separate raw inbound requests, admission results and strict outbound records; preserve identity-before-distance error precedence. Task 2.9.

### R6 — P2: Region entry cardinality is not enforced before fixed-buffer writes

`packages/engine/crates/mornlea_storage/src/region.rs:107` models fixed 1024 entries as a public `Vec`. `validate_region_bank` at line 373 does not check its length; the encoder at line 208 writes every entry into a fixed bank buffer. A vector of 1200 default entries causes a bounds panic (`range end 28676 out of range for slice of length 28672`). Short input silently pads, and overlong input can reach padding before panicking. Go uses a fixed array.

Use boxed fixed-size storage and a checked raw-vector conversion, retaining extent/checksum validation. Test 0/1023/1024/1025/1200 lengths and last-slot output. Task 2.8.

### R7 — P2: Companion encoding omits its 64-record bound

`packages/engine/crates/mornlea_storage/src/companion.rs:1205` clones and sorts all records without checking `MAX_STORED`. Sixty-five distinct inactive bodies with matching lifecycles encode successfully to 16038 bytes; decoding immediately rejects the count. Go rejects the same input before encoding. The omission also allows input-proportional clone/sort work beyond the format budget.

Validate body/lifecycle/queue counts before cloning, then retain existing active-count, identity-set and queue-membership checks. Task 2.6.

### R8 — P2: ContainerClosed is absent from the compiled public API

`packages/engine/crates/mornlea_protocol/src/container_closed.rs` exists and defines server/play packet 14, but `src/lib.rs` neither declares the module nor exports its type. The integration suite contains no `ContainerClosed` case. The historical claim that all 60 protocol inventory families were exported is false: this supported packet cannot be consumed by another crate.

Add compiled typed state/direction/ID dispatch and an external integration case. Counting source files or inventory names cannot prove coverage. Task 2.14. Family counts describe this review baseline only and are not a durable acceptance rule.

### R9 — P2: Trace validation admits incomplete evidence and path isolation is bypassable

`packages/tools/cmd/runtime-oracle/trace.go:196` validates only nonempty top-level fields. A trace with ticks `[1,2,3]`, no inputs, one observation at tick 999, no fixture digests and invented nonempty identity strings passes. An external test requiring rejection fails.

The separate path defect is at `trace.go:280`: it resolves only the target or immediate parent. A symlink ancestor into the repository followed by multiple nonexistent descendants is classified as safe; a later `MkdirAll` would follow it into the repository. The external test used an isolated fake repository and created no files in the actual checkout. Validate all case/checkpoint/content relationships and resolve the deepest existing ancestor before creation. Tasks 1.4–1.5.

### R10 — P2: Domain input ordering differs from the authoritative order

`packages/engine/crates/mornlea_domain/src/input.rs:190` sorts by sequence and then kind name, with no session identity or original-arrival tie. `packages/server/sim/runtime/engine_step.go:70` stable-sorts by session and sequence, then admits the first command for a duplicated sequence. Reversing two different command kinds can therefore change which command is selected; sorting all sessions by sequence also differs.

The new helper is not yet used by a production Rust runtime, but its claimed semantic parity is wrong. Introduce a session-tagged envelope and preserve the input arrival tie. Task 2.12. The full ordering integration was inspected statically; this review did not execute a complete server simulation replay.

## Architectural issues beyond individual defects

`mornlea_domain` currently owns only a small subset of value/input semantics and a digest-bearing `Observation`. Meanwhile protocol and storage each implement their own `PlayerId`, `ItemStack`, item numbering, durability and smelting rules. These are shared domain rules, not independent wire/save policies. Crate registration is correct, but shared ownership is incomplete. Design sections 2–3 specify consolidation while preserving format-local historical DTOs and raw sentinels.

The interfaces also lack a resource contract: framing allocates varint vectors and copies payloads; fixed-array parsing stages through a vector; compressed paths create contexts/copies per call. This review has not measured game throughput or proved an end-to-end performance regression. The finding is missing allocation/copy/work acceptance, addressed by caller-owned buffers, borrowed views, exclusive reusable contexts and deterministic bounds in design section 5.

The numerical stage must expose typed safe Rust APIs over the existing core, not make Rust callers construct C ABI packets. Go pathfinding's linear open-list scan is a behavior source, not a required data structure. Design sections 8–9 distinguish preserved semantics from Rust implementation choices and explicitly reserve unfinished per-family API packets for the controller.

## Validation performed

- Passed: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --test runtime_contract --locked` — 10 domain, 133 protocol, 63 storage tests (206 total). This command covers the named integration targets, not all workspace tests.
- Failed: `rustup run 1.97.1 cargo clippy --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --all-targets --locked -- -D warnings` — first failure is `PlayerInput::new`'s ten arguments (`clippy::too_many_arguments`, domain `input.rs:32`). `make rust-check` includes this warnings-as-errors gate. Protocol test builds also emit 16 warnings; no claim is made that fixing the first lint closes the gate.
- External Rust storage regressions: 4 expected-contract tests failed, proving R2/R3/R6/R7. A separate Go program verified the fixture queue IDs and rejected 65 companions and furnace-on-air.
- External Rust protocol regressions: 4 expected-contract tests failed, proving both R4 examples and both R5 admission examples.
- External Go oracle regressions: incomplete-checkpoint and symlink-ancestor rejection tests failed; changing only the seed left fixture observations unchanged, consistent with source inspection. Seed independence by itself is not a defect for seed-independent codec cases.
- External reproductions were placed in temporary directories. No runtime source, checked-in fixture or production save was modified. Their inputs and expected outcomes are specified above and in the repair tasks so worker regressions can be durable repository tests.

No full workspace race/dev/platform gate or graphical test was run in this review. Existing ledger evidence identifies an oracle-induced native-header audit failure and pre-existing documentation-version/comment audit failures; they remain closeout blockers. OpenSpec/document checks for this planning revision are recorded in the appended ledger entry.

## Assessment

Do not proceed on the premise that sections 1–2 are accepted. The immediate work is to repair known invariant violations, replace nonbehavioral evidence, and consolidate shared ownership. The controller's earlier completion reviews were insufficient: constructor tests, round-trips and filenames did not test the actual publication/admission/migration boundaries. The corrective workflow freezes those boundaries and their negative examples before implementation workers are dispatched. It does not delegate architecture back to the workers or claim that every future kernel/event detail has already been finalized.
