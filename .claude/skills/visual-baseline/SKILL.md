---
name: visual-baseline
description: Design, review, and migrate Mornlea visual evidence across the Rust core and Godot/Python presentation stack. Use for visual regression, capture contracts, baseline updates, or producer handoffs; not for gameplay correctness or image generation.
---

# Visual evidence and baseline migration

Read [`docs/architecture-target.md`](../../../docs/architecture-target.md) for final ownership and the active change for its migration boundary. Read [`testdata/visual-golden/README.md`](../../../testdata/visual-golden/README.md) for current registries and [`openspec/specs/visual-verification/spec.md`](../../../openspec/specs/visual-verification/spec.md) for live contracts. A plan or skill does not make a future producer available. Do not copy scene counts or threshold values into this skill.

## Select the work

- **Regression:** resolve the case's current producer and run its comparison-only path. Inspect actual and diff artifacts on failure; do not update evidence to clear a failure.
- **Intentional visual change:** capture candidates outside tracked evidence, inspect every affected case, then use the explicit update path within the authorized change. Re-run comparison and verify unrelated baselines are byte-identical.
- **Migration or new producer:** read [the handoff guide](references/producer-handoff.md). First establish semantic parity and capture comparability, then qualify the candidate, then transfer individual case ownership. A pilot report is not a handoff.
- **Harness design:** specify identity, readiness, bounded work, failure behavior, and update isolation in OpenSpec before implementing a new capture/comparison path. The active `godot-production-tooling` change owns the planned production evidence contract; do not claim that contract is already enforced.

## Separate the evidence layers

1. **Semantic correctness:** Rust server/domain/protocol/storage tests prove authority and compatibility; Rust client-core replay proves mirrors, prediction, corrections, revisions, and typed presentation values. During migration, Go is an offline oracle. Pixels cannot prove these contracts.
2. **Presentation behavior:** Godot and qualified embedded Python tests prove typed-value mapping, scene lifecycle, input/UI behavior, resource cleanup, and bounded application. Python never decodes packets, reads saves, reproduces numerical kernels, or becomes a second mirror. Use headless tests here when pixels are unnecessary.
3. **Rendered evidence:** capture through the production presentation path with frozen input and environment. Same-producer PNG regression uses the existing comparator; cross-producer parity requires explicit semantic mapping and reviewed differences. Motion remains bounded human-review evidence without automated GIF comparison.

Failure in an earlier layer cannot be repaired by accepting new pixels. Audio and device correctness need event/lifecycle tests; a screenshot or silent GIF is not audio evidence.

## Semantic routes

Route by the observable subject, never by renderer identity.

| Observable subject | Tracked class | Acceptance |
|---|---|---|
| Isolated UI component or window chrome | `ui/` | Stable PNG comparison and interaction tests |
| Settled world state, without UI chrome | `world/` | Stable PNG comparison and semantic readiness |
| A complete transition across ticks | `motion/` | Bounded GIF showing pre-trigger, outcome, and settled phases; human review |

Do not create `testdata/visual-golden/godot/` or another renderer-specific tracked class. UI fixtures do not recreate world pixels. Integrated world-plus-HUD screenshots may support untracked end-to-end review but must not silently become a fourth golden class. Keep one canonical producer per case; a migration may compare multiple candidates without duplicating tracked ownership. A PNG/GIF pair needs distinct documented responsibilities in the visual index.

## Capture and comparison contract

Before interpreting pixels, establish the case/fixture identity, semantic input or replay revision, source and asset provenance, producer/core/bridge/runtime versions, selected catalog, viewport and scale, camera and clock/tick, locale/fonts, graphics backend, and comparison policy. Reuse implemented metadata; record missing fields explicitly instead of inventing values or treating absence as agreement.

Use a fixed seed/input, embedded canonical assets, bounded settling condition, and explicit capture tick. Reject timeout, stale or mixed snapshot revisions, incomplete uploads, missing output, invalid identity, and I/O failure. Do not use an arbitrary sleep as readiness or rebuild a mock renderer solely for capture.

For each required case distinguish **comparable**, **not comparable**, and **failed**. Empty mappings, unsupported features, absent frames, and skipped cases are not passing coverage. A cross-producer difference classification is review evidence; it is not an automatic regression pass. Preserve actual images, diffs when meaningful, metrics, and reasons. Never scale, crop, mask, recolor, or increase tolerance merely to make results agree.

Godot dummy `--headless` rendering cannot supply real GPU pixels. Pixel evidence requires a qualified non-foreground graphics path with no focus stealing; otherwise report it unavailable and continue semantic/lifecycle checks. Never launch or focus a foreground game window without the user's explicit manual-acceptance request. Record the actual display driver and GPU, not a misleading “headless” label.

## Current tools and update safety

- World: `make visual-check`; intentional update: `make visual-update`. `SCENES=` narrows PNG selection but does not bypass the LOD near-ring control. The current update path also generates registered motion GIFs: inspect its full write set before running it. `VISUAL_OUT` does not redirect tracked writes. If that write set exceeds the authorized scope, run the updater in an isolated snapshot of the exact intended source, retain its guards, and publish only reviewed authorized files; compare all other files against their pre-task hashes. Never broadly update the shared checkout and restore unrelated files from `HEAD`.
- UI: `make frontend-visual-check`; intentional update: `make frontend-visual-update`. The current local Chrome harness is not a CI gate.
- Motion: use registered `--motion-demo`/`--motion-scene` outputs, or `GIFS=1` on the current comparison path. Review the entire process and timing, not only the final pose.
- Godot pilot: `make godot-visual-evidence` writes untracked `build/visual/godot-pilot/<run-id>/`. For reproducible comparison, use `scripts/godot/visual-compare.sh --run-dir <verified-run-directory>`; the convenience Make target can select the latest run or capture implicitly. Pilot commands must not write tracked goldens or relax thresholds. The pilot comparator does not itself invoke the identity validator or generate diff images: separately run `python3 scripts/godot/visual_evidence_contract.py --identity <run-directory>/identity.json --run-dir <run-directory>` against the actual identity filename and retain supplementary diffs where meaningful. Read the report and mapping coverage; exit zero alone is not parity.

Moving any case to a new producer requires an approved feature change and an explicit update request, with reviewed candidates, affected files, and rollback. Reuse authorization already given for that concrete scope; do not add repeated permission questions. Ordinary checks never accept changes, and a missing golden fails closed. A separately measured comparator-policy change needs its own contract review; migration is not permission to weaken existing limits.

Report the case coverage, semantic tests, capture environment, comparison or review outcome, unresolved differences, and whether ownership actually transferred. Keep Codex and Claude copies, including references, byte-identical.
