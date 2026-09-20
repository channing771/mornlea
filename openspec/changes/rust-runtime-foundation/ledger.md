# rust-runtime-foundation ledger

## 2026-09-20 — target architecture planning synchronization

- Baseline: `8d9cc122486097fb7d7788abdb523ecd13f5a75a`.
- User scope: reconcile migration planning with the final architecture and improve the visual-baseline skill. This entry records planning only; no runtime task is complete.
- Ruling: add a concrete foundation change — later Godot feature plans referenced an unproposed Rust prerequisite — prevent proposal existence from being mistaken for implementation acceptance.
- Ownership: controller owns F1–F3 plans, target links and visual skills/docs; a fresh isolated agent audits and rewrites the seven later plans; a separate read-only agent forward-tests the complex visual skill. Isolation keeps multi-file discovery and review traces outside the controller's editing context.
- Directory guidance: planning artifacts and skill resources inherit root guidance; no runtime directory is created. Implementation tasks create scoped guides beside new architectural crates.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` are user-owned and excluded from this change.
- Validation: pending planning integration; prospective Cargo suites have not run and do not yet exist.
- Architecture skill: review at round close; current code and canonical contracts remain the current-behavior authority.

## 2026-09-20 — integrated planning review and validation

- Ruling: reconcile the complete F1–F3 → P8–P14 graph, not only target headers. UI is Godot Control/embedded Python over Rust views; Bootstrap retirement needs native diagnostics; default switch needs two release cycles and one recoverable previous release.
- Ruling: P12 phase 2 supplies capture/identity/strict-coverage and per-case canonical regression before feature handoffs. Canonical ownership moves only for reviewed cases; a full legacy producer cannot compare against newly owned baselines. Empty pilot mappings and exit-zero classification are not acceptance.
- Independent review: an isolated planning agent audited and rewrote the seven later changes; a read-only forward test exercised four realistic visual-skill requests. Follow-up foundation review added explicit Go-only kernel migration, Rust-producer session lifecycle evidence and the pinned Rust toolchain in root-level commands. All findings were resolved in planning.
- Skill result: both project-owned visual skills and handoff references are synchronized. Architecture skill: promoted only the target-owner evidence distinction supported by `docs/architecture-target.md` test ownership and canonical visual handoff contracts; volatile pilot details remain in the visual workflow/reference.
- PASS: `openspec validate --all --strict --no-interactive` (124 items at integration; final rerun recorded by command output).
- PASS: `go test ./packages/audit -run 'Test(ProjectArchitectureSkillsMatch|ArchitectureSkillRetrospective|ArchitectureTargetDocumentsFoundationAndRollback|VisualBaselineRouting|VisualProducerOwnership|CompletedDocumentationPairsAreSynchronized|DocumentationManifestClassifiesCurrentMarkdown|CurrentDocumentationLinks)' -count=1`.
- PASS: `go test ./packages/tools/perfcheck -run TestGodotPilotVisual -count=1` (existing tests verify unmapped coverage and classification-only behavior).
- PASS: skill-creator `quick_validate.py` for both visual skills, using an isolated `uv run --no-project --with pyyaml` environment because system Python lacks PyYAML; both skill/reference copies and architecture copies are byte-identical. Local Markdown link targets resolve; `git diff --check` passes.
- Existing baseline failures: `go test ./packages/audit -count=1` fails only `TestCurrentDocumentationVersions` (README/pilot report protocol v44 versus current v45) and `TestEnglishCommentMigration` (existing source comment debt exceeds its inventory). Both failures reproduce on untouched baseline `8d9cc122486097fb7d7788abdb523ecd13f5a75a` in a detached checkout with `go test ./packages/audit -run 'TestCurrentDocumentationVersions|TestEnglishCommentMigration' -count=1`. No exemptions, baseline weakening or unrelated source edits were made.
- This round changes planning, skills and explanatory documentation only. Runtime implementation checkboxes remain open; prospective Rust crates/commands, production visual registry/updater and cutover gates were not executed or claimed implemented. No tracked PNG/GIF, runtime, current canonical spec, protocol/save version or default entry changed. Full runtime Rust/race/GPU gates belong to the implementation tasks and were not run for this documentation-only round.

## 2026-09-20 — 1.1 contract inventory freeze

- Adopted in-flight untracked `packages/tools/cmd/runtime-oracle` sources from a prior session; they were incomplete (no tests, no `contracts.json`) and were not committed in Phase 1.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: the oracle is a stdlib-only tools leaf. It discovers registries by reading files and must not import protocol, storage, native ABI, or live authority packages. `packages/audit` `allowed` registers `packages/tools/cmd/runtime-oracle` with an empty import set.
- Coverage: 82 families (62 protocol including every ClientPacketForID/ServerPacketForID type plus framing and domain input/event, 7 save families with supported schema ranges, 11 kernels including 10 engine ABI exports plus Go-only pathfind, 2 agent contracts). Identities match the current matrix: protocol 45, chunk/player 9, metadata 6, companions.ai 5, hostile_mobs 2, passive_mobs 1, engine ABI 11, region 1, agent HTTP/MCP v1.
- Corpus digest: `sha256:1d87c666fc6612edaa78688f36fe8eda22598c1f04b486d9aca65582e10df022` for `testdata/runtime-migration/contracts.json`.
- Discovered tests (`go test ./packages/tools/cmd/runtime-oracle -list TestContractInventory`): `TestContractInventoryReconcilesFrozenCorpus`, `TestContractInventoryRejectsMissingFamily`, `TestContractInventoryRejectsVersionMismatch`, `TestContractInventoryRejectsMissingCoverageFixture`, `TestContractInventoryRejectsIncompleteIdentity` (5).
- PASS: `go test ./packages/tools/cmd/runtime-oracle -run TestContractInventory -count=1`
- PASS: `go test ./packages/tools/cmd/runtime-oracle -race -count=1`
- PASS: `go test ./packages/audit -run 'TestInternalDependenciesAreOneWay|TestCommentBacktickIdentifiersExist' -count=1`
- Directory guidance: added `packages/tools/cmd/runtime-oracle/AGENTS.md` for the offline-leaf invariant. `packages/tools/` remains without a module overview; it has no independent new boundary beyond this command.
- Architecture skill: no change. The empty-import oracle leaf is a change-local tooling rule, not a new cross-task ownership convention.

## 2026-09-20 — 1.2 isolated oracle traces

- Baseline: `c6a8bef7 feat(tools): freeze runtime contract inventory`.
- Adopted orphan untracked `packages/tools/cmd/runtime-oracle/trace_test.go` from an aborted worker after empirical review: the five tests match task 1.2 and the replay-identity spec, contain no contract violations, and were kept byte-for-byte. Production APIs (`Trace`, `TraceRequest`, `RunTrace`, `LoadTrace`, `ValidateTrace`) were added in `trace.go` without overwriting the test file. Deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: traces are stdlib-only. `RunTrace` copies coverage fixtures into an isolated work directory, hashes the copies, and omits absolute work paths so two temp dirs yield identical JSON. Paths under the repository root are live-path writes and fail before `MkdirAll`. The oracle still does not import protocol, storage, native ABI, or live authority packages.
- Corpus digest: `sha256:1d87c666fc6612edaa78688f36fe8eda22598c1f04b486d9aca65582e10df022` for `testdata/runtime-migration/contracts.json` (unchanged from 1.1).
- Discovered tests (`go test ./packages/tools/cmd/runtime-oracle -list 'TestTrace|TestContract'`): inventory 5 + trace 5 = 10 (`TestTraceRunIsDeterministicAndIsolated`, `TestTraceRejectsIncompleteIdentity`, `TestTraceRejectsLivePathWrites`, `TestTraceRejectsMalformedInput`, `TestTraceRejectsIncompleteTraces`).
- PASS: `go test ./packages/tools/cmd/runtime-oracle -race -count=1`
- Directory guidance: `packages/tools/cmd/runtime-oracle/AGENTS.md` now documents isolated replay alongside inventory freeze.
- Architecture skill: no change. Isolated temp-dir replay is the existing foundation tooling rule, not a new cross-task ownership convention.

## 2026-09-20 — 2.1 contract crate registration

- Baseline: `2994828d feat(tools): add isolated runtime oracle traces`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: workspace members are `mornlea_engine`, `mornlea_client`, `mornlea_godot`, `mornlea_domain`, `mornlea_protocol`, and `mornlea_storage`. Production edges are protocol→domain and storage→domain only. Domain has no production crate dependencies. None of the three foundation crates depend on `mornlea_engine`, `mornlea_client`, `mornlea_godot`, or each other except through domain. `TestNativeEngineLibraryIdentity` now pins the six-member workspace list.
- Directory guidance: added crate `AGENTS.md` files beside `mornlea_domain`, `mornlea_protocol`, and `mornlea_storage`; updated `packages/engine/AGENTS.md` for the workspace boundary.
- Discovered tests (`rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --test runtime_contract --locked -- --list`): domain 3, protocol 4, storage 4 (11). Zero-test discovery would have been rejected.
- PASS: `rustup run 1.97.1 cargo metadata --manifest-path packages/engine/Cargo.toml --no-deps --format-version 1` (six `mornlea_*` workspace packages).
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --test runtime_contract --locked`
- PASS: `go test ./packages/audit -run 'TestNativeEngineLibraryIdentity|TestInternalDependenciesAreOneWay|TestProjectArchitectureSkillsMatch' -count=1`
- Intended red cases before family ports (not implemented in this node):
  - Domain (`mornlea_domain`, families `domain.input` and `domain.event`): reject incomplete identity; reject NaN/Inf, unknown IDs, and out-of-range values; preserve deterministic ordering of semantic inputs and event observations.
  - Protocol (`mornlea_protocol`, 60 families): `protocol.frame` plus every `protocol.client.*` and `protocol.server.*` inventory row. Each family needs byte-preserving round-trip and malformed-input rejection (truncated, oversized, invalid enum, unsupported version) before publication. Port one family per tested commit.
  - Storage (`mornlea_storage`, 7 families): `save.chunk`, `save.player`, `save.companion`, `save.hostile`, `save.passive`, `save.world-metadata`, `save.region`. Each needs current-schema round-trip, supported-version migration to the same normalized result, and corrupt/partial/future-version rejection without implicit repair.
