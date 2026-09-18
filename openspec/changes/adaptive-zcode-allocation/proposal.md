## Why

The synchronized Mornlea router can invoke the configured Z Code `GLM-5.3` worker, but its allocation guidance does not yet turn the current entitlement into a deterministic routing signal. A modest Z Code preference and a temporary afternoon blackout also need explicit, reviewable policy so healthy Z Code capacity remains usable from 14:00 through 18:00 local time.

## What Changes

- Add a quota-aware Z Code allocation policy to both synchronized `adaptive-model-router` skills.
- Define a non-secret, read-only quota snapshot contract with freshness, derivation, clamping, unknown-quota, and confirmed-exhaustion behavior.
- Apply a modest Z Code prior and a bounded quota factor to the existing six-axis routing score.
- Keep Z Code eligible during the local 14:00–18:00 window when its capability probe is healthy and quota is not confirmed exhausted.
- Preserve native fallback for high-consequence work, weak validation oracles, capability failures, provider rate limits, and confirmed zero quota.
- Extend the project audit so the two skill copies cannot silently lose the quota and time-window rules.

No breaking command, protocol, save, ABI, or game-runtime change is introduced.

## Capabilities

### New Capabilities

- `zcode-quota-aware-routing`: Deterministic quota-aware allocation and local-time eligibility rules for the external Z Code worker.

### Modified Capabilities

None.

## Impact

- Affected documents: synchronized `.codex/skills/adaptive-model-router/` and `.claude/skills/adaptive-model-router/` policy and capability-discovery files.
- Affected audit: `packages/audit/orchestration_skill_test.go` checks synchronized policy fragments.
- The quota snapshot is read-only and non-secret; no new package, credential store, provider endpoint, or inference request is added.
- No world state, save format, network protocol, client/server ABI, or benchmark scenario changes are applicable.
- Routing work remains bounded policy evaluation. Snapshot acquisition must not block an authoritative game or render path, and an unavailable snapshot uses the documented neutral behavior.
- Rollback is limited to removing the new policy and audit fragments; existing Z Code bridge commands and native fallback remain compatible.
