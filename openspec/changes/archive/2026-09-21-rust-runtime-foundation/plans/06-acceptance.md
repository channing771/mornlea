# Integration and acceptance plan

**Goal:** make completion a reproducible behavioral claim. **Architecture:** controller integrates reviewed family fragments, releases the numerical stage only after foundation contracts pass, then checks the whole dependency graph and corpus. **Tech stack:** existing Go/Rust gates, offline test reports, OpenSpec. **Spec:** every requirement in the foundation delta. **Global constraints:** all five subsystem plans and execution-contract. **Review focus:** a missing consumer/checkpoint or changed scalar must fail even if every file and task checkbox exists.

These are controller integration/review nodes, not opportunities to assign missing architecture to workers. They have exact dependencies in tasks.md and no new gameplay behavior. Each implementation node's focused tests must already pass at its result SHA before its acceptance evidence can be reused. Test registration alone does not qualify.

<a id="node-6-1"></a>

## 6.1 — Merge domain/protocol/storage evidence

Dependencies:1.6,2.14,3.69,4.7,4.8,4.9,4.10,4.11,4.12. Own `testdata/runtime-migration/contracts.json`, `packages/tools/cmd/runtime-oracle/inventory_test.go`, foundation ledger. Merge reviewed manifest fragments without overwriting legacy source fixtures. Expected sets:59 packets+framing,seven save families with exact version sets in storage-contracts,domain values/input/event transformations,and two externally owned Agent contracts. Numerical families may remain explicitly uncovered until6.3; a focused partial report must list them, never label itself final acceptance.

Reject duplicate packet keys,missing family/version,unknown family,missing compiled Rust consumer,missing producer test,source-only evidence and content digest mismatch. `external:agent-contract` is permitted only for agent.http/agent.mcp with package-local executed Go validator evidence; all Rust-owned families require real Rust consumers. Verify30 semantic events,19 sequenced intents+ChatIntent and the full15-reason mapping. Match all version constants to current code and record any baseline version drift as a blocker, not a silent corpus update.

Run `go test ./packages/tools/cmd/runtime-oracle -run 'TestContractInventory|TestProtocolOracle|TestStorageOracle|TestDomain.*Oracle|TestAgentContractOracle' -count=1` and the package-local producer commands from1.6,2.6,3.3,4.7,4.10. Commit `test(runtime): reconcile foundation contract evidence` only after controller review of generated files and exactSHA.

<a id="node-6-2"></a>

## 6.2 — Foundation contract gate before numerical workers

Dependencies:6.1. Own no production source; record evidence in ledger. Run:

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --test runtime_contract --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test allocation_contract --locked -- --test-threads=1
rustup run 1.97.1 cargo clippy --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --all-targets --locked -- -D warnings
go test ./packages/audit -run 'TestInternalDependenciesAreOneWay|TestNativeEngineBridgeBoundary' -count=1
```

Require nonzero discovery per named topic and actual execution of every corpus case. Inspect public module exports, shared-value ownership, raw-format exceptions, dependencies and error publication. Re-run the original review counterexamples (queue owner,65 companions,furnace-on-air,1200 bank entries,mutatedpacket,login admission,missing ContainerClosed,trace identity/path,command order) plus metadataweather7. No ignored test,allow-warning suppression or result-count claim replaces them. After success,5.1 and5.12 can start. Whole numerical inventory acceptance6.3 is deliberately **not** a prerequisite for those nodes. Record this stage as foundation-contract acceptance only.

<a id="node-6-3"></a>

## 6.3 — Executed differential replay and complete inventory

Dependencies:6.2,5.14. Own new `packages/engine/crates/mornlea_domain/tests/differential_replay.rs`, its topic modules, domain **dev-dependencies only** on protocol/storage/engine, common corpus support registrations and final manifest. Production domain remains dependency-free; protocol/storage production dependency direction remains domain-only plus approved compression.

The runner loads the frozen schema2 manifest, validates content/identity, and matches every selected Rust-owned family to an actual adapter. Execute its declared decode/encode/migrate/admit/order/kernel operation and normalize returned values/errors; compare normalized fields,exact noncompressed bytes,logical compressed bytes and checkpoint completeness. Existing-kernel rows explicitly label `same-core-boundary-parity`; pathfinding labels `independent-Go-algorithm-parity`; Agent rows label `external-contract-validation` and verify an executed producer report, never masquerading as Rust Agent behavior. Every row has a case-level result and source/result SHA. Failed operations count as executed only when category and unchanged-publication assertions match.

Generate Go reports in fresh temporary workspaces using the node producer commands, import reviewed frozen expected files, then run Rust independently without startingGo. Repeated executions of the same bound source/corpus/seed/schedule produce equal normalized reports. Seed0/1 worldgen cases must actually change the relevant output for a chosen nondegenerate fixture; seed-independent codecs are not forced to differ. At this point inventory may no longer contain any unexplained missing family/version/consumer/checkpoint. No full gameplay simulation equivalence is claimed by F1.

Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test differential_replay --locked -- --list` and the same command without `-- --list`; `go test ./packages/tools/cmd/runtime-oracle -race -count=1`. Commit `test(runtime): execute complete differential contract replay`.

