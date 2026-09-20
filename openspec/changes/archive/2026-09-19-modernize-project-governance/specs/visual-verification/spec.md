## ADDED Requirements

### Requirement: Visual baseline classes are independent of renderer host

The repository SHALL classify tracked visual evidence by observable semantics rather than by the renderer that produced it: window or UI fixtures belong to `ui/`, stable headless world frames belong to `world/`, and cross-tick behavior belongs to the existing `motion/` GIF class for bounded human review without automated pixel comparison. Adding a Godot renderer MUST NOT create a fourth tracked class or silently duplicate the same behavior under a renderer-specific golden tree.

#### Scenario: Godot pilot produces comparison evidence

- **GIVEN** the Godot client is still a pilot and the existing Rust client remains the canonical producer
- **WHEN** the Godot client captures a comparable UI, world, or cross-tick scene
- **THEN** the output SHALL be written as untracked, identity-complete comparison evidence outside `testdata/visual-golden/`
- **AND** it MUST NOT create, replace, or update a tracked golden

#### Scenario: Godot feature becomes the canonical producer

- **GIVEN** a separately approved migration change has established behavioral parity for a Godot feature
- **WHEN** visual ownership is transferred from the current producer to Godot
- **THEN** the change MUST identify the existing semantic class, scene or fixture identity, old producer, new producer, affected goldens, and rollback path
- **AND** the new producer MUST use the existing explicit-update and human-review discipline before any tracked golden changes

#### Scenario: Proposed renderer-specific baseline directory

- **GIVEN** a change proposes a tracked directory such as `testdata/visual-golden/godot/`
- **WHEN** visual-baseline routing validation runs
- **THEN** validation MUST reject the renderer-specific class
- **AND** require the evidence to be routed to `ui/`, `world/`, the human-review `motion/` GIF class, or an untracked pilot-evidence directory

### Requirement: Visual producer handoff preserves comparison discipline

A visual producer handoff SHALL preserve the existing scene semantics, capture boundaries, explicit update authorization, human inspection requirement, difference artifacts, and bounded frame budgets. Renderer-specific antialiasing, rasterization, or color-pipeline differences MUST be recorded and reviewed; they MUST NOT be accepted by weakening existing thresholds or by overwriting unrelated baselines.

#### Scenario: Handoff changes expected pixels

- **GIVEN** an approved producer handoff is expected to change pixels while preserving scene semantics
- **WHEN** candidate captures are generated
- **THEN** every affected image or GIF MUST be reviewed before the explicit update command writes tracked goldens
- **AND** unaffected baselines MUST remain byte-identical

#### Scenario: Pilot comparison exceeds current tolerance

- **GIVEN** a Godot pilot image differs from the canonical baseline beyond the current comparison tolerance
- **WHEN** the pilot report is generated
- **THEN** the report MUST classify and retain the difference as evidence
- **AND** it MUST NOT change the canonical threshold or baseline merely to make the pilot pass
