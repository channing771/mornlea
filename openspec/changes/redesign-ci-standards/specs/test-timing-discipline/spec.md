## REMOVED Requirements

### Requirement: CI workflow is unique per commit

**Reason**: CI run identity is broader than wall-clock test discipline and is superseded by the `continuous-integration` capability's candidate identity and cancellation contract.

**Migration**: Preserve one pull-request candidate workflow and stale-run cancellation under `continuous-integration`; no test timing behavior changes.

### Requirement: Rust build is single-source and SHA-bound

**Reason**: The macOS-only artifact topology cannot express the required Linux and macOS platform-owned builds and is superseded by the `continuous-integration` native-artifact contract.

**Migration**: Produce separate SHA-bound Linux and macOS artifacts, verify each platform manifest before consumption, and retain the single shared target-directory contract.

### Requirement: Race coverage remains complete and partitioned

**Reason**: Race inventory and platform routing are CI topology contracts and are superseded by the complete source-set and race-coverage requirement in `continuous-integration`.

**Migration**: Preserve six-module full race coverage, disjoint slice inventory, and the independent non-race server probe while routing slices to their owning platforms.

### Requirement: Final required check is fail-closed

**Reason**: The old requirement omits the required Linux bundle from its normative prerequisite list and is superseded by the stable merge-gate requirement in `continuous-integration`.

**Migration**: Protect the new stable merge-gate status, include every required Linux and macOS prerequisite, and keep Godot outside the gate until an approved cutover.

### Requirement: Failed-job rerun is isolated

**Reason**: Rerun behavior belongs with CI failure semantics and is superseded by the failure-isolation requirement in `continuous-integration`.

**Migration**: Retain failed-job rerun isolation without adding automatic command retries or whole-workflow retry logic.
