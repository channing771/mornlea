## Purpose

Provide a bounded, credential-safe lifecycle for running Z Code as an observable and steerable external subagent while Codex retains orchestration, ownership, and validation responsibility.

## ADDED Requirements

### Requirement: Live worker lifecycle is explicit and machine-readable

The bridge SHALL allow a controller to start a fresh Z Code worker in an existing absolute working directory with an explicit mode, an optional supported reasoning level, and a non-empty task brief. A successful start MUST return a stable local worker identifier, the provider and model identity, the resolved reasoning level, the Z Code session identifier when available, the current lifecycle state, and an event cursor. The worker MUST remain available for later status, wait, steering, stop, and close operations without replaying its prior conversation.

The bridge MUST reject an unavailable CLI, disabled or mismatched provider, missing model, missing credential, invalid directory, unsupported mode, or empty brief before creating a live worker. An editing mode MUST require the controller to assert isolated-worktree or exclusive-file ownership.

#### Scenario: Controller starts a read-only live worker

- **GIVEN** the configured Z Code CLI, exact provider, exact model, and credential are available
- **WHEN** the controller starts a worker in `plan` mode with an absolute existing directory and non-empty brief
- **THEN** the bridge MUST return a stable worker identifier, model identity, lifecycle state, and event cursor
- **AND** later operations MUST address the same accumulated worker context by identifier

#### Scenario: Controller selects an exposed reasoning level

- **GIVEN** the configured Z Code model exposes `low`, `high`, and `max` reasoning levels
- **WHEN** the controller starts a worker with one of those exact levels
- **THEN** the bridge MUST pass the level to session creation and return it as the resolved reasoning level
- **AND** an unsupported level MUST fail before sending the task brief

#### Scenario: Editing ownership is not asserted

- **GIVEN** the controller requests an editing-capable mode
- **WHEN** it does not assert an isolated worktree or exclusive non-overlapping file ownership
- **THEN** the bridge MUST reject the request before sending the task to Z Code

#### Scenario: Capability prerequisite is unavailable

- **GIVEN** the exact CLI, provider, model, credential, mode, or working directory is invalid
- **WHEN** the controller requests a live worker
- **THEN** the bridge MUST fail without changing Z Code desktop configuration or issuing model inference

### Requirement: Progress waiting is cursor-based, bounded, and meaningful

The bridge SHALL let a controller wait from a previously returned cursor for a bounded duration. The wait MUST return when the worker completes, fails, requests approval or user input, reaches a material checkpoint, reports a tool failure, or produces another controller-actionable lifecycle change. A timeout MUST return the latest bounded snapshot without treating the unchanged worker as failed.

Raw text, reasoning, and tool-input deltas MUST be coalesced into bounded summaries and MUST NOT cause one controller wake-up per token. Returned events MUST be ordered and associated with a monotonically advancing cursor so a retry from the same cursor does not silently lose a material event.

#### Scenario: Material progress wakes the controller

- **GIVEN** a controller is waiting after the latest acknowledged cursor
- **WHEN** the worker requests input, reports a failure, reaches a checkpoint, or finishes the active turn
- **THEN** the wait MUST return the ordered material events and a cursor covering those events
- **AND** the response MUST include a bounded current snapshot

#### Scenario: Streaming deltas remain below the wake-up boundary

- **GIVEN** the worker emits multiple text, reasoning, or tool-input deltas without a material lifecycle change
- **WHEN** the controller waits for progress
- **THEN** the bridge MUST coalesce those deltas rather than wake once per delta
- **AND** the bounded snapshot MAY summarize the latest activity without returning the raw stream

#### Scenario: Wait expires without progress

- **GIVEN** no material event occurs before the requested wait deadline
- **WHEN** the wait expires
- **THEN** the bridge MUST return a timeout result and the latest cursor and snapshot
- **AND** MUST NOT mark the worker failed or advance the cursor past an undisclosed material event

### Requirement: Controller intervention has explicit delivery semantics

The bridge SHALL support three intervention modes for non-empty controller text and a stable caller-provided command identifier: `guide` delivers guidance to the active turn at the next safe boundary, `queue` schedules text for a subsequent turn without interrupting the active turn, and `startNow` preempts the active turn before starting the new instruction. The result MUST identify the command, whether the instruction was accepted, deduplicated, queued, or rejected, and MUST preserve event ordering. A retry using the same command identifier MUST NOT execute the steering intent twice.

An intervention that the installed Z Code protocol cannot represent faithfully MUST fail as unsupported rather than silently falling back to a different delivery mode. Approval and user-input requests MUST remain visible to the controller and MUST NOT be automatically approved by the bridge.

#### Scenario: Controller guides an active turn

- **GIVEN** a worker has an active turn and the protocol supports safe-boundary guidance
- **WHEN** the controller sends non-empty text with `guide` delivery
- **THEN** the bridge MUST submit the text for the active turn without pretending that delivery was instantaneous
- **AND** a later progress event MUST expose whether the guidance was drained or the turn finished first

