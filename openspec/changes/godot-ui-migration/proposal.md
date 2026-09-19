## Why

The Godot pilot P7 review recorded Decision: GO in `docs/notes/godot-client-pilot-report.md`. P10 is a later, independently reviewable production step: choose and implement a production UI route (Godot Control or retained WebView) with versioned view-models. This candidate change exists so later work does not restart inside the pilot change. It MUST NOT be implemented as part of `pilot-godot-client-migration`.

## Target architecture gate

The final UI route is Godot Control driven by embedded Python and typed Rust semantic view-models. A retained WebView may remain as a rollback or compatibility producer during migration, but it is not the target route and must not receive new product UI ownership. This candidate MUST wait for Rust client-core stage F3 and MUST NOT introduce production GDScript, protocol access, Go mirror ownership, or a second UI state authority. See [`docs/architecture-target.md`](../../../../docs/architecture-target.md).

## What Changes

- Authorize a later OpenSpec apply for P10 after the pilot remains additive.
- Keep `apps/mornlea-godot/` as the stable project root.
- Keep the existing Rust `mornlea` client as the default entry until an explicit later switch.
- Choose and implement a production UI route (Godot Control or retained WebView) with versioned view-models.

## Capabilities

### New Capabilities

- `godot-ui-migration`: Choose and implement a production UI route (Godot Control or retained WebView) with versioned view-models.

### Modified Capabilities

None in this candidate. The live `godot-client-pilot` contract remains the pilot boundary until this change is independently applied.

## Impact

- Compatibility: no protocol, save, or ABI change is authorized by creating this candidate.
- Default startup remains the existing Rust client.
- Rollback is to leave this change unimplemented; the pilot and old client stay as they are.
- Performance and concurrency contracts are unchanged until implementation is separately applied.
