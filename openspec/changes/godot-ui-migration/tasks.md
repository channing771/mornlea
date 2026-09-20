## 1. Target preflight

- [ ] 1.1 Treat Godot Control with embedded Python as the final UI route; do not implement a new WebView route or production GDScript feature path. Verify with `rg -n 'Godot Control|embedded Python|no production GDScript' docs/architecture-target.md openspec/changes/godot-ui-migration/{proposal.md,design.md}`.
- [ ] 1.2 Do not implement P10 until Rust client-core stage F3 publishes versioned semantic UI view-models and the existing React/WebView producer remains rollback-capable. Verify with `go test ./packages/audit -run GodotIsOptionalForLegacyBuild -count=1`.

## 2. Rust view-model and Python Control UI

- [ ] 2.1 Inventory existing UI events, commands, hit/confirmation semantics, and visual fixtures, then define versioned Rust semantic view-models and typed intent families. Verify with `cargo test -p mornlea_client_core ui --locked` and the existing UI characterization tests.
- [ ] 2.2 Implement `apps/mornlea-godot/features/ui/` in embedded Python/Godot Control. It MUST consume typed views, never own the authoritative mirror, parse packets, call raw ABI symbols, or add GDScript gameplay. Verify with `make godot-python-check`, `make godot-project-check`, and `go test ./packages/audit -run 'GodotPythonBoundary|GodotFeatureBoundary' -count=1`.

## 3. Parity and closeout

- [ ] 3.1 Compare UI intent and view-model transcripts against the legacy producer offline, including stale-token, rejection, focus, resize, and reset paths. Verify with `cargo test -p mornlea_client_core ui_replay --locked` and the focused legacy UI tests.
- [ ] 3.2 Run semantic UI fixtures and human review, then request an explicit producer handoff before modifying tracked baselines. Verify with `make godot-visual-compare`, `go test ./packages/audit -run VisualBaselineRouting -count=1`, and `make frontend-check`.
- [ ] 3.3 Reconcile this candidate with current code, the target architecture, and rollback to the old UI producer. Verify with `openspec validate godot-ui-migration --type change --strict --no-interactive`.
