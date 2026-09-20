## Context

See `docs/notes/godot-client-pilot-report.md` and `openspec/changes/pilot-godot-client-migration/design.md` matrix row P14. This candidate is planning-only.

## Target-boundary decision

The default switch occurs only after the Rust server/client-core and Godot/Python presentation stack are the production path and the old release remains rollback-capable. The switch retires Go from real-time runtime ownership and removes the pilot Go client-core ABI only after release evidence; it does not remove the final embedded Python presentation runtime and does not allow a dual-authority period.

## Goals / Non-Goals

**Goals:**

- Record the P14 production route so later work can apply it independently.
- Preserve catalog replaceability and the stable Godot project root.

**Non-Goals:**

- Implementing P14 inside the pilot change.
- Switching the default `mornlea` entry point.
- Relaxing visual thresholds or writing renderer-specific goldens.

## Decisions

### Candidate only

Creating this change authorizes later apply work. It does not modify runtime code, the default client, or tracked visual producers.

### Rejected alternatives

- Implementing P14 immediately in the pilot change: rejected because P7 only authorizes the split.
- Recreating a second Godot project root: rejected; `apps/mornlea-godot/` stays stable.
