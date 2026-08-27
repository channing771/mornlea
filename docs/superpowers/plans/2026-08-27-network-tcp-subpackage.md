# Network TCP Subpackage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 TCP transport 从 `internal/network` 提取到单向依赖的 `internal/network/tcp` 子包，同时保持协议、登录、Memory/TCP 语义和测试入口不变。

**Architecture:** `internal/network` 保留 packet、codec、登录、共享 stream 接口、endpoint 包装和 Memory；`internal/network/tcp` 依赖根包并拥有 TCP socket、listener、stream 与 deadline 生命周期。根包不依赖 TCP 子包，所有仓库内 TCP 构造调用迁移到子包。

**Tech Stack:** Go 1.26、标准库 `net`/`context`/`sync`/`io`、现有 `internal/network` codec、Go race/vet、OpenSpec。

**Spec:** `openspec/changes/network-tcp-subpackage/proposal.md`、`openspec/changes/network-tcp-subpackage/specs/repository-code-organization/spec.md`、`openspec/changes/network-tcp-subpackage/design.md`

## Global Constraints

- `internal/network/tcp` MUST depend on `internal/network`; `internal/network` MUST NOT depend on `internal/network/tcp`。
- `internal/network` MUST retain `ClientPacketStream`、`ServerPacketStream`、`Listener`、`ClientEndpoint`、`ServerEndpoint` and `ErrClosed`。
- MUST NOT change packet bytes, protocol v27, login state transitions, timeout values, error semantics, capacity behavior, or concurrency ownership。
- MUST preserve every existing Test、Benchmark、Fuzz function name and `t.Run` label；TCP white-box tests may change package path but not names。
- MUST NOT modify `internal/sim/door_test.go` or any pre-existing unrelated worktree change。
- Go comments and documentation added in production code MUST be Chinese；task identifiers may appear only in planning artifacts。
- Every task ends with its scoped `gofmt`/test command before it is marked complete；final task also runs `go vet ./...`、`go test ./... -race` and strict OpenSpec validation。

---

## File Map

**Create or move:**

- `internal/network/tcp/tcp.go`: `ListenTCP`、`DialTCP`、TCP listener、socket configuration and TCP error classification。
- `internal/network/tcp/stream.go`: TCP stream、read/write owner gate、frame receive/send、deadline and close lifecycle。
- `internal/network/tcp/contract_test.go`: compile-time/runtime assertions that child constructors return root interfaces。
- `internal/network/tcp/tcp_test.go`: moved TCP white-box tests from `internal/network/tcp_test.go`。

**Modify:**

- `internal/network/stream.go`: retain only shared packet stream and listener interfaces。
- `internal/network/transport_consistency_test.go`: import `network/tcp` for TCP opener。
- `internal/network/benchmark_test.go`: import `network/tcp` for TCP benchmark pair creation。
- TCP constructor call sites in `cmd/mornlea`、`cmd/mornlea-server`、`internal/client` and `internal/server`。
- `internal/archcheck/dependency_test.go`: register the child-to-root dependency。
- `docs/architecture.md` and `internal/network/AGENTS.md`: document the new ownership boundary。

**Do not modify:** packet/message/codec/frame implementation, login implementation, Memory implementation, Rust code, protocol constants, storage files, or unrelated user changes。

## Task 1: Extract TCP Implementation

**Files:**
- Create: `internal/network/tcp/contract_test.go`
- Create: `internal/network/tcp/tcp.go`
- Create: `internal/network/tcp/stream.go`
- Move: `internal/network/tcp_test.go` → `internal/network/tcp/tcp_test.go`
- Modify: `internal/network/stream.go`
- Modify: `internal/network/transport_consistency_test.go`
- Modify: `internal/network/benchmark_test.go`

**Interfaces:**
- Consumes: root `network.ClientPacketStream`, `network.ServerPacketStream`, `network.Listener`, `network.Codec`, `network.ReadFrame`, `network.WriteFrame`, packet types and `network.ErrClosed`。
- Produces: `tcp.ListenTCP(address string) (network.Listener, error)` and `tcp.DialTCP(ctx context.Context, address string) (network.ClientPacketStream, error)`。

- [ ] **Step 1: Freeze the transport test and caller inventory**

Run:

```bash
go test ./internal/network -list '.*' | sort > /tmp/network-tcp-before.txt
git grep -nE 'network\.(ListenTCP|DialTCP)' -- '*.go' > /tmp/network-tcp-callers-before.txt
```

Expected: both snapshots are created; the caller snapshot is non-empty and is used only as the migration checklist。

