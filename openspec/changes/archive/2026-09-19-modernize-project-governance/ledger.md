# Implementation Ledger

## Baseline

- Change: `modernize-project-governance`
- Schema: `spec-driven`
- Baseline commit: `84f3e0e75dff6987e47cf2aa50f50d888e85eafb`
- Inventory date: 2026-09-15
- Controller: OpenAI Codex
- Execution shape: OpenAI-native adaptive orchestration. The user clarified that the project policy itself is standing authorization for ChatGPT/Codex to choose main-agent work, subagents, or a mixed approach without a separate per-task request. Initial inventory work was completed directly; subsequent independent work may be delegated. Required OpenSpec scope, tests, validation, ledger evidence, and completion gates remain unchanged.

## Initial inventory

The source-comment inventory uses this transitional lexical command until the token-aware scanner replaces it:

```bash
rg -n -P '^\s*(//+|/\*|\*)[^\n]*\p{Han}' packages scripts \
  --glob '*.go' --glob '*.rs' --glob '*.c' --glob '*.h' --glob '*.gd' \
  --glob '*.js' --glob '*.mjs' --glob '*.ts' --glob '*.tsx'
```

Observed result:

- 1,240 first-party source files with at least one matching comment line.
- 38,333 matching comment lines.
- File counts by extension: Go 1,126; Rust 47; C 1; headers 2; JavaScript 3; MJS 4; TypeScript 14; TSX 43; GDScript 0.

Task 5.2 replaced this preliminary lexical observation with the authoritative token-aware baseline in `testdata/audit/english-comment-migration.json`: 1,243 files and 38,636 Han-containing comment lines. The ratchet derives later totals from current source rather than treating these numbers as permanent constants.

Task 4.3 replaced the coarse canonical-spec file count with `testdata/audit/english-openspec-migration.json`: 112 canonical specification files and 10,656 Han-containing prose lines. Code fences, inline code/identifiers, and explicit `localized("...")` literals are excluded; active changes are zero-debt and archived changes remain outside the scan.

Planning and documentation inventory commands:

```bash
rg -l -P '\p{Han}' openspec/specs --glob '*.md'
find openspec/changes -path 'openspec/changes/archive' -prune -o -type f -name '*.md' -print
rg -l -P '\p{Han}' openspec/changes/archive --glob '*.md'
rg -l -P '\p{Han}' docs/superpowers/plans docs/superpowers/specs --glob '*.md'
find docs -type f -name '*.md' -not -path 'docs/superpowers/*'
```

Observed result:

- 112 canonical OpenSpec specs contain Han characters.
- Four active artifacts contain Han characters, all under `pilot-godot-client-migration`.
- 917 archived OpenSpec artifacts contain Han characters and remain historical evidence.
- 141 historical `docs/superpowers/plans` or `docs/superpowers/specs` files contain Han characters.
- 36 non-superpowers Markdown files currently require classification under the new documentation convention.

Current stale-claim findings:

- `README.md`, `README.en.md`, and `docs/architecture.md` contain current-looking engine ABI v10/client ABI v14 claims; code-owned current values are v11/v19.
- `docs/openspec.md` states that project Hooks are installed and describes `PreToolUse`, `PostToolUse`, and `Stop` configuration; root and script guidance state that those Hook configurations were removed.
- Historical progress, backlog, and plan records also contain older versions by design; they are not current claims and require explicit classification rather than bulk replacement.

## Pre-existing dirty paths

These paths predate governance implementation and are not owned by this change:

```text
.superpowers/sdd/tasks/progress.md
openspec/specs/voxel-visual-presentation/spec.md
packages/client/render/block_outline.go
packages/client/render/block_outline_test.go
packages/client/render/name_tag.go
packages/client/render/name_tag_test.go
testdata/visual-golden/world/target-block-feedback.png
docs/superpowers/plans/2026-09-08-b02-bucket.md
docs/superpowers/plans/2026-09-11-sneak-double-tap-sprint.md
docs/superpowers/specs/2026-09-11-sneak-double-tap-sprint-design.md
```

