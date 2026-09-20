## 1. Target preflight

- [ ] 1.1 Do not implement P13 until Rust server stage F2 and client-core stage F3 are complete enough to expose one local/remote login and validation path. Verify with `rg -n 'F2|F3|same.*path|local|remote' docs/architecture-target.md openspec/changes/godot-desktop-packaging/{proposal.md,design.md}`.
- [ ] 1.2 Keep the old default entry, legacy Memory path, and remote TCP path available until packaging and rollback are proven. Verify with `go test ./packages/audit -run GodotIsOptionalForLegacyBuild -count=1`.

## 2. One Rust server path and Python packaging

- [ ] 2.1 Choose and specify either an in-process Rust server-core loopback transport or a supervised Rust server process; prove that login, packets, validation, persistence, and error semantics are identical for local and remote play. Verify with `cargo test -p mornlea_server local_remote_parity --locked`.
- [ ] 2.2 Package the Godot client with the qualified embedded Python runtime and catalog-selected Python features. Reject system Python, runtime installation, the standalone Agent import, mobile/Web/console targets, and non-target native libraries. Verify with `make godot-project-check`, `make godot-python-check`, and `go test ./packages/audit -run 'GodotPythonIsolation|GodotDesktopOnly|ReleaseUnit' -count=1`.

## 3. Release and closeout

- [ ] 3.1 Run local/remote replay and save-compatibility tests, confirming that no privileged Go Memory implementation remains in the target package. Verify with `cargo test -p mornlea_server local_remote_replay --locked` and `go test ./packages/audit -run 'Authority|Replay|GodotBoundary' -count=1`.
- [ ] 3.2 Build and inspect macOS/Windows/Linux desktop release closures, licenses, checksums, and rollback artifacts without adding mobile/Web/console presets. Verify with `make godot-build`, `make godot-project-check`, and the release-unit audit.
- [ ] 3.3 Reconcile this candidate with current code, the target architecture, the default-entry rollback, and save/protocol compatibility before implementation closeout. Verify with `openspec validate godot-desktop-packaging --type change --strict --no-interactive`.
