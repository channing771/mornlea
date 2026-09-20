from __future__ import annotations

import importlib.util
import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = ROOT / "apps/mornlea-godot/features/world/environment/environment_feature.py"
SCENE = ROOT / "apps/mornlea-godot/features/world/environment/environment.tscn"
spec = importlib.util.spec_from_file_location("mornlea_environment_feature", SOURCE)
assert spec and spec.loader
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FakeEnvironment:
    def __init__(self) -> None:
        self.calls: list[tuple[str, object]] = []

    def call(self, name: str, *args: object) -> None:
        self.calls.append((name, args))


class FakeWorldEnvironment:
    def __init__(self) -> None:
        self.environment = FakeEnvironment()

    def call(self, name: str, *args: object) -> object:
        assert name == "get_environment"
        return self.environment


def frame(**overrides: object) -> dict[str, object]:
    result: dict[str, object] = {
        "status": 0,
        "phase": 4,
        "environment_ready": True,
        "weather": 0,
        "season": 1,
        "season_progress": 128,
        "daylight": 0.75,
        "sky_r": 0.2,
        "sky_g": 0.3,
        "sky_b": 0.4,
    }
    result.update(overrides)
    return result


class EnvironmentMappingTests(unittest.TestCase):
    def make_feature(self):
        feature = module.environment_feature.__new__(module.environment_feature)
        feature._world_environment = FakeWorldEnvironment()
        feature._active = True
        feature._last_error = ""
        feature._weather = module.WEATHER_CLEAR
        feature._season = 0
        feature._daylight = 0.0
        feature._sky_color = (0.0, 0.0, 0.0)
        return feature

    def test_confirmed_weather_and_sky_values_are_applied(self):
        feature = self.make_feature()
        self.assertEqual(
            feature.apply_typed_frame(frame(weather=module.WEATHER_RAIN)), ""
        )
        self.assertEqual(feature.weather_kind(), module.WEATHER_RAIN)
        self.assertEqual(feature.season_kind(), 1)
        self.assertEqual(feature.daylight(), 0.75)
        self.assertEqual(feature.sky_color(), (0.2, 0.3, 0.4))
        calls = feature._world_environment.environment.calls
        self.assertEqual([call[0] for call in calls], ["set", "set"])

    def test_unconfirmed_or_invalid_values_clear_without_fabrication(self):
        feature = self.make_feature()
        self.assertEqual(feature.apply_typed_frame(frame(environment_ready=False)), "")
        self.assertEqual(feature.daylight(), 0.0)
        self.assertEqual(feature.sky_color(), (0.0, 0.0, 0.0))
        self.assertEqual(feature._world_environment.environment.calls, [])
        self.assertNotEqual(feature.apply_typed_frame(frame(weather=99)), "")
        self.assertEqual(feature._world_environment.environment.calls, [])

    def test_scene_owns_one_world_environment_resource(self):
        text = SCENE.read_text(encoding="utf-8")
        self.assertIn('type="WorldEnvironment"', text)
        self.assertIn("background_color", text)


if __name__ == "__main__":
    unittest.main()
