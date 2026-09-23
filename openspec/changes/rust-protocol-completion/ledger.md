# Rust protocol completion ledger

## Planning entry — 2026-09-23

**Status:** planning artifacts only. No protocol implementation node is checked off and no production runtime has changed. Planning baseline is clean `main` at `2750284d`; execution must recheck it and reconcile drift before node 1.1.

**Scope ruling:** the user selected the F1 protocol successor. The prior F1 domain-event change is accepted and archived. F2's Rust authoritative server still requires protocol, storage, safe numerical APIs, pathfinding and final F1 acceptance. This change supplies the complete offline v45 protocol boundary for later Rust F2/F3 consumption and does not claim that F1 or F2 is complete. Current Go protocol, codec and login paths are compatibility authorities only. The final owner is Rust `mornlea_protocol` plus checked `mornlea_domain`; Godot/Python has no wire parsing or authoritative action path.

**Decomposition ruling:** do not mechanically preserve the earlier broad F1 task. The Go registry has 23 client and 36 server keys, while the frozen corpus has only two frame cases. The accepted domain surface has 19 sequenced client commands, distinct chat and keepalive inputs, and 30 publication events. The plan uses 25 independently verifiable nodes: executable evidence and common boundaries; five client packet groups; eleven server packet groups; closed registry, semantic bridge, coverage closure and whole-change review. A node owns exact files, an input/boundary table, red/green tests, producer and consumer evidence, focused commands, exclusions, commit and rollback. `tasks.md` alone carries status.

**Architecture decisions:** F1 owns direction/state/key dispatch, canonical framing, structural negotiation, raw wire DTOs, bounded codecs and explicit checked-value conversion. F2 owns session state, admission timing, command envelope metadata, simulation authority and routing. F3 consumes the inverse typed API. Snapshot compression owns one bounded context per caller and compares logical data across Go/Rust encoders. Raw `i32` container dimensions and exact absence sentinels remain wire-only; checked semantic types come from `mornlea_domain`. All public encoders validate current values before capacity and publication. The corpus has independent Go codec producers and Rust execution, with the login driver's decision order tested separately at its package boundary.

**Evidence and derived consumers:** the source-scanned Go registry and protocol/codec/login files are read-only compatibility inputs. `packages/tools/cmd/runtime-oracle/discover.go`, `inventory.go`, `runner_helpers_test.go` and the producer tests own discovery, closed routes, source hashes, case generation and the external create-exclusive export; the controller serializes route and `BaselineSourceRevision` edits. `testdata/runtime-migration/contracts.json` and case assets are read by the Go inventory/trace tests and by `packages/engine/tests/runtime_corpus.rs` plus the Rust protocol/domain integration suites. The controller reviews and merges candidate assets and the complete manifest, then runs the full runtime-oracle suite so preexisting exact totals remain current. `packages/audit/runtime_corpus_write_guard_test.go`, dependency and baseline tests scan producer flags, imports and version facts; they are downstream gates for new Go producer files. Rust `lib.rs`/codec exports and scoped guides are controller integration points; crate tests, `make rust-check` and the engine/audit gates validate them. No new Cargo dependency or lockfile change is planned. Repeat the repository-wide derived-consumer search if an editable-file set changes during execution.

**Review ruling:** an independent read-only plan review checked the 59 registry keys and found two interface gaps. Node 3.2 now exposes only snapshot-specific methods; node 4.1 adds `ServerPacket`-based codec dispatch after the enum exists. Node 1.4 now owns ordinary ClientHello/LoginStart decode/encode corpus routes and a paired package-local Go login-driver test for semantic-invalid inbound values that the outbound Go encoder cannot produce. No additional packet name was missing. The main controller owns these revisions and all later contract rulings.

**Orchestration:** three narrow read-only evidence agents and one independent plan reviewer were used; no implementation worker was dispatched. Isolation was justified by the breadth of Go/Rust registry extraction and independent interface criticism. No GPT-6 Astra subagent was used. Implementation mode remains unselected because this request is planning only; a later controller may choose direct, delegated or mixed execution under the standing project authorization and must check each node's readiness before dispatch. No architecture-skill promotion is warranted from planning-only observations: **Architecture skill: no change**.

