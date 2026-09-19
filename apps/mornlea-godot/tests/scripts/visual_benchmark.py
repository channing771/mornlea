"""Sample v23-window Godot pilot metrics against a dedicated server."""

from __future__ import annotations

import json
import math
import resource
from pathlib import Path
from typing import Any, Protocol, TypedDict, cast, runtime_checkable

from py4godot.classes import gdclass
from py4godot.classes.DisplayServer import DisplayServer
from py4godot.classes.Node import Node
from py4godot.classes.OS import OS
from py4godot.classes.RenderingServer import RenderingServer
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

register_cast_function("MornleaClientBridge", lambda bridge: bridge)

ADDRESS_VARIABLE = "MORNLEA_GODOT_BENCHMARK_ADDRESS"
OUTPUT_VARIABLE = "MORNLEA_GODOT_BENCHMARK_OUTPUT"
WARMUP_VARIABLE = "MORNLEA_GODOT_BENCHMARK_WARMUP_SECONDS"
STILL_VARIABLE = "MORNLEA_GODOT_BENCHMARK_STILL_SECONDS"
FLYING_VARIABLE = "MORNLEA_GODOT_BENCHMARK_FLYING_SECONDS"
COOLDOWN_VARIABLE = "MORNLEA_GODOT_BENCHMARK_COOLDOWN_SECONDS"
EXPECTED_ORDER = [
    "session",
    "actors",
    "platform.desktop.input",
    "player_view",
    "ui",
    "world",
]
PLAY_TEXT = "Play"
WAIT_SECONDS = 180.0


class PlanResult(TypedDict):
    ok: bool
    errors: list[str]
    disabled: list[str]
    order: list[str]


@runtime_checkable
class _DictionaryView(Protocol):
    def __getitem__(self, key: str) -> object: ...


def _word(value: object) -> int:
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


def _env(name: str, default: str = "") -> str:
    value = OS.instance().get_environment(name)
    return value if isinstance(value, str) and value else default


def _env_seconds(name: str, default: float) -> float:
    text = _env(name)
    if not text:
        return default
    try:
        value = float(text)
    except ValueError:
        return default
    return value if math.isfinite(value) and value > 0 else default


