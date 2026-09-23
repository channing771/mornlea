# Node 6.3.5: Diagnose Linux native physics-step rejection

## Identity and evidence

- Baseline: PR #185 Required CI run 35816085845. `race-server` job 107038383007 and `integration-server` job 107038383021 panic in passive movement with `nativeabi: physics step 输入非法` after verifying the Linux native artifact.
- Deliverable: an exact Linux candidate-run record distinguishing malformed step bytes from Rust displacement-bound rejection, including a reproducible failing test name and measured boundary values when applicable. This node diagnoses; it does not approve a physics-policy change.
- Required sub-skill: `superpowers:systematic-debugging`; use `superpowers:verification-before-completion` for the diagnostic evidence. Do not start the repair under a math hypothesis alone.

## Contract and narrow probe

`physics_step_with` in `packages/engine/crates/mornlea_engine/src/ffi.rs` maps both failed `physics_step_input_is_valid` and `physics_step` error to `MORNLEA_STATUS_INPUT`. The old logs lack the input bytes and cannot distinguish them. Rust `vec3_len` currently uses FMA to match Go arm64, while Go amd64 may differ, but this is only a hypothesis. Both observed packet lengths match valid 8/16-cell layouts; this does not prove the remaining input predicates.

In a temporary diagnostic commit, instrument only the two `MORNLEA_STATUS_INPUT` paths with bounded stderr markers whose write errors are ignored so ABI status remains unchanged. Emit `reject=decode` plus input length for failed validation; for a displacement rejection emit `reject=displacement`, failing axis, and minimum/displacement/maximum as `f32::to_bits()` hexadecimal. Do not log full world/player buffers, change statuses, widen bounds, or alter success behavior. The controller owns `packages/engine/crates/mornlea_engine/src/ffi.rs` and `step.rs`; no worker edits those files in this node. Remove the temporary markers in the later evidence-backed repair commit.

Build/format/test the instrumented Rust crate locally, then push normally with the other hosted CI repairs. The existing candidate-bound `native-linux` producer must build this version; capture either the focused `TestPassiveGrazeTurnsGrassToDirtAfterTwentyTicks` failure in `race-server` or another passive-movement failure in that same required workflow. Do not treat old-head or rerun jobs as evidence for a new head.

## Ruling after evidence

- If `decode`, inspect the exact failed predicate and Go encoder before changing any ABI rule.
- If `displacement`, reproduce the failing input/axis in a permanent focused test and compare Go sweep values with Rust integration before adjusting arithmetic or tolerance.
- Create a separate bounded repair task with exact files, tests, and compatibility ruling after the evidence arrives. This task is complete only when the diagnostic result is recorded in `ledger.md`; PR acceptance remains blocked until the repair is tested and temporary logging removed.
