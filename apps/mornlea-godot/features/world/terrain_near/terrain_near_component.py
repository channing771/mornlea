"""Pinned configuration surface of the near-field terrain compartment.

The compartment is assembled by the coarse world feature scene, never by a
manifest of its own: internal components do not own feature manifests. It
is an addressable Node3D anchor plus the pinned resource paths the world
feature validates at bind time. Packed-quad expansion, vertex buffers, and
rendering-server RIDs are owned by the Rust renderer that the later terrain
tasks attach to this anchor, so this script holds no terrain state, exposes
no geometry, and performs no per-frame work.

The Node3D base is deliberate: the pilot's RID-based meshes carry world-
space vertices, but the compartment still anchors spatial ownership in the
scene tree for the renderer's scenario attachment, and a 2D or plain-node
container would misrepresent the 3D domain it reserves.
"""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node3D import Node3D

# One pinned material per surface class of the decode's three-way split
# (opaque / cutout / water, mirroring design decision 5). Each
# ShaderMaterial references its same-stem shader and deliberately carries no
# texture: the atlas is built from the generated pixel file by the Rust
# renderer and assigned to the shader's atlas uniform at runtime, because
# the raw bytes are not a Godot texture resource and Python must not embed
# one.
OPAQUE_MATERIAL_PATH = "res://features/world/terrain_near/terrain_near_opaque.tres"
CUTOUT_MATERIAL_PATH = "res://features/world/terrain_near/terrain_near_cutout.tres"
WATER_MATERIAL_PATH = "res://features/world/terrain_near/terrain_near_water.tres"
OPAQUE_SHADER_PATH = "res://features/world/terrain_near/terrain_near_opaque.gdshader"
CUTOUT_SHADER_PATH = "res://features/world/terrain_near/terrain_near_cutout.gdshader"
WATER_SHADER_PATH = "res://features/world/terrain_near/terrain_near_water.gdshader"

# The generated atlas identity consumed by the pinned shaders: the manifest
# records the pixel file's layout, and the pixels stay untouched by Python.
# The Rust renderer reads the pixel file itself after the world feature's
# bind-time identity validation has accepted the manifest.
ATLAS_MANIFEST_PATH = "res://assets/generated/manifest.json"
ATLAS_PIXELS_PATH = "res://assets/generated/atlas.rgba8"


@gdclass
class terrain_near_component(Node3D):
    """Anchor the near-field terrain compartment without owning terrain data."""

    def opaque_material_path(self) -> str:
        return OPAQUE_MATERIAL_PATH

    def cutout_material_path(self) -> str:
        return CUTOUT_MATERIAL_PATH

    def water_material_path(self) -> str:
        return WATER_MATERIAL_PATH

    def opaque_shader_path(self) -> str:
        return OPAQUE_SHADER_PATH

    def cutout_shader_path(self) -> str:
        return CUTOUT_SHADER_PATH

    def water_shader_path(self) -> str:
        return WATER_SHADER_PATH

    def atlas_manifest_path(self) -> str:
        return ATLAS_MANIFEST_PATH

    def atlas_pixels_path(self) -> str:
        return ATLAS_PIXELS_PATH
