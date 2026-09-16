---
name: mornlea-architecture
description: Apply and maintain Mornlea's verified cross-task architecture conventions. Use before cross-boundary design or implementation, and at the end of each implementation round to promote durable findings.
---

# Mornlea Architecture Conventions

Use this skill as a compact decision aid, not as a replacement for repository truth. Resolve conflicts in this order: code and tests, canonical `openspec/specs/`, `docs/architecture.md`, current progress documentation, then historical evidence. Read the scoped `AGENTS.md` nearest the files being changed.

## Durable ownership rules

- The server is the sole authority for world, player, entity, inventory, and gameplay outcomes. Clients submit intent and hold mirrors, reversible prediction, and presentation state only.
- Local Memory and remote TCP reuse the same packet, login, validation, session, and authoritative simulation paths. Transport choice must not create a privileged gameplay path.
- Go owns application/domain orchestration and the CPU side of presentation. `mornlea_engine` remains the sole production numerical implementation for the kernels it owns. GPU and host APIs stay outside Go packages.
- Cross-language calls use the established versioned bridges. Validate identity, layout, pointers, lengths, alignment, overlap, capacity, and failure atomicity before publishing results. Do not add a production fallback beside a native implementation.
- The per-step client frame aggregate is an immutable semantic value in `packages/client/presentation` (`FrameSnapshot`). Every host and any client-core ABI frame family consume that record instead of growing a second host-private or renderer-specific frame encoding.
- A host with already-constructed session state consumes `packages/client/runtime` through the adoption seam instead of duplicating mirror, predictor, mesher, or sequence ownership; both Memory and remote TCP paths keep one behavior source, and payload-less drop operations never consume publication capacity reserved for upserts.
- The client-core ABI's canonical surface is the header at `packages/client/cmd/mornlea-godot-core/include/mornlea_client_core.h`: Go derives its constants through cgo, Rust mirrors constants and `#[repr(C)]` layouts while parsing the same header text, and both sides enforce both-direction define-set equality so a define can never drift silently between the two consumers.
- Messages and slices become immutable after a successful cross-goroutine send. Tick, network, render, bridge, and upload hot paths use explicit bounded work and avoid blocking I/O.

## Godot client direction

