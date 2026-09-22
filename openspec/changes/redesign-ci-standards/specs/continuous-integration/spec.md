## Purpose

Defines deterministic, platform-aware continuous integration contracts that give pull requests early actionable feedback while preserving complete fail-closed merge validation and explicitly separated migration-only evidence.

## ADDED Requirements

### Requirement: Each candidate tree has one active CI identity

Required CI SHALL run for pull requests and for pushes to `main`, and a pull-request branch push MUST NOT create a second required workflow for the same candidate tree. The validated identity MUST be the exact GitHub candidate SHA checked out by the workflow. A newer candidate for the same pull request or ref MUST cancel unfinished work for the older candidate.

#### Scenario: A synchronized pull request has one active candidate

- **GIVEN** a pull-request branch receives a new commit while an older required run is unfinished
- **WHEN** GitHub emits the synchronization event
- **THEN** exactly one required workflow SHALL validate the new candidate SHA
- **AND** unfinished work for the older candidate MUST be cancelled

#### Scenario: Main validates the landed tree

- **GIVEN** a pull request lands on `main`
- **WHEN** the `main` push workflow runs
- **THEN** every required artifact and summary MUST be bound to the landed commit SHA

### Requirement: Platform-neutral preflight is an independent first stage

The required workflow MUST start repository policy, OpenSpec, formatting, workflow-shape, script-policy, and dependency-contract checks without waiting for any native build artifact. Preflight MUST NOT import a platform-specific product artifact merely to validate platform-neutral policy. A preflight failure MUST provide its failing command or contract before native-dependent jobs finish.

#### Scenario: A malformed OpenSpec change fails before native completion

- **GIVEN** a pull request contains an invalid OpenSpec artifact and valid compilable source
- **WHEN** required CI starts
- **THEN** preflight MUST fail without waiting for a Linux or macOS native artifact

#### Scenario: Native build failure does not hide preflight evidence

- **GIVEN** both preflight and a native build fail on the same candidate
- **WHEN** the workflow completes
- **THEN** the preflight result and its actionable failure MUST remain independently visible

### Requirement: CI environments and tool dependencies are explicit and fail closed

Every CI job MUST use an explicitly selected runner image rather than a moving `latest` label, an explicit timeout, least-privilege token permissions, and pinned project toolchains. Reusable CI entry points and validation scripts MUST check required executable dependencies before evaluating their contracts. A missing dependency MUST fail before validation begins and MUST NOT permit a success message or partial pass. External actions MUST use an immutable reviewed revision compatible with the runner's supported Node runtime.

#### Scenario: A validator dependency is unavailable

- **GIVEN** a validation entry point requires an executable that is absent from `PATH`
- **WHEN** the entry point starts
- **THEN** it MUST fail with the missing executable name before reporting any validation success

#### Scenario: A runner image alias would move implicitly

- **GIVEN** a workflow job declares its operating-system runner
- **WHEN** the workflow definition is audited
- **THEN** the runner label MUST identify an approved image generation and MUST NOT use `ubuntu-latest`, `macos-latest`, or an equivalent moving alias

### Requirement: Native artifacts are platform-specific and candidate-bound

Linux and macOS native artifacts MUST be built as separate platform-owned units from the same candidate SHA. Each consumer MUST verify the artifact's candidate SHA, platform, manifest paths, sizes, and content digests before running dependent commands. Cargo output discovery MUST use one explicit target-directory contract shared by local and CI entry points; a producer and consumer MUST NOT infer different target roots. A cache hit MUST NOT substitute for artifact identity validation.

#### Scenario: Artifact from another candidate is rejected

- **GIVEN** a consumer downloads an artifact whose manifest SHA differs from the checked-out candidate SHA
- **WHEN** artifact validation runs
- **THEN** the consumer MUST fail before executing native-dependent tests

#### Scenario: Cargo target directory is overridden

- **GIVEN** a caller supplies an explicit Cargo target directory
- **WHEN** a native producer builds and a packaging consumer locates its output
- **THEN** both MUST resolve the same target-relative artifact path
- **AND** the consumer MUST NOT fall back to a repository-hardcoded target root

### Requirement: Required source sets and race coverage remain complete

Required CI MUST compile the repository on Linux and MUST validate the supported macOS native source set. Full Go race coverage MUST include every package from all six modules in committed `go.work`; the race slices MUST have a union equal to the workspace-wide package set and pairwise-empty intersections. Server and platform-neutral slices SHALL use Linux capacity, while a slice MAY use macOS only when its selected source set or runtime contract requires macOS. The independent server probe MUST remain outside race with its existing exact test identity and `-count=1`.

