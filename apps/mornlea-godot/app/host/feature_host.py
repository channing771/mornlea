"""Assemble coarse Godot features without importing project Python packages.

Godot resource paths are the discovery boundary. The embedded interpreter is
deliberately isolated from the repository root, so this host must remain
independent of ambient ``sys.path`` entries and sibling-module imports.
"""

from __future__ import annotations

import json
import sys
import time
from typing import Protocol, cast, runtime_checkable

from py4godot.classes import gdclass
from py4godot.classes.Node import Node
from py4godot.classes.Object import Object
from py4godot.classes.PackedScene import PackedScene
from py4godot.classes.Resource import Resource
from py4godot.classes.ResourceLoader import ResourceLoader
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# The pinned Py4Godot runtime cannot marshal a project-native-class object
# across script-module method boundaries (the callee receives a bare `Object`
# without its pointer), and patching that runtime is out of scope. The host and
# every feature therefore acquire the native bridge node through `get_node`
# with this identity cast, which is the established bridge acquisition pattern;
# only scene paths cross module boundaries.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)

HOST_PROTOCOL_MAJOR = 1
HOST_PROTOCOL_MINOR = 0
BUDGET_CLASSES = frozenset({"bootstrap", "light", "standard", "heavy"})
RESET_POLICIES = frozenset({"recreate", "reset"})


@runtime_checkable
class StringArrayView(Protocol):
    def size(self) -> int: ...

    def get(self, index: int) -> str: ...


@runtime_checkable
class _DictionaryView(Protocol):
    """Minimal typed view used by the host's one-frame fan-out seam."""

    def __getitem__(self, key: str) -> object: ...


class FeatureManifest:
    """Immutable-in-practice projection of one serialized feature resource."""

    def __init__(
        self,
        feature_id: str,
        host_protocol_major: int,
        host_protocol_minor: int,
        entry_scene_path: str,
        dependencies: tuple[str, ...],
        required_bridge_families: tuple[str, ...],
        required: bool,
        enabled: bool,
        budget_class: str,
        reset_policy: str,
    ) -> None:
        self.feature_id = feature_id
        self.host_protocol_major = host_protocol_major
        self.host_protocol_minor = host_protocol_minor
        self.entry_scene_path = entry_scene_path
        self.dependencies = dependencies
        self.required_bridge_families = required_bridge_families
        self.required = required
        self.enabled = enabled
        self.budget_class = budget_class
        self.reset_policy = reset_policy


class PlanResult:
    """Serializable result shared by planning and activation probes."""

    def __init__(
        self,
        ok: bool,
        errors: tuple[str, ...],
        disabled: tuple[str, ...],
        order: tuple[str, ...],
    ) -> None:
        self.ok = ok
        self.errors = errors
        self.disabled = disabled
        self.order = order

    def to_json(self) -> str:
        return json.dumps(
            {
                "ok": self.ok,
                "errors": list(self.errors),
                "disabled": list(self.disabled),
                "order": list(self.order),
            },
            separators=(",", ":"),
            sort_keys=True,
        )


def _allocated_blocks() -> int:
    getter = getattr(sys, "getallocatedblocks", None)
    if callable(getter):
        count = getter()
        if isinstance(count, int) and not isinstance(count, bool) and count >= 0:
            return count
    return 0


