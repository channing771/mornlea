## Purpose

Define compatible shared Rust runtime contracts and reproducible offline evidence before server or client ownership migrates.

## ADDED Requirements

### Requirement: Contract coverage preserves supported behavior

The foundation SHALL inventory every currently supported protocol packet, save family, semantic input/event family and deterministic kernel with its source, version, owner and compatibility fixture. It MUST reject completion when a supported family is missing or an incompatibility is unexplained.

#### Scenario: A supported family is absent

- **GIVEN** the current runtime supports a family absent from the migration corpus
- **WHEN** foundation acceptance runs
- **THEN** it MUST identify the uncovered family and fail acceptance rather than infer parity from the covered subset

#### Scenario: Compatible wire and save records

- **GIVEN** versioned fixtures produced by the current supported implementation
- **WHEN** Rust decodes and re-encodes each supported record
- **THEN** the output MUST preserve the specified bytes, values and version behavior
- **AND** older supported save migrations MUST produce the same normalized result without changing source fixtures

### Requirement: Invalid inputs fail before publication

Contract consumers SHALL enforce length, enum, identity, version, capacity and integrity bounds before publishing decoded state. Unknown versions and corrupt or incomplete records MUST produce stable failures without partial state or implicit repair.

#### Scenario: Malformed record

- **GIVEN** a truncated, oversized, invalid-enum or unsupported-version record
- **WHEN** either runtime processes it in the compatibility harness
- **THEN** it MUST reject the record with the recorded failure category and leave published state unchanged

### Requirement: Replay identity supports offline differential comparison

A replay SHALL identify source revision, contract versions, fixture digest, seed, ordered input and tick schedule, and deterministic state observations. Comparison MUST run Go and Rust independently offline; it MUST fail on missing identity, incomplete traces or unexplained outcome differences.

#### Scenario: Repeated deterministic replay

- **GIVEN** a complete frozen replay and independent Go and Rust runs
- **WHEN** both execute the same input schedule repeatedly
- **THEN** normalized state and event observations MUST agree at each declared checkpoint
- **AND** neither run MUST write to the other's state or live production saves

#### Scenario: Unidentified evidence

- **GIVEN** a result lacks a corpus digest or contract version
- **WHEN** acceptance reads the result
- **THEN** it MUST report incomplete evidence and fail rather than count the result as agreement

### Requirement: Foundation has no presentation or online side effects

Contract and numerical validation SHALL run without Godot, embedded Python, a graphical window, audio devices or a live game authority. Numerical behavior MUST have one production implementation; offline oracle work MUST NOT introduce another online writer.

#### Scenario: Headless foundation validation

- **GIVEN** no graphical host or Python presentation runtime is installed
- **WHEN** the foundation tests run
- **THEN** the tests MUST exercise contract and numerical behavior without accessing those dependencies or changing the default runtime
