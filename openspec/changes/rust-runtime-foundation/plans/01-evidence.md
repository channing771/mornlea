# Executable evidence plan

**Goal:** replace source-file hashes with independently executed operations and fail-closed coverage. **Architecture:** the production inventory CLI stays stdlib-only; Go `_test.go` producers execute established APIs, and Rust test consumers read reviewed frozen results. **Tech stack:** Go standard library, Rust test-only serde_json/sha2. **Spec:** the coverage, identity, isolation and resource requirements in `../specs/rust-runtime-foundation/spec.md`. **Global constraints:** `../execution-contract.md`; no service startup, online writer, source-fixture modification or Rust-to-Go subprocess. **Review focus:** a changed result must fail even when all source filenames still exist.

All paths in this plan are repository-relative. `ORACLE` means `packages/tools/cmd/runtime-oracle`, `CORPUS` means `testdata/runtime-migration`, and `SUPPORT` means `packages/engine/tests`. They are path abbreviations, not shell variables. Only `../tasks.md` tracks completion. Each node inherits the exact red/green/review/commit sequence and rollback policy from the execution contract.

<a id="node-1-1"></a>

## 1.1 — Manifest and shared consumer with one real framing case

Dependencies: accepted crate registration 2.1. Own `ORACLE/inventory.go`, `discover.go`, `inventory_test.go`, new `case_test.go`, `CORPUS/contracts.json`, new `CORPUS/cases/frame/`, `SUPPORT/runtime_corpus.rs`, `SUPPORT/AGENTS.md`, three foundation Cargo manifests and test registration lines, workspace Cargo.lock. The controller integrates manifests/lock; no packet family files are owned here.

Inventory schema 2 retains the existing identity vector and discovered family IDs. Replace the misleading `fixtures` evidence with separate `sources: [{path,sha256}]` and `cases: [case_id]`. Add top-level `source_revision` (40 lowercase hex), `cases: [CaseSpec]`. `CaseSpec` fields are `id`, `family`, `version`, `operation`, optional `packet_key {direction,state,id}`, `input {path,sha256}`, `expected {path,sha256}`, optional `encoded {path,sha256}`, `checkpoints: [u64-as-decimal-string]`, and `rust_consumer` (stable dispatch name). Allowed operations: `decode`, `encode`, `migrate`, `admit`, `order`, `kernel`, `agent-contract`. A case input is either binary decode bytes or JSON typed operation arguments, declared by `input_format: binary|json`; it is never inferred from extension. Inputs and expected outcomes obey the bounds and JSON normalization in the execution contract. Add domain families explicitly to discovery; do not infer them from a packet's role alone.

Canonical digest algorithm: serialize schema-2 manifest JSON with recursively sorted keys, no insignificant whitespace and one terminal newline; its digest field is not part of the manifest. Hash the UTF-8 prefix `mornlea-corpus-v2\n`, canonical manifest bytes, then records sorted by case ID, each `case_id + NUL + input_sha256 + NUL + expected_sha256 + NUL + encoded_sha256_or_empty + LF`. Digests use lowercase `sha256:` plus 64 hex digits. Binary payload digests bind all bytes. Source provenance hashes are a separate manifest field, never observations. Go uses `json.Decoder.UseNumber`; Rust never converts 64-bit decimal strings through f64.

Red cases: real frame `(id=0,payload=[0x2d])` is bytes `[0x02,0x00,0x2d]`; Go `codec.WriteFrame` and `ReadFrame` generate `protocol.framing/45/valid`. Source-only family coverage must fail; editing payload 0x2d→0x2c without its digest must fail; duplicate ID, absent Rust consumer name, absolute/`..`/symlink path, 8193 cases, 4 MiB+1 manifest and 4 MiB+1 binary fail before reads. A readable `.go` source path cannot satisfy a binary case.

