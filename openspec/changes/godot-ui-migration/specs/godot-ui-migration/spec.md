## Purpose

Provide production Godot Control UI over Rust semantic views, preserving tokens and confirmed interaction behavior. This planned contract becomes live only after its implementation and prerequisite gates pass.

## ADDED Requirements

### Requirement: UI presents Rust semantic views and submits validated intent

The Godot UI SHALL present versioned Rust semantic views and submit typed intent. Tokens, hits, confirmation, and rejection MUST follow Rust client-core contracts; local UI state MUST NOT become another authoritative mirror.

#### Scenario: A confirmed view is displayed

- **GIVEN** the active UI family supplies a consistent revision and valid tokens
- **WHEN** the user opens or updates a supported menu, HUD, or container
- **THEN** the controls show that view and enabled actions submit only typed intent bound to its tokens

#### Scenario: An action uses a stale token

- **GIVEN** a view token has expired or a command is rejected
- **WHEN** the user activates the stale control
- **THEN** the command is rejected or refreshed according to the contract without optimistic confirmation, duplicated inventory, or a guessed retry

#### Scenario: Focus and resize alter presentation

- **GIVEN** a valid UI view is displayed
- **WHEN** focus is lost, the window is resized, or the view is reset
- **THEN** input capture and controls update within the UI lifecycle while confirmed gameplay state and command ordering remain unchanged

### Requirement: The production UI uses the qualified presentation runtime

Production UI SHALL use Godot Control with embedded Python and typed semantic views. Retained WebView SHALL serve only the previous release or migration comparison path and MUST NOT receive new product UI ownership from this change.

#### Scenario: The qualified runtime is unavailable

- **GIVEN** the embedded runtime or required UI family fails qualification
- **WHEN** the production UI profile is selected
- **THEN** startup fails with a stable diagnostic; it does not fall back to GDScript, system Python, or a new WebView path

#### Scenario: The UI feature is disabled

- **GIVEN** a supported profile excludes the UI capability
- **WHEN** the host assembles its catalog
- **THEN** the feature allocates no controls or event subscriptions and other supported features retain their declared behavior

### Requirement: UI producer ownership follows reviewed fixture parity

UI SHALL pass semantic intent/view replay and complete per-fixture visual evidence before canonical ownership changes. Unaffected baselines MUST remain unchanged.

#### Scenario: A fixture cannot be compared

- **GIVEN** the candidate fixture lacks matching state, token, size, font, or asset identity
- **WHEN** UI acceptance runs
- **THEN** the fixture is non-comparable and blocks handoff, without relaxing tolerance or overwriting its canonical image

#### Scenario: UI differences are reviewed

- **GIVEN** all required fixture evidence is complete and intended rendering differences are listed
- **WHEN** human review and explicit per-fixture approval are recorded
- **THEN** the named fixtures can transfer producer through the explicit update path with a recoverable previous producer