## Ownership matrix

| Owner | Paths | Allowed work in this change |
|---|---|---|
| `modernize-project-governance` | Governance OpenSpec artifacts; root/scoped guidance; documentation manifest and pairs; project skills; audit/language/visual-routing gates | Governance implementation only |
| `pilot-godot-client-migration` | `openspec/changes/pilot-godot-client-migration/**`; future `apps/mornlea-godot/**`; Godot scripts/build entries | Planning reconciliation now; runtime implementation only after governance prerequisites |
| Existing user work | Every pre-existing dirty path listed above | Read-only; no translation, staging, cleanup, golden update, or semantic edit |
| Historical evidence | `openspec/changes/archive/**`, existing historical `docs/superpowers/**`, classified historical notes | Inventory/linking only; no bulk translation |

## Rulings

- Ruling: use OpenAI-native adaptive orchestration without per-task delegation confirmation — the user clarified that the project policy is standing authorization for ChatGPT/Codex to choose direct, delegated, or mixed execution — preserve every objective gate and ledger requirement, and obey any explicit user prohibition or higher-priority runtime restriction.
- Ruling: make main-agent execution the default and use subagents for context isolation rather than parallel speed — the user identified token and context cost as the controlling trade-off — never run more than two subagents concurrently and do not fill available slots without a material isolation need.
- Ruling: route each new context-isolation subagent through `adaptive-model-router` — the user wants simple delegated work to use an appropriately lower model/effort instead of inheriting the highest tier — use live host discovery, the router's OpenAI ceiling, lowest-sufficient selection, and evidence-based one-step escalation.
- Ruling: own and continuously improve the router inside the project — model choice affects both correctness and token cost across every future implementation — keep synchronized Codex/Claude copies, evaluate complexity from multiple independent axes, treat code quality and token efficiency as coequal, and update the skill only for verified reusable improvements.

## Delegation routing records

| Isolated context | Model | Effort | Basis | Fallback |
|---|---|---|---|---|
| Development-process and OpenSpec bilingual migration | `gpt-5.6-luna` | `low` | Deterministic translation with explicit source files, policy text, paths, and validation; isolation prevents a large translation corpus from polluting the main architecture context | `gpt-5.6-terra` at `medium` only if factual or link validation exposes unresolved contradictions |
| Architecture/documentation-map repair | `gpt-5.6-terra` | `medium` | The lower-tier pass left the English canonical files largely Chinese and retained a stale client ABI v14 paragraph; this is observable insufficiency in a correctness-sensitive architecture document | Main-agent source-verified repair if mixed language or stale identity remains |
| Godot active-artifact translation | `gpt-5.6-terra` | `medium` | Four large, cross-referenced planning artifacts formed a material isolated context; the current host exposed this eligible mid-tier option, and the earlier low-tier translation had already shown factual omissions | Main agent completes and validates any untranslated remainder instead of starting another agent |

The first translation pass preserved structure but compressed several valid process invariants. Targeted follow-up in the same isolated context restored claim/approval/confirmation rules, roles, TDD/ledger/evidence reuse, stage gates, PR/CI/archive behavior, parallel conflict rules, and detailed OpenSpec flow. No new agent was started.

The first architecture/documentation-map pass failed language and current-version inspection even though its narrow tests passed. A one-step routed escalation was started only for that same isolated context; task 3.4 remained open until the English files were fully English and every current ABI claim was verified.

The routed repair completed the architecture/documentation-map pair. Main-agent inspection found no Han prose in the English canonical files, no stale engine ABI v10 or client ABI v14 current claims, and no broken completed-pair links. The approved `apps/mornlea-godot/` root remains explicitly future, desktop-only architecture rather than implemented-runtime documentation.

