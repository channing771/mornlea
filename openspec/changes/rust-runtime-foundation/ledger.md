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
