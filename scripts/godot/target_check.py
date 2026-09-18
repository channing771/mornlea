from __future__ import annotations

import importlib.util
import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = (
    ROOT / "apps/mornlea-godot/features/player_view/target_feedback/target_feedback.py"
)
SCENE = (
    ROOT
    / "apps/mornlea-godot/features/player_view/target_feedback/target_feedback.tscn"
)
spec = importlib.util.spec_from_file_location("mornlea_target_feedback", SOURCE)
assert spec and spec.loader
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FakeNode:
    def __init__(self) -> None:
        self.calls: list[tuple[str, object]] = []

    def call(self, name: str, *args: object) -> None:
        self.calls.append((name, args[0] if len(args) == 1 else args))


def visible_frame() -> dict[str, object]:
    return {
        "status": 0,
        "phase": 4,
        "camera_ready": True,
        "target_visible": True,
        "target_x": 4,
        "target_y": 65,
        "target_z": -2,
        "target_name": "砖块",
    }


class TargetFeedbackTests(unittest.TestCase):
    def make_feature(self):
        feature = module.target_feedback.__new__(module.target_feedback)
        feature._bridge = object()
        feature._outline = FakeNode()
        feature._label = FakeNode()
        feature._ui_blocked = False
        feature._visible = False
        feature._last_name = ""
        feature._last_error = ""
        return feature

    def test_visible_target_updates_outline_and_localized_label(self):
        feature = self.make_feature()
        self.assertEqual(feature.apply_typed_frame(visible_frame()), "")
        self.assertTrue(feature.visible())
        self.assertEqual(feature.target_name(), "砖块")
        self.assertEqual(
            feature._label.calls[-2:], [("set_text", "砖块"), ("set_visible", True)]
        )
        self.assertEqual(feature._outline.calls[-1], ("set_visible", True))

    def test_hidden_and_terminal_frames_hide_in_the_same_apply(self):
        feature = self.make_feature()
        self.assertEqual(feature.apply_typed_frame(visible_frame()), "")
        hidden = visible_frame()
        hidden["target_visible"] = False
        hidden["target_x"] = hidden["target_y"] = hidden["target_z"] = 0
        hidden["target_name"] = ""
        self.assertEqual(feature.apply_typed_frame(hidden), "")
        self.assertFalse(feature.visible())
        terminal = visible_frame()
        terminal["phase"] = 5
        self.assertEqual(feature.apply_typed_frame(terminal), "")
        self.assertFalse(feature.visible())

    def test_scene_requires_depth_test_and_disables_depth_writes(self):
        text = SCENE.read_text(encoding="utf-8")
        self.assertIn("no_depth_test = false", text)
        self.assertIn("depth_draw_mode = 2", text)
        self.assertIn('metadata/depth_testing = "enabled"', text)
        self.assertIn('metadata/depth_writes = "disabled"', text)


if __name__ == "__main__":
    unittest.main()
