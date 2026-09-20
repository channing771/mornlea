## Context

See `docs/notes/godot-client-pilot-report.md` and `openspec/changes/pilot-godot-client-migration/design.md` matrix row P12. This candidate is planning-only.

## Target-boundary decision

Tooling consumes the language-neutral Rust replay and report contracts. Godot/Python may produce bounded presentation evidence; Rust owns authoritative replay, protocol fixtures, and performance-critical producers. Go remains an offline differential oracle only, and no tooling workflow may create a second online authority or require the standalone Agent inside the client.

## Goals / Non-Goals

**Goals:**

- Record the P12 production route so later work can apply it independently.
- Preserve catalog replaceability and the stable Godot project root.

**Non-Goals:**

- Implementing P12 inside the pilot change.
- Switching the default `mornlea` entry point.
- Relaxing visual thresholds or writing renderer-specific goldens.

## Decisions

### Candidate only

Creating this change authorizes later apply work. It does not modify runtime code, the default client, or tracked visual producers.

### Rejected alternatives

- Implementing P12 immediately in the pilot change: rejected because P7 only authorizes the split.
- Recreating a second Godot project root: rejected; `apps/mornlea-godot/` stays stable.
