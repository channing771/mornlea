# Runtime oracle

`packages/tools/cmd/runtime-oracle` is the offline contract inventory and replay
oracle for OpenSpec change `rust-runtime-foundation`. It observes repository
sources and testdata only. Production code must not import live authority,
native ABI, network transports, or storage codecs; production files have an
empty allowed internal import set. Only `_test.go` files are permitted to import
designated offline codec, world, companion, and storage packages
(`shared/network/codec`, `shared/network/protocol`, `shared/core`,
`shared/world`, `shared/companion`, `shared/pathfind`, `shared/nativeabi`, and
`server/storage/{chunk,player,companion,hostile,passive,region}`). Neither
production nor test files may import `server/server`, file stores, client/render,
or Agent process packages. These boundaries are enforced by `packages/audit`
`TestRuntimeOracleInternalDependencies` and `TestInternalDependenciesAreOneWay`.

## Inventory freeze (`inventory.go`, `discover.go`, `root.go`)

- `Discover` reads current protocol, save, kernel, and agent registries from
  files. It does not load production packages.
- `LoadInventory` reads the frozen corpus at `InventoryRelPath`
  (`testdata/runtime-migration/contracts.json`).
- `ReconcileWorking` permits in-progress manifests to contain zero-case families
  and returns sorted `Covered` and `Uncovered` coverage points without claiming
  complete acceptance; `ReconcileComplete` fails closed with an `*InventoryError`
  if any uncovered family/version point remains.
- Both reconciliation paths share a private validator that enforces identity,
  source provenance, case structural and cryptographic validity, case version
  membership in its family's `supported_versions`, expected outcome `kind: "ok"`
  plus `kind: "error"` (unless covered by a reviewed `NegativeCoverageExceptions`
  rationale), and exact family/version/operation membership in the closed
  `BaselineConsumerRegistry()`. A consumer registration binds its implementation
  kind to executable routes; empty names, invalid kinds, empty route sets, and
  known consumers used on unsupported routes fail closed.
- The baseline routes are `corpus_frame` → `protocol.frame/45/decode`;
  `mornlea_domain` → `domain.identity_values/current/admit`,
  `domain.values/current/admit`, `domain.command_control/current/admit`,
  `domain.command_inventory/current/admit`, and `domain.event/1/admit`;
  `external:agent-contract` → `agent.http/v1/agent-contract` and
  `agent.mcp/v1/agent-contract`; and `external:runtime-authority` →
  `domain.input/45/order`.
- Enforcement: `TestContractInventoryReconcilesFrozenCorpus`,
  `TestContractInventoryWorkingReportsZeroCaseFamilies`,
  `TestContractInventoryCompleteRejectsZeroCaseFamilies`,
  `TestContractInventoryRejectsUnknownConsumer`,
  `TestContractInventoryRejectsKnownConsumerOnUnsupportedRoute`,
  `TestContractInventoryRejectsInvalidConsumerRegistry`,
  `TestContractInventoryInputAssetBudgets`,
  `TestContractInventoryRejectsNonRegularAssets`,
  `TestLoadInventoryRejectsNonRegularFile`,
  `TestContractInventoryRejectsUnsupportedCaseVersion`,
  `TestContractInventoryWorkingAndCompleteCoverage`,
  `TestContractInventoryRejectsMissingFamily`,
  `TestContractInventoryRejectsVersionMismatch`,
  `TestContractInventoryRejectsMissingProvenanceSource`,
  `TestContractInventoryRejectsIncompleteIdentity`.

## Isolated replay (`trace.go`)

- `BuildTrace` assembles executed observations into a versioned `Trace` identity
  (source revision, contract versions, corpus digest, seed, ordered checkpoint
  inputs, tick schedule, and normalized observations). Trace assembly is pure
  and performs only structural validation without filesystem access.
- `TraceRequest` carries no work directory, repository root, or source revision.
  The harness owns the workspace.
- `NewTraceWorkspace` creates an exclusive temporary directory with
  `os.MkdirTemp("", "mornlea-runtime-oracle-")` and returns the cleanup that
  removes exactly that path. A workspace that would resolve inside the
  repository is rejected as a live-path write.
- `ValidateTraceAtRoot(root, trace, manifest)` validates a trace report against
  expected assets at an explicit corpus root. It loads every selected expected
  asset through bounded path, symlink, duplicate-key, and sha256 checks, and
  compares normalized outcomes. Every load error is recorded; none is skipped.
- `LoadTraceAtRoot(root, path, manifest)` loads a report from path, enforces
  duplicate-key and size budgets, and delegates to `ValidateTraceAtRoot`.
