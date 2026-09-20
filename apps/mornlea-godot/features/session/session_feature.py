"""Own the pilot session lifecycle display through the typed native bridge.

This is the first real Godot feature scene: it drives the producer session
(`create`/`connect`/`poll`/`close` are feature-owned bridge methods) and maps
the confirmed connection state onto a minimal Control display. The display
never fabricates state: every phase word comes from the bridge's typed poll
result, and the terminal phase plus its stable error line come from the
bridge's typed status view (`session_status_typed`). Record decoding is
Rust-owned per the pilot's design decision 4 — Python consumes typed
Godot-visible semantic values only, and any typed-view disagreement fails
closed to the stable internal error.

Address policy: the feature never dials on its own. Without an explicit
`request_connect(address)` call it shows the not-ready display, so headless
lifecycle smokes stay offline-honest and no address is ever hardcoded.
"""

from __future__ import annotations

from typing import Protocol, runtime_checkable

from py4godot.classes import gdclass
from py4godot.classes.Control import Control
from py4godot.classes.Node import Node
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# Features acquire the native bridge through `get_node` plus the identity
# cast, because the pinned runtime cannot marshal project-class objects
# across script-module method boundaries.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)

# Producer status words from the frozen client-core contract. Only the two
# words the display distinguishes are named; every other status observed
# while a session should be live fails closed to the internal error display.
STATUS_OK = 0
STATUS_INVALID_STATE = 6
STATUS_DISCONNECTED = 7

# Connection phase words: the identity cast of the presentation session
# phase domain (0 not ready, 1 connecting, 2 login, 3 loading, 4 play,
# 5 disconnected). The login word exists in the vocabulary for completeness
# even though this producer generation never reports it.
PHASE_DISCONNECTED = 5

# Stable display strings. The phase line covers the whole phase vocabulary;
# the error line is a stable English set keyed by the typed status view's
# terminal-cause classification, with unknown causes degraded to the
# internal message (the producer's documented safe downgrade direction).
PHASE_DISPLAY = {
    0: "Not ready",
    1: "Connecting",
    2: "Logging in",
    3: "Loading",
    4: "Play",
    PHASE_DISCONNECTED: "Disconnected",
}
CAUSE_DISPLAY = {
    0: "",
    1: "Could not reach the server.",
    2: "The server rejected the connection handshake.",
    3: "The server rejected the login.",
    4: "Incompatible server protocol.",
    5: "The connection was lost.",
    6: "An internal client error occurred.",
}
INTERNAL_ERROR_TEXT = "An internal client error occurred."


@runtime_checkable
class _DictionaryView(Protocol):
    """Minimal typed view of a bridge poll/typed-status result dictionary."""

    def __getitem__(self, key: str) -> object: ...


def _word_of(value: object) -> int:
    # Boolean values are rejected so a marshaling drift cannot masquerade as
    # the status word 0/1 that the display branches on.
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


