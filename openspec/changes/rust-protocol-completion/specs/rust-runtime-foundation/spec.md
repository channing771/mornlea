## ADDED Requirements

### Requirement: Rust protocol covers every current packet key

The Rust protocol SHALL recognize exactly the current v45 direction, state and packet-ID combinations, preserve every accepted field and wire value, and reject unknown combinations, reserved IDs, malformed payloads and trailing bytes. Equivalent non-compressed packets MUST retain their Go wire bytes; a compressed chunk snapshot MUST retain its complete logical contents and cross-decoder interoperability even when two zstd encoders emit different compressed blocks. A packet identifier MUST NOT become a domain command or event field.

#### Scenario: Complete bidirectional packet coverage

- **GIVEN** the frozen current v45 registry and one valid Go-produced payload for each registered key
- **WHEN** the Rust protocol decodes and re-encodes each key in its declared direction and state
- **THEN** every packet MUST preserve all fields and the expected wire or logical snapshot result
- **AND** the number of executed keys MUST equal the independently discovered registry count

#### Scenario: Wrong state or reserved key

- **GIVEN** a valid Play payload presented under Login, a reversed direction, or reserved client Play ID 1
- **WHEN** Rust dispatches the record
- **THEN** it MUST reject the entire record before publishing a typed packet

### Requirement: Inbound negotiation separates structure from admission

Inbound handshake and login decoding SHALL preserve structurally valid values long enough to return the established version or login rejection. Identity, canonical display name and view-distance admission MUST follow the current decision order. Outbound records MUST remain fully validated before encoding. The protocol contract MUST leave session creation, deadlines, subscription distance clamping and transport ownership to the server.

#### Scenario: Old client version

- **GIVEN** a structurally valid handshake declaring an older protocol version
- **WHEN** Rust evaluates negotiation
- **THEN** the result MUST be a version mismatch carrying current version 45, rather than an unclassified decode error or an admitted session

#### Scenario: Multiple invalid login fields

- **GIVEN** a structurally valid login with an invalid identity and a view distance outside 2..=64
- **WHEN** Rust evaluates admission
- **THEN** invalid identity MUST win, and no admitted identity MUST be published

#### Scenario: Normalized inbound name

- **GIVEN** a valid raw UTF-8 login name with trim-only surrounding whitespace and a valid canonical result
- **WHEN** Rust evaluates admission
- **THEN** it MUST retain the canonical name and the declared in-range distance without silently clamping either

### Requirement: Protocol failures are bounded and atomic

Every frame and packet operation SHALL check canonical encoding, declared lengths, semantic values, count and decompression limits before proportional allocation or publication. Frame bodies MUST stay within 2 MiB, small packet payloads within 64 KiB, compressed snapshots within 1 MiB and decoded snapshots within 2 MiB. An invalid input, destination capacity failure or checked reservation failure MUST return a stable failure without a partial packet, changed caller output buffer or unbounded work. A coalesced input MAY contain later frames but a single read MUST consume only the first.

#### Scenario: Invalid value with a short destination

- **GIVEN** a mutable packet with an invalid field and an output buffer shorter than its valid encoding
- **WHEN** Rust attempts to encode it
- **THEN** field validation MUST fail before capacity reporting, and the output buffer MUST remain unchanged

#### Scenario: Declared oversized batch or snapshot

- **GIVEN** a declared record count or decompressed length above its packet-specific ceiling
- **WHEN** Rust decodes the payload
- **THEN** it MUST reject before allocating or scanning the declared content and MUST publish no partial record

#### Scenario: Two frames in one input

- **GIVEN** two canonical frames concatenated in one byte slice
- **WHEN** Rust reads one frame
- **THEN** it MUST return only the first frame and its exact consumed byte count, leaving the second available to the caller

### Requirement: Wire-to-semantic conversion preserves ownership

The Rust protocol SHALL convert validated Play client packets to the existing Rust semantic input types and validated server publication packets to the existing Rust semantic event types without inventing authority metadata, losing fields, changing record order or interpreting absence as a valid identity. Transport lifecycle packets MUST remain outside semantic commands and events. Protocol batch limits MUST remain distinct from the domain's larger semantic work bound.

#### Scenario: Valid semantic conversion

- **GIVEN** a valid command or publication with ordered records and all declared fields
- **WHEN** Rust converts it between wire and semantic values
- **THEN** the semantic result MUST retain every value and order, and a reverse conversion MUST reproduce an equivalent wire record

#### Scenario: Domain-valid value exceeds one wire batch

- **GIVEN** a valid semantic batch above its packet-specific count ceiling but within the domain's 4,096-record work cap
- **WHEN** Rust requests one wire packet
- **THEN** the conversion MUST fail explicitly rather than truncate, silently partition or weaken the domain type

#### Scenario: Transport control value

- **GIVEN** a handshake, login, keepalive or disconnect packet
- **WHEN** Rust requests a gameplay command or publication
- **THEN** it MUST reject that conversion without creating a placeholder semantic value

### Requirement: Protocol parity evidence is executable and source-bound

The protocol qualification SHALL execute independent Go producer behavior and Rust consumer behavior for every current packet family and declared operation. It MUST include valid, invalid and boundary observations, compare normalized outcomes rather than localized error prose, reject an absent or zero-case family, detect field or dispatch mutations, and bind all reviewed corpus assets to their source and content identities. Producers MUST write candidates only to a harness-owned temporary tree or an external export destination and MUST NOT rewrite tracked evidence.

#### Scenario: One packet family is missing

- **GIVEN** a frozen v45 family with no executed Rust case or a missing registered key
- **WHEN** protocol acceptance runs
- **THEN** coverage MUST identify the exact family/version/operation and remain incomplete

#### Scenario: One field or key changes

- **GIVEN** a deliberate mutation to one expected packet field, record order, direction, state or packet ID
- **WHEN** the Rust consumer compares its actual result with the source-bound case
- **THEN** qualification MUST fail even if the number of files and cases is unchanged

#### Scenario: Export targets tracked evidence

- **GIVEN** a producer export path inside the repository or through a symbolic-link ancestor
- **WHEN** it attempts to publish a candidate
- **THEN** it MUST refuse without modifying any tracked corpus asset
