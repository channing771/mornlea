---
doc_id: visual-verification-guide
doc_revision: 2026-09-15.1
language: en
counterpart: visual-verification.zh.md
---

# Visual verification

Mornlea uses semantic visual evidence to make rendering errors reviewable. The authoritative behavior contract is `openspec/specs/visual-verification/spec.md`; the complete current registry is [the visual evidence index](../../testdata/visual-golden/README.md).

## Evidence classes and current producers

- `world/` contains 31 stable headless world-frame PNGs. `captureScenes` in `packages/client/cmd/mornlea/capture/capture.go` owns the current scene registry and order.
- `ui/` contains 31 window/UI component PNGs. `fixtureNames` in `packages/engine/crates/mornlea_client/frontend/visual/fixture-names.ts` owns the current fixture registry.
- `motion/` contains 11 bounded cross-tick GIFs. They show the complete pre-trigger, outcome, and settled process for human review and do not participate in automated pixel comparison.

Route evidence by observable semantics rather than renderer identity. World frames do not include window or WebView chrome, UI fixtures do not recreate world pixels, and motion evidence is not converted into isolated still-frame substitutes. The current Rust/WebView producers remain authoritative until an approved feature change transfers a specific producer.

## World capture and comparison

`--capture <directory>` runs the client through its headless offscreen path without creating or focusing a foreground game window. Each world scene uses deterministic state, a fixed camera, the embedded default material set, and an explicit settled point. Capture and benchmark/remote-connect modes are mutually exclusive.

World comparison uses the two metrics implemented in `packages/client/cmd/mornlea/capture/visual_compare.go`: maximum per-channel pixel difference and the ratio of differing pixels. Both must remain within the code-owned limits. This document does not duplicate threshold values. A missing baseline fails unless the caller explicitly requested an update.

```bash
make visual-check
VISUAL_OUT=/tmp/shots make visual-check
make visual-update
```

On failure, the output directory receives `<scene>-actual.png` and `<scene>-diff.png`; the diff image highlights changed pixels so reviewers can locate the problem.

## Focused runs and stage boundaries

`--capture-scenes` (Make variable `SCENES=`) selects a non-empty subset while preserving the authoritative full-list order. Unknown, duplicate, or empty names fail before capture. Focused checks accelerate the edit loop but do not replace the full `make visual-check` stage-boundary result.

```bash
make visual-check SCENES=mining-crack-early,mining-crack-heavy
make visual-update SCENES=mining-crack-early,mining-crack-heavy
```

Subset updates still run the existing LOD on/off near-ring control before writing any world baseline. `GIFS=1` requests motion generation during a comparison run; explicit visual updates generate the registered motion GIFs. GIFs remain human-review evidence and are not compared automatically.

## Godot pilot evidence

The Godot pilot writes identity-complete captures to `build/visual/godot-pilot/<run-id>/`. `godot-visual-evidence` produces captures, and `godot-visual-compare` produces comparison/diff reports. Neither command may write `testdata/visual-golden/`, create a renderer-specific tracked class, or relax thresholds.

A future producer handoff must name the existing UI/world/motion identity, old and new producers, expected differences, affected files, review evidence, and rollback. Tracked evidence changes only after explicit approval and human inspection through the existing update discipline.

## Historical re-attribution note

The natural-short-grass rollout originally re-attributed a 24-scene world baseline while the engine ABI was v10. Nine scenes changed only because deterministic short grass became visible, fifteen remained byte-identical, and two clean full comparisons reproduced the accepted result. This is historical provenance, not the current scene count or ABI identity; current values come from code and the visual index.

## Update discipline

Run an update only for an intentional visual change after opening and approving every affected candidate. Never overwrite baselines merely to clear a red comparison. Inspect actual and diff artifacts first, then decide whether implementation or evidence is wrong. Unaffected evidence must remain byte-identical, and thresholds change only through a separately measured and approved contract change.

Visual capture is a local GPU/manual-review workflow. It is not part of ordinary Go tests or CI.
