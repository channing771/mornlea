"""Exercise the native bridge session contract through the scene-held instance."""

from __future__ import annotations

import json
from typing import Any, cast

from py4godot.classes import gdclass
from py4godot.classes.core import PackedByteArray
from py4godot.classes.Node import Node
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# The generated Py4Godot cast table only knows engine classes, while the node
# wrapper `get_node` builds for the native bridge already owns its pointer.
# Registering the identity cast for the project class lets Python hold the
# scene node and reach its methods through Godot's dynamic `call`, without any
# binding patch and without touching the class's native memory.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)


# The pinned pilot family table mirrored from the frozen client-core contract:
# (family, version, record_limit, record_bytes) in ascending family order.
PINNED_FAMILIES = [
    (1, 1, 8, 24),
    (2, 1, 256, 0),
    (3, 1, 128, 0),
    (4, 1, 1, 24),
    (5, 1, 4096, 0),
    (6, 1, 7, 0),
    (7, 1, 64, 0),
    (8, 1, 1, 48),
]
PINNED_FAMILIES_JSON = json.dumps(
    [
        {
            "family": family,
            "record_bytes": record_bytes,
            "record_limit": record_limit,
            "version": version,
        }
        for family, version, record_limit, record_bytes in PINNED_FAMILIES
    ],
    separators=(",", ":"),
)

STATUS_OK = 0
STATUS_INVALID_STATE = 6
STATUS_DISCONNECTED = 7
PHASE_CONNECTING = 1
PHASE_LOGIN = 2


@gdclass
class bridge_contract_check(Node):
    """Drive the bridge through its Godot-visible typed values, offline."""

    def _ready(self) -> None:
        failures: list[str] = []
        bridge = self.get_node("ClientBridge")
        _check_identity(bridge, failures)
        _check_producer_identity(bridge, failures)
        _check_family_table(bridge, failures)
        _check_offline_session(bridge, failures)
        if failures:
            for failure in failures:
                print(f"Python bridge contract check failed: {failure}")
            self.get_tree().quit(1)
            return
        print("Python bridge contract check passed.")
        self.get_tree().quit(0)


def _check_identity(bridge: Node, failures: list[str]) -> None:
    _expect(_call_int(bridge, "client_core_abi_major") == 1, "client-core ABI major", failures)
    _expect(_call_int(bridge, "client_core_abi_minor") == 1, "client-core ABI minor", failures)
    _expect(_call_int(bridge, "godot_api_major") == 4, "Godot API major", failures)
    _expect(_call_int(bridge, "godot_api_minor") == 7, "Godot API minor", failures)
    _expect(_call_text(bridge, "godot_rust_version") == "0.5.5", "godot-rust version", failures)
    _expect(_call_text(bridge, "lifecycle_stage") == "main-loop", "lifecycle stage", failures)


def _check_producer_identity(bridge: Node, failures: list[str]) -> None:
    identity = json.loads(_call_text(bridge, "producer_identity_json"))
    _expect(
        identity == {"available": True, "major": 1, "minor": 1},
        f"producer identity mismatch: {identity}",
        failures,
    )


def _check_family_table(bridge: Node, failures: list[str]) -> None:
    reported = _call_text(bridge, "feature_families_json")
    _expect(
        reported == PINNED_FAMILIES_JSON,
        f"family table mismatch: {reported}",
        failures,
    )