- [ ] **Step 2: Write the boundary contract test first**

Create `internal/network/tcp/contract_test.go` with the following compile contract before adding production implementation:

```go
package tcp_test

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/internal/network"
	networktcp "github.com/channing771/mornlea/internal/network/tcp"
)

func TestTCPConstructorsExposeNetworkInterfaces(t *testing.T) {
	var listen func(string) (network.Listener, error) = networktcp.ListenTCP
	var dial func(context.Context, string) (network.ClientPacketStream, error) = networktcp.DialTCP
	if listen == nil || dial == nil {
		t.Fatal("TCP constructors are nil")
	}
}
```

Run `go test ./internal/network/tcp -run '^TestTCPConstructorsExposeNetworkInterfaces$'`. Expected: FAIL because the new child package implementation does not exist yet。

- [ ] **Step 3: Split the production implementation at the package boundary**

Move these TCP-only declarations from the root package into the new `tcp` package: `tcpSocket`、`tcpListener`、`DialTCP`、`ListenTCP`、`configureTCPSocket`、`isTCPClosedError`、`isTCPStreamClosedError`、`streamConn`、`ownerGate`、`tcpStream`、`tcpClientStream`、`tcpServerStream`、`receivePacket`、`installReadContext` and `installWriteContext`。Prefix root references with `network.` and keep `ClientPacketStream`、`ServerPacketStream` and `Listener` declarations in root `stream.go`。

Run:

```bash
gofmt -w internal/network/stream.go internal/network/tcp
go test ./internal/network/tcp -run '^TestTCPConstructorsExposeNetworkInterfaces$'
```

Expected: the contract test passes; root package tests may still fail until the TCP test and root openers are migrated in the next step。

- [ ] **Step 4: Move TCP tests and update root transport tests**

Move `internal/network/tcp_test.go` to `internal/network/tcp/tcp_test.go`, change its package declaration to `package tcp`, import `internal/network`, and qualify root packet/codec/frame/interface symbols with `network.` while leaving child private symbols unqualified. Update only the TCP constructor references in `transport_consistency_test.go` and `benchmark_test.go` to use the `networktcp` import; leave Memory constructors and all test names unchanged。

Run `gofmt -w internal/network/tcp internal/network/transport_consistency_test.go internal/network/benchmark_test.go && go test ./internal/network/... -race -count=1`. Expected: root and child network packages pass。

- [ ] **Step 5: Commit the isolated extraction**

```bash
git add internal/network
git commit -m "refactor(network): extract TCP transport package"
```

## Task 2: Migrate Repository Callers

**Files:**
- Modify: `cmd/mornlea/app_startup.go`
- Modify: `cmd/mornlea/app_dependencies.go`
- Modify: `cmd/mornlea-server/main.go`
- Modify: `internal/client/inventory_mirror_test.go`
- Modify: `internal/server/multiplayer_restart_test.go`
- Modify: `internal/server/multiplayer_tcp_capacity_test.go`
- Modify: `cmd/mornlea/app_celestial_test.go`
- Modify: `cmd/mornlea/app_lod_test.go`
- Modify: `cmd/mornlea/app_render_test.go`
- Modify: every additional Go file listed by the Task 1 caller snapshot if it directly calls `network.ListenTCP` or `network.DialTCP`

**Interfaces:**
- Consumes: `network.Listener` and `network.ClientPacketStream` shared interfaces from Task 1。
- Produces: application and test code importing `networktcp "github.com/channing771/mornlea/internal/network/tcp"` and calling `networktcp.ListenTCP` / `networktcp.DialTCP`。

- [ ] **Step 1: Migrate production constructor references**

In `cmd/mornlea/app_startup.go`、`cmd/mornlea/app_dependencies.go` and `cmd/mornlea-server/main.go`, add the `networktcp` import and replace only `network.ListenTCP` / `network.DialTCP`; preserve dependency function signatures using root interfaces and preserve `network.NewMemoryPair` / `network.NewMemoryStreamPair`。

Run:

```bash
gofmt -w cmd/mornlea/app_startup.go cmd/mornlea/app_dependencies.go cmd/mornlea-server/main.go
go test ./cmd/mornlea ./cmd/mornlea-server -run '^$'
```

Expected: both packages compile with no test execution。

- [ ] **Step 2: Migrate internal and command test callers**

Apply the same import-only constructor migration to the listed client/server/cmd test files and every file found by:

```bash
git grep -nE 'network\.(ListenTCP|DialTCP)' -- '*.go'
```

Do not change assertions, transport setup, login calls, test names or test labels. The command must produce no output after the edit。

