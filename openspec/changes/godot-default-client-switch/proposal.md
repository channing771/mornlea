## Why

The Godot pilot P7 review recorded Decision: GO in `docs/notes/godot-client-pilot-report.md`. P14 is a later, independently reviewable production step: switch the default graphical entry to Godot after a stable release and retire client ABI v19 only later. This candidate change exists so later work does not restart inside the pilot change. It MUST NOT be implemented as part of `pilot-godot-client-migration`.

## Target architecture gate

P14 is the final cutover, not a simple launcher switch. It is blocked until Rust F1–F3, all required Godot/Python presentation features, local/remote parity, packaging, replay compatibility, and a rollback release have passed. The cutover retires Go from the real-time product path and retires the pilot Go client-core ABI; it retains qualified embedded Python as the final Godot feature language. No dual-authority or partial deletion is allowed. See [`docs/architecture-target.md`](../../../../docs/architecture-target.md).

## What Changes

- Authorize a later OpenSpec apply for P14 after the pilot remains additive.
- Keep `apps/mornlea-godot/` as the stable project root.
- Keep the existing Rust `mornlea` client as the default entry until an explicit later switch.
- Switch the default graphical entry to Godot after a stable release and retire client ABI v19 only later.

## Capabilities

### New Capabilities

- `godot-default-client-switch`: Switch the default graphical entry to Godot after a stable release and retire client ABI v19 only later.

### Modified Capabilities

None in this candidate. The live `godot-client-pilot` contract remains the pilot boundary until this change is independently applied.

## Impact

- Compatibility: no protocol, save, or ABI change is authorized by creating this candidate.
- Default startup remains the existing Rust client.
- Rollback is to leave this change unimplemented; the pilot and old client stay as they are.
- Performance and concurrency contracts are unchanged until implementation is separately applied.
