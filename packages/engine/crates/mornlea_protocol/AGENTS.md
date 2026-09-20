# Protocol contracts

`packages/engine/crates/mornlea_protocol` owns versioned framing, negotiation,
and packet-family codecs. It is a windowless rlib. Production code may depend
only on `mornlea_domain` and must not depend on `mornlea_storage`,
`mornlea_engine`, `mornlea_client`, or `mornlea_godot`. Direction is enforced
by `tests/runtime_contract.rs` (`production_manifest_depends_only_on_domain`
and `domain_does_not_depend_on_protocol`).

## Inventory freeze (`src/lib.rs`, `tests/runtime_contract.rs`)

- Eventual owner of every `protocol.*` inventory row, including framing.
- Registration tests fail if the frozen corpus drops protocol rows or if
  production dependencies reverse onto domain or reach storage/kernel/host
  crates.
- Packet families are ported one inventory row at a time with byte-preserving
  and malformed-input cases; this crate must not infer parity from a covered
  subset.

## Framing (`src/frame.rs`, `src/varint.rs`, `tests/runtime_contract.rs`)

- `write_frame` / `read_frame` are the length-prefixed packet boundary.
  The length is a canonical uvarint and does not include itself.
- Empty, oversized, truncated, overlong, and non-canonical length prefixes
  fail before a payload is published
  (`frame_read_rejects_invalid_lengths_before_payload`,
  `frame_read_rejects_truncated_and_noncanonical_packet_id`,
  `frame_write_enforces_maximum_payload`).
- Capacity is `MAX_FRAME_BYTES`; do not copy that number here.
- Canonical uvarint vectors are pinned by
  `canonical_uvarint_round_trips_and_rejects_malformed`.
- Coalesced frames consume only one record
  (`frame_round_trip_preserves_packet_id_and_payload`).

## Client hello (`src/client_hello.rs`, `tests/runtime_contract.rs`)

- Handshake packet ID 0 payload is a canonical protocol-version uvarint.
- `ClientHello::new` / `decode` accept only `Identities::current().protocol`.
  Other versions are `UnsupportedVersion`; trailing bytes and truncated
  varints fail before publication
  (`client_hello_round_trip_preserves_current_version_bytes`,
  `client_hello_rejects_unknown_version_and_malformed_payload`).

## Server hello (`src/server_hello.rs`, `tests/runtime_contract.rs`)

- Handshake packet ID 0 payload is the same canonical protocol-version
  uvarint as ClientHello, on the server-to-client handshake ID space.
- `ServerHello::new` / `decode` accept only `Identities::current().protocol`.
  Other versions are `UnsupportedVersion`; trailing bytes and truncated
  varints fail before publication
  (`server_hello_round_trip_preserves_current_version_bytes`,
  `server_hello_rejects_unknown_version_and_malformed_payload`).

## Handshake reject (`src/handshake_reject.rs`, `src/bytes.rs`, `tests/runtime_contract.rs`)

- Handshake packet ID 1 payload is a canonical uvarint server protocol
  version, a one-byte reject code, and a length-prefixed UTF-8 message.
- Only `HANDSHAKE_VERSION_MISMATCH` (`1`) is a published code. The server
  version is informational and is not required to match the current
  protocol. Unknown codes are `InvalidEnum`; invalid UTF-8, oversized
  declared lengths, and oversized messages are `InvalidString`; trailing
  bytes fail before publication
  (`handshake_reject_round_trip_preserves_golden_bytes`,
  `handshake_reject_round_trip_preserves_empty_message`,
  `handshake_reject_rejects_unknown_code_and_malformed_payload`).
- `ByteEncoder` / `ByteDecoder` are crate-private payload primitives shared
  by later packet families. They are not a public codec surface.

## Login start (`src/login_start.rs`, `src/player_id.rs`, `tests/runtime_contract.rs`)

- Login packet ID 0 payload is a 16-byte UUIDv4, a length-prefixed display
  name, and a trailing view-distance byte.
- `PlayerId::new` accepts only non-zero UUIDv4 values. Display names must
  remain valid after trimming (`1..=32` runes, `<=128` bytes, no control
  characters). View distance is the closed interval `2..=64`. Failures are
  `InvalidIdentity`, `InvalidString`, or `InvalidRange` before publication
  (`login_start_round_trip_preserves_golden_bytes`,
  `login_start_rejects_invalid_identity_name_range_and_malformed_payload`).

## Login success (`src/login_success.rs`, `tests/runtime_contract.rs`)

- Login packet ID 0 payload is a 16-byte UUIDv4 followed by a little-endian
  `u64` world seed. Zero seeds are legal. Non-v4 identities, truncated
  payloads, and trailing bytes fail before publication
  (`login_success_round_trip_preserves_golden_bytes`,
  `login_success_rejects_invalid_identity_and_malformed_payload`).

## Login reject (`src/login_reject.rs`, `tests/runtime_contract.rs`)

- Login packet ID 1 payload is a one-byte reject code and a length-prefixed
  UTF-8 message (max 256 bytes/runes). Published codes are the closed
  interval `1..=7` copied from Go `LoginRejectCode`. Unknown codes are
  `InvalidEnum`; invalid UTF-8 and oversized declared lengths are
  `InvalidString`; trailing bytes fail before publication
  (`login_reject_round_trip_preserves_golden_bytes`,
  `login_reject_round_trip_preserves_empty_message_codes`,
  `login_reject_rejects_unknown_code_and_malformed_payload`).

## Disconnect (`src/disconnect.rs`, `tests/runtime_contract.rs`)

- Play packet ID 6 payload is a one-byte disconnect code and a
  length-prefixed UTF-8 message (max 256 bytes/runes). Published codes are
  the closed interval `1..=5` copied from Go `DisconnectCode`. Unknown
  codes are `InvalidEnum`; invalid UTF-8 and oversized declared lengths are
  `InvalidString`; trailing bytes fail before publication
  (`disconnect_round_trip_preserves_golden_bytes`,
  `disconnect_round_trip_preserves_empty_message_codes`,
  `disconnect_rejects_unknown_code_and_malformed_payload`).

## Keep alive (`src/keep_alive.rs`, `tests/runtime_contract.rs`)

- Play packet ID 5 payload is a little-endian `u64` token. Zero tokens are
  `InvalidRange`; truncated payloads and trailing bytes fail before
  publication (`keep_alive_round_trip_preserves_golden_bytes`,
  `keep_alive_rejects_zero_token_and_malformed_payload`).

## Keep alive reply (`src/keep_alive_reply.rs`, `tests/runtime_contract.rs`)

- Play packet ID 4 payload is a little-endian `u64` token. Zero tokens are
  `InvalidRange`; truncated payloads and trailing bytes fail before
  publication (`keep_alive_reply_round_trip_preserves_golden_bytes`,
  `keep_alive_reply_rejects_zero_token_and_malformed_payload`).

## Place block succeeded (`src/place_block_succeeded.rs`, `tests/runtime_contract.rs`)

- Play packet ID 20 payload is a little-endian `u64` sequence. Zero
  sequences are legal. Truncated payloads and trailing bytes fail before
  publication (`place_block_succeeded_round_trip_preserves_golden_bytes`,
  `place_block_succeeded_rejects_malformed_payload_and_accepts_zero_sequence`).

## Focused Verification

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked
```