#### Scenario: Controller queues follow-up work

- **GIVEN** a worker has an active turn
- **WHEN** the controller sends non-empty text with `queue` delivery
- **THEN** the current turn MUST remain active
- **AND** the instruction MUST be retained for the next eligible turn or reported as rejected

#### Scenario: Controller retries an ambiguously acknowledged instruction

- **GIVEN** a steering request used a stable command identifier and its first acknowledgement was ambiguous to the controller
- **WHEN** the controller retries the same intent with the same identifier
- **THEN** the bridge MUST return the upstream deduplication outcome
- **AND** MUST NOT replace the identifier or execute the intent twice

#### Scenario: Controller preempts incorrect work

- **GIVEN** a worker has an active turn
- **WHEN** the controller sends non-empty text with `startNow` delivery
- **THEN** the bridge MUST request preemption before starting the replacement instruction
- **AND** progress MUST expose the resulting stop, replacement turn, or failure in event order

### Requirement: Status, stop, close, and recovery preserve lifecycle truth

The bridge SHALL return a bounded status snapshot without requiring model inference. Stop MUST request cancellation of active work while retaining enough session identity for explicit recovery or follow-up. Close MUST stop owned child processes, reject later mutation commands for the closed worker, and remove transient local control state without deleting Z Code desktop configuration or conversation data not owned by the bridge.

If the local supervisor restarts, it MUST either recover a worker through stored non-secret identity and the Z Code session protocol or report the worker as unavailable with an actionable reason. It MUST NOT fabricate an active or completed state from stale local data.

#### Scenario: Controller stops active work

- **GIVEN** a worker is running an active turn
- **WHEN** the controller requests stop
- **THEN** the bridge MUST request cancellation and return the resulting lifecycle state
- **AND** MUST retain the session identity needed for a deliberate later follow-up when the upstream protocol permits it

#### Scenario: Controller closes a worker

- **GIVEN** a live or stopped worker exists
- **WHEN** the controller closes it
- **THEN** the bridge MUST terminate processes it owns and remove transient control state
- **AND** a later steering request for that worker MUST fail as closed or unknown

#### Scenario: Supervisor restarts during a task

- **GIVEN** a local worker record exists after the supervisor process exits unexpectedly
- **WHEN** a new supervisor serves a status or wait request
- **THEN** it MUST recover from non-secret session identity or report an actionable unavailable state
- **AND** MUST NOT expose credentials or claim progress that cannot be verified

### Requirement: Credentials, protocol compatibility, and output remain safe

The bridge MUST read the enabled desktop provider locally, pass its credential only to the owned child process, and redact the credential from standard output, standard error, persisted state, snapshots, and event summaries. It MUST never copy credentials into repository files or command arguments.

Because the Z Code application protocol is version-sensitive, live operation MUST perform a capability handshake or equivalent contract check before accepting a worker. Unsupported methods, malformed frames, sequence regressions, oversized frames, child exits, and protocol timeouts MUST produce bounded machine-readable errors. The existing synchronous `probe`, `run`, and `send` commands MUST remain compatible.

#### Scenario: Child output contains a credential

- **GIVEN** an upstream diagnostic includes the configured provider credential
- **WHEN** the bridge emits or persists the diagnostic
- **THEN** every occurrence MUST be replaced with a redaction marker
- **AND** no returned event, snapshot, or error MUST contain the credential

#### Scenario: Installed protocol is incompatible

- **GIVEN** the installed Z Code CLI does not support the required live lifecycle or steering contract
- **WHEN** the bridge performs its live capability check
- **THEN** live worker creation MUST fail with a machine-readable compatibility error
- **AND** the synchronous bridge commands MUST remain available when their own prerequisites still pass

#### Scenario: Legacy synchronous invocation remains valid

- **GIVEN** an existing caller uses `probe`, `run`, or `send`
- **WHEN** the live-supervisor capability is added
- **THEN** the caller MUST continue to receive the existing machine-readable result shape and failure semantics

### Requirement: The router skill owns its bridge runtime

Each synchronized `adaptive-model-router` skill package SHALL contain the Z Code bridge, live supervisor, and their focused tests under its own `scripts/` resource directory. The router instructions MUST resolve the executable relative to the selected skill root instead of depending on a project-level agent script. The Codex and Claude skill copies MUST remain byte-identical, and the repository MUST NOT retain a second bridge implementation under root `scripts/agents/`.

#### Scenario: An agent invokes the packaged bridge

- **GIVEN** an agent has loaded either synchronized `adaptive-model-router` skill
- **WHEN** it probes or starts the configured Z Code worker
- **THEN** it MUST invoke `scripts/zcode-agent.mjs` from that selected skill package
- **AND** the bridge MUST resolve its supervisor relative to its own module location
- **AND** no project-root bridge copy MUST be required
