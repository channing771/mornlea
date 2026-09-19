## Context

See [proposal.md](proposal.md) for motivation. This design follows current `main` code rather than version text in historical documents: production protocol v44, engine ABI v11, client ABI v19, and benchmark scenario v23. Current architecture documentation has been synchronized to those code-owned identities; implementation task P0 verifies them again without changing runtime behavior.

Mornlea's "in-house engine" is actually divided among three ownership domains and cannot be replaced as one unit:

```text
                    Current production data flow

     +------------------------------------------------+
     | Authoritative Go server                        |
     | world / sim / storage / server / agent client |
     +----------------------+-------------------------+
                            | protocol v44
                            v
     +------------------------------------------------+
     | Go client logic and CPU presentation half      |
     | mirror / predictor / receiver / mesh queue     |
     | visibility / frame assembly / UI state         |
     +---------------+------------------------+-------+
                     | engine ABI v11         | client ABI v19
                     v                        v
     +---------------------------+  +--------------------------+
     | Rust mornlea_engine       |  | Rust mornlea_client      |
     | Windowless numeric kernel |  | Darwin/winit/wgpu/WebView|
     +---------------------------+  +--------------------------+
```

Current scale is relevant to risk, not code-quality judgment: `mornlea_client/src` contains about 20,000 lines of Rust including same-file tests and test modules; `packages/client/client`, `render`, and `cmd/mornlea/app` together contain about 20,000 lines of non-test Go; and the embedded React frontend contains about 5,600 lines of TypeScript/CSS/JSON. Migration must be treated as building a second client and proving parity one capability at a time, not as replacing a window library.

The Godot baseline is the verifiable `4.7.2-stable` Standard release as of 2026-09-15, sourced from Godot's official maintained-release announcement and download archive. Godot 4.8 development snapshots are prohibited as the pilot baseline. The Rust side uses Godot 4 GDExtension bindings. The exact `godot` crate version is pinned after the first compatibility probe and MUST work with the repository's pinned Rust 1.97.1. If no compatible combination exists, the pilot stops before introducing gameplay code.

### Forward architecture ruling

The pilot deliberately preserves the current Go server and Go client-core so P0–P7 can be completed without a big-bang rewrite. That is a compatibility decision, not a target ownership decision. The final runtime is Rust authoritative server + Rust client-core + Godot with qualified embedded Python presentation, as recorded in [`docs/architecture-target.md`](../../../docs/architecture-target.md).

The embedded Godot Python language is retained in the final design. The migration replaces the Go-backed real-time core and the pilot's Python-to-Go data seam; it does not replace Python with GDScript. Pure GDScript remains a temporary openability/diagnostics Bootstrap only. The standalone Agent Python service remains a separate process and dependency closure.

All work after the completed pilot must use one of two explicit labels:

- **Target work**: adds or moves ownership toward Rust domain/server/client-core or Godot/Python presentation.
- **Transition work**: keeps an existing Go/Python seam alive for compatibility, names its removal condition, and adds no new authoritative or protocol behavior to that seam.

No online dual-authority or shadow-writer phase is permitted. Go is a replay/differential oracle during Rust migration, not a second server. A feature that cannot be expressed through the target typed semantic contract is blocked until the relevant Rust foundation change exists.

Python is not an official first-party Godot scripting language. The first candidate was the community Py4Godot `4.7-alpha21` line, which exposes Python through a second GDExtension and remains explicitly pre-stable. Exact qualification rejected its release artifact because legacy interpreter initialization accepted external `PYTHONPATH`, its headless editor integration emitted errors, and shutdown leaked objects/resources. The user subsequently approved a narrowly scoped project-maintained hardening route. P1 now reproduces a derivative from that exact upstream source revision plus an ordered repository patch stack; the fork may change only interpreter isolation, editor integration, packaging, and lifecycle correctness and may not add Mornlea gameplay/data APIs. The complete Godot 4.7.2, current macOS Apple Silicon, headless/editor, export, repeated teardown, and `mornlea_godot` coexistence matrix remains mandatory. If the hardened derivative still requires system Python, runtime `pip`, an unpinned build input, a broader fork, or a reduced gate, Python-primary gameplay work remains No-Go rather than silently falling back to GDScript.

## Goals / Non-Goals

**Goals:**

- Establish a Godot Standard pilot client that imports no server package, duplicates no authoritative rule, and connects to the existing dedicated server.
- Extract platform-independent session, mirror, prediction, mesh scheduling, and presentation semantics from the current Go client so two graphical hosts can consume them, rather than rewriting those rules in Python or GDScript.
- Use Python as the primary language for the Godot host, feature lifecycle, scene orchestration, desktop device adaptation, and thin presentation mappings. Keep GDScript limited to an engine-native bootstrap and setup diagnostics that remain runnable without Py4Godot.
- Use a Rust GDExtension to adapt Godot objects to the Go client core. Godot owns windows, input, and GPU resources; the Go core calls no Godot API.
- Compare correctness, visual differences, performance, loading, memory, and input latency for the current Rust client and the pilot under the same scenario identity.
- Provide a complete later migration route covering the principal capabilities on current `main`. Each later stage is proposed independently and remains reversible; this change does not implement them all.

**Non-Goals:**

- Move authority owned by `packages/server` or `packages/shared/world`, saves, or the companion Agent service into Godot.
- Reimplement numerical algorithms owned by `mornlea_engine` using Godot Physics, Navigation, or scripts.
- Reuse the standalone `packages/agent` Python environment, modules, process lifecycle, or dependencies inside the graphical client.
- Support local Memory single-player, the complete React UI, all entities/effects, production Windows/Linux releases, or store packaging in this change.
- Change the default `mornlea` entry point, remove Rust shaders/passes, bump client ABI, bump engine ABI, or change protocol v44 before the pilot passes.
- Require the pilot's first frame to be pixel-identical to the current wgpu client. Every difference must instead be classifiable and traceable, with critical interaction and depth/occlusion semantics decidable.

## Decisions

### 1. Separate the pilot change from later complete-migration changes

This change delivers only a remote-TCP vertical slice and a Go/No-Go report. The complete migration table still covers all current functionality, but P8 and later MUST be proposed and reviewed as new changes.

Rationale: complete migration spans windowing, rendering, UI, audio, platform integration, single-player assembly, capture, and release. Rewriting those systems before validating the Godot hot path would make rollback expensive and obscure whether failures come from architecture or incomplete features.

Rejected alternatives:

- Switch the existing client to Godot in one step: there would be no parallel baseline, and regression diagnosis and rollback would be unacceptably expensive.
- Build only an isolated Godot rendering demo: it would not validate protocol, Go mirror/predictor, mesh updates, or real frame boundaries.
- Delete `mornlea_client` before migration: that would break capture, benchmark, and the existing playable baseline.

### 2. Establish a Python Godot product shell with a replaceable transition data adapter

The Godot project does not live at the repository root or inside a Go/Rust module. It uses the stable application root `apps/mornlea-godot/`. This path is both the pilot project root and the future production-client project root. P8–P14 add or replace coarse features in place; they do not rename the directory, replace `project.godot`, or turn the repository root into a Godot project. Python remains the final feature language; only the pilot's Go-backed data adapter is replaceable.

Target layout:

