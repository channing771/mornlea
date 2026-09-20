## 1. Target preflight

- [ ] 1.1 Do not implement P9 until Rust foundation stages F1–F3 are independently proposed, validated, and linked from `docs/architecture-target.md`. Verify with `rg -n 'F1|F2|F3|typed entity|embedded Python' docs/architecture-target.md openspec/changes/pilot-godot-client-migration/design.md`.
- [ ] 1.2 Keep the existing `mornlea` default entry and legacy actor producer unchanged until each actor family has an approved producer handoff. Verify with `go test ./packages/audit -run GodotIsOptionalForLegacyBuild -count=1`.

## 2. Rust entity families and Python presentation

- [ ] 2.1 Define Rust typed entity families, identity/epoch/order/overflow semantics, and replay fixtures for companions, hostiles, passives, projectiles, viewmodel, and effects. Verify with `cargo test -p mornlea_client_core entity --locked` and `cargo test -p mornlea_server entity --locked`.
- [ ] 2.2 Implement `apps/mornlea-godot/features/actors/`, `effects/`, and `viewmodel/` as embedded Python presentation features over typed snapshots. Verify with `make godot-python-check`, `make godot-entity-check`, and `go test ./packages/audit -run 'GodotPythonBoundary|GodotFeatureBoundary' -count=1`.

## 3. Parity and closeout

- [ ] 3.1 Run offline Rust-versus-Go replay comparisons for spawn, update, interpolation, despawn, reset, duplicate, backward-order, and capacity failures; do not run two online writers. Verify with `cargo test -p mornlea_client_core entity_replay --locked` and `go test ./packages/audit -run 'Authority|Replay|GodotBoundary' -count=1`.
- [ ] 3.2 Produce semantic visual evidence for actor identity, occlusion, animation, and effect timing without adding a renderer-specific golden category. Verify with `make godot-visual-compare` and `go test ./packages/tools/perfcheck -run GodotPilotReport -count=1`.
- [ ] 3.3 Reconcile this candidate with current code, the target architecture, feature disable/rollback behavior, and the producer handoff before implementation closeout. Verify with `openspec validate godot-complete-actors --type change --strict --no-interactive`.
