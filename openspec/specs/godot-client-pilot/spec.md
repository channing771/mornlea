# godot-client-pilot Specification

## Purpose

This capability establishes an independently runnable, comparable, and removable Godot remote-client vertical slice without replacing Mornlea's authoritative server or current production client. Decidable evidence from the slice determines whether a complete client migration should continue.

## Requirements

### Requirement: The Godot pilot preserves sole server authority

The Godot pilot client SHALL submit only player input intent and present server-confirmed state. Final world, player, entity, inventory, container, damage, drop, crafting, weather, and time state MUST remain server-authoritative. The pilot client MUST NOT confirm any gameplay outcome from local presentation or prediction results.

#### Scenario: A newer authoritative state corrects prediction

- **GIVEN** the client displays a locally predicted position based on unconfirmed input
- **WHEN** the client receives player state from a newer server tick and its position differs from the prediction
- **THEN** the client MUST correct to the authoritative state and replay still-unconfirmed input according to the existing prediction rules
- **AND** the client MUST NOT send an additional state write to the server to confirm the prediction result

#### Scenario: An unconfirmed interaction does not change the gameplay mirror

- **GIVEN** the player initiates placement, mining, an attack, an item move, or a container operation
- **WHEN** the corresponding server confirmation has not arrived or returns a rejection
- **THEN** the client MUST NOT write the operation into the authoritative mirror as settled state
- **AND** the presentation layer MAY display reversible local feedback but MUST NOT change the authoritative input facts used by later commands

### Requirement: The pilot reuses the current remote protocol and login semantics

The Godot pilot client SHALL connect to the existing dedicated server through the existing TCP packet/codec, login state machine, and sole current protocol version. It MUST NOT introduce Godot-specific packets, bypass login, a privileged shared-memory path, or a legacy-protocol compatibility layer.

#### Scenario: The current protocol enters gameplay successfully

- **GIVEN** the existing dedicated server accepts the current protocol version and the player identity and view distance are valid
- **WHEN** the pilot client completes handshake and login
- **THEN** the client SHALL enter Play state and consume the same initial snapshot and subsequent messages as the existing remote client

#### Scenario: Protocol version mismatch

- **GIVEN** the server protocol version differs from the pilot client's current version
- **WHEN** the pilot client initiates a handshake
- **THEN** login MUST fail with the existing version-mismatch semantics
- **AND** the client MUST NOT create game-world presentation state or attempt a downgraded connection

#### Scenario: Invalid or truncated message

- **GIVEN** the connection receives a message with an invalid length, invalid enum, out-of-range count, truncation, or trailing bytes
- **WHEN** the client decodes that message
- **THEN** the client MUST reuse the existing strict decode-failure semantics to close or reject the session
- **AND** it MUST NOT publish a partial message result to the presentation layer

### Requirement: The pilot provides a minimum playable remote loop

The Godot pilot client SHALL provide at least window creation and resizing, keyboard and mouse input, cursor capture, a first-person camera, movement and look input, near-ring terrain, chunk deltas, block-target feedback, one remote-entity class, basic day/night and weather presentation, basic health/hunger/oxygen HUD, and understandable terminal connection states. Menus, containers, and visual effects outside this loop MUST be clearly shown as unimplemented or hidden and MUST NOT masquerade as available functionality.

#### Scenario: The player completes the minimum game loop

- **GIVEN** the server has loaded chunks around spawn and the session has entered Play state
- **WHEN** the player moves, turns the view, and observes the nearby world and a remote entity
- **THEN** the client SHALL continuously display a predicted and correctable player camera, loaded terrain, subsequent chunk changes, and at least one remote-entity class
- **AND** the current target block and basic HUD SHALL reflect only available confirmed state

#### Scenario: An unimplemented feature sends no invalid command

- **GIVEN** the pilot has not implemented a menu, container, or gameplay interaction
- **WHEN** the player triggers its corresponding input
- **THEN** the client MUST hide, disable, or clearly label that entry point
- **AND** it MUST NOT send protocol commands with an invalid shape, incomplete semantics, or guessed parameters

### Requirement: The Godot project root opens directly and is resource-closed

The Godot client SHALL provide a stable project root that a supported Godot Standard editor can directly recognize and open. Scenes, scripts, resources, Python runtime files, extension descriptors, and distributable native dependencies required to run the project MUST resolve through resource paths inside that project root. The system MUST NOT require the repository root to be recognized as a Godot project, or depend on parent-directory traversal, non-portable symbolic links, system Python, user site-packages, runtime package installation, or the system dynamic-library search path to run.

#### Scenario: Open the project before native libraries are prepared