```text
mornlea/
├── apps/
│   └── mornlea-godot/                         # Stable Godot project root
│       ├── project.godot                      # Godot entry and InputMap
│       ├── export_presets.cfg                 # Reproducible export configuration
│       ├── icon.svg
│       ├── README.md                          # English canonical open/bootstrap/run/diagnose guide
│       ├── README.zh.md                       # Chinese counterpart at the same revision
│       ├── pyproject.toml                     # Locked development-only Python quality tooling
│       ├── uv.lock                            # Reproducible tooling lock; not the runtime interpreter
│       ├── app/                               # Stable host kernel; no gameplay rules
│       │   ├── bootstrap/
│       │   │   ├── bootstrap.tscn             # Permanent main_scene
│       │   │   ├── bootstrap.gd               # Only production GDScript: probes and startup routing
│       │   │   └── setup_required.tscn        # Opens even without native artifacts
│       │   ├── host/
│       │   │   ├── app_root.tscn              # Runnable client root
│       │   │   ├── app_root.py                 # Python SceneTree lifecycle orchestration
│       │   │   └── feature_host.py             # Python bounded, explicit feature assembly
│       │   └── routing/                        # Python connect/load/play/terminal transitions
│       ├── config/
│       │   ├── feature_catalog.tres            # Sole feature allowlist
│       │   ├── launch_profiles/                # Remote; local modes may follow later
│       │   └── render_profiles/                # Quality, budgets, and platform configuration
│       ├── features/                           # Python-led vertical presentation capabilities
│       │   ├── session/                        # Connection, loading, disconnection
│       │   ├── player_view/                    # Camera, targeting, player view
│       │   ├── world/                          # Near terrain and environment
│       │   ├── actors/                         # Remote players; later entities
│       │   └── ui/                             # Basic HUD; later complete UI
│       ├── platform/                           # Python desktop-only device/lifecycle adaptation
│       │   └── desktop/
│       │       ├── input/
│       │       ├── lifecycle/
│       │       └── audio/                      # Disabled in pilot; enabled later
│       ├── shared/                             # Truly cross-feature Godot resources
│       │   ├── materials/
│       │   ├── shaders/
│       │   ├── themes/
│       │   └── fonts/
│       ├── assets/
│       │   ├── bootstrap/                      # Diagnostics available without native libraries
│       │   ├── generated/                      # Deterministically derived from existing resources
│       │   └── provenance/                     # Source, license, and checksums
│       ├── addons/
│       │   ├── py4godot/                       # Generated pinned hardened Python runtime
│       │   │   ├── py4godot.gdextension
│       │   │   ├── runtime/                    # Bundled CPython and standard library
│       │   │   └── bin/                        # Current approved desktop artifact only
│       │   └── mornlea_bridge/                 # Sole project-owned gameplay/data boundary
│       │       ├── mornlea_bridge.gdextension
│       │       ├── bridge_host.py              # Python typed facade over Godot-visible methods
│       │       ├── README.md
│       │       ├── README.zh.md
│       │       └── bin/                        # Generated platform distribution units
│       │           ├── macos-universal/{debug,release}/
│       │           ├── linux-x86_64/{debug,release}/
│       │           └── windows-x86_64/{debug,release}/
│       ├── tests/
│       │   ├── scenes/
│       │   ├── fixtures/
│       │   └── scripts/
│       └── .godot/                             # Untracked editor cache
├── packages/
│   ├── client/
│   │   ├── runtime/                            # Platform-independent session and frame stepping
│   │   ├── presentation/                       # Semantic snapshots and bridge encoding
│   │   └── cmd/mornlea-godot-core/             # Go c-shared ABI export
│   └── engine/
│       └── crates/mornlea_godot/               # Rust GDExtension implementation
├── scripts/godot/                              # Fetch/build/sync/check/package
│   └── py4godot/                               # Ordered hardening patches and locked build inputs
└── testdata/godot-pilot/                       # Cross-host transcripts and report inputs
```

`app/` is a stable microkernel. Its pure-GDScript Bootstrap owns only dependency diagnostics and the handoff to Python. After that handoff, typed Python owns session phases, feature assembly, frame order, and shutdown, and knows no terrain, concrete entity, or concrete UI rule. `features/` uses coarse vertical capability boundaries; each feature keeps its scenes, Python scripts, materials, shaders, and test resources nearby. Ordinary internal components do not own manifests. Device and system-API adaptation lives in `platform/desktop/` and is Python unless native performance or API evidence requires a Rust leaf. Only resources genuinely shared by at least two features belong in `shared/`. Complete migration can therefore add or replace features without continually expanding `feature_host.py`, creating dozens of tiny plugins, or relying on global autoloads.

Platform scope is fixed to desktop. `platform/` is not an abstraction for arbitrary devices:

| Platform | Status in this change | Permitted reservation | Explicit exclusion |
|---|---|---|---|
| macOS desktop | Required pilot and current development/validation target | Apple Silicon and, if necessary, universal desktop dylib; keyboard/mouse; focus and exit | iOS, touch, mobile pause/resume |
| Windows desktop | Candidate for a post-P13 change | x86_64 desktop DLL; keyboard/mouse and optional controller; window lifecycle | UWP/mobile and Xbox console release |
| Linux desktop | Candidate for a post-P13 change | x86_64 desktop SO; keyboard/mouse and optional controller; window lifecycle | Android and embedded/handheld-specific adaptation |
| Web/mobile/console | Unsupported | None | Web export, Android, iOS, and console SDKs are excluded from directories, dependencies, and CI |

Godot already provides cross-desktop window and input APIs. `platform/desktop` therefore contains only Mornlea device-semantic conversion, focus/capture/exit lifecycle, and a later audio sink. It does not duplicate ordinary input logic for macOS, Windows, and Linux. A target-specific leaf is added only for a tested OS difference; features MUST NOT branch on the operating system. Export presets, GDExtension library selectors, Go c-shared builds, and release manifests accept only approved desktop target triples. Unknown, mobile, or Web targets fail before templates are downloaded or artifacts are compiled or packaged.

The permanent main scene in `project.godot` is `res://app/bootstrap/bootstrap.tscn`. Bootstrap uses only built-in Godot nodes and pure GDScript; serialized Bootstrap resources MUST NOT directly reference Python scripts or a native class. It first probes the pinned Py4Godot runtime, bundled CPython identity, `MornleaClientBridge`, and distribution-unit identity through dynamic lookups. If either extension or any required runtime artifact is absent or version-incompatible, the editor still opens the project and Bootstrap enters `setup_required.tscn`, which displays the missing files, target triple, and preparation command without importing feature modules or starting network/world state. Only after both identities pass does Bootstrap dynamically load `app_root.tscn`, hand control to `app_root.py`, and assemble features allowed by the catalog.

Only coarse features that can be independently enabled, disabled, or replaced use explicit `feature.tres` manifests. Each manifest declares at least a stable `id`, feature-contract version, Python entry class or scene, feature dependencies, required bridge feature families, required/optional status, budget class, and reset policy. `feature_catalog.tres` explicitly lists the manifests permitted by a run configuration. Dependency direction is fixed as launch profile → feature catalog → feature manifest → required ABI families → client-core registry. The Python host does not scan directories for unknown features or use arbitrary dynamic imports; startup work remains bounded, test resources cannot load accidentally, and filenames do not determine product capability. The common lifecycle is validate → instantiate → bind → activate(epoch) → apply(budgeted data) → reset(epoch) → deactivate. Features consume only typed semantic views and host services exposed as Godot objects; they own no TCP, protocol, mirror, save, authoritative tick, C FFI, or engine-ABI access.

The pilot catalog initially enables desktop input, near terrain, first-person camera, target feedback, a remote player, basic environment, connection UI, and basic HUD. Later stages extend the same contract:

| Later stage | Added or replaced feature space | Does not change |
|---|---|---|
| P8 terrain productionization | Extend terrain/LOD/water/atmosphere components under `features/world/` | Project root, Bootstrap, Go authority boundary |
| P9 complete entities/effects | Extend `features/actors/`; add coarse `features/effects/` or `features/viewmodel/` if justified | Session lifecycle and existing features |
| P10 UI | Extend menu/inventory/crafting/chest/furnace/chat/settings components in `features/ui/` | Protocol and mirror ownership |
| P11 audio/devices | Extend `platform/desktop/{audio,input}/` | Cue-generation semantics and headless boundary |
| P12 tooling | Add capture/benchmark adapters under `tests/` and `scripts/godot/` | Production main scene and runtime features |
| P13 local/release | Extend `config/launch_profiles/` and platform distribution directories | Godot project root and remote-protocol path |
| P14 default switch | Change launch/release configuration and retire the old path | Godot layout, feature contract, and bridge direction |

