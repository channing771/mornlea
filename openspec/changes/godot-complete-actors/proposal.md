## Why

The Godot pilot P7 review recorded Decision: GO in `docs/notes/godot-client-pilot-report.md`. P9 is a later, independently reviewable production step: migrate remaining actor presentation (companions, hostiles, passives, projectiles, viewmodel, effects) behind typed entity families. This candidate change exists so later work does not restart inside the pilot change. It MUST NOT be implemented as part of `pilot-godot-client-migration`.

## Target architecture gate

This candidate is an actor presentation slice, not a new gameplay authority. It MUST wait for Rust domain/server/client-core foundation stages F1–F3 in [`docs/architecture-target.md`](../../../../docs/architecture-target.md). Rust owns authoritative entity state, typed entity families, interpolation inputs, and semantic snapshots; Godot's embedded Python owns scenes, animation, pooling, and presentation resources. It MUST NOT add entity rules, protocol decoding, prediction, persistence, or numerical fallback to Go, Python, or GDScript.

## What Changes

- Authorize a later OpenSpec apply for P9 after the pilot remains additive.
- Keep `apps/mornlea-godot/` as the stable project root.
- Keep the existing Rust `mornlea` client as the default entry until an explicit later switch.
- Migrate remaining actor presentation (companions, hostiles, passives, projectiles, viewmodel, effects) behind typed entity families.

## Capabilities

### New Capabilities

- `godot-complete-actors`: Migrate remaining actor presentation (companions, hostiles, passives, projectiles, viewmodel, effects) behind typed entity families.

### Modified Capabilities

None in this candidate. The live `godot-client-pilot` contract remains the pilot boundary until this change is independently applied.

## Impact

- Compatibility: no protocol, save, or ABI change is authorized by creating this candidate.
- Default startup remains the existing Rust client.
- Rollback is to leave this change unimplemented; the pilot and old client stay as they are.
- Performance and concurrency contracts are unchanged until implementation is separately applied.