- **GIVEN** a developer has a complete source checkout and a supported Godot Standard editor but has not built the native bridge libraries
- **WHEN** the developer selects the prescribed Godot project root in Godot Project Manager
- **THEN** the editor SHALL recognize and open the project and its pure-GDScript Bootstrap scene successfully
- **AND** running Bootstrap SHALL display the missing native bridge, Python runtime, target platform, and preparation command without importing feature modules, starting a network session, or pretending to be playable

#### Scenario: Run from the project root after preparation

- **GIVEN** resource synchronization and the native build have produced a complete, version-matched distribution unit containing the pinned Python extension and isolated interpreter for the current platform
- **WHEN** the developer runs the main scene from the same Godot project root
- **THEN** Bootstrap SHALL resolve the Python runtime, bridge library, and their dependencies only from the project distribution unit and verify their identities
- **AND** after successful verification it SHALL assemble the enabled capabilities without changing the project root or main scene

#### Scenario: A resource or dynamic-library reference escapes the project root

- **GIVEN** a scene, resource, extension, or dependency declaration contains parent-directory traversal, an absolute development-machine path, a non-portable symbolic link, or a system-search-path dependency
- **WHEN** project-closure checks or export validation run
- **THEN** validation MUST fail and identify the violating reference
- **AND** the build MUST NOT be marked runnable or distributable

### Requirement: Python is the primary Godot feature language behind a native bootstrap

The Godot client SHALL implement host lifecycle, feature orchestration, desktop adaptation, and ordinary presentation scripts primarily in typed Python through one exactly pinned, project-hardened derivative of the community Py4Godot GDExtension and an isolated interpreter. The derivative MUST be reproduced from an exact upstream source revision and a minimal repository-owned patch stack limited to interpreter isolation, editor integration, packaging, and lifecycle correctness. It MUST NOT expose project gameplay/data APIs or become a second bridge. Production GDScript MUST be limited to the minimum bootstrap and setup-diagnostic path required to open the project without that extension. Rust SHALL retain project-owned native bridging and performance-sensitive Godot resource work, and Go SHALL retain session, protocol, mirror, prediction, and presentation-semantic ownership. A failure to qualify the Python runtime MUST stop the Python-primary pilot and MUST NOT authorize a GDScript feature rewrite.

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

### Requirement: The Python runtime is pinned, isolated, and distributable

The pilot SHALL pin the Python GDExtension upstream release and source revision, repository patch-series digest, build-tool and source-input identities, embedded interpreter version, target architecture, output artifact checksum, licenses, and distribution obligations. The current macOS pilot MUST pass editor, headless, export, and repeated load/unload checks with the project-local runtime while system Python and user site-packages are unavailable. Runtime behavior MUST NOT invoke `pip`, download packages, resolve an unpinned dependency, or depend on a developer virtual environment. Build-time dependency acquisition MUST occur only in an explicit external cache preparation step and MUST be verifiable offline afterward.

#### Scenario: The approved hardened derivative is reproduced

- **GIVEN** the exact rejected Py4Godot upstream revision, the repository-owned ordered patch series, and pinned build inputs
- **WHEN** the current macOS runtime artifact is built and materialized
- **THEN** the build SHALL verify the upstream source identity, every patch, the patch-series digest, and the output artifact identity before Godot loads it
- **AND** the patches MUST be limited to interpreter isolation, editor integration, packaging, and lifecycle correctness, contain no Mornlea gameplay/data API, and state when they can be removed after an upstream fix

#### Scenario: The current macOS Python unit passes qualification

- **GIVEN** the approved Godot version and current macOS desktop target
- **WHEN** the pinned Python runtime is prepared and qualified
- **THEN** Python scripts SHALL load in the editor and headless runner, survive repeated project lifecycle tests, and run from an exported desktop unit
- **AND** the report SHALL record exact plugin, interpreter, target, checksum, and license identities

#### Scenario: The host environment supplies a different Python installation

- **GIVEN** system Python, `PYTHONPATH`, user site-packages, or a developer virtual environment differs from the pinned project runtime
- **WHEN** smoke tests or an exported pilot start
- **THEN** the pilot SHALL ignore those external environments and use only its project-local isolated runtime
- **AND** validation MUST fail if removing the external environment changes successful startup or feature behavior

#### Scenario: Neither upstream nor the approved hardened derivative qualifies

- **GIVEN** neither an exact upstream Python GDExtension artifact nor the explicitly approved hardened derivative passes the current Godot, macOS, headless, export, and lifecycle matrix
- **WHEN** P1 dependency qualification concludes
- **THEN** the change SHALL record a Python-primary No-Go before gameplay implementation continues
- **AND** it MUST NOT accept any broader or unreviewed fork, system interpreter, GDScript feature fallback, or reduced validation matrix without another explicit design revision