The hardened Py4Godot derivative is the approved scripting-runtime candidate and the only component allowed to embed CPython. The repository stores only the ordered patch stack, build-input lock, and provenance; exact upstream sources and generated binaries remain reproducible external-cache/project-local outputs. Every patch identifies the upstream defect, allowed boundary, and deletion condition, and the series has one digest. This derivative is replaceable behind the Python host contract and does not become a gameplay/data boundary. Rust `mornlea_godot` remains the sole project-owned native adapter: it registers `MornleaClientBridge` and low-level resource adapters, pulls Go client-core results through a C ABI, and writes Godot `RenderingServer` or resource objects. Python calls only Godot-visible typed methods/signals on that bridge and never loads its C symbols. The Go c-shared library exposes only a C data plane and accepts no Godot or Python callbacks. Each `addons/mornlea_bridge/bin/<target>/<profile>/` directory colocates the project GDExtension, Go client core, and dynamic libraries required by `mornlea_engine`; `addons/py4godot/` separately contains the generated hardened runtime artifact and bundled CPython. One release manifest records both extensions, the interpreter, upstream and patch identities, target, checksums, and licenses; neither extension may search system paths for Python or project libraries.

`packages/client/assets` remains the source of truth for registry, atlas, and existing licensed resources. `scripts/godot/sync-assets.sh` writes only deterministic Godot-consumable derivatives to `assets/generated/` and records input revision and checksums. Parent `res://../` references, absolute paths, and cross-platform-unstable symbolic links are prohibited. `.godot/` and native `bin/` are reproducible and untracked. `.uid` sidecars generated for scripts and shaders by Godot 4.4+ are committed to stabilize resource references.

Standard is selected instead of .NET because Python support arrives through GDExtension and does not require the C# runtime. Rust GDExtension is selected for the project-owned bridge to reuse the repository's Rust toolchain, maintain strong types and panic isolation at the Godot boundary, and keep high-volume mesh/resource work out of Python. Py4Godot remains provisional because its current release line targets Godot 4.7 and exposes Python scripts as Godot classes, but the rejected upstream binary is not approved. Only the explicitly reviewed hardened derivative may proceed, and it remains gated as an alpha community dependency with project-owned maintenance cost.

Rejected alternatives:

- Put `project.godot` at the repository root: `res://` would include all Go/Rust/document/test/build content, increasing import noise and accidental-reference risk.
- Use a pilot-only `packages/godot-client/` and move it during complete migration: that would require a second migration of UIDs, resource paths, CI, and developer workspaces.
- Globally bucket files into `scenes/`, `scripts/`, and `materials/`: each capability would spread across many locations, making deletion, tests, and ownership undecidable.
- Hard-code all functionality in an autoload or main scene: every P8–P14 stage would modify the host kernel, enlarge the regression surface, and create a giant Godot controller.
- Discover features by directory scan: arbitrary files would affect startup cost and product capabilities, violating bounded auditable assembly.
- Prebuild mobile/Web/console abstractions for possible future use: there is no current product requirement, and doing so would introduce touch, application lifecycle, sandboxing, native-ABI, and export-matrix costs. A future need requires an independent architecture change.
- Use GDScript as the primary feature language: this conflicts with the selected maintainability direction and would require a later feature rewrite; GDScript remains only where native startup without the Python extension is required.
- Implement protocol v44, prediction, numerical kernels, or bulk mesh transforms in Python or GDScript: this would create a second gameplay client or move hot paths behind interpreter/GIL overhead.
- Let Python call the client-core or engine C ABI directly: it would create a second unsafe binding and lifetime model beside the Rust bridge.
- Reuse `packages/agent` as the in-process runtime: the service has independent HTTP/MCP/process ownership and dependencies and must remain isolated from graphical-client scripting.
- Rewrite the client in C#: convenient P/Invoke would still add .NET distribution and C# ownership without reducing Go state-machine migration risk.
- Use per-frame IPC between Godot and a Go sidecar: it avoids FFI but increases terrain/entity batch copies, process-lifecycle complexity, backpressure, and diagnostics.
- Load `mornlea_engine` directly from Godot: this violates the existing rule that only `packages/shared/nativeabi` touches the engine ABI.

### 3. The platform-independent Go client core is the migration axis

New `packages/client/runtime` owns remote sessions, the receiver, client mirrors, prediction, sequence numbers, server ticks, mesh workers/queues, fixed frame budgets, and session reset. Current `packages/client/cmd/mornlea/app` continues to assemble the old client window, renderer, WebView, audio, and local Host and calls the same runtime through an adapter. Only after the pilot passes will a later change decide whether the old app fully adopts runtime.

Extraction order is "move tests and pure logic before moving ownership":

1. Add characterization tests for current app message draining, input semantics, reset, prediction, and presentation snapshots.
2. Extract values and interfaces that depend only on `packages/client/client`, `packages/client/mesh`, `packages/client/render`, and `packages/shared/*`.
3. `runtime` MUST NOT import `packages/server`, Godot, the client C ABI, Darwin APIs, WebView, GPU, or audio devices.
4. Both the old app and Godot core pull immutable results from `runtime`; neither host may mutate its mirrors directly.

`packages/client/presentation` stores platform-independent presentation semantics rather than Godot Node/RID values: camera, visible-section identity, mesh upsert/drop, entity kind and transforms, block target, crack stage, weather/day-night state, HUD values, connection phase, and reversible local feedback. The current 96-byte GPU instance and frame TLV are private `mornlea_client` encodings and cannot become the new semantic API.

Rejected alternative: have GDExtension call `packages/client/cmd/mornlea/app` directly. That package is constrained by Darwin build tags and concrete renderer/window lifecycles and is allowed to assemble the server at the application entry point, so it is not a portable core.

### 4. The new bridge uses a pull-only, versioned binary ABI with two-phase capacity queries

The new Go c-shared surface is provisionally client-core ABI v1 and evolves independently from protocol, engine ABI, and client ABI. Its public header lives at `packages/client/cmd/mornlea-godot-core/include/mornlea_client_core.h`; the Rust GDExtension is the sole Godot consumer.

ABI families are divided by responsibility:

| Family | Direction | Semantics | Hot-path constraint |
|---|---|---|---|
| identity/lifecycle | Rust → Go | ABI identity, create, destroy | Create failure releases in reverse order; destroy is idempotent |
| connection | Rust → Go | Begin TCP login, poll phase/error, disconnect | Begin does not block Godot's main thread; poll is bounded |
| input | Rust → Go | Per-frame keys, mouse delta, text/action events | Fixed header and event limit; reject whole batch |
| step | Rust → Go | Explicit elapsed time and message/mesh budgets | No implicit wall clock; no network/GPU wait |
| world | Go → Rust | Section mesh upsert/drop and atlas/registry revision | Monotonic revisions; complete two-phase batches |
| frame | Go → Rust | Camera, environment, entity, target, and HUD semantic snapshot | At most one per step; bounded counts |
| status/metrics | Go → Rust | Stable errors, counters, timing samples | Return no Go pointer or mutable slice |

Client-core ABI v1 returns major/minor version, supported feature families, each family contract version, and capacity limits. The pilot `frame` family carries only the current minimum loop and is not a giant object to which every future capability must add fields. Later capabilities preferentially add independently negotiated and pulled families such as `actors`, `ui-model`, `audio-cues`, `terrain-lod`, `capture`, or a new connection provider. Compatible additions increase minor/family versions and feature identity; changed existing layouts or semantics require a new major and parallel exports during migration. Existing version numbers MUST NOT be repurposed in place.

Each Godot feature declares and consumes only its required ABI feature families. The Python host validates every required family once before instantiating any world feature. A missing optional family disables only the corresponding optional feature. P8–P13 can therefore extend the ABI data plane independently without forcing all Godot features, the old Rust client, or client ABI v19 to rev together.

