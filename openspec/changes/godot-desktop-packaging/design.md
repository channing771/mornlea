## Context

See `docs/notes/godot-client-pilot-report.md` and `openspec/changes/pilot-godot-client-migration/design.md` matrix row P13. This candidate is planning-only.

## Target-boundary decision

Local and remote play use one Rust server-core and one login/packet/validation path. The implementation may select an in-process loopback transport or a supervised server process, but must not create a privileged Go local simulation. The packaged presentation is Godot with embedded Python; the standalone Agent remains a separate optional service. F2–F3 are prerequisites.

## Goals / Non-Goals

**Goals:**

- Record the P13 production route so later work can apply it independently.
- Preserve catalog replaceability and the stable Godot project root.

**Non-Goals:**

- Implementing P13 inside the pilot change.
- Switching the default `mornlea` entry point.
- Relaxing visual thresholds or writing renderer-specific goldens.

## Decisions

### Candidate only

Creating this change authorizes later apply work. It does not modify runtime code, the default client, or tracked visual producers.

### Rejected alternatives

- Implementing P13 immediately in the pilot change: rejected because P7 only authorizes the split.
- Recreating a second Godot project root: rejected; `apps/mornlea-godot/` stays stable.
