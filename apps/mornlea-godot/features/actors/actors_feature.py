"""Own bounded actor presentation while entity state remains in Go."""

from __future__ import annotations

from typing import Protocol, runtime_checkable

from py4godot.classes import gdclass
from py4godot.classes.Node import Node
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

register_cast_function("MornleaClientBridge", lambda bridge: bridge)


@runtime_checkable
class _DictionaryView(Protocol):
    def __getitem__(self, key: str) -> object: ...


@gdclass
class actors_feature(Node):
    """Reserve the actor scene boundary for typed, capacity-bounded snapshots."""

    _services: Node | None
    _remote_players: Node | None
    _epoch: int
    _active: bool

    def _ready(self) -> None:
        self._services = None
        self._remote_players = self.get_node_or_null("RemotePlayers")
        self._epoch = 0
        self._active = False

    def _process(self, _delta: float) -> None:
        if self._active:
            self.apply_frame()

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "actors" else "unexpected actors feature ID"

    def bind_host(self, services_path: str) -> str:
        # The bridge node is reacquired from its scene path because project
        # classes cannot cross Python module calls in the pinned runtime.
        services = self.get_node_or_null(services_path)
        if services is None or not services.has_method("session_frame_typed"):
            return "the typed entity bridge is missing"
        if self._remote_players is None:
            return "the remote-player presentation pool is missing"
        failure = self._remote_players.call("validate_pool")
        if not isinstance(failure, str) or failure:
            if isinstance(failure, str) and failure:
                return failure
            return "the remote-player pool is invalid"
        self._services = services
        return ""

    def activate_feature(self, epoch: int) -> str:
        if self._services is None or self._remote_players is None:
            return "actors activation before bind"
        self._epoch = epoch
        self._remote_players.call("reset_feature", epoch)
        self._active = True
        return ""

    def reset_feature(self, epoch: int) -> None:
        self._epoch = epoch
        if self._remote_players is not None:
            self._remote_players.call("reset_feature", epoch)

    def deactivate_feature(self) -> None:
        self._active = False
        if self._remote_players is not None:
            self._remote_players.call("deactivate_feature")
        self._services = None
        self._epoch = 0

    def apply_frame(self) -> str:
        """Sample one typed frame and fan only its entity values to the pool."""
        if self._services is None or self._remote_players is None:
            return "the actors feature is not bound"
        typed = self._services.call("session_frame_typed")
        if not isinstance(typed, _DictionaryView):
            return "the typed entity frame answer is not a dictionary"
        failure = self._remote_players.call("apply_typed_frame", typed)
        return failure if isinstance(failure, str) else "the remote-player apply result is invalid"
