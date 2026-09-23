## ADDED Requirements

### Requirement: Rust region banks preserve the fixed v1 format

The Rust region contract SHALL accept exactly 1,024 bank slots and SHALL NOT silently add, discard or index past slots supplied by a safe caller. It SHALL preserve the Go v1 superblock and bank geometry, signed region key bits, little-endian fields, CRC32C coverage and canonical zero padding. It MUST reject invalid entries before publishing a bank.

#### Scenario: Valid fixed bank

- **GIVEN** a region key with negative dimension and coordinates, a committed bank with 1,024 slots and a valid occupied extent starting at sector 15
- **WHEN** Rust encodes and decodes the region bank
- **THEN** the result MUST contain exactly the original 1,024 entries and the Go v1 byte layout

#### Scenario: Wrong slot cardinality

- **GIVEN** a safe caller supplies 1,023 or 1,025 entries for a new bank
- **WHEN** the bank is constructed for encoding
- **THEN** construction MUST fail before any bank bytes are published

#### Scenario: Invalid extent or reserved bytes

- **GIVEN** an occupied extent before sector 15, an overlapping or overflowing extent, or a bank with nonzero reserved or padding bytes and a resealed checksum
- **WHEN** Rust validates the bank
- **THEN** it MUST reject the complete bank as corrupt

### Requirement: Rust region encoding is atomic in caller buffers

The Rust region contract SHALL provide caller-buffer encoding for the 4,096-byte superblock and the 28,672-byte bank. A valid call MUST write exactly the format length and preserve any destination tail. An invalid bank MUST be reported before destination capacity, and every failed call MUST leave the entire destination unchanged.

#### Scenario: Short destination

- **GIVEN** a valid bank and a 28,671-byte destination filled with canary bytes
- **WHEN** Rust attempts caller-buffer encoding
- **THEN** it MUST report the required and available lengths and MUST leave every canary byte unchanged

#### Scenario: Invalid bank and short destination

- **GIVEN** an invalid occupied entry and a destination shorter than 28,672 bytes
- **WHEN** Rust attempts caller-buffer encoding
- **THEN** it MUST report the invalid bank first and MUST leave the destination unchanged

#### Scenario: Oversized destination

- **GIVEN** a valid superblock or bank and a destination longer than its format length
- **WHEN** Rust encodes into that destination
- **THEN** it MUST return the exact bytes written and MUST leave the remaining destination bytes unchanged

### Requirement: Region recovery selects only a valid committed bank

Rust region decoding SHALL distinguish unsupported future versions from corrupt records, reject truncated or trailing records, and enforce occupied extents against the declared file size. Recovery MUST select the newest valid nonzero-generation bank, prefer bank A on identical generation and content, and reject divergent equal-generation banks or two invalid banks.

#### Scenario: One committed bank survives corruption

- **GIVEN** bank A is corrupt and bank B is valid with nonzero generation
- **WHEN** Rust selects a region bank
- **THEN** it MUST select bank B without repairing or publishing bank A

#### Scenario: Equal generation conflict

- **GIVEN** two valid banks with the same nonzero generation but different entries
- **WHEN** Rust selects a region bank
- **THEN** it MUST reject the conflict instead of choosing a bank by position

#### Scenario: Future format version

- **GIVEN** a region superblock or bank whose version exceeds v1
- **WHEN** Rust decodes it
- **THEN** it MUST report a future-version failure rather than treating the record as a supported format