- `ExportTrace(root, target, trace, manifest)` publishes one report and stays
  separate from execution. It validates the trace against root first, resolves
  the target through its existing ancestor, rejects repository containment and
  symlink components, refuses a preexisting target, creates missing directories
  one component at a time, and stages the report in the target parent before
  publishing it with `os.Link`. The atomic publication success boundary is the
  successful link; staged-file cleanup is best-effort.
- Containment is judged on resolved paths, and only a `..` path element counts
  as leaving the repository: a component whose name merely starts with `..`
  (a sibling such as `..cache`) is a child of the repository, not an escape
  from it.
- Corpus assets are validated by the canonical inventory validator during
  reconciliation: every case's input, expected, and encoded asset must be
  reachable without a symlink component, stay inside its byte budget, and
  match its recorded digest. Manifests, provenance sources, inputs,
  expectations, and encoded assets must be regular files before reads or
  hashes. JSON inputs and expectations use the 256 KiB JSON budget; binary
  inputs and encoded assets use the 4 MiB binary budget.
- Incomplete source revision, missing contract identity, empty corpus digest,
  empty tick schedule, or missing observations fail closed. `LoadTraceAtRoot`
  rejects truncated bytes, non-object JSON, duplicate keys, and unsupported
  `schema_version`.
- Enforcement: `TestExecutedObservationMismatchFailsAgainstUnchangedExpected`,
  `TestTraceValidationRejectsMissingExpected`,
  `TestTraceValidationRejectsMalformedExpected`,
  `TestTraceValidationRejectsExpectedDigestMismatch`,
  `TestTraceValidationRejectsDifferentCorpusRoot`,
  `TestBuildTraceRejects*`, `TestTraceRejectsIncompleteIdentity`,
  `TestTraceRejectsLivePathWrites`, `TestTraceRejectsMalformedInput`,
  plus the isolation, path, output, and I/O regressions in
  `trace_isolation_test.go` (`TestTraceIsolation*`, `TestTracePath*`,
  `TestTraceOutput*`, `TestTraceIO*`).

## Independent operation runners (`runner_helpers_test.go`, `protocol_frame_test.go`)

`runner_helpers_test.go` declares `GoOperation`, the registries, `RunCases`,
and the isolated export helpers (`exportGeneratedAssets`,
`exportGeneratedAssetsFromEnvironment`).

- `GoOperation` and the `map[string]GoOperation` registry live in test code
  only. A producer receives the case specification and the case input bytes and
  returns the normalized outcome, its own encoded bytes, and an error. The
  recorded expected outcome is never handed to a producer, so an independent
  execution cannot be shaped by the evidence it is supposed to reproduce.
- `RunCases` enumerates the manifest selection, resolves each input under the
  input-format-specific corpus byte budget shared with reconciliation, proves
  the input digest matches the manifest, invokes the registered producer once
  per declared checkpoint, and derives every
  observation from the returned values. An unknown family, an unregistered
  operation, an operation that disagrees with its family's binding, a missing
  checkpoint, or a tampered input digest is a hard error.
- Protocol producers use the real production codec. The framing producer calls
  `codec.ReadFrame` and classifies a rejection into one of the frozen execution
  contract categories (`invalid-varint` for a non-canonical length prefix); an
  unclassified failure is an error rather than an unlabelled rejection.
- Every committed `*.expected.json` under `testdata/runtime-migration/cases/`
  publishes the frozen outcome vocabulary: `kind` is `ok` or `error`, and an
  `error` category is one of the structural, login admission, or storage values
  the execution contract names.
- Explicit fixture and asset export is gated by `RUNTIME_ORACLE_EXPORT_DIR`.
  There is no repository default: an unset variable exports nothing. When set,
  `exportGeneratedAssets` resolves the export root through its nearest existing
  ancestor, rejects repository containment and any symlink below that ancestor,
  and validates every asset path, duplicate, and file-as-parent collision before
  mutation. It creates export-root gaps, producer prefixes, and asset parents one
  component at a time with `Lstat`/`Mkdir`: existing components must be real
  directories, the final producer child (`<exportRoot>/<producerID>`) is created
  exclusively, and a preexisting final child is rejected. Containment is
  resolved again before fixed relative assets are opened with create-exclusive
  semantics, so exports cannot escape, follow a fixed-prefix symlink, or replace
  existing evidence.
  Production `main.go` reconciles and validates existing artifacts and has no
  trace-generation mode.