<a id="node-6-4"></a>

## 6.4 — Mutation and failure gate

Dependencies:6.3. Own replay/oracle test mutation modules only. In private test copies, independently mutate:remove server/play14 consumer;duplicate full packet key;flip one PlayerState.health;zero one legacy queue owner;change one valid path waypoint;omit scheduled observation;add tick999;duplicate input index;change fixture bytes withoutdigest;fake source SHA;use symlink ancestor plus two missing output components. Every mutation must make acceptance fail with the corresponding case/identity. Restore/copy fixtures through temporary test data, never patch tracked source as part of test execution.

For native API publication, use short-output canaries and reusable scratch across success/failure; for compressed codecs test decoded size lies; for fixed codecs count warm allocations. Rust test target must fail if its adapter match returns a synthetic success instead of executing a consumer. Add a test-only callback that increments an execution counter per case and assert it equals the declared operation schedule, while outcome mutations remain the substantive behavior checks.

Run replay target filter `mutation`; `go test ./packages/tools/cmd/runtime-oracle -run 'TestTrace|TestContractInventoryMutation' -count=1`; protocol allocation and ENGINE native_contract targets. Commit `test(runtime): reject incomplete and falsified compatibility evidence`.

<a id="node-6-5"></a>

## 6.5 — Architecture, guide and ledger review

Dependencies:6.4. Controller owns `ledger.md`, final design/tasks reconciliation, changed scoped guides and synchronized architecture-skill review. Inspect no runtime startup switch/no second online writer/no ABI request blob detour; review allocation/work bounds,raw-format fidelity,unmodified source fixtures,one owner per invariant,and rollback units. Record implementation SHA,nonempty test discovery,actual results,Go andRust corpus identities,known exclusions and remaining failures. A blocked full gate cannot be converted to an informational warning.

Promote only stable verified cross-task architecture rules into both project-owned architecture skills, with source/test citations; otherwise record `Architecture skill: no change`. Keep source/task chronology in this change. Runtime guide updates include engine native APIs,foundation dependency boundaries,oracle test-only imports and shared test support ownership. Do not add an AGENTS.md to every test topic; those inherit their crate guides. Run `go test ./packages/audit -count=1` and strict OpenSpec. Existing baseline doc/comment gate failures must be fixed in the proper scoped change or remain explicit closeout blockers. Commit reviewed coherent documentation separately when it does not overlap user-owned work.

<a id="node-6-6"></a>

## 6.6 — Final stage gates and release evidence

Dependencies:6.5. Run once on the integrated result SHA; repeat only after relevant changes/failures:

```bash
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
make rust-check
make dev-check
make test-race
go test ./packages/audit -count=1
openspec validate --all --strict --no-interactive
```

`make dev-check` includes six-module vet; `make test-race` covers all six Go modules. No foreground window,hook skip,force push or gate exemption. Record exact exit statuses and baseline SHA. All tasks remain unchecked until actual acceptance; planning validation is not implementation evidence. Do not automatically push,merge,archive or switch default runtime. F2/F3 can consume F1 only after complete evidence,not merely because the plan is detailed.

## Requirement and review coverage

| Contract/finding | Implementation | Decisive acceptance |
| --- | --- | --- |
| Callable complete protocol |3.10–3.68,3.4–3.5|6.1 key matrix;6.4 missing ContainerClosed|
| Independent executable evidence |1.1–1.6,all family producers|6.3 actual dispatch;6.4 scalar mutation|
| Inbound login admission |3.3,3.10,3.13|version44 and identity-vs-distance precedence|
| No encoder panic/partial output |3.1,all packet nodes,4.6–4.12|canaries and invalid+short precedence|
| Legacy queue associations |4.1,4.11|fullv2/v3/v4 ID/task/FIFO/summary|
| Companion64/active4 |4.2|64/65 and4/5 boundaries|
| Chunk aggregate associations |4.3|full/logical/schema encode on-air/duplicate|
| Fixed region shape |4.4,4.12|0/1023/1024/1025/1200 cardinalities|
| Raw armor/weather fidelity |4.5–4.7|armor(4242,65,999),weather7,phaseMAX|
| Semantic ordering |2.4–2.6|session/sequence/arrival duplicate winner|
| No digest-only domain events |2.7–2.14,3.5|30 typed events and scalar mutation|
| Trace completeness/isolation |1.3–1.4|tick999,missing checkpoint,deep symlink|
| Ten safe numerical APIs |5.1–5.11|5.14 actual API/ABI and capacity failure|
| Deterministic Rust-native paths |5.12–5.13|exactGo waypoints,all 4 independent transitions,4096/4097|
| Bounded resources/headless |3.69,4.10,5.14|allocation/capacity/work gates;6.6|