**Planning validation:** `openspec validate rust-protocol-completion --strict --no-interactive` exited 0 after the final contract edits (`Change 'rust-protocol-completion' is valid`). `openspec validate --all --strict --no-interactive` reported 127 passed and 0 failed. A local link/whitespace scan found ten Markdown files, zero bad relative links, zero trailing whitespace and zero missing final newlines. The task/anchor scan found 25 unique open nodes with 25 matching anchors (groups 5/5/11/4). `git diff --check` initially found one extra blank EOF line in the wire matrix, which was removed; the final `git diff --check` exited 0. Implementation tests and closeout gates have not run for this planning-only change.

## Node 1.1 — 2026-09-23

**Status:** complete. Commits `9e9f3bfb` (code+tests) → `2a01e8ba` (controller corpus integration) → `6e398222` (review fixes) on branch `codex/rust-protocol-completion`; baseline `0f5ff747`.

**Deliverable:** executable encode corpus route (`protocol.frame/45/encode`), protocol-only runner `RunProtocolCases` with closed route map (missing route rejected before inputs), manifest merger `mergeProtocolSelections`, Rust `CorpusConsumer::Protocol` (`mornlea_protocol`) registration, and exactly the 18 named protocol producer IDs allowlisted beside the frame producer.

**Evidence:** case `protocol.frame/45/encode-id-128` (input `{"packet_id":128,"payload":"010203"}`, wire `[5,0x80,0x01,1,2,3]`, digest `sha256:06e2771e…` independently recomputed by controller). Manifest diff confined to `protocol.frame` case list + one top-level entry; `source_revision` `736af2f4…` and all 85 unrelated families byte-identical; frame provenance hashes unchanged (production codec sources untouched). Go: `^TestProtocolCorpus` 14/14, full oracle package `ok` (15s) and `-race` `ok` (94s, controller-run). Rust: `protocol_corpus` 3/3 after integration, `runtime_contract` 134/134, `corpus_domain` 54/54 after total fix, clippy `-D warnings` clean, fmt clean, audit `ok`.

