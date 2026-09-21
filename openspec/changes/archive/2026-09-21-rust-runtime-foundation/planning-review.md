# Controller planning review — 2026-09-20

This review qualifies the **plan**, not runtime correctness. The user asked for Superpowers-based redesign of unstarted work and a persistent rule that the main Agent completely designs architecture/functionality before assigning implementation to workers. Runtime implementation remains at the reviewed baseline; only original registration2.1 retains acceptance.

## Method and ownership

The main Agent used the installed Superpowers6.4.1 `brainstorming` and `writing-plans` resources, project architecture/update/orchestration guidance and skill-creator guidance. Design, task briefs and status stay in the active OpenSpec change. Installed plugin files were read only; their machine-specific locations are not persisted as policy. The main Agent compared broad delegation, whole-subsystem rewrite and contract-first bounded nodes, then selected the last and authored the interfaces, algorithms, compatibility rulings, tests and dependency graph itself.

Isolated read-only agents supplied source facts about domain/protocol, numerical kernels and storage; they did not choose target architecture or write repository implementation. The storage follow-up was limited to callable codecs, versions, private test seams and format fidelity. No runtime worker was dispatched. The controller integrated and corrected the facts before freezing plans.

## Five focused reviews

| Review focus | Finding and final ruling | Concrete nodes |
| --- | --- | --- |
| Ingress and ordering | ChatCommand has no sequence; ordinary command ties preserve intake arrival within session. TakeCraftingOutput alone rejects sequence0 at its boundary. Replace kind-string sorting with reusable checked ordering scratch. |2.4–2.6,3.3,3.5|
| Domain versus format | Current player armor is raw byte fidelity, not ordinary ItemStack admission. Metadata weather7/255,raw dimension and full phase are preserved by Go codec; runtime restoration owns normalization. Companion active cap is4,not32. |2.3,4.2,4.5–4.7|
| Buffer and numerical ownership | Every public encoder preflights before publication. Mesh needs payload staging, not only count staging; custom accepted registries require a conservative40960-quad stage. Metadata alias checks precede clearing. Rescan native interior guarantees halo; legacy failed outer reads retain status9. |3.1,3.10–3.69,4.6–4.12,5.1,5.10–5.11|
| Exact algorithm compatibility | Self-review caught and corrected an initial else-if path-transition description: Go independently emits flat/jump/fall/gap candidates. Preserve heap first-insertion/equal-g ties and4096 expansion boundary. Worldgen extreme X/Z arithmetic matches deployed release wrapping explicitly in debug too. |5.5–5.6,5.12–5.13|
| Evidence and dependency closure | Source hashes are provenance. Actual Go producers and Rust consumers bind manifest input/output content. Metadata/Agent/private Go paths use existing package-local test seams; no invented production exports or Python service. Foundation acceptance6.2 releases kernels; final replay6.3 waits for them, avoiding a cycle. |1.1–1.6,all family producers,6.1–6.4|

Additional self-review corrections: PlayerInput is23 bytes; outbound login checks raw name bytes while inbound admission trims; zstd DecodeAll-compatible concatenated/skippable streams are not incorrectly forbidden; UUID maximum-batch seeds vary the last16 bits without wrapping0xff; fixed-array decoding uses stable Rust APIs; per-family truncation matrices stay within corpus budgets. Domain migration2.4 includes its existing protocol validation caller so producer/consumer crates stay compilable without a temporary clippy-failing constructor; over-seven-argument packet constructors are replaced within each family task.

## Readiness and coverage

- 117 unique nodes: one retained accepted registration and116 unchecked implementation/acceptance nodes. All116 link an explicit subsystem packet; tasks.md is the sole status source.
- 59 individual packet nodes exactly match the current direction/state registry. Each names its source module, Go/Rust tests, concrete valid seed, family-specific negative/boundary cases and exact commands. Colocated stack packets have serial dependencies.
- Seven save families have explicit supported version sets and every historical fixture source, including private metadata/chunk builder seams. Ten existing numerical families plus pathfinding have typed signatures, capacities, scratch lifetime, failure/ABI adaptation and concrete tests.
- Dependency checks found no cycle or missing node. Whole numerical inventory/replay acceptance is not a prerequisite for starting numerical implementation. Shared exports/test roots/manifest/lock/FFI have a single integration owner.
- 135 relative document/task links were checked, including all task anchors. The two orchestration SKILL.md files and worker-planning references are byte-identical.
- No executable brief leaves an architectural decision for a later controller packet. References to future runtime ownership, F2/F3 or unsupported additional algorithms are explicit exclusions, not unfinished F1 design.

The checklist was exercised on an old brief saying only “remaining encoder families, controller chooses validation later”; it fails exact-interface, case, ownership and readiness checks. Its replacement is each3.10–3.68 packet with a complete seed, validators, family limits, buffer procedure, named Go operation, named Rust filter, predecessor list and rollback. Length of prose was not used as acceptance.

## Planning validation

Passed on the current worktree:

1. `git diff --check`.
2. `openspec validate --all --strict --no-interactive`:125 passed,0 failed.
3. Focused provider/orchestration/directory-guidance audits, including both mirrored project skills.
4. Documentation manifest, bilingual metadata/pair, legacy-preservation, links and semantic-pair drift audits.
5. Official skill-creator `quick_validate.py` for both project orchestration skills, run with the existing Agent virtualenv Python providing PyYAML; both report valid. System Python initially lacked PyYAML; no dependency was installed to bypass that environment issue.
6. Read-only plan coverage/link/dependency checks after correcting the two documentation-manifest revision entries to2026-09-20.1.

The first bilingual audit correctly rejected changed document revisions without matching manifest revisions; both entries were synchronized and the same audit passed. This is fixed, not a remaining blocker. Existing code-review failures and broad runtime audit baseline issues are still recorded in review.md/ledger.md; this planning-only round does not repair or waive them. No cargo/runtime/race/build gate is claimed from planning validation; actual final gates remain6.6.

## Preservation and architecture promotion

Preexisting skill deletions and unrelated changes remain intact. The foundation task/ledger files already contain user/prior-round modifications; the ledger receives an append-only record. No broad staging, commit, source fixture regeneration, live-save access, plugin installation or runtime implementation occurred in this round. Planning documents under this change inherit the project/OpenSpec guide; the plans directory has no independent runtime boundary requiring another AGENTS.md.

Architecture skill: no change. The durable new planning rule belongs in root AGENTS.md, OpenSpec rules and the synchronized orchestration skill. Numerical and compatibility details here are change-scoped design decisions until implementation/tests verify them; they are not yet eligible for promotion as verified cross-task architecture.