The isolated Godot translation completed `proposal.md` but stopped after an external translation-service rate limit, leaving the other three artifacts untouched. The main agent then translated `spec.md`, `design.md`, and `tasks.md` directly, preserving 13 requirements, 31 scenarios, 87 task checkboxes, desktop-only scope, stable feature boundaries, and renderer-neutral evidence routing. No replacement agent was started.
- Ruling: keep Godot platform support desktop-only — the approved target set is macOS, Windows, and Linux desktop, with macOS as the pilot target — reject Android, iOS, Web, and console adapters, templates, and export presets.
- Ruling: treat visual baseline classes as renderer-neutral — Godot pilot captures are untracked evidence — do not modify the user's existing `target-block-feedback.png` change or any tracked golden.
- Ruling: preserve archived OpenSpec and historical plans in their original language — they are evidence rather than current normative guidance — enforce English on active/canonical/new or substantively revised planning artifacts.

## Validation evidence

| Task | Commit/worktree identity | Commands | Result |
|---|---|---|---|
| 1.1 | baseline plus uncommitted governance planning | `git status --short`; inventory commands above; `openspec validate modernize-project-governance --type change --strict --no-interactive` | Dirty ownership recorded; inventories recorded; change valid |
| 1.3 | same | `git status --short`; compare against ownership matrix | All pre-existing dirty paths assigned to existing user work and excluded from governance edits |
| 1.2 | same | `go test ./packages/audit -run 'DocumentationManifest|DocumentationClassification' -count=1`; `jq empty docs/documentation-manifest.json` | 41 current governed Markdown paths classified across bilingual, machine, plan, historical, and generated entries; license class defined with no current governed path |
| 7.1–7.3 | same | `openspec validate pilot-godot-client-migration --type change --strict --no-interactive`; targeted `rg` checks for feature layout, desktop-only scope, one-way catalog/ABI dependency, export exclusions, pilot evidence commands, and retained `make visual-check` | Godot plan valid; coarse features and desktop-only platform scope recorded; pilot evidence cannot write tracked goldens |
| 7.4 | same | Han scan across all four active artifacts; requirement/scenario/task counts; `openspec validate pilot-godot-client-migration --type change --strict --no-interactive` | All four artifacts are English; 13 requirements, 31 scenarios, and 87 task checkboxes remain; tasks require English canonical explanatory docs plus synchronized `.zh.md` counterparts |
| 4.1–4.3 | same | expected-red missing-baseline run; explicit `MORNLEA_UPDATE_OPENSPEC_LANGUAGE_BASELINE=1` initialization; `go test ./packages/audit -run 'OpenSpecLanguage|PlanLanguage|HistoricalPlanningExemption' -count=1`; `openspec validate --all --strict --no-interactive` | Active OpenSpec prose contains no Han text; changed current plans are gated; the canonical-spec debt ratchet records 112 files/10,656 prose lines and rejects new, increased, or same-count replaced debt while excluding archived changes and historical plans |
| 3.6 | same | `go test ./packages/audit -run 'CodeCommentLanguagePolicy|DocumentationPair|DocumentationManifest|DocumentationSemanticClaims|CurrentDocumentationVersions|DocumentationLinks' -count=1` | Test-organization and AGENTS-writing standards migrated to English canonical plus Chinese counterparts; helper/test placement, naming, no-task-ID, and English source-comment rules remain intact, with commands and repository-path claims synchronized |
| 8.2 | same | `go test ./packages/audit -count=1`; `node --test scripts/agent-hooks/guard.test.mjs`; `openspec validate --all --strict --no-interactive`; project-skill validation and byte comparisons; `git diff --check` | Full audit passed, all 20 Hook tests passed, all 114 OpenSpec items passed strict validation, project skills validated, mirrored skills remained identical, and the diff had no whitespace errors |
| 6.1 | same | `packages/agent/.venv/bin/python .../quick_validate.py` for both project skill copies; `cmp -s`; Han-text scan | Both visual-baseline skills valid, byte-identical, and English; routing matches current truth that `motion/` GIFs are bounded human-review evidence without automated pixel comparison |
| 2.4–2.6 | same | `quick_validate.py` for both orchestration skills; `cmp -s`; `go test ./packages/audit -run 'ProjectOrchestrationSkillsMatch|OpenSpecApplySkillsRouteThroughProjectOrchestration|ProjectOrchestrationRejectsModelNameHeuristics' -count=1`; `openspec instructions apply --change modernize-project-governance --json`; `git diff --check` | Mirrored skills valid and identical; standing OpenAI authorization and strict fallback recorded; both apply entry points route through the project skill without changing OpenSpec state handling |
| 2.1–2.3 | same | `go test ./packages/audit -run 'ProviderAwareOrchestration|BaselineVersionsMatchCode|AgentGuidance|CodeCommentLanguagePolicy' -count=1`; proposal/apply instruction JSON checks; `openspec validate modernize-project-governance --type change --strict --no-interactive`; `git diff --check` | Root/scoped guidance and OpenSpec configuration use English; standing OpenAI authorization, strict fallback, English comments, current version values, and removed-Hook state are coherent |
| 2.7 | same | `go test ./packages/audit -run 'ProviderAwareOrchestration|DelegationBudget|AdaptiveModelRouting' -count=1`; both orchestration-skill validators; byte comparison | Main-agent default, context-isolation-only delegation, two-subagent ceiling, and live lowest-sufficient `adaptive-model-router` selection enforced in root guidance and both skill copies |
| 2.8 | same | both architecture-skill validators; byte comparison; `go test ./packages/audit -run 'ProjectArchitectureSkillsMatch|ArchitectureSkillRetrospective' -count=1` | Project architecture skills are valid, byte-identical decision aids with verified durable conventions and bounded promotion criteria |
| 2.9 / 3.5 | same | documentation pair/manifest/governance/orchestration focused audit; `node --test scripts/agent-hooks/guard.test.mjs`; `jq empty`; `git diff --check` | Development-process and OpenSpec docs migrated to English canonical plus Chinese counterparts; removed-Hook truth, context-isolation delegation, two-subagent ceiling, adaptive routing, and round-end architecture retrospective are consistent |
| 2.10–2.11 | same | `quick_validate.py` for both project router packages and both orchestration skills; recursive router comparison; orchestration-skill byte comparison; `go test ./packages/audit -run 'ProjectAdaptiveModelRouter|DelegationBudgetAndAdaptiveModelRouting|ProjectOrchestrationSkillsMatch|ProviderAwareOrchestration|DocumentationPair|DocumentationSemanticClaims' -count=1`; full audit; strict OpenSpec validation; `openspec instructions apply --change modernize-project-governance --json` plus `jq` assertion | Project-owned router packages are valid and synchronized; root guidance, emitted OpenSpec apply guidance, orchestration skills, and the bilingual development process make quality/token efficiency coequal, require multi-axis lowest-sufficient routing, and enforce round-end router improvement or an explicit no-change record. Quoting the guidance entry prevents YAML from treating `Model router: no change` as a mapping and proves the rule is actually injected. |
| 3.3 | same | root README pair/manifest/current-identity focused audit; stale ABI and obsolete-link scans; `jq empty`; `git diff --check` | English canonical `README.md` and Chinese `README.zh.md` established; `README.en.md` removed; internal navigation repaired; all current README ABI claims use v11/v19 |
| 3.4 | same | Han and stale-ABI scans; `go test ./packages/audit -run 'DocumentationPair|DocumentationManifest|DocumentationSemanticClaims|CurrentDocumentationVersions|DocumentationLinks|Architecture' -count=1` | Documentation map and architecture migrated to synchronized English canonical/Chinese counterpart pairs; current architecture remains code-grounded and the desktop-only Godot root is labeled as an unimplemented future target |
| 3.1–3.2 | same | `gofmt`; `go test ./packages/audit -run 'DocumentationPair|DocumentationManifest|DocumentationSemanticClaims|CurrentDocumentationVersions|DocumentationLinks' -count=1` | Manifest/pair gates now reject duplicate, unclassified, missing, mismatched, wrongly suffixed, and `*.en.md` entries; completed pairs compare normalized commands, repository paths, local links, and code-owned version claims. The strengthened gate exposed and repaired three real bilingual omissions rather than weakening comparison. |
| 5.1 | same | `go test ./packages/audit -run 'CodeCommentLanguage|CommentScanner' -count=1 -race`; `git diff --check` | Go parser plus conservative Rust/C/GDScript/JavaScript/TypeScript scanners detect Han prose in comments while accepting localized string literals and excluding only declared generated/vendor/license trees |
| 6.2 | same | pair/manifest audit; `jq empty`; actual `world/*.png`, `ui/*.png`, and `motion/*.gif` counts; registry count checks; `git diff --check` | Visual index migrated to English `README.md` plus Chinese `README.zh.md`; 31 world, 31 UI, and 11 motion entries preserved; motion remains human-review-only; no PNG/GIF changed by governance work |
| 5.2 | same | expected-red missing-baseline run; explicit `MORNLEA_UPDATE_ENGLISH_COMMENT_BASELINE=1` initialization; `go test ./packages/audit -run 'EnglishCommentMigration|CodeCommentLanguage|CommentScanner' -count=1`; JSON inspection; `git diff --check` | Token-aware baseline created for 1,243 files/38,636 comment lines; per-file count/digest ratchet rejects new, increased, or same-count replaced debt and permits only explicit decreasing updates |
| 6.4 | same | `go test ./packages/audit -run 'VisualBaselineRouting|VisualProducerOwnership' -count=1`; `git diff --check` | Renderer-specific tracked classes rejected; pilot evidence constrained to build output; tracked producer updates require an approved handoff |
| 5.3 | same | expected-red `TestEnglishCommentGateIntegration`; `bash -n scripts/agents/gates.sh`; `make comment-language-check`; `GATES_SKIP_RACE=1 scripts/agents/gates.sh`; `git diff --check` | Makefile, local gate runner, and CI invoke the English-comment ratchet. Initial local gate exposed unrelated `.worktrees` formatting noise; repository formatting checks now exclude isolated worktrees and the rerun passed every non-race gate, including Rust build and 114-item OpenSpec validation |
| 6.3 | same | documentation pair/manifest and visual-routing audits; `go test ./packages/client/cmd/mornlea/capture -race -count=1`; `jq empty`; `git diff --check` | Visual guide migrated to English canonical plus Chinese counterpart; current 31/31/11 routing, code-owned thresholds, Godot pilot evidence, producer handoff, and historical 24-scene/v10 provenance are clearly separated |
| 5.4 | same | governance artifact reconciliation; Godot task-language checks; `make comment-language-check`; strict OpenSpec validation | Per the user's revised scope, existing non-English comments are grandfathered and require no migration. Five briefly scaffolded comment-translation follow-up changes were removed before implementation. The checked-in 1,243-file/38,636-comment inventory now serves only as a non-growth boundary; all new Godot architecture code and all new or substantively rewritten comments must use English. |
| 8.1 | same | `make fmt`; `cd packages/engine && rustup run 1.97.1 cargo fmt --check`; `quick_validate.py` for both orchestration and both architecture skill copies | Go/Rust formatting completed; all four project skill copies are valid. Pre-check confirmed no Go file required formatting before the repository-wide formatter, so the command did not absorb unformatted user work. |
| 8.3 | same | `make rust-check`; `go test ./packages/audit -count=1`; `make dev-check`; `go test ./packages/server/server -run '^TestWarpParityMemoryVsTCP$' -count=1`; repeat with `-count=5`; focused `-race -count=1` | Rust check and audit passed. `make dev-check` failed in the server short-test phase because `TestWarpParityMemoryVsTCP` rejects a passive spawn in dimension 1 and closes the TCP transport. The non-race focused test reproduces, including a five-run sample; the focused race run passes. Per user ruling, no server code was changed and the failure is recorded as an external pre-existing server finding. |
| 8.4 | same | `make test-race`; `make frontend-check`; `make visual-check`; `git diff --stat -- testdata/visual-golden`; binary-only diff inspection | The full six-module race gate and frontend gate passed (232 frontend tests plus typecheck/build/dist equality). The non-update visual check failed on six scenes: `grass-closeup`, `oak-grove`, `mining-crack-early`, `mining-crack-heavy`, `rain-noon`, and `snow-cover`. Per user ruling, the differences remain external to governance; no PNG/GIF or threshold was updated. The only modified tracked image is the pre-existing user-owned `world/target-block-feedback.png`. |
| 8.5 | same | `openspec validate --all --strict --no-interactive`; documentation manifest and grandfather mutation audits | All 114 current specs and changes passed strict validation. The 112-file/10,656-line canonical-spec inventory and 1,243-source-file/38,636-comment inventory are permanent non-growth boundaries, not migration commitments. Seventeen unchanged pre-policy explanatory documents are explicitly classified as `legacy`; no incomplete bilingual classification remains. |

