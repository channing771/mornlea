"""Exercise the complete minimum pilot loop against one real dedicated server.

The production feature catalog owns every presentation path in this scenario.
This driver supplies only primitive semantic input and observes typed Godot
views after the host has completed its ordered input, session, terrain, and
single-frame fan-out pass. It never decodes a wire record or mutates authority.
"""

from __future__ import annotations

import json
import math
from typing import Protocol, TypedDict, cast, runtime_checkable

from py4godot.classes import gdclass
from py4godot.classes.Node import Node
from py4godot.classes.OS import OS
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

register_cast_function("MornleaClientBridge", lambda bridge: bridge)

ADDRESS_VARIABLE = "MORNLEA_PLAYABLE_SMOKE_ADDRESS"
DURATION_VARIABLE = "MORNLEA_PLAYABLE_SMOKE_DURATION_SECONDS"
REMOTE_PLAYER_ID_VARIABLE = "MORNLEA_PLAYABLE_SMOKE_REMOTE_PLAYER_ID"
EXPECTED_ORDER = [
    "session",
    "actors",
    "platform.desktop.input",
    "player_view",
    "ui",
    "world",
]
EXPECTED_DISABLED = {"platform.desktop.audio", "platform.desktop.lifecycle"}
PLAY_TEXT = "Play"
LOADING_TEXT = "Loading"
DISCONNECTED_TEXT = "Disconnected"
WAIT_SECONDS = 180.0
MOVE_DISTANCE_SQUARED = 0.0025
TARGET_PROBE_PITCH = -1.55
DURATION_SAMPLE_SECONDS = 0.25
INITIAL_SESSION_EPOCH = 1


class PlanResult(TypedDict):
    ok: bool
    errors: list[str]
    disabled: list[str]
    order: list[str]


@runtime_checkable
class _DictionaryView(Protocol):
    def __getitem__(self, key: str) -> object: ...


@runtime_checkable
class _ArrayView(Protocol):
    def size(self) -> int: ...

    def get(self, index: int) -> object: ...


def _word(value: object) -> int:
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


def _number(value: object) -> float | None:
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        result = float(value)
        return result if math.isfinite(result) else None
    return None


def _call_text(target: Node, method: str, *arguments: object) -> str:
    value = target.call(method, *arguments)
    return value if isinstance(value, str) else ""


def _text_value(value: object) -> str | None:
    """Normalize typed-view text before applying the strict UUID validator."""
    if isinstance(value, str):
        return value
    try:
        return str(value)
    except Exception:
        return None


def _canonical_player_id(value: object) -> str | None:
    value_text = _text_value(value)
    if value_text is None or len(value_text) != 36:
        return None
    if any(value_text[index] != "-" for index in (8, 13, 18, 23)):
        return None
    compact = value_text.replace("-", "")
    if compact.lower() != compact or any(
        character not in "0123456789abcdef" for character in compact
    ):
        return None
    if value_text[14] != "4" or value_text[19] not in "89ab":
        return None
    return value_text


