## 1. Target preflight

- [ ] 1.1 Do not implement P14 until F1–F3, all required P8–P13 feature changes, the target Python runtime qualification, and the rollback release are complete. Verify with `rg -n 'F1|F2|F3|P8|P9|P10|P11|P12|P13|rollback|final.*Python' docs/architecture-target.md openspec/changes/pilot-godot-client-migration/design.md`.
- [ ] 1.2 Keep the existing `mornlea` default entry unchanged until the cutover change is independently approved and the old release remains runnable. Verify with `go test ./packages/audit -run GodotIsOptionalForLegacyBuild -count=1`.

## 2. Final cutover

- [ ] 2.1 Prove that the default Godot path uses Rust server/client-core semantic contracts and embedded Python presentation, with no Go real-time dependency, production GDScript feature path, or standalone Agent import. Verify with `go test ./packages/audit -run 'Architecture|GodotPythonBoundary|GodotIsOptionalForLegacyBuild' -count=1` and the Rust release gates.
- [ ] 2.2 Run at least two release-cycle parity/rollback checks covering protocol, saves, local/remote play, visual semantics, performance hard failures, and clean removal of the old entry only after the rollback package is published. Verify with the approved release checklist and `make dev-check`.

## 3. Closeout when implementation is requested

- [ ] 3.1 Reconcile this candidate with current code, the target architecture, all P8–P13 decisions, release evidence, and the retirement boundary. Verify with `openspec validate godot-default-client-switch --type change --strict --no-interactive` and `openspec validate --all --strict --no-interactive`.
