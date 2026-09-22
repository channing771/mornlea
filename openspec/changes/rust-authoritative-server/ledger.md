# rust-authoritative-server ledger

## 2026-09-20 — target architecture planning synchronization

- Baseline: `8d9cc122486097fb7d7788abdb523ecd13f5a75a`.
- User scope: reconcile migration planning with the final architecture and improve the visual-baseline skill. This entry records planning only; no runtime task is complete.
- Ruling: add a concrete foundation change — later Godot feature plans referenced an unproposed Rust prerequisite — prevent proposal existence from being mistaken for implementation acceptance.
- Ownership: controller owns F1–F3 plans, target links and visual skills/docs; a fresh isolated agent audits and rewrites the seven later plans; a separate read-only agent forward-tests the complex visual skill. Isolation keeps multi-file discovery and review traces outside the controller's editing context.
- Directory guidance: planning artifacts and skill resources inherit root guidance; no runtime directory is created. Implementation tasks create scoped guides beside new architectural crates.
- Existing unrelated work: deleted `.codex/skills/pr-submit/SKILL.md` and `.claude/skills/pr-submit/SKILL.md` are user-owned and excluded from this change.
- Validation: pending planning integration; prospective Cargo suites have not run and do not yet exist.
- Architecture skill: review at round close; current code and canonical contracts remain the current-behavior authority.

## 2026-09-20 — integrated planning review and validation

- Integrated with F1–F3, P8–P14, the synchronized visual skill/reference, and bilingual target/visual documentation. This remains a planning-only change; every runtime task is pending.
- Shared review, orchestration decisions, exact validation commands and results are recorded in the [F1 integration ledger](../rust-runtime-foundation/ledger.md). OpenSpec strict validation passed 124 items; focused architecture/documentation/visual checks and skill validation passed.
- The full audit's existing protocol-documentation and English-comment-inventory failures reproduced on untouched baseline `8d9cc122486097fb7d7788abdb523ecd13f5a75a`; no exemptions or unrelated changes were introduced.
- Architecture skill: promoted the verified distinction between Rust semantic correctness and Godot/Python presentation evidence. Implementation-specific future facts stay in these plans and the visual handoff guide.
- No current canonical spec, runtime, tracked image, version or default entry changed; the user-owned `pr-submit` skill deletions remain excluded.

## 2026-09-22 — F1 prerequisite correction after baseline archive

- Baseline: `effd8a247427d2ab5710a8f481349c4cd4676721`, the merge commit of PR #184 on `main`; CI failure is not treated as acceptance evidence.
- Discovery: the archived `rust-runtime-foundation-baseline` explicitly leaves mob/object/chat events, complete protocol and storage evidence, safe public numerical APIs, pathfinding and final F1 acceptance to successor changes. Current code and the frozen corpus confirm those gaps.
- Ruling: F2 remains planning-only and may not start from the archived baseline. The first active successor is `rust-domain-event-completion`; later protocol, storage, numerical-API and pathfinding successors plus a zero-gap F1 acceptance remain mandatory prerequisites.
- Artifact reconciliation: `proposal.md`, `design.md` and task 1.1 now require complete F1 evidence rather than treating the baseline archive or a stale `rust-runtime-foundation` link as completion.
- Orchestration: the controller performed the bounded planning correction directly because the artifacts and prerequisite ruling are tightly coupled; no implementation or delegated worker was started.
- Architecture skill: no change. This is an application of the existing target-architecture and migration-seam rules, not a new stable cross-task convention.
