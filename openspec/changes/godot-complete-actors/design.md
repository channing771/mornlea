## Context

See `docs/notes/godot-client-pilot-report.md` and `openspec/changes/pilot-godot-client-migration/design.md` matrix row P9. This candidate is planning-only.

## Goals / Non-Goals

**Goals:**

- Record the P9 production route so later work can apply it independently.
- Preserve catalog replaceability and the stable Godot project root.

**Non-Goals:**

- Implementing P9 inside the pilot change.
- Switching the default `mornlea` entry point.
- Relaxing visual thresholds or writing renderer-specific goldens.

## Decisions

### Candidate only

Creating this change authorizes later apply work. It does not modify runtime code, the default client, or tracked visual producers.

### Rejected alternatives

- Implementing P9 immediately in the pilot change: rejected because P7 only authorizes the split.
- Recreating a second Godot project root: rejected; `apps/mornlea-godot/` stays stable.
