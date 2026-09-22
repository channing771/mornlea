## Purpose

Define compatible shared Rust runtime contracts and reproducible offline evidence before server or client ownership migrates.

## ADDED Requirements

### Requirement: Contract coverage preserves supported behavior

The foundation SHALL inventory every currently supported protocol packet, save family, semantic input/event family and deterministic kernel with its source, version, owner and executable compatibility cases. It MUST reject completion when a supported family is missing, has no callable implementation, or has an unexplained incompatibility. Source-file existence and self-round-trip success MUST NOT substitute for independent behavioral evidence.

#### Scenario: A supported family is absent

- **GIVEN** the current runtime supports a family absent from the migration corpus
- **WHEN** foundation acceptance runs
- **THEN** it MUST identify the uncovered family and fail acceptance rather than infer parity from the covered subset

#### Scenario: Compatible wire and save records

- **GIVEN** versioned fixtures produced by the current supported implementation
- **WHEN** Rust decodes and re-encodes each supported record
- **THEN** the output MUST preserve the specified bytes, values and version behavior
- **AND** older supported save migrations MUST produce the same normalized result without changing source fixtures

#### Scenario: Compressed representations differ

- **GIVEN** a supported chunk save or chunk snapshot with zstd compression
- **WHEN** Go and Rust exchange encoded records
- **THEN** each decoder MUST recover exactly the same logical payload and normalized values with required integrity checks
- **AND** different compressed block bytes MUST NOT count as incompatibility for these two families

#### Scenario: A source file is not a callable packet

- **GIVEN** a packet has a source file but is absent from the compiled state-and-direction dispatch
- **WHEN** coverage acceptance runs
- **THEN** it MUST fail for that packet and MUST NOT count the source file as implementation evidence

### Requirement: Invalid inputs fail before publication

Contract consumers SHALL enforce structural length, capacity and integrity bounds before publishing decoded values, and enforce semantic admission before publishing accepted domain state. Malformed records and unsupported save versions MUST produce stable failures without partial state or implicit repair. Structurally valid handshake/login requests MUST remain observable to the login admission policy even when their version, identity or requested view distance is unacceptable. This exception MUST NOT admit an invalid session or permit invalid outbound requests.

#### Scenario: Malformed record

- **GIVEN** a truncated, oversized or invalid-enum record, or an unsupported save version
- **WHEN** either runtime processes it in the compatibility harness
- **THEN** it MUST reject the record with the recorded failure category and leave published state unchanged

#### Scenario: Login admission preserves explicit rejection

- **GIVEN** a structurally complete hello with a mismatched protocol, or a login request with an invalid identity or view distance
- **WHEN** the request crosses the inbound codec boundary
- **THEN** admission MUST be able to produce the existing version-mismatch, invalid-identity or protocol-violation response respectively
- **AND** a codec error MUST NOT replace that response, while truncated or trailing input MUST still fail structurally

### Requirement: Encoders preserve aggregate validity

Encoding SHALL validate the complete caller-provided value, including fixed cardinality, capacity, identities and cross-record relationships, before publishing bytes. Invalid values MUST return a failure without panic or partial output. Accepted encodings MUST be readable by both supported implementations with equal normalized values.

#### Scenario: An invalid container placement is encoded

- **GIVEN** an active furnace or chest refers to an out-of-range position, a duplicate occupied position, or a nonmatching block
- **WHEN** a chunk save is encoded
- **THEN** encoding MUST fail before publishing a compressed record

#### Scenario: Counts or public values are invalid

- **GIVEN** a region bank has other than 1024 entries, a companion save exceeds 64 stored records, or a packet carries a non-finite required angle or invalid selected slot
- **WHEN** encoding is requested
- **THEN** it MUST reject the value without panicking, silently padding, truncating or publishing unreadable bytes

### Requirement: Historical migration preserves associations

Supported save migration SHALL preserve the relationship between each entity identity, lifecycle and nonempty task queue. Migration MUST NOT orphan a queue, silently drop state, or repair inconsistent identities by guessing.

#### Scenario: A legacy companion has queued work

- **GIVEN** a schema 2, 3 or 4 companion record with a task, FIFO or summary
- **WHEN** the record is decoded and migrated
- **THEN** the queue MUST retain the owning companion's identity and content, matching the Go decoder

### Requirement: Semantic ordering preserves session behavior

Ordered input SHALL retain tick, session, sequence and arrival order. Within a tick, commands MUST sort by session and then sequence while retaining arrival order for equal keys. Different command kinds MUST NOT alter tie-breaking. Sequence admission remains a session-owner decision.

#### Scenario: Equal sequences have different command kinds

- **GIVEN** two commands from one session with the same sequence and different kinds, plus commands from another session
- **WHEN** an offline input ordering case runs
- **THEN** the per-session sequence ordering and first-arrival tie MUST match the authoritative Go path
- **AND** neither a lexical kind sort nor a global sequence-only sort may change the selected command

### Requirement: Replay identity supports offline differential comparison

A replay SHALL identify source revision, contract versions, content-derived corpus digest, seed, ordered input and tick schedule, and deterministic observations produced by executing the declared operation. Comparison MUST run Go and Rust independently offline; it MUST fail on missing identity, incomplete traces or unexplained outcome differences. F1 observations cover contract transformations and numerical operations; full authoritative gameplay state replay is a later runtime gate. Hashing source files or copying the same fixture digest into each checkpoint MUST NOT qualify as behavioral execution.

#### Scenario: Repeated deterministic replay

- **GIVEN** a complete frozen replay and independent Go and Rust runs
- **WHEN** both execute the same input schedule repeatedly
- **THEN** normalized state and event observations MUST agree at each declared checkpoint
- **AND** neither run MUST write to the other's state or live production saves

