# Task 7A implementation report

## Scope

- Starting baseline: `148b935c`
- Final integration baseline after the independent Task 6 commit: `0ea46c31`
- Worktree: `/Users/chen/work/mornlea/.worktrees/extract-companion-agent-service`
- Scope is limited to the MCP v1 domain-result contract prerequisite described by the Task 7A brief.
- Concurrent Task 6 changes in `internal/config/agent_service_test.go` and `internal/companion/agent_client_round3_test.go` are not part of this task and will not be staged.

## RED

### Go contract fixtures

Command:

```text
go test ./internal/companion -run 'ContractFixture' -count=1
```

Result: failed as intended. The failures proved that the manifest did not yet contain `domain_result_codes`, the public find/query result schemas still required only their success fields, and `validator_hint` still accepted empty, control, NUL, edge-whitespace, and lone-surrogate inputs.

### Python contract, adapter, and Planner

Command:

```text
cd services/companion-agent && uv run pytest tests/test_contracts.py tests/test_mcp_adapter.py tests/test_planner.py -q
```

Result: failed during collection as intended because the closed failure result types (`FindVisibleBlocksFailureResult` and peers) did not yet exist.

## GREEN

### Go contract fixtures

Command:

```text
go test ./internal/companion -run 'ContractFixture' -count=1
```

Result: PASS (`ok github.com/channing771/mornlea/internal/companion`).

### Python focused contract, adapter, and Planner tests

Command:

```text
cd services/companion-agent && uv run pytest tests/test_contracts.py tests/test_mcp_adapter.py tests/test_planner.py -q
```

Result: PASS (`118 passed in 16.50s`).

### Python formatting and static analysis

Command:

```text
cd services/companion-agent && uv run ruff format --check . && uv run ruff check . && uv run mypy src
```

Result: PASS (`36 files already formatted`, `All checks passed`, and no mypy issues in 23 source files).

### Full Python regression

Command:

```text
cd services/companion-agent && uv run pytest -q
```

Result: PASS (`401 passed in 18.07s`).

### Diff hygiene

Command:

```text
git diff --check
```

Result: PASS.

## Fixture counts

- MCP valid golden cases: 20
- MCP invalid golden cases: 44
- MCP mine-classification cases: 8
- Total MCP golden cases: 72

## Concerns

None. Recoverable `unknown_block` and `out_of_bounds` results now remain `isError=false` normal tool results; protocol, schema, duplicate-content, and `isError=true` failures remain unavailable.