@gdclass
class feature_host(Node):
    """Own the bounded feature lifecycle while remaining feature agnostic.

    The host binds the scene-provided native bridge node as its typed service:
    it acquires the object through `get_node` plus the identity cast above,
    negotiates capability from the bridge's typed family table, and hands the
    bridge's scene path to features so each acquires the same object itself.
    Session methods belong to features alone, and no code here branches on
    concrete feature identities.
    """

    _instances: dict[str, Node]
    _active_order: list[str]
    _bridge: Node | None
    _bridge_path: str
    _trace: list[str]
    _last_process_ns: int
    _last_apply_ns: int
    _last_allocation_delta: int

    def _ready(self) -> None:
        self._instances = {}
        self._active_order = []
        self._bridge = None
        self._bridge_path = ""
        self._trace = []
        self._last_process_ns = 0
        self._last_apply_ns = 0
        self._last_allocation_delta = 0

    def _exit_tree(self) -> None:
        _deactivate(self)

    def _process(self, delta: float) -> None:
        """Run one ordered bounded step and fan out one immutable typed frame."""
        started = time.perf_counter_ns()
        allocated_before = _allocated_blocks()
        apply_ns = 0
        try:
            if self._bridge is None:
                return
            for feature_id in self._active_order:
                instance = self._instances.get(feature_id)
                if instance is not None and instance.has_method("drive_input"):
                    instance.call("drive_input")
            elapsed_ns = max(0, min(int(delta * 1_000_000_000), 100_000_000))
            stepped = False
            for feature_id in self._active_order:
                instance = self._instances.get(feature_id)
                if instance is None or not instance.has_method("drive_session"):
                    continue
                status = instance.call("drive_session", elapsed_ns, 64, 32)
                if not isinstance(status, int) or isinstance(status, bool) or status != 0:
                    return
                stepped = True
            if not stepped:
                return
            for feature_id in self._active_order:
                instance = self._instances.get(feature_id)
                if instance is not None and instance.has_method("drive_world"):
                    instance.call("drive_world")
            typed: _DictionaryView | None = None
            for feature_id in self._active_order:
                instance = self._instances.get(feature_id)
                if instance is None or not instance.has_method("pull_typed_frame"):
                    continue
                candidate = instance.call("pull_typed_frame")
                if isinstance(candidate, _DictionaryView):
                    typed = candidate
                    break
            if typed is None:
                return
            apply_started = time.perf_counter_ns()
            for feature_id in self._active_order:
                instance = self._instances.get(feature_id)
                if instance is not None and instance.has_method("apply_typed_frame"):
                    instance.call("apply_typed_frame", typed)
            apply_ns = time.perf_counter_ns() - apply_started
        finally:
            # Recording-only host cost for the P7 report: interpreter apply
            # duration and allocation pressure stay visible instead of hiding
            # inside aggregate frame time. These counters never gate work.
            self._last_process_ns = time.perf_counter_ns() - started
            self._last_apply_ns = apply_ns
            self._last_allocation_delta = max(0, _allocated_blocks() - allocated_before)

    def last_process_ns(self) -> int:
        return self._last_process_ns

    def last_apply_ns(self) -> int:
        return self._last_apply_ns

    def last_allocation_delta(self) -> int:
        return self._last_allocation_delta

    def plan_catalog(self, catalog_path: str, bridge_path: str) -> str:
        bridge = self.get_node(bridge_path)
        if bridge is None:
            return _missing_bridge_result(bridge_path).to_json()
        result, _ = _build_plan(catalog_path, bridge)
        return result.to_json()

    def activate_catalog(self, catalog_path: str, bridge_path: str, epoch: int) -> str:
        bridge = self.get_node(bridge_path)
        if bridge is None:
            return _missing_bridge_result(bridge_path).to_json()
        return _activate(self, catalog_path, bridge, epoch).to_json()

    def reset_features(self, epoch: int) -> None:
        _reset(self, epoch)

    def deactivate_features(self) -> None:
        _deactivate(self)

    def active_count(self) -> int:
        return len(self._active_order)

    def trace_json(self) -> str:
        return json.dumps(self._trace, separators=(",", ":"))


def _missing_bridge_result(bridge_path: str) -> PlanResult:
    return PlanResult(False, (f"bridge node is missing at {bridge_path}",), (), ())


def _metadata(resource: Object, name: str, default: object) -> object:
    if not resource.has_meta(name):
        return default
    return resource.get_meta(name, default)


def _metadata_strings(resource: Object, name: str) -> tuple[str, ...]:
    if not resource.has_meta(name):
        return ()
    value = resource.get_meta(name)
    if isinstance(value, StringArrayView):
        return tuple(value.get(index) for index in range(value.size()))
    if isinstance(value, str):
        return (value,)
    return ()


def _load_resource(path: str) -> Resource | None:
    loaded = ResourceLoader.instance().load(path)
    if loaded is None:
        return None
    return cast(Resource, loaded)