#### Scenario: Unidentified evidence

- **GIVEN** a result lacks a corpus digest or contract version
- **WHEN** acceptance reads the result
- **THEN** it MUST report incomplete evidence and fail rather than count the result as agreement

#### Scenario: Checkpoints or case coverage are incomplete

- **GIVEN** a trace omits a scheduled checkpoint or supported case, duplicates an input index, references an unknown family, or reports an unscheduled tick
- **WHEN** acceptance validates the trace against the frozen manifest
- **THEN** it MUST fail with the missing or inconsistent identity identified

#### Scenario: Behavior changes while sources remain discoverable

- **GIVEN** one codec result, migration association or numerical output is deliberately changed without removing its source file
- **WHEN** differential acceptance runs
- **THEN** it MUST detect the behavioral difference

#### Scenario: Output traverses a symbolic-link ancestor

- **GIVEN** an output path resolves into the repository through a symbolic-link ancestor followed by multiple nonexistent path components
- **WHEN** the offline harness prepares its output
- **THEN** it MUST reject the path before creating any directory or file in the repository

### Requirement: Resource work is bounded at the contract boundary

Each operation SHALL declare and enforce its maximum counts, byte lengths and deterministic work budget before proportional allocation or processing. Fixed-layout hot-path codecs MUST support caller-owned storage without per-field allocation. Performance measurements remain informational; overflow, data loss, bound violations and incomplete evidence MUST remain hard failures.

#### Scenario: Oversized decoded or declared input

- **GIVEN** a count or decompressed size exceeds its family limit or available input bytes
- **WHEN** the consumer validates the request
- **THEN** it MUST reject before allocating or processing proportionally to the untrusted size

#### Scenario: Path search exhausts its deterministic budget

- **GIVEN** a valid immutable grid whose goal would require expanding a 4097th node
- **WHEN** the numerical search runs
- **THEN** it MUST report budget exhaustion before that expansion, separately from unreachable
- **AND** equal-cost paths MUST retain the existing deterministic neighbor and insertion ordering

### Requirement: Foundation has no presentation or online side effects

Contract and numerical validation SHALL run without Godot, embedded Python, a graphical window, audio devices or a live game authority. Numerical behavior MUST have one production implementation; offline oracle work MUST NOT introduce another online writer.

#### Scenario: Headless foundation validation

- **GIVEN** no graphical host or Python presentation runtime is installed
- **WHEN** the foundation tests run
- **THEN** the tests MUST exercise contract and numerical behavior without accessing those dependencies or changing the default runtime


### Requirement: Format fidelity and admission are separate

Save codecs SHALL preserve raw fields that the supported Go format intentionally retains, including player armor triples and world metadata weather, spawn dimension and day-phase values. They MUST apply schema-specific migration defaults and format invariants without moving runtime normalization into the codec. Ordinary inventory stack admission SHALL remain distinct from raw equipped-armor persistence.

#### Scenario: Current metadata carries a raw weather value

- **GIVEN** a checksum-valid version6 metadata record with weather7, spawn dimension−3, a full-width day-phase value and valid difficulty
- **WHEN** Go and Rust decode and re-encode the record
- **THEN** both MUST preserve those raw values without weather clamping or codec rejection
- **AND** later runtime restoration remains responsible for interpreting them

#### Scenario: Equipped armor is preserved as stored

- **GIVEN** a supported player save whose armor slot contains a raw item/count/durability triple that ordinary inventory would reject
- **WHEN** the save codec performs a current-format round trip
- **THEN** the armor triple MUST remain unchanged
- **AND** the ordinary inventory stack validator MUST NOT be relaxed to make that armor triple a valid ordinary stack

### Requirement: Chat intake does not invent a command sequence

The shared input model SHALL preserve the distinction between sequenced play commands and chat intent. Chat MUST NOT acquire a synthetic wire sequence or be merged into sequenced-command ordering. Semantic events SHALL contain their actual value fields rather than only a digest or family label.

#### Scenario: Chat and equal-sequence commands arrive together

- **GIVEN** chat input with no sequence and two equal-sequence commands from one session
- **WHEN** the foundation converts and orders input
- **THEN** chat MUST remain a separate FIFO intent
- **AND** command tie-breaking MUST preserve first arrival independently of chat and command kind

### Requirement: Native numerical access preserves bounded ownership

Native Rust numerical callers SHALL use typed inputs, immutable views and explicitly owned reusable scratch, without encoding ABI request blobs. Each operation MUST publish only complete results, return a typed failure for invalid safe inputs and enforce its stated count/capacity/work bound. Legacy ABI symbols and successful supported outputs MUST remain compatible; malformed pointer aliases and late geometry failures MUST NOT publish partial payloads.

#### Scenario: A mesh fails after generating some geometry

- **GIVEN** a request that reaches geometry generation but fails packing or output capacity
- **WHEN** native or legacy ABI meshing returns failure
- **THEN** caller-visible payload bytes/elements MUST remain unchanged
- **AND** scratch may be reused only through its documented reset behavior

#### Scenario: ABI output metadata overlaps an input

- **GIVEN** a legacy numerical call whose output-count pointer aliases a buffer
- **WHEN** pointer and overlap validation rejects it
- **THEN** it MUST reject before clearing that aliased metadata or changing the buffer

#### Scenario: Native fluid work exceeds its batch limit

- **GIVEN** a native fluid-evaluation request with4097 items
- **WHEN** the safe Rust API validates it
- **THEN** it MUST reject before processing or publishing items
- **AND** the separate legacy ABI adapter MUST preserve its existing accepted larger-count behavior using the same bounded inner algorithm