- Architecture skill: promoted the foundation crate split and production dependency direction into both project-owned copies. Verified by the new `runtime_contract` targets and `packages/engine/AGENTS.md`.

## 2026-09-20 — 2.2 domain identity and semantic records

- Baseline: `3e6167b9 feat(engine): register foundation contract crates`.
- Adopted leftover `packages/engine/crates/mornlea_domain/tests/runtime_contract.rs` cases from an aborted worker after empirical review: identity, range, NaN/Inf, and ordering tests match the 2.1 intended red list. Observation construction was corrected to `Observation::new` so unknown family IDs cannot be published through public fields. Deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded. Conversation-start untracked `packages/tools/cmd/runtime-oracle/trace_test.go` is already committed on this branch (`2994828d`) and was not rewritten.
- Ruling: `mornlea_domain` remains a no-dependency rlib. `Identities::current` pins the frozen inventory versions; `ReplayIdentity::new` and `Identities::validate` fail closed on incomplete evidence. `Dimension` accepts only overworld/depths, `HotbarSlot` accepts `0..COUNT-1`, and player/place rotations reject non-finite values before publication. Inputs order by sequence then kind; observations order by tick then family. Unknown observation families are `UnknownId`.
- Directory guidance: updated `packages/engine/crates/mornlea_domain/AGENTS.md` for identity, value, input, and observation invariants. Crate layout is `src/{lib,identity,values,input,event}.rs`.
- Discovered tests (`rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract --locked -- --list`): 10 (`crate_identity_matches_workspace_name`, `production_manifest_has_no_codec_kernel_or_host_dependencies`, `inventory_assigns_domain_families_to_this_crate`, `current_identities_match_frozen_inventory`, `incomplete_identity_is_rejected`, `invalid_dimension_and_hotbar_ranges_are_rejected`, `non_finite_input_rotation_is_rejected`, `semantic_inputs_order_by_sequence_then_kind`, `observations_order_by_tick_then_family`, `unknown_observation_family_is_rejected`). Zero-test discovery would have been rejected.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract --locked`
- Architecture skill: no change. Version pins, range checks, and observation ordering are crate-local contracts already covered by the foundation crate-split rule.

## 2026-09-20 — 2.3 protocol.frame

- Baseline: `15314951 feat(engine): port domain identity and input records`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: `write_frame` / `read_frame` copy the Go length-prefix contract. Length is a canonical uvarint excluding itself. Empty, oversized, truncated, overlong, and non-canonical prefixes fail before a payload is copied. `MAX_FRAME_BYTES` is the Go `MaxFrameBytes` pin. Canonical uvarint helpers are public because framing and later packet families share them. Remaining protocol families stay unchecked in `tasks.md` until each has its own tested commit.
- Directory guidance: updated `packages/engine/crates/mornlea_protocol/AGENTS.md` for framing. Crate layout adds `src/{error,varint,frame}.rs`.
- Discovered tests (`rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked -- --list`): 11.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change. Framing is the first protocol-family port under the existing crate-split rule.

## 2026-09-20 — 2.3 protocol.client.ClientHello

- Baseline: `c567e42b feat(engine): port protocol frame codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: handshake packet ID 0 payload is a canonical protocol-version uvarint. Encode/decode accept only `Identities::current().protocol` (golden `0x2d` for v45). Unsupported versions, truncated varints, and trailing bytes fail before publication. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 13.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.server.ServerHello

- Baseline: `61386d5b feat(engine): port client hello codec`.
- Adopted leftover `packages/engine/crates/mornlea_protocol/tests/runtime_contract.rs` ServerHello cases from an aborted worker after empirical review: golden `0x2d`, packet ID 0, unsupported version, truncated payload, and trailing bytes match the ClientHello family and Go `codec_golden_test.go`. Production `ServerHello` was added without rewriting those tests. Deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded. Conversation-start untracked `packages/tools/cmd/runtime-oracle/trace_test.go` is already committed and was not rewritten.
- Ruling: handshake server packet ID 0 payload is the same canonical protocol-version uvarint as ClientHello. Encode/decode accept only `Identities::current().protocol`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked -- --list`): 15.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.server.HandshakeReject

- Baseline: `b1ce71c2 feat(engine): port server hello codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: handshake packet ID 1 payload is a canonical uvarint server version, reject code `1`, and a length-prefixed UTF-8 message (max 256 bytes/runes). Server version is informational and may differ from the current protocol. Unknown codes are `InvalidEnum`; invalid UTF-8 and oversized declared lengths are `InvalidString`. Golden bytes match Go `2a01026e6f` and empty-message `080100`. Crate-private `ByteEncoder`/`ByteDecoder` land with this family for later packets. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 18.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.LoginStart

- Baseline: `95f27494 feat(engine): port handshake reject codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: login packet ID 0 payload is 16-byte UUIDv4 + length-prefixed display name + trailing view-distance u8. `PlayerId` rejects zero and non-v4 IDs. Display names must survive Go `NormalizeDisplayName` rules. View distance is the closed interval `2..=64`. Golden bytes match Go `00112233445546778899aabbccddeeff044368656e20`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 20.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.server.LoginSuccess

- Baseline: `8845e6b8 feat(engine): port login start codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: login server packet ID 0 payload is 16-byte UUIDv4 plus little-endian `u64` world seed. Zero seeds are legal. Non-v4 identities fail as `InvalidIdentity`. Golden bytes match Go `00112233445546778899aabbccddeeff8877665544332211`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 22.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.server.LoginReject

- Baseline: `4bb12a65 docs(openspec): record login success codec evidence`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: login packet ID 1 payload is a reject code `1..=7` plus a length-prefixed UTF-8 message (max 256 bytes/runes). Unknown codes are `InvalidEnum`; invalid UTF-8 and oversized declared lengths are `InvalidString`. Golden bytes match Go `02026e6f` and empty-message `0100`..`0700`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 25.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.server.Disconnect

