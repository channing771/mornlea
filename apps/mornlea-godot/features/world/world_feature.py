"""Own the coarse world feature lifecycle while bulk terrain work stays native.

Division of labor (design decision 5 of the Godot client migration): this
Python feature owns bounded lifecycle and configuration only. Packed-quad
expansion, vertex buffers, rendering-server RIDs, and bulk resource
submission belong to the Rust renderer attached to the TerrainNear
compartment, so nothing here fabricates terrain state, expands meshes, or
touches a RID. Per frame this feature makes exactly two typed bridge calls
(world ingestion and one terrain frame drive); every terrain decision,
including the camera derivation and the drop bounding, stays Rust-side.

Configuration is validated once per bind: every pinned material must load,
reference its pinned shader resource, and still carry the pinned
sampler/alpha/depth tokens, and the generated atlas manifest must still
report the identity the pinned shaders consume. Both checks are bounded
(three small resource loads, one bounded manifest file read plus its JSON
parse, and one file-existence probe); every failure returns a stable
English error so the host can disable or fail the feature deterministically
instead of rendering with drifted parameters.
"""

from __future__ import annotations

import json
from typing import Protocol, runtime_checkable

from py4godot.classes import gdclass
from py4godot.classes.ClassDB import ClassDB
from py4godot.classes.Node import Node
from py4godot.classes.Object import Object
from py4godot.classes.ResourceLoader import ResourceLoader
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# The bridge node is acquired through `get_node` plus this identity cast,
# the established acquisition pattern: the pinned runtime's smart-cast table
# has no entry for the project-native bridge class, so without this
# registration every bridge lookup would fail closed with a KeyError.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)

# Structural atlas identity pinned against the generator contract
# (packages/client/cmd/mornlea-godot-assets writing the same layout the wgpu
# renderer consumes: 16x16 RGBA8 layers with 5 precomputed mip levels in
# layer-major, mip-major order). The byte-level currency of the pixels is
# enforced by the asset gate (`make godot-asset-check`) and by the Rust
# loader that reads them; this bind-time read pins only the identity the
# pinned shaders depend on.
ATLAS_MANIFEST_SCHEMA_VERSION = 1
ATLAS_FORMAT = "RGBA8"
ATLAS_LAYER_SIZE = 16
ATLAS_MIP_LEVELS = 5
ATLAS_STORAGE_ORDER = "layer-major,mip-major"
ATLAS_FILE_NAME = "atlas.rgba8"

# Tokens every pinned terrain shader must still declare. A shader that lost
# one of these would silently change the pilot's pinned rendering, so bind
# fails closed instead of activating with drifted parameters. The tokens
# name the sampler hints (nearest filtering with mipmapping, repeat
# addressing), the layered atlas uniform, and the per-frame daylight
# uniform.
SHADER_REQUIRED_TOKENS = (
    "shader_type spatial",
    "sampler2DArray atlas",
    "uniform float daylight",
    "filter_nearest_mipmap",
    "repeat_enable",
    "cull_disabled",
    "varying vec4 terrain_data",
)
# Class-discriminating pins: the cutout class keeps the wgpu terrain pass's
# alpha-scissor discard, the water class keeps its alpha blend and hands the
# untouched texel alpha to the blend stage, and the opaque and cutout
# classes keep depth writing while staying blend-free by never writing
# ALPHA (the Godot spatial equivalent of the wgpu REPLACE blend).
SHADER_CLASS_TOKENS = {
    "opaque": ("depth_draw_always",),
    "cutout": ("depth_draw_always", "discard"),
    "water": ("blend_mix", "depth_draw_never", "ALPHA ="),
}


@runtime_checkable
class _DictionaryView(Protocol):
    """Minimal typed view of the bridge attach result dictionary."""

    def __getitem__(self, key: str) -> object: ...


def _word_of(value: object) -> int:
    # Boolean values are rejected so a marshaling drift cannot masquerade as
    # the status word the activation branches on.
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


@gdclass
class world_feature(Node):
    """Own bounded world configuration; the Rust renderer owns the data plane."""

    _services: Node | None
    _terrain_near: Node | None
    _epoch: int
    _active: bool

    def _ready(self) -> None:
        self._services = None
        self._terrain_near = None
        self._epoch = 0
        self._active = False

    def _process(self, _delta: float) -> None:
        # The bounded per-frame surface: exactly two typed bridge calls,
        # world ingestion and one terrain frame drive. Without an attached
        # renderer or a live producer session both answer the decodable
        # invalid-state word inside the bridge, so the offline frame stays
        # offline-honest (no terrain without a session) at zero Python cost.
        if not self._active:
            return
        bridge = self._services
        if bridge is None:
            return
        bridge.call("terrain_ingest_world")
        bridge.call("terrain_frame")

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "world" else "unexpected world feature ID"

    def bind_host(self, services_path: str) -> str:
        # Same typed-bridge acquisition as the session feature: the pinned
        # runtime cannot marshal project-class objects across script-module
        # boundaries, so the bridge node is re-acquired from its scene path
        # and identified by its family-table method.
        services = self.get_node_or_null(services_path)
        if services is None:
            return "the typed bridge service node is missing"
        if not services.has_method("feature_families_json"):
            return "the typed bridge family table is missing"
        terrain_near = self.get_node_or_null("TerrainNear")
        if terrain_near is None:
            return "the near-field terrain compartment is missing"
        failure = _configuration_failure(terrain_near)
        if failure:
            return failure
        self._services = services
        self._terrain_near = terrain_near
        return ""

    def activate_feature(self, epoch: int) -> str:
        if self._services is None or self._terrain_near is None:
            return "activation before bind"
        terrain_near = self._terrain_near
        result = self._services.call(
            "terrain_attach",
            _call_text(terrain_near, "opaque_material_path"),
            _call_text(terrain_near, "cutout_material_path"),
            _call_text(terrain_near, "water_material_path"),
            _node_path_text(terrain_near),
        )
        if not isinstance(result, _DictionaryView):
            return "the native terrain renderer attach answer was not typed"
        status = _word_of(result["status"])
        if status != 0:
            detail = result["detail"]
            suffix = f": {detail}" if isinstance(detail, str) and detail else ""
            return f"the native terrain renderer could not attach (status {status}){suffix}"
        self._epoch = epoch
        self._active = True
        return ""

    def reset_feature(self, epoch: int) -> None:
        # A host reset advances the epoch only; terrain state reset is owned
        # by the native renderer and rides the producer-session close, and
        # this feature holds no observation to refresh.
        self._epoch = epoch

    def deactivate_feature(self) -> None:
        self._active = False
        self._services = None
        self._terrain_near = None
        self._epoch = 0


