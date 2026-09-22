## MODIFIED Requirements

### Requirement: Python is the primary Godot feature language behind a native bootstrap

The Godot client SHALL implement host lifecycle, feature orchestration, desktop adaptation, and ordinary presentation scripts primarily in typed Python through one exactly pinned, project-hardened derivative of the community Py4Godot GDExtension and an isolated interpreter. The derivative MUST be reproduced from an exact upstream source revision and a minimal repository-owned patch stack limited to interpreter isolation, editor integration, packaging, and lifecycle correctness. It MUST NOT expose project gameplay/data APIs or become a second bridge. Production GDScript MUST be limited to the minimum bootstrap and setup-diagnostic path required to open the project without that extension. Rust SHALL retain project-owned native bridging and performance-sensitive Godot resource work, and the current pilot Go core SHALL retain session, protocol, mirror, prediction, and presentation-semantic ownership only until its accepted Rust client-core replacement. After that replacement, Rust SHALL own those responsibilities and the Godot bridge MUST NOT load the pilot Go core; Python presentation and the migration diagnostic Bootstrap remain in their qualified scopes. A failure to qualify the Python runtime MUST stop the Python-primary pilot and MUST NOT authorize a GDScript feature rewrite.

#### Scenario: A prepared project hands control to Python

- **GIVEN** Bootstrap has verified the exact Godot version, Python extension, interpreter identity, project bridge identity, and desktop target
- **WHEN** the developer runs the main scene
- **THEN** Bootstrap SHALL dynamically hand control to the Python host, which assembles the declared feature catalog
- **AND** host, feature, and desktop-adapter behavior after that handoff MUST be implemented in Python unless an approved native hot-path boundary applies

#### Scenario: The Python runtime is missing or incompatible

- **GIVEN** the Python extension, bundled interpreter, standard library, or declared runtime identity is missing, modified, or incompatible
- **WHEN** Bootstrap validates the distribution unit
- **THEN** Bootstrap SHALL remain in the pure-GDScript setup-diagnostic path without importing a Python feature or starting a network session
- **AND** the pilot MUST NOT fall back to a GDScript feature host or advertise gameplay readiness

#### Scenario: A production GDScript feature is introduced

- **GIVEN** a new production script outside the explicit bootstrap and setup-diagnostic allowlist uses GDScript for host, feature, UI, or platform behavior
- **WHEN** project architecture validation runs
- **THEN** validation MUST fail and identify the script and allowed ownership boundary
- **AND** the change MUST either move the behavior to Python, justify a Rust native hot path through architecture review, or revise this specification explicitly

#### Scenario: Python attempts to bypass the project bridge

- **GIVEN** Python code imports a direct Go/client-core C binding, calls the engine ABI, imports the standalone companion Agent service, or loads a system library to reach those implementations
- **WHEN** source, dependency, or distribution validation runs
- **THEN** validation MUST fail before the pilot is packaged
- **AND** Python SHALL consume client data only through bounded Godot-visible methods, signals, and value objects exposed by `MornleaClientBridge`

### Requirement: Cross-runtime boundaries publish only bounded atomic snapshots

Inputs, events, world deltas, and frame snapshots crossing between the Rust client core (or the transition-only Go pilot core before its accepted replacement), Rust bridge, Python host, and Godot presentation runtime SHALL carry an explicit layout version, epoch or revision, and length and count limits. If any version, length, capacity, reserved-field, ordering, or content validation fails, the receiver MUST reject the entire batch and preserve its pre-call state. Python MUST NOT retain native buffer views or mutable sender-owned data after a bridge call returns.

#### Scenario: A valid batch is published atomically

- **GIVEN** a version-matched batch within capacity, with a strictly valid revision and valid records
- **WHEN** the receiver applies the batch
- **THEN** the observable state in the batch SHALL become visible as a whole
- **AND** the receiver MUST NOT retain a mutable-memory reference owned by the sender after the call returns

#### Scenario: A record in the middle of a batch is invalid

- **GIVEN** a middle record in a multi-record batch contains invalid coordinates, an invalid length, a stale revision, or an out-of-range resource count
- **WHEN** the receiver validates the batch
- **THEN** the entire batch MUST fail
- **AND** records already validated at the beginning MUST NOT leave partial presentation or cache state

#### Scenario: Output capacity is insufficient

- **GIVEN** the caller's output buffer cannot contain the complete event or snapshot
- **WHEN** the producer prepares to write the result
- **THEN** the producer MUST return a decidable capacity failure and the required capacity or stable-limit information
- **AND** it MUST NOT truncate the result and report apparent success