Implementation: bounded stat/read; reject duplicate JSON keys in identity/case documents; validate paths; validate identities/operations; hash real content; check family/version coverage; return sorted diagnostics. Add the shared Rust helper signatures from the execution contract, with root resolution by walking from CARGO_MANIFEST_DIR to the workspace's known testdata root, not process cwd. Unit-test helper failures using private temporary manifests. One integration assertion must decode the framing bytes with the existing Rust API, not just parse the manifest. Add only the approved dev dependencies. `SUPPORT/AGENTS.md` owns offline evidence and forbids runtime imports into production dependency sets.

Validation: `go test ./packages/tools/cmd/runtime-oracle -run '^TestContractInventory' -count=1`; `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked corpus_frame`. Discover the same Rust filter with `-- --list` and require a real case. Commit: `test(runtime): bind corpus cases to executed framing evidence`.

<a id="node-1-2"></a>

## 1.2 — Narrow oracle dependency and header-provenance gates

Dependencies: 1.1. Own `packages/audit/dependency_test.go`, `packages/audit/platform_test.go`, their existing gate helpers, `ORACLE/AGENTS.md`; read `packages/audit/AGENTS.md` and the existing gate test organization before editing.

Permit only `_test.go` files in ORACLE to import `shared/network/codec`, `shared/network/protocol`, `shared/core`, `shared/world`, `shared/companion`, `shared/pathfind`, `shared/nativeabi`, and `server/storage/chunk`, `server/storage/player`, `server/storage/companion`, `server/storage/hostile`, `server/storage/passive`, `server/storage/region`. Do not grant permission to import `server/server`, file stores, client/render, or Agent process packages. Production ORACLE imports remain standard library. Package-local tests for unexported Go behavior are specified in the consuming nodes, so no online authority import exception is needed here.

Native boundary scanner: classify Go AST string literals used as read-only header paths separately from cgo preambles and build directives. The literal `packages/engine/include/mornlea_engine.h` in discovery is provenance. A cgo preamble containing `#include`, `#cgo LDFLAGS` linking the engine, or a production bridge import outside the allowed bridge remains rejected. Do not delete tokens or whitelist ORACLE wholesale.

Red matrix: accepted read-only path; rejected same file with cgo include; rejected external link directive; rejected ordinary `.go` codec import; accepted `_test.go` codec import; rejected `_test.go` online server import. Run `go test ./packages/audit -run 'TestInternalDependenciesAreOneWay|TestNativeEngineBridgeBoundary' -count=1`. Record preexisting unrelated audit failures separately. Commit: `test(audit): distinguish oracle evidence from production dependencies`.

<a id="node-1-3"></a>

## 1.3 — Schema-2 trace completeness

Dependencies: 1.1. Own `ORACLE/trace.go`, `trace_test.go`. Keep path publishing out of this node. New signatures: `ValidateTrace(trace Trace, manifest Inventory) error` and `LoadTrace(path string, manifest Inventory) (Trace,error)`. There is no manifest-free acceptance API. `Trace` contains schema=2, source revision, corpus digest, identities, seed decimal string, checkpoint schedule, inputs and observations. Each input has contiguous `index` starting at 0, `case_id`, tick, input digest and typed arguments digest. Each observation has `(tick,case_id)`, outcome and expected digest. Copying fixture digests is removed.

Validation order: schema/byte/count budgets → actual manifest/content binding → strictly increasing nonempty schedule → exact input membership and indices → exact observation key set → normalized outcome equality. Build expected pairs from each selected case's declared checkpoints. Reject missing/extra/duplicate input indices and `(tick,case_id)` pairs, unknown cases, unsupported versions, and mismatched source/seed/schedule. Codec cases declare checkpoint 0; stateful kernels can declare multiple checkpoints. A trace may select a bounded subset for a focused test; whole-inventory acceptance explicitly requires every manifest case. A subset trace must list its selected IDs and cannot label itself complete inventory coverage.

