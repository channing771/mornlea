## Context

See `proposal.md` for motivation and `specs/zcode-live-agent-supervision/spec.md` for observable behavior. The synchronized `adaptive-model-router` skill packages own a synchronous Node bridge in `scripts/zcode-agent.mjs`; each `run` or `send` invocation launches the packaged Z Code CLI, waits for final JSON, and exits. The installed CLI also exposes a long-lived stdio `app-server` with session events and v4 conversation commands, but that surface is application-internal and may change between Z Code releases.

Codex tool calls are request/response operations. An external process cannot inject an unsolicited model turn into an idle Codex task. Native-like interaction therefore requires a bounded wait call that remains pending until a material worker event arrives, analogous to a long-polling task wait.

The bridge is developer tooling, not game runtime code. It must preserve user-owned worktrees and desktop configuration, keep credentials process-local, and count each running Z Code worker against the project subagent ceiling.

## Goals / Non-Goals

**Goals:**

- Keep a Z Code task alive across separate controller commands and preserve session/event identity.
- Give the controller bounded progress, status, safe intervention, cancellation, and cleanup operations.
- Make retries cursor-safe and filter token-level noise before it enters the controller context.
- Fail closed when the installed private protocol no longer matches the adapter contract.
- Preserve existing synchronous callers and provide deterministic fake-server tests without paid inference.

**Non-Goals:**

- Register `GLM-5.3` in the native Codex model enum or claim native lifecycle integration.
- Wake a Codex model that has not issued a pending wait or configured a separate automation.
- Auto-approve permissions, resolve user-input requests, or allow two editing agents to share ownership.
- Generalize the supervisor into a provider-neutral daemon in this change.
- Persist credentials, raw reasoning streams, or unbounded transcripts.

## Decisions

### Use a local supervisor daemon with request/response clients

The selected `adaptive-model-router` skill's `scripts/zcode-agent.mjs` is the public command entry point. It imports its live supervisor relative to its own module, so either the Codex or Claude skill package is independently executable without a project-root wrapper. Live commands connect to a detached local Node supervisor over a user-private Unix-domain socket. The client starts the supervisor on demand, waits for a readiness handshake, sends one length-bounded JSON request, and prints one JSON response. The daemon owns worker child processes and remains alive across commands.

State lives below a user-private runtime directory derived from the platform temporary directory and numeric user identity, never in the repository. The directory contains only the socket, daemon metadata, and sanitized non-secret worker metadata needed for recovery. Files are created with user-only permissions. Worker records are keyed by opaque random identifiers rather than Z Code session IDs.

Supervisor startup is serialized by an exclusive-create lock held across stale-socket verification, unlink, and bind. A competing starter waits for the lock owner to publish a responsive socket and never unlinks a socket it did not win the right to recover. An abandoned lock is removed only after its recorded process is confirmed absent.

Alternative considered: require Codex to keep one foreground CLI process open through terminal session APIs. That avoids a daemon but couples the bridge to one host tool implementation, makes independent status calls awkward, and cannot support ordinary command invocations. Alternative considered: one app-server for every command. That loses the active process and event subscription needed for mid-turn steering.

Alternative considered: keep a single bridge under root `scripts/agents/`. That centralizes executable code but makes the skill depend on repository layout and leaves its runtime outside the package that defines and invokes it. Mirroring the small standard-library-only runtime with the synchronized skill is the clearer ownership boundary, and audit tests prevent the two copies from drifting.

### Own one app-server child per live worker

Each worker launches one packaged `app-server` child with credential and endpoint values in the child environment. The worker adapter serializes writes, assigns monotonically increasing request IDs, parses newline-delimited JSON frames with a maximum size, and routes responses and notifications separately. One child per worker gives clear ownership and fault isolation; the project ceiling bounds the process count.

The daemon launches each app-server through a minimal Node worker wrapper whose command line carries a random process-identity token. Sanitized metadata stores the wrapper PID and token. After a daemon restart, a stale close may signal that PID only when the current process command still names this module's worker mode and the exact token; otherwise it reports an identity mismatch and leaves the process untouched. Normal and recovered close paths retain metadata until wrapper exit is verified. The wrapper propagates normal close signals to its app-server child and, after a bounded grace period, force-terminates only that owned child. A cleanup timeout remains retryable and does not fabricate a closed state.

The observed transport uses newline-delimited `{id, method, params}` requests, `{id, result}` or `{id, error}` responses, and method notifications without a `jsonrpc` member. The child starts as `node zcode.cjs app-server --cwd <absolute-path> --surface terminal`. The adapter first performs side-effect-free behavioral probes such as `session/list` plus intentionally invalid `session/subscribe` and `v4/command` requests; any recognized-method validation response proves method presence while `-32601` rejects the live route. It then creates the upstream session with `session/create`, including the selected `thoughtLevel`, subscribes once with `session/subscribe` using `deliveryKind: "desktop-continuous"`, `afterSeq`, and `includeSnapshot`, and accepts subsequent `session/event` notifications. If the required contract is absent, creation fails and the child is terminated before a prompt is sent.

Alternative considered: share one app-server among all workers. That reduces process overhead but makes one malformed stream or child exit affect unrelated workers and complicates ownership and recovery. The existing two-worker ceiling makes per-worker isolation acceptable.

### Normalize upstream events into a bounded bridge event model

The supervisor maintains two sequences:

1. the upstream Z Code event sequence used to avoid re-reading protocol events; and
2. a bridge cursor covering only normalized material events returned to Codex.