### Requirement: The Godot client targets desktop platforms only

The Godot client SHALL support only macOS, Windows, and Linux desktop environments. The pilot SHALL require implementation only for the current macOS development environment and SHALL reserve platform boundaries only for later Windows/Linux desktop distribution. The project, input, lifecycle, native bridge, and export configuration MUST NOT introduce Android, iOS, Web, or console support, mobile touch semantics, or a mobile application lifecycle.

#### Scenario: Prepare the project on a supported desktop platform

- **GIVEN** the target platform is the currently approved macOS desktop target, or a Windows/Linux desktop target enabled by a later independent change
- **WHEN** Bootstrap, the native build, or export validation resolves the target platform
- **THEN** the system SHALL select the corresponding desktop dynamic libraries, desktop input, and desktop window configuration
- **AND** it MUST NOT load any mobile, Web, or console adapter

#### Scenario: Request a mobile or Web export

- **GIVEN** the target platform is Android, iOS, Web, or an unapproved console platform
- **WHEN** build, Bootstrap, or export-preset validation runs
- **THEN** the system MUST reject that target with a stable error before compiling or packaging platform-specific artifacts
- **AND** it MUST NOT create a distributable artifact through an empty adapter, compatibility mode, or untested template

#### Scenario: Desktop input reserves no touch bypass

- **GIVEN** the pilot declares only keyboard, mouse, and later optional desktop-controller semantics
- **WHEN** the feature catalog and platform adaptation layer are assembled
- **THEN** input capabilities SHALL expose only implemented desktop semantic actions
- **AND** they MUST NOT add touch, mobile sensors, virtual joysticks, or a mobile pause/resume lifecycle bypass

### Requirement: Godot host capabilities evolve independently

The Godot client SHALL use a stable host lifecycle to assemble presentation or device capabilities that have stable identities, contract versions, dependencies, availability states, and work-budget classes. Adding terrain, entity, UI, audio, local-launch, capture, or platform-release capabilities MUST preserve the existing project root, Bootstrap, server-authority boundary, and compatible behavior of supported capabilities. Unimplemented or incompatible capabilities MUST NOT be advertised as available.

#### Scenario: A later optional capability is added

- **GIVEN** a later migration stage adds an optional capability with a new identity or compatible contract version
- **WHEN** the client starts in configurations that include and exclude that capability
- **THEN** the including configuration SHALL assemble the capability according to its declared dependencies and budget
- **AND** the excluding configuration SHALL preserve the existing supported capabilities, startup path, and network semantics

#### Scenario: A required capability is missing or version-incompatible

- **GIVEN** a required capability declared by the current run configuration is missing, has unsatisfied dependencies, or has an incompatible contract version
- **WHEN** the Python host validates the feature catalog after Bootstrap has verified its runtime
- **THEN** the client MUST fail before creating world-presentation state and display a stable diagnostic
- **AND** it MUST NOT partially enable features that depend on that capability or switch to an undeclared fallback implementation

#### Scenario: An optional capability fails to initialize

- **GIVEN** an optional presentation capability that does not affect minimum-session correctness fails during initialization
- **WHEN** the host completes capability assembly
- **THEN** the host MAY disable that capability and record an observable diagnostic
- **AND** disabling it MUST NOT modify the authoritative mirror, bypass input validation, or corrupt other capabilities' state

### Requirement: Cross-runtime boundaries publish only bounded atomic snapshots

Inputs, events, world deltas, and frame snapshots crossing between the Go client core, Rust bridge, Python host, and Godot presentation runtime SHALL carry an explicit layout version, epoch or revision, and length and count limits. If any version, length, capacity, reserved-field, ordering, or content validation fails, the receiver MUST reject the entire batch and preserve its pre-call state. Python MUST NOT retain native buffer views or mutable sender-owned data after a bridge call returns.

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

### Requirement: Numerical production has one implementation

Voxel mesh, lighting, collision, raycast, player-physics integration, world generation, LOD, and fluid numerical results in the pilot MUST continue to come from the current Rust numerical kernel or an output adaptation that does not rewrite its semantics. Python, GDScript, Godot extensions, and the client core MUST NOT add a second production algorithm.

#### Scenario: Identical input produces identical numerical facts

- **GIVEN** the existing client and Godot pilot receive the same chunks, registry, player state, input sequence, and tick timing
- **WHEN** they request mesh, collision, raycast, or physics results
- **THEN** both SHALL consume results from the same production kernel
- **AND** differences MAY occur only in coordinate-system, vertex-layout, material, or presentation adaptation

