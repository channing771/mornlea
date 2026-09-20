# Engine Test Support

`packages/engine/tests` provides shared test-only support for offline evidence and corpus verification across the foundation crates (`mornlea_domain`, `mornlea_protocol`, `mornlea_storage`).

## Invariants

- Code in this directory is test-only support. It must NEVER be added to production `[dependencies]` in any crate manifest.
- Only offline evidence and corpus fixture loading belong here. No production authority, network transports, or live game simulation may be imported.
- All file access is bounded by the harness budgets (manifest <= 4 MiB, case JSON <= 256 KiB, binary inputs <= 4 MiB, cases <= 8192).
- Paths are resolved relative to the workspace repository root discovered from `CARGO_MANIFEST_DIR`. Absolute paths, parent traversals (`..`), and symlinks are strictly rejected.
