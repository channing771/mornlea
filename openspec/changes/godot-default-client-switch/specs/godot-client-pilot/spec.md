## MODIFIED Requirements

### Requirement: The Godot project root opens directly and is resource-closed

The Godot client SHALL provide a stable project root that a supported Godot Standard editor can directly recognize and open. Scenes, scripts, resources, Python runtime files, extension descriptors, and distributable native dependencies required to run the project MUST resolve through resource paths inside that project root. The system MUST NOT require the repository root to be recognized as a Godot project, or depend on parent-directory traversal, non-portable symbolic links, system Python, user site-packages, runtime package installation, or the system dynamic-library search path to run.

#### Scenario: Open the project before native libraries are prepared

- **GIVEN** a developer has a complete source checkout and a supported Godot Standard editor but has not built the native bridge libraries
- **WHEN** the developer selects the prescribed Godot project root in Godot Project Manager
- **THEN** the editor SHALL recognize and open the project and its diagnostic entry successfully
- **AND** the migration Bootstrap, or the qualified native diagnostic entry after cutover, SHALL display the missing native bridge, Python runtime, target platform, and preparation command without importing feature modules, starting a network session, or pretending to be playable

#### Scenario: Run from the project root after preparation

- **GIVEN** resource synchronization and the native build have produced a complete, version-matched distribution unit containing the pinned Python extension and isolated interpreter for the current platform
- **WHEN** the developer runs the main scene from the same Godot project root
- **THEN** the selected startup entry SHALL resolve the Python runtime, bridge library, and their dependencies only from the project distribution unit and verify their identities
- **AND** after successful verification it SHALL assemble the enabled capabilities without changing the project root; a cutover main-scene change requires equivalent native diagnostics before Bootstrap retirement

#### Scenario: A resource or dynamic-library reference escapes the project root

- **GIVEN** a scene, resource, extension, or dependency declaration contains parent-directory traversal, an absolute development-machine path, a non-portable symbolic link, or a system-search-path dependency
- **WHEN** project-closure checks or export validation run
- **THEN** validation MUST fail and identify the violating reference
- **AND** the build MUST NOT be marked runnable or distributable

#### Scenario: The migration Bootstrap is retired

- **GIVEN** the native launcher has passed equivalent missing-runtime, source-openability, exported-package, and repeated-lifecycle qualification
- **WHEN** the accepted product cutover retires the pure-GDScript Bootstrap
- **THEN** the project SHALL remain directly openable and its native diagnostic entry SHALL report missing or incompatible artifacts before Python imports or networking
- **AND** Bootstrap removal MUST remain blocked until that equivalent behavior is proven

### Requirement: Python is the primary Godot feature language behind a native bootstrap

The Godot client SHALL implement host lifecycle, feature orchestration, desktop adaptation, and ordinary presentation scripts primarily in typed Python through one exactly pinned, project-hardened derivative of the community Py4Godot GDExtension and an isolated interpreter. The derivative MUST be reproduced from an exact upstream source revision and a minimal repository-owned patch stack limited to interpreter isolation, editor integration, packaging, and lifecycle correctness. It MUST NOT expose project gameplay/data APIs or become a second bridge. Before accepted product cutover, GDScript MUST be limited to the minimum migration bootstrap and setup-diagnostic path required to open the project without that extension. After native diagnostic qualification and accepted cutover, that migration path SHALL be retired; production gameplay GDScript remains prohibited. Rust SHALL retain project-owned native bridging and performance-sensitive Godot resource work, and the current pilot Go core SHALL retain session, protocol, mirror, prediction, and presentation-semantic ownership only until its accepted Rust client-core replacement. After the accepted Rust client-core replacement, Rust SHALL own those responsibilities and the bridge MUST NOT load the pilot Go core. The final product SHALL exclude Go real-time dependencies. A failure to qualify the Python runtime MUST stop the Python-primary pilot and MUST NOT authorize a GDScript feature rewrite.

#### Scenario: A prepared project hands control to Python

- **GIVEN** the selected migration Bootstrap or qualified native launcher has verified the exact Godot version, Python extension, interpreter identity, project bridge identity, and desktop target
- **WHEN** the developer runs the main scene
- **THEN** the selected startup entry SHALL dynamically hand control to the Python host, which assembles the declared feature catalog
- **AND** host, feature, and desktop-adapter behavior after that handoff MUST be implemented in Python unless an approved native hot-path boundary applies

#### Scenario: The Python runtime is missing or incompatible

