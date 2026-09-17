---
doc_id: godot-client-project
language: en
counterpart: README.zh.md
revision: 2026-09-16.5
---

# Mornlea Godot Client

This directory is the stable Godot project root for the desktop client pilot and any later approved full migration. Open this directory itself in Godot Project Manager; do not open the repository root. The project remains here through later migration stages so scene UIDs, `res://` paths, tooling, and release identities do not need a second relocation.

## Open the project

The project requires Godot 4.7.2 Standard. Verify the pinned official artifacts without downloading them:

```bash
scripts/godot/fetch.sh --verify-only
```

Open and import the project without a foreground window:

```bash
scripts/godot/godot.sh --headless --path apps/mornlea-godot --editor --quit
```

You may set `MORNLEA_GODOT_BIN` to an absolute path for an exact Godot 4.7.2 executable. The wrapper also accepts the official `/Applications/Godot.app` installation when its version matches the project pin.

The permanent main scene is pure GDScript and built-in Godot nodes. Python and native artifacts are optional while opening the editor. Bootstrap reports missing Python extension, embedded interpreter, standard-library, and project-bridge files; incompatible Godot or extension descriptors; the detected desktop target; and the exact preparation command. This diagnostic path neither imports Python features nor connects to a server.

Prepare the current macOS Apple Silicon runtime and bridge with:

```bash
scripts/godot/build-python-runtime.sh --verify --offline
scripts/godot/build-extension.sh --target aarch64-apple-darwin --profile debug --verify
```

Exercise both clean-state diagnostic paths without modifying the project checkout:

```bash
scripts/godot/openable-smoke.sh --without-native
scripts/godot/openable-smoke.sh --without-python
```

## Asset synchronization

`packages/client/assets` remains authoritative for the registered atlas, and `packages/client/render/assets` remains authoritative for the registered Noto Sans CJK font. Materialize their Godot-local derivatives with:

```bash
scripts/godot/sync-assets.sh
scripts/godot/sync-assets.sh --check
```

The generated directory contains the layer-major, mip-major RGBA8 atlas, the registered font and its OFL/provenance files, retained material license records, and a deterministic manifest. The manifest records the source Git trees, every input checksum, one aggregate input checksum, atlas layout, and every output checksum. Do not edit or add files under `assets/generated/`; the checker rejects missing, changed, symbolic-link, and hand-authored files.

## Python development checks

Production Python dependencies are empty. The embedded CPython runtime contains only the qualified Py4Godot unit and does not install development tools. Ruff and mypy are development-only dependencies resolved exactly by `uv.lock`; `uv` may populate the local ignored `.venv/` from that lock, but it does not modify or install into the embedded runtime. Local Py4Godot stubs under `typing/` provide the checked interface without importing generated add-on code.

Run formatting, lint, strict typing, boundary mutation tests, and source-policy checks with:

```bash
scripts/godot/python-check.sh --locked
```

The boundary check rejects companion Agent imports, direct native ABI or dynamic-library access, Python-side networking, runtime installers, process execution, unrestricted dynamic imports, and non-English source comments.

## Python feature host contract

After Bootstrap completes the dependency handoff, `app/host/app_root.py` and `app/host/feature_host.py` own catalog planning and feature lifecycle. `config/feature_catalog.tres` is the explicit allowlist; it names coarse `feature.tres` manifests instead of scanning directories. Manifests declare host protocol `1.0`, one project-local entry scene, stable dependencies, versioned bridge-family requirements, required or optional failure semantics, a budget class, and a reset policy.

The lifecycle is deterministic: validate the catalog, instantiate in sorted dependency order, validate the Python feature, inject one typed Godot bridge service, activate for an epoch, reset, and deactivate in reverse order. An incompatible or failed required feature stops assembly and releases already active features. An optional feature is disabled with an observable result, and dependents cannot silently activate through it. Feature scripts do not import sibling project modules or discover implementations dynamically; Godot resource paths provide bounded composition while the isolated interpreter keeps project directories out of `sys.path`.

Run the embedded-Python contract, additive-extension, and native-bridge integration checks with:

```bash
scripts/godot/feature-contract-check.sh
scripts/godot/feature-contract-check.sh --extensibility-probe
scripts/godot/feature-contract-check.sh --bridge-integration
```

## Architecture boundary

Godot owns desktop windowing, keyboard/mouse collection, presentation, pilot UI, and Godot resource lifecycles. The Go client runtime will continue to own protocol v44, mirrors, prediction, semantic frame state, and bounded network processing. Numerical mesh, lighting, collision, raycast, and physics remain in engine ABI v11. The authoritative Go server remains the only owner of world and player truth.

Future functionality extends the same root through coarse `features/`, `platform/desktop/`, `config/`, and the single `addons/mornlea_bridge/` boundary. Bootstrap must remain feature-agnostic. Mobile, Web, console, touch, sensors, and mobile lifecycle support are outside this project.

Generated editor cache and native bridge binaries are ignored. Script and shader `.uid` sidecars are intentionally tracked once Godot generates them.
