## 1. Candidate planning only

- [ ] 1.1 Do not implement P12 until this change is independently applied. Verify with `rg -n 'Decision: GO' docs/notes/godot-client-pilot-report.md`.
- [ ] 1.2 Keep the existing `mornlea` default entry unchanged. Verify with `go test ./packages/audit -run GodotIsOptionalForLegacyBuild -count=1`.

## 2. Closeout when implementation is requested

- [ ] 2.1 Reconcile this candidate with current code before any implementation. Verify with `openspec validate godot-production-tooling --type change --strict --no-interactive`.
