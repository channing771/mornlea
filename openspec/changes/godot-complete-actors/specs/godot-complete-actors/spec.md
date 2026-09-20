## Purpose

Present complete actors and effects from Rust semantic families without creating gameplay authority. This planned contract becomes live only after its implementation and prerequisite gates pass.

## ADDED Requirements

### Requirement: Actor presentation preserves entity identity and confirmed outcomes

Actor features SHALL consume typed Rust semantic snapshots for companions, hostiles, passives, projectiles, viewmodel, and effects. Server facts MUST remain authoritative, while animation and pooling MUST NOT write gameplay state.

#### Scenario: A confirmed actor set is presented

- **GIVEN** a compatible family publishes stable entity identities and an active epoch
- **WHEN** spawn/update/despawn records are applied
- **THEN** each live identity has at most one presentation instance and pose/effect inputs reflect the accepted snapshot without guessed commands

#### Scenario: Actor ordering or capacity is invalid

- **GIVEN** a batch has duplicate identities, backward ordering, stale epoch, or excessive capacity
- **WHEN** the actor feature receives the batch
- **THEN** the whole batch is rejected without partial spawn, despawn, or pool mutation

#### Scenario: A reset races an effect completion

- **GIVEN** an effect or animation callback from the previous epoch is pending
- **WHEN** the actor feature resets or deactivates
- **THEN** the callback cannot resurrect an actor or effect and every owned resource is returned or released

### Requirement: Actor capabilities are independently enableable

Each supported actor family SHALL declare its required semantic family and lifecycle. Unsupported kinds MUST stay unavailable; optional disable MUST preserve the correctness of other families.

#### Scenario: One optional actor family is disabled

- **GIVEN** companions are disabled while another supported actor family remains enabled
- **WHEN** the catalog assembles the client
- **THEN** only the enabled family starts, dependencies remain valid, and neither feature issues gameplay commands to approximate missing presentation

#### Scenario: A required actor family is unavailable

- **GIVEN** the selected profile requires a family whose version is unsupported
- **WHEN** catalog negotiation runs
- **THEN** startup fails with a stable diagnostic before partial actor presentation

### Requirement: Actor acceptance covers motion and occlusion semantics

Actor acceptance SHALL cover identity, spawn/despawn, occlusion, animation, and effect timing through semantic replay and per-case candidate evidence. Cross-tick GIF evidence MUST receive human review and MUST NOT become an automated pixel gate.

#### Scenario: Animation evidence has a missing phase

- **GIVEN** an actor script lacks required spawn, action, or despawn coverage
- **WHEN** acceptance evaluates the explicit run
- **THEN** the required case fails coverage and cannot be handed off despite any available still image

#### Scenario: Reviewed actor cases are handed off

- **GIVEN** semantic parity and reviewed still/motion evidence are complete
- **WHEN** a per-case handoff is explicitly approved
- **THEN** only those cases change canonical producer with rollback, and unrelated actor cases keep their producer
