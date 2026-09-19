## Context

See `docs/notes/godot-client-pilot-report.md` and `openspec/changes/pilot-godot-client-migration/design.md` matrix row P11. This candidate is planning-only.

## Target-boundary decision

Rust client-core produces validated semantic cue and device-intent events. Embedded Godot Python and Godot desktop APIs own playback, controller mapping, focus behavior, and resource lifecycle. The independent Agent Python runtime is not a dependency. The change must wait for F3 and must not add Go real-time logic, Python authority, numerical loops, or GDScript features.

## Goals / Non-Goals

**Goals:**

- Record the P11 production route so later work can apply it independently.
- Preserve catalog replaceability and the stable Godot project root.

**Non-Goals:**

- Implementing P11 inside the pilot change.
- Switching the default `mornlea` entry point.
- Relaxing visual thresholds or writing renderer-specific goldens.

## Decisions

### Candidate only

Creating this change authorizes later apply work. It does not modify runtime code, the default client, or tracked visual producers.

### Rejected alternatives

- Implementing P11 immediately in the pilot change: rejected because P7 only authorizes the split.
- Recreating a second Godot project root: rejected; `apps/mornlea-godot/` stays stable.
