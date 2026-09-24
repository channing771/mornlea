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

## Node 3.1 — 2026-09-23

**Status:** complete. Commits `9afe565a` (`feat(protocol): qualify world delta packets`) + controller integration `5f9fbe41` (`chore(corpus): integrate world delta evidence`) on top of `75650d9c`. First server-to-client node of group 3.

**Deliverable:** the two world delta families (`BlockChanges` S/Play/1, `ForgetChunks` S/Play/2) on the common fallible surface as the first variable-count batch families the corpus suite executes: count bound (≤4096; ≥1 for ForgetChunks) fires before any record scan/reservation on decode and inside the gate before the content loop, verified against the Go decode arms' own order; empty BlockChanges stays the legal revision barrier; ForgetChunks preserves submitted wire order and rejects duplicates via a fallible reserved scratch (`Allocation` on failure, no publication); `encoded_len` = 28 + varint + 14×count / 4 + varint + 8×count with checked arithmetic. 22 corpus cases (14 BlockChanges + 8 ForgetChunks: valid pairs incl. the empty barrier, base-zero/revision-gap/y-above-world/wrong-chunk/unregistered-block-90/unsorted-index/count-above-max, forget zero-count/duplicate/dimension-two, encode twins, truncation/trailing) through the real Go codec, producer `runtime-oracle/protocol-world-delta`; 4 routes registered and pinned; group test `tests/protocol_world_delta.rs` (4096/4097 boundary pins, u64::MAX−1/MAX revision pins, order-preservation pin, variant-split pin); mechanical `runtime_contract.rs` updates; AGENTS.md sync.

**Controller rulings frozen:** (1) the collapsed constructor check split — unregistered block → `InvalidEnum`/`invalid-enum`, out-of-world Y → `InvalidRange`/`invalid-value` (the pre-existing collapse was a genuine category divergence the corpus `decode-y-above-world` case now pins closed); (2) the decode-side count bound precedes the exact-record-length rule on both sides; (3) 4096/4097 boundary admits/rejects are group-test pins, keeping the frozen corpus small.

**Controller arithmetic slips (recorded):** the brief's prose said "13/21/870" while its binding BlockChanges table enumerates 14 labels (22 cases, 849→871 registered verbatim), and called the ForgetChunks literal 25 bytes where the Go encoder publishes 21; the implementer followed the binding table and the encoder in both cases.

**Evidence:** group test 15/15; `runtime_contract` 134/134; Go producer 9/9; `TestBlockChanges|TestForgetChunks` ok; full oracle package `ok`; audit `ok`; fmt/clippy/gofmt/vet clean. Corpus integrity: exactly 2 families changed, 849→871 strict superset (+22/−0), `source_revision` preserved; controller spot-checked the 43-byte BlockChanges literal, the 29-byte empty barrier, the 21-byte ForgetChunks literal and the y-above-world category agreement. Post-integration whole protocol crate 258/258 with the 22 cases executed.

**Review ruling:** Approved, 0 Crit/0 Imp/4 Minor. Deferred Minors: the "first variable-count batch families in this crate" phrasing in three docs overstates (pre-existing uvarint-count families predate the fallible surface — scope to "the corpus suite executes"); the BlockChanges gate runs per-record field rules across the whole loop before the sortedness relation where Go interleaves per record (no corpus case combines violations; flagged for the later semantic-adapter node); a dead `let _ = needed;` and unused fixture fields; the template's double value gate in `encode`.

**Rollback:** revert `9afe565a` and the integration commit individually; the two families return to `cases: null` and the corpus index returns to 849.

**Architecture skill: no change.**


## Node 3.2 — 2026-09-24

**Status:** complete. Commits `dde28196` (`feat(protocol): qualify bounded chunk snapshots`) + controller integration `1962f4c2` (`chore(corpus): integrate chunk snapshot evidence`) on top of `9c68e61a`.

**Deliverable:** the compressed ChunkSnapshot family (S/Play/0) qualified: new `src/codec.rs` owns one `ProtocolCodec` with a reused zstd encoder+decoder context (`new()` typed failure, no per-call construction); `decode_snapshot`/`encode_snapshot_into` are the snapshot-only methods (node 4.1 adds dispatch); `encode_logical_checked` replaces the infallible paths (the sections-pop panic red is now a typed refusal); scratch via `try_reserve` (`Allocation`), 1 MiB/2 MiB ceilings, destination checked before the single copy. 13 corpus cases (fixture decode verbatim, mixed/all-single valid pairs with LOGICAL digests, five encode-side logical negatives, four decode-side compressed-layer negatives) through the real Go codec, producer `runtime-oracle/protocol-snapshot`.