Every variable-length output uses query-required-size → caller allocation/reuse → exact write. The producer validates and encodes completely into owned, fixed-limit scratch space before one copy to the caller. Insufficient capacity returns required bytes and never writes partially. Rust retains no Go pointer across calls; Go retains no Godot/Rust pointer. Both sides convert panic/recover into stable status codes and never unwind across FFI.

Go workers never call back into Godot or Python. Network and mesh workers write only bounded Go queues, and the Python host pulls through `MornleaClientBridge` at Godot `_process`/`_physics_process` boundaries. This avoids accessing Godot objects from non-main threads and preserves message/render budget control. Python receives compact semantic objects or bounded summaries only; bulk packed terrain and reusable native buffers remain inside Rust. A Python callback may not perform file/network I/O, package installation, unbounded iteration, or CPU-heavy numerical work on the Godot main thread.

### 5. Pilot terrain starts with the standard Mesh path; data selects the production path

`mornlea_engine` currently outputs an 8-byte packed face for each quad, and the current wgpu shader expands it on the GPU. Godot `ArrayMesh`/`RenderingServer` requires standard vertex attributes or Godot-native buffer layouts, so a packed face cannot be used directly as a generic Mesh.

For the pilot, after receiving a valid section-mesh batch, the Rust GDExtension performs bounded expansion of packed quads into vertices, normals, UVs, color/lighting, and indices. It creates separate opaque/cutout/water surfaces and manages RIDs through low-level `RenderingServer`. Section identity is `(dimension,x,y,z,revision)`; replacement and drop are atomic whole-section operations. Expansion MUST NOT happen in the authoritative Go tick or consume unbounded Godot-main-thread time. A Rust worker first prepares owned CPU arrays; the main thread submits them under a fixed per-frame upload budget.

This decision prioritizes correctness and integration validation and does not promise the final performance design. The pilot report records packed-input bytes, expanded vertex/index bytes, per-section CPU expansion time, main-thread submission time, RID/mesh counts, and peak memory.

A later production change selects one of:

- Standard Mesh: simpler and compatible with Godot tooling, but more CPU expansion and GPU memory.
- RenderingDevice/custom shader: retains packed quads and GPU expansion efficiency but reintroduces more custom rendering pipeline, reducing the benefit of a mature engine.

The pilot MUST NOT change engine ABI v11 mesh output. If multiple backend layouts become necessary, a separate engine-ABI change uses profiler data, oracles, and cross-language tests to decide.

### 6. Coordinates, time, and render semantics convert exactly once at the adapter

Go/core retains current world coordinates, yaw/pitch, column-major matrices, authoritative ticks, and explicit elapsed time. The Godot adapter defines one conversion-function family with golden/property tests:

| Semantics | Go/Mornlea | Godot adaptation rule |
|---|---|---|
| World position | Current `core` coordinates | Convert only when writing Node/RenderingServer transforms |
| Section identity | Dimension plus signed X/Y/Z | Map to a stable RID-table key, never a scene path |
| Camera orientation | Current yaw/pitch and camera mode | One function generates Godot Basis; entities do not duplicate formulas |
| Time | Server tick plus explicit elapsed | Gameplay/weather uses tick; interpolation/local feedback uses passed elapsed |
| Color/texture | Current atlas and linear/nonlinear conventions | Pin filter/mipmap/color-space import settings; do not rely on editor defaults |
| Transparency | Opaque/cutout/water routing | Separate surfaces/materials/passes, not one blended queue |
| Depth | Target outline, cracks, and name tags have distinct existing semantics | Specify depth test/write for each class and cover occlusion visually |

Godot Physics does not participate in player or block-authoritative collision. A Godot Node transform is only the final confirmed or predicted presentation value; physics queries continue through Go → `nativeabi` → `mornlea_engine`.

### 7. UI, audio, and local mode stay minimal in the pilot and are decided independently later

The pilot HUD uses Python-authored Godot Control scenes only for connection state, health, hunger, oxygen, and an explicit limited-pilot label; it does not port the complete React/WKWebView UI. UI input produces only semantic actions. Go runtime validates phase, view token, and authoritative prerequisites before sending commands.

Complete UI migration has two candidate routes:

- Rewrite with Godot Control: gains cross-platform and editor-layout capabilities but rebuilds about 5,600 lines of frontend behavior and visual baselines.
- Retain React: requires a cross-platform WebView plugin, transparent composition, input participation, resource scheme, and security boundary, reducing Godot's direct benefit.

This change does not decide the complete UI route. The pilot Control HUD is sufficient to validate downstream state and upstream input.

The pilot plays no audio, avoiding simultaneous Godot and Darwin AudioQueue devices. A separate complete-migration change keeps cue triggering in Go and gives device, mixing, volume, and resource lifecycle to Godot AudioServer.

The pilot supports remote TCP only. Complete migration separately compares:

- Have Godot start and supervise a `mornlea-server` child process and log in over loopback TCP. This is simpler but changes the current single-process/Memory distribution shape.
- Embed Host and Memory transport inside the Go client-core c-shared library. This preserves the shape but expands ABI lifecycle and the crash domain.

### 8. Dependency direction and distribution units remain explicit

Target dependency graph:

```text
apps/mornlea-godot/project.godot
        |
        +-- pure-GDScript app/bootstrap (dependency probe only)
        |                   |
        |                   +-- addons/py4godot (pinned embedded CPython runtime)
        |                                      |
        +-- Python app/host + features/* + platform/desktop/*
                            | typed Godot API + feature contract
                            v
              addons/mornlea_bridge (project-owned data boundary)
              mornlea_godot (Rust GDExtension)
                         | client-core ABI v1 + feature families
                         v
              mornlea-godot-core (Go c-shared main)
                         |
                         +-- packages/client/runtime
                         +-- packages/client/presentation
                         +-- packages/client/client (mirror/predictor/mesher)
                         +-- packages/shared/{network,core,physics,nativeabi,...}
                                                               | engine ABI v11
                                                               v
                                                       Rust mornlea_engine
```

Forbidden edges: Python/GDScript → protocol, prediction, numerical, or bulk-mesh reimplementation; Python → client-core/engine C ABI or `packages/agent`; feature/platform → TCP/mirror/save; feature → another feature's private implementation; Godot → server packages; `mornlea_godot` → engine ABI; Go runtime → Godot/Python/client ABI/Darwin; server → Godot/client. Inter-feature dependencies use only stable IDs declared by manifests and host services, never absolute scene paths, unrestricted dynamic imports, autoload lookup, or shared mutable singletons. Production GDScript outside the bootstrap/diagnostics allowlist is rejected.

The pilot distribution unit contains the Godot executable/project pack, catalog-selected Python features, required desktop adapters, the exactly pinned Py4Godot extension with its embedded/isolated CPython runtime and standard library, the `mornlea_godot` GDExtension, the Go client-core dynamic library, and the `mornlea_engine` dynamic library. Export presets exclude tests, development Python tooling, setup diagnostics, provenance, non-selected features, and non-target-platform dynamic libraries. The release manifest records all component versions, enabled features, target triple, checksums, and licenses. No system Python, user site-packages, online package resolution, or system dynamic-library search may participate in development smoke tests or exported runtime behavior. Complete migration extends this same release manifest rather than creating another product directory or distribution identity.

### 9. Validation uses three evidence chains: semantic parity, visual classification, and performance records

Semantic parity takes priority over pixel parity. Identical network messages and input sequences must produce identical mirrors, prediction, commands, and presentation-semantic snapshots. Offline transcripts drive runtime without requiring Godot or a GPU.

During the pilot, Godot is not a formal visual-baseline producer. `godot-visual-evidence` writes captures, run identity, and structural summaries to `build/visual/godot-pilot/<run-id>/`; `godot-visual-compare` reads an existing same-semantics baseline and produces a difference/classification report. Neither writes `testdata/visual-golden/`. Visual classes remain based on observable semantics rather than renderer: window/UI in `ui/`, a stable headless world frame in `world/`, and cross-tick processes in `motion/` for bounded human end-to-end review only, not automated pixel comparison. `testdata/visual-golden/godot/` is prohibited.

