## Why

The current router lets a healthy, quota-favored Z Code worker compete for some high-consequence work, but architecture and high-level product design need a stronger quality floor than ordinary bounded implementation. These decisions shape package boundaries, contracts, lifecycle, and user-visible behavior, so they must be handled by an advanced OpenAI model before any quota-based preference is applied.

## What Changes

- Add a hard pre-scoring gate for code architecture, feature design, and other high-level design tasks.
- Define the advanced OpenAI route as the highest eligible OpenAI model under the project ceiling, currently `gpt-5.6-sol`, with `high` or `max` reasoning.
- Require architecture/design tasks to remain on the native OpenAI candidate set; Z Code quota, the afternoon blackout, and other non-OpenAI scores MUST NOT bypass the gate.
- Treat ambiguous tasks as high-level when they affect multiple packages, durable contracts, ownership boundaries, or system behavior.
- If no eligible advanced OpenAI configuration is exposed, report the routing failure rather than silently selecting Z Code or another non-OpenAI backend.
- Synchronize the policy and capability-discovery guidance in both project skill copies and extend the project audit.

No game-runtime, save, network, ABI, or existing Z Code bridge command changes are introduced.

## Capabilities

### New Capabilities

- `openai-advanced-design-routing`: Mandatory advanced OpenAI routing for architecture and high-level design work.

### Modified Capabilities

None.

## Impact

- Affected documents: `.codex/skills/adaptive-model-router/` and `.claude/skills/adaptive-model-router/`, including capability-discovery guidance and the skill prompt metadata.
- Affected audit: `packages/audit/orchestration_skill_test.go` checks the hard gate and synchronized copies.
- Existing quota-aware Z Code routing remains available for eligible implementation, exploration, and review tasks outside the gate.
- No provider credential, model API, protocol, persistence, gameplay, concurrency, or performance contract changes are applicable.
- Rollback removes the new pre-scoring rule and audit fragments without changing the existing bridge or quota policy.