## External validation findings

- The non-update `make visual-check` generated no baseline writes but failed on six scenes: `grass-closeup` (maximum channel difference 127; 1,287/230,400 pixels), `oak-grove` (227; 87/230,400), `mining-crack-early` (48; 620/230,400), `mining-crack-heavy` (48; 576/230,400), `rain-noon` (224; 87/230,400), and `snow-cover` (224; 87/230,400). The worktree already contained user-owned render and `target-block-feedback.png` changes before governance implementation. Per the user's scope ruling, governance neither accepts, updates, reverts, nor attributes these pixels; the finding belongs to the owning visual work.
- The ordinary short-test path reproducibly fails `TestWarpParityMemoryVsTCP` after a dimension-1 passive spawn reaches the TCP login path. The race build passes the same focused test and the entire six-module race gate. Per the user's scope ruling, this is recorded as a pre-existing timing-sensitive server finding and does not authorize changes outside governance.

## Scope reconciliation: legacy comments

- User decision: existing code comments do not need migration. Every new architecture file and every new or substantively rewritten source comment must use English.
- Implementation: retain the token-aware inventory as a grandfather/non-growth boundary; reject new paths, count increases, and same-count non-English replacements; allow unchanged legacy comment text to remain when surrounding executable code changes.
- Removed work: the five `translate-comments-*` follow-up changes created earlier in this round were deleted before implementation because they no longer represent approved scope.

