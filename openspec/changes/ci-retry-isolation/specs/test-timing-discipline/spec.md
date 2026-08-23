## ADDED Requirements

### Requirement: CI workflow is unique per commit

CI SHALL run `pull_request` for pull requests and `push` only for `main`; a pull-request branch push MUST NOT create a second workflow for the same SHA. Concurrency MUST be keyed by PR number or ref and MUST cancel an unfinished older SHA in the same PR/ref group.

#### Scenario: PR SHA is not duplicated

- **GIVEN** a commit is pushed to a branch with an open pull request
- **WHEN** GitHub emits the branch push and pull-request synchronization events
- **THEN** exactly one pull-request workflow is eligible to run for that SHA

#### Scenario: New PR SHA cancels stale work

- **GIVEN** an older SHA is still running for a pull request
- **WHEN** a newer SHA arrives for the same pull request
- **THEN** the older unfinished run is cancelled and the newer SHA owns the active run

### Requirement: Rust build is single-source and SHA-bound

The macOS workflow MUST run `make rust` exactly once in `native-macos`. Every macOS downstream job MUST consume the artifact from that job and MUST verify its exact `${{ github.sha }}` binding, manifest path, size and SHA-256 before running Go commands. Missing, mismatched or corrupted artifacts MUST fail closed.

#### Scenario: Artifact from another SHA is rejected

- **GIVEN** a downstream job downloads an artifact whose manifest SHA differs from the current `${{ github.sha }}`
- **WHEN** artifact validation runs
- **THEN** the job fails before any Go test and does not substitute another run's artifact

#### Scenario: Rust is not rebuilt downstream

- **GIVEN** `native-macos` has produced and validated the artifact
- **WHEN** `quality`, any `go-race` slice or `integration` runs
- **THEN** each job uses the downloaded artifact and does not execute another `make rust`

### Requirement: Race coverage remains complete and partitioned

The `go-race` matrix MUST contain exactly `cmd`, `internal/server` and the remaining `internal` packages. Their package union MUST equal, and their pairwise intersections MUST be empty relative to, the package set selected by the existing `go test ./... -race -p=1 -skip '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$'` command. The 50ms probe MUST remain a separate non-race integration step with `-count=1`.

#### Scenario: No package is lost or duplicated

- **GIVEN** the package lists for all three race slices are generated
- **WHEN** they are compared with the old command's package set
- **THEN** the union is byte-for-byte the same set and no package occurs in two slices

#### Scenario: Probe remains independent

- **GIVEN** the race matrix is executing
- **WHEN** the 50ms probe is run
- **THEN** it runs in `integration` outside race with its original test name and `-count=1`

### Requirement: Final required check is fail-closed

The required `test` job MUST succeed only when `native-macos`, `quality`, all three race slices and `integration` succeed. A failure, cancellation, skipped prerequisite or artifact validation error MUST make `test` fail; no job MAY use `continue-on-error` or allow-failure to turn a red gate green. `linux-server` MUST remain an independent unchanged job, and performance output MUST remain informational only.

#### Scenario: A failed slice fails the required check

- **GIVEN** one race slice or integration job fails
- **WHEN** the final `test` summary runs
- **THEN** required check `test` fails even if every other macOS job succeeds

#### Scenario: Skipped prerequisite fails closed

- **GIVEN** a required upstream job is skipped or cancelled
- **WHEN** the final summary evaluates job results
- **THEN** `test` fails rather than reporting success from partial coverage

### Requirement: Failed-job rerun is isolated

The workflow MUST support GitHub “rerun failed jobs” such that a failed race slice or integration job can be rerun without rerunning successful slices; the final `test` summary reruns with the failed jobs. The workflow MUST NOT automatically rerun the complete workflow or silently retry a failed command.

#### Scenario: One failed race slice is rerun alone

- **GIVEN** two race slices and all quality/integration jobs succeeded, but one race slice failed
- **WHEN** an operator selects “rerun failed jobs”
- **THEN** only that failed slice and the required final summary rerun; successful slices remain complete
