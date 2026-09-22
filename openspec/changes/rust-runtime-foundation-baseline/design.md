## Context

See `proposal.md` for motivation and
`specs/rust-runtime-foundation/spec.md` for observable behavior. The extracted
implementation spans Go offline tooling, package-local Go producers, Rust
test-only corpus support and the dependency-free `mornlea_domain` crate. At the
planning baseline, focused tests pass, but independent reviews proved that
several accepted paths validate names or copy expected values instead of
executing the claimed behavior.

This change repairs and accepts only nodes 1.1–1.6 and 2.1–2.10 from the
superseded parent. It does not advance the remaining event, packet, storage or
kernel frontier.

## Goals / Non-Goals

**Goals:**

- Give in-progress manifests a truthful partial status while making complete
  acceptance fail closed on any uncovered family, version or consumer.
- Ensure traces and domain corpus tests execute actual operations independently
  of committed expectations.
- Make Go and Rust apply equivalent corpus identity and path-integrity rules.
- Remove every known tracked-corpus mutation path from test execution.
- Bound public domain validation before proportional work or temporary
  allocation.
- Preserve all externally observable protocol, save, ABI and online-runtime
  behavior.

**Non-Goals:**

- Completing nodes 2.11–2.14 or any protocol, storage, numerical, pathfinding or
  final acceptance successor.
- Adding a production serialization dependency, a runtime fallback, a live
  writer, an online replay path or a new game-format version.
- Regenerating expected values from Rust output or modifying legacy save
  fixtures.

## Decisions

### 1. Validation mode is explicit

Replace the ambiguous single inventory reconciliation entry point with:

```go
type ConsumerKind uint8

const (
    ConsumerRust ConsumerKind = iota + 1
    ConsumerExternalGo
)

type ConsumerRoute struct {
    FamilyID  string
    Version   string
    Operation string
}

type ConsumerRegistration struct {
    Kind   ConsumerKind
    Routes map[ConsumerRoute]struct{}
}

type ConsumerRegistry map[string]ConsumerRegistration

type CoveragePoint struct {
    FamilyID string
    Version  string
}

type CoverageReport struct {
    Covered   []CoveragePoint
    Uncovered []CoveragePoint
}

// NegativeCoverageExceptions names family/version points for which no invalid
// representation exists. Every entry carries a nonempty reviewed rationale.
type NegativeCoverageExceptions map[CoveragePoint]string

func ReconcileWorking(
    root string,
    inventory Inventory,
    discovered []Family,
    live Identities,
    consumers ConsumerRegistry,
    negativeExceptions NegativeCoverageExceptions,
) (CoverageReport, error)

func ReconcileComplete(
    root string,
    inventory Inventory,
    discovered []Family,
    live Identities,
    consumers ConsumerRegistry,
    negativeExceptions NegativeCoverageExceptions,
) (CoverageReport, error)
```

Both paths validate identities, source provenance, every present case, supported
case version membership and the consumer registry. Working mode permits a
family/version's case set to be empty and returns it in the sorted
`Uncovered` slice; it never labels the result complete. A point enters sorted
`Covered` only when its validated expected outcomes contain at least one
`kind: "ok"` and, when an invalid representation is applicable, one
`kind: "error"`. A point may omit the negative only when it appears in
`NegativeCoverageExceptions` with a nonempty reviewed rationale; an unknown
point or empty rationale is itself invalid. Complete mode returns the same
report but additionally rejects every uncovered point. The baseline consumer
registry contains exactly the following executable routes; name membership
without an exact family/version/operation route never counts as coverage:

- `corpus_frame`: `protocol.frame/45/decode`;
- `mornlea_domain`: `domain.identity_values/current/admit`,
  `domain.values/current/admit`, `domain.command_control/current/admit`,
  `domain.command_inventory/current/admit`, and `domain.event/1/admit`;
- `external:agent-contract`: `agent.http/v1/agent-contract` and
  `agent.mcp/v1/agent-contract`;
- `external:runtime-authority`: `domain.input/45/order`.

The last route owns the one engine-step case whose admission/effect fields
cannot be produced by `mornlea_domain::order_commands`. The validator rejects
unknown kinds, empty route sets, and a known consumer used on an unregistered
family, version, or operation in both working and complete modes. Successor
changes extend this closed registry only together with a real test dispatch.
`BaselineNegativeCoverageExceptions()` is empty; a successor must add and
review an exception together with the representation that makes rejection
inapplicable.

The CLI calls `ReconcileWorking` and prints both covered and uncovered
family/version counts.
Only the later final acceptance gate calls `ReconcileComplete`. Keeping the
partial mode is necessary because the baseline deliberately inventories future
families; treating partial inventory as complete or making future families
disappear are both invalid alternatives.

