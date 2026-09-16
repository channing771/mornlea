# Change Ledger

## Orchestration

- Execution shape: mixed. The controller owns artifacts and implementation; a read-only native reviewer inspects the private Z Code protocol because that bounded binary-analysis context would otherwise pollute the controller.
- Routing decision: `model: gpt-5.6-terra`, `effort: high`, `backend: native`, `basis: medium-high protocol reasoning, moderate consequence, narrow read-only scope, no paid inference, fixture-verifiable output`, `fallback: gpt-5.6-sol high if literal protocol evidence remains contradictory`.

## Progress

- Created the change proposal, behavioral delta specification, and technical design.
- Read-only protocol review verified Z Code CLI `0.16.5`, NDJSON request/response framing, legacy `session/create` and `session/subscribe` lifecycle events, v4 `sendText` delivery modes, and legacy stop/close semantics without model inference.
- Implemented the protocol adapter and persistent local supervisor with fake app-server coverage. Controller review added sequence-gap rejection, status replay, terminal wait wake-up, exact-secret redaction, and diagnostic-only idle shutdown.
- Added public `live-probe`, `start`, `wait`, `status`, `steer`, `stop`, and `close` commands while retaining the synchronous command result shapes.
- Updated both synchronized router copies with deterministic scoring, conservative `GLM-5.3` routing bands, live intervention semantics, and progress filtering.
- Added atomic supervisor startup ownership so concurrent recovery cannot unlink a newly bound live socket.
- Added verified normal and stale worker termination. Metadata remains until wrapper exit is confirmed, cleanup timeouts remain retryable, and the owned wrapper force-terminates only its unresponsive child after a bounded grace period.
- Corrected runtime ownership after user review: moved the bridge, supervisor, and focused tests from root `scripts/agents/` into `scripts/` resources owned by both synchronized `adaptive-model-router` skill packages. Router commands now resolve the selected skill root, so either package is independently executable.

## Validation

- The initial public-command red state used the pre-packaging path: 4 existing synchronous tests passed and 3 new live-command tests failed because the public commands were not implemented yet.
- `node --check .codex/skills/adaptive-model-router/scripts/zcode-agent.mjs` and `node --check .codex/skills/adaptive-model-router/scripts/zcode-live-agent.mjs`: passed.
- `node --test .codex/skills/adaptive-model-router/scripts/zcode-agent.test.mjs .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs`: 35 passed, 0 failed after final lifecycle review fixes; the live-supervisor subset contributed 28 passing tests.
- The skill-local `scripts/zcode-agent.mjs probe`: available `builtin:bigmodel-coding-plan/GLM-5.3`, default reasoning `max`, supported levels `low`, `max`, and `high`; credential presence reported without disclosure.
- The skill-local `scripts/zcode-agent.mjs live-probe --cwd /Users/chen/work/mornlea`: compatible `zcode-live` app-server contract, CLI identity `zcode.cjs`; no session or model prompt created.
- Router `cmp` checks: all three `.codex` and `.claude` files matched byte-for-byte.
- `go test ./packages/audit -run 'Test.*Orchestration|Test.*Governance|TestProjectAdaptiveModelRouter|TestProjectRouter' -count=1`: passed.
- Repository formatter discovery found no Node formatter configured for the skill scripts; no formatter was applied. `git diff --check` passed.
- `make dev-check`: vet completed, but the short-test phase failed in unchanged `packages/server/server` at `TestWarpParityMemoryVsTCP`; the TCP path rejected passive spawn dimension `1` and cleanup reported `unsupported passive dimension 1`. A focused `go test ./packages/server/server -run '^TestWarpParityMemoryVsTCP$' -short -count=1` reproduced the same out-of-scope failure.
- `make test-race`: passed the Rust release build and all six Go module loops. The Go tool reused cached package results, including `packages/server/server`, so this successful command does not invalidate the uncached short-test failure above.
- `openspec validate --all --strict --no-interactive`: 115 passed, 0 failed.
- Skill-runtime placement audit red state: the focused audit failed on all four missing skill resources, all four root-level legacy copies, and the old router path before migration.
- Post-migration focused suites: 35 Node tests passed; the router placement/synchronization/policy audit passed; both skill packages passed `quick_validate.py` in an ephemeral `uv --with pyyaml` environment.
- Recursive Codex/Claude skill comparison passed, no `scripts/agents/zcode-*.mjs` reference or copy remained, and both inference-free installed-CLI probes passed from the Codex skill package.

## Review

- Every live-worker lifecycle scenario is covered by validation of exact provider/model/reasoning resolution, pre-inference input checks, editing ownership, stable worker/session identity, and synchronous-command compatibility.
- Every progress scenario is covered by cursor ordering, bounded event count and aggregate bytes, delta coalescing, timeout snapshots, sequence gap/regression rejection, status replay, and terminal wait wake-up.
- Every intervention scenario is covered by explicit `guide`, `queue`, and `startNow` delivery, stable caller command IDs, upstream duplicate acknowledgement, per-worker mutation ordering, and no automatic approval path.
- Every stop, close, and recovery scenario is covered by session identity retention, conservative unavailable recovery, PID/token verification, atomic socket recovery, metadata retention until verified exit, and retryable cleanup state.
- Every credential and compatibility scenario is covered by exact-secret redaction, credential-free metadata, malformed/oversized/unsupported protocol failures, child-exit handling, side-effect-free live probing, and preserved synchronous result shapes.
- Independent native review found and verified fixes for concurrent stale-socket recovery, stale-wrapper exit verification, and retryable normal-close cleanup. The final review reported no remaining P0/P1 findings.

## Rulings

- The live bridge is an external supervised agent, not an extension of the native Codex model enum.
- Controller wake-up requires a pending bounded wait; unsolicited delivery into an idle model turn is outside this change.
- Legacy `session/event` sequence is the primary upstream cursor; v4 is used for mutation commands rather than as a second competing projection stream.
- The Z Code runtime belongs to the skill that defines its routing contract. Project-root wrappers were rejected because they make the skill depend on repository layout and permit implementation drift outside the synchronized package boundary.

## Round-End Retrospective

- Architecture skill: no change. The verified discoveries are local developer-tool lifecycle rules, not stable game architecture boundaries.
- Model router: updated. Both project-owned copies now encode live Z Code discovery and supervision, deterministic 35/20/15/15/10/5 routing factors, conservative `GLM-5.3` effort bands, context-isolation preference, native fallback for high-consequence or weak-oracle work, and self-contained ownership of the bridge runtime.
- Routing outcome: the two bounded native reviews justified `gpt-5.6-terra` at `high`; they found lifecycle defects without importing private-protocol analysis into the controller context. No model escalation was needed.
- Commit boundary: explicit path staging isolates the router skills, their packaged runtime, governance references, focused audits, and this OpenSpec change from unrelated Godot, rendering, visual-baseline, and progress-board worktree changes.