After a later feature reaches production parity, its independent change may propose a producer handoff. The handoff records the old and new producer, scene/fixture identity, expected differences, affected files, item-by-item human review, and rollback. Before explicit approval, Godot evidence outside thresholds is only Go/No-Go data and MUST NOT be erased by relaxing thresholds or overwriting a baseline.

Visual validation has three classes:

1. Must be equivalent: chunk content, material choice, transparency class, entity identity/position, camera target, HUD values, and outline depth relationship.
2. May differ within approved bounds: antialiasing, tone mapping, shadow sampling, and font rasterization.
3. Not covered by the pilot: complete menus, containers, viewmodel, particles, far ring, and advanced weather. The report explicitly lists each missing item.

Performance reports use the same scenario version, seed, resolution, view distance, warmup, sampling window, and hardware:

| Metric | Current client | Godot pilot | Decision method |
|---|---|---|---|
| Stable-frame CPU/GPU P50/P95/P99 | Existing benchmark | New Godot runner | Record values; explain the cause of any P95 regression |
| Cold start to login/visible world | Existing loading/capture | Same criterion | MUST NOT fake faster loading with fewer target chunks |
| Section-mesh CPU and upload | Packed upload | Expansion plus upload | Report amplification and main-thread duration |
| Script-host cost | Not applicable | Python host/apply P50/P95/P99 plus allocation pressure | Explain any unbounded callback or frame-time concentration |
| RSS/VRAM proxy | Existing RSS/resource counts | RSS plus RID/mesh bytes | Record peak and steady state |
| Input-to-presentation latency | Existing explicit probe | Same input sequence | Do not count server-tick delay as an engine difference |
| Queue/overflow | Existing counters and hard failure | Bridge/runtime counters | Any data loss is immediate No-Go |

Go/No-Go is determined by these hard conditions:

- Go: the exact hardened Py4Godot/CPython artifact is reproducible from the pinned upstream revision and patch series, works offline on the current macOS target, and exports with complete provenance; no authority/protocol/prediction fork exists; no silent loss or partial batch occurs; terrain and the minimum entity run stably; both GDExtensions and the Go core close repeatedly; Python main-thread cost is bounded; performance regressions have concrete optimization points; and Godot demonstrably removes in-house window/GPU/cross-platform shell burden.
- No-Go: embedded Python execution requires system Python, runtime package installation, an unpinned build input, a broader or unreviewed binding fork, or a GDScript gameplay fallback; protocol or physics must be rewritten in Godot/Python; Python enters bulk terrain/numerical hot paths; packed-mesh adaptation cannot meet bounded budgets for the target scene and low-level custom rendering is the only route; either GDExtension/Go c-shared lifecycle is unstable; the default client must change prematurely; or report identity is incomplete. A failed Python qualification blocks the Godot cutover; it does not authorize moving presentation features to unqualified GDScript.

### 10. Principal current-`main` feature migration matrix

"Retain" means remain the current source of truth during this pilot, not necessarily the final language owner. "Extract" means first make semantics host-independent; the final extraction target is Rust client-core/server unless a row explicitly says otherwise. "Adapt" means preserve semantics while changing the Godot/Python presentation outlet. "Rewrite" is allowed only for pure presentation/device implementation. "Retire" happens only after complete parity and a default switch.