### 2. Trace execution and validation never read expectations as observations

Remove the production `RunTrace` constructor that reads expected files. Test
operation runners execute a closed `map[string]GoOperation`; their results are
converted into `ExecutedObservation` values and then assembled into a trace:

```go
type ExecutedObservation struct {
    Tick    uint64
    CaseID  string
    Outcome Outcome
}

type TraceRequest struct {
    Seed          string
    TickSchedule  []uint64
    SelectedCases []string
    Coverage      string
}

func BuildTrace(
    request TraceRequest,
    inventory Inventory,
    executed []ExecutedObservation,
) (Trace, error)

func ValidateTraceAtRoot(root string, trace Trace, inventory Inventory) error
func LoadTraceAtRoot(
    root string,
    path string,
    inventory Inventory,
) (Trace, error)
```

`BuildTrace` derives source revision, identities and the canonical inventory
digest from `inventory`; `TraceRequest` no longer carries a repository root or
source revision that could disagree. It validates case selection and exact
checkpoint membership but never opens an expected file.
`ValidateTraceAtRoot` loads and hashes expected values
from the supplied corpus root; every root, stat, read, digest, duplicate-key or
decode failure is returned. No helper falls back to the ambient checkout.
`ExportTrace` validates through the same root-bound API before publication.

### 3. Generation is external and no-replace

All package-local producers continue executing real Go APIs. Their default
output remains memory or `t.TempDir()`. An explicit
`RUNTIME_ORACLE_EXPORT_DIR` names an external export root outside the
repository. Each producer owns one fixed child such as
`runtime-oracle/domain-values` or `companion/agent-contract`; it creates that
child component by component after symlink and containment checks and publishes
files without replacement. Every asset path is validated before any directory
is created. Existing producer-prefix components must be real directories;
symlinks and non-directories are rejected even when their resolved destination
is inside the repository, and the final producer component is created
exclusively. Asset-parent components use the same walk rather than
`MkdirAll`. This permits one test process to export multiple producers without
allowing any producer to replace another producer's result or redirect output
through a fixed-prefix symlink.
Boolean update flags that point at tracked directories are removed. The
already read-only CLI remains read-only.

The Go oracle centralizes this behavior in test support. The companion package
uses a local helper with the same contract because importing the oracle would
violate package direction. Tests build synthetic corpus roots entirely under
temporary storage. A controller may review and copy an exported tree in a later
commit; no test does that merge.

### 4. Rust loader is a strict independent consumer

`runtime_corpus.rs` gains a root-parameterized loader used by negative tests and
the existing repository-root convenience wrapper. A component walk based on
`symlink_metadata` rejects every symlink below the resolved root and verifies
the final resolved path remains contained. A custom serde visitor constructs
JSON values while rejecting duplicate object keys. The domain and protocol
manifests add the already locked serde version as a direct test-only dependency;
production dependency sets remain unchanged. `input_format` is a closed enum
(`binary`, `json`), and consumer identity is matched against the same four
baseline identities rather than accepted for being nonempty.

The Rust loader independently validates the shared manifest/case subset it
consumes: schema and count budgets, unique family and case IDs, family
membership, family case-list equality, supported case versions, case-ID prefix,
closed operation/input-format/consumer values, nonempty decimal checkpoints,
relative contained asset paths, every-component symlink rejection, regular
files, byte budgets, duplicate-free JSON and sha256. Go remains the sole owner
of live registry discovery, current identity reconciliation and source
provenance comparison; Rust acceptance is not described as a substitute for
that Go-only reconciliation.

The loader continues checking file budgets before reads and sha256 after reads.
No production crate depends on serde or sha2; this remains shared integration
test support.

### 5. Domain corpus has one real dispatcher

Add a `corpus_domain` integration target under `mornlea_domain`. A small root
and shared-support node is followed by the eight existing Go producer
boundaries—identity/text, scalar values, control commands, inventory/chat
commands, player events, world events, inventory events and
people events—so no worker owns all 376 Rust-owned cases. It enumerates
all manifest cases whose family is one of:

- `domain.identity_values`
- `domain.values`
- `domain.command_control`
- `domain.command_inventory`
- the currently implemented player, world, inventory and people cases recorded
  under `domain.event`

