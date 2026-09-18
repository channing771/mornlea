## Purpose

This capability makes external Z Code allocation deterministic, quota-aware, and available throughout the afternoon without exposing credentials or sacrificing native fallback for risky work.

## ADDED Requirements

### Requirement: Quota-aware Z Code score

The router MUST obtain a fresh, non-secret, read-only quota snapshot before scoring an eligible Z Code candidate. The snapshot MUST accept `used`, `limit`, and `remaining` with `observed_at` and optional `reset_at`; when `remaining` is absent, the router MUST derive it as `limit - used`. For a valid snapshot, the router MUST compute `quota ratio = clamp(remaining / limit, 0, 1)` and apply the following modifier after the existing six-axis base score:

```text
ZCode score = base score × Z Code prior × quota factor × time factor
Z Code prior = 1.06
quota factor = 0.90 + 0.20 × quota ratio
```

The quota factor MUST therefore remain between `0.90` and `1.10`. A missing, malformed, stale, or otherwise unknown snapshot MUST use ratio `0.50`; it MUST NOT be interpreted as zero quota. A confirmed zero `remaining` value or provider rate-limit response MUST temporarily remove Z Code from the candidate set until reset or a fresh positive snapshot.

#### Scenario: Full quota receives a modest preference

- **GIVEN** the Z Code probe is healthy and a fresh snapshot reports `remaining = limit > 0`
- **WHEN** the router scores Z Code against otherwise eligible candidates
- **THEN** it MUST use ratio `1.00`, quota factor `1.10`, prior `1.06`, and the resulting adjusted score MUST compete in the normal candidate set

#### Scenario: Partial quota lowers allocation pressure

- **GIVEN** the Z Code probe is healthy and a fresh snapshot reports positive `remaining < limit`
- **WHEN** the router scores Z Code
- **THEN** it MUST derive the clamped ratio from `remaining / limit` and lower the quota factor continuously within the `0.90..1.10` bounds

#### Scenario: Unknown quota is neutral rather than exhausted

- **GIVEN** the snapshot is absent, malformed, lacks a positive `limit`, or is older than 15 minutes
- **WHEN** the router scores Z Code
- **THEN** it MUST classify the value as unknown quota, use ratio `0.50`, retain the modest prior, and keep Z Code eligible if no other failure exists

#### Scenario: Confirmed exhaustion falls back

- **GIVEN** the provider reports `remaining = 0` or returns a rate-limit response
- **WHEN** the router builds the candidate set
- **THEN** it MUST omit Z Code for that decision and retain the next eligible native or external fallback

### Requirement: Afternoon Z Code eligibility

The router MUST use the host's local timezone for its time check. During the stated `14:00–18:00` local window, a healthy Z Code probe with quota that is not confirmed exhausted MUST remain eligible. The time factor MUST remain `1.00`; the router MUST NOT create a time-based blackout or set the Z Code score to zero. Actual capability, quota, provider, user, or validation constraints MAY still select a fallback.

#### Scenario: Healthy Z Code remains available in the afternoon

- **GIVEN** local host time is within `14:00–18:00`, the Z Code probe is healthy, and quota is fresh or unknown but not confirmed exhausted
- **WHEN** the router evaluates candidates
- **THEN** Z Code MUST remain in the candidate set with time factor `1.00`

#### Scenario: Afternoon provider failure still falls back

- **GIVEN** local host time is within `14:00–18:00` but the Z Code probe or provider request fails
- **WHEN** the router evaluates candidates
- **THEN** it MUST use the normal fallback path for that failure and MUST NOT fabricate Z Code availability

### Requirement: Credential-safe quota observation

Quota discovery MUST use only a read-only account-scoped host or provider source, or a non-secret snapshot supplied by that source. The router MUST NOT send an inference request solely to discover quota, print credentials, persist raw account responses, or include credentials in a routing decision. A routing record MAY retain only the normalized ratio, freshness classification, local timezone, and observation timestamp.

#### Scenario: Read-only snapshot does not disclose secrets

- **GIVEN** a quota source returns usage data together with provider authentication material
- **WHEN** the router records or reports its routing decision
- **THEN** it MUST retain only normalized non-secret quota and timing fields and MUST omit credentials and raw account payloads

#### Scenario: Unavailable quota source remains safe

- **GIVEN** no read-only quota source is available
- **WHEN** the router prepares a Z Code candidate
- **THEN** it MUST use the unknown-quota behavior without issuing a paid inference probe or treating the entitlement as exhausted
