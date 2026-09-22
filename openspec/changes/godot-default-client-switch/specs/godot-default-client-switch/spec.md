## Purpose

Switch the default product to Rust server/client-core and Godot/Python after two release cycles, with complete rollback and explicit transition retirement. This planned contract becomes live only after its implementation and prerequisite gates pass.

## ADDED Requirements

### Requirement: Default startup changes only after complete release evidence

The default graphical entry SHALL switch to Godot with embedded Python only after F1–F3 and every required P8–P13 capability pass, at least two distinct release cycles pass full parity and rollback acceptance, and the previous complete release remains recoverable. A pilot Go decision or a single stable release MUST NOT authorize cutover.

#### Scenario: One release cycle or prerequisite is missing

- **GIVEN** only one qualifying cycle exists or a required feature/platform/foundation gate is incomplete
- **WHEN** default cutover is requested
- **THEN** the request is blocked and current default startup remains unchanged

#### Scenario: The complete cutover gate passes

- **GIVEN** all prerequisites and two distinct identified release cycles have accepted parity, package, and rollback evidence
- **WHEN** the approved cutover is applied
- **THEN** default startup selects the Rust server/client-core plus Godot/Python product and records the exact previous release and rollback procedure

#### Scenario: The selected release fails to initialize

- **GIVEN** a default launch cannot initialize its pinned runtime or resources
- **WHEN** startup reaches the failure boundary
- **THEN** it reports a stable diagnostic and releases resources without silently loading a mixed old/new runtime

### Requirement: The final real-time product excludes transitional Go ownership

The selected product release SHALL run authoritative server and client-core logic in Rust and presentation in qualified embedded Python. It MUST exclude the pilot Go core ABI and Go real-time dependencies. Approved Go offline tools and previous complete releases MAY remain for comparison and rollback.

#### Scenario: A transitive Go runtime dependency remains

- **GIVEN** the package or default launcher still loads the pilot Go shared library or invokes Go for session/server work
- **WHEN** cutover closure validation runs
- **THEN** the cutover fails even if Godot renders successfully

#### Scenario: Legacy client ABI and pilot core ABI are inventoried

- **GIVEN** the previous renderer uses client ABI v19 and the pilot exposes its distinct Go client-core ABI
- **WHEN** retirement is planned
- **THEN** each identity and consumer set is recorded separately; no shared version assumption or partial library deletion is accepted

#### Scenario: Old code is retired

- **GIVEN** the complete old release is preserved and replay/tool consumers are explicitly inventoried
- **WHEN** an approved retirement node removes unused runtime code
- **THEN** remaining offline tools stay runnable and rollback restores the complete old release rather than combining incompatible libraries

### Requirement: Native diagnostics replace the migration Bootstrap before retirement

The final product SHALL diagnose missing or incompatible native/Python artifacts through a qualified native launcher before importing Python or creating a session. The migration-only pure-GDScript Bootstrap MUST remain until equivalent source-openability and diagnostic behavior passes, then SHALL be retired without adding gameplay GDScript.

#### Scenario: Bootstrap removal is attempted too early

- **GIVEN** the native launcher lacks a missing-runtime or source-openability acceptance case
- **WHEN** retirement validation runs
- **THEN** removal is blocked and the migration diagnostic path remains

#### Scenario: Native diagnostics are qualified

- **GIVEN** the launcher has passed prepared/unprepared source, export, corrupt-artifact, and repeated lifecycle cases
- **WHEN** the product retires the Bootstrap
- **THEN** the stable Godot project root remains openable and failures still report the artifact/target/preparation action before feature imports or networking

### Requirement: Cutover updates enforcement without weakening unrelated boundaries

Cutover SHALL replace only the optional-entry and migration-Bootstrap checks that conflict with the accepted product contract. Replacement tests MUST reject mixed runtime dependencies, unsupported fallbacks, missing diagnostics, and unsafe rollback before default entry changes.

#### Scenario: An optional-only audit blocks a legitimate cutover

- **GIVEN** the accepted product intentionally makes Godot part of default startup
- **WHEN** the cutover changes enforcement
- **THEN** a reviewed scoped contract and failing replacement tests land with the new entry; no exemption flag, skipped hook, blanket test deletion, or unrelated boundary relaxation is used

#### Scenario: Rollback is exercised after cutover

- **GIVEN** the previous release and compatible save backup are present
- **WHEN** the accepted rollback action runs
- **THEN** one complete prior runtime regains exclusive world ownership and the restored release passes its own identity and gameplay smoke checks