| Current area / representative files | Principal current function | Pilot treatment | Complete migration target | Key validation |
|---|---|---|---|---|
| `packages/contracts` | Shared Go/Python JSON contracts | Untouched | Retain | Contract goldens unchanged |
| `packages/agent/companion` | Planner/Dialogue/memory service | Untouched and forbidden as a client import | Retain independent service | Agent integration gates unchanged; source guard rejects client reuse |
| `packages/shared/core` | Coordinates, blocks/items, raycast domain entry | Reuse directly | Transition domain identities and kernel-facing contracts to Rust | Coordinate-conversion property tests |
| `packages/shared/world` | Chunk/section/palette/snapshot data model | Reuse directly | Transition world data model and semantic snapshots to Rust domain | Three section-storage forms round-trip |
| `packages/shared/network/protocol` | Protocol v44 packets and domains | Reuse directly | Transition the canonical wire contract to Rust protocol with compatibility fixtures | Handshake, codec, fuzz |
| `packages/shared/network/codec,tcp` | Framing, strict codecs, TCP stream | Reuse directly | Transition session/codec ownership to Rust server/client-core | Truncation/trailing/limit failures |
| `packages/shared/physics` | Player collision source and physics-semantic adapter | Reuse directly | Transition deterministic physics semantics to Rust kernel | Predictor/authority oracle |
| `packages/shared/nativeabi` | Sole Go bridge to engine ABI v11 | Reuse directly | Retire after Rust server/client-core consume the Rust kernel directly | ABI/capacity/panic tests |
| `packages/server/sim/*` | Authoritative tick, realm, entities, rule resolution | Untouched | Transition authority to Rust server/domain through replay parity | Server race and simulation tests |
| `packages/server/fluid` | Bounded fluid orchestration | Untouched | Retain | Existing fluid tests |
| `packages/server/storage` | World/player/entity saves and migration | Pilot does not access | Transition save ownership and migrations to Rust storage/server | Schemas/goldens unchanged |
| `packages/server/server` | Host, login, session, publication, shutdown | Existing TCP service only | Transition network/session authority to Rust server | Existing remote-login/shutdown tests |
| `packages/client/client/mirror.go`, `snapshot.go` | Chunk mirror and reset | Compose/reuse in runtime | Transition to Rust client-core after F3 | Transcript-state equivalence |
| `receiver.go` | Bounded server-message receive | Move/wrap in runtime | Transition to Rust protocol/client-core after F2/F3 | Overflow, close, immutability |
| `predictor*.go`, `collision.go` | Input prediction, replay, authoritative correction | Compose/reuse in runtime | Retain | Same-input bitwise/tolerance equivalence |
| `input.go`, `camera*.go` | Semantic input and camera modes/occlusion | Extract device-independent input | Retain semantics; Godot only collects devices | Cursor, F5, three-view tests |
| `remote_players.go`, `companions.go`, `hostiles.go`, `passives.go`, `projectiles.go` | Authoritative entity mirrors and interpolation | Pilot starts with remote players | Adapt all entities in batches | Tick and spawn/state/despawn order |
| `inventory.go`, `chest.go`, `furnace.go`, `chat.go` | Authoritative UI mirrors | Basic HUD subset only | Transition semantic mirror ownership to Rust client-core; Python only presents it | Unconfirmed operations do not alter mirrors |
| `mesher*.go`, `mesher_*queue.go` | Mesh workers, scheduling, backpressure | Include in runtime | Transition scheduling and bulk preparation to Rust client-core/kernel | Zero extra steady-state allocation, overflow |
| `render_world_update.go` | MRW1 atomic world-cache update encoding | Design reference; no direct reuse promise | May evolve into host-neutral world delta | Epoch/revision/tombstone |
| `packages/client/mesh` | Section-input encoding and engine mesh calls | Reuse | Retain | Mesh oracle and capacity tests |
| `packages/client/lod` | Far-ring tile scheduling and encoding | Not in pilot | Adapt to Godot later | Far radius/fog/budgets |
| `packages/client/assets` | Registry, atlas, procedural materials, item icons/geometry | Reuse registry/atlas | Retain data generation; add Godot import adapter | Layers, alpha, mipmaps, authorization |
| `packages/client/render/section_scheduler.go` etc. | Connectivity, visibility, upload/drop budgets | Extract host-neutral decisions | Transition decisions to Rust client-core; Godot owns resources | Visible set and upload-sequence parity |
| `render/avatar*.go`, `drop*.go`, `projectile*.go` | Entity presentation poses and current GPU-instance encoding | Reuse semantic poses only | Retain poses; eventually retire 96-byte encoding | Pose/death/interpolation goldens |
| `render/daylight.go`, `weather.go`, `celestial.go` | Day/night, weather, sky, precipitation parameters | Basic environment in pilot | Retain parameters; rewrite shaders | Tick phase and clear/rain branches |
| `render/block_outline.go`, `block_crack.go` | Target outline and cracks | Implement target feedback | Adapt Godot material/pass | Depth test, reset, stages |
| `render/viewmodel*.go` | First-person hand/item pose and action | Not in pilot | Adapt later | Action continuity, near clip, HUD safe area |
| `render/name_tag.go`, font atlas | Name-tag layout and glyph upload | Absent or debug-text substitute | Retain layout semantics; Godot font rendering | Unicode, occlusion, capacity |
| `render/hud` | HUD/container data and historical GPU layout | Basic values only | Rewrite presentation with selected UI route | Values, hit testing, visual baseline |
| `packages/client/audio` | Darwin AudioQueue procedural cues | Silent pilot | Rewrite device layer with Godot AudioServer | Rising edge, volume, headless silence |
| `app/app_startup.go`, `app_dependencies.go` | Local/remote assembly and reverse-order cleanup | New remote-only runtime constructor | Split platform core and host assembly | Failure order, no premature window |
| `app/app_messages.go` | Message drain and mirror/entity/UI/audio routing | Extract to runtime and event output | Transition to Rust client-core event output | Same state/event result per message |
| `app/app_input.go`, `app_game_ui.go` | Phase gates, command assembly, view token | Pilot only movement/look/target | Migrate all semantic actions later | Stale-token/unconfirmed rejection |
| `app/app_frame.go`, `app_render.go` | Per-frame drain, prediction, mesh, presentation assembly | Split into runtime step plus Godot apply | Eventually retire concrete old-renderer path | Same budgets and frame snapshot |
| `app/interactive.go` | Menu/loading/game event loop | Godot SceneTree owns outer loop | Retain state-machine semantics | Pause/resize/focus/close |
| `app/app_menu*.go`, `app_settings.go`, `debug_panel.go` | Menus, settings, pause, F3 state | Connection/error UI only | Later Godot UI or React decision | State documentation and input participation |
| `app/app_load.go` | Loaded criterion and loading progress | Reuse criterion | Retain | Same view-distance target; no fabricated completion |
| `app/app_audio.go` | Derive cues from confirmed state | May record without playback | Rust emits semantic cue; Python/Godot owns playback | Cue exactly once |
| `cmd/mornlea/capture` | Windowless fixed-scene PNG | Keep old path | Add Godot headless capture later | No focused window, same scene identity |
| `cmd/mornlea/benchmark` | Benchmark v23 scenario and report | Comparison truth | Share scenario description later | Identity, warmup, sample completeness |
| `cmd/mornlea/devcapture` | Interactive-window capture coordination | Not in pilot | Adapt to Godot viewport capture later | Non-blocking delivery |
| `mornlea_engine` | Mesh/light/collision/raycast/physics/worldgen/LOD/fluid | Keep ABI and sole implementation | Fold into Rust domain/kernel contracts as adapters are retired | Rust determinism, replay, and numerical oracles |
| `mornlea_client/window.rs,input.rs,camera.rs` | Darwin window/input/native camera calculations | Keep old client | Retire corresponding exports after Godot parity | Old-client baseline continues to pass |
| `mornlea_client/render/*,shaders.rs` | wgpu GPU resources, passes, shaders | Keep old client | Retire in stages after Godot parity | Visual/benchmark/capacity |
| `mornlea_client/overlay.rs,webview.rs` and `frontend` | WKWebView plus complete React UI | Keep old client | Migrate UI to Godot Control/Python in P10; retire after producer handoff | Bridge schema and visual fixtures |
| `apps/mornlea-godot/app` | Migration-only pure-GDScript Bootstrap plus Python host lifecycle and phase routing | Establish stable microkernel and language allowlist | Retain Python host; remove or minimize the Bootstrap after the final launcher can diagnose missing runtime | Opens without libraries, Python handoff, idempotent close, stable main scene |
| `apps/mornlea-godot/config` | Feature catalog and launch/render profiles | Establish pilot catalog | Evolve through profiles and coarse manifests | Dependency, version, budget, required/optional validation |
| `apps/mornlea-godot/features` | Python-led vertical scene-based presentation capabilities | Minimum-loop features only | Extend by capability through P8–P13 against Rust semantic views; Python remains the final feature language | Python typing, isolation, missing/failure semantics, no hidden dependency |
| `apps/mornlea-godot/platform/desktop` | Python desktop input/audio/window-lifecycle adaptation | Keyboard/mouse and window lifecycle only | Add desktop audio/controller later; no mobile/Web reservation | Headless avoids device, semantic input, target rejection |
| `scripts/godot/py4godot`, `apps/mornlea-godot/addons/py4godot` | Ordered project hardening stack plus generated Python scripting runtime and embedded CPython | Exact upstream base and approved minimal derivative only after compatibility gate | Replaceable only through an independent binding change | Upstream/patch/build/output identity, license, offline startup/export, teardown, platform matrix |
| `apps/mornlea-godot/addons/mornlea_bridge` | Project-owned Godot native adapter and platform-library descriptors | New sole gameplay/data entry | Evolve through additive feature families | GDExtension/ABI/distribution identity; Python never loads C symbols |
| `apps/mornlea-godot/assets`, `shared` | Godot-consumable derivatives and cross-feature resources | Synchronize minimum authorized resources | Continue deterministic generation from source assets | `res://` closure, checksum, license, UID |
| `packages/audit` | Dependency, version, no-Go-GPU, and sole-authority guards | Add Godot boundary checks | Retain and update | Full audit passes |
| `scripts/godot`, Makefile, CI | Rust/Go/resource/visual/release gates | Add optional Godot bootstrap/checks | Join formal release gates after cutover | Pinned versions, resource closure, no foreground window |

### 11. Complete migration stage table

Size is relative engineering complexity: S is one boundary; M spans multiple packages with stable behavior; L crosses runtimes or visual systems; XL is a product-level transition. P0–P7 belong to this change. P8–P14 define feature routes for later changes, but they cannot skip the Rust foundation stages below.

