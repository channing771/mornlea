## ADDED Requirements

### Requirement: 寻路实现是独立内部包

仓库 MUST 将可复用的有界寻路值与算法作为独立内部包提供，并在提取时保持既有寻路
行为与测试入口不变。

#### Scenario: pathfind owns reusable pathfinding values and algorithms
- **GIVEN** the repository builds its internal packages
- **WHEN** callers construct a path grid or execute a path search
- **THEN** they MUST use `internal/pathfind` values and functions
- **AND** `internal/pathfind` MUST directly depend only on `internal/core`

#### Scenario: pathfinding extraction preserves existing behavior
- **GIVEN** the pre-extraction companion and server test entry inventory
- **WHEN** the package extraction is complete
- **THEN** existing Test, Benchmark, Fuzz names and `t.Run` labels MUST remain available
- **AND** path results, revision validation, errors and bounded resource behavior MUST remain unchanged
