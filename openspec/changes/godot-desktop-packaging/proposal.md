## Why

The Godot pilot P7 review recorded Decision: GO in `docs/notes/godot-client-pilot-report.md`. P13 is a later, independently reviewable production step: add local Memory play and macOS/Windows/Linux desktop packaging for the stable Godot project root. This candidate change exists so later work does not restart inside the pilot change. It MUST NOT be implemented as part of `pilot-godot-client-migration`.

## Target architecture gate

Local play MUST use the same Rust server-core, login, packet, validation, persistence, and client-core semantics as remote TCP. An in-process loopback transport or a supervised server process may be selected during design, but a privileged Go Memory implementation is not a final option. The packaged client uses Godot embedded Python for presentation and keeps the standalone Agent separate. This candidate MUST wait for F2–F3. See [`docs/architecture-target.md`](../../../../docs/architecture-target.md).

## What Changes

- Authorize a later OpenSpec apply for P13 after the pilot remains additive.
- Keep `apps/mornlea-godot/` as the stable project root.
- Keep the existing Rust `mornlea` client as the default entry until an explicit later switch.
- Add local Memory play and macOS/Windows/Linux desktop packaging for the stable Godot project root.

## Capabilities

### New Capabilities

- `godot-desktop-packaging`: Add local Memory play and macOS/Windows/Linux desktop packaging for the stable Godot project root.

### Modified Capabilities

None in this candidate. The live `godot-client-pilot` contract remains the pilot boundary until this change is independently applied.

## Impact

- Compatibility: no protocol, save, or ABI change is authorized by creating this candidate.
- Default startup remains the existing Rust client.
- Rollback is to leave this change unimplemented; the pilot and old client stay as they are.
- Performance and concurrency contracts are unchanged until implementation is separately applied.
