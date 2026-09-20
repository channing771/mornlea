## Context

See `docs/notes/godot-client-pilot-report.md` and `openspec/changes/pilot-godot-client-migration/design.md` matrix row P8. This candidate is planning-only.

## Target-boundary decision

This change may only implement the terrain presentation capability after Rust foundation stages F1–F3 have supplied authoritative world/protocol contracts and a Rust client-core semantic terrain family. Rust owns chunk interpretation, bulk mesh preparation, scheduling, budgets, and revisions; embedded Godot Python owns feature assembly and Godot resource application. The current Go runtime may be used as a replay oracle or compatibility adapter, but no new Go terrain ownership is permitted. Pure GDScript is not a feature implementation language.

## Goals / Non-Goals

**Goals:**

- Record the P8 production route so later work can apply it independently.
- Preserve catalog replaceability and the stable Godot project root.

**Non-Goals:**

- Implementing P8 inside the pilot change.
- Switching the default `mornlea` entry point.
- Relaxing visual thresholds or writing renderer-specific goldens.

## Decisions

### Candidate only

Creating this change authorizes later apply work. It does not modify runtime code, the default client, or tracked visual producers.

### Rejected alternatives

- Implementing P8 immediately in the pilot change: rejected because P7 only authorizes the split.
- Recreating a second Godot project root: rejected; `apps/mornlea-godot/` stays stable.
