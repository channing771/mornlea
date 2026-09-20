from __future__ import annotations

import importlib.util
import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
SOURCE = ROOT / "apps/mornlea-godot/features/actors/remote_player/remote_player.py"
spec = importlib.util.spec_from_file_location("mornlea_remote_player", SOURCE)
assert spec and spec.loader
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class String:
    """Model the exact Py4Godot core string wrapper seen in typed dictionaries."""

    __module__ = "py4godot.classes.core"

    def __init__(self, value: str) -> None:
        self.value = value

    def __str__(self) -> str:
        return self.value


class InvalidString:
    def __str__(self) -> str:
        return "not-a-player-id"


class FakeSlot:
    def __init__(self) -> None:
        self.calls: list[tuple[str, object]] = []
        self.meta: dict[str, object] = {}

    def call(self, name: str, *args: object) -> None:
        self.calls.append((name, args[0] if len(args) == 1 else args))

    def set_meta(self, name: str, value: object) -> None:
        self.meta[name] = value


def entity(player: int, x: float = 1.0) -> dict[str, object]:
    return {
        "kind": 1,
        "player_id": f"{player:08x}-0000-4000-8000-000000000000",
        "dimension": 0,
        "position_x": x,
        "position_y": 65.0,
        "position_z": -2.0,
        "yaw": 0.25,
        "pitch": -0.125,
    }


def frame(
    entities: list[dict[str, object]],
    *,
    revision: int = 1,
    epoch: int = 1,
    server_tick: int = 1,
) -> dict[str, object]:
    return {
        "status": 0,
        "phase": 4,
        "revision": revision,
        "epoch": epoch,
        "entity_server_tick": server_tick,
        "entities": entities,
    }


class RemotePlayerFeatureTests(unittest.TestCase):
    def make_feature(self):
        feature = module.remote_player.__new__(module.remote_player)
        feature._slots = [FakeSlot() for _ in range(module.MAX_REMOTE_PLAYERS)]
        feature._player_slots = {}
        feature._last_revision = 0
        feature._last_server_tick = 0
        feature._epoch = 0
        feature._last_error = ""
        feature._last_callback_units = 0
        return feature

    def test_spawn_state_despawn_and_callback_budget(self):
        feature = self.make_feature()
        wrapped = entity(1)
        wrapped["player_id"] = String(str(wrapped["player_id"]))
        self.assertEqual(feature.apply_typed_frame(frame([wrapped])), "")
        slot = feature._slots[0]
        self.assertEqual(slot.meta["mornlea_player_id"], entity(1)["player_id"])
        self.assertIn(("set_visible", True), slot.calls)
        self.assertEqual(
            feature.apply_typed_frame(
                frame([entity(1, 3.0)], revision=2, server_tick=2)
            ),
            "",
        )
        self.assertEqual(
            feature.apply_typed_frame(frame([], revision=3, server_tick=3)), ""
        )
        self.assertIn(("set_visible", False), slot.calls)
        self.assertLessEqual(feature.last_callback_units(), module.MAX_REMOTE_PLAYERS)

    def test_player_id_wrapper_is_normalized_only_after_uuid_validation(self):
        wrapped = String("00000001-0000-4000-8000-000000000000")
        self.assertEqual(module._canonical_player_id(wrapped), str(wrapped))
        self.assertIsNone(module._canonical_player_id(InvalidString()))

    def test_duplicate_out_of_order_and_capacity_are_atomic(self):
        feature = self.make_feature()
        self.assertEqual(feature.apply_typed_frame(frame([entity(1)])), "")
        before = dict(feature._player_slots)
        self.assertNotEqual(
            feature.apply_typed_frame(
                frame([entity(1), entity(1)], revision=2, server_tick=2)
            ),
            "",
        )
        self.assertEqual(feature._player_slots, before)
        self.assertNotEqual(
            feature.apply_typed_frame(frame([entity(1)], revision=2, server_tick=0)),
            "",
        )
        self.assertEqual(feature._player_slots, before)
        overflow = [entity(index + 1) for index in range(module.MAX_REMOTE_PLAYERS + 1)]
        self.assertNotEqual(
            feature.apply_typed_frame(frame(overflow, revision=2, server_tick=2)),
            "",
        )
        self.assertEqual(feature._player_slots, before)

    def test_new_epoch_clears_old_slots_before_accepting_lower_tick(self):
        feature = self.make_feature()
        self.assertEqual(
            feature.apply_typed_frame(
                frame([entity(1)], revision=4, epoch=2, server_tick=8)
            ),
            "",
        )
        old_slot = feature._slots[0]
        self.assertEqual(
            feature.apply_typed_frame(
                frame([entity(2)], revision=5, epoch=3, server_tick=1)
            ),
            "",
        )
        self.assertNotIn(entity(1)["player_id"], feature._player_slots)
        self.assertIn(entity(2)["player_id"], feature._player_slots)
        self.assertIn(("set_visible", False), old_slot.calls)

    def test_incomplete_pool_rejects_frame_without_node_mutation(self):
        feature = self.make_feature()
        feature._slots.pop()
        before_calls = [list(slot.calls) for slot in feature._slots]

        self.assertNotEqual(feature.apply_typed_frame(frame([entity(1)])), "")
        self.assertEqual([slot.calls for slot in feature._slots], before_calls)
        self.assertEqual(feature._player_slots, {})

    def test_session_reset_clears_pool_and_callback_state(self):
        feature = self.make_feature()
        self.assertEqual(feature.apply_typed_frame(frame([entity(1)])), "")
        slot = feature._slots[0]
        self.assertEqual(feature.last_callback_units(), 1)

        feature.reset_feature(2)

        self.assertEqual(feature.active_count(), 0)
        self.assertEqual(feature.last_callback_units(), 0)
        self.assertIn(("set_visible", False), slot.calls)


if __name__ == "__main__":
    unittest.main()