Text, reasoning, and tool-input deltas update an in-memory rolling summary but do not each create a bridge event. Lifecycle changes, turn completion/failure, tool failure, permission requests, user-input requests, steering acknowledgement/drain, checkpoints, child exit, and protocol failure create material events. The retained ring buffer has explicit event-count and byte limits; a cursor older than retained history returns a cursor-expired error with the earliest available cursor rather than silently skipping events.

Wait requests register against a worker and cursor with a maximum deadline. They complete on the first new material event, terminal state, or timeout. Only one mutation runs per worker at a time; multiple read-only status or wait clients may coexist. Closing a worker completes pending waits before removing control state.

Alternative considered: forward every upstream event. That provides maximum detail but imports raw reasoning and token-level noise into the controller, defeats context isolation, and creates excessive IPC traffic.

### Map interventions to explicit v4 delivery modes

Live intervention uses upstream `v4/command` with `type: "sendText"`, a caller-visible retry-stable `commandId`, and `payload.requestedDelivery` because that field distinguishes `guide`, `queue`, and `startNow`. Mutations run through a per-worker FIFO, and an upstream duplicate acknowledgement is returned as a successful deduplication rather than resent with a new identifier. The bridge never maps the legacy `session/send` behavior onto those modes because legacy send rejects active prompts and cannot represent all three semantics faithfully.

The adapter returns acceptance separately from eventual delivery. A `guide` acknowledgement means the upstream accepted guidance for a safe boundary, not that the model has consumed it; `turn.steerQueued` and `turn.steerDrained` expose later progress. `queue` and `startNow` likewise remain observable through subsequent material events. Stop uses `session/stop` and preserves session identity. Close uses `session/close`, which releases the resident runtime without deleting the persisted Z Code conversation, then terminates the child and removes bridge-owned state.

Alternative considered: implement guide by cancel-and-resume. That changes task semantics, can discard tool state, and falsely reports safe-boundary steering.

### Recover conservatively and version-gate the private protocol

Sanitized metadata records working directory, mode, provider/model identity, upstream session identity, last acknowledged sequences, lifecycle state, and process birth identity. It never records prompts, credentials, raw reasoning, or tool payloads. After daemon restart, a worker is recoverable only if the upstream session can be resumed and subscribed with the required contract; otherwise status reports unavailable and requires an explicit new worker or synchronous follow-up.

Protocol compatibility is established by behavior and required method support rather than trusting only a CLI version string. The current observed CLI version is still recorded in diagnostics so failures can be correlated. Tests drive the adapter with a fixture app-server implementing success, malformed-frame, missing-method, sequence-gap, active-turn, timeout, and child-exit cases.

Alternative considered: pin only the observed Z Code version. A version check is useful diagnostics but cannot detect repackaged or partially compatible builds; a handshake gives a stronger failure boundary.

### Keep routing scores as policy, not model truth

The synchronized router skills will add a deterministic scoring rubric: task/capability fit 35%, validation strength 20%, control integration 15%, context-isolation benefit 15%, total token/quota efficiency 10%, and startup latency 5%. These are routing policy weights, not measured accuracy probabilities.

`GLM-5.3` receives a conservative prior: `low` for mechanical or read-heavy tasks, `high` for ordinary bounded implementation, and `max` for long-context implementation with strong validation. High-consequence or weak-oracle work continues to prefer eligible OpenAI models until repository-specific evaluation shows otherwise. Small tasks remain in the controller or a lower-bootstrap native worker. Live supervision may improve the control-integration component but cannot erase capability or validation risk.

Alternative considered: encode vendor benchmark ranks as fixed model order. Benchmark suites and harnesses do not measure the repository's complete task distribution, so fixed equivalence would be overconfident and quickly stale.

## Risks / Trade-offs

- **Private Z Code protocol changes** → Fail the handshake, preserve synchronous commands, include contract fixture tests, and report the installed CLI identity in sanitized diagnostics.
- **Detached daemon or worker leak** → Bind without unlinking an active socket, track the worker wrapper's PID and random birth token, terminate only a process whose current command matches that identity, and reject mismatched stale metadata rather than signaling an unrelated process.
- **Lost or duplicated progress after retry** → Use monotonic bridge cursors, a bounded retained ring, and explicit cursor-expired errors.
- **Controller context pollution** → Coalesce streaming deltas and cap every snapshot, event batch, diagnostic, and error.
- **Credential leakage through child diagnostics** → Redact before buffering, persistence, or output; never place credentials in arguments or metadata.
- **Concurrent editing conflict** → Require an explicit ownership assertion for editing modes and retain controller responsibility for worktree isolation.
- **Preemption causes partial edits** → Expose `startNow` as destructive to the active turn, preserve ordered stop/replacement events, and require controller-side validation before integration.
- **Daemon is not a native Codex tool** → Document command contracts in the router skill; a future MCP wrapper can reuse the same supervisor protocol without changing worker semantics.

## Migration Plan

1. Add fixture-driven protocol and supervisor tests inside the synchronized router skill packages while the synchronous bridge remains unchanged.
2. Add live commands behind the capability handshake and retain `probe`, `run`, and `send` compatibility tests.
3. Update both project router copies only after focused tests pass, then validate their byte equality.
4. Run a read-only local smoke test against the installed Z Code app-server without sending model inference where possible; any paid live task remains explicit and bounded.
5. Roll back by removing the live commands and daemon from both synchronized skill packages while leaving the synchronous bridge intact. Runtime state is transient and can be closed without a data migration.

No save, gameplay network protocol, schema, ABI, or benchmark migration is required.
