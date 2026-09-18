from __future__ import annotations

import importlib.util
import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = ROOT / "apps/mornlea-godot/platform/desktop/input/desktop_input_feature.py"
spec = importlib.util.spec_from_file_location("desktop_input_feature", SOURCE)
assert spec and spec.loader
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FakeSource:
    def __init__(
        self, pressed: set[str] | None = None, delta: tuple[float, float] = (0, 0)
    ):
        self.pressed = pressed or set()
        self.delta = delta

    def is_action_pressed(self, name: str) -> bool:
        return name in self.pressed

    def mouse_delta(self) -> tuple[float, float]:
        return self.delta


class FakeServices:
    def __init__(self) -> None:
        self.submissions: list[tuple[object, ...]] = []

    def has_method(self, name: str) -> bool:
        return name in {"host_protocol_version", "session_submit_semantic"}

    def call(self, name: str, *arguments: object) -> int:
        assert name == "session_submit_semantic"
        self.submissions.append(arguments)
        return 0


class InputContractTests(unittest.TestCase):
    def make_feature(self):
        feature = module.desktop_input_feature.__new__(module.desktop_input_feature)
        feature._ready()
        return feature

    def test_semantic_actions_and_unsupported_actions(self):
        self.assertEqual(
            module.SUPPORTED_ACTIONS - module.UNSUPPORTED_ACTIONS,
            module.SUPPORTED_ACTIONS,
        )
        self.assertEqual(module.LOCAL_ACTIONS, {"f5", "escape"})
        self.assertIn("hotbar", module.UNSUPPORTED_ACTIONS)
        self.assertNotIn("touch", " ".join(module.SUPPORTED_ACTIONS))

    def test_desktop_actions_encode_movement_buttons_and_mouse(self):
        feature = self.make_feature()
        services = FakeServices()
        self.assertEqual(feature.bind_host(services), "")
        self.assertEqual(
            feature.collect_input(
                FakeSource(
                    {"move_forward", "move_right", "jump", "sprint", "primary"}, (4, -2)
                )
            ),
            0,
        )
        self.assertEqual(
            services.submissions[-1], (1, 1, True, True, False, True, False, 0.0, 0.0)
        )

    def test_focus_loss_is_neutral_and_recapture_discards_baseline(self):
        feature = self.make_feature()
        services = FakeServices()
        feature.bind_host(services)
        feature.capture_cursor()
        feature.collect_input(FakeSource(delta=(100, 100)))
        feature.on_focus_changed(False)
        neutral = feature.collect_input(FakeSource({"jump"}, (99, 99)))
        self.assertEqual(neutral, 0)
        self.assertEqual(
            services.submissions[-1],
            (0, 0, False, False, False, False, False, 0.0, 0.0),
        )
        feature.on_focus_changed(True)
        first = feature.collect_input(FakeSource(delta=(9, 9)))
        self.assertEqual(first, 0)
        self.assertEqual(services.submissions[-1][-2:], (0.0, 0.0))

    def test_f5_and_escape_are_local_actions(self):
        feature = self.make_feature()
        services = FakeServices()
        feature.bind_host(services)
        feature.capture_cursor()
        feature.collect_input(FakeSource({"f5"}))
        self.assertTrue(feature.f5_down())
        feature.collect_input(FakeSource({"escape"}, (12, 12)))
        self.assertTrue(feature.escape_down())
        self.assertEqual(
            services.submissions[-1],
            (0, 0, False, False, False, False, False, 0.0, 0.0),
        )


if __name__ == "__main__":
    unittest.main()