#### Scenario: Darwin-only API leaks into a Linux command

- **GIVEN** a Linux-buildable command references an API available only in Darwin-tagged files
- **WHEN** required Linux source-set compilation runs
- **THEN** required CI MUST fail with the compile error before the merge gate can succeed

#### Scenario: Race slice inventory loses a package

- **GIVEN** the committed workspace package set contains a package absent from every configured race slice
- **WHEN** the race inventory check runs
- **THEN** the candidate MUST fail before the merge gate can succeed

### Requirement: One stable merge gate summarizes every required result

The repository MUST expose one stable required merge-gate status. It MUST succeed only when preflight, Rust quality, both native platform builds and artifact checks, Linux source-set quality, frontend validation, every race slice, required integration checks, and the Linux server bundle/load contract succeed for the same candidate SHA. Failure, cancellation, skipped execution, timeout, or artifact validation failure in any required prerequisite MUST make the merge gate fail. Required commands MUST NOT use allow-failure or `continue-on-error` semantics.

#### Scenario: Linux bundle fails while other jobs pass

- **GIVEN** every required job except the Linux server bundle/load contract succeeds
- **WHEN** the merge gate evaluates prerequisite results
- **THEN** the merge gate MUST fail

#### Scenario: A required job is skipped

- **GIVEN** a required prerequisite is skipped or cancelled
- **WHEN** the merge gate evaluates the candidate
- **THEN** it MUST fail rather than treating partial coverage as success

### Requirement: Godot migration CI remains an honest optional leaf

Until an independently approved cutover change promotes the Godot client, Godot project, embedded-Python, bridge, export, and lifecycle qualification MUST execute in a separate workflow outside the required merge gate. The workflow MUST run for relevant Godot, bridge, Python-runtime, deterministic-asset, and gate-definition changes, and MUST support explicit manual execution. Unrelated changes MUST NOT download or build the Godot toolchain. A Godot failure MUST remain red and diagnosable; optional status MUST NOT be implemented by swallowing command failures.

#### Scenario: Unrelated server-only change avoids Godot setup

- **GIVEN** a pull request changes only authoritative server code and no Godot input, bridge contract, generated-asset input, or gate definition
- **WHEN** CI routing is evaluated
- **THEN** the Godot workflow MUST NOT download Godot, export templates, or the embedded Python runtime

#### Scenario: Relevant Godot change fails lifecycle qualification

- **GIVEN** a Godot-relevant pull request violates lifecycle qualification
- **WHEN** the optional Godot workflow runs
- **THEN** the Godot check MUST report failure with the original command result
- **AND** the required merge gate MUST continue to reflect only approved required contracts

### Requirement: CI commands are locally reproducible and caches are non-authoritative

Each workflow stage MUST invoke a repository-owned entry point that can run from a clean local checkout with the same semantic inputs. Workflow YAML MUST NOT contain a second implementation of package selection, artifact identity, or validation rules. Caches MAY reduce download or compilation time but MUST NOT change the selected source set, skip validation, supply publishable artifacts, or be required for a cold run to pass.

#### Scenario: Cold cache execution

- **GIVEN** no dependency or build cache is available
- **WHEN** a required CI entry point runs with its declared toolchains and inputs
- **THEN** it MUST execute the same checks and produce the same pass/fail conclusion as a warm-cache run

#### Scenario: Local reproduction

- **GIVEN** a required workflow stage fails
- **WHEN** a developer invokes the reported repository-owned entry point on the same candidate and supported platform
- **THEN** the entry point MUST exercise the same contract without requiring workflow-only inline logic

### Requirement: Failures are isolated without silent retries

The workflow MUST support GitHub failed-job reruns so that failed independent jobs and the merge gate can rerun without rerunning successful independent jobs. Required commands MUST NOT retry automatically, convert a failed attempt into success, or rerun the complete workflow silently. Stage durations and runner identity MUST be recorded for diagnosis, while duration values and benchmark values remain informational and MUST NOT independently change command exit status.

#### Scenario: One race slice fails

- **GIVEN** all independent jobs succeed except one race slice
- **WHEN** an operator selects failed-job rerun
- **THEN** the failed slice and dependent merge gate SHALL rerun
- **AND** already successful independent slices SHALL remain completed

#### Scenario: A performance measurement regresses without structural failure

- **GIVEN** a complete performance report has valid identity, samples, and bounded outputs but slower numeric results
- **WHEN** CI records the report
- **THEN** the numeric regression MUST remain informational
- **AND** missing identity, incomplete samples, overflow, data loss, or I/O errors MUST still fail
