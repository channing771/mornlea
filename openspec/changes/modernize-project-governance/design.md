## Context

See [proposal.md](proposal.md) for motivation. Current policy is distributed across root and scoped `AGENTS.md` files, `openspec/config.yaml`, project-owned OpenSpec skills, `docs/development-process.md`, `docs/openspec.md`, test-organization guidance, visual-baseline skills, and visual documentation. Several sources now contradict the requested policy and each other:

- root guidance, OpenSpec apply guidance, and development-process documentation unconditionally require one implementer and one reviewer per task;
- root, Rust, and test-organization guidance require Chinese source comments;
- OpenSpec configuration requires Chinese proposals and specifications;
- `docs/openspec.md` still describes installed Hooks although the root and script guidance say those Hooks were removed;
- current README and architecture sections still contain engine ABI v10/client ABI v14 claims while code and the root baseline are v11/v19;
- visual routing is tied to the current Rust/WebView producers even though the Godot pilot introduces a second capture host.

The token-aware implementation baseline on 2026-09-15 found 1,243 first-party source files and 38,636 comment lines containing Han characters. It also found 112 canonical OpenSpec specifications containing Chinese, four active Godot-change artifacts containing Chinese, 917 archived OpenSpec artifacts containing Chinese, 141 historical project plan/design files containing Chinese, and 36 non-superpowers Markdown documents without pairs under the new naming convention. These counts are baseline observations; later gates derive current values rather than hard-coding them as permanent product constants.

The worktree already contains unrelated user changes, including client-render code, one world golden, a main visual specification, plans, and SDD progress. Governance implementation must not translate, rewrite, stage, or otherwise absorb those changes without their owners' explicit scope.

## Goals / Non-Goals

**Goals:**

- Establish one provider-aware implementation policy that every current governance surface states consistently.
- Make English the canonical language for planning, governance, and source comments.
- Establish `*.md` as the English canonical explanatory/architectural document and `*.zh.md` as its synchronized Chinese counterpart.
- Introduce bounded migration gates that improve monotonically without forcing one unreviewable repository-wide rewrite.
- Preserve the three semantic visual-baseline classes while allowing Rust, WebView, and Godot producers to coexist during migration.
- Repair current Hook, ABI, documentation-link, and language-policy drift.

**Non-Goals:**

- Do not change gameplay, protocol, save formats, runtime ownership, ABI numbers, benchmark scenario semantics, visual pixels, or comparison thresholds.
- Do not translate archived OpenSpec changes or historical plans merely for style consistency.
- Do not edit third-party licenses, generated files, localized user-visible strings, wire payloads, or test fixtures under the source-comment rule.
- Do not implement the Godot client; only align governance and visual ownership with its approved migration plan.

## Decisions

### 1. Separate repository governance from the Godot client change

`modernize-project-governance` owns global language, agent orchestration, documentation pairing, visual routing, and their gates. `pilot-godot-client-migration` owns the Godot application root, feature layout, bridge, runtime extraction, pilot evidence, and migration decision.

The Godot change will be revised to consume the new rules: its artifacts become English, its explanatory-document tasks produce `*.md`/`*.zh.md` pairs, its new source comments use English, and its visual tasks produce pilot evidence without changing tracked goldens. It will not own the global comment policy or provider policy.

Rejected alternative: adding all governance work to the Godot change. That would make a renderer pilot responsible for unrelated source comments, Claude/Codex policy, every current document, and repository-wide CI behavior, preventing an independent rollback.

### 2. Use an explicit language classification and naming contract

Language ownership is:

| Artifact class | Canonical language and naming | Migration treatment |
|---|---|---|
| New or substantively revised explanatory or architectural documentation | English `name.md`; Chinese `name.zh.md` | Both sides required and revision-locked |
| Unchanged pre-policy explanatory or architectural documentation | Existing path and language | Classify as `legacy`; migrate atomically on first substantive revision |
| Active/new OpenSpec artifacts | English | Translate active change immediately; reject new non-English prose |
| Unchanged pre-policy canonical OpenSpec specifications | Existing language | Grandfather through a fixed non-growth inventory; translate only through separately approved work |
| New or substantively revised project plans, briefs, and ledgers | English | Historical evidence remains unchanged |
| `AGENTS.md`, project `SKILL.md`, OpenSpec config, schemas, gate configuration | English | One normative machine-oriented copy |
| New or substantively rewritten first-party source comments and doc comments | English | Grandfather unchanged legacy comment text; reject new debt |
| User-visible localized strings and localization assets | Product language as specified | Explicitly outside comment-language checks |
| Third-party licenses, vendored and generated sources | Original/generated form | Excluded by recorded path rules |