def _load_catalog(catalog_path: str) -> tuple[list[FeatureManifest], list[str]]:
    # Loading explicit resource paths keeps product assembly auditable and bounded.
    errors: list[str] = []
    catalog = _load_resource(catalog_path)
    if catalog is None:
        return [], [f"feature catalog is missing: {catalog_path}"]
    major = cast(int, _metadata(catalog, "host_protocol_major", 0))
    minor = cast(int, _metadata(catalog, "host_protocol_minor", 0))
    if major != HOST_PROTOCOL_MAJOR or minor > HOST_PROTOCOL_MINOR:
        errors.append(f"feature catalog has incompatible host protocol {major}.{minor}")
    manifest_paths = _metadata_strings(catalog, "manifest_paths")
    if not manifest_paths:
        errors.append("feature catalog has no manifest paths")
    manifests: list[FeatureManifest] = []
    for manifest_path in manifest_paths:
        manifest_resource = _load_resource(manifest_path)
        if manifest_resource is None:
            errors.append(f"feature manifest is missing: {manifest_path}")
            continue
        manifests.append(
            FeatureManifest(
                feature_id=cast(str, _metadata(manifest_resource, "feature_id", "")),
                host_protocol_major=cast(
                    int, _metadata(manifest_resource, "host_protocol_major", 0)
                ),
                host_protocol_minor=cast(
                    int, _metadata(manifest_resource, "host_protocol_minor", 0)
                ),
                entry_scene_path=cast(str, _metadata(manifest_resource, "entry_scene_path", "")),
                dependencies=_metadata_strings(manifest_resource, "dependencies"),
                required_bridge_families=_metadata_strings(
                    manifest_resource, "required_bridge_families"
                ),
                required=cast(bool, _metadata(manifest_resource, "required", True)),
                enabled=cast(bool, _metadata(manifest_resource, "enabled", True)),
                budget_class=cast(str, _metadata(manifest_resource, "budget_class", "standard")),
                reset_policy=cast(str, _metadata(manifest_resource, "reset_policy", "reset")),
            )
        )
    return manifests, errors


def _parse_version(value: str) -> tuple[int, int] | None:
    parts = value.split(".", 1)
    if len(parts) != 2 or not all(part.isdigit() for part in parts):
        return None
    return int(parts[0]), int(parts[1])


def _compatible_version(actual: str, required_major: int, required_minor: int) -> bool:
    # Major versions must match; newer compatible minors may satisfy a requirement.
    parsed = _parse_version(actual)
    return parsed is not None and parsed[0] == required_major and parsed[1] >= required_minor


def _call_text(target: Object, method: str, *arguments: object) -> str:
    result = target.call(method, *arguments)
    return result if isinstance(result, str) else ""


class FamilyTable:
    """Parsed projection of the bridge's typed feature-family JSON table.

    The table is keyed by the numeric registry family identifier as a string
    (the bridge reports ``family`` as an integer), so a manifest requirement
    must spell its family part as that numeric string, for example ``"2@1.0"``
    for the connection family. The registry version word is a single contract
    number, so it projects onto the requirement grammar as that major with an
    implicit zero minor.
    """

    def __init__(self, versions: dict[str, tuple[int, int]]) -> None:
        self.versions = versions


def _bridge_family_table(bridge: Node) -> tuple[FamilyTable | None, str]:
    # The bridge identity is its negotiated capability table, reported as one
    # typed JSON value; the host never inspects identity records or packets.
    if not bridge.has_method("feature_families_json"):
        return None, "bridge is missing the feature family table identity"
    table_text = _call_text(bridge, "feature_families_json")
    try:
        entries = json.loads(table_text)
    except json.JSONDecodeError:
        return None, "bridge reported an unreadable feature family table"
    if not isinstance(entries, list):
        return None, "bridge reported an unreadable feature family table"
    versions: dict[str, tuple[int, int]] = {}
    for entry in entries:
        if not isinstance(entry, dict):
            return None, "bridge reported an unreadable feature family table"
        family = entry.get("family")
        version = entry.get("version")
        if (
            not isinstance(family, int)
            or not isinstance(version, int)
            or family <= 0
            or version <= 0
        ):
            return None, "bridge reported an invalid feature family table entry"
        versions[str(family)] = (version, 0)
    return FamilyTable(versions), ""


def _family_incompatibility(table: FamilyTable | None, family_spec: str) -> str:
    family_parts = family_spec.rsplit("@", 1)
    if len(family_parts) != 2:
        return f"has invalid bridge family requirement {family_spec}"
    required = _parse_version(family_parts[1])
    if required is None:
        return f"has invalid bridge family requirement {family_spec}"
    actual = table.versions.get(family_parts[0]) if table is not None else None
    if actual is None:
        return f"requires unavailable bridge family {family_parts[0]}"
    if not _compatible_version(f"{actual[0]}.{actual[1]}", required[0], required[1]):
        return f"requires newer bridge family {family_spec}"
    return ""