def _check_offline_session(bridge: Node, failures: list[str]) -> None:
    _expect(_call_int(bridge, "session_create") == STATUS_OK, "session create failed", failures)

    identity = cast(Any, bridge.call("pull_identity"))
    _expect(identity["status"] == STATUS_OK, "identity pull failed", failures)
    record = identity["record"]
    _expect(record.size() == 216, f"identity record size: {record.size()}", failures)
    if record.size() == 216:
        header = _decode_header(record, 0)
        _expect(
            header == [0x3149434D, 1, 1, 1, 8],
            f"identity header mismatch: {[hex(word) for word in header]}",
            failures,
        )
        descriptors = [
            (
                _decode32(record, 24 + index * 24),
                _decode32(record, 24 + index * 24 + 4),
                _decode32(record, 24 + index * 24 + 8),
                _decode32(record, 24 + index * 24 + 12),
            )
            for index in range(8)
        ]
        _expect(
            descriptors == PINNED_FAMILIES,
            f"producer descriptors differ from the pinned table: {descriptors}",
            failures,
        )

    world = cast(Any, bridge.call("pull_world"))
    _expect(world["status"] == STATUS_OK, "world pull failed before steps", failures)
    _expect(world["record"].size() == 0, "world pull returned a batch before any step", failures)

    frame = cast(Any, bridge.call("pull_frame"))
    _expect(frame["status"] == STATUS_OK, "frame pull failed before steps", failures)
    _expect(frame["record"].size() == 0, "frame pull returned a frame before any step", failures)

    status = cast(Any, bridge.call("pull_status"))
    _expect(status["status"] == STATUS_OK, "status pull failed", failures)
    status_record = status["record"]
    _expect(status_record.size() == 80, f"status record size: {status_record.size()}", failures)
    if status_record.size() == 80:
        status_header = _decode_header(status_record, 0)
        _expect(
            status_header[0:2] == [0x314D434D, 1] and status_header[2] == 4,
            f"status header mismatch: {[hex(word) for word in status_header]}",
            failures,
        )

    empty_batch = PackedByteArray.from_list(
        [0x4D, 0x43, 0x4E, 0x31, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
    )
    _expect(
        _call_int(bridge, "session_submit", empty_batch) == STATUS_INVALID_STATE,
        "submit before connect was not rejected with invalid state",
        failures,
    )
    _expect(
        _call_int(bridge, "session_step", 16666667, 64, 32) == STATUS_INVALID_STATE,
        "step before connect was not rejected with invalid state",
        failures,
    )
    # Wrapped garbage arguments must not panic or misroute: negative and
    # oversized values marshal into the producer's rejection domains and the
    # offline state check still answers first. The content-level rejection of
    # the wrapped elapsed word needs an online session and belongs to the
    # later playable smokes.
    _expect(
        _call_int(bridge, "session_step", -1, -1, 2**40) == STATUS_INVALID_STATE,
        "wrapped step arguments panicked or misrouted",
        failures,
    )

    # The asynchronous contract: begin returns immediately, the first poll
    # observes connecting or the already-terminal disconnect, and close cancels
    # the establishment. Network access is denied in the qualification sandbox,
    # so the dial cannot succeed and this path stays honest.
    _expect(
        _call_int(bridge, "session_connect", "127.0.0.1:9") == STATUS_OK,
        "connect begin failed",
        failures,
    )
    polled = cast(Any, bridge.call("session_poll"))
    poll_status = polled["status"]
    poll_phase = polled["phase"]
    connecting = poll_status == STATUS_OK and poll_phase in (PHASE_CONNECTING, PHASE_LOGIN)
    terminal = poll_status == STATUS_DISCONNECTED
    _expect(
        connecting or terminal,
        f"connect poll did not report an offline path: status={poll_status} phase={poll_phase}",
        failures,
    )

    _expect(_call_int(bridge, "session_close") == STATUS_OK, "first close failed", failures)
    _expect(_call_int(bridge, "session_close") == STATUS_OK, "second close failed", failures)
    closed_poll = cast(Any, bridge.call("session_poll"))
    _expect(
        closed_poll["status"] == STATUS_INVALID_STATE,
        "poll after close did not report invalid state",
        failures,
    )
    closed_pull = cast(Any, bridge.call("pull_world"))
    _expect(
        closed_pull["status"] == STATUS_INVALID_STATE,
        "pull after close did not report invalid state",
        failures,
    )

    # A fresh session over the same bridge instance still works after close.
    _expect(
        _call_int(bridge, "session_create") == STATUS_OK,
        "re-create after close failed",
        failures,
    )
    _expect(
        _call_int(bridge, "session_close") == STATUS_OK, "close after re-create failed", failures
    )


def _call_int(target: Any, method: str, *arguments: Any) -> int:
    value = target.call(method, *arguments)
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


def _call_text(target: Any, method: str, *arguments: Any) -> str:
    value = target.call(method, *arguments)
    return value if isinstance(value, str) else ""


def _decode32(record: Any, offset: int) -> int:
    value = 0
    for index in range(4):
        value |= int(record[offset + index]) << (8 * index)
    return value


def _decode_header(record: Any, offset: int) -> list[int]:
    return [
        _decode32(record, offset),
        _decode32(record, offset + 4),
        _decode32(record, offset + 8),
        _decode32(record, offset + 12),
        _decode32(record, offset + 16),
    ]


def _expect(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)
