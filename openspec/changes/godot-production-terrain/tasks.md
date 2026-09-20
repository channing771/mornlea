## 1. Target preflight

- [ ] 1.1 Do not implement P8 until Rust foundation stages F1–F3 are independently proposed, validated, and linked from `docs/architecture-target.md`. Verify with `rg -n 'F1|F2|F3|Rust client-core|embedded Python' docs/architecture-target.md openspec/changes/pilot-godot-client-migration/design.md`.
- [ ] 1.2 Keep the existing `mornlea` default entry and legacy terrain producer unchanged until an approved producer handoff. Verify with `go test ./packages/audit -run GodotIsOptionalForLegacyBuild -count=1`.

## 2. Rust terrain contract and Python presentation

- [ ] 2.1 Add or consume the Rust domain/client-core terrain family for chunk revisions, visibility, mesh preparation, LOD inputs, water/cutout classification, and bounded upload work; write replay/property tests before implementation. Verify with `cargo test -p mornlea_client_core terrain --locked` and `cargo test -p mornlea_kernel terrain --locked`.
- [ ] 2.2 Implement `apps/mornlea-godot/features/world/` in embedded Python as a typed presentation consumer. It MUST not decode protocol bytes, own revisions, call raw ABI symbols, or add production GDScript. Verify with `make godot-python-check`, `make godot-project-check`, and `go test ./packages/audit -run 'GodotPythonBoundary|GodotFeatureBoundary' -count=1`.

## 3. Parity and closeout

- [ ] 3.1 Compare the Rust terrain path against the recorded Go/legacy transcripts offline; reject stale revisions, overflow, data loss, and any online dual-authority path. Verify with `cargo test -p mornlea_client_core terrain_replay --locked` and `go test ./packages/audit -run 'Authority|Replay|GodotBoundary' -count=1`.
- [ ] 3.2 Produce semantic visual and bounded performance evidence for the existing `world/` baseline without creating a renderer-specific golden class. Verify with `make godot-visual-compare`, `make godot-benchmark`, and `go test ./packages/tools/perfcheck -run GodotPilotReport -count=1`.
- [ ] 3.3 Reconcile this candidate with current code, the target architecture, and the producer-handoff rollback before implementation closeout. Verify with `openspec validate godot-production-terrain --type change --strict --no-interactive`.
