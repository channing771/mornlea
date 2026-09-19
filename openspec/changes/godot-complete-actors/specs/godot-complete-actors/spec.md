## Purpose

Candidate P9 production route authorized by the Godot pilot GO decision. This specification is not live production behavior until the change is independently applied.

## ADDED Requirements

### Requirement: P9 stays a later catalog-replaceable production step

Godot actor features SHALL consume only typed entity snapshots, and unimplemented actor kinds MUST stay disabled rather than sending guessed commands. The existing Rust graphical client MUST remain the default entry until a later explicit switch. Implementation of this requirement is deferred until this change is independently applied.

#### Scenario: The candidate is not applied

- **GIVEN** this OpenSpec change exists as a candidate after P7 GO
- **WHEN** a user starts the game without selecting a later Godot production change
- **THEN** the existing Rust client SHALL remain the default graphical path
- **AND** this capability MUST NOT be treated as already implemented

#### Scenario: Later apply preserves replaceability

- **GIVEN** an independent apply of this change is requested
- **WHEN** the production feature is added or replaced
- **THEN** it SHALL enter through the Godot feature catalog
- **AND** it MUST NOT require recreating `apps/mornlea-godot/` or changing the pilot Bootstrap as the permanent main scene