- [ ] **Step 3: Run affected package tests**

```bash
gofmt -w cmd/mornlea cmd/mornlea-server internal/client internal/server
go test ./cmd/mornlea ./cmd/mornlea-server ./internal/client ./internal/server -race -count=1
```

Expected: all four packages pass；if a test fails, inspect only the import-path migration and do not alter runtime behavior。

- [ ] **Step 4: Commit caller migration**

```bash
git add cmd/mornlea cmd/mornlea-server internal/client internal/server
git commit -m "refactor(network): migrate TCP callers"
```

## Task 3: Update Architecture Guard and Documentation

**Files:**
- Modify: `internal/archcheck/dependency_test.go`
- Modify: `docs/architecture.md`
- Modify: `internal/network/AGENTS.md`
- Modify: `openspec/changes/network-tcp-subpackage/ledger.md`

**Interfaces:**
- Consumes: the child package import graph and the `repository-code-organization` delta spec。
- Produces: an explicit archcheck entry for `internal/network/tcp` allowing only `internal/network`, plus current ownership documentation。

- [ ] **Step 1: Add the dependency whitelist entry**

Add exactly one entry to `allowed` in `internal/archcheck/dependency_test.go`:

```go
"internal/network/tcp": {"internal/network"},
```

Do not add `internal/network/tcp` to any other package's allowed dependencies and do not alter existing entries. Run `go test ./internal/archcheck -count=1`; Expected: PASS。

- [ ] **Step 2: Update current architecture documentation**

In `docs/architecture.md` section 4, state that `internal/network` owns packet、codec、login、shared interfaces and Memory, while `internal/network/tcp` owns TCP listener/dial/stream implementation and depends only on `internal/network`。In the same section, retain the existing no-WebGPU and ABI boundaries. In `internal/network/AGENTS.md`, update the transport ownership sentence to name both packages and state that TCP remains a transport-only implementation。

Run `git diff --check`; Expected: PASS with documentation changes limited to the new package boundary。

- [ ] **Step 3: Record the task decision and verification**

Append the implementation commit IDs, review outcomes and verification commands to `openspec/changes/network-tcp-subpackage/ledger.md`; record any failed command with its root cause and rerun result. Do not record unrelated changes as part of this change。

- [ ] **Step 4: Commit architecture documentation**

```bash
git add internal/archcheck/dependency_test.go docs/architecture.md internal/network/AGENTS.md openspec/changes/network-tcp-subpackage/ledger.md
git commit -m "docs(network): record TCP package boundary"
```

## Task 4: Final Verification and Handoff

**Files:**
- Verify: all files in the change diff
- Modify: `openspec/changes/network-tcp-subpackage/tasks.md` and `openspec/changes/network-tcp-subpackage/ledger.md` only to record completed work and evidence

**Interfaces:**
- Consumes: completed Tasks 1–3, the approved design, and all OpenSpec artifacts。
- Produces: a clean, validated change ready for final review and implementation handoff。

- [ ] **Step 1: Compare test entry names before and after migration**

Use the Task 1 snapshot and current package lists:

```bash
go test ./internal/network -list '.*' | sort > /tmp/network-root-after.txt
go test ./internal/network/tcp -list '.*' | sort > /tmp/network-tcp-after.txt
git grep -nE 'network\.(ListenTCP|DialTCP)' -- '*.go'
```

Expected: the grep command produces no output; every name previously declared by `internal/network/tcp_test.go` is present in `/tmp/network-tcp-after.txt`, and root packet/codec/login/Memory entries remain present。

- [ ] **Step 2: Inspect the final diff boundary**

```bash
git status --short --branch
git diff --check
git diff --name-only 123c51f1..HEAD
```

Expected: only the planned network package, caller, archcheck, documentation and OpenSpec files appear; `internal/sim/door_test.go` and other unrelated worktree changes do not appear。

- [ ] **Step 3: Run all required gates**

```bash
make rust
gofmt -l .
go vet ./...
go test ./... -race -count=1
make rust-check
openspec validate --all --strict --no-interactive
```

Expected: `gofmt -l .` prints nothing and every command exits 0. Record the complete command results and any benchmark values in `ledger.md`; do not change thresholds or golden files。

- [ ] **Step 4: Complete OpenSpec and handoff**

Mark the matching checkboxes in `openspec/changes/network-tcp-subpackage/tasks.md` only after their commands pass. Add the final review conclusion and the exact final HEAD to `ledger.md`, then report the plan and change paths. Do not archive or merge until the user chooses the execution path and final review is complete。