#### Scenario: The numerical kernel fails

- **GIVEN** a numerical call encounters invalid input, insufficient scratch space, output overflow, or a status converted from panic
- **WHEN** the pilot handles that result
- **THEN** the pilot MUST explicitly fail that work batch
- **AND** it MUST NOT switch to a Godot or Go fallback algorithm and continue presenting untrusted results

### Requirement: Network and render hot paths remain bounded and non-blocking

The pilot SHALL use fixed budgets to process per-frame messages, mesh completions, world uploads, Python callbacks, entity updates, and UI events. The Godot main thread MUST NOT wait for network, disk, Python package resolution, a server tick, unbounded Python or numerical work, or synchronous GPU readback. Bulk terrain expansion and native resource buffers MUST remain outside Python. A batch and its slices MUST be treated as immutable after a successful cross-thread send.

#### Scenario: A message burst exceeds the frame budget

- **GIVEN** the receive queue contains more pending messages than the per-frame budget
- **WHEN** the client advances one frame
- **THEN** the client SHALL process no more than the budgeted work and leave the remaining valid work for later frames
- **AND** it MUST NOT perform an unbounded drain in the same frame

#### Scenario: A bounded queue truly overflows

- **GIVEN** a bounded queue is full and a new result cannot be enqueued without loss
- **WHEN** the producer submits the new result
- **THEN** the system MUST stop the affected session or pilot run with an observable error
- **AND** it MUST NOT silently discard world or entity updates that would change final state

#### Scenario: Python feature work exceeds its frame budget

- **GIVEN** a Python feature has more valid presentation work than its declared callback or apply budget permits
- **WHEN** the host advances one frame
- **THEN** it SHALL process only bounded work and defer or coalesce only where the feature contract explicitly permits
- **AND** it MUST NOT perform unbounded iteration, synchronous I/O, numerical fallback, or bulk native-buffer conversion on the main thread

### Requirement: Pilot validation reports have comparable identity

The Godot pilot SHALL generate visual and performance reports using the same scenario semantics, seed, resolution, view distance, sampling window, and work budgets as the current client. Reports MUST record commit identity, protocol version, engine ABI, Godot version, Python extension and interpreter identity, platform, GPU, scenario version, and valid sample count. They SHALL separately record Python host/apply duration and allocation pressure so interpreter overhead is not hidden inside aggregate frame time. Performance measurements are recorded only and MUST NOT independently change command exit status.

#### Scenario: Both clients generate evidence for the same scenario

- **GIVEN** the current client and the Godot pilot use the same fixed scenario input
- **WHEN** the validation workflow completes
- **THEN** the report SHALL provide captures suitable for side-by-side inspection and CPU frame, GPU frame, load time, chunk-upload volume, memory, and input-to-presentation latency
- **AND** every result MUST be traceable to a complete run identity

#### Scenario: Report identity is incomplete

- **GIVEN** the scenario version, commit identity, engine version, Python runtime identity, platform, or valid sample count is missing
- **WHEN** the report is prepared for submission
- **THEN** the report-completeness check MUST fail
- **AND** the report MUST NOT be used to decide whether to continue or stop migration

### Requirement: Godot pilot evidence does not fork visual-baseline classification

The Godot pilot SHALL generate only untracked visual evidence with complete identity and SHALL continue to interpret existing visual evidence through the three observable semantic classes of window/UI, headless single-frame world, and cross-tick `motion/` GIF. `motion/` is only for bounded human end-to-end review and is not subject to automated pixel comparison. The pilot MUST NOT create a renderer-specific golden directory, overwrite an existing baseline, or relax current comparison thresholds. Only a later, independently approved feature handoff may transfer ownership of an existing scene, fixture, or motion producer to Godot.

#### Scenario: The pilot generates side-by-side comparison evidence

- **GIVEN** the existing Rust/WebView client remains the formal producer for the corresponding scene or fixture
- **WHEN** the Godot pilot captures an image or process with the same semantics
- **THEN** output SHALL be written to an untracked pilot-evidence directory and record run identity
- **AND** PNG files, GIF files, registries, and thresholds in `testdata/visual-golden/` MUST remain unchanged

#### Scenario: A Godot-specific golden category is proposed

- **GIVEN** an implementation or script attempts to write to `testdata/visual-golden/godot/` or a similar renderer-specific directory
- **WHEN** the visual-routing gate runs
- **THEN** the gate MUST reject that directory
- **AND** it SHALL require the content to use an existing `ui/`, `world/`, or GIF category, or remain in untracked pilot evidence