## Scope reconciliation: legacy documentation and canonical specs

- User decision: unchanged pre-policy explanatory documents and canonical specifications need not be bulk-translated in this governance change.
- Implementation: classify 17 unchanged explanatory documents as `legacy`, require their first substantive revision to create synchronized English `*.md` and Chinese `*.zh.md` files, and retain 112 canonical specification files/10,656 prose lines behind the non-growth language inventory.
- Gate result: legacy mutation tests reject substantive edits without pair migration; canonical-spec tests accept unchanged debt but reject new, increased, or replaced non-English prose.

## Architecture retrospective

Architecture skill: updated — the user established a durable cross-task distinction between grandfathered comments in existing code and English-only comments in new architecture code or substantively rewritten comments. Both project skill copies record that placement/review rule without copying volatile debt counts. The router addition changes orchestration policy rather than runtime ownership or dependency architecture, so it required no further architecture-skill expansion.

## Model-router retrospective

Model router: updated — prior routing evidence showed both under-routing (a low-tier architecture/documentation pass omitted required invariants and stale-version repair) and appropriate downshifting (deterministic translation and bounded validation). The project skill now evaluates multiple independent axes, treats code quality and token efficiency as coequal, uses live lowest-sufficient selection, escalates only on observable insufficiency, and requires every future implementation round to repeat this retrospective. Both project copies validate and remain byte-identical.

