---
name: visual-baseline
description: Route Mornlea visual evidence by observable semantics and enforce explicit, reviewed baseline updates across Rust, WebView, and Godot producers. Use before adding, moving, or updating visual baselines.
---

# Visual Baseline Routing

Tracked visual evidence lives under `testdata/visual-golden/`. Read that directory's English canonical `README.md` before changing a baseline; it owns the current registries and routing details. Do not copy scene lists, counts, or comparison thresholds into this skill.

## Semantic routes

- Window or UI component fixtures belong to `ui/`. The current producer is the local Chrome frontend harness and its registry is `fixture-names.ts`.
- Stable headless world frames belong to `world/`. The current producer is the offscreen client capture path and its registry is `captureScenes` in `capture/capture.go`.
- Cross-tick state transitions belong to `motion/` as bounded full-process GIFs that include the pre-trigger, outcome, and settled phases. They are human-review evidence and do not participate in automated pixel comparison.

Route by the observable subject, never by renderer identity. Do not create renderer-specific tracked classes such as `testdata/visual-golden/godot/`. World frames must not contain window chrome, UI fixtures must not recreate world pixels, and one behavior must not have both PNG and GIF evidence unless the visual README records distinct responsibilities.

## Pilot and producer handoff

Godot pilot captures are untracked evidence under `build/visual/godot-pilot/<run-id>/`. Pilot commands may compare against current evidence and produce diffs, but must not write tracked goldens or relax thresholds.

Moving a scene, fixture, or motion producer to Godot requires an approved feature change that names the semantic class, old and new producers, affected files, expected differences, review evidence, and rollback. Only after that handoff is approved may the existing explicit update path write tracked evidence.

## Current entry points

- World comparison: `make visual-check`; explicit update: `make visual-update`. Use `SCENES=` for a focused edit loop when appropriate, but keep the full comparison for stage boundaries.
- UI comparison/update: `make frontend-visual-check` and `make frontend-visual-update`, or the corresponding frozen pnpm commands inside the frontend directory.
- Motion evidence: use the registered `--motion-demo`/`--motion-scene` path or `GIFS=1` where the current capture flow supports it. Motion GIFs are generated for human review, not compared automatically.

## Update discipline

Inspect every expected visual change before an explicit update. Ordinary checks compare only and never accept changes. If comparison fails, inspect actual and diff outputs before deciding whether code or evidence is wrong. Missing baselines must fail rather than being created silently. Comparison behavior and thresholds are owned by the current comparison implementations, not this skill.