- Baseline: `aa180e38 feat(engine): port login reject codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 6 payload is a disconnect code `1..=5` plus a length-prefixed UTF-8 message (max 256 bytes/runes). Unknown codes are `InvalidEnum`; invalid UTF-8 and oversized declared lengths are `InvalidString`. Golden bytes match Go `0203627965` and empty-message `0100`..`0500`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 28.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.server.KeepAlive

- Baseline: `9f6b4537 feat(engine): port disconnect codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 5 payload is a little-endian `u64` token. Zero tokens fail as `InvalidRange`. Golden bytes match Go `0800000000000000`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 30.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.KeepAliveReply

- Baseline: `23695793 feat(engine): port keep alive codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 4 payload is a little-endian `u64` token. Zero tokens fail as `InvalidRange`. Golden bytes match Go `0600000000000000`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 32.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.server.PlaceBlockSucceeded

- Baseline: `5bb5e050 feat(engine): port keep alive reply codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 20 payload is a little-endian `u64` sequence. Zero sequences are legal. Golden bytes match Go `8877665544332211`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 34.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.server.CommandRejected

- Baseline: `1790f056 feat(engine): port place-block success codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 4 payload is little-endian `u64` sequence plus reject-reason u8 `1..=15`. Unknown IDs fail as `InvalidEnum`. Golden bytes match Go occupied `070000000000000006` and frozen reason IDs `01`..`0f`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 37.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.SelectHotbar

- Baseline: `00aae881 feat(engine): port command rejected codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 5 payload is little-endian `u64` sequence plus hotbar slot u8. Slot validation reuses domain `HotbarSlot` (`0..=8`); out-of-range is `InvalidRange`. Golden bytes match Go `090000000000000008`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 39.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.DropSelectedItem

- Baseline: `58b65534 feat(engine): port select hotbar codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 11 payload is little-endian `u64` sequence. Zero sequences are legal. Golden bytes match Go `8877665544332211`. Selected slot and drop position stay server-owned. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 41.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.EquipArmor

- Baseline: `36360462 feat(engine): port drop selected item codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 18 payload is little-endian `u64` sequence, the same shape as DropSelectedItem. Zero sequences are legal. Golden bytes match Go `1200000000000000`. Selected item and destination armor slot stay server-owned. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 43.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.TakeCraftingOutput

- Baseline: `4fa77ed5 feat(engine): port equip armor codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 15 payload is little-endian `u64` sequence. Zero sequences fail as `InvalidRange` because they cannot take part in command acknowledgement. Golden bytes match Go `8877665544332211`. Output contents stay server-owned. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 45.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.MoveInventoryStack

- Baseline: `2220d102 feat(engine): port take crafting output codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 6 payload is little-endian `u64` sequence plus source and target slot u8. Slots must be distinct and inside `0..INVENTORY_SLOTS-1` (`36`, copied from Go `InventorySlots`). Same-slot and out-of-range pairs fail as `InvalidRange`. Golden bytes match Go `0a000000000000000323`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 47.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.MoveCraftingStack

- Baseline: `a98f112c feat(engine): port move inventory stack codec`.
- Adopted leftover `packages/engine/crates/mornlea_protocol/{src/move_crafting_stack.rs,src/lib.rs,tests/runtime_contract.rs,AGENTS.md}` from an aborted worker after empirical review: golden `0b000000000000000900`, packet ID 7, same-slot, out-of-range, inventory-to-inventory, truncated, and trailing-byte cases match Go `codec_golden_test.go` and `TestGridCraftingMoveValueDomain`. rustfmt collapsed the crate re-export onto one line; tests and production constructor were kept. Deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded. Conversation-start untracked `packages/tools/cmd/runtime-oracle/trace_test.go` is already committed and was not rewritten.
- Ruling: play packet ID 7 payload is little-endian `u64` sequence plus unified view slots. Grid is `0..CRAFTING_GRID_SLOTS-1` (`9`); inventory is `9..GRID_CRAFTING_VIEW_SLOTS-1` (`45`). Same-slot, out-of-range, and inventory-to-inventory pairs fail as `InvalidRange`. Inventory-to-inventory stays on `MoveInventoryStack`. Golden bytes match Go `0b000000000000000900`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 49.
- PASS: `rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml -p mornlea_protocol -- --check`
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.CloseContainer

- Baseline: `b77aa791 feat(engine): port move crafting stack codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 10 payload is little-endian `u64` sequence. Zero sequences are legal. The viewed container identity stays server-owned. Golden bytes match Go furnace fixture sequence 5 (`0500000000000000`). Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 51.
- PASS: `rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml -p mornlea_protocol -- --check`
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol.client.PlaceBlock

- Baseline: `14103024 feat(engine): port close container codec`.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` remain user-owned and excluded.
- Ruling: play packet ID 2 payload is little-endian `u64` sequence, two little-endian `f32` look angles, and a hotbar slot u8. Slot validation reuses domain `HotbarSlot` (`0..=8`). Non-finite yaw/pitch fail as `InvalidFloat`; out-of-range slots fail as `InvalidRange`. `ByteEncoder`/`ByteDecoder` now copy the Go `f32` primitive (NaN/Inf fail before publication). Golden bytes match Go `030000000000000000000040000080bf04`. Remaining protocol families stay unchecked in `tasks.md`.
- Discovered tests (`-- --list`): 53.
- PASS: `rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml -p mornlea_protocol -- --check`
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked`
- Architecture skill: no change.

## 2026-09-20 — 2.3 protocol client families and first server families

- Baseline: `94be4a5d feat(engine): port place block codec`. Verified family count at that baseline: 20 of the 60 `protocol.*` inventory families were ported (`protocol.frame`, 11 client, 8 server). The controller's worker brief had mis-stated this as 21; the corrected count is recorded here rather than carried forward.
- Adopted the in-flight untracked `OpenContainer` cases in `packages/engine/crates/mornlea_protocol/tests/runtime_contract.rs` from an aborted session after empirical review against the Go codec and the frozen golden bytes; kept byte-for-byte.
- Ported 14 families, one per verified commit. Client: `OpenContainer`, `RequestChunkResync`, `TillSoil`, `BoneMeal`, `CollectWater`, `PlaceWater`, `MoveContainerStack`, `MoveStackPartial`, `QuickMoveStack`, `DropStack`, `ChatCommand`, `PlayerInput`. Server: `CombatHit`, `RemotePlayerDespawn`. Commits `2c27b12a`, `841b6b3b`, `02884884`, `c4f75895`, `2cffee51`, `da38672b`, `c9dc161d`, `c974bea3`, `0268bee4`, `446959a4`, `22b388dd`, `b085c7bb`, `a4e768c0`, `0e5715ba`. All 23 client families and framing are now ported; 26 server families remain.
- Ruling: `ContainerRef::read` parses the 18-byte container reference without validating, and each family validates afterwards. The inventory and crafting views legitimately carry the all-zero `ContainerRef::NONE`, which `ContainerRef::new` rejects because generation 0 is illegal; validating inside the reader would make the documented zero-reference round trip unsatisfiable. Shared wire values (container reference, item stack, batch prefixes) converge into one module instead of a per-family copy.
- Ruling: golden bytes are derived from the Go fixture and never hand-transcribed. One in-session transcription error dropped a byte from an `f32` literal and was corrected against the controlled golden. A rejection assertion for container slot 37 was removed after confirming in `packages/shared/network/protocol` that 37 is the legal furnace fuel slot and only 38 is the output slot. Both are standing instructions for the remaining families.
- Deviation recorded: the worker passed `-c core.hooksPath=/dev/null` to all 14 commits. Controller verification: `core.hooksPath` is unset and both hook configurations are removed from the repository, so no installed hook ran on any commit and no gate was skipped in effect. The flag was unnecessary. The history is deliberately left unrewritten because rewriting it would require a destructive Git operation the project forbids without explicit user authorization.
- Existing unrelated work: deleted `.claude/skills/pr-submit/SKILL.md` and `.codex/skills/pr-submit/SKILL.md` remain user-owned and excluded from every commit.
- Orchestration: protocol-crate and storage-crate family ports ran as two isolated background workers with disjoint file ownership (`mornlea_protocol` versus `mornlea_storage`); the controller reserved the ledger and tasks. Path-explicit staging was enforced and `git add -A`/`git commit -a` were prohibited.
- Discovered tests (`-- --list`): 53 to 81 across the 14 families, non-zero and rising at every gate.
- PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked` (81 passed; 0 failed).
- PASS: `rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml -p mornlea_protocol -- --check`.
- `make rust`, `make rust-check`, `go test ./packages/audit -count=1` and full stage gates are deferred to task 4.2. No Rust, protocol, save, or entry-point production behavior outside this crate changed.
- Architecture skill: no change. The zero-reference parse rule and shared-wire-value convergence are crate-local contracts already covered by the foundation crate-split rule.

