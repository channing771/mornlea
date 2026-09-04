## MODIFIED Requirements

### Requirement: Race coverage remains complete and partitioned

The `go-race` matrix MUST partition the package union of every Go module listed in the committed `go.work`（root module 与 `packages/` 下全部 Go 模块）into slices whose union MUST equal, and whose pairwise intersections MUST be empty relative to, the package set enumerated by `go list ./...` within each workspace module. Slices MAY be organized by unit module or by legacy `cmd`/`internal/server`/rest groupings during the unit migration; a self-check step MUST compare the slice union against the workspace-wide enumeration on every run. The 50ms probe MUST remain a separate non-race integration step with `-count=1`.

#### Scenario: No package is lost or duplicated

- **GIVEN** the package lists for all race slices are generated
- **WHEN** they are compared with the workspace-wide package enumeration across all `go.work` modules
- **THEN** the union is byte-for-byte the same set and no package occurs in two slices

#### Scenario: Probe remains independent

- **GIVEN** the race matrix is executing
- **WHEN** the 50ms probe is run
- **THEN** it runs in `integration` outside race with its original test name and `-count=1`