| Stage | Size | Scope and principal code | Prerequisite | Deliverable | Exit condition | Rollback |
|---|---:|---|---|---|---|---|
| P0 baseline freeze | S | Current versions, scenarios, client feature inventory, old benchmark/visual | None | Baseline manifest and difference classes | Identity complete; old gates reproducible | Delete new documents/reports |
| P1 stable host and dependency probe | L | `apps/mornlea-godot`, pure-GDScript Bootstrap, Python capability host, Godot 4.7.2, Rust 1.97.1, godot-rust, Py4Godot/CPython, two GDExtensions | P0 | Directly openable stable project root, missing-library/runtime diagnostics, Python host handoff, 100 empty-feature load/unload cycles, CI headless/export smoke | Opens without native libraries; exact Python/runtime/bridge unit runs offline on macOS; no crash, leaked handle, system Python, runtime install, or escaped resource | Record Python-primary No-Go and delete crate/project |
| P2 Go runtime extraction | L | Receiver, mirror, predictor, message drain, reset, budgets | P0 | Platform-independent runtime and transcript tests | Old-app characterization passes; runtime has no platform dependency | Keep old implementation and revert extraction |
| P3 client-core ABI | L | Go c-shared, Rust GDExtension, feature negotiation, input/step/world/frame/status | P1/P2 | ABI v1 header, bindings, feature-family identity, fuzz/capacity matrix | Invalid input fails atomically; required families decidable; repeated create/destroy stable | Delete new ABI without affecting v19 |
| P4 terrain path | XL | Atlas, mesher, section queue, quad expansion, RenderingServer | P3 | Near world and deltas load | Content/transparency correct, no data loss, bounded budgets | Keep pilot as No-Go evidence or delete it |
| P5 remote game loop | L | TCP login, input, predictor, camera, target, disconnect | P4 | Movable/correctable first-person loop | v44 behavior equivalent; disconnect/errors understandable | Old client unchanged |
| P6 minimum presentation | M | Remote player, day/night/weather, health/hunger/oxygen HUD | P5 | Required minimum entity and HUD | State comes only from confirmed mirror; count limits valid | Remove optional presentation nodes |
| P7 dual-client decision | M | Capture, benchmark, RSS, report, review | P6 | Go/No-Go report and later recommendations | Every hard condition has evidence; OpenSpec strict validation passes | On No-Go archive evidence and delete runtime entry |
| F1 Rust domain/protocol/kernel foundation | XL | Rust domain, protocol, storage contracts, deterministic kernels, replay corpus | P7 Go | Language-neutral contracts and Rust reference behavior | Go replay oracle and Rust implementation agree; no second online writer | Keep Go production path and discard Rust adapter |
| F2 Rust authoritative server | XL | Rust tick, world/entity rules, persistence, validation, network session | F1 | Rust server-core and migration adapters | Deterministic replay, save migration, failure-path parity, same Memory/TCP semantics | Keep Go authority; no dual-write mode |
| F3 Rust client-core and typed Godot bridge | XL | Rust session, mirror, prediction, reconciliation, semantic snapshots, direct Godot bridge | F1; F2 protocol | Go-free client-core path consumed by Python features | Transcript parity, correction/replay, bounded bridge, repeated lifecycle | Keep pilot Go core without adding features |
| P8 productionize terrain | XL | Rust terrain/mesh pipeline plus Python `features/world/*`, LOD, water, cutout, fog, lighting, resource pools | F3 | Independent world feature and semantic families | Target view distance and stable frame meet approved budgets; no Go terrain logic added | Catalog points to prior feature; old client remains default |
| P9 complete entities/effects | L | Rust typed entity families plus Python `features/actors/*`, effects, viewmodel | F3; P8 where terrain resources are required | Independent presentation change | Each behavior/visual specification reaches parity; features independently disableable | Disable by feature in catalog |
| P10 UI migration | XL | Python Godot Control features and versioned Rust view-model family | F3 | Independent UI change set | Token/hit/confirmation/visual fixture parity without new GDScript or WebView dependency | Keep old UI client and disable new manifests |
| P11 audio/devices | M | Python `platform/desktop/*`, Godot AudioServer, semantic cue family, keyboard/mouse/controller | F3 | Independent change | Cue exactly once, headless touches no device, desktop adapter replaceable | Disable adapter or use old client |
| P12 tooling | L | Rust replay/perf contracts, Python/Godot tests, capture, benchmark, import, CI | F1–P9 as applicable | Independent change | Replaces acceptance without focusing windows or product-runtime Python tooling | Continue old toolchain |
| P13 local play and desktop release | XL | Rust server-core local mode, shared protocol path, Python presentation packaging, macOS/Windows/Linux | F2–P12 | Independent change | Local/remote semantics share one source; desktop unit is closed; mobile/Web rejected | Return to remote-only or old client |
| P14 default switch and retirement | XL | Default entry, release catalog, retirement of Go runtime/client ABI and pilot-only bootstrap seams | F1–P13 complete | Final cutover change | Two release cycles without blocking regression and a usable rollback package; Python Godot layout stable | Restore previous release; prohibit partial deletion |

## Concurrency and frame boundary

Each pilot Godot frame follows this order. This is the migration-era Go-backed path; the final frame boundary replaces the Go calls with direct Rust client-core calls while keeping Python as the presentation language. Numeric limits become named constants and report identity during implementation design review:

```text
Godot main thread
  1. Python desktop adapter polls Godot input -> bounded semantic InputBatch
  2. Python host calls MornleaClientBridge.submit_input(InputBatch)
  3. Python host calls MornleaClientBridge.step(elapsed, messageBudget, meshBudget)
       +-- Go receiver goroutine publishes only immutable messages to a bounded queue
       +-- Go main runtime drains, validates, and updates mirror/predictor
       +-- Go mesh workers publish only owned results to a bounded completion queue
  4. Rust bridge drains at most uploadBudget WorldBatch values
  5. Rust worker expands packed quads; main thread submits ready Godot Mesh/RID values
  6. Python pulls one compact FrameSnapshot and updates camera/entities/environment/HUD
  7. Godot renders; no synchronous readback in this frame
```

Connection establishment and DNS/TCP/login MUST NOT block on the Godot main thread. Python callbacks have explicit work budgets and do not hold or expose native buffers after a bridge call. Shutdown first stops Python input/feature dispatch, cancels receiver/mesh work, waits for bounded native workers, drains or discards presentation results with an old epoch, releases the Go session and project bridge resources, deactivates Python features, and finally releases Godot RIDs, the Python runtime, and dynamic-library handles. Repeated shutdown is safe.

The final frame boundary is:

```text
Godot main thread
  1. Python desktop adapter samples bounded semantic input
  2. Rust Godot bridge submits input to Rust client-core
  3. Rust client-core performs bounded receive, mirror, prediction, reconciliation, and mesh scheduling
  4. Rust client-core publishes one immutable semantic FrameSnapshot
  5. Python features apply typed views to Godot scenes/resources
  6. Godot renders without synchronous protocol, storage, or server queries
```

The migration must prove semantic parity between these two frame boundaries before removing the Go-backed path.

## Compatibility and versioning

| Contract | This change | Notes |
|---|---|---|
| protocol | Remains v44 | Pilot consumes only current version; no old-server compatibility |
| player/chunk/world metadata | Unchanged | Pilot does not directly read or write saves |
| companion/hostile/passive schemas | Unchanged | Still owned by server/storage |
| engine ABI | Remains v11 | Godot never calls it directly; still reached through Go nativeabi |
| client ABI | Remains v19 | Old production client path unchanged |
| client-core ABI | New v1 transition contract | Pilot-only Go c-shared ABI; final Rust client-core uses a separately versioned typed Godot bridge and must not preserve Go as a runtime dependency |
| Godot project root | Fixed at `apps/mornlea-godot/` | If pilot passes, production client grows in place through P8–P14 |
| Godot feature contract | New v1 | Coarse manifest/catalog/lifecycle; compatible additive growth, explicit version bump for breaking change |
| benchmark scenario | Remains v23 | New reports reuse identity; a changed scenario shape requires a separate bump |
| Godot | Pinned `4.7.2-stable` | Binary/export templates and checksums recorded in manifest |
| Python scripting runtime | New pinned hardened Py4Godot derivative | P1 records exact upstream, patch-series, build inputs, generated plugin, embedded CPython, artifact checksums, licenses, supported target, and export identity; alpha status and project maintenance cost are not waived |
| Python host contract | New v1 and retained in the target | Python owns Godot host/features/platform scripts; pure GDScript owns only migration Bootstrap diagnostics; breaking host changes require explicit versioning |
| platform scope | Desktop only | Pilot macOS Apple Silicon; later Windows/Linux desktop only; Android/iOS/Web/console rejected |

## Risks / Trade-offs

