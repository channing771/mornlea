## Purpose

Candidate P8 production route authorized by the Godot pilot GO decision. This specification is not live production behavior until the change is independently applied.

## ADDED Requirements

### Requirement: P8 stays a later catalog-replaceable production step

The production Godot world feature SHALL present near-ring terrain at the approved view distance with bounded upload budgets, and the pilot world feature MUST remain independently replaceable through the catalog. The existing Rust graphical client MUST remain the default entry until a later explicit switch. Implementation of this requirement is deferred until this change is independently applied.

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