- Enforcement: `TestExportGeneratedAssetsRejectsRepositoryContainedRoots`,
  `TestExportGeneratedAssetsRejectsSymlinkedAncestor`,
  `TestExportGeneratedAssetsRejectsProducerPrefixSymlink`,
  `TestExportGeneratedAssetsRejectsNonDirectoryProducerPrefix`,
  `TestExportGeneratedAssetsRejectsEscapingRelativePath`,
  `TestExportGeneratedAssetsRejectsInvalidAssetSetBeforeCreation`,
  `TestExportGeneratedAssetsRejectsPreexistingProducerChild`,
  `TestExportGeneratedAssetsSuccessfulMultiProducerExport`,
  `TestProtocolOracleFrameIndependentOutcomes`,
  `TestCorpusOutcomeVocabularyMatchesExecutionContract`,
  `TestProtocolOracleFrameOutcomesDistinguishCases`,
  `TestProtocolOracleFrameRunnerRejects*`,
  `TestProtocolOracleFrameRunnerHandsProducerOnlyCaseAndInput`,
  `TestProtocolOracleFrameRunnerInvokesProducerOncePerCheckpoint`,
  `TestReadCaseInputUsesFormatBudget`,
  `TestProtocolOracleFrameExport*`.

## Family evidence (`domain_*_test.go`, `agent_contract_test.go`)

Each executable corpus family in this package has exactly one producer file,
and each declares a family-scoped working manifest so a run never hands its
producer another family's cases: `protocol_frame_test.go` (framing), and
`domain_values_test.go`, `domain_identity_values_test.go`,
`domain_command_control_test.go`, `domain_command_inventory_test.go`,
`domain_event_player_test.go`, `domain_event_world_test.go`,
`domain_event_inventory_test.go`, `domain_event_people_test.go`,
`domain_event_mobs_test.go`, `domain_event_objects_test.go` and
`domain_event_chat_test.go` (the domain families). The world-event file also
carries the shared router arm (`runDomainEvent`) that the inventory, people,
mobs, objects and chat producers register their rule names into, because the
`domain.event` family is shared by seven producers and the rule name is the
only discriminator the manifest carries.

- `domain_event_mobs_test.go` executes the six hostile and passive mob
  publication rules (`hostile-spawn`, `hostile-state`, `hostile-despawn`,
  `passive-spawn`, `passive-state`, `passive-despawn`) through the Go
  `protocol` DTOs. Its 68 frozen cases live under
  `testdata/runtime-migration/cases/domain/event_mobs/` but are not yet
  registered in the canonical manifest; that registration is a later node's
  work, so publication is external-only through `RUNTIME_ORACLE_EXPORT_DIR`
  under the producer ID `runtime-oracle/domain-event-mobs`, with the export
  running before the committed-bytes comparison so an initial export can
  materialize the full candidate while the tracked directory is still absent.
  IDs and ticks are decimal strings in the frozen input so the full `u64`
  range stays lossless, grazing renders as a JSON Boolean in the normalized
  outcome, a rejected record retains the raw value of an unknown enum, and
  the 64-record wire batch caps stay transport budgets no case sits above.
  Rejection categories stay inside the frozen vocabulary: `invalid-identity`
  for a zero entity ID, `invalid-enum` for dimension, kind, grazing and
  reason, and `invalid-value` for everything else, with rule names shaped
  `<rule>.record_<index>.<field>`, `<rule>.count_range` and
  `<rule>.strictly_increasing_ids`.

- `domain_event_objects_test.go` executes the five projectile and item-drop
  publication rules (`projectile-spawn`, `projectile-state`,
  `projectile-despawn`, `item-drop-upserts`, `item-drop-removes`) through the
  Go `protocol` DTOs. Its 45 frozen cases live under
  `testdata/runtime-migration/cases/domain/event_objects/` but are not yet
  registered in the canonical manifest; that registration is a later node's
  work, so publication is external-only through `RUNTIME_ORACLE_EXPORT_DIR`
  under the producer ID `runtime-oracle/domain-event-objects`, with the export
  running before the committed-bytes comparison so an initial export can
  materialize the full candidate while the tracked directory is still absent.
  Projectile IDs and ticks are decimal strings in the frozen input so the full
  `u64` range stays lossless, a drop identity stays the object of its five
  ordered key fields with the raw i32 dimension first, and a carried stack
  stays the numeric `item/count/durability` object. A drop's raw dimension is
  deliberately unvalidated — the Go `DropID.Valid` rule checks only the slot
  range and the generation, so a negative raw dimension is an admitted
  boundary and the batch ordering compares the raw dimension first.
  Rejection categories stay inside the frozen vocabulary: `invalid-identity`
  for a zero projectile ID and for drop-ID slot/generation errors,
  `invalid-enum` for an unknown projectile kind or dimension, and
  `invalid-value` for block-index, stack, non-finite and aggregate errors,
  with rule names shaped `<rule>.record_<index>.<field>`,
  `item_drop_upserts.drop_<index>.<field>` and
  `item_drop_removes.id_<index>.<field>`, plus `<rule>.count_range` and
  `<rule>.strictly_increasing_ids`. The 128/32-record wire batch caps stay
  transport budgets no case sits above.

