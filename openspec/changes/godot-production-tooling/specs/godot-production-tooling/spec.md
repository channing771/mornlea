## Purpose

Provide reproducible migration evidence and reviewed per-case visual producer handoff without expanding runtime authority. This planned contract becomes live only after its implementation and prerequisite gates pass.

## ADDED Requirements

### Requirement: Migration evidence is identified per case and producer

The migration evidence system SHALL register each case by stable semantic identity, class, current producer, candidate producer, required coverage, input, capture boundary, and work limits. Every result MUST retain source and runtime identities and be classified as comparable, non-comparable, or failed. A selected run MUST be explicit and reproducible.

#### Scenario: A complete candidate run is compared

- **GIVEN** the explicitly selected run contains all required cases with compatible identity and semantic replay results
- **WHEN** comparison runs without update authorization
- **THEN** it emits candidate evidence, per-case status, and difference artifacts without changing tracked images, ownership, or thresholds

#### Scenario: A required case has no mapping

- **GIVEN** the candidate registry is empty or lacks a required case mapping
- **WHEN** strict acceptance evaluates the selected feature
- **THEN** it exits nonzero and identifies missing coverage; an empty pilot report is never a pass

#### Scenario: A report is stale or non-comparable

- **GIVEN** a case has stale source identity, mismatched input/capture identity, or unsupported producer compatibility
- **WHEN** strict acceptance evaluates that case
- **THEN** it reports non-comparable or failed with the reason and exits nonzero for required cases rather than choosing another or latest run

### Requirement: Comparison separates semantic migration evidence from regression pixels

Acceptance SHALL first compare semantic replay, then candidate presentation, then reviewed canonical ownership. Same-producer pixel regression MUST retain existing thresholds. Cross-producer parity MUST describe semantic agreement and reviewed rendering differences instead of claiming pixel equivalence solely from relaxed tolerance.

#### Scenario: Two renderers preserve semantics but differ in pixels

- **GIVEN** the same case has valid semantic replay and an identified rasterization difference
- **WHEN** cross-producer comparison runs
- **THEN** it retains source/candidate/diff artifacts and requires human review before handoff without changing any tolerance

#### Scenario: A motion case is captured

- **GIVEN** a required case expresses a cross-tick behavior script
- **WHEN** the capture completes
- **THEN** coverage includes the declared start/action/end boundaries and emits bounded motion evidence for human review, without an automated GIF pixel verdict

#### Scenario: A dummy renderer is selected for GPU evidence

- **GIVEN** the selected headless backend provides no real GPU pixels
- **WHEN** the run requests a GPU world/UI frame
- **THEN** the run fails or reports non-comparable; a zero/blank dummy image cannot satisfy coverage

### Requirement: Canonical handoff is explicit atomic and reversible

Producer ownership SHALL transfer per named case only after semantic parity, complete candidate evidence, manual difference review, and explicit approval. The update MUST preserve unrelated image bytes, registries, and thresholds and MUST publish the reviewed ownership/image set atomically or restore the previous set on failure.

#### Scenario: A reviewed case changes producer

- **GIVEN** approval names the exact cases, source run, expected differences, old/new producer, and rollback
- **WHEN** the authorized updater publishes the change
- **THEN** only the reviewed image and ownership set changes; the final check uses the new canonical producer and recorded rollback restores the previous set

#### Scenario: A world update would rewrite motion files

- **GIVEN** the chosen update entry point also regenerates motion evidence outside the approved world case set
- **WHEN** the updater builds and validates its candidate output
- **THEN** it detects and excludes or rejects unapproved side effects before publishing, preserving unrelated motion bytes

#### Scenario: An update fails during I/O

- **GIVEN** the updater has staged a reviewed set but cannot complete a write or verification
- **WHEN** publication is attempted
- **THEN** the command fails and preserves or restores the complete old ownership/image set without partial canonical state

### Requirement: Tooling remains outside real-time product authority

Replay, report, capture, import, and CI tools SHALL consume the Rust contracts without creating an online second authority. Development comparison tooling MUST be excluded from production release closures.

#### Scenario: Legacy and Rust behavior are compared

- **GIVEN** Go and Rust consume the same frozen replay corpus
- **WHEN** the parity tool runs
- **THEN** comparison is offline and only one selected runtime can write a live world; the client does not import the standalone Agent service

#### Scenario: A release closure includes a comparison tool

- **GIVEN** a desktop package contains development-only replay/CLI dependencies or tooling assets
- **WHEN** release validation runs
- **THEN** the package fails validation until the tooling closure is excluded

### Requirement: Partial migration compares each case with its canonical owner

Canonical regression SHALL dispatch each required case to exactly one registered producer and its accepted baseline. Unmigrated cases MUST retain legacy coverage. Legacy whole-suite commands MUST NOT be treated as current acceptance when they render cases already owned by another producer.

#### Scenario: UI migrates before world

- **GIVEN** selected UI fixtures have accepted Godot ownership while world cases remain legacy-owned
- **WHEN** canonical regression runs
- **THEN** it MUST compare the transferred UI with Godot and the remaining cases with their registered legacy producers, preserving full required coverage
- **AND** missing ownership or inability to select the required legacy subset MUST fail rather than silently skip cases

#### Scenario: Previous release is verified for rollback

- **GIVEN** a complete previous release retains its producer and baseline identities
- **WHEN** its full legacy regression runs
- **THEN** it MUST use that release's baseline set and MUST NOT overwrite or compare against the current new-producer baseline set