Red table: schedule `[1,2,3]` with only tick999; duplicate tick2; reversed schedule; unknown case; missing second input; duplicate index0; fake nonempty digest; source SHA mismatch; one changed normalized scalar; empty selection; schema1. Each fails with the relevant identity and creates no output. Positive table: one real framing case at0; two declared cases at1/2 with exact observations; same inputs repeated produce identical JSON.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestTraceIdentity' -count=1`; require at least the 12 negative subcases above and two positive cases. Commit: `fix(runtime): require manifest-bound complete traces`.

<a id="node-1-4"></a>

## 1.4 — Exclusive temporary work and atomic export

Dependencies: 1.3. Own `ORACLE/trace.go`, new `trace_isolation_test.go`; serialize integration with 1.3. Remove caller-owned WorkDir from generation. `NewTraceWorkspace(root string) (dir string, cleanup func(), err error)` uses `os.MkdirTemp("", "mornlea-runtime-oracle-")`; the harness exclusively owns it. Keep `ExportTrace(root, target string, trace Trace, manifest Inventory) error` separate from execution.

Algorithm: validate the trace first; resolve repository root or fail; walk the target upward with Lstat until an existing ancestor is found, collecting missing suffix components; EvalSymlinks that ancestor; reconstruct the suffix; reject repository/live-save containment using `rel == "." || (rel != ".." && !strings.HasPrefix(rel,".."+separator))`. Do not treat a sibling named `..cache` as contained. Walk existing target components with Lstat and reject symlinks for export, even if they resolve outside. Refuse a preexisting final target. Create missing directories one component at a time, rechecking existing components. Fixture inputs must be regular files with no symlink components. Use a temporary file in the target parent, checked write/sync/close, then `os.Link(temp,target)` for atomic no-replace publication on supported local filesystems; remove temp afterwards. Link failure is a hard I/O failure; do not fall back to overwrite/rename. This is an exclusively owned offline directory contract, not a claim of defending a concurrently hostile filesystem. Only remove harness-owned temporary paths during cleanup.

Red cases: repository symlink ancestor with `missing-a/missing-b/out.json`; lexical sibling; source fixture symlink; preexisting output sentinel; permission/I/O failure; invalid trace paired with a nonexistent path. Assert unchanged repository tree and no final report on failure. Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestTrace(Isolation|Path|Output|IO)' -count=1`. Commit: `fix(runtime): isolate trace work and publish reports atomically`.

<a id="node-1-5"></a>

## 1.5 — Independent operation runners

Dependencies: 1.2, 1.3, 1.4. Own new `ORACLE/runner_test.go`, `protocol_frame_test.go`, and `SUPPORT/runtime_corpus.rs` runner additions. `type GoOperation func(CaseSpec, []byte) (Outcome, []byte, error)` and `map[string]GoOperation` exist in test code only. Unknown operation/family is a hard error. `RunCases` enumerates manifest selection, hashes input, invokes the registered function once per declared checkpoint and produces observations from returned values. It never supplies expected outcomes to the callback. The stdlib CLI reconciles/validates existing artifacts; it cannot generate fake execution traces.

Protocol producers use `codec.NewCodec`, `EncodeClient/DecodeClient` or `EncodeServer/DecodeServer` and the state/direction registry. Always close codec contexts. Framing uses `codec.WriteFrame/ReadFrame`. Rust `FrozenCase` adds parsed operation and typed argument JSON; dispatch is a match over real consumers, not a name count. Begin with frame case above, including a noncanonical `[0x82,0x00,0x00,0x2d]` length. Assert equal independent outcomes and a mutation of the Rust decoded ID fails. Later packet nodes add exactly one family producer/consumer each.