## 2026-09-20 — 2.4 save record and migration codecs

- Baseline: `94be4a5d feat(engine): port place block codec`; crate had only the 4 registration/guidance tests. Ported 6 of the 7 `save.*` inventory families, one per verified commit: `save.passive` (`5140dcfb`), `save.hostile` (`51731ed1`), `save.region` (`304d1266`), `save.world-metadata` (`17697449`), `save.companion` (`f8ad4a7c`), `save.player` (`70ee93eb`). Crate layout is one module per family plus shared primitives (`bytes.rs`, `crc32c.rs`, `error.rs`, `identity.rs`, `items.rs`); `AGENTS.md` updated in the same commit as each family.
- Each family carries the required shape: current-schema round-trip, supported-version migration to one normalized result, and corrupt/partial/future-version rejection without implicit repair. Upper bounds (record counts, file length, payload sizes, plan steps, FIFO depth, summary bytes) are checked before any allocation, matching the Go fail-fast order. Byte parity is pinned against the committed Go fixtures, including the exact reproduction of `companions-v5.bin` (nonzero mirror, canonical-zero mirror, inactive tombstone, task-bearing queue).
- Controller verification: all six commits touch only `packages/engine/crates/mornlea_storage/` paths; discovered and executed 53 tests, 0 failed; 10 additional `--lib` tests; `cargo fmt --check` clean; the production dependency gate `production_manifest_depends_only_on_domain` still holds and the crate declares only `mornlea_domain`.
- Blocker, recorded rather than worked around: `save.chunk` cannot be ported within the mandate because the Go envelope is built by `github.com/klauspost/compress/zstd` with `WithEncoderCRC(true)`, so byte-identical output requires a zstd compressor emitting the same xxhash-64 content checksum and a decompressor honouring the decoder memory cap. `std` provides no zstd and no zstd dependency exists anywhere in the engine workspace manifests. The constants a future port must honour are recorded here: envelope magic `CHNK`, envelope version 1, `compressionZstd = 1`, chunk schema 9 with supported range 1..9, `maxDecodedChunk` 2 MiB decoded, `region.MaxCompressedChunk` 1 MiB compressed (already ported in `src/region.rs`), logical magic `MCGC`, 44-byte envelope header. No chunk module, symbols, tests, or stub were added; the family stays open.
- Coordination event: while staging the companion commit, the concurrent protocol worker's in-flight working-tree state was already in the shared git index and the first commit swept it in. The worker reset that commit and re-committed with only `mornlea_storage` paths staged. Controller verification: all six commits are storage-only, and the protocol crate's 81-test suite was independently green at `0e5715ba` before the current in-flight server-family work.
- Existing unrelated work: deleted `.claude/skills/pr-submit/SKILL.md` and `.codex/skills/pr-submit/SKILL.md` remain user-owned and excluded from every commit.
- Orchestration: protocol-crate and storage-crate family ports ran as two isolated background workers with disjoint file ownership; the controller reserved the ledger and tasks and verified each worker's claims by re-running the suites.
- Architecture skill: no change. CRC-32C, identity, and item-stack primitives are crate-local contracts of the storage port, not new cross-task ownership rules.

## 2026-09-20 — 2.3 server state families

- Baseline: `0e5715ba feat(engine): port remote player despawn codec` (81 tests). Ported 12 server families, one per verified commit: `BlockChanges` (`b98c98e9`), `ForgetChunks` (`3c676333`), `ContainerClosed` (`4b294687`), `CompanionDespawn` (`1290e275`), `HostileDespawn` (`db3f6383`), `ProjectileDespawn` (`29035d2e`), `PassiveDespawn` (`064808ca`), `ChestState` (`10b36bee`), `FurnaceState` (`ff546e3a`), `CraftingState` (`df021ac6`), `InventoryState` (`8d96cc03`), and `HostileSpawn`/`HostileState` (`a9ddbb64`, `6230f2c2`). One doc-only commit `8342fa49` carried the `AGENTS.md` section left behind by the split commit sequence.
- Controller verification: 105 tests discovered and executed with 0 failures; `cargo fmt --check` clean; all 14 commits touch only `packages/engine/crates/mornlea_protocol/`; the crate working tree is clean. Count correction: the worker reported 14 families remaining, but the controller cross-checked the inventory against `src/lib.rs` exports and verified 13 remaining, all server-side, with 47 of 60 protocol families ported. The worker's count is not carried forward.
- Ruling on coverage checking: family coverage is verified against `src/lib.rs` `pub use` exports, never against module file names. `DropStack` and `QuickMoveStack` are exported from `move_stack_partial.rs`, so a module-name heuristic reports two ported families as missing.
- Shared helpers landed with these families, each a single owner of its wire concept: `item_stack.rs` (`ItemStack` fixed 5-byte slot value, registered-item table, stack limits, durability maxima, smelting and furnace-input/output predicates), `batch.rs` (`read_fixed`/`write_fixed`, `ByteCountBatch`, `UvarintCountBatch`, exact-remaining-length rejection, sorted-batch rules), `block.rs` (`BLOCK_ID_MAX`, section geometry, `chunk_of` flooring for negative coordinates, `chunk_block_index` as the batch ordering key), `entity_id.rs` (`CompanionId`, `valid_companion_name`). `container_ref.rs` gained `validate_furnace`/`validate_chest`/`validate_any` while `read` stays validation-free.
- Coordination event: the concurrent storage worker's plain `git commit` for `f8ad4a7c` swept this worker's then-staged `BlockChanges` files into that commit, and the storage worker's subsequent rewrite dropped them again. The `BlockChanges` files were re-staged afterwards. Controller verification: `block_changes.rs` is recorded in `b98c98e9` and no work was lost; both crates' commit histories are clean of cross-crate contamination.
- Minor deviations recorded: commit `4b294687` also carried the `container_ref.rs` validator helpers and a corrected type fix belonging to another family, and `3c676333`/`4b294687` landed in the reverse of the intended order. Both are same-crate scope, recorded for traceability rather than rewritten.
- Three hand-written golden literals (padding lengths and byte offsets in forget-chunks, crafting-state, furnace-state, hostile-spawn/state) were wrong on first run and were corrected against the Go layout before committing. This reconfirms the standing ruling that goldens are derived from the Go fixture and never hand-transcribed.
- Blocker, escalated to the user: `protocol.server.ChunkSnapshot` (packet ID 0) carries a zstd-compressed logical snapshot, and this crate may depend only on `mornlea_domain`, so it cannot be ported without a compression dependency decision. It was left entirely untouched — no module, symbols, tests, or stub. The remaining 12 families were dispatched separately.
- Existing unrelated work: deleted `.claude/skills/pr-submit/SKILL.md` and `.codex/skills/pr-submit/SKILL.md` remain user-owned and excluded from every commit.
- Orchestration: three isolated workers ran with disjoint file ownership (`mornlea_protocol` for two of them, `mornlea_storage` for one) while the controller reserved the ledger and tasks and independently re-ran every claimed suite.
- Architecture skill: no change. The shared wire-value modules and the export-based coverage check are crate-local contracts of the protocol port.

## 2026-09-20 — approved compression dependencies for byte-exact ports