`docs/documentation-manifest.json` classifies Markdown documents as `bilingual`, `legacy`, `plan`, `historical`, `machine`, `license`, or `generated`. A `legacy` entry identifies an unchanged pre-policy explanatory document that remains in its existing language until substantive revision. For a bilingual entry the manifest records `doc_id`, English path, Chinese path, and revision. The same identity and revision are also present in each paired document's front matter so a moved or copied file cannot silently assume another document's classification.

For every new or migrated pair, the English file is the unsuffixed `*.md` and Chinese is `*.zh.md`. New `*.en.md` files are forbidden. A legacy document cannot be edited substantively while remaining `legacy`; its first substantive revision creates both paired paths and matching metadata atomically. The existing root migration is:

```text
before: README.md       = Chinese primary with embedded English summary
        README.en.md    = English document

after:  README.md       = English canonical document
        README.zh.md    = Chinese counterpart
```

`README.en.md` is removed in the same atomic change and every repository-owned link switches to `README.md` or `README.zh.md`. This intentionally prefers an unambiguous two-file contract over preserving an obsolete third filename.

Rejected alternatives:

- Chinese at `*.md` and English at `*.en.md`: directly contradicts the requested convention.
- Both languages in one document: doubles navigation and merge conflicts, and cannot prove synchronized headings or links cleanly.
- Detecting document class solely from directory names: `docs/notes/` currently mixes current guidance, reports, and historical evidence, so a manifest is required.
- Bulk-translating every pre-policy document in this governance change: stale documents require independent factual review and would make the governance diff unreviewable.

### 3. Pair synchronization uses identity/revision metadata plus semantic invariants

A bilingual document starts with equivalent metadata:

```yaml
---
doc_id: architecture
doc_revision: 2026-09-15.1
language: en
counterpart: architecture.zh.md
---
```

The Chinese side uses the same `doc_id` and revision, `language: zh-CN`, and points back to the English file. Audit checks:

1. both paths exist and are uniquely classified;
2. identity, revision, and reciprocal paths match;
3. local links resolve;
4. fenced commands, referenced source paths, and versioned protocol/schema/ABI/benchmark claims agree;
5. current-version claims match code-owned sources;
6. a changed bilingual document cannot pass with only one side's revision advanced.

The audit cannot prove natural-language translation quality. Human review remains responsible for semantic equivalence beyond mechanically comparable claims.

### 4. OpenAI controllers choose orchestration; strict SDD remains the fallback

The project adds mirrored project-owned orchestration and routing skills:

```text
.codex/skills/mornlea-implementation-orchestration/SKILL.md
.claude/skills/mornlea-implementation-orchestration/SKILL.md
.codex/skills/adaptive-model-router/SKILL.md
.claude/skills/adaptive-model-router/SKILL.md
```

The skill selects one of two modes:

| Mode | Selection | Execution rule |
|---|---|---|
| OpenAI-native | Controller is verifiably ChatGPT/Codex on an OpenAI model | Standing project authorization lets the controller isolate selected feature/review contexts in subagents without a separate per-task user request; main-agent execution remains the default |
| Strict SDD | Controller is non-OpenAI or provider identity is unknown | Use `subagent-driven-development` with independent implementer/reviewer responsibilities |

OpenAI-native mode does not prescribe a fixed agent topology. The project policy itself is standing authorization to delegate, so silence about subagents in an individual task does not force main-agent-only execution; an explicit user prohibition or a higher-priority runtime restriction still controls. To bound token and coordination cost, at most two subagents may run concurrently. A third independent item is queued, performed by the controller, or started after a slot closes.