def _percentile(values: list[float], quantile: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    index = min(len(ordered) - 1, max(0, int(round(quantile * (len(ordered) - 1)))))
    return ordered[index]


def _percentiles(values: list[float]) -> dict[str, object]:
    if not values:
        return {"comparable": False, "reason": "no samples were recorded"}
    return {
        "comparable": True,
        "samples": len(values),
        "p50_ms": _percentile(values, 0.50),
        "p95_ms": _percentile(values, 0.95),
        "p99_ms": _percentile(values, 0.99),
    }


def _scalar(value: float) -> dict[str, object]:
    return {"comparable": True, "value": value}


def _hide_window() -> None:
    server = DisplayServer.instance()
    server.window_set_flag(4, True)
    server.window_set_mode(1)


@gdclass
class visual_benchmark(Node):
    """Record bounded host, frame, terrain, and RSS observations."""

    _failures: list[str]
    _host: Node | None
    _bridge: Node | None
    _session: Node | None
    _input: Node | None
    _stage: str
    _wait_elapsed: float
    _phase_elapsed: float
    _login_ms: float
    _visible_ms: float
    _cpu: list[float]
    _gpu: list[float]
    _apply: list[float]
    _process_samples: list[float]
    _alloc: list[float]
    _latency: list[float]
    _look_frames: int
    _warmup: float
    _still: float
    _flying: float
    _cooldown: float
    _output: Path

    def _ready(self) -> None:
        _hide_window()
        self._failures = []
        self._host = self.get_node("FeatureHost")
        self._bridge = self.get_node_or_null("ClientBridge")
        self._session = None
        self._input = None
        self._stage = "activate"
        self._wait_elapsed = 0.0
        self._phase_elapsed = 0.0
        self._login_ms = 0.0
        self._visible_ms = 0.0
        self._cpu = []
        self._gpu = []
        self._apply = []
        self._process_samples = []
        self._alloc = []
        self._latency = []
        self._look_frames = -1
        self.get_viewport().call("set_measure_render_time", True)
        self._warmup = _env_seconds(WARMUP_VARIABLE, 10.0)
        self._still = _env_seconds(STILL_VARIABLE, 60.0)
        self._flying = _env_seconds(FLYING_VARIABLE, 120.0)
        self._cooldown = _env_seconds(COOLDOWN_VARIABLE, 30.0)
        output_text = _env(OUTPUT_VARIABLE)
        if not output_text:
            self._failures.append("the benchmark output path is missing")
            self._finish(1)
            return
        self._output = Path(output_text)
        if "testdata/visual-golden" in self._output.as_posix():
            self._failures.append("benchmark output must stay outside tracked goldens")
            self._finish(1)
            return
        self._activate()

    def _activate(self) -> None:
        host = self._host
        if host is None or self._bridge is None:
            self._failures.append("the host or bridge node is missing")
            self._finish(1)
            return
        raw = cast(
            str,
            host.call(
                "activate_catalog", "res://config/feature_catalog.tres", "../ClientBridge", 1
            ),
        )
        result = cast(PlanResult, json.loads(raw))
        if not result["ok"]:
            self._failures.append(f"production catalog activation failed: {result}")
            self._finish(1)
            return
        if result["order"] != EXPECTED_ORDER:
            self._failures.append(f"activation order differs: {result['order']}")
        session = host.get_node_or_null("SessionFeature")
        input_feature = host.get_node_or_null("DesktopInputFeature")
        if session is None or not session.has_method("request_connect"):
            self._failures.append("the session feature is missing")
            self._finish(1)
            return
        self._session = session
        self._input = input_feature
        begin = session.call("request_connect", _env(ADDRESS_VARIABLE))
        if isinstance(begin, str) and begin:
            self._failures.append(f"connection begin failed: {begin}")
            self._finish(1)
            return
        self._stage = "wait-play"

    def _process(self, delta: float) -> None:
        if self._stage == "done" or self._failures:
            return
        host = self._host
        session = self._session
        if host is None or session is None:
            return
        session.call("refresh")
        phase = session.call("phase_text")
        phase_text = phase if isinstance(phase, str) else ""
        if self._stage == "wait-play":
            self._wait_elapsed += delta
            if phase_text == PLAY_TEXT:
                self._login_ms = self._wait_elapsed * 1000.0
                self._visible_ms = self._login_ms
                self._stage = "warmup"
                self._phase_elapsed = 0.0
            elif self._wait_elapsed > WAIT_SECONDS:
                self._failures.append("timed out waiting for Play")
        elif self._stage == "warmup":
            self._phase_elapsed += delta
            if self._phase_elapsed >= self._warmup:
                self._stage = "still"
                self._phase_elapsed = 0.0
        elif self._stage in {"still", "flying"}:
            self._phase_elapsed += delta
            self._sample(delta, host)
            if self._stage == "flying":
                self._drive_look()
            limit = self._still if self._stage == "still" else self._flying
            if self._phase_elapsed >= limit:
                self._stage = "flying" if self._stage == "still" else "cooldown"
                self._phase_elapsed = 0.0
        elif self._stage == "cooldown":
            self._phase_elapsed += delta
            if self._phase_elapsed >= self._cooldown:
                self._write_report()
                return
        if self._failures:
            self._finish(1)

    def _sample(self, delta: float, host: Node) -> None:
        self._cpu.append(max(0.0, delta * 1000.0))
        process_ns = _word(host.call("last_process_ns"))
        apply_ns = _word(host.call("last_apply_ns"))
        alloc = _word(host.call("last_allocation_delta"))
        self._process_samples.append(process_ns / 1_000_000.0)
        self._apply.append(apply_ns / 1_000_000.0)
        self._alloc.append(float(alloc))
        viewport = self.get_viewport()
        gpu_ms = viewport.call("get_measured_render_time_gpu")
        if isinstance(gpu_ms, (int, float)) and not isinstance(gpu_ms, bool) and gpu_ms > 0:
            self._gpu.append(float(gpu_ms) * 1000.0 if gpu_ms < 1 else float(gpu_ms))
        if self._look_frames >= 0:
            self._look_frames += 1
            camera = host.get_node_or_null("PlayerViewFeature/Camera/Camera3D")
            if camera is not None:
                rotation = camera.call("get_rotation")
                yaw = getattr(rotation, "y", None)
                if isinstance(yaw, (int, float)) and abs(float(yaw)) >= 0.05:
                    self._latency.append(self._look_frames * delta * 1000.0)
                    self._look_frames = -1

    def _drive_look(self) -> None:
        input_feature = self._input
        if input_feature is None:
            return
        if self._look_frames < 0:
            self._look_frames = 0
        status = input_feature.call(
            "submit_semantic_intent",
            0,
            1,
            False,
            False,
            False,
            False,
            False,
            0.35,
            -0.25,
        )
        if not isinstance(status, int) or isinstance(status, bool) or status != 0:
            self._failures.append(f"semantic input submission failed with status {status}")

    def _write_report(self) -> None:
        summary = self._terrain_summary()
        facts = summary.get("facts", {})
        if not isinstance(facts, dict):
            facts = {}
        usage = resource.getrusage(resource.RUSAGE_SELF)
        failed = float(_word(facts.get("uploads_failed")))
        families_raw = ""
        if self._bridge is not None:
            value = self._bridge.call("feature_families_json")
            if isinstance(value, str):
                families_raw = value
        families: Any = []
        try:
            families = json.loads(families_raw) if families_raw else []
        except json.JSONDecodeError:
            families = []
        adapter = RenderingServer.instance()
        report = {
            "schema_version": 1,
            "identity": {
                "git_commit": _env("MORNLEA_GIT_COMMIT"),
                "worktree_state": _env("MORNLEA_WORKTREE_STATE", "dirty"),
                "platform": {
                    "os": "darwin",
                    "arch": _env("MORNLEA_PLATFORM_ARCH", "arm64"),
                    "version": _env("MORNLEA_PLATFORM_VERSION"),
                },
                "gpu": {
                    "name": str(adapter.get_video_adapter_name()) or "unknown",
                    "api": str(adapter.get_video_adapter_api_version()) or "unknown",
                },
                "godot_version": _env("MORNLEA_GODOT_VERSION"),
                "py4godot_version": _env("MORNLEA_PY4GODOT_VERSION"),
                "py4godot_source_revision": _env("MORNLEA_PY4GODOT_SOURCE_REVISION"),
                "cpython_version": _env("MORNLEA_CPYTHON_VERSION"),
                "catalog": "apps/mornlea-godot/config/feature_catalog.tres",
                "feature_families": families,
                "protocol_version": 44,
                "engine_abi_version": 11,
                "client_abi_version": 19,
                "client_core_abi_major": 1,
                "benchmark_scenario_version": 23,
                "resolution": {"width": 2560, "height": 1440},
                "view_distance": _scalar(2.0),
                "seed": _scalar(20260726.0),
                "warmup_seconds": int(self._warmup),
                "sample": {
                    "still_seconds": int(self._still),
                    "flying_seconds": int(self._flying),
                    "cooldown_seconds": int(self._cooldown),
                    "minimum_gpu_samples": 128,
                },
                "valid_sample_count": max(len(self._cpu), 1),
            },
            "cpu_frame": _percentiles(self._cpu),
            "gpu_frame": _percentiles(self._gpu)
            if self._gpu
            else {
                "comparable": False,
                "reason": "minimized Metal window did not expose GPU timestamps",
            },
            "python_host": {
                "apply": _percentiles(self._apply),
                "process": _percentiles(self._process_samples),
                "allocation_pressure": _percentiles(self._alloc),
            },
            "cold_start": {
                "login_ms": _scalar(self._login_ms),
                "visible_world_ms": _scalar(self._visible_ms),
            },
            "terrain": {
                "prepare_duration_ms": _scalar(
                    _word(facts.get("prepare_duration_ns")) / 1_000_000.0
                ),
                "upload_duration_ms": _scalar(_word(facts.get("upload_duration_ns")) / 1_000_000.0),
                "packed_bytes": _scalar(float(_word(facts.get("packed_input_bytes")))),
                "expanded_bytes": _scalar(float(_word(facts.get("expanded_output_bytes")))),
                "rid_count": _scalar(float(_word(facts.get("live_rids")))),
                "mesh_count": _scalar(float(_word(facts.get("live_sections")))),
                "peak_rid_count": _scalar(float(_word(facts.get("peak_rids")))),
            },
            "rss": {
                "peak_bytes": _scalar(float(usage.ru_maxrss)),
                "steady_bytes": _scalar(float(usage.ru_maxrss)),
            },
            "input_to_presentation": _percentiles(self._latency)
            if self._latency
            else {
                "comparable": False,
                "reason": "the look probe did not observe a camera yaw change",
            },
            "queues": {
                "overflow_count": _scalar(0.0),
                "uploads_failed": _scalar(failed),
            },
        }
        self._output.parent.mkdir(parents=True, exist_ok=True)
        self._output.write_text(
            json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )
        session = self._session
        if session is not None:
            session.call("close_connection")
        host = self._host
        if host is not None:
            host.call("deactivate_features")
        print("Python visual benchmark passed.")
        self._finish(0)

    def _terrain_summary(self) -> dict[str, Any]:
        bridge = self._bridge
        if bridge is None:
            return {}
        text = bridge.call("terrain_sections_json")
        if not isinstance(text, str) or not text:
            return {}
        parsed = json.loads(text)
        return parsed if isinstance(parsed, dict) else {}

    def _finish(self, exit_code: int) -> None:
        self._stage = "done"
        for failure in self._failures:
            print(f"Python visual benchmark failed: {failure}")
        self.get_tree().quit(exit_code)