- **GIVEN** the Python extension, bundled interpreter, standard library, or declared runtime identity is missing, modified, or incompatible
- **WHEN** the selected startup entry validates the distribution unit
- **THEN** the migration Bootstrap SHALL remain in its setup-diagnostic path before cutover, or the native launcher SHALL report an equivalent diagnostic after cutover, without importing a Python feature or starting a network session
- **AND** the pilot MUST NOT fall back to a GDScript feature host or advertise gameplay readiness

#### Scenario: A production GDScript feature is introduced

- **GIVEN** a new production script outside the applicable migration-only bootstrap and setup-diagnostic allowlist uses GDScript for host, feature, UI, or platform behavior
- **WHEN** project architecture validation runs
- **THEN** validation MUST fail and identify the script and allowed ownership boundary
- **AND** the change MUST either move the behavior to Python, justify a Rust native hot path through architecture review, or revise this specification explicitly

#### Scenario: Python attempts to bypass the project bridge

- **GIVEN** Python code imports a direct Go/client-core C binding, calls the engine ABI, imports the standalone companion Agent service, or loads a system library to reach those implementations
- **WHEN** source, dependency, or distribution validation runs
- **THEN** validation MUST fail before the selected client is packaged
- **AND** Python SHALL consume client data only through bounded Godot-visible methods, signals, and value objects exposed by `MornleaClientBridge`

### Requirement: Godot host capabilities evolve independently

The Godot client SHALL use a stable host lifecycle to assemble presentation or device capabilities that have stable identities, contract versions, dependencies, availability states, and work-budget classes. Adding terrain, entity, UI, audio, local-launch, capture, or platform-release capabilities MUST preserve the existing project root, qualified startup diagnostics, server-authority boundary, and compatible behavior of supported capabilities. Unimplemented or incompatible capabilities MUST NOT be advertised as available.

#### Scenario: A later optional capability is added

- **GIVEN** a later migration stage adds an optional capability with a new identity or compatible contract version
- **WHEN** the client starts in configurations that include and exclude that capability
- **THEN** the including configuration SHALL assemble the capability according to its declared dependencies and budget
- **AND** the excluding configuration SHALL preserve the existing supported capabilities, startup path, and network semantics

#### Scenario: A required capability is missing or version-incompatible

- **GIVEN** a required capability declared by the current run configuration is missing, has unsatisfied dependencies, or has an incompatible contract version
- **WHEN** the Python host validates the feature catalog after the selected startup entry has verified its runtime
- **THEN** the client MUST fail before creating world-presentation state and display a stable diagnostic
- **AND** it MUST NOT partially enable features that depend on that capability or switch to an undeclared fallback implementation

#### Scenario: An optional capability fails to initialize

- **GIVEN** an optional presentation capability that does not affect minimum-session correctness fails during initialization
- **WHEN** the host completes capability assembly
- **THEN** the host MAY disable that capability and record an observable diagnostic
- **AND** disabling it MUST NOT modify the authoritative mirror, bypass input validation, or corrupt other capabilities' state

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

### Requirement: The existing client and default startup behavior remain unchanged

Before the independently accepted default-product cutover, the pilot SHALL coexist with the existing Rust graphical client as an explicitly selected independent entry point. Default `mornlea`, local Memory gameplay, remote connections, capture, benchmark, visual baselines, and client ABI behavior MUST remain unchanged by pilot work. Before cutover, removing the pilot files and build entry points SHALL restore the repository to its pre-pilot production behavior.

#### Scenario: An ordinary user does not select the pilot

- **GIVEN** the product cutover has not been accepted and a user invokes an existing startup command without explicitly selecting the Godot pilot
- **WHEN** the client starts, connects, or runs capture or benchmark
- **THEN** the system SHALL continue to use the existing Rust window and rendering production path
- **AND** it MUST NOT probe, load, or depend on the Godot runtime

#### Scenario: Pilot initialization fails

- **GIVEN** Godot, the Python runtime, the project native adapter, GPU, or resource initialization fails
- **WHEN** the user explicitly starts the pilot
- **THEN** the pilot MUST return a clear failure and release acquired resources
- **AND** it MUST NOT silently fall back to the old renderer in the same process and create a mixed runtime

#### Scenario: An accepted product cutover changes the default

- **GIVEN** Rust foundation/server/client-core acceptance, all required production capabilities, two distinct release cycles, and complete rollback evidence have passed
- **WHEN** the separately approved product cutover changes default startup
- **THEN** the default SHALL select Rust authority and client-core with qualified Godot/Python presentation, while the complete previous release remains recoverable
- **AND** post-cutover rollback SHALL restore the previous release with compatible save state instead of deleting pilot files or mixing old and new runtime libraries