**Controller rulings frozen:** (1) corpus digests are of the canonical LOGICAL payload, derived by each side decompressing its own output — no compressed-byte comparison anywhere (the klauspost read-back in the producer is this ruling's prescribed mechanism; it adds no go.mod edge); (2) logical negatives encode-side, compressed negatives decode-side byte edits — the producer never hand-compresses case inputs; (3) the truncated/integrity split — a cut inside the body is caught by the envelope declared-length check first (`truncated`); a byte flip in a length-complete frame fails zstd checksum verification and maps to the NEW single `ProtocolError::Integrity` → `integrity` (editable set extended to `src/error.rs` for exactly this variant, per a NEEDS_CONTEXT escalation the worker correctly raised); (4) `SectionData` palette-slot and direct-high-bits gates aligned `InvalidEnum` → `InvalidRange` (node 3.1 split precedent; producer classifies from the real Go messages).

**Orchestration note:** the implementer's first dispatch correctly stopped NEEDS_CONTEXT on the two genuine contract conflicts; both were ruled and the same agent resumed and completed.

**Evidence:** group test 9/9; `runtime_contract` 134/134; Go producer 9/9; codec read-only confirmations ok; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean; the exported 13 cases were run through the real Rust consumer against the merged manifest BEFORE commit (all matched). Corpus integrity: exactly 1 family changed, 871→884 (+13/−0), `source_revision` preserved, the committed Go fixture byte-identical as the decode-fixture input (sha d1397d23…). Post-integration whole protocol crate 268/268.

**Review ruling:** Approved, 0 Crit/0 Imp/5 Minor. Deferred Minors: the 1 MiB compressed bound rests on the structural logical-size argument rather than an explicit reservation clamp; no deterministic seam to pin the `try_reserve` failure mapping; the stale `// indirect` klauspost marker in tools go.mod; the corpus decode arm uses the one-shot `decode` path (pinned identical to the owned-context path); a render-asymmetry nit in the section renderer.

**Rollback:** revert `dde28196` and `1962f4c2` individually; the family returns to `cases: null` and the corpus index returns to 871.

**Architecture skill: no change.**


## Node 3.3 — 2026-09-24

**Status:** complete. Commits `5daf6445` (`feat(protocol): qualify player and outcome packets`) + controller integration `ac0a3b1a` on top of `712e7d10`.

**Deliverable:** the four owner-private families (`PlayerState` 3, `CommandRejected` 4, `PlaceBlockSucceeded` 20, `CombatHit` 25) on the common fallible surface; `reject_reason_to_wire`/`reject_reason_from_wire` exported as compile-time-exhaustive closed 15-row bidirectional matches aligned with domain `wire_id()` and Go `CommandRejectReasonID` (produced for node 4.2); PlayerState keeps the Go gate order (finite vecs → rotation → scalar ranges → mining union), never clips pitch/temperature, preserves `-0.0` bits, and pins the 93-byte field order. 32 corpus cases (14/6/4/8) through the real Go codec, producer `runtime-oracle/protocol-player-outcomes`; 8 routes registered and pinned (the large inventory diffs are gofmt realignment from longer family names); group test `tests/protocol_player_outcomes.rs` (15/15: both-direction reason matrix against domain `wire_id()`, full i8 temperature, mining-union edges incl. progress==required, mutation matrix, truncation/trailing).

**Controller arithmetic slip (recorded, fourth occurrence):** the brief's prose said 13/31/915 while its enumerated PlayerState table carries 14 labels — the binding table governed: 32 cases, 884→916.

**Evidence:** group 15/15; `runtime_contract` 134/134; Go producer 9/9 with 32/32 route observations; codec read-only confirmations ok; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean. Corpus integrity: exactly 4 families changed, 884→916 strict superset (+32/−0), `source_revision` preserved. Post-integration whole protocol crate 284/284.

**Review ruling:** Approved, 0 Crit/0 Imp/2 Minor. Deferred Minors: the interval predicate beside the closed reason match in `command_rejected.rs` (one-representation cleanup); the PlayerState corpus encode arm builds through the constructor while its siblings use public fields (comment contrast inconsistent).

**Rollback:** revert `5daf6445` and `ac0a3b1a` individually; the four families return to `cases: null` and the corpus index returns to 884.

**Architecture skill: no change.**


## Node 3.4 — 2026-09-24

**Status:** complete. Commits `0865bb4b` (`feat(protocol): qualify inventory publication packets`) + controller integration `c36dead0` on top of `e21b7e5f`.

**Deliverable:** the five inventory/container publication families (`InventoryState` 10, `CraftingState` 21, `FurnaceState` 13, `ChestState` 15, `ContainerClosed` 14) on the common fallible surface; `ContainerClosed` exported from `lib.rs` (the compile-error red); fixed arrays via `read_fixed` checked indexed reads; stacks through the shared domain `ItemStack` rule; 18 raw ref bytes preserved through one `wire_bytes` source; no invented timer/stack relation. 37 corpus cases (7/8/10/7/5) through the real Go codec, producer `runtime-oracle/protocol-inventory-publication`; 10 routes registered and pinned; group test 14/14 incl. residue/size pins and the no-invented-relation pin.

**Category rulings frozen (implementer-applied, reviewer-verified against Go sources):** unknown crafting size (0/4) and furnace slot-whitelist violations report `InvalidRange`/`invalid-value` (the Go messages are numeric-domain, not enum-tag); the latent boundary where an UNREGISTERED item in a furnace/crafting slot classifies differently (Rust `InvalidEnum` at decode vs Go `invalid-value`) is documented and deliberately unexercised by any case — a future node wanting such a case returns to the controller.

**Controller arithmetic slips (recorded, fifth/sixth):** prose said 36 cases (table carries 37, governed) and called the FurnaceState stride 39 bytes (true stride 36, verified against the Go encoder).

**Evidence:** group 14/14; `runtime_contract` 134/134; Go producer 8/8; `TestProtocolV12ChestStateGolden|TestFurnace` ok; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean; the 37 exported cases ran through the real Rust consumer against the merged manifest BEFORE commit (caught two consumer field-name bugs pre-review). Corpus integrity: exactly 5 families changed, 916→953 (+37/−0), `source_revision` preserved; merged family provenance rows carry the expected four-path union (the merge helper unions the two codec test-file sources). Post-integration whole protocol crate green.

**Review ruling:** Approved, 0 Crit/0 Imp/2 Minor. Deferred Minors: the idle-timer no-relation pin uses empty stacks (a content-stacks-at-zero-timers record would also pin the zero-timer class); report wording on provenance row paths.

**Rollback:** revert `0865bb4b` and `c36dead0` individually; the five families return to `cases: null` and the corpus index returns to 916.

**Architecture skill: no change.**


## Node 3.5 — 2026-09-24

**Status:** complete. Commits `975b44c5` (`feat(protocol): qualify remote player packets`) + controller integration `d7e7b4b7` on top of `bff08510`.

**Deliverable:** the three remote-player families (`RemotePlayerSpawn` 7, `RemotePlayerDespawn` 8, `RemotePlayerStates` 9) on the common fallible surface; pitch unrestricted (finite-only — the companion ±π/2 rule stays out); UUID strict order by unsigned byte comparison; States' count bound (1..=7) before the record-length rule and the fixed 296-byte wire ceiling before allocation (exact 296 admits, 297 → capacity); −0.0/finite f32 bit preservation. 20 corpus cases (Spawn 5, Despawn 4, States 11) through the real Go codec, producer `runtime-oracle/protocol-remote-players`; 6 routes; group test 14/14 incl. the latent-boundary section.

**Controller ruling frozen (implementer NEEDS_CONTEXT, correctly escalated):** Go's `RemotePlayerSpawn.Validate` folds identity/name/dimension/finiteness into ONE message, so the Spawn corpus freezes only agreed boundaries (valid pair, padded-name pair → invalid-value, nan-pitch at the f32 primitive → invalid-value); zero-uuid/wrong-version-uuid/dimension-two are a LATENT cross-implementation class — pinned as Rust group-test boundaries with the coarsening documented, never corpus cases (the 3.4-furnace discipline).

**Evidence:** group 14/14; `runtime_contract` 134/134; Go producer 8/8 (incl. `-race`); `TestProtocolV2RemotePlayerGolden` ok; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean. Corpus integrity: 3 families changed, 953→973 (+20/−0), `source_revision` preserved. Post-integration whole protocol crate 314/314.

**Review ruling:** Approved, 0 Crit/0 Imp/3 Minor (duplicated encode assertion; no dedicated truncation sweep on the 296-byte literal; the prefix-coarse `"remote player state "` classifier arm noted for future producers).

**Rollback:** revert `975b44c5` and `d7e7b4b7` individually; corpus returns to 953.

**Architecture skill: no change.**


## Node 3.6 — 2026-09-24

**Status:** complete. Commits `d9e00fc7` (`feat(protocol): qualify companion packets`) + controller integration `90a76cbe` on top of `8f109aff`.

**Deliverable:** the three companion families (`CompanionSpawn` 17, `CompanionStates` 18, `CompanionDespawn` 19) on the common fallible surface; overworld-only; pitch ∈ [−π/2,+π/2] inclusive pinned at exact bits (next float rejects); companion name rule (no embedded whitespace); count 1..=4 before record scan; strict UUID byte order. 19 corpus cases (6/9/4) through the real Go codec, producer `runtime-oracle/protocol-companions`; 6 routes; group test 16/16 incl. the latent-boundary section.

**Rulings frozen:** the node-3.5 folded-validator precedent applied proactively (no NEEDS_CONTEXT round trip) — the producer maps the folded spawn/state messages to invalid-value; zero/wrong-version-ID and dimension-1 are latent Rust-only pins. Two implementer-found category corrections (reviewer-verified against Go source): `CompanionStates` decode-trailing-byte is `truncated` (the Go batch's exact remaining-length rule, mirrored by `require_records`), and `CompanionDespawn` decode-trailing-byte is `capacity` (Go's pre-parse 16-byte fixed maximum; the Rust despawn decoder gained the same pre-read ceiling — the one Rust behavior change not red-driven, justified to keep both sides on one boundary). The plan's literal NONE red did not exist on the baseline (`CompanionId` was already the checked newtype); unconstructibility pinned instead.

**Evidence:** group 16/16; `runtime_contract` 134/134; Go producer 8/8; codec golden filters ok; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean; the 19 exported cases ran through the real Rust consumer pre-commit (all matched). Corpus integrity: 3 families changed, 973→992 (+19/−0), `source_revision` preserved. Post-integration whole protocol crate 331/331.

**Review ruling:** Approved, 0 Crit/0 Imp/3 Minor (group-numbering drift in the corpus doc-comment — one renumbering pass pending; the despawn `runtime_contract` trailing pin updated to `FrameTooLarge` per the ceiling ruling; the spawn name-prefix cut pins note).

**Rollback:** revert `d9e00fc7` and `90a76cbe` individually; corpus returns to 973.

**Architecture skill: no change.**


## Node 3.7 — 2026-09-24

**Status:** complete. Commits `d0c9e2c3` (`feat(protocol): qualify item-drop packets`) + controller integration `eb8a78e1` on top of `6fdc92c4`.

**Deliverable:** the two item-drop families (`ItemDropUpserts` 11, `ItemDropRemoves` 12) on the common fallible surface; raw i32 dimension never narrowed or validated (−1/256 round-trip verbatim, identity ordering compares the raw dimension first); the exact empty triple stays wire-valid while non-canonical empties refuse; count 1..=32 before record scan (minimum-records rule → trailing-byte `trailing` on both sides, implementer-verified); block index < 98304 no-clamp. 19 corpus cases (10/9) through the real Go codec, producer `runtime-oracle/protocol-drops`; 4 routes; group test 15/15.

**Rulings frozen (pre-ruled + implementer findings, reviewer-verified):** drop-ID slot/generation rejections map `invalid-identity` (domain-corpus precedent; `drop_id::read` remapped to `InvalidIdentity` — a baseline behavior change confined to the ID gate); the folded stack message keeps count/stack-limit violations in the corpus while unregistered item 66 stays the latent Rust-only pin (same class 3.4 recorded for inventory slots); the now-dead `drop_id::write`/`item_stack::write` helpers removed; the count case uses count 65 on the limit-64 stone (no registered item has a 16 limit — same boundary class, label/count unchanged).

**Evidence:** group 15/15; `runtime_contract` 134/134; Go producer 8/8; `TestProtocolV4DropGolden|TestItemDropDecodeRejectsOversizedCountBeforeAllocation` ok; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean; the 19 exported cases ran through the real Rust consumer pre-commit (all matched). Corpus integrity: 2 families changed, 992→1011 (+19/−0), `source_revision` preserved. Post-integration whole protocol crate 347/347.

**Review ruling:** Approved, 0 Crit/0 Imp/2 Minor (the mutation matrix asserts validate/encoded_len only transitively through encode; the ten-closure shared round-trip helper is a readability note).

**Rollback:** revert `d0c9e2c3` and `eb8a78e1` individually; corpus returns to 992.

**Architecture skill: no change.**


## Node 3.8 — 2026-09-24

**Status:** complete. Commits `cc852015` (`feat(protocol): qualify hostile packets`) + controller integration `d8368493` on top of `a9b77b53`.

**Deliverable:** the three hostile families (`HostileSpawn` 22, `HostileState` 23, `HostileDespawn` 24) on the common fallible surface; new shim `src/hostile_id.rs` re-exports the domain `HostileId` with the 8-byte wire edge (zero unconstructible outbound; `HOSTILE_ID_WIRE_BYTES` exported for the passive/projectile successors); strides 30/38/8 with state omitting dimension and spawn omitting velocity; count 1..=64 before record scan; strict numeric ID order; the exact remaining-length rule → trailing-byte `truncated` (verified in `hostile_wire.go`). 30 corpus cases (12/10/8), producer `runtime-oracle/protocol-hostiles`; 6 routes; group test 19/19 incl. 64-record admits on all three families.

**Rulings frozen:** distinct-message category mapping (zero ID invalid-identity, dim invalid-enum, health invalid-value, kind invalid-enum, nonfinite/count/sort invalid-value) — no latent classes in this group; the `runtime_contract.rs` zero-ID constructor cases moved to decode-path assertions (the checked newtype makes the mutation inexpressible — reviewer judged the coverage equivalent); the real codec-package hostile filters are `TestHostileMessagesWireLayoutIsFrozen|TestHostileMessagesDecodeRejectsInvalidWire|TestHostileMessagesWireLimitsAreFrozen|TestHostileMessageCodecRoundTripsProperty` + `FuzzHostileMessageCodec`.

**Evidence:** group 19/19; `runtime_contract` 134/134; Go producer 8/8 (+`-race`); codec filters ok; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean. Corpus integrity: 3 families changed, 1011→1041 (+30/−0), `source_revision` preserved. Post-integration whole protocol crate 367/367.

**Review ruling:** Approved, 0 Crit/0 Imp/2 Minor (the 9-byte batch-header stride privately redefined per family — consolidation candidate; a redundant doc sentence).

**Rollback:** revert `cc852015` and `d8368493` individually; corpus returns to 1011.

**Architecture skill: no change.**


## Node 3.9 — 2026-09-24

**Status:** complete. Commits `cb53f365` (`feat(protocol): qualify passive packets`) + review-fix `f26f7690` (`docs(protocol): record batch wire-ceiling latent class` — the reviewer's one Important finding, applied by the controller verbatim) + controller integration `4f103497` (`chore(corpus): integrate passive evidence`, incl. the dispatch-arm fix) on top of `a40e57db`.

**Deliverable:** the three passive families (`PassiveSpawn` 26, `PassiveState` 27, `PassiveDespawn` 28) on the common fallible surface; `src/passive_id.rs` shim (domain `PassiveId` + 8-byte wire edge, zero unconstructible outbound); strides 29/38/9 (spawn omits kind, state omits dimension); count 1..=64 with the 64-vs-32 pin (the 32-actor live cap named authority-only, kept out of the packet layer); exact-remaining-length → trailing `truncated` (verified in `passive_wire.go`); grazing/reason closed 0/1 matches. 30 corpus cases (11/10/9), producer `runtime-oracle/protocol-passives`; 6 routes; group test 19/19.

**Rulings frozen:** distinct-message category mapping (no latent classes on exercised boundaries); the OVER-CEILING latent class — Go's fixed per-family wire ceiling answers `capacity` while the Rust hostile/passive decoders answer `Truncated` — recorded in BOTH guides by the review fix (the projectiles node must rule before freezing any such case); hostile-prefixed corpus JSON readers renamed to neutral `record_*` (purely mechanical, ~10 hostile call-sites); `PASSIVE_*_MAX_WIRE_BYTES` derived from strides (mirror the Go declarations without gating Rust decode); the real codec passive filters are `TestPassiveMessagesWireLayoutIsFrozen|TestPassiveMessagesDecodeRejectsInvalidWire|TestPassiveMessagesWireLimitsAreFrozen|TestPassiveMessageCodecRoundTripsProperty`.

**Controller integration incident + fix (the node-1.5 pattern):** the generic `dispatch_case` lacked the three passive family constants (the passive group test used its own dispatch) — fixed inside the integration commit; producer-paragraph ordering in the oracle guide (passive inserted before hostile) also corrected in the review-fix commit.

**Evidence:** group 19/19; `runtime_contract` 134/134; Go producer 8/8; codec passive filters ok; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean; the 30 exported cases ran through the real Rust consumer pre-commit. Corpus integrity: 3 families changed, 1041→1071 (+30/−0), `source_revision` preserved. Post-integration whole protocol crate 387/387 after the dispatch fix.

**Review ruling:** Approved, 0 Crit/1 Imp(docs — fixed by `f26f7690`)/3 Minor (the 9-byte header stride consolidation; `*_MAX_WIRE_BYTES` retention inconsistency vs hostile; the state cap sourced from the sibling module).

**Rollback:** revert `cb53f365`, `f26f7690` and `4f103497` individually; corpus returns to 1041.

**Architecture skill: no change.**


## Node 3.10 — 2026-09-24

**Status:** complete. Commits `cbbd1de9` (`feat(protocol): qualify projectile packets`) + controller integration `1b65ad28` on top of `fe92253b`.

**Deliverable:** the three projectile families (`ProjectileSpawn` 29, `ProjectileState` 30, `ProjectileDespawn` 31) on the common fallible surface; `src/projectile_id.rs` shim (domain `ProjectileId` + 8-byte wire edge); strides 37/20/8 (kind before dimension on spawn; state carries ID+position only — the narrowest record); kind×dimension independence (all four combos admit, the server's narrower rule stays off this wire); count 1..=128 before record scan; exact remaining-length → trailing `truncated`; AND the over-ceiling latent class RESOLVED — pre-parse `FrameTooLarge` ceilings (4745/2569/1033, derived 9+128×stride) on all three Rust decoders, one `ProjectileDespawn/decode-over-ceiling` → capacity corpus case frozen on the real Go message, the crate-guide note updated (projectiles resolved; hostile/passive keep the recorded divergence). 29 corpus cases (11/9/9), producer `runtime-oracle/protocol-projectiles`; 6 routes; group test 20/20.

**Rulings frozen:** the category mapping per the hostile/passive template; the over-ceiling resolution ruling (the node-3.6 despawn pre-parse precedent generalized to all three families); the dispatch-arm constants present this time (no 3.9-style integration incident).

**Evidence:** group 20/20; `runtime_contract` 134/134; Go producer 13/13 (+`-race`, 320s); codec projectile filters ok; full oracle ok; audit ok (one Go comment reworded to avoid backticking the Rust name `FrameTooLarge` — the audit identifier gate); fmt/clippy/gofmt/vet clean. Corpus integrity: 3 families changed, 1071→1100 (+29/−0), `source_revision` preserved. Post-integration whole protocol crate 408/408.

**Review ruling:** Approved, 0 Crit/0 Imp/2 Minor (report-wording on where the baseline bytes are recorded; the unused `PROJECTILE_ID_WIRE_BYTES` re-export).

**Rollback:** revert `cbbd1de9` and `1b65ad28` individually; corpus returns to 1071.

**Architecture skill: no change.**


## Node 3.11 — 2026-09-24

**Status:** complete. Commits `38de05b5` (`feat(protocol): qualify chat event union`) + producer-splice fix `758b21a3` (`fix(runtime-oracle): splice chat event name mutation correctly`) + controller integration `c172e6a9` (`chore(corpus): integrate chat event evidence`, incl. the zeroCasePoint move and the producer-count doc fix) on top of `d5e48c60`. GROUP 3 CLOSED: all 60 protocol families now carry executed corpus cases.

**Deliverable:** the ChatEvent family (S/Play/16) on the common fallible surface; the closed `(kind,reason)` match chooses exactly one text slot and identity shape; the bidirectional `TryFrom` pair to the domain `ChatBody` union exported for node 4.2 (raw zero companion UUID → semantic absence only in InvalidFormat/UnknownCompanion); the Go Validate precedence mirrored (global gates → kind dispatch → speech-slot exclusivity → per-branch rules); the 1328-byte per-payload ceiling verified decode-reachable and mirrored pre-parse (exactly-1328 admits). 24 corpus cases (14 valid branches incl. encode pairs + 10 negatives), producer `runtime-oracle/protocol-chat-event`; 2 routes; group test 13/13 incl. the 16-branch × both-directions domain matrix.

**Rulings frozen:** the slot-exclusivity negatives are not wire-constructible (one kind-chosen slot) — the two cases record the branch's own requirement at that slot (both invalid-value, genuine single-violation Go rejections) and the DTO-level exclusivity is group-test-pinned (reviewer-verified honest); the folded player-identity gate is classified per-case from real bytes; the producer-count doc error (19→18) fixed at integration.

**Integration incidents + fixes:** (1) the `decode-noncanonical-player-name` export spliced the name mutation wrong (inserted over the old name, shifting all later fields — both sides rejected at different boundaries); the resumed implementer fixed the splice to a proper replace-and-shift, verified both sides publish invalid-value on the corrected bytes, and re-exported; the controller re-integrated the one case's bytes and manifest. (2) With ChatEvent filled, NO protocol family remains zero-case — the `zeroCasePoint` fixtures moved to `kernel.mornlea_collision_resolve`/11 inside the integration commit.

**Evidence:** group 13/13; `runtime_contract` 134/134; Go producer 8/8 (and 9/9 with the fixed splice); companion-message golden/invalid filters ok; domain `event_chat` 10/10 unchanged; full oracle ok; audit ok; fmt/clippy/gofmt/vet clean. Corpus integrity: 1 family changed, 1100→1124 (+24/−0), `source_revision` preserved. Post-integration whole protocol crate 422/422.

**Review ruling:** Approved, 0 Crit/1 Imp(docs — fixed at integration)/5 Minor (case-ID naming for the substituted pair; the dead `chatEventDerivedFrom` truncated arm; a node-reference in a comment; a wording ambiguity; report-wording).

**Rollback:** revert `38de05b5`, `758b21a3` and `c172e6a9` individually; corpus returns to 1100 and the fixtures move back.

**Architecture skill: no change.**


## Node 4.1 — 2026-09-24

**Status:** complete. Commit `46b71203` (`feat(protocol): close typed packet registry`) on top of `d3adc8d9`. NO corpus change — this is the typed-dispatch closure.

**Deliverable:** `src/registry.rs` with the design §1 API — `Direction`/`State`/`PacketKey`, the 23-variant `ClientPacket` (raw `InboundHello`/`InboundLoginStart` for the two negotiation variants) and 36-variant `ServerPacket`, exhaustive `(state,id)` dispatch in `decode_client` and `ProtocolCodec::decode_server`/`encode_server_into`, `key()` on both enums — verified line by line against Go `registry.go` (all 59 keys exact, C/Play/1 reserved, C/Play/22 and S/Play/32 unassigned). Group test `tests/protocol_registry.rs` 70/70: 59 named per-key dispatch cases over embedded Go-produced literals, direction/state mutation invariants over the REQUESTED key, the cross-legitimate C/Handshake/0↔S/Handshake/0 collision pinned and documented, the three table-tamper detections by key/variant, and the old-version hello decoding structurally then answering `VersionMismatch{45}` through `validate_hello`.

**Rulings frozen (reviewer-verified):** (A) the raw inbound records gained the standard fallible encode surface so `encode_client_into` stays exhaustive — no admission rule leaks, gates total; (B) the direction-mutation invariant is asserted over the requested key with `UnknownPacket` only for unregistered keys; (C) the encode-side 64 KiB ceiling is genuinely unreachable (largest non-snapshot payload ≈57.4 KiB BlockChanges) and the decode-side ceiling matches Go's pre-switch placement with the snapshot routed away; (D) an unknown state byte is unrepresentable in the closed `State` enum, pinned Go-side. The registry route table in `ORACLE/inventory.go` was already complete (60 families integrated at 3.11) — no controller merge was needed.

**Evidence:** group 70/70; whole crate ALL GREEN with no staged red (`protocol_corpus` 134, `runtime_contract` 134); fmt/clippy clean; audit ok; Go registry ID tests pass. **Review ruling:** Approved, 0 Crit/0 Imp/3 Minor (a `mutated_keys` doc undercount; one wrong supporting number in the report; a future-proofing catch-all in `ClientPacket::key()`'s state match).

**Rollback:** revert `46b71203` as one unit; the concrete codecs are untouched.

**Architecture skill: no change.**


## Node 4.2 — 2026-09-24

**Status:** complete. Commit `9706c644` (`feat(protocol): add exhaustive semantic adapters`) on top of `7574ec63`. NO corpus change.

**Deliverable:** `src/semantic.rs` — `PlayIntent` and the four `TryFrom` pairs over the 4.1 typed enums and the checked domain Command/Event values: 19 sequenced client commands (the `StackView` split through the 1.3 neutral gate), Chat/KeepAliveReply only, Handshake/Login refused; 30 publications mapped to the same-named Event variants (ChatEvent→Event::Chat), the six control variants refused; EVERY wire cap (4/7/32/64/128/4096×2) checked by one shared `check_record_cap` as the first statement of all 15 batch arms before any copy; no session/tick/arrival/recipient invention; `CommandEnvelope` never constructed (documented + needle-scanned). Group test `tests/protocol_semantic.rs` 19/19 incl. the independent 23/36-row key tables (double-swap-proof), the 5-record companion rejection, all cap boundaries, the dimension no-alias set, the chat absence mapping, and the six-field mutation tests.

**Rulings frozen (reviewer-verified):** (A) the brief's claim that TakeCraftingOutput's wire packet accepts sequence 0 was WRONG — both the packet gate and the domain envelope refuse it; the adapter carries the value verbatim and both gates are pinned; (B) the CommandEnvelope pin by needle-scan + docs is adequate (clippy closes the escape forms); (C) every inbound arm re-runs the record's own gate, keeping the view-reference regime and caps honest for hand-assembled variants.

**Evidence:** group 19/19; whole crate green at 530 tests / 24 binaries, NO staged red; fmt/clippy clean; domain `event_surface` 4/4; audit ok; corpus tree unchanged. **Review ruling:** Approved, 0 Crit/0 Imp/4 Minor (an unreachable `InvalidContainerGeneration`→`InvalidIdentity` mapping arm; plan-node numbers in four doc comments; one needle-list completion; an outbound `.expect` vs the inbound checked precedent).

**Rollback:** revert `9706c644` as one unit.

**Architecture skill: no change.**

## Node 4.3 — 2026-09-24

**Status:** complete. One scoped commit `test(protocol): close executable v45 corpus coverage` on top of `c747ee21`. NO corpus change — this node verifies and closes rather than adds; the tracked `testdata/runtime-migration` tree is byte-identical (`git diff --exit-code -- testdata/runtime-migration` clean before and after the commit).

**Deliverable:** the protocol-only zero-gap evidence. Go `packages/tools/cmd/runtime-oracle/protocol_coverage_test.go` (new): `TestProtocolCorpusComplete` proves the closed state from three independent sides — live registry discovery, the tracked manifest and `BaselineConsumerRegistry` — all naming the same 60 protocol families (59 packet + `protocol.frame`), each with a decode and an encode route at protocol 45, each family carrying >=1 valid decode, >=1 valid encode and >=1 invalid/boundary case, every case carrying exactly one zero checkpoint, and every case's packet-key direction agreeing with its `protocol.client.`/`protocol.server.` prefix. `ReconcileWorking` under the closed union reports ZERO uncovered `protocol.*` points and all 60 covered; `ReconcileComplete` still refuses with only the 45 non-protocol uncovered points, so complete F1 stays unclaimed. Rust `crates/mornlea_protocol/tests/protocol_corpus.rs` adds `protocol_corpus_protocol_family_set_is_closed` (executed family set equals the pinned 60-identity list; per-family minimums; 433 packet + 3 frame cases counted from the loaded selections) and `protocol_corpus_every_group_mutation_fails_the_comparison` (18-group table: one reviewed case per producer group executes through the real dispatch, the unmutated comparison passes outside `catch_unwind`, then exactly one drifted expectation value — a bumped number or an extended text — makes the same Rust comparison panic).

**Per-family case counts (60 families, 436 cases, one checkpoint each):** `protocol.client.BoneMeal` 5; `protocol.client.ChatCommand` 13; `protocol.client.ClientHello` 4; `protocol.client.CloseContainer` 4; `protocol.client.CollectWater` 5; `protocol.client.DropSelectedItem` 4; `protocol.client.DropStack` 5; `protocol.client.EquipArmor` 4; `protocol.client.KeepAliveReply` 4; `protocol.client.LoginStart` 6; `protocol.client.MoveContainerStack` 10; `protocol.client.MoveCraftingStack` 6; `protocol.client.MoveInventoryStack` 6; `protocol.client.MoveStackPartial` 9; `protocol.client.OpenContainer` 5; `protocol.client.PlaceBlock` 5; `protocol.client.PlaceWater` 5; `protocol.client.PlayerInput` 6; `protocol.client.QuickMoveStack` 5; `protocol.client.RequestChunkResync` 4; `protocol.client.SelectHotbar` 4; `protocol.client.TakeCraftingOutput` 4; `protocol.client.TillSoil` 5; `protocol.frame` 3; `protocol.server.BlockChanges` 14; `protocol.server.ChatEvent` 24; `protocol.server.ChestState` 7; `protocol.server.ChunkSnapshot` 13; `protocol.server.CombatHit` 8; `protocol.server.CommandRejected` 6; `protocol.server.CompanionDespawn` 4; `protocol.server.CompanionSpawn` 6; `protocol.server.CompanionStates` 9; `protocol.server.ContainerClosed` 5; `protocol.server.CraftingState` 8; `protocol.server.Disconnect` 8; `protocol.server.ForgetChunks` 8; `protocol.server.FurnaceState` 10; `protocol.server.HandshakeReject` 5; `protocol.server.HostileDespawn` 8; `protocol.server.HostileSpawn` 12; `protocol.server.HostileState` 10; `protocol.server.InventoryState` 7; `protocol.server.ItemDropRemoves` 9; `protocol.server.ItemDropUpserts` 10; `protocol.server.KeepAlive` 3; `protocol.server.LoginReject` 7; `protocol.server.LoginSuccess` 4; `protocol.server.PassiveDespawn` 9; `protocol.server.PassiveSpawn` 11; `protocol.server.PassiveState` 10; `protocol.server.PlaceBlockSucceeded` 4; `protocol.server.PlayerState` 14; `protocol.server.ProjectileDespawn` 9; `protocol.server.ProjectileSpawn` 11; `protocol.server.ProjectileState` 9; `protocol.server.RemotePlayerDespawn` 4; `protocol.server.RemotePlayerSpawn` 5; `protocol.server.RemotePlayerStates` 11; `protocol.server.ServerHello` 3.

**Mutation evidence (each FAILS):** manifest level — zero-case family (Working lists the point uncovered, Complete refuses), duplicate case ID, missing route (`unsupported route protocol.server.KeepAlive/45/migrate`), missing source hash (`invalid sha256`), mismatched expected digest (`does not match disk`), unsupported case version (`supported_versions`), unexecuted case (`has no registered Go producer`). Execution level — per producer group (all 18): the unmutated case reproduces the frozen expectation first, then an altered key, a wrong direction and a wrong state are each refused at the key boundary (`names key` / `names handshake key` / `names login key` / `names unknown state`). Domain/agent evidence unchanged: exact totals pinned at 534 domain, 154 agent and 436 protocol cases, with the complete manifest re-reconciled.

**Exported candidate:** `RUNTIME_ORACLE_EXPORT_DIR=/private/tmp/mornlea-4.3-export` publishes `runtime-oracle/domain-event-manifest/contracts.json` — the complete manifest with `source_revision` refreshed to the closure commit's HEAD revision at export time — through the reviewed create-exclusive exporter. The tracked manifest and `BaselineSourceRevision` (`736af2f4...`) stay on their recorded values; the controller verifies at integration that the exported revision equals `git rev-parse --short HEAD` and records the integrated SHA in the close entry, updating both sides in the same integration commit. The worker did not edit `inventory.go` (the closed route union was verified complete as integrated at the typed-registry node) or the tracked corpus.

**Non-protocol uncovered list (45 points, unchanged):** `domain.input/45`; `kernel.mornlea_collision_resolve/11`, `kernel.mornlea_fluid_eval_batch/11`, `kernel.mornlea_fluid_rescan/11`, `kernel.mornlea_lod_shell/11`, `kernel.mornlea_mesh_section/11`, `kernel.mornlea_physics_step/11`, `kernel.mornlea_raycast_batch/11`, `kernel.mornlea_tree_blocks/11`, `kernel.mornlea_worldgen_chunk/11`, `kernel.mornlea_worldgen_probe/11`, `kernel.pathfind/1`; `save.chunk/1..9`, `save.companion/1..5`, `save.hostile/1..2`, `save.passive/1`, `save.player/1..9`, `save.region/1`, `save.world-metadata/1..6`.

**Evidence:** Go `^TestProtocolCorpus` family green (closure, mutations, 18-group key gate with three mutations each, non-protocol totals, group table); full oracle package `ok` (56.4s) and `-race` `ok` (354.5s); `go test ./packages/shared/network/... -race -count=1` green (4 packages); `go test ./packages/audit -count=1` `ok` (28.0s); `gofmt`/`go vet` clean. Rust: whole `mornlea_protocol` crate 513 tests across 25 binaries green with no staged red; `cargo fmt --check` clean; `clippy --all-targets -D warnings` clean.

**Rollback:** revert the single commit as one unit; the tracked corpus, the route union and `BaselineSourceRevision` are untouched, so a revert restores the exact pre-node state.

**Architecture skill: no change** (the closure-evidence pattern — three-side family-set equality plus a per-group value-mutation proof — is change-local; the durable route-runner and producer/consumer separation rules are already recorded in the synchronized skill).


## Node 4.3 — 2026-09-24

**Status:** complete. Commits `b6043f00` (`test(protocol): close executable v45 corpus coverage`) + review-fix `accdb5d7` (`test(runtime-oracle): pin altered-source provenance rejection`) + controller integration `29ec84e0` (`chore(corpus): refresh protocol closure source revision`) on top of `c747ee21`.

**Deliverable:** the protocol-only zero-gap closure. `TestProtocolCorpusComplete` proves the 60-family set three-way (live Go discovery = frozen manifest = closed route union), counts per-family minimums from the actual manifest (436 protocol cases: 433 packet + 3 frame ≥ the 180 minimum), asserts ZERO uncovered `protocol.*` in `ReconcileWorking` while `ReconcileComplete` still refuses the 45 non-protocol points (F1 unclaimed), and verifies every failure-mode mutation class (zero-case, duplicate, missing route/hash, altered key, wrong direction/state/version, unexecuted, mismatched digest) plus one REAL-Rust-consumer value-mutation comparison failure per each of the 18 producer groups. The review fix added the named disposable-copy Go source-byte provenance rejection (`TestProtocolCorpusCompleteRejectsAlteredGoSourceByte` — the production source-hash-content branch now covered; unmutated-then-mutated discipline).

**Source revision refresh (the plan's single act):** `source_revision` and `BaselineSourceRevision` refreshed ONCE together to `b6043f004176055a2e39a98508b662691c3e4ef7` (the closure-evidence commit that contains every Go producer; the fix commit `accdb5d7` intentionally follows it — the revision names the evidence baseline, not HEAD). Both sides changed in the same integration commit; the node's own gate pins their equality.

**Evidence:** 7 new Go tests + 2 new Rust tests; the `^TestProtocolCorpus` family 21/21; full oracle ok (implementer also ran `-race`, 351s); whole protocol crate 513/513; network subtree `-race` ok; audit ok; fmt/clippy/gofmt/vet clean; tracked corpus unchanged until the controller's two-field integration commit. **Non-protocol uncovered (45, exactly as visible):** `domain.input/45`; 11 kernel points; `save.chunk/1..9`, `save.companion/1..5`, `save.hostile/1..2`, `save.passive/1`, `save.player/1..9`, `save.region/1`, `save.world-metadata/1..6`.

**Review ruling:** initially Needs fixes on the one missing provenance check (Important) — fixed by `accdb5d7` and verified; 2 Minor (the producer-path reuse comment — added in the fix; coarse ledger evidence counts — recorded here).

**Rollback:** revert `accdb5d7`, `b6043f00` and `29ec84e0` individually; the corpus keeps all 60 families and the revision returns to `736af2f4`.

**Architecture skill: no change.**


## Node 4.4 — 2026-09-24

**Status:** complete. All stage gates green on HEAD `a3018819`:

- `rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check` — exit 0.
- `openspec validate --all --strict --no-interactive` — 128 passed, 0 failed.
- `go test ./packages/audit -count=1` — ok (24.7s).
- `make rust-check` — exit 0 (54 Rust test binaries all ok, incl. every protocol/domain/storage suite).
- `make dev-check` — exit 0.
- `make test-race` — exit 0, 0 FAIL, every workspace module ok (incl. the full `-race` oracle at the refreshed `b6043f00` source revision).
- `git diff --exit-code -- testdata/runtime-migration` — clean (the frozen corpus is byte-identical post-closure).

**Standing notes:** the whole-change review (the SDD final whole-branch review with the accumulated Minor-findings triage) remains the one open item outside the checkbox scope — the branch `codex/rust-protocol-completion` from `0f5ff747` to HEAD carries 45 scoped commits across nodes 1.1–4.3; the change ledger records every node's evidence, rulings, and rollback. No failed required gate was accepted.

**Architecture skill: no change** (final promotion review deferred to the whole-change review).

**Node 4.4 commit:** this ledger entry plus the `tasks.md` checkbox.

## Final whole-branch review and remediation plan — 2026-09-24

**Status:** the historical 4.4 stage run remains recorded, but the independent final review did not accept the branch for merge. Nodes 5.1–5.6 are open and hold the repair and integrated acceptance. Reviewed source range: `0f5ff747`..`f311d1fb`; current main at review start: `0a75dec6`. The dirty original checkout has an unrelated modified root `AGENTS.md` and three untracked `.cursor/` files, all preserved. The repair uses the isolated `codex/rust-protocol-final-review` worktree.

**Independent review findings:** the Rust implementation reviewer found two Important public-boundary defects: mutable strict outbound `ClientHello`/`LoginStart` can bypass validation or panic, and public snapshot envelope/compression helpers can exceed the advertised resource ceilings. The corpus reviewer found two Important evidence defects: `FrozenCase` drops `packet_key`, so Rust concrete dispatch ignores key mutations, and no Go decoder consumes Rust-produced compressed snapshot output. The allocation in `BlockChanges::valid` is a Minor but on a public encode validation path, so it is included in node 5.4. Both reviewers returned not-ready verdicts; no Critical was reported. The baseline's 59-key registry/admission split and Go producer path were independently inspected. The protocol reviewer ran 95 focused tests successfully, which does not discharge the findings.

**Integration findings:** three-way merge has no textual conflict but silently selects the branch's stale `mornlea_engine/src/step.rs` source hash in `contracts.json`; main's newer byte hash must be retained for `ReconcileWorking`. The branch also contains an active planning copy of `rust-region-format-completion` from a concurrent commit, while main has already implemented, synchronized and archived that change. Node 5.6 preserves the archive and removes only the stale active copy. The protocol `source_revision`/`BaselineSourceRevision` still bind the Go producer baseline `b6043f00` and do not change because of the later physics source edit.

**Controller design and readiness:** Superpowers brainstorming and writing-plans plus the Mornlea orchestration checklist were applied to the accepted protocol delta. The review findings refine its existing explicit bounds, key identity, safe encoding and mutual-decode requirements; they do not add a packet, live authority or version. The new nodes have exact file ownership, producer/consumer APIs, red cases, gates, rollback and serial integration. Node 5.1 may use an isolated Rust-corpus worker; node 5.3 may use an isolated Go-decoder evidence worker; the controller owns public API repairs, shared guides, corpus provenance, spec sync, archive and final merge. The workers receive only their node briefs and cannot change another node's files. This isolation avoids two editing agents sharing one worktree or index; neither worker uses GPT-6 Astra. No architecture-skill promotion is justified yet: **Architecture skill: no change** pending final verified review.

**Planning/baseline evidence:** `openspec validate rust-protocol-completion --strict --no-interactive` passed after the task/design/packet update; `git diff --check` on planning files passed. In the isolated checkout, `make rust` exited 0, Rust `protocol_corpus` ran 23/23 tests, and Go `TestProtocolCorpusComplete` passed. These establish a green baseline for the review repairs, not completion acceptance.

**Node 5.1 interface ruling:** the worker found four additional test-only `FrozenCase` struct literals in the domain corpus. The controller retains the explicit optional packet-key field and owns the four `packet_key: None` updates at integration; the worker remains restricted to the loader and protocol corpus files. The task packet now names the derived consumers and adds the domain corpus gate. No production ownership or corpus asset changes result.

## Node 5.2 — 2026-09-24

**Status:** complete in the isolated final-review branch. Public snapshot envelopes now keep their length and compressed bytes private, expose read-only getters, and recheck both wire ceilings before decompression. The public one-shot compressor checks the logical and worst-case compressed bounds before constructing a zstd context; the reusable codec checks the same bound before reserving scratch. Tests construct forged envelopes inside the module and oversized one-shot requests, observing `FrameTooLarge` instead of integrity errors or successful compression. Test-only expansion bombs now use zstd directly to keep the decoder attack case independent of the bounded public encoder.

**Red → green:** the two new unit tests initially failed (one forged envelope returned `Integrity`, and one oversize request succeeded), then passed after the boundary changes. Focused Rust unit tests 2/2, `protocol_snapshot` 9/9, `runtime_contract` 134/134; Go `^TestChunkSnapshot` passed; Rust Clippy with `-D warnings`, `cargo fmt`, and `git diff --check` passed. The scoped crate guide records the public API boundary. Rollback is this one node commit; no corpus bytes, protocol ID, or version changed. **Architecture skill: no change**; the bound-before-allocation rule is already in the synchronized skill.

## Node 5.1 — 2026-09-24

**Status:** complete. The isolated worker's `8b882f2c` was cherry-picked as `7a0fa505` and integrated with four domain-only synthetic `FrozenCase` literals. The loader now retains an exact optional packet key and requires one on packet cases but none on frames. Successful frozen decode cases execute the public typed dispatcher, establishing keys for all 59 packet families; all 433 packet comparisons check their frozen key against the family before the concrete outcome. A mutation test leaves payload and expected fields untouched while changing direction, state, or ID, and requires each comparison to fail.

**Red → green:** the worker observed the direction mutation pass unexpectedly before the fix, then all three mutations failed as required. On the integrated tree, Rust `protocol_corpus` 24/24, domain `corpus_loader` 10/10, and domain `corpus_domain` 54/54 passed. The worker's full protocol suite, targeted Clippy, formatting and unchanged-corpus checks also passed. The four domain literals are `packet_key: None` because their families are synthetic domain probes. Rollback reverses this node's integration commit and `7a0fa505`; no frozen asset or source revision changed. **Architecture skill: no change**; typed route identity was already a cross-task rule.

## Node 5.3 — 2026-09-24

**Status:** complete. The isolated worker's `dbdf603a` was cherry-picked as `4908e9b6`; the controller pinned the fixture against the current Rust `ProtocolCodec` in `tests/protocol_snapshot.rs`. The committed 456-byte Rust fixture has SHA-256 `aab7b542775cc9b5351907f6c89da1881f5e9a0949356224afe50716269e34d6`. The worker's independent temporary probe showed the production Go `DecodeServer` accepts it and yields the same complete `ChunkSnapshot` as Go's fixture, despite different compressed bytes. The Go test also flips a checksum byte; the worker demonstrated that removing the flip made this negative assertion fail.

**Validation:** on the integrated tree, Rust `protocol_snapshot` 10/10 and Go `^TestRustSnapshot` passed; the worker's full Go runtime-oracle package and `make rust` passed. The Rust and Go scoped guides record fixture ownership and both directions of evidence. No runtime-migration corpus asset or source revision changed. Rollback reverts this pin commit and `4908e9b6`; the fixture is test-only. **Architecture skill: no change**; the existing logical parity rule already covers this evidence.

## Node 5.4 — 2026-09-24

**Status:** complete. `ClientHello` and `LoginStart` now share the crate's fallible `validate` → `encoded_len` → `encode_into` → `encode` pattern. The strict outbound records recheck mutable public fields on every publication. Invalid name precedes distance and destination capacity; invalid version precedes capacity. A valid short destination remains byte-for-byte unchanged. Raw inbound negotiation records and their admission policy are untouched. `BlockChanges::valid` checks every field before comparing adjacent indices, preserving error precedence without allocating an index vector. All direct strict-encoder callers were updated; valid wire literals and frozen corpus assets remain unchanged.

**Red → green:** the new admission tests first failed to compile because the required public fallible methods did not exist, then passed after implementation. The integrated focused command passed `protocol_admission` 18/18, `protocol_corpus` 24/24, `protocol_semantic` 19/19, `runtime_contract` 134/134 and `protocol_world_delta` including the new precedence case; Clippy `-D warnings`, formatting and `git diff --check` passed. Revert this one API node commit for rollback. **Architecture skill: no change**; revalidation at public encode boundaries already appears in the synchronized skill.

## Node 5.6 integration — 2026-09-24

**Current main:** fetched `origin/main` equals local `main` at `0a75dec671e9f41e048637642014d7256f17dff1`; the isolated review branch merged it at `42b85a8c`. The merge's actual manifest result retained main's SHA-256 `188cb73517c29e471afc2733a0c49bced834087ed04871ca408dabf5b7446aac` for `mornlea_engine/src/step.rs`, matching the merged file bytes. The protocol manifest `source_revision` and Go `BaselineSourceRevision` both remain `b6043f004176055a2e39a98508b662691c3e4ef7`, the frozen Go producer baseline. Focused Go `TestContractInventoryReconcilesFrozenCorpus`, `TestProtocolCorpusComplete`, and `TestRustSnapshotDecodesInGo` passed on the merged tree.

Main had completed all region tasks, synchronized their requirements into `openspec/specs/rust-runtime-foundation/spec.md`, and archived the change at `openspec/changes/archive/2026-09-23-rust-region-format-completion/`. The protocol branch carried an older active planning copy with unchecked tasks. This integration removes only that stale active copy; the completed archive, implementation and canonical requirements remain. `openspec validate --all --strict --no-interactive` passed 127 items after the removal. Whole-tree gates, independent final review, protocol spec sync and archive remain open.

## Additional whole-branch review finding — 2026-09-24

The independent protocol reviewer found one further P2 interoperability defect on the integrated tree: a valid Go fixture edited to declare a 128 MiB zstd window still decodes through both Rust snapshot entry points, while the production Go v45 decoder rejects `window size exceeded`. The Rust context's `WindowLogMax(21)` is a streaming parameter and does not constrain libzstd's one-shot bulk decode. The reviewer supplied an exact, unchanged-body mutation and confirmed the normal focused tests still pass; therefore the first review round's green tests do not establish this boundary. Node 5.5 now owns a bounded frame-header preflight, paired Rust/Go regression, and focused gate. Integrated acceptance moves to 5.6. The current long-running Rust/audit gate attempts began before this finding and will not count as final acceptance; they may be reused only as build-cache or unrelated-gate evidence. Main remains unchanged.