**Review ruling:** task reviewer found one Critical — the integrated corpus broke the pre-existing `corpus_domain.rs` exact-total pin (2 vs 3 frame cases; reviewer reproduced). Controller fixed the total, removed a stale AGENTS.md test name, and applied a clippy fix; re-review **Approved**. Deferred Minors recorded (non-blocking): dual-consumer frame-route claim under `mornlea_protocol` (controller ruling: registry forbids empty route sets, so the packet consumer is born with the framing routes it executes today; node 4.3's closed-union gate is the backstop); `runFrameEncode` maps every `WriteFrame` error to `capacity` (only the bounds refusal is reachable today); `protocol_corpus_packet_consumer_selection_is_executable` is a zero-case placeholder until the first packet group; `mergeProtocolSelections` uses a fixed-name temp file (no `t.Parallel` in package). `RunCases` gained a duplicate-checkpoint rejection and case-ID error prefixes (fail-closed deltas, full package green).

**Rollback:** revert `9e9f3bfb..6e398222` as one unit; the two preexisting frame cases and their digests are untouched, so revert restores the exact pre-node corpus.

**Architecture skill: no change** (route-runner split and closed consumer registry are already recorded in the runtime-oracle guide).

## Node 1.2 — 2026-09-23

**Status:** complete. Commit `d2f715e5` (`feat(protocol): add bounded caller-buffer framing`) on top of `41527e4c`.

**Deliverable:** `FrameRef<'a>` borrowed first-frame read (`read_frame_ref`: canonical prefix, nonzero body, 2 MiB ceiling, exact `consumed`, borrow at original offset), `write_frame_into` caller-buffer write with size-before-capacity ordering, private `SliceWriter`/`publish_packet` exact-prefix publication, new `ProtocolError::{UnknownPacket, OutputTooSmall{needed,available}, Allocation}`; allocating wrappers retained as delegating compatibility paths.

**Evidence:** `protocol_frame` 13/13 (two reds first: missing-API compile failure + baseline allocating reader); `runtime_contract` 134/134 unchanged; `protocol_corpus` 3/3; whole crate ok; Go `TestFrame|TestCanonicalUvarint` ok; fmt + clippy `-D warnings` clean. Allocation: borrowed read 0 allocs over 256 reads after warm-up, allocating contrast ≥1 (counting GlobalAlloc, thread-local; repo precedent followed). Boundary matrix (2,097,152 admits / 2,097,153 rejects; varint lengths 1/2/3/4/5; fifth byte 0x0f max; `[0xa5;5]` untouched with `OutputTooSmall{6,5}`) all pinned.

**Review ruling:** Approved, 0 Crit/0 Imp. Controller rulings from review: (1) corpus category mapping — `ProtocolError::Allocation` maps to the frozen `capacity` category ("declared size or destination shortage") when packet nodes freeze negative encode cases; (2) `SliceWriter::f32` deliberately publishes raw bits — packet modules must reject non-finite values in `validate()` before any write, per design §4 ordering; the first packet node adds an explicit non-finite encode rejection case. Deferred Minors for final review triage: unused `written()`/`remaining()` writer surface until first packet consumer, unused `encode_uvarint` import (crate allows unused_imports), double `frame_sizes` on the compatibility write path, AGENTS.md half-open range wording, debug-only cursor assertion (all crate gates run debug).

**Rollback:** revert `d2f715e5`; wrappers' byte output is provably identical to the pre-node encoder, so `runtime_contract` and the corpus stay valid on either side.

**Architecture skill: no change.**

## Node 1.3 — 2026-09-23

**Status:** complete. Commit `38cf5eb4` (`refactor(protocol): use checked domain values`) on top of `e5059ed0`.

**Deliverable:** protocol reexports domain `PlayerId`/`CompanionId`/`ItemStack`/`DropId` (duplicate protocol item tables deleted; domain is single owner); wire `ContainerRef` keeps raw i32 dimension with exact `NONE` sentinel, `to_domain_present`/`to_domain_optional`; `CompanionId::NONE` unreachable as domain identity (chat absent-branch stays raw bytes); checked `TryFrom` section conversions moving palette/words exactly with separate Y; domain gains only `trim_pinned_whitespace` (same pinned predicate; `try_from_canonical` still does not trim). Packet modules mechanically updated; `runtime_contract` golden bytes and error expectations unchanged.

**Evidence:** protocol_values 8/8 (reds first: runtime dimension-256-narrowing + missing conversion APIs); domain items_locations 14, identity_values 13, runtime_contract 7, event_world 17; whole protocol crate 158 green; fmt + clippy `-D warnings` clean both crates. Reviewer verified duplicate-table removal by grep, exact sentinel semantics, lossless section moves, pinned-whitespace predicate equality with the Go set (U+200B kept, U+00A0/U+3000 trimmed).

**Review ruling:** Approved, 0 Crit/0 Imp/4 Minor. Controller ruling from Minor 1: the neutral container gate now checks dimension before kind (`to_domain_present` delegation), changing observable error variants for doubly-invalid references while the admitted set is unchanged — node 2.4 MUST verify this order against the real Go validator's observable rejection when freezing corpus categories; a genuine Go/Rust category divergence returns to the controller for a design ruling, not a silent Rust-side fix. Deferred Minors: validation-path `String` allocation in name gates, one process-language doc phrase, one weak OR assertion.

**Rollback:** revert `38cf5eb4`; domain's `trim_pinned_whitespace` is additive, and packet modules return to the deleted local tables within the same commit.

**Architecture skill: no change.**

## Node 1.4 — 2026-09-23

**Status:** complete. Commits `59f545b9` (`feat(protocol): separate inbound admission from parsing`) + controller integration `e7bfbdf5` (`chore(corpus): integrate negotiation packet evidence`) on top of `cf24bed1`.

**Deliverable:** structural `decode_inbound` for ClientHello/LoginStart (canonical uvarint/UTF-8/64 KiB/full-consumption only — old version and over-128-byte raw name survive decoding); pure `validate_hello` (≠45 → `VersionMismatch{45}`) and `admit_login` (UUID → pinned-trim canonical name → distance 2..=64; `InvalidIdentity`/`ProtocolViolation`); `AdmittedLogin` read-only. Corpus: 10 cases under `protocol.client.{ClientHello,LoginStart}/45` (4+6: valid decode + valid encode + malformed decodes) through the real Go codec, `mornlea_protocol` consumer routes, producer `runtime-oracle/protocol-negotiation`. Paired Go evidence: `packages/shared/network/protocol_admission_oracle_test.go` observes `BeginServerLogin` through the existing stream seam (no runtime-oracle import, no duplicated decision tree); 12 paired case IDs matched bidirectionally with the Rust suite.

**Evidence:** protocol_admission 16/16 (reds first: UnsupportedVersion-at-decode + name-limit-before-trim); Go admission oracle 5 tests; negotiation oracle 9; full oracle package ok + `-race` ok (95s); whole protocol crate green post-integration (4 corpus incl. the 10 negotiation cases + 16 admission + 134 runtime_contract); clippy/fmt/vet/gofmt clean; audit ok. Corpus integrity: only the two families changed, case index 691→701 strict superset, `source_revision` preserved, provenance = real production codec/protocol files.

**Controller integration incident + fix:** post-integration the Rust expected-case list compared in authoring order while the merged manifest sorts IDs; fixed by sorting the expected vector (reviewer verified the fix correct and minimal). **Review ruling:** Approved, 0 Crit/0 Imp/6 Minor. Category rulings recorded: invalid UTF-8 name → `invalid-value`; 64 KiB payload refusal → `capacity`; latent declared-length-exceeds-remaining pair (Go `truncated` vs Rust `InvalidString`) — the first node freezing such a case must pin `truncated` (the frozen vocabulary's incomplete-bytes category) and align the Rust derivation; divergence returns to controller. Deferred Minors: doc-comment wording (panic vs compile-time exhaustiveness), dead empty-guard, sort-both-sides robustness suggestion.

**Rollback:** revert `59f545b9..e7bfbdf5`; the two families return to zero cases and the corpus index returns to 691.

**Architecture skill: no change.**

## Node 1.5 — 2026-09-23

**Status:** complete. Commits `27ff5a60` (`feat(protocol): qualify control packet codecs`) + controller integration `ce197b28` (`chore(corpus): integrate control packet evidence`) on top of `a6affab3`.

**Deliverable:** the seven control families (ServerHello, HandshakeReject, LoginSuccess, LoginReject, KeepAlive, KeepAliveReply, Disconnect) share the common fallible surface (validate → checked encoded_len → encode_into → fallible encode → decode) over the node-1.2 writer; shared `pub(crate)` control-message helpers (single publication half, no public leak); 34 corpus cases (4+8+5+3+7+4+3) through the real Go codec under the `mornlea_protocol` consumer, producer `runtime-oracle/protocol-control`; exhaustive proper-truncation loops over the nine canonical vectors; invalid-before-capacity mutation matrix with sentinel-unchanged checks across five call shapes.

**Evidence:** protocol_control 10/10 (red: infallible encode surface); runtime_contract 134/134 (mechanical `encode().expect` updates only, bytes/errors pinned); Go control oracle 9/9; full oracle package ok; audit ok; fmt/clippy/vet/gofmt clean; post-integration whole protocol crate green (corpus 5/5 incl. 34 control + generic consumer selection). Corpus integrity: exactly 7 families changed, `cases: null` markers removed only, 701→735 strict superset, `source_revision` preserved, provenance hashes re-verified by reviewer.

**Controller integration incident + fix:** the generic `dispatch_case` in `protocol_corpus.rs` lacked the seven control family constants (the control group test used its own dispatch path); fixed by adding the constants to the dispatch arm (reviewer verified correct and minimal). **Review ruling:** Approved, 0 Crit/1 Imp/6 Min. The Important item is procedural and now ruled: payloads cut inside a length-prefix varint publish `invalid-varint` from Go's sentinel mapping but `truncated` from Rust's `Truncated` — **no corpus case may sit on that boundary until the two sides agree**; the target category per the frozen vocabulary's semantics is `truncated` (incomplete bytes), to be achieved by routing the Go producer's `errInvalidUvarint` on proper-prefix payloads through the rejected-prefix resolver while Rust keeps `Truncated`. A node needing such a case first returns to the controller. Deferred Minors: truncation enumeration over the 9 canonical vectors only (max-message payloads excluded), case-ID-keyed resolver heuristic, unreachable `Allocation` path in `control_message_len`, `.expect` in `message_length_prefix` guarded by caller order, four-fold control-family list duplication in the corpus test, order-dependent export guard.

**Rollback:** revert `27ff5a60..ce197b28`; the seven families return to null case lists and the corpus index returns to 701.

**Architecture skill: no change.**

## Node 2.1 — 2026-09-23

**Status:** complete. Commits `9e707cf5` (`feat(protocol): qualify client control packets`) + controller integration `8ce2d132` (`chore(corpus): integrate client control packet evidence`) on top of `5abf9eb6`; an unrelated parallel-session planning commit `cdf8676a` interleaved between them (path-disjoint).

**Deliverable:** the four client control families (`PlayerInput`, `PlaceBlock`, `RequestChunkResync`, `SelectHotbar`) on the common fallible surface (validate → checked encoded_len → encode_into → fallible encode → decode) over the node-1.2 writer; `RequestChunkResync::decode` replaces the `u8::try_from(..).unwrap_or(u8::MAX)` dimension narrowing with an explicit raw-i32 {0,1} match rejecting everything else as `InvalidEnum`; 19 corpus cases (PlayerInput 6 / PlaceBlock 5 / RequestChunkResync 4 / SelectHotbar 4) through the real Go codec under the `mornlea_protocol` consumer, producer `runtime-oracle/protocol-client-control`; the eight decode/encode routes registered in `ORACLE/inventory.go` and pinned in `inventory_test.go`; corpus consumer arms + exact case-ID list in `tests/protocol_corpus.rs`; new group test `tests/protocol_client_control.rs`; mechanical `.encode().expect` updates for the four families in `runtime_contract.rs` (pinned bytes/errors unchanged); AGENTS.md sync for both crates' guides.

**Controller rulings frozen by this node:** f32 values travel as eight-digit lowercase-hexadecimal bit strings in encode-case JSON requests and normalized fields (both sides), so `-0.0` survives JSON deterministically — later packet groups with float fields reuse this encoding; the four encode-negative twins exercise the Go outbound validator on values the inbound path also rejects; the group's Go category resolver matches the shared substring `slot is outside 0..8` for both hotbar validators.

**Evidence:** group test 9/9 with `--list` nonzero; `runtime_contract` 134/134; Go `TestProtocolClientControlOracle*` 8/8; full oracle package `ok` (19s); `packages/audit` `ok`; fmt/clippy/gofmt/vet clean (implementer-run). Corpus integrity: exactly 4 families changed (`cases: null` markers removed only), 735→754 strict superset (+19/−0), `source_revision` preserved, provenance = the two named live sources with identical hashes across all four families; controller spot-checked wire bytes (PlayerInput 23-byte valid literal, mutation offsets), normalized field encodings and SHA256 digests against the merged manifest before integration. Post-integration whole protocol crate green: `protocol_corpus` 6/6 with the 19 client-control cases executed, tracked corpus byte-identical after the runs. Corpus tree: `git rev-parse 8ce2d132:testdata/runtime-migration`.

**Review ruling:** Approved, 0 Crit/0 Imp/4 Minor. Deferred Minors for final review triage: comment typo "headecimal" (`protocol_client_control_test.go` float-encoding comment); PlayerInput decode arm in `protocol_corpus.rs` inlines the field map the shared renderer already produces; the no-narrowing pin covers dimensions 2 and 256 but not −1; an unreachable `exceeds 64 KiB` → capacity branch in the group's category resolver (latent misclassification risk if a future producer reuses the table).

**Incidents:** during task review an external actor switched the shared checkout to `main` (observed by the reviewer; node commits were safe on the branch); the user authorized switching back, and the parallel session's own planning commit `cdf8676a` landed on this branch during the gate runs — all node-2.1 commits are path-scoped and disjoint from both events.

**Rollback:** revert `9e707cf5` and `8ce2d132` individually (the interleaved `cdf8676a` is path-disjoint and must not be reverted with them); the four families return to `cases: null` and the corpus index returns to 735.

**Architecture skill: no change.**

## Node 2.2 — 2026-09-23

**Status:** complete. Commits `ed944b5f` (`feat(protocol): qualify client ray actions`) + controller integration `df40d0d3` (`chore(corpus): integrate client ray evidence`) on top of `fd77dc9e`.

**Deliverable:** the five ray action families (`OpenContainer`, `TillSoil`, `BoneMeal`, `CollectWater`, `PlaceWater`, Play C→S IDs 8/13/14/16/17) on the common fallible surface over the node-1.2 writer; each payload is exactly 16 bytes (seq u64 LE, yaw f32 LE bits, pitch f32 LE bits) with no target position, held item, container kind or result; 25 corpus cases (5 per family: decode-valid, encode-valid, decode-nan-yaw, decode-infinite-pitch, encode-nan-yaw) through the real Go codec under the `mornlea_protocol` consumer, producer `runtime-oracle/protocol-client-rays`; the ten decode/encode routes registered in `ORACLE/inventory.go` and pinned in `inventory_test.go`; corpus consumer arms + exact case-ID list in `tests/protocol_corpus.rs`; new group test `tests/protocol_client_rays.rs` (including the `-0.0` bit pin, the 16-byte purity pin, the NaN/Inf mutation matrix and the baseline `.expect`-panic red); mechanical `.encode().expect` updates for the five families in `runtime_contract.rs`; AGENTS.md sync for both guides. Node 2.1's f32 hex-bit encoding ruling applied unchanged and its float request/consumer helpers were reused, not forked.

**Evidence:** group test 8/8 with `--list` nonzero; `runtime_contract` 134/134; Go `TestProtocolClientRaysOracle*` 8/8; `TestPrimitiveFloatRejectsNonFiniteValues` ok (implementer-run). Corpus integrity: exactly 5 families changed (`cases: null` markers removed only), 754→779 strict superset (+25/−0), `source_revision` preserved, provenance = per-family message file (`message_container.go` for OpenContainer, `message_command.go` for the other four) plus `codec_client.go`; controller spot-checked the 16-byte literal, the NaN word at offset 8 and hex-bit request fields before integration. Post-integration whole protocol crate green (`protocol_corpus` 7/7 with the 25 ray cases executed), full oracle package `ok`, audit `ok`.

**Controller integration incident + fix:** `inventory_test.go`'s two zero-case-family fixtures used `protocol.client.BoneMeal`, which this node filled; the fixtures now use `protocol.client.ChatCommand` (still zero-case until node 2.5). This is the documented shared-family maintenance act; the fix is part of the integration commit.

**Review ruling:** Approved, 0 Crit/0 Imp/3 Minor. Deferred Minors for final review triage: the template's allocating `encode` double-validates through `encoded_len` (byte-identical to the node-2.1 template; final review decides whether the template itself is simplified); resolver comment order vs branch order; RED-2 baseline-panic evidence captured via a scratch test deleted after capture (corroborated by the removed `.expect` line in the diff).

**Orchestration note:** both dispatches for this node used the generic subagent type instead of the newly mandated project subagents `superpowers-implementer`/`superpowers-reviewer` (controller error at dispatch time); the full role prompts were embedded verbatim and review remained an independent gate. Subsequent nodes dispatch through the project subagents.

**Rollback:** revert `ed944b5f` and `df40d0d3` individually; the five families return to `cases: null` and the corpus index returns to 754 (the zero-case fixture swap returns with them).

**Architecture skill: no change.**

## Node 2.3 — 2026-09-23

**Status:** complete. Commits `136811d7` (`feat(protocol): qualify inventory command bytes`) + controller integration `13ea634e` (`chore(corpus): integrate inventory command evidence`) on top of `1311882e`.

**Deliverable:** the six simple inventory/crafting families (`MoveInventoryStack` 6, `MoveCraftingStack` 7, `CloseContainer` 10, `DropSelectedItem` 11, `EquipArmor` 18, `TakeCraftingOutput` 15, all Play C→S) on the common fallible surface; the two move payloads are 10 bytes (seq + from + to) with the Go rule set (inventory 0..35 distinct; crafting view 0..44 distinct with the both-in-inventory exclusion), the four sequence-only payloads are 8 bytes with `TakeCraftingOutput` alone rejecting sequence 0; no personal-grid 4..8 authority rule entered the protocol. 28 corpus cases (6+6+4+4+4+4: valid pairs, the named slot/same-slot/two-inventory/zero-sequence negatives as decode mutations plus encode twins, truncation/trailing for the sequence-only families) through the real Go codec, producer `runtime-oracle/protocol-client-inventory`; 12 routes registered and pinned; corpus consumer arms + exact 28-ID list; group test `tests/protocol_client_inventory.rs` (mutation matrix for the three rule-bearing families, total-validate u64::MAX round-trip for the three sequence-only families, `encoded_len` 10/8 pins, purity pins); mechanical `runtime_contract.rs` updates; AGENTS.md sync for both guides.

**Evidence:** group test 9/9; `runtime_contract` 134/134; Go `TestProtocolClientInventoryOracle*` 8/8; `TestSmallPacket` ok; full oracle package `ok`; audit `ok`. Corpus integrity: exactly 6 families changed, 779→807 strict superset (+28/−0), `source_revision` preserved; controller spot-checked the three valid literals (`00×8|00|23`, `00×8|08|2c`, `01 00×7`) and the inventory-to-inventory mutation bytes before integration. Post-integration whole protocol crate green with the 28 cases executed.

**Controller arithmetic slip (recorded):** the brief's prose said "26 cases / 779 → 805" while its binding label table enumerates 28; the implementer followed the binding table (literal-over-prose precedent from node 2.1).

**Review ruling:** Approved, 0 Crit/0 Imp/3 Minor. Deferred Minors: a no-op `resize` in the sequence-only round-trip test; a `want_stride` assertion implied by the payload-length check (pin per family if a real stride pin is wanted); the total-family doc-comment phrase "shared by encode_into and decode" inherited from `request_chunk_resync.rs` (decode routes through `new` for those families) — normalize only if the crate decides the phrase must be exact everywhere.

**Rollback:** revert `136811d7` and `13ea634e` individually; the six families return to `cases: null` and the corpus index returns to 779.

**Architecture skill: no change.**

## Node 2.4 — 2026-09-23

**Status:** complete. Commits `b348b0e5` (`fix(protocol): validate all stack-view references`) + review-fix `734564bf` (`docs(protocol): correct stack-view reference wording`) + controller integration `1143189e` (`chore(corpus): integrate stack-view command evidence`) on top of `b4c6f70a`.

**Deliverable:** the four container and view-addressed families (`MoveContainerStack` 9, `MoveStackPartial` 19, `QuickMoveStack` 20, `DropStack` 21) on the common fallible surface with real container-reference validation: the shared private `validate_stack_view` requires the exact all-zero `NONE` sentinel in views 0/1 and runs `to_domain_present` in view 2 before index bounds; `MoveContainerStack`'s gate reorders to ref → same-slot → per-kind range → furnace-output-target, matching Go; `ContainerRef::read` keeps the raw i32 dimension untouched (the node-1.3 raw-fidelity contract now pinned at the conversion layer — `to_domain_present` refuses 256/−1 — with the packet gate refusing at the boundary). 29 corpus cases (10/9/5/5) through the real Go codec, producer `runtime-oracle/protocol-client-stack-views`; 8 routes registered and pinned; group test `tests/protocol_client_stack_views.rs`; mechanical `runtime_contract.rs` updates (no fixture repair needed); AGENTS.md sync.

**Controller ruling frozen by this node:** every corpus case and group-test negative carries exactly ONE violation — kind∉{0,1} combined with a nonzero dimension classifies differently on the two sides (Go kind-first → invalid-enum; Rust neutral conversion dimension-first → invalid-value) — the ruling node 1.3 deferred to this node.

**Controller arithmetic slips (recorded):** the brief's stride prose said 28/31/27/27; the binding wire literals and Go codec publish 28/30/28/28, which the implementer pinned. The brief's parenthetical mutation bytes for two MoveStackPartial cases described a different single-violation mutation than implemented; the implemented bytes are single-violation and category-identical (reviewer verified).

**Evidence:** group test 14/14; `runtime_contract` 134/134; `protocol_values` 8/8 with the renamed pin; Go producer 8/8 over 29 cases; `TestStackSplit|TestDropStack` 6/6; fmt/clippy/gofmt/vet clean; full oracle package `ok`; audit `ok`. Corpus integrity: exactly 4 families changed, 807→836 strict superset (+29/−0), `source_revision` preserved; controller spot-checked the furnace ref literal, the NONE-sentinel QuickMove vector, the single-violation unknown-view mutation and the nested zero-object encode request. Post-integration whole protocol crate 230/230 with the 29 cases executed.

**Review ruling:** initially Needs fixes on two Important documentation defects (a stale "until their node lands" clause and a kind-first claim contradicting `validate_any`'s dimension-first delegation); controller applied the reviewer's prescribed one-line corrections plus three same-file doc Minors (stale phrasing in `protocol_values.rs`' module doc and the guide summary, missing Focused Verification entry) as commit `734564bf` — re-review dispensed with as proportionate for verbatim one-line doc fixes; grep evidence shows the stale clauses gone and both suites green. Deferred Minors for final review: the either-variant matcher in one pinned-invalid test (exact variants pinned elsewhere); two stale dispatch styles coexisting in `MoveContainerStack::valid`; report-file inaccuracies about two case bytes.

**Rollback:** revert `b348b0e5`, `734564bf` and `1143189e` individually; the four families return to `cases: null` and the corpus index returns to 807.

**Architecture skill: no change.**

## Node 2.5 — 2026-09-23

**Status:** complete. Commits `c3c36945` (`feat(protocol): qualify chat command wire`) + controller integration `9b217547` (`chore(corpus): integrate chat command evidence`, incl. the zero-case fixture move) on top of `4cc5ed2c`. Group 2 (all five client packet nodes) is now closed: every one of the 23 client families carries executed corpus cases.

**Deliverable:** the unsequenced ChatCommand (Play C→S 12) on the common fallible surface as the first variable-length client payload: the gate routes through `mornlea_domain::CommandText::try_from_canonical` (the local `valid_command_text` copy retired for the command slot; `valid_bounded_text` stays for the speech slot until node 3.11), the decode reader mirrors `read_control_message` so a declared length the payload cannot complete reports `Truncated`, and the 1026-byte payload ceiling (`ChatCommandMaxWireBytes` mirrored) refuses with `FrameTooLarge` before any parse. 13 corpus cases through the real Go codec (valid pairs incl. the 1024-byte boundary admit, the encode-side 1025 rejection, the wire-ceiling `capacity` case, empty/untrimmed-NBSP/control/invalid-UTF-8/noncanonical-variant/truncated/trailing negatives), producer `runtime-oracle/protocol-client-chat`; 2 routes registered and pinned; group test `tests/protocol_client_chat.rs`; the single semantic `runtime_contract.rs` pin update (`InvalidString` → `Truncated` for the incomplete payload) matches ruling 2.

**Controller rulings frozen:** (1) domain `CommandText` route for the command slot; (2) incomplete-payload category `truncated` via the Rust reader boundary and the Go prefix resolver, with the empty-payload cut-0 divergence (Rust `Truncated` vs Go invalid-uvarint) documented at the assertion site and excluded from the corpus per the node-1.5 ruling; (3) the 1026-byte wire ceiling maps to `capacity` on both sides.

**Evidence:** group test 11/11; `runtime_contract` 134/134; Go producer 9/9; `TestCompanionMessage` ok; fmt/clippy/gofmt/vet clean; full oracle package `ok`; audit `ok`. Corpus integrity: exactly 1 family changed, 836→849 strict superset (+13/−0), `source_revision` preserved; controller spot-checked the 13-byte valid literal, the 1026-byte max-text payload and the noncanonical variant bytes. Post-integration whole protocol crate 242/242 with the 13 cases executed.

**Integration incident (expected):** filling ChatCommand leaves zero zero-case client families, so the two `zeroCasePoint` fixtures in `inventory_test.go` moved from `protocol.client.ChatCommand` to `protocol.server.ChatEvent` (zero-case until node 3.11) inside the integration commit; the full oracle package re-ran green.

**Review ruling:** Approved, 0 Crit/0 Imp/4 Minor. Deferred Minors: the gate allocates through the owning `CommandText` constructor (a borrowing predicate would be a cross-crate change); the plan-authorized six-line reader duplication with `read_control_message`; the `capacity` category for this family resting on the shared pre-existing mapper until integration (now integrated and executed); an unreachable `"short input"` resolver branch carried defensively.

**Rollback:** revert `c3c36945` and `9b217547` individually; the family returns to `cases: null` and the corpus index returns to 836 (the fixture move returns with them).

**Architecture skill: no change.**