def _node_path_text(node: Node) -> str:
    # The runtime's `NodePath` wrapper has no usable text conversion, so the
    # path is rebuilt from its concatenated name components (the host's
    # established helper pattern).
    path = node.get_path()
    names = str(path.get_concatenated_names())
    if path.is_absolute():
        return f"/{names}"
    return names


def _call_text(target: Node, method: str) -> str:
    value = target.call(method)
    return value if isinstance(value, str) else ""


def _configuration_failure(terrain_near: Node) -> str:
    # One bounded pass over the pinned configuration surface: three material
    # loads (each pulling its shader resource) plus one manifest read.
    for surface in ("opaque", "cutout", "water"):
        for accessor, description in (
            (f"{surface}_material_path", "material path"),
            (f"{surface}_shader_path", "shader path"),
        ):
            if not terrain_near.has_method(accessor):
                return f"the {surface} terrain {description} accessor is missing"
            if not _call_text(terrain_near, accessor):
                return f"the {surface} terrain {description} is missing"
        failure = _material_failure(
            _call_text(terrain_near, f"{surface}_material_path"),
            _call_text(terrain_near, f"{surface}_shader_path"),
            surface,
        )
        if failure:
            return failure
    if not terrain_near.has_method("atlas_manifest_path"):
        return "the atlas manifest accessor is missing"
    manifest_path = _call_text(terrain_near, "atlas_manifest_path")
    manifest_text = ClassDB.instance().class_call_static(
        "FileAccess", "get_file_as_string", manifest_path
    )
    if not isinstance(manifest_text, str):
        return "the atlas manifest could not be read"
    failure = _atlas_failure(manifest_text)
    if failure:
        return failure
    if not terrain_near.has_method("atlas_pixels_path"):
        return "the atlas pixels accessor is missing"
    pixels_path = _call_text(terrain_near, "atlas_pixels_path")
    exists = ClassDB.instance().class_call_static("FileAccess", "file_exists", pixels_path)
    if not isinstance(exists, bool) or not exists:
        return "the generated atlas pixels are missing"
    return ""


def _material_failure(material_path: str, shader_path: str, surface: str) -> str:
    material = ResourceLoader.instance().load(material_path)
    if not isinstance(material, Object):
        return f"the {surface} terrain material could not be loaded"
    shader = material.call("get_shader")
    if not isinstance(shader, Object):
        return f"the {surface} terrain material has no shader"
    loaded_shader_path = shader.call("get_path")
    if loaded_shader_path != shader_path:
        return f"the {surface} terrain shader is not the pinned resource"
    code = shader.call("get_code")
    if not isinstance(code, str) or not code:
        return f"the {surface} terrain shader code is unavailable"
    for token in (*SHADER_REQUIRED_TOKENS, *SHADER_CLASS_TOKENS[surface]):
        if token not in code:
            return f"the {surface} terrain shader lost the pinned parameter {token}"
    return ""


def _atlas_failure(manifest_text: str) -> str:
    if not manifest_text:
        return "the atlas manifest is missing"
    try:
        parsed = json.loads(manifest_text)
    except json.JSONDecodeError:
        return "the atlas manifest is not valid JSON"
    if not isinstance(parsed, dict):
        return "the atlas manifest is not an object"
    schema = parsed.get("schema_version")
    if not isinstance(schema, int) or isinstance(schema, bool):
        return "the atlas manifest schema version is missing"
    if schema != ATLAS_MANIFEST_SCHEMA_VERSION:
        return "the atlas manifest schema version is not supported"
    atlas = parsed.get("atlas")
    if not isinstance(atlas, dict):
        return "the atlas manifest has no atlas identity"
    for field, expected, description in (
        ("format", ATLAS_FORMAT, "pixel format"),
        ("storage_order", ATLAS_STORAGE_ORDER, "storage order"),
        ("path", ATLAS_FILE_NAME, "pixel file name"),
    ):
        text = atlas.get(field)
        if not isinstance(text, str) or text != expected:
            return f"the atlas manifest {description} is not {expected}"
    for field, limit, description in (
        ("width", ATLAS_LAYER_SIZE, "layer width"),
        ("height", ATLAS_LAYER_SIZE, "layer height"),
        ("mip_levels", ATLAS_MIP_LEVELS, "mip level count"),
    ):
        number = atlas.get(field)
        if not isinstance(number, int) or isinstance(number, bool) or number != limit:
            return f"the atlas manifest {description} is not {limit}"
    layers = atlas.get("layers")
    if not isinstance(layers, int) or isinstance(layers, bool) or layers < 1:
        return "the atlas manifest reports no atlas layers"
    return ""
