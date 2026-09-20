## 1. Target preflight

- [ ] 1.1 Do not implement P11 until Rust client-core stage F3 defines semantic cue and device-intent families. Verify with `rg -n 'F3|semantic cue|embedded Python|Godot' docs/architecture-target.md openspec/changes/godot-desktop-audio/{proposal.md,design.md}`.
- [ ] 1.2 Keep the old audio/device producer and default `mornlea` path unchanged until the desktop handoff is approved. Verify with `go test ./packages/audit -run GodotIsOptionalForLegacyBuild -count=1`.

## 2. Rust events and Python desktop adapters

- [ ] 2.1 Define Rust semantic cue, controller-intent, focus, and headless policies with replay and exactly-once tests. Verify with `cargo test -p mornlea_client_core audio_input --locked`.
- [ ] 2.2 Implement `apps/mornlea-godot/platform/desktop/` in embedded Python and Godot AudioServer/Input APIs. Python MUST not import the standalone Agent, call raw ABIs, own authority, or add production GDScript. Verify with `make godot-python-check`, `make godot-input-check`, and `go test ./packages/audit -run 'GodotPythonBoundary|GodotDesktopOnly' -count=1`.

## 3. Parity and closeout

- [ ] 3.1 Compare cue and device-intent transcripts with the legacy client, including focus loss, repeated state, cancellation, headless execution, and teardown. Verify with `cargo test -p mornlea_client_core audio_input_replay --locked` and the focused legacy audio tests.
- [ ] 3.2 Run desktop-only and no-device validation on the supported targets, recording semantic behavior rather than adding renderer-specific baselines. Verify with `make godot-check`, `make godot-smoke`, and `go test ./packages/audit -run 'GodotDesktopOnly|VisualBaselineRouting' -count=1`.
- [ ] 3.3 Reconcile this candidate with current code, the target architecture, and adapter rollback before implementation closeout. Verify with `openspec validate godot-desktop-audio --type change --strict --no-interactive`.