The test requires both manifest `rust_consumer == "mornlea_domain"` and input
JSON `consumer == "mornlea_domain"` before invoking a topic executor, dispatches
by a closed operation/topic tag, constructs the actual Rust value or calls
the appropriate checked constructor, and normalizes the result to the corpus
JSON rules. For accepted command-control and command-inventory cases, the
domain comparator removes only `fields.wire` from the independently produced
Go expectation; protocol successors own those bytes. Every selected ID executes
exactly once. Unknown topic or case shape fails rather than being skipped. A
private mutation test changes one normalized semantic field after real
execution and proves the comparator rejects it.

The frozen `domain.input` case is corrected from `mornlea_domain` to
`external:runtime-authority`: its expectation contains authoritative admission,
deduplication and world effects that a pure orderer neither owns nor can
produce. The existing Rust `command_order` suite remains the acceptance gate
for `(tick, session, sequence, arrival_index)`, duplicate-arrival rejection and
caller-owned scratch. No task fabricates those engine effects in the domain
crate.

The existing handwritten topic tests remain valuable boundary tests, but they
are not counted as independent Go/Rust corpus evidence.
The exact per-rule input, constructor, normalization and rejection precedence
is frozen in `plans/03-domain-adapters.md`; a worker reports contrary source
evidence instead of inventing an adapter policy.

Float input accepts a decimal grammar with an optional sign, decimal point and
`e`/`E` exponent whose exponent may itself be signed. At least one mantissa
digit and, when present, one exponent digit are required. The only intentional
non-finite spellings remain `NaN`, `Inf`, `+Inf`, and `-Inf`; parsed decimal
overflow is rejected and negative-zero bits are preserved.

### 6. Domain work budgets preserve multi-frame semantics

Add these public error categories and shared bound:

```rust
pub const MAX_SEMANTIC_BATCH_RECORDS: usize = 4096;

pub enum DomainError {
    // existing variants
    NonFiniteValue,
    BatchTooLarge,
    Allocation,
}
```

`FiniteVec3` returns `NonFiniteValue`; `LookAngles` retains
`NonFiniteRotation`. Display-name admission checks the 128-byte limit before
iterating scalars. A private display-name scanner accepts a nonescaping
per-scalar observer; production passes a no-op and a same-module unit test
proves that an oversized byte input invokes the observer zero times. This is a
testable ordering seam, not a public hook. `BlockChanges`, `ForgetChunks`, `RemotePlayerStates` and
`CompanionStates` reject a length above 4096 before a scan or copy.
`ForgetChunks` checks the bound, calls `try_reserve_exact`, copies into reserved
scratch, sorts and checks duplicates; reserve failure maps to `Allocation`.

The 4096 bound is intentionally above the remote-player and companion wire caps
so an authority can retain a bounded semantic collection before packetization.
Using the wire caps in domain would reverse the selected ownership; leaving the
collections unlimited would violate the resource-bound requirement.

### 7. Ownership and integration

The Go oracle owns inventory/trace policy. Package-local producers own access to
unexported Go behavior. Rust shared test support owns corpus parsing and
integrity. `mornlea_domain` owns semantic validation and normalized execution.
No task adds production imports across these boundaries.

Tasks execute serially where files or one crate root overlap. After the corpus
root and support module land, each producer-boundary module has exclusive file
ownership, but those nodes still integrate serially in the shared checkout; a
later closure node alone edits the root coverage assertion. The controller owns
the frozen manifest, architectural decisions, task status, ledger, integration
and archive. A linked node's editable-file list grants one worker temporary
exclusive ownership of those files, including shared test support or public
exports; no other worker edits them until review and commit. Workers do not
update status. The initial six repair areas decomposed into fifteen bounded
implementation nodes; final review adds thirteen bounded repair/retrospective nodes
before a renewed controller closeout. Each node receives one
exclusive file set, one behavioral red/green cycle, one independent code
review, one scoped implementation commit, and one controller-only status
commit that records the now-known implementation SHA in `ledger.md` and marks
the checkbox. The two-commit sequence avoids a self-referential commit SHA and
leaves a clean accepted baseline before the next worker starts.

### 8. Final-review repair and evidence policy

The first closeout attempt is not acceptance evidence because its required
stage gates were red. The dated archive is restored to the active change and
the premature canonical spec is removed until every repair and required gate
passes. Historical commits remain intact; the ledger records the correction
rather than rewriting history.

The Agent HTTP/MCP corpus remains real package-local execution evidence in
`packages/shared/companion`. The runtime-oracle package does not publish an
"executed" Agent trace until it has a callable operation; copying committed
expectations into `ExecutedObservation` is removed rather than relabelled.
Likewise, the server command-order suite remains read-only comparison evidence:
its tracked update flag and ad-hoc export writer are removed. A repository-wide
source audit enumerates runtime-migration producer tests and rejects their
future Go test flags that advertise update, rewrite, or regeneration. Separate
storage/protocol golden-fixture workflows remain outside this corpus rule.

