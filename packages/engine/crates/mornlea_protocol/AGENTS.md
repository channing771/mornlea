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

## Focused Verification

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked
```
