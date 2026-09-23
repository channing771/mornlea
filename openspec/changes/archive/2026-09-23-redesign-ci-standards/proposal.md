## Why

The current pull-request workflow delays inexpensive policy feedback behind a thirteen-minute macOS native build, concentrates unrelated checks on scarce macOS runners, and mixes required product gates with the optional Godot migration leaf. PR #184 also exposed three deterministic contract defects that local macOS validation did not catch: a Darwin-only asset export consumed by a Linux command, a project validator that can continue after `rg` is unavailable, and a Godot extension build that disagrees with the repository-wide Cargo target directory.

## What Changes

- Replace the single mixed workflow with a staged required pipeline that starts platform-neutral preflight immediately, builds SHA-bound Linux and macOS native artifacts in parallel, routes tests to the platform that owns their source set, and exposes one stable fail-closed merge gate.
- Move the Godot migration checks into a separate path-scoped optional workflow until an approved cutover promotes them, while retaining their real failures and full lifecycle qualification.
- Make local and CI entry points share explicit dependency checks, artifact identity rules, Cargo target-directory resolution, and platform-native package enumeration.
- Repair the three deterministic failures that constitute the first acceptance cases for the new standard: cross-platform atlas export, missing-tool fail-open validation, and inconsistent Godot extension output lookup.
- Reconcile canonical CI requirements, test guidance, workflow names, runner policy, timeout policy, and branch-protection expectations with the implemented topology.
- Preserve complete race coverage, exact-tree validation, hard failures for overflow, data loss, report identity, and I/O errors, and informational-only performance measurements.
- Do not add automatic retries, allow-failure semantics, or path-based skipping for required core gates.

## Capabilities

### New Capabilities

- `continuous-integration`: Defines required and optional workflow topology, platform ownership, artifact identity, fail-closed dependency handling, local parity, merge-gate behavior, and measurable feedback expectations.

### Modified Capabilities

- `test-timing-discipline`: Removes CI topology requirements that no longer belong in the test wall-clock discipline capability; those contracts move to `continuous-integration` with updated platform and merge-gate semantics.

## Impact

- Affected automation and build surfaces: `.github/workflows/`, `Makefile`, `scripts/ci/`, `scripts/godot/`, and CI-facing documentation.
- Affected source boundary: `packages/client/assets` and `packages/client/cmd/mornlea-godot-assets` will make CPU atlas export available on every supported build platform while retaining Darwin-only GPU upload ownership.
- Affected specifications: a new `continuous-integration` capability and the CI-specific requirements currently embedded in `test-timing-discipline`.
- User-observable outcome: pull requests receive actionable policy failures without waiting for native builds, required status is represented by one stable merge gate, and optional Godot failures are clearly separated from merge eligibility.
- Compatibility: no network protocol, save schema, engine ABI, client ABI, benchmark scenario, or visual-baseline change. No runtime concurrency semantics change; only CI job scheduling and artifact flow change. Product performance is unaffected, while CI latency is measured separately and caches remain non-authoritative accelerators.
