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

## Quality repair round 1

### RED

Command:

```text
cd services/companion-agent && uv run pytest tests/test_mcp_adapter.py::test_tool_discovery_requires_exact_unique_six_without_pagination -q
```

Result: failed as intended for only the `output_schema` parameter (`1 failed, 7 passed`). The new delivered-response sentinel stayed at zero, proving that the obsolete top-level `properties` lookup raised inside the transport mock and that `PlannerUnavailable` had previously hidden the mock failure.

Command:

```text
cd services/companion-agent && uv run pytest tests/test_contracts.py::test_validator_hint_uses_one_shared_runtime_definition -q
```

Result: failed as intended because `domain.common.ValidatorHint` and `domain.mcp_v1.ValidatorHint` were distinct public runtime definitions with different validation semantics.

### GREEN

- The query-result drift mutation now reaches `outputSchema.oneOf[0].properties.terrain.maxItems`, and the test asserts both the returned mutated payload and exactly one SDK `tools/list` request.
- `ValidatorHint` now has one definition in `domain.common`: strict string, 1..256 canonical UTF-8 bytes, without NUL, control characters, or edge whitespace. MCP v1 imports and registers that same definition.
- The remaining transport fault-injection tests were audited. Mutated domain failures, oversized responses, initialize capabilities/version drift, forbidden response modes, redirects, invalid text fallback, result-shape drift, and forced serialization failure now assert that their intended fault was constructed or invoked before accepting `PlannerUnavailable`.

Focused contract and adapter validation:

```text
go test ./internal/companion -run 'ContractFixture' -count=1
cd services/companion-agent && uv run pytest tests/test_contracts.py tests/test_mcp_adapter.py tests/test_planner.py -q
```

Result: PASS (`ok github.com/channing771/mornlea/internal/companion`; `119 passed in 16.40s`).

Formatting and static analysis:

```text
cd services/companion-agent && uv run ruff format --check . && uv run ruff check . && uv run mypy src
```

Result: PASS (`36 files already formatted`, `All checks passed`, and no mypy issues in 23 source files).

Full Python regression:

```text
cd services/companion-agent && uv run pytest -q
```

Result: PASS (`402 passed in 18.08s`).

Diff hygiene:

```text
git diff --check
```

Result: PASS.