- `apps/mornlea-godot/` is the stable Godot project root. `project.godot` and the pure Godot bootstrap remain stable as the pilot grows into a production client.
- Treat the source project and every export preset as explicit resource closures. Keep one root `project.godot`; reject parent or absolute resource references, escaping or broken source symlinks, implicit autoload state, ignored UID/import sidecars, and generated editor cache state in Git. Each preset may target macOS, Windows, or Linux desktop only and must exclude tests, development tooling, setup diagnostics, provenance, catalog-unselected features, and every non-target native/Python library family. Add target-specific closure rules when another desktop preset is introduced instead of weakening the common gate.
- `app/` is a feature-agnostic host. Coarse independently replaceable vertical capabilities live under `features/`; ordinary scenes and leaf components do not receive plugin manifests.
- Product assembly flows one way: launch profile → feature catalog → feature manifest → required client-core feature families → client-core registry.
- Godot and GDScript own scene composition, desktop input adaptation, UI, rendering resources, and presentation. They do not own protocol decoding, authoritative mirrors, prediction rules, saves, or numerical gameplay kernels.
- `platform/desktop/` is the only platform-adapter family. The supported product scope is macOS, Windows, and Linux desktop. Do not prebuild Android, iOS, Web, console, touch, sensor, or mobile-lifecycle abstractions.
- On macOS export, treat GDExtension libraries, the selected embedded CPython tree, and catalog-selected Python sources as an explicit filesystem packaging closure. Resolve libraries from the exported application layout and materialize Python runtime/source payloads under `Contents/Resources`; do not assume Python can import or load source from the PCK, current directory, system paths, or user paths.
- Keep Godot Python production and development environments separate. Production dependencies remain explicitly empty unless independently approved; the embedded Py4Godot/CPython unit never receives Ruff, mypy, `uv`, `pip`, or developer packages. Lock development-only tools with the project `uv.lock` and type against local Py4Godot stubs rather than generated add-on modules.
- Compose isolated Godot Python features through explicit catalog, manifest, scene, and resource paths. Do not depend on the project root being in `sys.path`, directory scanning, unrestricted dynamic imports, or sibling implementation imports; Godot owns discovery and Python owns the typed host lifecycle behind those resources.
- Keep dynamic Godot resource ownership correct in the Py4Godot binding adapter or audited derivative. Host and feature code must not carry manual reference-count workarounds for generated-wrapper defects; repair and qualify those defects at the binding boundary so load, instantiate, reset, deactivate, and teardown remain leak-free.
- With the current qualified Py4Godot derivative, Python must not use `ClassDB.instantiate()` for project GDExtension classes: it returns an unusable generic wrapper without an owned native pointer. Keep identity-only calls static through Godot `ClassDB`; any later Python-held native instance requires a binding-boundary repair plus lifecycle qualification before feature code may consume it.
- Validate the Godot distribution lifecycle as one ordered stack: Rust initializes `Scene` → optional `Editor` → `MainLoop`, then Python assembles the host; shutdown deactivates Python features and releases the bridge before Rust deinitializes `MainLoop` → optional `Editor` → `Scene`. Convert Rust panics and invalid transitions to stable boundary failures, and repeat the complete headless process cycle when lifecycle ownership changes.
- Keep `packages/client/assets` and the registered client font sources authoritative. Materialize Godot-local atlas, font, license, and provenance derivatives only through the deterministic asset synchronizer; bind them to source Git trees plus per-input and aggregate checksums, and reject missing, changed, symbolic-link, or hand-authored files under `assets/generated/`.
- Keep Godot build gates optional leaf entry points: legacy make entries (`build`, `test`, `run`, `companion-agent-check`) never reference `scripts/godot` or a `godot-*` target, `scripts/godot` references are admitted only inside the explicit godot gate rules, and the CI Godot job stays outside required pipeline gates until an approved cutover change promotes it.

## Visual and documentation direction

- Route visual evidence by observable semantics, not renderer identity: UI fixtures use `ui/`, stable headless world frames use `world/`, and cross-tick human-review GIFs use `motion/` without automated pixel comparison.
- Godot pilot captures are untracked evidence under `build/visual/godot-pilot/`. A tracked producer changes only through an approved handoff; never add a renderer-specific golden class or relax thresholds to make a pilot pass.
- English is canonical for active/new OpenSpec prose, plans, machine governance, and all new or substantively rewritten source comments. Existing non-English comments and unchanged canonical-spec prose are grandfathered behind non-growth inventories. New or substantively revised explanatory and architectural documents use English `*.md` plus synchronized Chinese `*.zh.md`; unchanged pre-policy documents may remain manifest-classified `legacy` until revision.
- New architecture and boundary code must include concise English comments or doc comments at ownership, lifecycle, compatibility, and non-obvious failure decisions. Explain intent and trade-offs rather than restating syntax; a new architectural unit with no explanatory comments is incomplete.

## Round-end promotion

At the end of each completed OpenSpec task or explicitly batched set of small related tasks, review ownership, dependency, lifecycle, concurrency, platform, visual, validation, and documentation findings.

Promote a finding into this skill only when all conditions hold:

1. current code, tests, or canonical specifications verify it;
2. it applies across future tasks rather than only the completed task;
3. it changes a future placement, dependency, lifecycle, validation, or orchestration decision;
4. it is not already stated more authoritatively elsewhere;
5. it is concise, references its authority, and contains no volatile count or task history.

Update the Codex and Claude copies atomically and validate their byte equality. If nothing qualifies, do not edit the skill; record `Architecture skill: no change` with the reason in the active change ledger.