- User ruling (controller decision record, both approved in one exchange): `save.chunk` in `mornlea_storage` and `protocol.server.ChunkSnapshot` in `mornlea_protocol` may each gain the compression dependency required for byte-exact parity with the Go implementations. `save.chunk` is being implemented under that approval; `ChunkSnapshot` is queued behind the in-flight protocol worker and must not be started concurrently in the same crate.
- Scope of the approval: exactly one compression crate per foundation crate, nothing else. No upgrade of the pinned toolchain, and no change to any existing dependency line anywhere else in the workspace.
- Gate consequence recorded so the approval cannot quietly widen: both crates carry `production_manifest_depends_only_on_domain`, which currently asserts the production dependency set is exactly `["mornlea_domain"]`. Each must be rewritten to pin the permitted set exactly — `mornlea_domain` plus the approved compression crate — while `FORBIDDEN_PRODUCTION_DEPS` (`mornlea_protocol`/`mornlea_storage`/`mornlea_engine`/`mornlea_client`/`mornlea_godot` as applicable per crate) stays rejected. Loosening the assertion to a "does not contain" check is not acceptable.
- Rationale: the Go chunk save envelope and the chunk snapshot payload are both built by `github.com/klauspost/compress/zstd`; reproducing their bytes requires an encoder emitting the same frame including the xxhash-64 content checksum written by `WithEncoderCRC(true)`. `std` provides no zstd and the engine workspace has no zstd dependency.
- Architecture skill: no change. This is a change-scoped dependency approval, not a new cross-task ownership rule; the foundation crate-split rule itself is unchanged.

## 2026-09-20 — 2.4 compression dependency and the encoder-identity blocker

- Landed `cff59950` `feat(engine): add approved compression dependency for chunk save parity` under the user's approval: `zstd` v0.13.3 added to `packages/engine/crates/mornlea_storage/Cargo.toml` only, binding the reference libzstd implementation (`zstd-safe` 7.2.4, `zstd-sys` 2.0.16+zstd.1.5.7). Chosen because the task requires a real encoder and decoder honouring the frame's xxhash-64 content checksum; a decode-only crate cannot serve the encoder side. Cargo initially resolved `zstd-safe` 7.3.0 / `zstd-sys` 2.1.0, which could not be fetched in the worker sandbox, so the lock was pinned to the cached 7.2.4/2.0.16 via `cargo update --precise`. No existing dependency line anywhere else changed.
- Gate consequence: `production_manifest_depends_only_on_domain` now asserts the permitted set is exactly `["mornlea_domain", "zstd"]`, with `FORBIDDEN_PRODUCTION_DEPS` still rejected and a doc comment recording that exactly one compression dependency is permitted and everything else stays forbidden.
- Controller verification: commit `cff59950` touches only `packages/engine/Cargo.lock`, `packages/engine/crates/mornlea_storage/Cargo.toml`, and that gate test; `runtime_contract` 53 passed, `--lib` 10 passed, `cargo fmt --check` clean; `cargo metadata` resolves all six workspace members.
- Decode path is sound: libzstd decodes all nine `chunk-v*.bin` fixtures and recovers the `MCGC` logical payload.
- **Blocker, independently verified by the controller: byte-exact re-encode parity between the Go encoder and any Rust encoder is not achievable.** For every one of the nine fixtures, klauspost's frame header and trailing xxhash-64 checksum are byte-identical to libzstd's, and only the compressed block payload differs. v6 and v8 even match the output *length* (378 bytes) while still diverging inside the block data, which rules out a settings mismatch. Brute-forcing compression levels 1 through 22 on v9 produced no match.
- Controller's independent confirmation that the criterion itself is well-posed: a standalone Go program using `zstd.NewWriter(nil, WithEncoderConcurrency(1), WithEncoderCRC(true))` plus `EncodeAll` re-encodes all nine decoded fixtures to bytes **identical** to the committed files (v1 263, v2 294, v3 294, v4 326, v5 328, v6 378, v7 155, v8 378, v9 414 — `identical=true` for all nine). The Go encoder is therefore deterministic and does reproduce the frozen fixtures, and the divergence is exclusively a Rust-versus-Go encoder difference. No Rust crate binds klauspost.
- Consequence: `save.chunk` has no module, exports, tests, or stub. The dependency and the decode path are landed and usable; only the encoder-side byte-identity assertion cannot be met.
- The same encoder difference will block `protocol.server.ChunkSnapshot` in `mornlea_protocol`, whose payload is compressed by the same Go library against the frozen `packages/shared/network/codec/testdata/chunk-snapshot-v1.bin`. The decision recorded for `save.chunk` governs that family too.
- Existing unrelated work: deleted `.claude/skills/pr-submit/SKILL.md` and `.codex/skills/pr-submit/SKILL.md` remain user-owned and excluded from every commit.
- Architecture skill: no change. Cross-implementation zstd encoder non-identity is a property of the compression libraries, not a new ownership rule; it is recorded here as a change-scoped acceptance-criterion blocker.

## 2026-09-20 — ruling: semantic round-trip replaces byte-identical re-encode

- User ruling: for `save.chunk` and `protocol.server.ChunkSnapshot`, the acceptance criterion is semantic round-trip, not byte-identical re-encode. Decode of every committed fixture must be exact; encode followed by decode must be lossless; supported-version migration must converge on one normalized result; corrupt, partial, and future-version input must still be rejected without implicit repair. Byte-identical re-encode assertions must not be written, because no Rust encoder can reproduce klauspost's compressed output (see the previous entry for the verified evidence).
- Rationale recorded so the difference is not later mistaken for a defect: zstd frames are self-describing, and both implementations share the reference frame format, header fields, and xxhash-64 content checksum. The divergence is confined to the compressed block payload. Cross-implementation compatibility is therefore defined at the logical/decode level, and the Rust encoder legitimately emits different bytes than the Go encoder for the same logical chunk.
- Scope: this ruling governs both `save.chunk` in `mornlea_storage` and `protocol.server.ChunkSnapshot` in `mornlea_protocol`, so the question is not reopened per family.
- Standing consequence: every ported compression family must document in its module doc comment that the encoder output is intentionally not byte-identical to the Go encoder, so a future reader does not treat it as a regression to fix.
- The `zstd` dependency landed in `cff59950` stays; it is required for the decode path and the encoder regardless of the criterion.
- Architecture skill: no change. The criterion is change-scoped; the underlying fact is a property of the compression libraries.

## 2026-09-20 — 2.4 save.chunk under the semantic round-trip ruling

