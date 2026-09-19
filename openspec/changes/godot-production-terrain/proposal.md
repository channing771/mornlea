## Why

The Godot pilot P7 review recorded Decision: GO in `docs/notes/godot-client-pilot-report.md`. P8 is a later, independently reviewable production step: productionize Godot near-ring terrain into replaceable world features with LOD, water, cutout, fog, lighting, and resource pools. This candidate change exists so later work does not restart inside the pilot change. It MUST NOT be implemented as part of `pilot-godot-client-migration`.

## Target architecture gate

This candidate is a terrain presentation slice, not permission to expand the pilot's Go client-core. It MUST wait for the Rust foundation stages F1–F3 in [`docs/architecture-target.md`](../../../../docs/architecture-target.md): Rust owns terrain data, bulk preparation, mesh scheduling, and semantic world publication; Godot's embedded Python owns the presentation feature and resource lifecycle. No new terrain rule, protocol, prediction, or numerical fallback may be added to Go, Python, or GDScript. The candidate remains independently reversible until a producer handoff is approved.

## What Changes

- Authorize a later OpenSpec apply for P8 after the pilot remains additive.
- Keep `apps/mornlea-godot/` as the stable project root.
- Keep the existing Rust `mornlea` client as the default entry until an explicit later switch.
- Productionize Godot near-ring terrain into replaceable world features with LOD, water, cutout, fog, lighting, and resource pools.

## Capabilities

### New Capabilities

- `godot-production-terrain`: Productionize Godot near-ring terrain into replaceable world features with LOD, water, cutout, fog, lighting, and resource pools.

### Modified Capabilities

None in this candidate. The live `godot-client-pilot` contract remains the pilot boundary until this change is independently applied.

## Impact

- Compatibility: no protocol, save, or ABI change is authorized by creating this candidate.
- Default startup remains the existing Rust client.
- Rollback is to leave this change unimplemented; the pilot and old client stay as they are.
- Performance and concurrency contracts are unchanged until implementation is separately applied.
