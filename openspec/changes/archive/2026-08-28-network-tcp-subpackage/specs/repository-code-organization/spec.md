## ADDED Requirements

### Requirement: TCP transport has a one-way package boundary

The repository MUST place TCP listener, dial, stream, deadline, and socket
lifecycle implementation in `internal/network/tcp`. The `internal/network/tcp`
package MUST depend on `internal/network`, while `internal/network` MUST NOT
depend on `internal/network/tcp`. The root package MUST retain the shared packet
stream interfaces used by login and application assembly.

#### Scenario: Dependency graph accepts the TCP child package

- **GIVEN** the repository contains `internal/network/tcp`
- **WHEN** the architecture dependency check enumerates all internal packages
- **THEN** it MUST accept `internal/network/tcp -> internal/network`
- **AND** it MUST reject any reverse dependency from `internal/network` to
  `internal/network/tcp`

#### Scenario: TCP constructors are consumed from the child package

- **GIVEN** an application or test needs to create a TCP listener or dial a TCP
  stream
- **WHEN** it compiles against the reorganized repository
- **THEN** it MUST resolve the constructor through
  `internal/network/tcp`
- **AND** the returned values MUST satisfy the shared root-package listener or
  packet-stream interfaces

### Requirement: Transport package reorganization preserves transport behavior

The repository MUST preserve the existing Memory/TCP packet, login, close,
backpressure, deadline, peer-address, validation, and error semantics while
moving TCP implementation files between packages. The reorganization MUST NOT
change wire bytes, protocol state transitions, or test function names.

#### Scenario: Memory and TCP retain the same login contract

- **GIVEN** a client and server use either Memory transport or TCP transport
- **WHEN** they execute the existing handshake, login, and Play packet flow
- **THEN** both transports MUST continue to use the same login state machine and
  packet validation contract
- **AND** the resulting packet values and login outcomes MUST remain unchanged

#### Scenario: Existing transport tests remain addressable

- **GIVEN** the TCP white-box tests are moved into the TCP child package
- **WHEN** the reorganized repository runs its package tests
- **THEN** every existing TCP test function and subtest label MUST remain
  present without renaming
- **AND** Memory, codec, packet, and login test entry points MUST remain present
