## Purpose

Provide bounded desktop audio and device adapters with confirmed cue identity and device-free automated execution. This planned contract becomes live only after its implementation and prerequisite gates pass.

## ADDED Requirements

### Requirement: Confirmed semantic cues are consumed once

Desktop audio SHALL consume bounded Rust semantic cues with session identity and ordering. A confirmed cue MUST be consumed at most once; prediction, rejected commands, duplicates, and stale sessions MUST NOT create confirmation sounds.

#### Scenario: A new cue arrives twice

- **GIVEN** a valid cue has already been accepted for the active session
- **WHEN** the same or older cue is replayed
- **THEN** no second playback is scheduled, while the first valid cue is scheduled exactly once when the device is available

#### Scenario: The device is unavailable

- **GIVEN** audio initialization fails or the device is removed
- **WHEN** confirmed cues arrive
- **THEN** the client continues with an observable silent path, without retry storms, queued stale playback, or loss of authoritative confirmation

#### Scenario: Audio runs in headless or capture mode

- **GIVEN** a headless, automated capture, or no-device profile is active
- **WHEN** the desktop adapter initializes and processes cues
- **THEN** no audio device is opened and no device-dependent behavior blocks replay or teardown

### Requirement: Desktop input adaptation remains typed and bounded

Desktop adapters SHALL translate keyboard, mouse, optional controller, and focus events into bounded semantic intent. Device handling MUST NOT bypass Rust input validation or introduce mobile/Web/console lifecycles.

#### Scenario: Focus is lost with held controls

- **GIVEN** movement or controller input is held
- **WHEN** the application loses focus or the device disconnects
- **THEN** the next semantic input state releases held actions according to the contract and no background input remains stuck

#### Scenario: Input event capacity is exceeded

- **GIVEN** an adapter receives more events than its bounded queue permits
- **WHEN** the next batch is assembled
- **THEN** the contract reports overload or coalesces only permitted state events; it cannot truncate ordered actions while reporting success

### Requirement: Desktop adapters release devices across lifecycle changes

Audio and input adapters SHALL be independently disableable and SHALL release device handles, subscriptions, and pending work on teardown. Repeated initialization MUST not duplicate callbacks.

#### Scenario: A desktop adapter is repeatedly restarted

- **GIVEN** the same supported device profile is activated and deactivated repeatedly
- **WHEN** a final cue and input event are delivered
- **THEN** each reaches only the active adapter and no retired adapter retains a handle or callback

#### Scenario: A platform has no qualified adapter

- **GIVEN** the selected desktop target lacks accepted device/runtime evidence
- **WHEN** a release profile is assembled
- **THEN** the unsupported capability is disabled or the required profile fails explicitly; another platform result does not qualify it
