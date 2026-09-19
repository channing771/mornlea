## Why

The Godot pilot P7 review recorded Decision: GO in `docs/notes/godot-client-pilot-report.md`. P11 is a later, independently reviewable production step: enable desktop audio cues and optional controller input through replaceable platform adapters. This candidate change exists so later work does not restart inside the pilot change. It MUST NOT be implemented as part of `pilot-godot-client-migration`.

## What Changes

- Authorize a later OpenSpec apply for P11 after the pilot remains additive.
- Keep `apps/mornlea-godot/` as the stable project root.
- Keep the existing Rust `mornlea` client as the default entry until an explicit later switch.
- Enable desktop audio cues and optional controller input through replaceable platform adapters.

## Capabilities

### New Capabilities

- `godot-desktop-audio`: Enable desktop audio cues and optional controller input through replaceable platform adapters.

### Modified Capabilities

None in this candidate. The live `godot-client-pilot` contract remains the pilot boundary until this change is independently applied.

## Impact

- Compatibility: no protocol, save, or ABI change is authorized by creating this candidate.
- Default startup remains the existing Rust client.
- Rollback is to leave this change unimplemented; the pilot and old client stay as they are.
- Performance and concurrency contracts are unchanged until implementation is separately applied.
