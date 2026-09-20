# rust-runtime-foundation ledger

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

- Ruling: reconcile the complete F1–F3 → P8–P14 graph, not only target headers. UI is Godot Control/embedded Python over Rust views; Bootstrap retirement needs native diagnostics; default switch needs two release cycles and one recoverable previous release.
- Ruling: P12 phase 2 supplies capture/identity/strict-coverage and per-case canonical regression before feature handoffs. Canonical ownership moves only for reviewed cases; a full legacy producer cannot compare against newly owned baselines. Empty pilot mappings and exit-zero classification are not acceptance.
- Independent review: an isolated planning agent audited and rewrote the seven later changes; a read-only forward test exercised four realistic visual-skill requests. Follow-up foundation review added explicit Go-only kernel migration, Rust-producer session lifecycle evidence and the pinned Rust toolchain in root-level commands. All findings were resolved in planning.
- Skill result: both project-owned visual skills and handoff references are synchronized. Architecture skill: promoted only the target-owner evidence distinction supported by `docs/architecture-target.md` test ownership and canonical visual handoff contracts; volatile pilot details remain in the visual workflow/reference.
- PASS: `openspec validate --all --strict --no-interactive` (124 items at integration; final rerun recorded by command output).
- PASS: `go test ./packages/audit -run 'Test(ProjectArchitectureSkillsMatch|ArchitectureSkillRetrospective|ArchitectureTargetDocumentsFoundationAndRollback|VisualBaselineRouting|VisualProducerOwnership|CompletedDocumentationPairsAreSynchronized|DocumentationManifestClassifiesCurrentMarkdown|CurrentDocumentationLinks)' -count=1`.
- PASS: `go test ./packages/tools/perfcheck -run TestGodotPilotVisual -count=1` (existing tests verify unmapped coverage and classification-only behavior).
- PASS: skill-creator `quick_validate.py` for both visual skills, using an isolated `uv run --no-project --with pyyaml` environment because system Python lacks PyYAML; both skill/reference copies and architecture copies are byte-identical. Local Markdown link targets resolve; `git diff --check` passes.
- Existing baseline failures: `go test ./packages/audit -count=1` fails only `TestCurrentDocumentationVersions` (README/pilot report protocol v44 versus current v45) and `TestEnglishCommentMigration` (existing source comment debt exceeds its inventory). Both failures reproduce on untouched baseline `8d9cc122486097fb7d7788abdb523ecd13f5a75a` in a detached checkout with `go test ./packages/audit -run 'TestCurrentDocumentationVersions|TestEnglishCommentMigration' -count=1`. No exemptions, baseline weakening or unrelated source edits were made.
- This round changes planning, skills and explanatory documentation only. Runtime implementation checkboxes remain open; prospective Rust crates/commands, production visual registry/updater and cutover gates were not executed or claimed implemented. No tracked PNG/GIF, runtime, current canonical spec, protocol/save version or default entry changed. Full runtime Rust/race/GPU gates belong to the implementation tasks and were not run for this documentation-only round.
