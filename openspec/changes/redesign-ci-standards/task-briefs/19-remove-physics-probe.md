# Node 6.3.7: Remove temporary native rejection probe

## Identity and ownership

- Prerequisite: Node 6.3.6 has passed focused Go, golden, and sneak-edge regressions and has a scoped commit. This node is owned by the controller after reviewing the Go repair.
- Deliverable: remove only the temporary `reject=decode` and `reject=displacement` stderr branches introduced in `660f0ccd`/`13bdb45a`, restoring the pre-diagnostic Rust behavior and exact `step.rs` source provenance. No change to ABI status, rejection threshold, integrator arithmetic, or test corpus.
- Editable files: `packages/engine/crates/mornlea_engine/src/ffi.rs`, `packages/engine/crates/mornlea_engine/src/step.rs`, and `testdata/runtime-migration/contracts.json`. OpenSpec status and evidence remain controller-owned. All other source files are read-only.
- Required Superpowers sub-skills: `test-driven-development` for the cleanup guard, `verification-before-completion`, and `requesting-code-review` for the joint Go/Rust result. Do not use `git checkout` or a broad reset to restore files; remove exact diagnostic hunks with a reviewed patch.

## Execution and proof

1. Confirm Rust displacement rejection is still covered by the existing one-ULP-accept/two-ULP-reject tests and Go output parity by frozen vectors. Verify the temporary marker is present before the cleanup; `rg -n 'reject=(decode|displacement)'` is the pre-cleanup witness.
2. Remove the diagnostic-only enum/writer and FFI logging, leaving the original return codes and validation order. `rg -n 'reject=(decode|displacement)' packages/engine/crates/mornlea_engine/src` must return no matches. `shasum -a 256 packages/engine/crates/mornlea_engine/src/step.rs` must equal the original frozen SHA `fd1648625bf2d176dd46d02c0d67ce22d918c4f69def4b447ff81f39b996db67`; set only the `kernel.mornlea_physics_step` source digest in `testdata/runtime-migration/contracts.json` back to that value. If exact restoration differs, inspect the diff and record the reason rather than forcing the old hash.
3. Run Rust formatting and the engine crate tests, rebuild the native library, run focused runtime-oracle replay and the shared physics tests, then full local stage-boundary gates. Run an independent code review of the Go parity change plus probe cleanup before push.

This node closes only after the provenance guard and tests pass. Linux acceptance still requires a new candidate-bound Required CI run, and the optional Godot run remains separately visible. A rollback reintroduces the diagnostic only as an explicitly scoped new probe, not as part of a production PR.
