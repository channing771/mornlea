# rust-runtime-foundation Specification

## Purpose

Define the reviewed offline evidence substrate and bounded shared Rust domain
contracts that later server, client, protocol, storage and numerical migration
changes can consume without changing the current online authority.

## Requirements

### Requirement: Working evidence and complete acceptance are distinct

The foundation SHALL provide an in-progress validation mode that validates every
present family and case while allowing explicitly uncovered families, and a
separate complete-acceptance mode that MUST reject every supported family or
version without executable coverage. Every case consumer identity MUST resolve
through a closed registry; a nonempty arbitrary string MUST NOT count as a
callable consumer.

#### Scenario: An in-progress manifest names future work

- **WHEN** a valid inventory contains a registered family with no cases and is
  checked in working mode
- **THEN** the present identities, sources and cases MUST be validated
- **AND** the uncovered family MUST remain explicitly visible rather than being
  counted as complete coverage

#### Scenario: Complete acceptance sees a zero-case family

- **WHEN** the same inventory is checked in complete-acceptance mode
- **THEN** validation MUST fail with the uncovered family and supported version
  identified

#### Scenario: A case names an unknown consumer

- **WHEN** a case names a consumer outside the closed callable registry
- **THEN** both working and complete validation MUST reject it

### Requirement: Trace observations come from executed operations

An offline trace SHALL contain observations produced by executing the selected
registered operations. The trace builder MUST NOT manufacture observations by
copying committed expected values. Validation MUST bind the trace to an explicit
corpus root and MUST fail on root resolution, file reads, digests, JSON shape or
normalized-value comparison errors.

#### Scenario: Behavior changes while expected files remain unchanged

- **WHEN** one selected operation returns a changed normalized scalar while the
  manifest, sources and expected files remain present
- **THEN** trace comparison MUST report the behavioral mismatch

#### Scenario: Expected evidence cannot be loaded

- **WHEN** an expected asset is missing, malformed, has a wrong digest or is
  resolved from a different corpus root
- **THEN** trace validation MUST fail before accepting the observation

### Requirement: Corpus generation never rewrites tracked evidence

Fixture producers SHALL write only into harness-owned temporary storage or a
fresh external export directory that passes the same containment, symlink and
no-replace checks as trace publication. Tests and commands MUST NOT rename,
replace or rewrite a tracked manifest or case asset. Committing generated output
remains a separately reviewed controller operation.

#### Scenario: A producer is asked to export into the repository

- **WHEN** a fixture producer receives an export path inside the repository or
  through a symbolic-link ancestor
- **THEN** it MUST reject the path without modifying tracked evidence

#### Scenario: A test checks symbolic-link rejection

- **WHEN** a test needs a malformed repository-shaped corpus
- **THEN** it MUST construct that corpus under harness-owned temporary storage
  rather than replacing a tracked fixture

### Requirement: Rust and Go enforce the same corpus integrity

For the shared manifest and case-asset subset it consumes, the Rust corpus
consumer SHALL enforce schema and case budgets, unique family/case identity,
family membership and case lists, supported case versions, ID prefixes, closed
operation/input-format/consumer values, nonempty decimal checkpoints,
repository-relative contained paths, every-component symbolic-link rejection,
regular-file and byte budgets, duplicate-key rejection and content digests
before returning a case. Live registry discovery, current identity comparison
and source-provenance comparison remain Go reconciliation responsibilities and
MUST NOT be claimed by the Rust loader.

#### Scenario: Corpus metadata is ambiguous

- **WHEN** a manifest contains a duplicate JSON key, unknown input format,
  unknown consumer or a case path with a symbolic-link ancestor
- **THEN** the Rust consumer MUST reject it before executing the case

### Requirement: Implemented domain cases execute independently in Rust

Every frozen case whose registered consumer is `mornlea_domain` SHALL execute
through the corresponding Rust domain constructor and compare its normalized
semantic outcome with the Go-produced expectation. Command expectations MAY
project out the Go wire bytes, because protocol encoding belongs to a successor
consumer, but every remaining field and every rejection MUST compare exactly.
The authoritative `domain.input` engine-step case MUST be registered to a
closed external Go consumer rather than misrepresented as executable by the
domain-only Rust orderer. Rust command ordering remains required through its
focused behavioral suite. Ownership MUST be bound by case consumer and family
identity, and a test filter that discovers zero cases MUST NOT qualify as
evidence.

#### Scenario: All baseline domain cases agree

- **WHEN** the Rust-owned baseline domain corpus is executed at its recorded identity
- **THEN** every selected case MUST run exactly once through Rust
- **AND** the normalized accepted value or typed rejection MUST equal the
  independently produced Go semantic result

#### Scenario: One domain outcome is mutated

- **WHEN** one executed domain value differs from the frozen Go expectation
- **THEN** the domain corpus gate MUST fail even though ownership strings and
  source files remain unchanged

### Requirement: Shared domain values preserve current semantics

The foundation SHALL preserve the current validated identity, text, item,
location, command and implemented publication semantics. Sequenced commands
MUST order by tick, session, sequence and arrival without a command-kind tie
breaker; chat MUST remain a separate unsequenced intent. Domain publication
records MUST preserve their validated source order and fields without wire IDs,
save migration policy or online authority state.

#### Scenario: Equal-sequence commands and chat arrive together

- **WHEN** two commands from one session share a sequence but have distinct
  arrival positions and chat arrives through its separate intake
- **THEN** the earlier command arrival MUST win the command ordering tie
- **AND** chat MUST retain no fabricated command sequence

#### Scenario: Implemented publication values are constructed

- **WHEN** player, world, inventory, remote-player or companion records are
  converted into shared domain values
- **THEN** their validated semantic fields and order MUST be preserved
- **AND** no packet ID, digest-only surrogate or authority-only state may replace
  those values

### Requirement: Domain construction is bounded before proportional work

Public domain validation SHALL enforce byte or record budgets before
proportional scanning, sorting or copying. Text validation MUST reject a display
name above 128 UTF-8 bytes before scalar iteration. Variable semantic batches
MUST reject more than 4096 records; smaller protocol packet limits remain
separate. Fallible temporary allocation MUST return a typed failure without
partial publication.

#### Scenario: An oversized semantic batch is submitted

- **WHEN** a safe Rust caller submits 4097 block changes, forgotten chunks,
  remote-player states or companion states
- **THEN** construction MUST return the bounded-batch failure before scanning,
  sorting or copying the records

#### Scenario: Bounded uniqueness scratch cannot be allocated

- **WHEN** uniqueness validation cannot reserve its bounded temporary storage
- **THEN** construction MUST return a typed allocation failure
- **AND** no partially validated value may be published

### Requirement: Baseline validation is headless and dependency-safe

The baseline SHALL run without a graphical host, audio device, embedded Python
runtime, live network authority or live save access. Foundation production
dependency direction MUST remain domain with no dependencies and protocol or
storage depending only on domain plus their approved compression dependency.

#### Scenario: Baseline gates run in a headless environment

- **WHEN** the evidence and domain validation suites run without presentation
  runtimes or a live server
- **THEN** they MUST execute their behavior and dependency checks successfully
  without changing the default Go runtime