- [Godot, CPython, Rust, and Go runtime layers increase lifecycle complexity] → Use a pull-only C ABI behind one Rust bridge, no cross-thread Godot/Python callbacks, repeated create/destroy tests for both GDExtensions, and strict reverse-order release; P1/P3 failure is immediate No-Go.
- [A pre-stable community Python binding and project patch stack become a production dependency] → Pin the upstream source, ordered patch series, build inputs, generated artifact, and embedded CPython with checksums/licenses; prohibit gameplay/data APIs in the fork; prove editor/headless/export behavior on the current macOS target; run mutation and 100-cycle lifecycle tests; and stop at P1 on any unresolved crash, platform gap, system-runtime dependency, or irreproducible build. A matching Godot minor label is not sufficient evidence.
- [Python obscures performance or moves work behind the GIL] → Retain Python as the final bounded presentation language, keep bulk terrain/resource preparation and state/protocol work in Rust in the target (Go only during transition), measure Python callback/apply duration and allocation pressure, and fail review if a hot path requires unbounded Python work.
- [Developers obtain different Python environments] → Runtime uses only the project-local pinned Py4Godot/CPython unit; development tools use the checked `uv.lock`; system Python, user site-packages, and runtime `pip` are rejected by validation.
- [The standalone companion Agent and client Python are conflated] → Keep separate project roots, locks, imports, and processes; audit that `apps/mornlea-godot` never imports `packages/agent`.
- [Packed-quad expansion substantially increases CPU, memory, or upload volume] → P4 records input/expanded/VRAM proxies; the pilot exposes amplification, and production chooses standard Mesh or low-level buffers from data.
- [Runtime extraction changes existing client behavior] → Add characterization/transcript tests first and keep the old client consuming the shared logic; move only one ownership loop at a time.
- [Two clients drift] → Use language-neutral transcripts and Rust as the target reference. During the transition, every new feature explicitly names whether it is a compatibility seam or target work; gameplay is never duplicated in two online authorities.
- [The stable host becomes a service locator or giant controller] → `app/` exposes only lifecycle, budgets, bridge views, and diagnostics. Concrete presentation lives in explicit coarse features; new host services require design and dependency audit.
- [Feature growth creates hidden dependencies or assembly-order drift] → Only independently enableable/replaceable boundaries own manifests; the catalog fixes directed dependencies, versions, and required/optional status. Directory scans, cross-feature private paths, and shared mutable autoloads are prohibited.
- [Feature catalog and ABI registry become duplicate or cyclic truths] → Direction is profile→catalog→manifest→required ABI families→client-core registry. The catalog chooses product assembly; the registry only reports data-plane availability.
- [Feature families fragment or create an unmanageable compatibility matrix] → One registry and release manifest own family IDs/versions/limits; additive and breaking evolution rules are fixed, and features declare minimum dependencies only.
- [Godot pilot output becomes a fourth visual-baseline class] → The pilot writes only `build/visual/godot-pilot` evidence. Formal producer handoff is separately approved and reuses existing UI/world/GIF classes.
- ["Godot-openable" is mistaken for "playable without a build"] → Bootstrap distinguishes editor-openable from gameplay-ready; missing libraries show setup diagnostics only, and CI separately validates opening without native artifacts and running a complete distribution.
- [Godot/GDExtension, godot-rust, Py4Godot, or CPython upgrade breaks compatibility] → Pin exact versions, lock Cargo/Python tooling, and record download checksums. Upgrades are independent changes rather than incidental feature changes.
- [Godot main-thread upload stutters] → Prepare CPU expansion on Rust workers; the main thread consumes only fixed-budget completions. Per-block Nodes and synchronous server queries are prohibited.
- [One scene Node per section/entity is too expensive] → Terrain prefers a RenderingServer RID table. Entity counts decide among Nodes, MultiMesh, or low-level instances; one-object-one-Node is not assumed.
- [Visual differences are misclassified as functional errors or accepted without review] → Classify each as must-match, bounded difference, or uncovered, with an owner and evidence.
- [UI rewrite cost is underestimated] → P10 is independent and uses Godot Control with Python as the target feature language; UI is not mixed into the terrain pilot and does not introduce a GDScript feature fallback.
- [Local mode changes single-process semantics] → P13 uses the same Rust server-core, login, validation, and protocol path for local and remote play. The pilot is remote-only and does not silently change default single-player.
- [New assets violate authorization or plugins cannot be redistributed] → Audit provenance, licenses, sources, and checksums; default to existing procedural/project-owned assets.
- [Copied derivative resources drift from source assets] → `packages/client/assets` stays authoritative; `sync-assets` deterministically generates and verifies input revisions/checksums; manual edits in `assets/generated/` are prohibited.
- [Stale architecture-document versions mislead review] → P0 verifies code-owned identities through audit and treats no historical document as behavioral truth.

## Migration Plan

### Implementation and deployment in this change

1. Complete P0/P1 and establish the stable project root, pure-GDScript Bootstrap, reproducible hardened Py4Godot/CPython runtime, Python capability host, and pilot catalog in `apps/mornlea-godot/`. Default builds, tests, and runtime remain independent of Godot. Before native libraries are built, the project still opens and provides preparation diagnostics. The rejected upstream artifact does not itself block the explicitly approved hardening task, but P2 MUST NOT begin unless the hardened Python compatibility/export/lifecycle gate is Go.
2. Through P2, the old client remains the only playable client. Existing tests must prove that runtime extraction does not change behavior.
3. After P3–P6, add an explicit `godot-pilot` build/launch entry. It connects only to an explicit address, does not automatically start a local world, and never writes saves. Every pilot capability enters through a catalog manifest; it cannot be added directly to Bootstrap or as an unnegotiated ABI field.
4. P7 produces a complete-identity dual-client report and a reviewed Go/No-Go decision. P8 MUST NOT begin without that decision.
5. A Go decision authorizes only proposals for P8 and later changes; it does not authorize a default switch or old-code deletion. Before any P8–P14 feature is implemented, the Rust foundation stages F1–F3 in the migration table must be proposed and independently validated.
6. F1–F3 use replay and differential evidence to move server, protocol, persistence, client-core, prediction, and mesh scheduling ownership to Rust. The Go runtime remains an offline oracle or explicit compatibility adapter and never becomes a second online authority.
7. P8–P13 then migrate feature presentation into Godot/Python against Rust semantic families. Python remains the final Godot feature language; pure GDScript is not a feature fallback.
8. P14 switches the default only after Rust server/client-core, Godot/Python presentation, local/remote parity, packaging, and rollback have passed the release criteria in `docs/architecture-target.md`.

### Rollback

- P1–P7 are additive. On pilot No-Go, remove `apps/mornlea-godot/`, both GDExtensions, the bundled Python runtime, client-core ABI, and optional scripts, and revert runtime-extraction wiring. On Go, `apps/mornlea-godot/` becomes the stable host for later production migration and is not recreated or moved.
- `mornlea`, `mornlea-server`, production Memory/TCP paths, engine/client ABIs, and saves remain unchanged, so no data rollback is required.
- If the pilot fails at runtime, it does not silently switch to the old renderer in process. The user explicitly exits the pilot and launches the existing client.
- Until P14, the old client remains buildable, runnable, and able to generate the original benchmark/capture. No stage may remove its gates using "temporary compatibility."

### Later change boundaries

- `productionize-godot-terrain-rendering`: P8.
- `port-godot-entity-presentation`: P9, further splittable by entity/effect.
- `port-game-ui-to-godot` or `embed-cross-platform-web-ui-in-godot`: choose one for P10.
- `port-client-audio-and-device-input-to-godot`: P11.
- `add-godot-capture-and-benchmark`: P12.
- `package-godot-local-and-remote-client`: P13.
- `cut-over-default-client-to-godot`: first part of P14.
- `retire-go-client-core-and-pilot-abi`: second part of P14, only after at least one stable release stage. The qualified embedded Godot Python runtime remains part of the final presentation architecture.

## Open Questions

- P13 may choose whether the Rust server-core runs in-process behind the same loopback protocol or as a supervised child process. Either choice must use the same login, validation, and packet path; it cannot restore a privileged Go Memory implementation.
- P8 may choose standard Godot Mesh or RenderingDevice/custom packed buffers from expansion, upload, and memory measurements. This does not change Rust ownership of bulk preparation or Python ownership of presentation orchestration.
- The first formal cross-platform desktop target may be Windows, Linux, or both. The pilot first validates the architecture on the current Darwin/macOS machine. This question cannot expand scope to Android, iOS, Web, or console.