@gdclass
class playable_smoke_check(Node):
    """Drive and verify the minimum playable feature catalog for a bounded run."""

    _failures: list[str]
    _host: Node | None
    _bridge: Node | None
    _session: Node | None
    _input: Node | None
    _camera: Node | None
    _target: Node | None
    _remotes: Node | None
    _environment: Node | None
    _hud: Node | None
    _duration: float
    _wait_elapsed: float
    _play_elapsed: float
    _duration_elapsed: float
    _duration_sample_elapsed: float
    _stage: str
    _phases: set[str]
    _initial_position: tuple[float, float, float] | None
    _initial_look: tuple[float, float] | None
    _movement_seen: bool
    _look_seen: bool
    _camera_seen: bool
    _terrain_seen: bool
    _terrain_update_seen: bool
    _target_seen: bool
    _remote_seen: bool
    _environment_seen: bool
    _hud_seen: bool
    _last_frame: str
    _last_terrain: str
    _expected_remote_player_id: str
    _initial_terrain_uploads: int | None
    _latest_terrain_uploads: int
    _duration_samples: int
    _last_sample_revision: int
    _last_sample_steps: int
    _last_observed_revision: int
    _last_observed_steps: int

    def _ready(self) -> None:
        self._failures = []
        self._host = self.get_node_or_null("FeatureHost")
        self._bridge = self.get_node_or_null("ClientBridge")
        self._session = None
        self._input = None
        self._camera = None
        self._target = None
        self._remotes = None
        self._environment = None
        self._hud = None
        self._duration = 0.0
        self._wait_elapsed = 0.0
        self._play_elapsed = 0.0
        self._duration_elapsed = 0.0
        self._duration_sample_elapsed = 0.0
        self._stage = "activate"
        self._phases = set()
        self._initial_position = None
        self._initial_look = None
        self._movement_seen = False
        self._look_seen = False
        self._camera_seen = False
        self._terrain_seen = False
        self._terrain_update_seen = False
        self._target_seen = False
        self._remote_seen = False
        self._environment_seen = False
        self._hud_seen = False
        self._last_frame = ""
        self._last_terrain = ""
        self._expected_remote_player_id = ""
        self._initial_terrain_uploads = None
        self._latest_terrain_uploads = 0
        self._duration_samples = 0
        self._last_sample_revision = 0
        self._last_sample_steps = 0
        self._last_observed_revision = 0
        self._last_observed_steps = 0
        address = OS.instance().get_environment(ADDRESS_VARIABLE)
        duration_text = OS.instance().get_environment(DURATION_VARIABLE)
        remote_player_id = OS.instance().get_environment(REMOTE_PLAYER_ID_VARIABLE)
        try:
            self._duration = float(duration_text)
        except TypeError:
            self._failures.append(f"{DURATION_VARIABLE} is not a duration in seconds")
        except ValueError:
            self._failures.append(f"{DURATION_VARIABLE} is not a duration in seconds")
        if not isinstance(address, str) or ":" not in address:
            self._failures.append(f"{ADDRESS_VARIABLE} is not a host:port address")
        if not math.isfinite(self._duration) or self._duration <= 0.0:
            self._failures.append("the playable duration must be finite and positive")
        canonical_remote_id = _canonical_player_id(remote_player_id)
        if canonical_remote_id is None:
            self._failures.append(f"{REMOTE_PLAYER_ID_VARIABLE} is not a canonical player UUID")
        else:
            self._expected_remote_player_id = canonical_remote_id
        if self._host is None or self._bridge is None:
            self._failures.append("the playable scene is missing its host or typed bridge")
        if self._failures:
            self._finish(1)
            return
        self._activate(address)

    def _activate(self, address: str) -> None:
        host = self._host
        if host is None:
            return
        raw = cast(
            str,
            host.call(
                "activate_catalog",
                "res://config/feature_catalog.tres",
                "../ClientBridge",
                INITIAL_SESSION_EPOCH,
            ),
        )
        result = cast(PlanResult, json.loads(raw))
        disabled = set(result["disabled"])
        if not result["ok"] or result["order"] != EXPECTED_ORDER or disabled != EXPECTED_DISABLED:
            trace = _call_text(host, "trace_json")
            self._failures.append(
                f"production catalog activation differed: {result}; trace={trace}"
            )
            self._finish(1)
            return
        if cast(int, host.call("active_count")) != len(EXPECTED_ORDER):
            self._failures.append("the complete minimum feature set did not stay active")
            self._finish(1)
            return
        self._resolve_features(host)
        if self._failures:
            self._finish(1)
            return
        # Hold the production host step until the asynchronous connection has
        # visibly reached Loading. This makes Connecting -> Loading -> Play a
        # decidable lifecycle observation without substituting a fake session.
        host.call("set_process", False)
        session = self._session
        if session is None:
            return
        failure = _call_text(session, "request_connect", address)
        if failure:
            self._failures.append(f"connection begin failed: {failure}")
            self._finish(1)
            return
        self._record_phase(_call_text(session, "phase_text"))
        self._stage = "wait-loading"

    def _resolve_features(self, host: Node) -> None:
        session = host.get_node_or_null("SessionFeature")
        input_feature = host.get_node_or_null("DesktopInputFeature")
        player_view = host.get_node_or_null("PlayerViewFeature")
        actors = host.get_node_or_null("ActorsFeature")
        world = host.get_node_or_null("WorldFeature")
        ui = host.get_node_or_null("UIFeature")
        if any(node is None for node in (session, input_feature, player_view, actors, world, ui)):
            self._failures.append("one or more minimum feature roots are missing")
            return
        assert (
            player_view is not None and actors is not None and world is not None and ui is not None
        )
        self._session = session
        self._input = input_feature
        self._camera = player_view.get_node_or_null("Camera")
        self._target = player_view.get_node_or_null("TargetFeedback")
        self._remotes = actors.get_node_or_null("RemotePlayers")
        self._environment = world.get_node_or_null("Environment")
        self._hud = ui.get_node_or_null("HUD")
        if any(
            node is None
            for node in (self._camera, self._target, self._remotes, self._environment, self._hud)
        ):
            self._failures.append("one or more minimum presentation components are missing")

    def _process(self, delta: float) -> None:
        if self._stage == "done" or self._failures:
            return
        self._wait_elapsed += max(0.0, float(delta))
        session = self._session
        if session is None:
            return
        session.call("refresh")
        phase = _call_text(session, "phase_text")
        self._record_phase(phase)
        if phase == DISCONNECTED_TEXT:
            self._failures.append(
                f"the session disconnected early: {_call_text(session, 'error_text')}"
            )
        elif self._stage == "wait-loading" and phase == LOADING_TEXT:
            self._stage = "wait-play"
            host = self._host
            if host is not None:
                host.call("set_process", True)
        elif self._stage == "wait-play" and phase == PLAY_TEXT:
            self._stage = "run"
        if self._failures:
            self._finish(1)
            return
        if self._stage in {"run", "duration"}:
            self._play_elapsed += max(0.0, float(delta))
            typed, current_valid = self._observe_playable_frame()
            self._drive_intent()
            if self._stage == "run" and self._minimum_ready():
                if typed is not None and current_valid:
                    self._start_duration(typed)
            elif self._stage == "duration":
                self._duration_elapsed += max(0.0, float(delta))
                self._duration_sample_elapsed += max(0.0, float(delta))
                if typed is None or not current_valid:
                    self._failures.append(
                        "the current minimum-loop presentation became invalid during duration"
                    )
                else:
                    self._verify_duration_frame(typed)
            if self._failures:
                self._finish(1)
                return
            if self._stage == "duration" and self._duration_elapsed >= self._duration:
                self._conclude()
                return
        if self._wait_elapsed > WAIT_SECONDS + self._duration:
            self._failures.append(f"stage {self._stage} exceeded its bounded wall-time budget")
            self._finish(1)

    def _observe_playable_frame(self) -> tuple[_DictionaryView | None, bool]:
        bridge = self._bridge
        if bridge is None:
            return None, False
        typed = bridge.call("session_frame_typed")
        if not isinstance(typed, _DictionaryView):
            self._failures.append("the playable frame is not a typed dictionary")
            return None, False
        status = _word(typed["status"])
        phase = _word(typed["phase"])
        entities = typed["entities"]
        entity_count = entities.size() if isinstance(entities, _ArrayView) else -1
        pose = tuple(
            _number(typed[key])
            for key in ("position_x", "position_y", "position_z", "yaw", "pitch")
        )
        self._last_frame = (
            f"status={status} phase={phase} revision={_word(typed['revision'])} "
            f"entities={entity_count} pose={pose} target={bool(typed['target_visible'])}"
        )
        if status != 0 or phase != 4:
            return typed, False
        self._observe_motion(typed)
        components_valid = self._observe_components(typed)
        terrain_valid = self._observe_terrain()
        return typed, components_valid and terrain_valid

    def _observe_motion(self, typed: _DictionaryView) -> None:
        values = [_number(typed[key]) for key in ("position_x", "position_y", "position_z")]
        yaw = _number(typed["yaw"])
        pitch = _number(typed["pitch"])
        if any(value is None for value in values) or yaw is None or pitch is None:
            self._failures.append("the typed camera pose is not finite")
            return
        position = cast(tuple[float, float, float], tuple(values))
        if self._initial_position is None:
            self._initial_position = position
            self._initial_look = (yaw, pitch)
            return
        distance_squared = sum(
            (position[index] - self._initial_position[index]) ** 2 for index in range(3)
        )
        self._movement_seen = self._movement_seen or distance_squared >= MOVE_DISTANCE_SQUARED
        assert self._initial_look is not None
        self._look_seen = self._look_seen or (
            abs(yaw - self._initial_look[0]) >= 0.05 or abs(pitch - self._initial_look[1]) >= 0.05
        )

    def _observe_components(self, typed: _DictionaryView) -> bool:
        camera = self._camera
        target = self._target
        remotes = self._remotes
        environment = self._environment
        hud = self._hud
        camera_valid = camera is not None and (
            bool(camera.call("camera_ready")) and not _call_text(camera, "last_error")
        )
        target_valid = target is not None and (
            bool(target.call("visible")) and bool(_call_text(target, "target_name"))
        )
        remote_valid = remotes is not None and self._has_expected_remote(remotes, typed)
        environment_valid = False
        if environment is not None and bool(typed["environment_ready"]):
            weather = cast(int, environment.call("weather_kind"))
            season = cast(int, environment.call("season_kind"))
            daylight = _number(environment.call("daylight"))
            environment_valid = (
                0 <= weather <= 2
                and 0 <= season <= 3
                and daylight is not None
                and 0.0 <= daylight <= 1.0
                and not _call_text(environment, "last_error")
            )
        hud_valid = False
        if hud is not None and bool(typed["hud_ready"]):
            hud_valid = self._confirmed_hud_visible(hud, typed)
        self._camera_seen = self._camera_seen or camera_valid
        self._target_seen = self._target_seen or target_valid
        self._remote_seen = self._remote_seen or remote_valid
        self._environment_seen = self._environment_seen or environment_valid
        self._hud_seen = self._hud_seen or hud_valid
        return all((camera_valid, target_valid, remote_valid, environment_valid, hud_valid))

    def _has_expected_remote(self, remotes: Node, typed: _DictionaryView) -> bool:
        """Confirm the exact typed UUID also reached the production actor pool."""
        entities = typed["entities"]
        identity_seen = False
        if isinstance(entities, _ArrayView):
            for index in range(entities.size()):
                entity = entities.get(index)
                if not isinstance(entity, _DictionaryView):
                    continue
                if _canonical_player_id(entity["player_id"]) == self._expected_remote_player_id:
                    identity_seen = True
                    break
        return (
            identity_seen
            and cast(int, remotes.call("active_count")) > 0
            and not _call_text(remotes, "last_error")
        )

    def _confirmed_hud_visible(self, hud: Node, typed: _DictionaryView) -> bool:
        expected = {
            "HealthLabel": f"Health: {_word(typed['health'])}/20",
            "HungerLabel": f"Hunger: {_word(typed['hunger'])}/20",
            "OxygenLabel": f"Oxygen: {_word(typed['oxygen'])}/300",
            "ConnectionLabel": "Connection: Play",
            "PilotLabel": "Limited Godot pilot",
        }
        for name, text in expected.items():
            label = hud.get_node_or_null(name)
            if label is None or not bool(label.call("is_visible")):
                return False
            if _call_text(label, "get_text") != text:
                return False
        return not _call_text(hud, "last_error")

    def _observe_terrain(self) -> bool:
        bridge = self._bridge
        if bridge is None:
            return False
        raw = bridge.call("terrain_sections_json")
        if not isinstance(raw, str) or not raw:
            return False
        self._last_terrain = raw
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError:
            self._failures.append("the terrain structural summary is not valid JSON")
            return False
        if not isinstance(parsed, dict) or _word(parsed.get("status")) != 0:
            return False
        sections = parsed.get("sections")
        facts = parsed.get("facts")
        uploads = _word(facts.get("uploads_applied")) if isinstance(facts, dict) else -1
        current = (
            isinstance(sections, list)
            and any(isinstance(section, dict) for section in sections)
            and uploads > 0
        )
        if not current:
            return False
        if self._initial_terrain_uploads is None:
            self._initial_terrain_uploads = uploads
        elif uploads < self._latest_terrain_uploads:
            self._failures.append("the terrain upload counter moved backward")
        elif uploads > self._initial_terrain_uploads:
            self._terrain_update_seen = True
        self._latest_terrain_uploads = uploads
        self._terrain_seen = True
        return True

    def _start_duration(self, typed: _DictionaryView) -> None:
        self._stage = "duration"
        self._duration_elapsed = 0.0
        self._duration_sample_elapsed = 0.0
        self._duration_samples = 0
        self._last_sample_revision = _word(typed["revision"])
        self._last_observed_revision = self._last_sample_revision
        status = self._typed_status()
        if status is None:
            return
        self._last_sample_steps = _word(status["steps_completed"])
        self._last_observed_steps = self._last_sample_steps

    def _typed_status(self) -> _DictionaryView | None:
        bridge = self._bridge
        if bridge is None:
            return None
        typed = bridge.call("session_status_typed")
        if not isinstance(typed, _DictionaryView):
            self._failures.append("the typed session status is not a dictionary")
            return None
        if _word(typed["status"]) != 0 or _word(typed["phase"]) != 4:
            self._failures.append("the typed session status did not remain in Play")
            return None
        if _word(typed["terminal_cause"]) != 0:
            self._failures.append("the typed session status reported a terminal cause during Play")
            return None
        return typed

    def _verify_duration_frame(self, typed: _DictionaryView) -> None:
        revision = _word(typed["revision"])
        status = self._typed_status()
        if status is None:
            return
        steps = _word(status["steps_completed"])
        if revision <= 0 or revision < self._last_observed_revision:
            self._failures.append("the duration frame revision is invalid or moved backward")
            return
        if steps <= 0 or steps < self._last_observed_steps:
            self._failures.append("the duration step counter is invalid or moved backward")
            return
        self._last_observed_revision = revision
        self._last_observed_steps = steps
        if self._duration_sample_elapsed < DURATION_SAMPLE_SECONDS:
            return
        if revision <= self._last_sample_revision or steps <= self._last_sample_steps:
            self._failures.append("the duration sample did not advance frame and step revisions")
            return
        self._last_sample_revision = revision
        self._last_sample_steps = steps
        self._duration_samples += 1
        self._duration_sample_elapsed = 0.0

    def _drive_intent(self) -> None:
        input_feature = self._input
        if input_feature is None:
            return
        # Probe almost vertically so the fail-closed ray stays inside the
        # spawn chunk while adjacent snapshots are still arriving. Movement
        # starts only after the real mirror yields target feedback, then holds
        # neutral input after movement is observed so the long-run oracle stays
        # inside the already-qualified loaded area.
        direction = 0
        if not self._movement_seen:
            direction = 1 if self._target_seen else 0
        yaw = 0.0 if not self._target_seen else 0.35
        status = input_feature.call(
            "submit_semantic_intent",
            0,
            direction,
            False,
            False,
            False,
            False,
            False,
            yaw,
            TARGET_PROBE_PITCH,
        )
        if not isinstance(status, int) or isinstance(status, bool) or status != 0:
            self._failures.append(f"semantic input submission failed with status {status}")

    def _record_phase(self, phase: str) -> None:
        if phase:
            self._phases.add(phase)

    def _minimum_ready(self) -> bool:
        return all(
            (
                self._camera_seen,
                self._terrain_seen,
                self._terrain_update_seen,
                self._target_seen,
                self._remote_seen,
                self._environment_seen,
                self._hud_seen,
            )
        )

    def _conclude(self) -> None:
        expected = {
            "Connecting": "Connecting" in self._phases,
            "Loading": LOADING_TEXT in self._phases,
            "Play": PLAY_TEXT in self._phases,
            "movement": self._movement_seen,
            "look": self._look_seen,
            "camera": self._camera_seen,
            "terrain": self._terrain_seen,
            "terrain update": self._terrain_update_seen,
            "target": self._target_seen,
            "remote player": self._remote_seen,
            "environment": self._environment_seen,
            "HUD": self._hud_seen,
            "advancing duration samples": self._duration_samples > 0,
        }
        missing = [name for name, observed in expected.items() if not observed]
        if missing:
            self._failures.append(f"playable evidence is missing: {', '.join(missing)}")
        session = self._session
        if session is not None:
            session.call("close_connection")
            if _call_text(session, "phase_text") != DISCONNECTED_TEXT:
                self._failures.append("clean close did not publish Disconnected")
        host = self._host
        if host is not None:
            host.call("deactivate_features")
            if cast(int, host.call("active_count")) != 0:
                self._failures.append("feature teardown left active instances")
        if self._failures:
            self._finish(1)
            return
        print(
            "Python playable smoke passed: "
            f"duration={self._duration:.3f}s phases={sorted(self._phases)} "
            f"samples={self._duration_samples} terrain_uploads="
            f"{self._initial_terrain_uploads}->{self._latest_terrain_uploads} "
            f"remote={self._expected_remote_player_id} "
            "movement=1 look=1 terrain=1 terrain_update=1 target=1 remote=1 "
            "environment=1 hud=1 teardown=1."
        )
        self._finish(0)

    def _finish(self, exit_code: int) -> None:
        self._stage = "done"
        if self._failures:
            for failure in self._failures:
                print(f"Python playable smoke failed: {failure}")
            print(
                "Python playable smoke evidence: "
                f"phases={sorted(self._phases)} movement={self._movement_seen} "
                f"look={self._look_seen} camera={self._camera_seen} terrain={self._terrain_seen} "
                f"terrain_update={self._terrain_update_seen} samples={self._duration_samples} "
                f"target={self._target_seen} remote={self._remote_seen} "
                f"environment={self._environment_seen} hud={self._hud_seen}"
            )
            print(f"Python playable smoke last frame: {self._last_frame}")
            print(f"Python playable smoke last terrain: {self._last_terrain}")
        self.call_deferred("_quit", exit_code)

    def _quit(self, exit_code: int) -> None:
        self.get_tree().quit(exit_code)
