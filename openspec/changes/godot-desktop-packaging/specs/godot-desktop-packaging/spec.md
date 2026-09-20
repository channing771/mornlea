## Purpose

Deliver desktop release closures and local play through the same authoritative Rust runtime used remotely. This planned contract becomes live only after its implementation and prerequisite gates pass.

## ADDED Requirements

### Requirement: Local and remote play share one authoritative Rust path

Local and remote play SHALL use the same Rust server login, packet validation, simulation, persistence, and client-core semantics. Local launch MUST NOT bypass protocol or validation and MUST NOT create a second online authority.

#### Scenario: A local session starts

- **GIVEN** a supported local profile and valid world are selected
- **WHEN** the client starts local play
- **THEN** exactly one Rust server owns the world and the client completes the same versioned login and validated command path as remote TCP

#### Scenario: Local and remote receive the same replay

- **GIVEN** both transports begin from the same compatible save and fixed input sequence
- **WHEN** the sessions execute independently
- **THEN** confirmed outcomes, rejection behavior, and persisted state agree after transport-independent normalization

#### Scenario: The local server fails or a second writer is attempted

- **GIVEN** a local world is already owned or the supervised server exits
- **WHEN** the client attempts launch or continues the session
- **THEN** launch/session fails with a stable diagnostic and releases owned resources; it never starts another writer or silently rewrites the save

### Requirement: Each desktop release is a qualified closed distribution

Each supported macOS, Windows, or Linux release SHALL contain exactly its qualified Godot, native, embedded Python, catalog-selected feature, asset, and license closure. It MUST run without system Python, user packages, runtime installation, developer paths, or comparison tooling.

#### Scenario: An isolated desktop package starts

- **GIVEN** a target package has accepted runtime, export, and lifecycle evidence
- **WHEN** it starts from a clean target environment
- **THEN** the client resolves all dependencies from its package and the optional standalone Agent remains a separate service

#### Scenario: A package contains an escaped or wrong-target dependency

- **GIVEN** a package references an external developer path, missing runtime payload, non-target native library, or Agent import
- **WHEN** release closure validation runs
- **THEN** validation fails and the package cannot be declared supported

#### Scenario: Only one target has qualification evidence

- **GIVEN** macOS passes but Windows or Linux evidence is absent
- **WHEN** release status is reported
- **THEN** only the qualified target is marked supported; another target remains blocked instead of inheriting the macOS result

### Requirement: Desktop rollout preserves save compatibility and a usable rollback

Desktop release acceptance SHALL include local/remote replay, persistence failure paths, checksums/licenses, and restore of the previous release. It MUST NOT switch default startup before P14 acceptance.

#### Scenario: A release is rolled back

- **GIVEN** the previous release and compatible pre-launch save backup are retained
- **WHEN** the new release fails acceptance or is explicitly reverted
- **THEN** the previous complete runtime is restored with the compatible save state and no concurrent writer or implicit downgrade rewrite

#### Scenario: A required package or compatibility case is missing

- **GIVEN** one required target or local/remote save failure case lacks evidence
- **WHEN** packaging closeout runs
- **THEN** the release remains incomplete and the default product entry remains unchanged