Small or low-context work stays with the controller. A free slot, independent file ownership, or possible speedup is not a reason to delegate. Subagents are appropriate only when a bounded feature, research domain, or review surface is large/noisy/specialized enough that keeping its details in the main conversation would materially degrade the controller's architectural reasoning. The controller records the isolation reason, expected integration point, and validation in the change ledger, but that record is not a request for user approval unless the planned action itself requires approval.

When isolation is justified, the controller uses the project-owned `adaptive-model-router` before the first delegation in the task. Live capability discovery comes from the active host; routing applies user constraints and the router's OpenAI ceiling, then evaluates reasoning difficulty, consequence, context breadth, tool horizon, modalities, validation strength, and user cost/speed/quality priorities. Code quality and token efficiency are coequal: the result is the lowest-cost model/effort that is credibly sufficient, not the cheapest option regardless of risk or the highest option regardless of verification strength. Routine translation, extraction, deterministic edits, and narrow validation use a low eligible tier; ordinary coding and bounded synthesis use a mid tier; high tiers are reserved for ambiguous, consequential architecture or correctness work. Except for explicit project ceiling identifiers, the project skill does not copy a model catalog or hard-code currently exposed model names. Existing agents are not restarted solely to downshift them; future independent delegations use the routed choice.

At each implementation-round closeout, routing is reviewed for over-routing, under-routing, retries, escalation, context-transfer cost, validation quality, and token use. Both project router copies change only when observed evidence supports a stable cross-task routing rule; otherwise the ledger records `Model router: no change`. This keeps the router adaptive without turning it into a task log or a volatile model catalog.

Every mode preserves OpenSpec scope, TDD, ledger rulings, validation, ownership, destructive-action safety, and explicit authorization for externally consequential actions.

The project skill owns this decision; cached third-party `subagent-driven-development` files are never patched. Root guidance, OpenSpec apply guidance, development-process docs, and project OpenSpec apply entry points refer to the project policy instead of independently restating divergent variants.

Rejected alternatives:

- Delete SDD entirely: non-OpenAI and unknown-provider execution would lose the requested strict fallback.
- Detect provider by model-name substrings in repository scripts: names change and scripts do not have a trustworthy runtime identity.
- Modify third-party plugin cache: updates would overwrite the project policy and contaminate user-global state.
- Assign every subagent the controller's inherited model: simple isolated work would consume the same expensive model as architecture work and defeat cost-aware routing.
- Keep a project-local static model table: host availability and supported reasoning levels change, so live discovery through `adaptive-model-router` remains authoritative.
- Append every routing outcome to the skill: task history and one-off model anecdotes consume context without establishing a reusable decision rule.

### 5. Architecture learning is a controlled promotion step, not automatic note accumulation

The project adds synchronized skills:

```text
.codex/skills/mornlea-architecture/SKILL.md
.claude/skills/mornlea-architecture/SKILL.md
```

An implementation round means one completed OpenSpec task or one explicitly batched set of small, tightly related tasks that shares validation evidence. At round closeout, the controller performs a short architecture retrospective across ownership, dependency direction, lifecycle, concurrency, platform scope, visual routing, validation, and documentation conventions.

A finding is promoted only when all criteria hold:

1. current code, tests, or canonical specifications verify it;
2. it applies across future tasks rather than only the completed task;
3. it changes a future placement, dependency, lifecycle, validation, or orchestration decision;
4. it is not already stated more authoritatively in root guidance or a canonical document;
5. it can be written as a concise rule with references and without volatile counts.

The architecture skill is a decision aid and routing layer, not a new source of truth. It begins with the current durable conventions relevant across domains: server authority, bridge ownership, bounded hot paths, the stable Godot project/feature layout, desktop-only platform scope, renderer-neutral visual evidence, and documentation language ownership. It links to authoritative sources instead of copying package allowlists, version numbers, scene lists, or thresholds.

If no finding passes promotion, the ledger records `Architecture skill: no change` with a reason. This satisfies the retrospective without forcing meaningless edits. When a convention is promoted, both skill copies change atomically, pass skill validation and synchronization tests, and identify the evidence that justified promotion.

Rejected alternatives:

- Append every task summary to a skill: the skill would become a chronological log and consume increasing context without improving decisions.
- Let the skill override code/specs: it would create another architecture authority and increase drift.
- Update only the active agent's copy: Codex and Claude would apply different architectural conventions.

