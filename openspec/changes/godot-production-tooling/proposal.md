## Why

The Godot pilot P7 review recorded Decision: GO in `docs/notes/godot-client-pilot-report.md`. P12 is a later, independently reviewable production step: move capture, benchmark, devcapture, import, and CI tooling onto Godot after a producer handoff. This candidate change exists so later work does not restart inside the pilot change. It MUST NOT be implemented as part of `pilot-godot-client-migration`.

## Target architecture gate

This candidate may move presentation evidence and tooling to Godot/Python only after the Rust replay, protocol, server, and client-core contracts needed by the tool are stable. Rust owns replay identities and performance-critical producers; Python/Godot owns bounded presentation tests. Go may remain an offline comparison oracle, but no tool may make Go and Rust concurrent online authorities. See [`docs/architecture-target.md`](../../../../docs/architecture-target.md).

## What Changes

- Authorize a later OpenSpec apply for P12 after the pilot remains additive.
- Keep `apps/mornlea-godot/` as the stable project root.
- Keep the existing Rust `mornlea` client as the default entry until an explicit later switch.
- Move capture, benchmark, devcapture, import, and CI tooling onto Godot after a producer handoff.

## Capabilities

### New Capabilities

- `godot-production-tooling`: Move capture, benchmark, devcapture, import, and CI tooling onto Godot after a producer handoff.

### Modified Capabilities

None in this candidate. The live `godot-client-pilot` contract remains the pilot boundary until this change is independently applied.

## Impact

- Compatibility: no protocol, save, or ABI change is authorized by creating this candidate.
- Default startup remains the existing Rust client.
- Rollback is to leave this change unimplemented; the pilot and old client stay as they are.
- Performance and concurrency contracts are unchanged until implementation is separately applied.
