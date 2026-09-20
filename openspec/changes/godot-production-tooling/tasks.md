## 1. Target preflight

- [ ] 1.1 Do not implement P12 until Rust replay/protocol/client-core contracts required by each tool are stable and the producer handoff is explicitly approved. Verify with `rg -n 'F1|F2|F3|replay|offline oracle|embedded Python' docs/architecture-target.md openspec/changes/godot-production-tooling/{proposal.md,design.md}`.
- [ ] 1.2 Keep the legacy capture, benchmark, and CI producers available as rollback paths; do not make Go and Rust concurrent online authorities. Verify with `go test ./packages/audit -run GodotIsOptionalForLegacyBuild -count=1`.

## 2. Language-neutral tooling and Python evidence

- [ ] 2.1 Define Rust-owned replay identities, report schemas, and deterministic fixture interfaces that can drive server and client-core without Godot or Python. Verify with `cargo test -p mornlea_tools replay --locked` and `go test ./packages/tools/perfcheck -run 'GodotPilot|Replay' -count=1`.
- [ ] 2.2 Implement Godot/Python capture, semantic visual comparison, benchmark adapters, and resource import checks without protocol decoding or unbounded callbacks. Verify with `make godot-python-check`, `make godot-visual-evidence`, and `make godot-visual-compare`.

## 3. Handoff and closeout

- [ ] 3.1 Compare every proposed new producer with the old producer using the same scenario identity, and record missing/non-comparable evidence explicitly. Verify with `make godot-benchmark`, `make visual-check`, and `go test ./packages/audit -run VisualBaselineRouting -count=1`.
- [ ] 3.2 Request an explicit producer handoff before changing tracked baselines or required CI ownership; a pilot capture alone cannot authorize the handoff. Verify with `go test ./packages/audit -run 'Architecture|ReleaseUnit|VisualBaselineRouting' -count=1`.
- [ ] 3.3 Reconcile this candidate with current code, the target architecture, replay rollback, and legacy tool availability. Verify with `openspec validate godot-production-tooling --type change --strict --no-interactive`.
