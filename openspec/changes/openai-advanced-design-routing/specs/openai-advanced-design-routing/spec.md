## Purpose

This capability guarantees that architecture, feature design, and other high-level system decisions use an advanced native OpenAI model with strong reasoning instead of a quota-preferred external worker.

## ADDED Requirements

### Requirement: High-level design tasks use advanced OpenAI

Before applying any quota score or backend preference, the router MUST classify a task as high-level when it requests or materially affects code architecture, package or module boundaries, ownership or dependency direction, lifecycle or concurrency design, protocol/ABI/storage contracts, a multi-component feature design, user-facing behavior across components, or another durable system-level decision. When classified as high-level, the router MUST filter the candidate set to native OpenAI-backed models and MUST select the highest eligible OpenAI model under the project ceiling, currently `gpt-5.6-sol`, with `high` or `max` reasoning effort. Z Code and other non-OpenAI backends MUST NOT compete through quota or routing scores for that task.

#### Scenario: Architecture task bypasses Z Code preference

- **GIVEN** a task proposes package boundaries, ownership rules, or a cross-module architecture
- **WHEN** the router evaluates native and Z Code candidates
- **THEN** it MUST select the highest eligible native OpenAI model with `high` or `max` reasoning before any Z Code quota score is considered

#### Scenario: Feature design uses the advanced OpenAI tier

- **GIVEN** a task defines a multi-component feature, user-facing behavior, or a durable API/contract
- **WHEN** the router builds the candidate set
- **THEN** only eligible native OpenAI models remain and the selected model MUST be the current advanced tier under the project ceiling

#### Scenario: Ordinary bounded implementation can still use Z Code

- **GIVEN** a task is a narrow implementation or review with no architecture, feature-design, contract, ownership, or high-level behavior decision
- **WHEN** the router evaluates candidates
- **THEN** the high-level hard gate MUST NOT apply and the existing validated Z Code quota policy MAY participate

### Requirement: Ambiguity and unavailable advanced models fail safely

If the task's level is ambiguous and could affect durable boundaries or multiple components, the router MUST classify it as high-level. If no eligible native OpenAI advanced configuration is exposed after capability discovery, the router MUST report that no compliant route is available and MUST NOT silently fall back to Z Code or another non-OpenAI backend. The router MAY use the next eligible native OpenAI model only when it still satisfies the project's advanced-tier policy; model availability MUST be verified from the live invocation surface.

#### Scenario: Ambiguous cross-package request is elevated

- **GIVEN** a request could be either implementation or a change to multiple package boundaries and the intent is not yet clear
- **WHEN** the router classifies the work
- **THEN** it MUST use the high-level gate and route only to the advanced native OpenAI candidate set

#### Scenario: No compliant model is available

- **GIVEN** capability discovery exposes no eligible advanced native OpenAI model with `high` or `max` reasoning
- **WHEN** the router attempts to route a high-level task
- **THEN** it MUST return a clear unavailable-route result and MUST NOT choose Z Code because its quota is positive

### Requirement: The hard gate is auditable and ordered

The routing decision MUST record the task classification, selected provider/backend, exact model, reasoning effort, and whether the high-level gate filtered candidates. The decision MUST show that classification and OpenAI eligibility filtering occurred before quota scoring. It MUST NOT record credentials or raw provider responses.

#### Scenario: Decision records the gate before quota

- **GIVEN** a high-level task has a healthy Z Code probe and a full quota snapshot
- **WHEN** the router records its decision
- **THEN** the record MUST identify the high-level classification and native OpenAI route, and MUST show that Z Code quota was not used to select the backend
