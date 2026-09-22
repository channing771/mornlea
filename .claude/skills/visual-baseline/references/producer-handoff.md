# Producer handoff during Rust/Godot migration

Use this reference only when designing evidence infrastructure or transferring a case to a new producer. The target stage definitions live in [`docs/architecture-target.md`](../../../../docs/architecture-target.md); implementation tasks and evidence belong in the owning OpenSpec change.

## Progress through the architecture stages

| Stage | Evidence to establish | What it does not authorize |
|---|---|---|
| F1: shared Rust contracts | Versioned replay/wire/save identities; deterministic domain/kernel results against the offline Go oracle | Replacing a graphical producer |
| F2: Rust authority | Tick, validation, save/failure parity and shared local/remote semantics | Two online writers or screenshot-based authority proof |
| F3: Rust client-core | Typed snapshot/intent parity, correction/reset, bounds and repeated bridge teardown | A new feature in the pilot Go core or raw wire work in Python |
| P8–P11: presentation features | Per-feature world/UI/motion cases and typed behavior/lifecycle tests; separate audio/device evidence | Updating every baseline because one feature migrated |
| P12: evidence and tooling | Case registry, identity validator, explicit candidate comparison, strict coverage acceptance and reviewed update tooling | Handing off cases before their feature is ready |
| P13: distribution | Captures and lifecycle checks from each qualified desktop artifact, with packaged assets/fonts/Python | Generalizing a macOS source-tree run to Windows/Linux exports |
| P14: default and retirement | Two release cycles, all required case owners transferred or explicitly retired with replacement coverage, usable rollback release | Removing old producers before rollback and coverage are proven |

P12 infrastructure can be developed after its Rust prerequisites and before individual feature handoffs; final handoff consumes feature evidence. Do not create a dependency cycle by requiring all feature handoffs before the tooling they need exists. Keep current legacy regression running for cases that have not transferred.

## Handoff record

Record the following in the feature design/ledger, or in its machine-readable registry when P12 implements that registry. This is a planning checklist, not a claim that a new JSON schema or command already exists.

- **Case and behavior:** stable identity, `ui`/`world`/`motion` route, current registry source, observable assertion, PNG/GIF responsibility, feature/catalog selection.
- **Owners:** current canonical producer, candidate producer, Rust semantic-family version, feature owner and handoff status (`legacy`, `candidate`, `accepted`, or explicitly `retired`). Only one accepted canonical owner writes a given tracked file.
- **Input and readiness:** replay/fixture digest and version, seed, confirmed tick/revision, camera, viewport/scale, time/weather, locale/font and asset digests, bounded settle rule, frame limits and failure reason.
- **Provenance:** Git SHA and dirty state/patch identity, run identity, server/core/bridge versions, Godot/embedded Python versions, export or source-tree identity, OS/arch, actual display/graphics driver and GPU, output paths and hashes.
- **Comparison:** old-to-new semantic mapping, environment compatibility, comparator/policy identity, old/candidate/diff artifacts, metrics where meaningful, expected rendering differences, reviewer decision, and missing/non-comparable cases.
- **Transfer and rollback:** exact tracked write set, authorized update command and its side effects, restored producer/registry/baseline revision on rollback, and the follow-up regression command. Release rollback also preserves compatible saves and selects one server authority.

Use the existing identity validator's fields now. If planned metadata is absent, keep supplementary evidence and identify the gap; implement schema/version changes under P12 before treating them as machine-enforced gates.

## Compare the right things

**Same producer, same environment:** use the current PNG comparator and measured thresholds. Dimension mismatch, missing baseline, identity failure, or missing output is a failure. Record actual and diff outputs. Current world comparison also protects near-ring invariants during explicit updates.

**Different producers:** first match the semantic input, camera, viewport, assets, readiness and intended observable behavior. A similarly named scene is insufficient. If no valid mapping exists, retain both runs as not comparable and block that case's handoff. Do not fabricate a successful mapping or use a pixel metric to certify gameplay parity.

Once semantics match, classify rasterization, color, material, layout, or timing differences and inspect the result. Review can accept an intentional renderer change without pretending it passed the old producer's pixel gate. Establish repeatability within the new producer/environment before explicit update, preserving the existing comparison policy. If repeatability cannot meet it, the handoff remains blocked pending a separately justified policy change.

**Motion:** compare the pre-trigger, trigger, outcome and settled phases, frame count, cadence and semantic event sequence. Preserve bounded full-process GIFs for review; do not introduce automated GIF pixel acceptance. Diagnostic key PNGs stay untracked unless an independent static contract is explicitly recorded.

## Transfer sequence

1. Confirm the feature's Rust prerequisites and typed semantic tests with evidence tied to its code revision. A checked plan, successful `rg`, or zero-test command is not completion.
2. Identify every affected case and its old owner; keep disabled/unsupported cases visible in the coverage report. Read the current capture script before running it, especially implicit latest-run behavior, GPU requirements and write targets.
3. Capture candidates outside tracked baselines. For the current pilot use `build/visual/godot-pilot/<run-id>/`; a later production run root must be explicitly implemented and documented by P12.
4. Verify identity and completeness, establish mappings, run semantic tests and meaningful comparisons, and inspect every affected PNG/GIF. Retain mismatch and non-comparable results.
5. With an approved feature handoff and scoped explicit update authorization, transfer the registry owner and reviewed tracked files together. Reject missing/unreviewed cases, path escapes, unexpected write targets and partial publication; P12 must implement these failure guarantees before advertising an atomic updater.
6. Run the new producer's comparison-only gate, the still-owned legacy coverage, and affected routing/ownership checks. Record rollback and retain the previous release. Do not remove the old tool globally while it owns other cases. After partial handoff, run checks through the implemented per-case owner dispatch; never point a whole legacy suite at new-owner baselines. A full old-producer run uses the retained old release and its own baseline set. If the current adapter cannot select the remaining legacy cases, implement and qualify that scoped selection under P12 before accepting handoff.

## Current limitations to inspect

The current pilot semantic mapping source is `testdata/godot-pilot/visual-semantics.json`; check its actual contents. An empty `mappings` array yields no demonstrated baseline parity. `scripts/godot/visual-compare.sh` and `packages/tools/perfcheck` classify pilot differences; inspect the report rather than interpreting command success as complete coverage. Do not assume pilot capture supports arbitrary UI/motion features or desktop exports.

The existing world updater can generate motion GIFs even for a PNG subset. A proposed production updater needs staged candidates and a reviewed write manifest, but that future behavior must not be attributed to today's command. Keep unsupported environment/capture capabilities as explicit blockers to handoff, without blocking unrelated semantic tests.