def _manifest_incompatibility(manifest: FeatureManifest, table: FamilyTable | None) -> str:
    if not manifest.enabled:
        return "is explicitly disabled"
    if (
        manifest.host_protocol_major != HOST_PROTOCOL_MAJOR
        or manifest.host_protocol_minor > HOST_PROTOCOL_MINOR
    ):
        return (
            "has incompatible host protocol "
            f"{manifest.host_protocol_major}.{manifest.host_protocol_minor}"
        )
    if not manifest.entry_scene_path.startswith("res://"):
        return "has no project-local entry scene"
    if manifest.budget_class not in BUDGET_CLASSES:
        return f"has unknown budget class {manifest.budget_class}"
    if manifest.reset_policy not in RESET_POLICIES:
        return f"has unknown reset policy {manifest.reset_policy}"
    for family_spec in manifest.required_bridge_families:
        incompatibility = _family_incompatibility(table, family_spec)
        if incompatibility:
            return incompatibility
    return ""


def _exclude_or_fail(
    manifest: FeatureManifest,
    reason: str,
    errors: list[str],
    disabled: list[str],
    excluded: set[str],
) -> None:
    if manifest.feature_id in excluded:
        return
    excluded.add(manifest.feature_id)
    if manifest.required:
        errors.append(f"required feature {manifest.feature_id} {reason}")
    else:
        disabled.append(manifest.feature_id)


def _visit_feature(
    feature_id: str,
    manifests: dict[str, FeatureManifest],
    excluded: set[str],
    states: dict[str, int],
    stack: tuple[str, ...],
    order: list[str],
    errors: list[str],
) -> None:
    state = states.get(feature_id, 0)
    if state == 2:
        return
    if state == 1:
        start = stack.index(feature_id)
        cycle = (*stack[start:], feature_id)
        errors.append(f"feature dependency cycle: {' -> '.join(cycle)}")
        return
    states[feature_id] = 1
    next_stack = (*stack, feature_id)
    for dependency in sorted(manifests[feature_id].dependencies):
        if dependency in manifests and dependency not in excluded:
            _visit_feature(
                dependency,
                manifests,
                excluded,
                states,
                next_stack,
                order,
                errors,
            )
    states[feature_id] = 2
    if feature_id not in order:
        order.append(feature_id)


def _build_plan(catalog_path: str, bridge: Node) -> tuple[PlanResult, dict[str, FeatureManifest]]:
    manifests_list, errors = _load_catalog(catalog_path)
    table, bridge_error = _bridge_family_table(bridge)
    if bridge_error:
        errors.append(bridge_error)
    disabled: list[str] = []
    excluded: set[str] = set()
    manifests: dict[str, FeatureManifest] = {}
    # Reject incompatible manifests before traversing dependencies so world state is
    # never partially instantiated from an invalid catalog.
    for manifest in manifests_list:
        if not manifest.feature_id:
            errors.append("feature manifest has an empty ID")
            continue
        if manifest.feature_id in manifests:
            errors.append(f"duplicate feature ID: {manifest.feature_id}")
            continue
        manifests[manifest.feature_id] = manifest
        incompatibility = _manifest_incompatibility(manifest, table)
        if incompatibility:
            _exclude_or_fail(manifest, incompatibility, errors, disabled, excluded)
    changed = True
    # Dependency exclusion is a fixed-point operation: disabling one optional
    # feature may make another feature unavailable on the next pass.
    while changed:
        changed = False
        for feature_id in sorted(manifests):
            if feature_id in excluded:
                continue
            manifest = manifests[feature_id]
            if len(set(manifest.dependencies)) != len(manifest.dependencies):
                _exclude_or_fail(
                    manifest,
                    "has a duplicate dependency",
                    errors,
                    disabled,
                    excluded,
                )
                changed = True
                continue
            unavailable = next(
                (
                    dependency
                    for dependency in manifest.dependencies
                    if dependency not in manifests or dependency in excluded
                ),
                "",
            )
            if unavailable:
                _exclude_or_fail(
                    manifest,
                    f"has missing or disabled dependency {unavailable}",
                    errors,
                    disabled,
                    excluded,
                )
                changed = True
    states: dict[str, int] = {}
    order: list[str] = []
    # Sorted traversal makes activation order independent of resource serialization.
    for feature_id in sorted(manifests):
        if feature_id not in excluded:
            _visit_feature(feature_id, manifests, excluded, states, (), order, errors)
    return (
        PlanResult(
            ok=not errors,
            errors=tuple(errors),
            disabled=tuple(sorted(set(disabled))),
            order=tuple(order),
        ),
        manifests,
    )