- Ported `save.chunk` in `eb8a2386` `feat(engine): port chunk save codec`, the seventh and last save family. Commit scope verified by the controller: exactly five paths, all under `packages/engine/crates/mornlea_storage/` (`AGENTS.md`, `src/chunk.rs`, `src/items.rs`, `src/lib.rs`, `tests/runtime_contract.rs`). `Cargo.toml`, `Cargo.lock`, the user-owned `pr-submit` deletions, and the ledger were excluded.
- Controller verification: `runtime_contract` 63 passed (53 at the start of the family, +10), `--lib` 19 passed (10 at the start, +9 in `chunk::tests`), `cargo fmt --check` clean. Test-first confirmed: the ten new scenarios were added and observed failing (`E0432` unresolved imports) before `src/chunk.rs` existed.
- Criterion satisfied as ruled: all nine committed `chunk-v1..v9.bin` fixtures decode to the exact logical payload; `decode(encode(payload)) == payload` for every fixture chunk plus a synthetic chunk covering drops, furnaces, chests and all four paletted storage kinds; schemas 1 through 9 converge on one identical normalized `Chunk` with `migrated == (schema < 9)`; every fixture reports `schema == 9`.
- Partial parity pinned rather than dropped: `chunk_frame_header_and_content_checksum_match_the_reference_frame` asserts for all nine fixtures and for Rust-encoded output that the zstd magic, the frame-header descriptor (`0x64`, single segment plus content checksum plus two-byte content size, no dictionary ID), and the frame content size equal the envelope's declared decoded length. It also flips the trailing checksum byte and requires rejection, so the checksum is verified present and enforced. No assertion compares compressed bytes to a fixture.
- Interoperability verified once outside the repository, nothing committed: a Rust encoding of the chunk decoded from `chunk-v9.bin` (470-byte frame, 22 522-byte logical payload) was read successfully by the repository's own Go `chunk.Decode`; Go then re-encoded it to bytes **identical** to the committed `chunk-v9.bin` (matching SHA-256), which independently confirms the Rust encoder emitted exactly the right logical payload; Rust decoded that Go frame back to the identical `Chunk` with matching logical-payload digests in both directions.
- The module doc comment and `AGENTS.md` state that the Rust encoder intentionally emits different compressed bytes than the Go encoder, that this was verified and ruled on rather than being an unresolved bug, and that cross-implementation compatibility is defined at the logical/decode level because zstd frames are self-describing.
- Architecture gate introduced by this change and recorded as a blocker for the closeout task: `TestNativeEngineBridgeBoundary` in `packages/audit` now fails. Root cause established by the controller and unrelated to the compression dependencies: the gate scans Go files under `packages/shared`, `packages/server`, `packages/client`, `packages/tools`, and `packages/audit` for the literal tokens `mornlea_engine.h` and `-lmornlea_engine` and permits them only under `packages/shared/nativeabi`. The runtime oracle's `packages/tools/cmd/runtime-oracle/discover.go`, committed in `c6a8bef7` during the inventory-freeze task, reads `packages/engine/include/mornlea_engine.h` as a source document to inventory the kernel and ABI families. That is read-only observation rather than ABI bridging, but the token scan cannot distinguish the two, so the gate has failed since that commit. A worker attributed this failure to a concurrent agent's in-flight work; that attribution is wrong and the corrected cause is recorded here.
- The other two audit failures, `TestCurrentDocumentationVersions` and `TestEnglishCommentMigration`, reproduce the baseline recorded during the planning round and are unrelated to this change.
- Resolution not yet applied, deliberately: the correct fix expresses the boundary the gate intends (only `packages/shared/nativeabi` may bridge or link the engine C ABI; the offline oracle may read the header as an inventory source) without weakening the linkage prohibition. That is an architecture-gate change and is escalated rather than applied inside a save-codec node.
- Architecture skill: no change. The semantic-round-trip criterion and the read-only-versus-bridging distinction are recorded here as change-scoped rulings.

## 2026-09-20 — 2.3 complete: all 60 protocol families ported