JSON case inputs use the 256 KiB JSON budget in Go as they already do in Rust;
binary inputs retain the 4 MiB binary budget. Manifests, provenance sources,
inputs, expectations, and encoded assets must be regular files before hashing
or reading, so a FIFO or directory cannot turn validation into blocking I/O.

`docs/notes/godot-client-pilot-report{,.zh}.md` is immutable measured evidence
for protocol v44 and is classified as `historical`. It is not edited to claim
the current protocol version. The report-completeness test pins the measured
identity, while current-version gates exclude the historical classification.

`tasks.md` remains the sole plan identity and status source even when it links
packet files. The change ledger is the only durable execution record; a flat or
packet-keyed `.superpowers/sdd` progress file is not an alternative status
store. A required closeout failure keeps the node open unless the plan and its
acceptance contract are explicitly revised before archive. Requirements using
"every" or "no path" include a repository-wide producer enumeration in their
worker packet and review focus, not only the files originally assigned to one
worker.

Editable-file ownership also includes derived consumers. Before dispatch, the
controller enumerates hashes, generated or embedded artifacts, and source
scanners that consume every editable file, even for comment-only work. The
packet assigns any reviewed refresh, its exact authority and algorithm, and the
downstream consumer gate. Final review repeats the dependency enumeration;
focused package tests do not prove that provenance or generated consumers are
current.

## Risks / Trade-offs

### Final-gate mining harness repair

The final non-race short suite exposed a latent mining fixture defect: the
grass surface admits passive spawning and grazing, so different login-ready
tick windows can add an unrelated authoritative dirt update to the strict
mining completion frame. Keep that four-message assertion unchanged. The
mining generator retains `integrationChunk` and its central stone target, but
replaces every y=0 surface cell with dirt before adding the two side targets.
Do not filter extra block updates, disable production simulation, or retry a
failed gate until it happens to pass.

The same harness only shuts down its host on the success path. Introduce a
file-private host constructor that registers bounded `t.Cleanup` immediately
after creation; keep the explicit successful shutdown and persistence checks.
Register endpoint closure after login. Exercise normal return and `SkipNow`
(the same `Goexit` cleanup path used by `Fatal`) in nested subtests, requiring
the actual host runtime and closed channels to be closed after the child ends.
The existing global shutdown-goroutine assertion stays unchanged.

This is test-only acceptance repair, not a new runtime boundary. Root and
`packages/server/AGENTS.md` govern the existing directory; no new guide or
architecture skill rule is required.

- A complete gate cannot pass until successor families land → keep complete
  acceptance separate and test its failures now; do not call it from the
  baseline CLI.
- The domain dispatcher is verbose → prefer explicit topic matches over a
  reflection-like generic interpreter that could reproduce the Go producer's
  mistake.
- A 4096-record semantic cap is looser than current packet caps → retain tighter
  protocol validation and test both boundaries separately.
- Duplicate-key rejection requires custom test-only parsing → confine it to the
  shared corpus helper and avoid a production dependency.
- Removing update flags changes developer workflow → preserve explicit external
  export and document controller-reviewed import rather than allowing tracked
  writes.

## Migration Plan

1. Land inventory-mode and trace execution repairs before changing consumers.
2. Land isolated generation and Rust loader parity, then rerun framing and Agent
   evidence.
3. Land the executable domain corpus and prove nonzero discovery plus mutation
   failure.
4. Land domain work bounds and rerun all domain/producer suites.
5. Repair the final-review findings in closed consumer routing, truthful
   execution evidence, export containment, Rust input parsing, and historical
   documentation identity.
6. Clear the English-comment stage gate in three non-overlapping source groups
   and ratchet only the resulting decrease.
7. Promote the verified orchestration lessons and record the invalidated
   closeout attempt without rewriting history.
8. Refresh the six provenance rows invalidated by reviewed source changes,
   repair duplicate-family working manifests and stale corpus workflow prose,
   then promote the derived-consumer planning rule.
9. Repair final-gate mining fixture isolation and failure-path host cleanup,
   preserving strict transcript and shutdown oracles.
10. Run the integrated baseline gates, independent review and strict OpenSpec
   validation at one result SHA.
11. Sync the narrow delta into the canonical specification and archive this
   baseline change. Keep the existing
   `2026-09-21-rust-runtime-foundation` historical archive unchanged and do not
   sync its remaining successor scope.

Each implementation node rolls back independently. Rolling back this change
does not alter the default Go runtime, any save, protocol/ABI version, or an
accepted historical fixture.