### 6. Existing comments are grandfathered while new code is English-only

An AST/token-aware scanner extracts comments from first-party Go and Rust source and a conservative lexical scanner covers C, GDScript, JavaScript, and TypeScript. It excludes generated files only through an explicit path/header allowlist. It checks comment text, not strings, identifiers, fixtures, or diagnostic messages.

The initial governance change stores a generated per-file count and digest inventory under `testdata/audit/english-comment-migration.json`. The inventory is a grandfather boundary, not a commitment to translate existing code. While it remains checked in:

- changed comments must contain no Han prose;
- new first-party files must contain no Han comments;
- per-file and global legacy counts may stay unchanged or decrease, but never increase;
- deleting a file removes its debt; renaming must preserve or reduce it;
- updating the baseline upward is forbidden;
- every new architecture file and every new or substantively rewritten comment must use English.

No bulk comment-translation program is required by this change. Existing comments may remain in place even when executable code around them changes, provided their comment text and recorded debt do not grow or change. Optional cleanup may reduce the inventory through the explicit update path, but zero debt and baseline removal require a separately approved future decision.

Rejected alternatives:

- Enable a whole-repository zero-tolerance gate immediately: the repository would become permanently red because existing code is explicitly grandfathered.
- Maintain only a file allowlist: it would permit additional Chinese comments inside grandfathered files.
- Require domain-by-domain translation now: the user explicitly excluded existing-code comment migration, and such a program would create large behavior-neutral diffs unrelated to the Godot architecture.

### 7. OpenSpec and plan language distinguishes new prose from grandfathered debt

`openspec/config.yaml` becomes English and instructs all future artifacts to use English. Active changes, including `pilot-godot-client-migration`, are translated before implementation. Existing Chinese prose under `openspec/specs/` is grandfathered through the per-file count/digest inventory at `testdata/audit/english-openspec-migration.json`. The inventory is not a deadline to translate canonical specs; it rejects new paths, increases, and same-count replacement debt while permitting unchanged prose to remain.

Archived changes and historical `docs/superpowers/` plans remain immutable evidence. New or substantively revised plan files are English. An in-flight user-owned plan is not silently translated by another change; it enters the English rule when its owner next revises or replaces it.

The forward gate therefore has two layers:

1. reject Chinese in active changes and new/substantively revised plan prose immediately;
2. keep unchanged canonical-spec prose behind a non-growth grandfather inventory. Optional translations may decrease it through the explicit update path, but zero-debt cutover requires a separately approved future decision.

### 8. Visual baseline classes remain semantic and producer ownership is explicit

The three existing routes remain:

| Observable subject | Tracked location | Producer ownership |
|---|---|---|
| Window/UI fixture | `testdata/visual-golden/ui/` | Current WebView/Chrome producer until explicit feature handoff |
| Stable headless world frame | `testdata/visual-golden/world/` | Current Rust client capture until explicit scene handoff |
| Cross-tick behavior | `testdata/visual-golden/motion/` | Current registered script/producer until explicit handoff; bounded human review only, no automated pixel comparison |

Godot pilot output goes to `build/visual/godot-pilot/<run-identity>/` and is never a tracked golden. `godot-visual-evidence` captures identity-complete output; `godot-visual-compare` generates comparison data and diff artifacts. Neither command writes `testdata/visual-golden/`.

A later feature-specific handoff must declare the semantic class, scene/fixture id, old and new producer, expected differences, affected files, and rollback. Only after manual inspection may the existing explicit update path write tracked goldens. No `testdata/visual-golden/godot/` directory is permitted, and thresholds cannot be weakened to make a pilot pass.

The mirrored `visual-baseline` skills, `testdata/visual-golden/README.md`/`.zh.md`, visual-verification docs, and the Godot migration change are updated together.

### 9. Current documentation drift is repaired before new gates become authoritative

The first documentation batch corrects all current engine ABI v10/client ABI v14 claims to code-owned v11/v19, removes claims that project Hooks are installed, and updates repository maps for the approved `apps/mornlea-godot/` future root without describing unimplemented runtime behavior as current fact.