## Post-completion workflow refinement

- User feedback on 2026-09-16 established two durable requirements: new architecture/boundary units need concise English comments at non-obvious ownership and lifecycle decisions, and independently verified small feature nodes need scoped Git checkpoints before more implementation accumulates.
- Root guidance, both architecture-skill copies, and both orchestration-skill copies now encode those requirements. The rule rejects zero-comment architecture units and broad dirty-worktree commits while preserving the existing rule against low-value syntax narration and unrelated staging.
- Validation: all four updated skills passed `quick_validate.py`; both mirrored pairs are byte-identical; `make comment-language-check`; `go test ./packages/audit -run 'ProjectArchitectureSkillsMatch|ArchitectureSkillRetrospective|ProjectOrchestrationSkillsMatch|GodotFeatureHost|CodeCommentLanguage' -count=1`; and both feature-contract modes passed after adding English boundary comments to the Godot host path.

## Isolation-first routing and Z Code bridge refinement

- User feedback on 2026-09-16 superseded the earlier main-agent-first preference: independently briefable discovery, multi-file reasoning, specialized review, and long work traces now default to fresh isolated workers when the saved main-context retention exceeds handoff cost. Tiny or tightly coupled work remains direct, and native plus external workers share the existing two-agent ceiling.
- Local capability evidence verified Z Code 3.11.2, its embedded CLI 0.16.5, and the enabled `builtin:bigmodel-coding-plan/GLM-5.3` route. A minimal headless turn and session resume both completed before the bridge was finalized. The repository bridge's inference-free `probe` now reports that exact route with reasoning default `max` and no credential disclosure.
- Ruling: expose Z Code through the selected `adaptive-model-router` skill's `scripts/zcode-agent.mjs` as an external isolated agent with `probe`, fresh `run`, and session-ID `send` operations — the current Codex native model enum is closed and cannot truthfully register `GLM-5.3` through repository policy — keep credentials in the child environment, redact provider failures, and require an isolated worktree or exclusive files for editing modes.
- Ruling: send every worker a concise task brief and fresh or minimal context — full-history forks defeat the user's context-cleanliness objective — preserve only required evidence, paths, constraints, ownership, integration points, and acceptance criteria.
- Validation: the four fixture-backed bridge tests pass; the real read-only bridge probe passes; both router copies and both orchestration copies are byte-identical and pass `quick_validate.py`; the focused orchestration, routing, provider, and documentation audits pass; `openspec validate --all --strict --no-interactive` passes all 114 items; and the scoped diff check passes. The full audit remains blocked by the unrelated missing pre-existing `apps/mornlea-godot/features/world/feature_root.py` fixture required by `TestGodotPythonFeatureSkeletonsAndScriptOwnership`.
- Architecture skill: no change — this refinement changes development orchestration and an agent invocation bridge, not stable game ownership, dependency, lifecycle, platform, or runtime architecture.
- Model router: updated — backend selection now includes the verified external Z Code worker, accounts for startup cost and saved controller context, distinguishes external sessions from the native delegation surface, and supports targeted follow-up by session ID without replaying the worker context.

## Directory-scoped guidance update

- User ruling: important repository directories should carry local agent guidance so later agents can identify ownership and behavior without rediscovering the layout; the repository convention is uppercase `AGENTS.md`, not lowercase `agent.md`.
- Architecture ruling: define an important directory as the root, a top-level module/package, or a subtree root with an independent ownership, dependency, lifecycle, or validation boundary or a multi-package/entry-point/asset coordination role. Require a concise purpose, directory map, boundaries, entry points, lifecycle constraints, and focused validation. Directories without independent invariants inherit their parent and do not receive symmetry-only files.
- Implementation: synchronized both architecture and orchestration skills, root/config guidance, the bilingual AGENTS style guide, and a focused audit assertion. No blanket directory-guide generation was performed.
- Configuration repair: quote the existing colon-bearing OpenSpec apply-guidance string so YAML preserves it as a string and the seven apply guidance entries are injected; the policy text is unchanged.
- Validation: `go test ./packages/audit -run 'DirectoryScopedGuidance|ProjectArchitectureSkillsMatch|ProjectOrchestrationSkillsMatch' -count=1`, byte comparisons for both mirrored skill pairs, project skill validation, and `git diff --check`.
