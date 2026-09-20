from __future__ import annotations

import importlib.util
import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = ROOT / "apps/mornlea-godot/features/ui/hud/hud_feature.py"
UI_SOURCE = ROOT / "apps/mornlea-godot/features/ui/ui_feature.py"
SCENE = ROOT / "apps/mornlea-godot/features/ui/hud/hud.tscn"
spec = importlib.util.spec_from_file_location("mornlea_hud_feature", SOURCE)
assert spec and spec.loader
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FakeLabel:
    def __init__(self) -> None:
        self.calls: list[tuple[str, object]] = []

    def call(self, name: str, *args: object) -> None:
        self.calls.append((name, args[0] if len(args) == 1 else args))


def frame(**overrides: object) -> dict[str, object]:
    result: dict[str, object] = {
        "status": 0,
        "phase": 4,
        "hud_ready": True,
        "health": 18,
        "hunger": 16,
        "oxygen": 300,
    }
    result.update(overrides)
    return result


class HudMappingTests(unittest.TestCase):
    def make_feature(self):
        feature = module.hud_feature.__new__(module.hud_feature)
        feature._health_label = FakeLabel()
        feature._hunger_label = FakeLabel()
        feature._oxygen_label = FakeLabel()
        feature._connection_label = FakeLabel()
        feature._pilot_label = FakeLabel()
        feature._active = True
        feature._last_error = ""
        return feature

    def test_confirmed_play_values_are_visible_and_pilot_label_persists(self):
        feature = self.make_feature()
        self.assertEqual(feature.apply_typed_frame(frame()), "")
        self.assertIn(("set_text", "Health: 18/20"), feature._health_label.calls)
        self.assertIn(("set_text", "Hunger: 16/20"), feature._hunger_label.calls)
        self.assertIn(("set_text", "Oxygen: 300/300"), feature._oxygen_label.calls)
        feature._set_text(feature._pilot_label, "Limited Godot pilot")
        self.assertIn(("set_text", "Limited Godot pilot"), feature._pilot_label.calls)

    def test_unconfirmed_values_stay_hidden(self):
        feature = self.make_feature()
        self.assertEqual(feature.apply_typed_frame(frame(hud_ready=False)), "")
        self.assertIn(("set_visible", False), feature._health_label.calls)
        self.assertIn(("set_visible", False), feature._hunger_label.calls)
        self.assertIn(("set_visible", False), feature._oxygen_label.calls)
        self.assertEqual(feature.apply_typed_frame(frame(status=6)), "")

    def test_out_of_domain_confirmed_values_fail_closed(self):
        feature = self.make_feature()
        self.assertNotEqual(feature.apply_typed_frame(frame(health=21)), "")
        self.assertIn(("set_visible", False), feature._health_label.calls)

    def test_scene_declares_required_labels(self):
        text = SCENE.read_text(encoding="utf-8")
        for name in (
            "PilotLabel",
            "ConnectionLabel",
            "HealthLabel",
            "HungerLabel",
            "OxygenLabel",
        ):
            self.assertIn(f'name="{name}"', text)

    def test_ui_coordinator_binds_the_native_bridge_identity(self):
        text = UI_SOURCE.read_text(encoding="utf-8")
        self.assertIn('has_method("feature_families_json")', text)
        self.assertNotIn('has_method("host_protocol_version")', text)


if __name__ == "__main__":
    unittest.main()