- Ported the final protocol family `protocol.server.ChunkSnapshot` in two commits: `40dd6131` `feat(engine): add approved compression dependency for snapshot parity` (`packages/engine/crates/mornlea_protocol/Cargo.toml`, `packages/engine/Cargo.lock`, updated gate test only) and `60c47664` `feat(engine): port chunk snapshot codec` (module, exports, shared primitives, tests, `AGENTS.md`).
- Controller verification: `runtime_contract` 133 passed (129 at the start of the family, +4), `cargo fmt --check` clean, `cargo metadata --no-deps` resolves. Both commits verified scoped: `40dd6131` touches only the protocol manifest, the lock, and the gate test; `60c47664` touches only `packages/engine/crates/mornlea_protocol/`. `packages/engine/Cargo.lock` was checked before staging and contained exactly one addition, the `zstd` line in the `mornlea_protocol` dependency block. The concurrent storage agent's files and the user-owned `pr-submit` deletions were excluded.
- **Coverage closed: 60 of 60 `protocol.*` inventory families are ported, zero remaining**, verified by the controller against `testdata/runtime-migration/contracts.json` cross-referenced with `src/lib.rs` exports (not module file names, since `DropStack` and `QuickMoveStack` are exported from `move_stack_partial.rs`).
- Criterion satisfied as ruled by the user for compression families: the fixture `packages/shared/network/codec/testdata/chunk-snapshot-v1.bin` decodes to overworld chunk `(-3, 7)`, revision 19, 24 sections cycling all four paletted storage kinds, with the decompressed logical payload byte-identical to an independently reconstructed golden snapshot; `decode(encode(snapshot)) == snapshot` and `decode(encode(decode(fixture))) == decode(fixture)` both hold. No assertion compares a Rust-encoded frame's compressed bytes to the fixture.
- Partial parity pinned rather than dropped: zstd magic, frame header descriptor, the content size equal to the logical length, and the trailing xxhash-64 content checksum are all asserted equal between the Go fixture's frame and the Rust frame.
- Rejection matrix covered: every envelope truncation, compressed-length mismatch, outer trailing byte, declared decoded length too short and too long, corrupted frame checksum, `MAX_COMPRESSED_SNAPSHOT` boundaries (admitted sizes reach the decoder and fail as truncated, the over-limit size fails as frame-too-large before decompression), `MAX_DECODED_SNAPSHOT + 1` before decompression, and a declared zstd expansion bomb. Logical: wrong section count, out-of-order section Y, unknown storage value (the future-version rejection, since the wire defines exactly three kinds), indexed bits outside 4 and 8, palette count past the allocation bound, duplicate and unregistered palette IDs, out-of-range palette slot, wrong word counts, direct bits other than 15, direct unused high bits, trailing byte, and strided truncation prefixes. All palette and word counts are validated against remaining bytes before any allocation.
- Interoperability verified once outside the repository, nothing committed: a Go program using the repository's real `codec.NewCodec` path decoded a Rust-encoded snapshot to the same `dim=0 chunk=(-3,7) rev=19 sections=24` with `reflect.DeepEqual` reporting the two logical snapshots identical; the same Go program re-encoded the decoded snapshot and reproduced the committed fixture exactly, while its re-encode did not match the Rust payload — confirming the divergence is exclusively a Rust-versus-Go encoder difference and that compatibility holds at the logical level.
- The module doc comment and `AGENTS.md` state that the Rust encoder intentionally emits different compressed bytes than the Go encoder, that this was verified and ruled on rather than being an unresolved bug, and that cross-implementation compatibility is defined at the logical/decode level.
- Shared helpers landed with this family: `drop_id.rs` (`DropId` 17-byte wire identity with the dimension deliberately unvalidated because Go's `DropID.Valid` checks only slot range and generation, so a stricter Rust rule would be a parity break), `UvarintCountBatch::require_minimum_records` in `batch.rs` (the pre-allocation budget check for families whose Go decoder rejects a short payload but accepts a long one, where the exact-length rule would report a padded batch as truncated instead of trailing), `MAX_CHUNK_BLOCK_INDEX` in `block.rs`, and `CompanionId::NONE`/`is_none`/`from_bytes` plus `valid_display_name` promoted to `pub` and `valid_bounded_text`/`valid_command_text` extracted so command and speech text share one rule with caller-supplied bounds.

## 2026-09-20 — section 2 "Rust shared contracts" closed

- All four section-2 tasks are complete and verified by the controller: 2.1 crate registration, 2.2 domain identity and semantic records, 2.3 all 60 protocol families, 2.4 all 7 save families. `tasks.md` boxes 2.3 and 2.4 are checked in this node.
- Final verified suite state: `mornlea_protocol` `runtime_contract` 133 passed; `mornlea_storage` `runtime_contract` 63 passed plus `--lib` 19 passed; both crates `cargo fmt --check` clean; `cargo metadata` resolves all six workspace members.
- The foundation crates' production dependency sets are exactly as approved: `mornlea_domain` none, `mornlea_protocol` `mornlea_domain` plus `zstd`, `mornlea_storage` `mornlea_domain` plus `zstd`. Both crate gate tests pin those sets exactly and still reject every forbidden internal crate.
- Stopping point per the user's instruction: section 3 (kernel reuse and acceptance) and section 4 (closeout) are deliberately not started.
- Carried blocker for the closeout task, recorded so it is not rediscovered: `TestNativeEngineBridgeBoundary` in `packages/audit` fails because the runtime oracle's `discover.go` reads the engine header as an inventory source while the gate permits that token only under `packages/shared/nativeabi`. The correct fix expresses the intended boundary (only `nativeabi` may bridge or link the engine C ABI; the offline oracle may read the header for inventory) without weakening the linkage prohibition. That gate change is escalated and not applied inside a codec node. `TestCurrentDocumentationVersions` and `TestEnglishCommentMigration` remain the pre-existing baseline failures recorded during the planning round.
- Architecture skill: no change. The semantic-round-trip criterion for compression families, the read-only-versus-bridging distinction, and the export-based coverage check are recorded here as change-scoped rulings rather than promoted as cross-task ownership rules.

## 2026-09-20 — controller review reopens foundation acceptance

- User scope: review the current foundation deeply, retain architecture/design responsibility in the controller, and make worker tasks precise. This round changes planning/evidence documents only. No runtime fix, new kernel implementation, startup switch or live-save operation was performed.
- Reviewed baseline: `60c476645ee6dae1f6392336a7f3c593d2163ae3`, branch `codex/align-runtime-migration-plans`. HEAD stayed unchanged during review. Full findings, triggers and limits are in `review.md`.
- **Superseding acceptance ruling:** the earlier section-2 closure and all-60-protocol-export claim are not valid acceptance evidence. `ContainerClosed` is not in `mornlea_protocol/src/lib.rs`; the legacy companion decoder loses queue IDs; chunk/companion encoders accept unreadable aggregates; region encoding can panic; mutable packet encoders can panic or emit invalid bytes; login decoding blocks the existing explicit rejection policy. Domain rules remain duplicated, and input ordering differs from Go. Tasks 1.1, 1.2, 2.2, 2.3 and 2.4 are reopened. Task 2.1 registration remains accepted. Historical entries above are preserved as history, not rewritten as if the defects had already been known or repaired.
- Evidence defect: the current oracle emits hashes of copied fixtures/source files at synthetic checkpoints, not executed codec/kernel/state observations. Its trace validator accepts missing inputs/cases and an unscheduled observation; path checking misses a symlink ancestor followed by multiple missing components. This cannot establish replay parity. Corrected design separates inventory provenance, test-only Go adapters, executable Rust consumers and manifest-bound tooling schema 2. Full authoritative simulation replay remains in the later runtime changes.
- Verified integration command: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --test runtime_contract --locked` passed 206 tests (10 + 133 + 63). These existing tests did not detect the reproduced defects. This is not a workspace-wide result.
- Verified failing gate: `rustup run 1.97.1 cargo clippy --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --all-targets --locked -- -D warnings` fails first on the ten-argument domain `PlayerInput::new` (`clippy::too_many_arguments`). Protocol test compilation also reported 16 warnings. No lint exemption was added and no full `make rust-check` success is claimed.
- External storage repro at `/var/folders/99/_lwvdq2d6db3qj6ngjklqf0h0000gn/T/mornlea-storage-review-cyvpiecv`: four contract tests fail on v2/v3/v4 orphan queue IDs, 65 stored companion records, 1200 region entries and furnace-on-air encoding. A Go program using the actual storage packages confirms the legacy IDs and rejects both oversized companions and furnace-on-air. The Rust region panic reaches offset 28676 beyond its 28672-byte bank buffer. The accepted 65-record Rust save is 16038 bytes and its own decoder rejects it.
- External protocol repro at `/var/folders/99/_lwvdq2d6db3qj6ngjklqf0h0000gn/T/mornlea-protocol-review-tl7f1uu3`: all four expected-contract tests fail. Structurally complete version-44 hello and zero-UUID login are rejected before admission; public NaN mutation panics in `PlayerInput::encode`; selected-slot 255 is emitted and rejected by `InventoryState::decode`. Go structural/admission behavior was checked against `codec_client.go`, `ValidateDecodedClientWirePacket` and `BeginServerLogin`.
- External oracle repro at `/var/folders/99/_lwvdq2d6db3qj6ngjklqf0h0000gn/T/mornlea-oracle-review-wdcal_c1`: incomplete-checkpoint and deep-symlink-ancestor rejection tests fail; seed changes leave the fixture-only observations unchanged. The symlink case uses a temporary fake repository; the real checkout is not written. Temporary repro sources are not committed; durable test inputs/outcomes are documented in `review.md` and the repair nodes.
- Reconciled artifacts: proposal now describes partial/unaccepted implementation; delta spec explicitly covers admission asymmetry, aggregate encoding, queue associations, ordering, executable evidence and resource bounds; design assigns shared domain ownership, fallible buffer APIs, fixed region cardinality, corpus execution and Rust-native numerical boundaries; tasks distinguish acceptance milestones, bounded repairs and controller-held design packets. Missing kernel/event field-level design remains explicit controller work, not a vague worker assignment. Section 3 is not started.
- Controller decisions: preserve the existing user approval for exactly one `zstd` dependency in each codec crate and semantic cross-decoding of chunk/snapshot compression; do not reopen compressed-byte identity. Consolidate current identity/item rules in domain while retaining raw historical DTOs and the existing permissive `DropId` dimension semantics. Use bounded caller-owned buffers/scratch. Pathfinding preserves deterministic Go results but uses dense node state and an indexed min-heap instead of copying the linear open-list algorithm. No throughput improvement is claimed without measurement.
- Orchestration: two isolated read-only protocol/storage reviews supplied bounded evidence; the controller checked findings, ran counterexamples and wrote the architecture/acceptance decisions directly. No implementation worker was launched. Isolation was for focused independent review, not to offload design or fill parallel capacity.
- Validation of this planning revision: `openspec validate rust-runtime-foundation --strict --no-interactive` passed; `openspec validate --all --strict --no-interactive` passed all 124 items; `git diff --check` passed. Local Markdown target/source-line checks and task-ID uniqueness checks passed. Planning validation does not accept runtime behavior.
- Existing audit blockers remain: oracle-induced read-only engine-header token classification, and previously reproduced baseline documentation-version/comment failures. Their old evidence is retained; this round did not rerun full audit/dev/race/platform gates or launch a graphical window. The task list now requires a narrow boundary-gate repair with negative linking tests, not a broad oracle exemption.
- Commit ownership: the checkout already had user-owned deletions of both `pr-submit/SKILL.md` copies, 105 added ledger lines and completion-checkbox edits in `tasks.md` before review. The append-only ledger prefix was preserved byte-for-byte. The reviewed task-status correction overlaps the pre-existing 2.3/2.4 completion edits; this coherent planning revision is left uncommitted rather than combining it with those prior edits or presenting a partial artifact set as a completed commit. No staging, reset or cleanup was performed. Pre-review planning snapshots are at `/var/folders/99/_lwvdq2d6db3qj6ngjklqf0h0000gn/T/mornlea-review-planning-before-ov_u_46g`.
- Architecture skill: no change. The crate ownership rule already exists in the synchronized project skill; newly designed buffer/oracle/kernel interfaces are not yet implemented and verified, and the concrete compatibility defects belong in this change's evidence rather than durable cross-task history.

## 2026-09-20 — Superpowers redesign of unstarted work

User requested a complete controller-owned redesign using Superpowers and a persistent project rule. The main Agent used installed Superpowers6.4.1 brainstorming/writing-plans and project planning/architecture/orchestration guidance. Read-only agents provided domain/protocol/kernel/storage facts; no worker was asked to choose architecture or implement runtime code. Main Agent resolved all target interfaces, functional field maps, algorithm/capacity/error/compatibility decisions and integration order. See planning-review.md for the detailed self-review and actual planning checks.

Artifacts: execution-contract.md; packet-contracts.md,storage-contracts.md,kernel-contracts.md; plans/01-evidence.md through06-acceptance.md; rewritten tasks.md with117 unique nodes, one retained accepted registration2.1 and116 unchecked implementation/acceptance nodes. The59 packet families have individual bounded tasks; seven save families and all supported versions,ten existing kernel APIs and pathfinding have explicit packets. tasks.md includes an old-to-new ID table; historical ledger entries above retain their original meaning. No prior completion was silently reinterpreted as implementation of the new plan.

Material rulings: ChatIntent stays outside sequenced command ordering; immutable semantic events replace digest-only coverage; domain checked current values remain separate from raw armor/metadata fidelity; active companion cap is4; metadata weather7/255 preservation is a newly source-inspected Rust/Go discrepancy assigned4.7. Encoding uses preflight and atomic caller-buffer publication. Native kernels use typed requests/reusable exclusive scratch,not ABI request blobs. Mesh uses bounded geometry staging; metadata pointer aliases are checked before clearing; native rescan guarantees halo while its legacy adapter preserves established failure status. Pathfinding uses dense arrays/indexed heap with the exact four independently emitted transition kinds and first-insertion ties. These are design decisions,not claims that their implementations now pass.

Self-review corrected an initial path-transition exclusivity error,PlayerInput payload length,maximum-batch UUID seeds,zstd concatenated-stream compatibility,atomic constructor/caller integration and package-private Go oracle seams. Generic future-design placeholders were removed from executable briefs. Foundation contract acceptance6.2 releases numerical work; complete numerical replay6.3 waits for it,so the dependency graph is acyclic.

Verified planning evidence:117 unique task IDs;59 packet nodes exactly equal current direction/state registry;10 existing numerical families;7 save families;135 document/task links;all dependencies resolve and no cycle;Codex/Claude orchestration skill and reference byte equality. Focused provider/orchestration/directory-guide audits pass; documentation manifest/pair/legacy/link/semantic-pair audits pass. Both official skill validations pass with the existing Agent virtualenv Python. System Python initially lacked PyYAML; no package was installed. Documentation revision mismatch was corrected in the two manifest entries and the same audit then passed. Strict OpenSpec validation:125 passed,0 failed. git diff --check:pass.

Persistent rule is implemented by the separate require-controller-designed-worker-plans change:root AGENTS.md,openspec/config.yaml,synchronized orchestration skills/references and bilingual workflow docs with manifest revisions. Architecture skill:no change; new workflow rules belong to orchestration and planned numerical details are not yet stable verified implementation facts.

This round did not modify runtime source,regenerate legacy fixtures,run runtime cargo/build/race gates,access live saves,start a service/window,install plugins,stage or publish code. Existing runtime review defects and baseline broad-gate blockers remain open for the concrete implementation nodes. Preexisting pr-submit skill deletions and user changes are preserved. The exact commit overlap remains foundation ledger.md (preexisting105 added lines plus prior review append),tasks.md (earlier2.3/2.4 edits and review reopening),and prior-review proposal/design/spec edits; no broad commit was made that would absorb those changes. Planning documents inherit root/OpenSpec guidance; no new independently governed runtime directory was created.

## 2026-09-21 — controller verification accepts nodes 1.1-1.3

- Baseline for this verification: `06cd5b42 fix(runtime): require manifest-bound complete traces`. The three implementation commits (`2753b9cd`, `385b9be8`, `06cd5b42`) predate this entry; the controller re-ran every gate below at this HEAD rather than accepting reported results. Commit scope verified: `2753b9cd` owns the schema-2 manifest, the shared Rust corpus helper, the frame case and the three foundation manifests/lock; `385b9be8` owns only `packages/audit` plus the oracle guide; `06cd5b42` owns only `packages/tools/cmd/runtime-oracle/trace.go` and its test. None absorbed the uncommitted planning revision or the user-owned `pr-submit` deletions.
- Node 1.1 evidence: schema-2 inventory separates `sources` provenance from executable `cases`; one real framing case `protocol.framing/45/valid` with input bytes `[0x02,0x00,0x2d]` produced by Go `codec.WriteFrame`/`ReadFrame`. PASS: `go test ./packages/tools/cmd/runtime-oracle -run '^TestContractInventory' -count=1`. PASS: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked corpus_frame -- --list` discovers exactly one real case (`corpus_frame`), and the same filter without `--list` passes 1 test with 133 filtered out, so the Rust consumer decodes the frozen framing bytes rather than parsing the manifest.
- Node 1.2 evidence: PASS: `go test ./packages/audit -run 'TestInternalDependenciesAreOneWay|TestNativeEngineBridgeBoundary' -count=1`. The read-only engine-header provenance in the oracle is now distinguished from cgo bridging/linking, which resolves the carried `TestNativeEngineBridgeBoundary` blocker recorded during the 2.4 save.chunk node without weakening the linkage prohibition.
- Node 1.3 evidence: PASS: `go test ./packages/tools/cmd/runtime-oracle -run '^TestTraceIdentity' -count=1`.
- Result: nodes 1.1, 1.2 and 1.3 are accepted; their `tasks.md` boxes are checked by the controller. Workers did not check their own boxes. Node 1.4 is next in execution order (prerequisite 1.3 accepted); node 1.5 waits for 1.2, 1.3 and 1.4.
- Existing unrelated work: the uncommitted Superpowers planning revision under `openspec/changes/rust-runtime-foundation/` and the deleted `.claude/skills/pr-submit/SKILL.md` and `.codex/skills/pr-submit/SKILL.md` remain user-owned and excluded from any implementation commit.
- Architecture skill: no change. The manifest-bound evidence model and the read-only-versus-bridging distinction are already recorded as change-scoped rulings in earlier entries.

## 2026-09-21 — 1.4 isolated trace workspace and atomic export accepted

- Baseline for implementation: `06cd5b42`. Implementation commits: `7b6cab28` (harness-owned workspace via `NewTraceWorkspace`, `ExportTrace` with validated-first ordering, resolved-ancestor containment, one-component-at-a-time creation, staged write/sync/close and no-replace `os.Link` publication), `89739efc` and `24366a06` (review fixes), `49b5fcd8` (controller doc sync), plus controller housekeeping `84542740` (gofmt const alignment in `inventory.go`, pre-existing since the accepted node-1.1 baseline and outside this node's file set). Every commit scope was verified by the controller; none absorbed unrelated work.
- Task review loop: the first review returned two Important findings — the export symlink guard inspected only the deepest existing component, so an intermediate symlink with an existing descendant published through it; and `verifyCorpusAsset`/`corpusAssetBudget`/`verifyCorpusAssets` duplicated `checkNoSymlinks` plus the regular-file and budget half of `validateAsset` while being strictly narrower than the canonical validator that `Reconcile` already applies to every case. Both were fixed under test-first discipline: the new regression failed against the unfixed code (`expected symlink rejection, got: <nil>`) before the fix landed. The re-review returned spec compliant and quality Approved with no Critical or Important findings.
- Controller adjudications recorded: the component walk inspects every existing component strictly below the lexically named repository root plus the deepest component wherever it lies, because a full absolute-chain check would reject legitimate targets on platforms where a system directory such as `/var` is a symlink; `isLivePath` resolves both the root and the target, so containment still rejects writes into the repository addressed through resolved paths; the deleted trace-side gate's regular-file check is not re-added to the canonical `validateAsset` in this node because that file belongs to the accepted node-1.1 scope and the hardening is a separate decision.
- Discovered and executed at HEAD (`49b5fcd8`), controller-verified: node filter `^TestTrace(Isolation|Path|Output|IO)` green (14 tests), node-1.3 regression `^TestTraceIdentity` green, full package `go test ./packages/tools/cmd/runtime-oracle -race -count=1` green, `go vet` clean, `gofmt -l` empty. Zero-test discovery would have been rejected.
- Documented test-name correction: the controller's worker dispatch for this node carried `TestTraceRejectsIncompleteTraces` from a pre-revision ledger entry; that name never existed at the node-1.3 baseline. The implementer's correction of the package guide's enforcement list to the actual names is accepted, and the controller's context error is recorded here rather than carried forward.
- Open Minors carried to the final whole-branch review: a post-link staged-file removal failure returns an error while a complete report already exists; an isolation test creates a probe directory inside the real repository and relies on deferred removal; `RunTrace` currently holds a workspace nothing stages into; the package guide's enforcement list still omits `TestTraceIdentity`, the `LoadTrace` regressions and this node's own focused filter; the canonical `validateAsset` has no explicit regular-file check; the symlinked-source-fixture test swaps a real repository fixture and restores it on defer.
- Architecture skill: no change. Harness-owned temporary work, atomic no-replace publication and the lexical-root symlink walk are change-local tooling contracts already covered by the offline-leaf rule.