Test writers default to `t.TempDir()`. Explicit fixture export is `RUNTIME_ORACLE_EXPORT_DIR=<fresh-harness-dir>` and must pass the same no-symlink/containment checks; no repository default is accepted. Package-local Go producers (metadata, login admission and runtime ordering) use this same JSON schema, standard-library encoding/hashing and no shared test package imports. They are invoked separately by their node's command; the controller validates and imports results. No production exports are added just to reach private Go behavior.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleFrame' -count=1`; run the Rust `corpus_frame` command from 1.1. Commit: `test(runtime): execute independent corpus operations`.

<a id="node-1-6"></a>

## 1.6 — Agent HTTP/MCP contract evidence without a service

Dependencies:1.5. Own new `packages/shared/companion/runtime_contract_oracle_test.go`, `ORACLE/agent_contract_test.go`, `CORPUS/cases/agent/`. The producer is package-local to reuse the **existing pure Go contract validator**, not a newly invented schema interpreter. Read the companion subtree guide. `contractLoadSchemas`, `contractLoadGolden` and `validateDefinition` already exist in `contract_fixtures_test.go`; use the same schema/keyword audit, canonical context and expected error-path/rule comparison as `TestContractFixtureSchemasValidateGoldens`.

Run every valid/invalid fixture case in `packages/contracts/companion-agent/http-v1/golden/{valid,invalid}.json` and `mcp-v1/golden/{valid,invalid}.json`, plus `mcp-v1/golden/mine-validation.json`; add all three schemas/manifests to provenance. HTTP application versionv1; MCP applicationv1/protocol2025-11-25. Preserve fixture names as case labels after a deterministic lowercase slug plus source index; duplicate labels fail. Outcomes contain accepted canonical values or actual rejected path/keyword/rule from the existing validator. They are not copied from expected_error. Explicit red mutations: delete lease_id; delete plan generation; delete dialogue memory_epoch; set new_memory_epoch0; upper case UUID; contract_versionv2; inject snapshot_id into get_planning_context_input; unknown step kind; place without block; follow before last step; empty plan steps. Include the fixture context for terrain result/input correspondence.

Check manifest endpoints/identity profiles and exactly six MCP tools: get_planning_context,list_affordances,inspect_inventory,find_visible_blocks,query_terrain,validate_plan. Reuse the existing manifest-consistency helpers/tests; do not reimplement HTTP dispatch, planning or Python schemas. For actual DTO parsing, reuse the mapping from `agent_manifest_contract_test.go::TestAgentContractGoldenDrivesActualDTOCodecs`; for tool-input validation, reuse `planningToolLease` and `ExecutePlanningTool` as in `TestPlanningToolsConsumeValidMachineFixtures`. These calls use an immutable test snapshot and in-process functions, no listener or Agent service. No world actions are submitted.

The source owner of these two families remains companion-agent-service. Their manifest consumer is explicitly `external:agent-contract`, plus Go test target, rather than a fictitious Rust Agent runtime. Rust acceptance validates report identity/content and carries this independently executed Go contract evidence; it does not claim Rust Agent behavior parity. All Rust-owned codec/domain/kernel families still require actual Rust consumers. Unsupported schema/rule names fail the existing validator and coverage gate; they cannot be skipped.

Run `go test ./packages/shared/companion -run 'TestRuntimeAgentContractOracle|TestContractFixtureSchemasValidateGoldens|TestAgentContractGoldenDrivesActualDTOCodecs|TestPlanningToolsConsumeValidMachineFixtures' -count=1`; ORACLE verifies the exported report with `go test ./packages/tools/cmd/runtime-oracle -run '^TestAgentContractOracle' -count=1`. Commit `test(runtime): execute agent contract fixture validation`.

## Integration and exclusions

Every subsequent family node owns its new Go test producer and corpus directory as well as Rust behavior. Only the controller merges `contracts.json`, shared test registrations and Cargo.lock, in node order. This prevents simultaneous workers from regenerating or overwriting the full manifest. Each submitted family supplies a manifest fragment with its stable IDs and reviewed source SHA; 6.1 merges and reconciles coverage. Existing historical binary fixtures are read-only inputs.

No timing threshold, file-count assertion, self-round-trip or OpenSpec completion flag can close an evidence node. Full coverage waits for domain, protocol, storage and numerical consumers and is therefore not a prerequisite for those consumers.
