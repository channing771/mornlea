# Persistence White-Box Test Scope

## Moved White-Box Tests

- `internal/server/persistence_backpressure_test.go`
- `internal/server/persistence_helpers_test.go`
- `internal/server/persistence_retry_test.go`
- `internal/server/persistence_schedule_test.go`
- `internal/server/player_persistence_cache_test.go`
- `internal/server/player_persistence_concurrency_test.go`
- `internal/server/player_persistence_helpers_test.go`
- `internal/server/player_persistence_lifecycle_test.go`
- `internal/server/player_persistence_retry_test.go`
- `internal/server/player_save_scheduler_test.go`
- `internal/server/player_flush_test.go`
- `internal/server/player_flush_barrier_test.go`
- `internal/server/player_flush_stall_test.go`
- `internal/server/hunger_persistence_test.go`
- `internal/server/respawn_persistence_test.go`
- `internal/server/companion_persistence_test.go`
- `internal/server/hostile_persistence_test.go`

The three `player_flush*_test.go` files are in scope even though they do not
currently contribute a `t.Run` label.

## Retained Root Integration Tests

- `internal/server/persistence_integration_test.go`: external `server_test`
  coverage retained at the root API boundary.
- `internal/server/metadata_persistence_test.go`: root coordinator integration
  coverage retained because its three tests exercise `Server.Shutdown`,
  `shutdownTestStore`, sync/close ordering, context retry, and Store ownership,
  rather than child metadata schedule/retry/status details.
- `internal/server/companion_bootstrap_test.go`: Host assembly coverage retained
  at the root API boundary.
- `internal/server/hostile_restore_test.go`: Host startup and restore coverage
  retained at the root API boundary.
- `internal/server/hostile_restart_test.go`: Host lifetime/restart coverage
  retained at the root API boundary.

The retained root integration tests are intentionally excluded from the raw and
normalized label captures.