Version checks are expanded from only root `AGENTS.md` and `openspec/config.yaml` to every manifest-classified current document that carries a version claim. Historical documents remain exempt only when classified as historical and linked as evidence rather than current instructions.

### 10. Gates remain layered and bounded

Focused edits run syntax/metadata and changed-file language checks. `packages/audit` runs current-document pairing, policy consistency, version claims, visual routing, and repository comment ratchets without launching Godot, browsers, GPU capture, or network services. Full translation and visual capture remain explicit stage-boundary work.

The governance change does not alter visual comparison thresholds or benchmark exit semantics. Real data loss, overflow, I/O failure, missing report identity, invalid documentation pairs, and policy contradictions remain hard failures.

## Risks / Trade-offs

- [Two language files can drift semantically] → Pair identity/revision, compare commands/paths/version claims mechanically, and require human translation review.
- [Removing `README.en.md` can break external deep links] → Update every repository-owned link atomically and record the deliberate path break; do not preserve a third current document that can drift.
- [Provider identity may be ambiguous] → Default to strict SDD when identity is not verifiable; never infer from an arbitrary model-name string in repository code.
- [OpenAI-native flexibility could reduce review quality] → Keep objective tests, gates, ledger evidence, and completion verification mandatory; the controller records its risk-based review decision.
- [Excessive delegation can waste tokens and coordination time] → Default to main-agent execution, delegate only for material context isolation, never for parallel speed alone, and enforce a two-subagent concurrency ceiling.
- [Delegated work still overuses expensive models] → Route every new isolation boundary through `adaptive-model-router`, choose the lowest sufficient live configuration, and escalate only on observed insufficiency.
- [Round-end skill updates can become self-referential policy churn] → Promote only verified cross-task decision rules, reject duplicates and volatile facts, and record a no-change retrospective when nothing qualifies.
- [The grandfather inventory is large] → Leave existing comments untouched, reject new or replaced non-English comment debt, and require all new architecture comments to be English.
- [Language scanners may flag localized examples] → Scan comment tokens only and use narrow, reviewed exclusions for generated/vendor content; localized string literals are not comments.
- [Grandfathered canonical OpenSpec prose can remain Chinese] → Freeze its exact per-file debt, reject new or replaced Chinese prose, and require separately approved semantic review for optional translation.
- [Godot evidence may be mistaken for a new golden] → Use an untracked build path, distinct command names, and an audit rule that forbids renderer-specific tracked baseline directories.
- [Legacy documentation may be stale] → Classify it explicitly, keep it below code/tests/specs in the source-of-truth order, and require factual review plus an atomic bilingual pair on first substantive revision.

## Migration Plan

1. Freeze exact inventories and protect the current dirty worktree; create debt manifests without changing executable behavior.
2. Update English machine-governance sources and add the mirrored provider-aware orchestration and architecture skills, including the two-subagent ceiling and round-end promotion criteria.
3. Change OpenSpec configuration to English and translate active change artifacts, including the Godot pilot.
4. Migrate the root README and governance-critical architecture/process/testing/visual documents to English `*.md` plus Chinese `*.zh.md`, correcting stale ABI and Hook claims; classify unchanged pre-policy explanatory notes as `legacy`.
5. Add documentation-pair, current-version, provider-policy, changed-comment, and visual-routing gates in report-first tests; make them hard only after their initial inventories are checked in and reproducible.
6. Update visual-baseline skills and the Godot plan so pilot captures remain evidence and formal handoff reuses the existing three classes.
7. Retain canonical OpenSpec and source-comment inventories as grandfather boundaries; reject new debt without scheduling bulk translation in this change.
8. Require all new architecture code and new or rewritten comments to pass the English-comment gate; do not schedule legacy comment translation in this change.
9. Run focused audit/skill/script checks, full development gates, full race, Rust checks, and OpenSpec strict validation before archive.

Rollback is additive and policy-scoped: revert the new gates, manifests, skills, paired documents, and current guidance as one governance unit. Specification translations already reviewed as behavior-neutral need not be reversed, but no partial rollback may leave `*.md`/`*.zh.md` ownership or provider policy contradictory. No data or runtime rollback is required.
