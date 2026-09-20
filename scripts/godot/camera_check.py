from __future__ import annotations

import importlib.util
import math
import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = ROOT / "apps/mornlea-godot/features/player_view/camera/camera_feature.py"
SCENE = ROOT / "apps/mornlea-godot/features/player_view/camera/camera.tscn"
spec = importlib.util.spec_from_file_location("mornlea_camera_feature", SOURCE)
assert spec and spec.loader
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FakeCamera:
    def __init__(self) -> None:
        self.calls: list[tuple[str, object]] = []
        self.meta: dict[str, object] = {}

    def call(self, name: str, *args: object) -> None:
        self.calls.append((name, args[0] if len(args) == 1 else args))

    def set_meta(self, name: str, value: object) -> None:
        self.meta[name] = value


class FakeBridge:
    def __init__(self, frame: dict[str, object]) -> None:
        self.frame = frame
        self.calls = 0

    def call(self, name: str) -> dict[str, object]:
        assert name == "session_frame_typed"
        self.calls += 1
        return self.frame


def valid_frame() -> dict[str, object]:
    return {
        "status": 0,
        "phase": 4,
        "camera_ready": True,
        "position_x": 1.0,
        "position_y": 65.0,
        "position_z": -2.0,
        "yaw": 0.5,
        "pitch": -0.2,
        "fov_y": 1.2,
        "aspect": 16.0 / 9.0,
        "near": 0.1,
        "far": 1000.0,
    }


class CameraMappingTests(unittest.TestCase):
    def make_feature(self, frame: dict[str, object] | None = None):
        feature = module.camera_feature.__new__(module.camera_feature)
        feature._camera = FakeCamera()
        feature._bridge = FakeBridge(frame or valid_frame())
        feature._aspect_override = None
        feature._ui_blocked = False
        feature._ready_state = False
        feature._last_error = ""
        feature._apply_count = 0
        return feature

    def test_maps_pose_projection_and_resize_without_matrix_ownership(self):
        feature = self.make_feature()
        self.assertEqual(feature.resize_aspect(1920, 1080), "")
        self.assertEqual(feature.apply_frame(), "")
        self.assertTrue(feature.camera_ready())
        self.assertEqual(feature._camera.meta["mornlea_aspect"], 16.0 / 9.0)
        calls = dict(feature._camera.calls)
        self.assertEqual(calls["set_position"], (1.0, 65.0, -2.0))
        self.assertEqual(calls["set_rotation"], (0.2, 0.5, 0.0))
        self.assertAlmostEqual(float(calls["set_fov"]), math.degrees(1.2))
        self.assertEqual(calls["set_near"], 0.1)
        self.assertEqual(calls["set_far"], 1000.0)
        self.assertEqual(feature._bridge.calls, 1)

    def test_invalid_domains_and_boundaries_hide_camera(self):
        feature = self.make_feature()
        self.assertNotEqual(feature.resize_aspect(0, 720), "")
        frame = valid_frame()
        frame["pitch"] = math.pi / 2
        self.assertNotEqual(feature.apply_typed_frame(frame), "")
        self.assertFalse(feature.camera_ready())
        frame = valid_frame()
        frame["phase"] = 3
        self.assertEqual(feature.apply_typed_frame(frame), "")
        self.assertFalse(feature.camera_ready())
        feature.set_ui_blocked(True)
        self.assertFalse(feature.camera_ready())

    def test_scene_attaches_the_camera_feature_script(self):
        text = SCENE.read_text(encoding="utf-8")
        self.assertIn('path="res://features/player_view/camera/camera_feature.py"', text)
        self.assertIn('script = ExtResource("1_script")', text)


if __name__ == "__main__":
    unittest.main()
