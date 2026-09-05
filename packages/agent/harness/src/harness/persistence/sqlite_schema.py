"""SQLite schema 权威定义：DDL、列形状与版本常量。"""

from __future__ import annotations

import re

_SCHEMA_VERSION = 1
_EXPECTED_COLUMNS = {
    "agent_schema": {"singleton", "schema_version"},
    "namespace_lease_history": {"lease_id"},
    "namespace_leases": {
        "namespace_id",
        "client_instance_id",
        "lease_id",
        "expires_at_unix_ms",
    },
    "companion_memory": {
        "namespace_id",
        "companion_id",
        "memory_epoch",
        "active",
        "revision",
        "operation_id",
        "summary",
        "tombstone_operation_id",
        "tombstone_old_epoch",
    },
    "memory_operations": {
        "namespace_id",
        "companion_id",
        "operation_id",
        "operation_kind",
        "commit_lease_id",
        "payload_fingerprint",
        "state_fingerprint",
        "result_epoch",
        "result_revision",
    },
}

_SCHEMA_DDL = {
    "agent_schema": """
CREATE TABLE agent_schema (
    singleton INTEGER PRIMARY KEY NOT NULL CHECK (singleton = 1),
    schema_version INTEGER NOT NULL CHECK (schema_version = 1)
) STRICT, WITHOUT ROWID
""".strip(),
    "namespace_lease_history": """
CREATE TABLE namespace_lease_history (
    lease_id TEXT PRIMARY KEY NOT NULL
) STRICT, WITHOUT ROWID
""".strip(),
    "namespace_leases": """
CREATE TABLE namespace_leases (
    namespace_id TEXT PRIMARY KEY NOT NULL,
    client_instance_id TEXT NOT NULL,
    lease_id TEXT NOT NULL UNIQUE,
    expires_at_unix_ms INTEGER NOT NULL CHECK (expires_at_unix_ms >= 0),
    FOREIGN KEY (lease_id) REFERENCES namespace_lease_history (lease_id)
) STRICT, WITHOUT ROWID
""".strip(),
    "companion_memory": """
CREATE TABLE companion_memory (
    namespace_id TEXT NOT NULL,
    companion_id TEXT NOT NULL,
    memory_epoch BLOB NOT NULL
        CHECK (typeof(memory_epoch) = 'blob' AND length(memory_epoch) = 8),
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    revision BLOB
        CHECK (revision IS NULL OR (typeof(revision) = 'blob' AND length(revision) = 8)),
    operation_id TEXT,
    summary BLOB
        CHECK (summary IS NULL OR (typeof(summary) = 'blob' AND length(summary) <= 2048)),
    tombstone_operation_id TEXT,
    tombstone_old_epoch BLOB
        CHECK (
            tombstone_old_epoch IS NULL
            OR (typeof(tombstone_old_epoch) = 'blob' AND length(tombstone_old_epoch) = 8)
        ),
    PRIMARY KEY (namespace_id, companion_id),
    CHECK (
        (
            active = 1
            AND revision IS NOT NULL
            AND summary IS NOT NULL
            AND tombstone_operation_id IS NULL
            AND tombstone_old_epoch IS NULL
            AND (
                (revision = X'0000000000000000' AND operation_id IS NULL AND length(summary) = 0)
                OR (revision != X'0000000000000000' AND operation_id IS NOT NULL)
            )
        )
        OR (
            active = 0
            AND revision IS NULL
            AND operation_id IS NULL
            AND summary IS NULL
            AND tombstone_operation_id IS NOT NULL
        )
    )
) STRICT, WITHOUT ROWID
""".strip(),
    "memory_operations": """
CREATE TABLE memory_operations (
    namespace_id TEXT NOT NULL,
    companion_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    operation_kind TEXT NOT NULL
        CHECK (operation_kind IN ('commit', 'active_mirror', 'tombstone')),
    commit_lease_id TEXT
        CHECK (commit_lease_id IS NULL OR length(commit_lease_id) = 36),
    payload_fingerprint BLOB NOT NULL
        CHECK (typeof(payload_fingerprint) = 'blob' AND length(payload_fingerprint) = 32),
    state_fingerprint BLOB NOT NULL
        CHECK (typeof(state_fingerprint) = 'blob' AND length(state_fingerprint) = 32),
    result_epoch BLOB NOT NULL
        CHECK (typeof(result_epoch) = 'blob' AND length(result_epoch) = 8),
    result_revision BLOB
        CHECK (
            result_revision IS NULL
            OR (typeof(result_revision) = 'blob' AND length(result_revision) = 8)
        ),
    PRIMARY KEY (namespace_id, companion_id, operation_id),
    FOREIGN KEY (commit_lease_id) REFERENCES namespace_lease_history (lease_id),
    CHECK (
        (
            operation_kind = 'commit'
            AND commit_lease_id IS NOT NULL
            AND result_revision IS NOT NULL
        )
        OR (
            operation_kind = 'active_mirror'
            AND commit_lease_id IS NULL
            AND result_revision IS NOT NULL
        )
        OR (
            operation_kind = 'tombstone'
            AND commit_lease_id IS NULL
            AND result_revision IS NULL
        )
    )
) STRICT, WITHOUT ROWID
""".strip(),
}


def _normalize_sql(value: str) -> str:
    return re.sub(r"\s+", " ", value.strip())
