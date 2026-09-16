## Why

Mornlea's current governance documents encode a mandatory per-task subagent implementation/review loop, Chinese-first documentation, and Chinese code comments. Those rules now conflict with the requested OpenAI-native orchestration policy, English-first OpenSpec and planning artifacts, bilingual explanatory documentation, and English-only code comments; the existing documentation also contains stale ABI and Hook claims that demonstrate the need for enforceable synchronization.

## What Changes

- Introduce a provider-aware implementation policy: verified OpenAI ChatGPT/Codex controllers have standing project authorization to choose direct, delegated, or mixed execution without a separate per-task user request, while non-OpenAI or unknown-provider controllers retain the strict `subagent-driven-development` loop.
- Make main-agent execution the OpenAI-native default. Delegate only when isolating a large, noisy, or specialized feature/review context prevents it from polluting the main agent's reasoning; do not delegate merely for parallel speed, and never run more than two subagents concurrently.
- Before an approved subagent delegation, route the subtask through synchronized project-owned `adaptive-model-router` skills using the live host capability set, the configured OpenAI ceiling, and a multi-axis assessment of complexity, consequence, context, tools, validation, and user priorities. Treat code quality and token efficiency as coequal objectives; do not default every subagent to the highest model or under-route risky work merely to save tokens.
- Add a project-owned implementation-orchestration skill for Codex and Claude entry points instead of modifying cached third-party skills.
- Add a synchronized project architecture skill. At the end of each implementation round, the controller reviews verified architectural discoveries and promotes only stable, cross-task conventions into that skill; when nothing qualifies, it records a no-change retrospective instead of growing the skill mechanically.
- At the same round boundary, review model routing for over-routing, under-routing, retries, escalation, context-transfer cost, validation quality, and token use. Improve both project router copies only for verified, reusable rules; otherwise record an explicit no-change result.
- Make English the canonical language for active/new OpenSpec artifacts, new or substantively revised project plans, task briefs, ledgers, machine-oriented governance files, and source-code comments; grandfather unchanged canonical-spec debt through a non-growth inventory.
- Make new or substantively revised explanatory and architectural documentation use an English canonical `*.md` file with a synchronized Chinese `*.zh.md` counterpart; classify unchanged pre-policy documents as legacy until their first substantive revision. New policy MUST NOT introduce `*.en.md` names.
- Migrate the existing root `README.md`/`README.en.md` pair to English `README.md` plus Chinese `README.zh.md`, remove the obsolete `README.en.md` path, update repository links, and repair stale engine/client ABI and Hook statements in current documentation.
- Add a source-comment language gate that grandfathers existing legacy comments but rejects new non-English comments, non-English comments in new architecture code, and non-English rewrites of grandfathered comments.
- Preserve the three existing visual-evidence classes across render hosts. Godot pilot captures remain untracked comparison evidence until a separately approved producer handoff transfers an existing `world/`, `ui/`, or human-review motion GIF producer to Godot.
- Add bilingual-document pairing/revision checks, legacy-document classification, current-version consistency checks, provider-policy consistency checks, and visual-baseline ownership checks to the audit surface.
- Preserve archived OpenSpec changes, historical plans, and unchanged pre-policy canonical specifications as grandfathered evidence; apply the English-only rule to active, new, or substantively revised planning prose.

User-visible result: contributors and coding agents receive one coherent set of current rules, readers can navigate matching English and Chinese explanatory documents, new code comments are consistently English, and introducing the Godot renderer does not fork the visual-baseline taxonomy.

Non-goals: this change does not modify gameplay, protocol, persistence, runtime behavior, visual pixels, comparison thresholds, or production renderer ownership. It does not bulk-translate existing code comments, unchanged pre-policy documentation, canonical specifications, archived changes, or historical plans, and it does not complete the Godot client migration.

## Capabilities

### New Capabilities

- `development-governance`: Defines provider-aware implementation orchestration, artifact and documentation language policy, forward-looking code-comment rules, synchronization metadata, and enforceable repository gates.

### Modified Capabilities

- `visual-verification`: Extends the existing visual-verification contract so multiple renderer hosts share the same semantic baseline classes, pilot evidence cannot mutate tracked goldens, and producer ownership changes only through an explicit handoff.

## Impact

- Governance sources: root and scoped `AGENTS.md` files, `openspec/config.yaml`, `docs/development-process.md`, `docs/openspec.md`, `docs/test-organization.md`, and `docs/agents-md-style.md`.
- Project skills: new mirrored `mornlea-implementation-orchestration`, `adaptive-model-router`, and `mornlea-architecture` skills for Codex/Claude, plus updates to the mirrored `visual-baseline` skills.
- Documentation: English canonical `*.md` plus Chinese `*.zh.md` for migrated, new, or substantively revised explanatory and architectural documents; unchanged pre-policy documents are explicitly classified as legacy until revision. Existing `*.en.md` paths are migrated and removed rather than retained as parallel sources.
- Gates and tooling: `packages/audit`, `scripts/agents`, Makefile, and CI checks for language, pairing, revisions, current version claims, orchestration policy, and visual ownership.
- Grandfather inventories: the token-aware baselines identify 1,243 first-party source files and 38,636 comment lines containing Han characters plus 112 canonical OpenSpec specs containing 10,656 Chinese prose lines. Seventeen unchanged pre-policy explanatory documents remain explicitly classified as legacy. These inventories prevent new debt but are not bulk-migration commitments; four active Godot-change artifacts were translated immediately, and 917 archived OpenSpec artifacts remain historical evidence.
- Compatibility: no protocol, save schema, engine ABI, client ABI, or benchmark scenario change. Repository-owned links are updated atomically; the obsolete external `README.en.md` URL is an intentional documentation-path break required by the new naming convention.
- Concurrency and performance: no runtime concurrency change. New repository gates must be bounded and must not add visual capture or full-repository translation work to ordinary focused test loops.
