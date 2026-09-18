"""Map confirmed environment values onto the Godot world environment."""

from __future__ import annotations

import math
from typing import Any, Protocol, cast, runtime_checkable

try:
    from py4godot.classes import gdclass
    from py4godot.classes.core import Color
    from py4godot.classes.Node import Node
    from py4godot.classes.Object import Object
except ImportError:  # pragma: no cover - source-level checks run without Godot

    def gdclass(value: Any) -> Any:  # type: ignore[misc]
        return value

    class Node:  # type: ignore[no-redef]
        pass

    class Object:  # type: ignore[no-redef]
        pass

    class Color:  # type: ignore[no-redef]
        @staticmethod
        def new4(r: float, g: float, b: float, a: float) -> tuple[float, float, float, float]:
            return (r, g, b, a)


@runtime_checkable
class _DictionaryView(Protocol):
    def __getitem__(self, key: str) -> object: ...


STATUS_OK = 0
PHASE_PLAY = 4
WEATHER_CLEAR = 0
WEATHER_RAIN = 1
WEATHER_THUNDER = 2
SEASON_MAX = 3


def _number(value: object) -> float | None:
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        number = float(value)
        return number if math.isfinite(number) else None
    return None


def _word(value: object) -> int:
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


@gdclass
class environment_feature(Node):
    """Own bounded Godot resource writes; Go owns environment calculations."""

    _world_environment: Node | None
    _active: bool
    _last_error: str
    _weather: int
    _season: int
    _daylight: float
    _sky_color: tuple[float, float, float]

    def _ready(self) -> None:
        self._world_environment = self.get_node_or_null("WorldEnvironment")
        self._active = False
        self._last_error = ""
        self._weather = WEATHER_CLEAR
        self._season = 0
        self._daylight = 0.0
        self._sky_color = (0.0, 0.0, 0.0)

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "world.environment" else "unexpected environment feature ID"

    def bind_host(self, _services_path: str) -> str:
        if self._world_environment is None:
            return "the WorldEnvironment node is missing"
        return ""

    def activate_feature(self, _epoch: int) -> str:
        if self._world_environment is None:
            return "environment activation before bind"
        self._active = True
        return ""

    def reset_feature(self, _epoch: int) -> None:
        self._active = False
        self._last_error = ""
        self._daylight = 0.0
        self._sky_color = (0.0, 0.0, 0.0)

    def deactivate_feature(self) -> None:
        self._active = False
        self._world_environment = None

    def apply_typed_frame(self, frame: _DictionaryView) -> str:
        """Apply one already-sampled frame without inventing unavailable state."""
        if not self._active:
            return ""
        if _word(frame["status"]) != STATUS_OK:
            return self._fail("the typed environment frame is unavailable")
        if _word(frame["phase"]) != PHASE_PLAY or not bool(frame["environment_ready"]):
            self._clear()
            return ""
        weather = _word(frame["weather"])
        season = _word(frame["season"])
        progress = _word(frame["season_progress"])
        daylight = _number(frame["daylight"])
        sky_r = _number(frame["sky_r"])
        sky_g = _number(frame["sky_g"])
        sky_b = _number(frame["sky_b"])
        if weather < WEATHER_CLEAR or weather > WEATHER_THUNDER:
            return self._fail("the typed weather value is outside the supported domain")
        if season < 0 or season > SEASON_MAX or progress < 0 or progress > 255:
            return self._fail("the typed season value is outside the supported domain")
        if (
            daylight is None
            or not 0.0 <= daylight <= 1.0
            or any(color is None or not 0.0 <= color <= 1.0 for color in (sky_r, sky_g, sky_b))
        ):
            return self._fail("the typed sky values are outside the supported domain")
        self._weather = weather
        self._season = season
        self._daylight = daylight
        assert sky_r is not None and sky_g is not None and sky_b is not None
        self._sky_color = (sky_r, sky_g, sky_b)
        world_environment = self._world_environment
        if world_environment is not None:
            environment = world_environment.call("get_environment")
            if environment is not None:
                environment_object = cast(Object, environment)
                environment_object.call(
                    "set", "background_color", Color.new4(*self._sky_color, 1.0)
                )
                environment_object.call("set", "ambient_light_energy", daylight)
        self._last_error = ""
        return ""

    def weather_kind(self) -> int:
        return self._weather

    def season_kind(self) -> int:
        return self._season

    def daylight(self) -> float:
        return self._daylight

    def sky_color(self) -> tuple[float, float, float]:
        return self._sky_color

    def last_error(self) -> str:
        return self._last_error

    def _clear(self) -> None:
        self._daylight = 0.0
        self._sky_color = (0.0, 0.0, 0.0)

    def _fail(self, message: str) -> str:
        self._clear()
        self._last_error = message
        return message