def _feature_failure(
    host: feature_host,
    instance: Node,
    manifest: FeatureManifest,
    epoch: int,
) -> str:
    # Lifecycle methods are structural Godot contracts because Py4Godot scripts are
    # loaded as independent resource modules rather than a shared Python package.
    lifecycle = (
        "validate_feature",
        "bind_host",
        "activate_feature",
        "reset_feature",
        "deactivate_feature",
    )
    for method_name in lifecycle:
        if not instance.has_method(method_name):
            return f"is missing lifecycle method {method_name}"
    host._trace.append(f"validate:{manifest.feature_id}")
    failure = _call_text(instance, "validate_feature", manifest.feature_id)
    if failure:
        return f"validate failed: {failure}"
    host._trace.append(f"bind:{manifest.feature_id}")
    # Features receive the bridge's absolute scene path because project-class
    # objects cannot cross script-module method boundaries in the pinned
    # runtime; each feature acquires the typed object through `get_node`.
    failure = _call_text(instance, "bind_host", host._bridge_path)
    if failure:
        return f"bind failed: {failure}"
    host._trace.append(f"activate:{manifest.feature_id}:{epoch}")
    failure = _call_text(instance, "activate_feature", epoch)
    if failure:
        return f"activate failed: {failure}"
    return ""


def _release_instance(instance: Node) -> None:
    instance.queue_free()


def _absolute_path_text(node: Node) -> str:
    # The runtime's `NodePath` wrapper has no usable text conversion, so the
    # path is rebuilt from its concatenated name components.
    path = node.get_path()
    names = str(path.get_concatenated_names())
    if path.is_absolute():
        return f"/{names}"
    return names


def _activate(host: feature_host, catalog_path: str, bridge: Node, epoch: int) -> PlanResult:
    # Each activation begins from a clean host so a failed prior catalog cannot leak
    # instances or bridge state into the next session epoch.
    _deactivate(host)
    host._trace = []
    host._bridge = bridge
    host._bridge_path = _absolute_path_text(bridge)
    plan, manifests = _build_plan(catalog_path, bridge)
    if not plan.ok:
        return plan
    disabled = list(plan.disabled)
    errors: list[str] = []
    for feature_id in plan.order:
        manifest = manifests[feature_id]
        unavailable = next(
            (
                dependency
                for dependency in manifest.dependencies
                if dependency not in host._instances
            ),
            "",
        )
        if unavailable:
            message = (
                f"feature {feature_id} cannot activate after dependency {unavailable} was disabled"
            )
            if manifest.required:
                errors.append(message)
                _deactivate(host)
                return PlanResult(False, tuple(errors), tuple(sorted(disabled)), plan.order)
            disabled.append(feature_id)
            continue
        scene_resource = _load_resource(manifest.entry_scene_path)
        if scene_resource is None:
            failure = "entry scene could not be loaded"
            instance = None
        else:
            scene = cast(PackedScene, scene_resource)
            instance = scene.instantiate()
            failure = "entry scene could not be instantiated" if instance is None else ""
        if instance is not None:
            host._trace.append(f"instantiate:{feature_id}")
            host.add_child(instance)
            failure = _feature_failure(host, instance, manifest, epoch)
        if failure:
            if instance is not None:
                _release_instance(instance)
            if manifest.required:
                # Required failure rolls back already-active dependencies in reverse
                # order; optional failure is isolated to the affected feature.
                errors.append(f"feature {feature_id} {failure}")
                _deactivate(host)
                return PlanResult(False, tuple(errors), tuple(sorted(disabled)), plan.order)
            disabled.append(feature_id)
            continue
        if instance is not None:
            host._instances[feature_id] = instance
            host._active_order.append(feature_id)
    return PlanResult(True, (), tuple(sorted(set(disabled))), plan.order)


def _reset(host: feature_host, epoch: int) -> None:
    # Reset preserves dependency order so providers refresh before their consumers.
    for feature_id in host._active_order:
        host._trace.append(f"reset:{feature_id}:{epoch}")
        host._instances[feature_id].call("reset_feature", epoch)


def _deactivate(host: feature_host) -> None:
    if not hasattr(host, "_active_order"):
        return
    # Consumers deactivate before providers to preserve dependency lifetime safety.
    for feature_id in reversed(host._active_order):
        host._trace.append(f"deactivate:{feature_id}")
        instance = host._instances[feature_id]
        instance.call("deactivate_feature")
        _release_instance(instance)
    host._instances.clear()
    host._active_order.clear()
    host._bridge = None
    host._bridge_path = ""
