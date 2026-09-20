## Purpose

Present production terrain from Rust semantic publications with bounded resources and reversible visual ownership. This planned contract becomes live only after its implementation and prerequisite gates pass.

## ADDED Requirements

### Requirement: Terrain consumes confirmed bounded semantic publications

Terrain presentation SHALL consume immutable semantic world publications from the Rust client core, preserving epoch, revision, visibility, and complete-batch validation. Python SHALL only apply bounded presentation resources; it MUST NOT derive terrain rules or numerical results.

#### Scenario: A valid world publication becomes visible

- **GIVEN** the selected catalog feature has a version-compatible terrain family and sufficient budget
- **WHEN** a complete publication for the active epoch arrives
- **THEN** the approved near/far visibility, LOD, water/cutout material class, lighting, and fog are presented using that publication, without reading protocol bytes or save records

#### Scenario: An invalid or oversized publication is rejected atomically

- **GIVEN** a publication contains a stale epoch, backward revision, invalid record, or real capacity overflow
- **WHEN** the feature validates the publication
- **THEN** it rejects the whole publication, reports a stable failure, and preserves the last valid resources without truncated success

#### Scenario: Frame work exceeds the apply budget

- **GIVEN** valid pending terrain work exceeds the declared per-frame resource budget
- **WHEN** a frame is presented
- **THEN** work is deferred in bounded queues without blocking the main thread; real queue overflow is observable and cannot silently drop accepted updates

### Requirement: Terrain can be disabled and torn down without changing authority

The world feature SHALL own its Godot resource lifecycle and remain catalog-replaceable. Reset and teardown MUST release old-epoch resources and cancel pending work; disabling the feature MUST NOT change server state or the default client.

#### Scenario: Reset invalidates pending completions

- **GIVEN** old-epoch mesh completions and pooled resources remain pending
- **WHEN** the session resets or the feature is deactivated
- **THEN** old completions cannot resurrect terrain, owned resources are released, and reactivation starts from a fresh semantic snapshot

#### Scenario: The production terrain feature is unavailable

- **GIVEN** its catalog entry is disabled or required family negotiation fails
- **WHEN** the user starts a supported configuration
- **THEN** disabled configurations retain their declared behavior and required configurations fail before partial world presentation; no Go/Python numerical fallback is selected

### Requirement: Terrain parity precedes canonical visual ownership

Terrain SHALL pass semantic replay parity and identity-complete candidate evidence for every required world case before requesting canonical ownership. Same-producer regression thresholds MUST remain unchanged; a cross-producer visual difference requires explicit review.

#### Scenario: A world case is non-comparable

- **GIVEN** required scenario, asset, runtime, or capture identity is missing or differs incompatibly
- **WHEN** terrain acceptance runs
- **THEN** the case is marked non-comparable and blocks handoff; the canonical producer and tracked image remain unchanged

#### Scenario: Reviewed terrain is promoted

- **GIVEN** all required semantic and visual cases are complete and differences have been reviewed
- **WHEN** an explicit per-case producer handoff is approved
- **THEN** only the named existing world cases change producer through the reviewed update path, with unrelated baselines preserved and rollback recorded