@gdclass
class session_feature(Control):
    """Display confirmed connection state and own the pilot session lifecycle."""

    _bridge: Node | None
    _epoch: int
    _session_open: bool
    _terminal_latched: bool
    _phase_text: str
    _error_text: str
    _phase_label: Node | None
    _error_label: Node | None

    def _ready(self) -> None:
        self._bridge = None
        self._epoch = 0
        self._session_open = False
        self._terminal_latched = False
        self._phase_label = self.get_node_or_null("PhaseLabel")
        self._error_label = self.get_node_or_null("ErrorLabel")
        # The initial display is honestly not-ready: without a requested
        # address there is no confirmed connection state to show.
        self._set_display(PHASE_DISPLAY[0], "")

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "session" else "unexpected session feature ID"

    def bind_host(self, services_path: str) -> str:
        # The typed service identity is the bridge's family table; the
        # family versions themselves are negotiated by the host against the
        # manifest, so this check only fails closed on a wrong node.
        bridge = self.get_node_or_null(services_path)
        if bridge is None:
            return "the typed bridge service node is missing"
        if not bridge.has_method("feature_families_json"):
            return "the typed bridge family table is missing"
        self._bridge = bridge
        return ""

    def activate_feature(self, epoch: int) -> str:
        if self._bridge is None:
            return "activation before bind"
        self._epoch = epoch
        # Activation performs no network work and creates no session; the
        # display shows whatever confirmed state the bridge reports.
        self.refresh()
        return ""

    def reset_feature(self, epoch: int) -> None:
        # A host reset refreshes observation only: the producer session this
        # feature owns is not torn down or fabricated by a display reset.
        self._epoch = epoch
        self.refresh()

    def deactivate_feature(self) -> None:
        # Closing here keeps feature teardown ahead of the bridge release
        # in the shutdown order; the close itself is idempotent.
        self._release_session()
        self._bridge = None
        self._terminal_latched = True

    def request_connect(self, address: str) -> str:
        """Begin one connection attempt; returns an empty string on success."""
        bridge = self._bridge
        if bridge is None:
            return "the typed bridge service is not bound"
        if self._session_open:
            return "a connection request is already active"
        if not address:
            return "a server address is required"
        create_status = _word_of(bridge.call("session_create"))
        if create_status != STATUS_OK:
            return "the bridge session could not be created"
        connect_status = _word_of(bridge.call("session_connect", address))
        if connect_status != STATUS_OK:
            bridge.call("session_close")
            return "the connection request was rejected"
        self._session_open = True
        self._terminal_latched = False
        # The first observation may already be terminal, so the display is
        # refreshed from confirmed state instead of an optimistic phase.
        self.refresh()
        return ""

    def close_connection(self) -> None:
        """Close the owned session and publish the clean-close display."""
        bridge = self._bridge
        if bridge is None or not self._session_open:
            return
        close_status = _word_of(bridge.call("session_close"))
        self._session_open = False
        self._terminal_latched = True
        if close_status == STATUS_OK:
            # A clean user-requested close records no terminal cause, so the
            # confirmed close status is the evidence for the error-free line.
            self._set_display(PHASE_DISPLAY[PHASE_DISCONNECTED], "")
        else:
            self._set_display(PHASE_DISPLAY[PHASE_DISCONNECTED], INTERNAL_ERROR_TEXT)

    def drive_session(self, elapsed_ns: int, message_budget: int, mesh_budget: int) -> int:
        """Advance the owned session for the host's single bounded step."""
        bridge = self._bridge
        if bridge is None or not self._session_open or self._terminal_latched:
            return STATUS_INVALID_STATE
        return _word_of(bridge.call("session_step", elapsed_ns, message_budget, mesh_budget))

    def pull_typed_frame(self) -> object:
        """Return the one typed frame selected by the host fan-out pass."""
        bridge = self._bridge
        if bridge is None or not self._session_open or self._terminal_latched:
            return None
        return bridge.call("session_frame_typed")

    def refresh(self) -> None:
        """Observe the bridge once and map the confirmed state to the display."""
        bridge = self._bridge
        if bridge is None or not self._session_open or self._terminal_latched:
            return
        polled = bridge.call("session_poll")
        if not isinstance(polled, _DictionaryView):
            self._fail_closed()
            return
        status = _word_of(polled["status"])
        if status == STATUS_DISCONNECTED:
            # The disconnected status is the terminal signal; the phase and
            # cause come from the typed status view read right here.
            self._apply_terminal()
            return
        if status != STATUS_OK:
            self._fail_closed()
            return
        self._display_phase(_word_of(polled["phase"]))

    def phase_text(self) -> str:
        return self._phase_text

    def error_text(self) -> str:
        return self._error_text

    def _display_phase(self, phase: int) -> None:
        if phase == PHASE_DISCONNECTED:
            self._apply_terminal()
            return
        text = PHASE_DISPLAY.get(phase)
        if text is None:
            self._fail_closed()
            return
        self._set_display(text, "")

    def _apply_terminal(self) -> None:
        bridge = self._bridge
        if bridge is None:
            self._fail_closed()
            return
        typed = bridge.call("session_status_typed")
        if not isinstance(typed, _DictionaryView):
            self._fail_closed()
            return
        if _word_of(typed["status"]) != STATUS_OK:
            self._fail_closed()
            return
        phase = _word_of(typed["phase"])
        cause = _word_of(typed["terminal_cause"])
        if phase != PHASE_DISCONNECTED:
            # The terminal signal and the typed status view disagree, which
            # is producer drift: both are displayed as the stable internal
            # error rather than a guessed phase or cause.
            self._fail_closed()
            return
        self._terminal_latched = True
        self._set_display(
            PHASE_DISPLAY[PHASE_DISCONNECTED],
            CAUSE_DISPLAY.get(cause, INTERNAL_ERROR_TEXT),
        )

    def _fail_closed(self) -> None:
        # Every unexpected observation lands on the same stable display, and
        # the session is released defensively: the close is idempotent, so a
        # producer that already tore the session down is harmless.
        self._release_session()
        self._terminal_latched = True
        self._set_display(PHASE_DISPLAY[PHASE_DISCONNECTED], INTERNAL_ERROR_TEXT)

    def _release_session(self) -> None:
        bridge = self._bridge
        if bridge is not None and self._session_open:
            bridge.call("session_close")
            self._session_open = False

    def _set_display(self, phase: str, error: str) -> None:
        self._phase_text = phase
        self._error_text = error
        self._apply_label(self._phase_label, phase)
        self._apply_label(self._error_label, error)

    def _apply_label(self, label: Node | None, text: str) -> None:
        if label is not None:
            label.call("set_text", text)