#### Scenario: Visual-producer ownership is formally transferred later

- **GIVEN** a later feature change has proven that its Godot implementation meets approved semantic parity
- **WHEN** that change requests ownership of an existing visual scene or fixture
- **THEN** it MUST identify the old and new producers, semantic category, affected baselines, expected differences, human review, and rollback
- **AND** it MAY use the existing update path to write a tracked baseline only after explicit approval and item-by-item human review

### Requirement: The existing client and default startup behavior remain unchanged

The pilot SHALL coexist with the existing Rust graphical client as an explicitly selected independent entry point. Default `mornlea`, local Memory gameplay, remote connections, capture, benchmark, visual baselines, and client ABI behavior MUST remain unchanged. Removing the pilot files and build entry points SHALL restore the repository to its pre-pilot production behavior.

#### Scenario: An ordinary user does not select the pilot

- **GIVEN** a user invokes an existing startup command without explicitly selecting the Godot pilot
- **WHEN** the client starts, connects, or runs capture or benchmark
- **THEN** the system SHALL continue to use the existing Rust window and rendering production path
- **AND** it MUST NOT probe, load, or depend on the Godot runtime

#### Scenario: Pilot initialization fails

- **GIVEN** Godot, the Python runtime, the project native adapter, GPU, or resource initialization fails
- **WHEN** the user explicitly starts the pilot
- **THEN** the pilot MUST return a clear failure and release acquired resources
- **AND** it MUST NOT silently fall back to the old renderer in the same process and create a mixed runtime

### Requirement: Pilot assets satisfy project authorization boundaries

The pilot SHALL use only project-owned or procedurally generated code and assets, or license-compatible code and assets with recorded provenance. It MUST NOT introduce Mojang-copyrighted textures, unauthorized binary art assets, Python packages, interpreters, or engine plugins that cannot lawfully and reproducibly be redistributed with the distribution unit.

#### Scenario: An external asset or plugin is added

- **GIVEN** the pilot plans to add an external texture, font, model, audio asset, or Godot plugin
- **WHEN** that dependency enters the repository or build artifact
- **THEN** its source, version, license, and distribution obligations MUST be recorded and pass audit
- **AND** any asset whose authorization cannot be confirmed MUST be rejected

### Requirement: Forward migration preserves the final Python presentation boundary

The pilot SHALL identify the embedded Godot Python host and features as the intended final presentation-language boundary, while identifying the current Go client-core, Go server, and pure-GDScript Bootstrap as migration seams. New migration work MUST NOT replace Python features with production GDScript or add new authoritative, protocol, prediction, persistence, or numerical behavior to Python. A qualified embedded Python runtime remains a prerequisite for the final Godot cutover; a failed qualification MUST block cutover rather than authorize an unqualified language fallback.

#### Scenario: A later feature is added after the pilot

- **GIVEN** a later terrain, actor, UI, audio, or tooling change needs Godot behavior
- **WHEN** the change declares its language and ownership boundary
- **THEN** presentation orchestration SHALL be implemented through the qualified embedded Python boundary and typed semantic views
- **AND** authoritative or bulk computation SHALL be assigned to the Rust target runtime rather than new Go, Python, or GDScript logic

#### Scenario: The embedded Python runtime does not qualify

- **GIVEN** the selected embedded Python runtime fails isolation, export, lifecycle, or reproducibility qualification
- **WHEN** a change attempts to proceed toward the final Godot product cutover
- **THEN** the change MUST be marked blocked or No-Go
- **AND** it MUST NOT add production GDScript features, system-Python dependencies, or a second client data path as a workaround

### Requirement: Transition work keeps one online authority

The pilot and its follow-on changes SHALL allow only one online authoritative world writer at a time. Go may serve as a current implementation or offline replay/differential oracle while Rust ownership is built, but Go and Rust MUST NOT both mutate authoritative state online, and a shadow writer MUST NOT be used as an implicit migration mechanism.

#### Scenario: Rust server behavior is compared with the Go server

- **GIVEN** a migration change needs to compare Rust behavior with the current Go implementation
- **WHEN** the comparison is executed
- **THEN** it SHALL use recorded inputs, packets, saves, or replay transcripts outside the live authoritative session
- **AND** only the selected runtime SHALL write world state during an online run

#### Scenario: A follow-on task would add a new Go gameplay path

- **GIVEN** a feature can be implemented quickly by extending the current Go runtime
- **WHEN** the feature has not been assigned a documented transition exception with a removal condition
- **THEN** the task MUST fail architecture review before implementation
- **AND** the plan SHALL identify the corresponding Rust target owner or defer the feature until that owner exists