- `domain_event_chat_test.go` executes the sole closed-chat publication rule
  (`chat`) through the Go `protocol.ChatEvent` DTO, closing the Go evidence
  stage beside the mob (68 cases) and object (45 cases) producers with its
  44 frozen cases. They live under
  `testdata/runtime-migration/cases/domain/event_chat/` but are not yet
  registered in the canonical manifest; that registration is a later node's
  work, so publication is external-only through `RUNTIME_ORACLE_EXPORT_DIR`
  under the producer ID `runtime-oracle/domain-event-chat`, with the export
  running before the committed-bytes comparison so an initial export can
  materialize the full candidate while the tracked directory is still absent.
  The record is a semantic union rather than a flat payload: every raw input
  key is always present (a decimal-string event identity, the two
  32-lowercase-hex UUID identities, numeric kind and reason, and the two text
  slots), and an admitted event publishes its exact semantic branch
  (`accepted`, the four rejection branches, the five task-fact branches,
  `task-failed-<reason>` or `speech`) with only that branch's legal
  companion/name/command/speech data — the empty wire sentinels a branch
  forbids are not retained. The classifier mirrors the exact branch order of
  `ChatEvent.Validate`: global identity and name failures precede kind
  dispatch, a non-speech kind carrying speech fails before the switch, and
  inside the switch reason, companion identity/name, then command/speech text
  decide. Rejection categories stay inside the frozen vocabulary:
  `invalid-identity` for a zero event/player identity, `invalid-enum` for an
  unknown kind, the reserved reject reason 3 and failure reasons outside
  16..20, and `invalid-value` for every text-boundary (the 1,024/256 bounds
  and their plus-one rows, untrimmed or empty names, commands and speech) and
  every illegal cross-field combination (speech leak, command on speech,
  reason-not-none, missing companion, zero companion where illegal). Rule
  names begin `chat_event.` and name the failing field or combination:
  `chat_event.rejected.reason` for the reserved reason, `chat_event.task_failed.reason`
  for the failure-reason domain edges, `chat_event.kind` for an unknown kind,
  and `chat_event.<field>` for everything else. A rejection retains the raw
  semantic inputs, including an unknown numeric enum value.

- `agent_contract_test.go` is deliberately not an executable Agent producer. It
  validates manifest identity, case presence, golden coverage and the
  service-free ownership boundary for the explicit `external:agent-contract`
  consumer. Real Agent HTTP/MCP serialization execution remains package-local
  to `packages/shared/companion`; this package must not copy expected outcomes
  into `ExecutedObservation` values or publish a nominal Agent trace.
- Expected outcomes come from executing the real Go validator or codec, never
  from a hand-written value or a Rust result. Every test run compares generated
  bytes against the frozen corpus read-only; package-test flags or code paths
  capable of rewriting the tracked corpus are strictly prohibited.
- No file in this package may rewrite the frozen manifest: the CLI has no such
  flag and the test binary registers none, because both the discovery stub and
  any partial regeneration would silently gut the corpus. Manifest merges are
  controller-side and manual.
- Enforcement: `TestDomainOracle_<topic>` per producer,
  `TestAgentContractOracle*`, `TestCorpusOutcomeVocabularyMatchesExecutionContract`
  over every committed expectation, and `TestTestBinaryFlagsCannotRewriteTheFrozenCorpus`
  (which asserts that no flag containing `update` or mentioning tracked corpus
  rewrites can be registered in the test binary). The repository-wide
  `packages/audit` guards `TestCorpusTestFlagsCannotRewriteFrozenEvidence` and
  `TestCorpusWriterFlagGuardDetectsDrift` enumerate runtime-migration producer
  tests and reject update, rewrite, regeneration and write flags while leaving
  separately governed storage/protocol golden workflows outside this rule.

## Focused Verification

```bash
go test ./packages/tools/cmd/runtime-oracle -run TestContractInventory -count=1
go test ./packages/tools/cmd/runtime-oracle -list TestContractInventory
go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleFrame' -count=1
go test ./packages/tools/cmd/runtime-oracle -race -count=1
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked corpus_frame
```
