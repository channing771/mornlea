//! CPU decode and expansion of the engine's 8-byte packed terrain quad.
//!
//! Ownership: the pilot Godot terrain path (design decision 5 of the Godot
//! client migration). The engine mesher emits one packed `u64` per quad and
//! the wgpu client expands it on the GPU (`terrain.wgsl` / `water.wgsl`);
//! Godot's standard `ArrayMesh` path needs real vertex attributes, so this
//! module performs the same expansion on the CPU inside the GDExtension.
//! The module is pure computation over an owned copy of the pulled bytes:
//! no unsafe code, no Godot objects, no workers and no RIDs, which land with
//! the later terrain tasks.
//!
//! Producer contract: the bit layout is owned by `packages/client/mesh/
//! quad.go` (`Pack`/`UnpackQuad`) and restated bit-for-bit by the engine's
//! `quad.rs` packer; the expansion semantics mirrored here are
//! `terrain.wgsl`, `water.wgsl` (corner heights, UV projection, face
//! shading) and `cull.wgsl` (face normals, plant back-face flags). The
//! pinning tests below parse those sources so a change on any side fails
//! here first.
//!
//! Fail-closed contract: decoding one packed quad rejects, with
//! [`abi::STATUS_INTERNAL`], exactly the patterns the producer contract
//! forbids and Go's own trust boundary (`UnpackQuad`) treats as panics:
//! bit 63 set (the layout's only spare bit and it must stay clear), a
//! plant-set material on an axial face, and a cross-diagonal quad (face
//! 6/7) with nonzero reserved bits 13..19. Through `decode_section` one
//! invalid quad rejects the whole section, mirroring the all-or-nothing
//! presentation batch. Violations that the 4-bit field widths make
//! structurally impossible (x/y/z beyond 15, w/h beyond 16, ao/light beyond
//! 255, material beyond u16) are not checked because no bit pattern can
//! express them. Two tolerances are deliberate: a cross-diagonal quad may
//! carry nonzero bits 55..62 (the producer writes zero there, but Go's
//! boundary ignores those bits for face 6/7, and this decode keeps an
//! identical acceptance domain), and a corner-height quad with corner 2
//! equal to zero is indistinguishable from a merged quad (the format
//! cannot express that rejection; the corner-2 structural discriminator is
//! the shared boundary rule).
//!
//! UV convention: the wgpu convention is pinned here on purpose. v = 0 is
//! the image top row, so side faces and the plant diagonals use -y as the
//! longitudinal component; top and bottom faces project (z, x). Section
//! origins are integer block coordinates and the atlas repeats once per
//! block, so computing UVs from section-local positions samples exactly
//! the same texels as the shader's world-space UVs. Godot's texture
//! coordinate convention differs; flipping is a presentation concern owned
//! by the later Godot material tasks, not this decode.
//!
//! Lighting split: the shader multiplies `face_shade * ao_factor *
//! hemi_factor * base * night_expo`, where `base` and the night gating
//! depend on the per-frame daylight uniform. This decode emits the
//! daylight-independent product plus the raw sky and block light nibbles;
//! completing the blend is the Godot material's job.

// This module is deliberately ahead of its non-test consumers: the terrain
// worker and the RenderingServer upload path land with the later terrain
// tasks, and until then only the tests below reference most items. Allow
// `dead_code` module-wide so the decode does not fail
// `cargo clippy --all-targets -- -D warnings` before those consumers
// exist; remove this allowance once production code consumes the module.
#![allow(dead_code)]

use crate::abi;

/// Bit offset of the section-local X cell coordinate (4 bits).
pub(crate) const SHIFT_X: u32 = 0;
/// Bit offset of the section-local Y cell coordinate (4 bits).
pub(crate) const SHIFT_Y: u32 = 4;
/// Bit offset of the section-local Z cell coordinate (4 bits).
pub(crate) const SHIFT_Z: u32 = 8;
/// Bit offset of the merged-quad width minus one (4 bits) or corner 0's
/// raw height on corner-height quads and the plant back flag's byte.
pub(crate) const SHIFT_W: u32 = 12;
/// Bit offset of the merged-quad height minus one (4 bits) or corner 1's
/// raw height on corner-height quads.
pub(crate) const SHIFT_H: u32 = 16;
/// Bit offset of the 3-bit face field.
pub(crate) const SHIFT_FACE: u32 = 20;
/// Bit offset of the 16-bit atlas material layer, spanning the low word's
/// top 9 bits and the high word's bottom 7 bits.
pub(crate) const SHIFT_MAT: u32 = 23;
/// Bit offset of the packed ambient-occlusion byte (four 2-bit levels).
pub(crate) const SHIFT_AO: u32 = 39;
/// Bit offset of the packed light byte (high nibble sky, low block).
pub(crate) const SHIFT_LIGHT: u32 = 47;
/// Bit offset of corner 2's raw height (4 bits) on corner-height quads.
pub(crate) const SHIFT_CORNER2: u32 = 55;
/// Bit offset of corner 3's raw height (4 bits) on corner-height quads.
pub(crate) const SHIFT_CORNER3: u32 = 59;
/// Bit offset of the plant cross-diagonal front/back flag, borrowed from
/// the always-one width field. Equal to [`SHIFT_W`].
pub(crate) const SHIFT_PLANT_BACK: u32 = SHIFT_W;
/// Reserved bits 13..19 of a cross-diagonal quad: the width/height byte
/// minus the plant back flag. Must be zero; both packers reject it.
pub(crate) const PLANT_RESERVED_MASK: u64 = (0xFF << SHIFT_W) ^ (1 << SHIFT_PLANT_BACK);

/// Wire bytes of one packed quad.
pub(crate) const QUAD_BYTES: usize = 8;

/// First material layer of the contiguous crop (plant) range, pinned
/// against `quad.go` and the engine's `quad.rs` by the source tests below.
pub(crate) const PLANT_MATERIAL_FIRST: u16 = 31;
/// Last material layer of the contiguous crop (plant) range.
pub(crate) const PLANT_MATERIAL_LAST: u16 = 54;
/// Discrete short-grass material layer of the plant set.
pub(crate) const PLANT_MATERIAL_SHORT_GRASS: u16 = 68;
/// Discrete sapling material layer of the plant set.
pub(crate) const PLANT_MATERIAL_SAPLING: u16 = 164;
/// Door material layer. Named by the mesh contract because it is the
/// first non-plant layer after the crop range; its texture is fully
/// opaque, so it classifies as opaque, not cutout.
pub(crate) const DOOR_MATERIAL: u16 = 55;
/// Water material layer. The one layer the producer routes into the
/// translucent water stream; the pilot's combined per-section payload is
/// re-split on exactly this value (see [`stream_of`]).
pub(crate) const WATER_MATERIAL: u16 = 28;
/// Dry-farmland material layer; farmland is a constant-corner short block.
pub(crate) const FARMLAND_MATERIAL_FIRST: u16 = 29;
/// Wet-farmland material layer.
pub(crate) const FARMLAND_MATERIAL_LAST: u16 = 30;
/// Torch material layer; wall-torch slants are corner-height quads.
pub(crate) const TORCH_MATERIAL: u16 = 59;
/// First bed-surface material layer; beds are corner-height short blocks.
pub(crate) const BED_MATERIAL_FIRST: u16 = 60;
/// Last bed-surface material layer.
pub(crate) const BED_MATERIAL_LAST: u16 = 67;
/// Leaves material layer; a cutout-class terrain material.
pub(crate) const CUTOUT_MATERIAL_LEAVES: u16 = 12;
/// Glass material layer; a cutout-class terrain material.
pub(crate) const CUTOUT_MATERIAL_GLASS: u16 = 13;

/// Membership of the plant material set (crops, short grass, sapling).
pub(crate) fn plant_material(material: u16) -> bool {
    (PLANT_MATERIAL_FIRST..=PLANT_MATERIAL_LAST).contains(&material)
        || material == PLANT_MATERIAL_SHORT_GRASS
        || material == PLANT_MATERIAL_SAPLING
}

/// Membership of the corner-height material routing set: the short-block
/// and torch-slab layers the shader routes by material. Routing is
/// completed by the corner-2 structural discriminator, so a short block
/// outside these sets still expands correctly (the bed side boards rely on
/// exactly that catch).
pub(crate) fn corner_height_material(material: u16) -> bool {
    (FARMLAND_MATERIAL_FIRST..=FARMLAND_MATERIAL_LAST).contains(&material)
        || material == TORCH_MATERIAL
        || (BED_MATERIAL_FIRST..=BED_MATERIAL_LAST).contains(&material)
}

/// Membership of the cutout set for terrain-stream quads: binary-alpha
/// layers drawn through the terrain pass's discard. Pinned against the Go
/// asset registry's `isCutoutLayer` (the layers it lists that can appear
/// as quad materials): leaves, glass, the plant set, and torch. Crack,
/// beef, and item-icon layers are also cutout in Go but never appear as
/// quad materials, so they are not repeated here.
pub(crate) fn cutout_material(material: u16) -> bool {
    material == CUTOUT_MATERIAL_LEAVES
        || material == CUTOUT_MATERIAL_GLASS
        || material == TORCH_MATERIAL
        || plant_material(material)
}

/// One of the six axis faces or the two plant cross diagonals, mirroring
/// `mesh.Face` and the engine's `quad.rs` `Face`.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum Face {
    NegX = 0,
    PosX,
    NegY,
    PosY,
    NegZ,
    PosZ,
    /// Cross diagonal from in-cell `(x, z)` to `(x+1, z+1)`.
    PlantDiagA,
    /// Cross diagonal from in-cell `(x+1, z)` to `(x, z+1)`.
    PlantDiagB,
}

impl Face {
    /// Whether this face is one of the two cross diagonals; they carry no
    /// axis semantics.
    pub(crate) fn plant(self) -> bool {
        matches!(self, Face::PlantDiagA | Face::PlantDiagB)
    }

    /// Normal axis of an axial face: 0 = X, 1 = Y, 2 = Z. Only meaningful
    /// for faces 0..=5; `decode_quad` guarantees cross diagonals never
    /// reach axial-only consumers.
    pub(crate) fn axis(self) -> usize {
        (self as usize) >> 1
    }

    /// Whether an axial face's normal points at the positive axis end.
    pub(crate) fn positive(self) -> bool {
        (self as usize & 1) == 1
    }
}

/// The geometry routing of one decoded quad, decided by the same three-way
/// discriminator as `UnpackQuad` and the shader's vertex construction.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum Shape {
    /// Plain greedily merged axis-face quad; width and height in 1..=16.
    Merged { w: u8, h: u8 },
    /// Corner-height quad (fluids, short blocks, torch slants): always
    /// 1x1, carrying four 4-bit raw corner heights. Real height is
    /// `(raw + 1) / 16`; zero means the vertex sits on the block base.
    CornerHeights { corners: [u8; 4] },
    /// In-cell cross diagonal (plants, standing torch): always 1x1; the
    /// back flag flips the normal used for back-face culling while the
    /// geometry stays identical.
    PlantDiagonal { back: bool },
}

/// The typed, validated view of one packed quad. Decode guarantees the
/// face/shape pairing: cross diagonals only appear as
/// [`Shape::PlantDiagonal`] and axial faces only as [`Shape::Merged`] or
/// [`Shape::CornerHeights`].
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) struct DecodedQuad {
    /// Section-local cell coordinates, each 0..=15.
    pub(crate) x: u8,
    pub(crate) y: u8,
    pub(crate) z: u8,
    pub(crate) face: Face,
    /// Atlas material layer; the full u16 domain is legal because the quad
    /// contract does not bound the layer by the atlas size.
    pub(crate) material: u16,
    /// Four 2-bit ambient-occlusion levels, one per corner in corner order.
    pub(crate) ao: u8,
    /// Packed light: high nibble sky, low nibble block.
    pub(crate) light: u8,
    pub(crate) shape: Shape,
}

/// Which producer stream a quad arrived on. The engine-facing client ABI
/// uploads sections as two streams (opaque+cutout and water); the pilot's
/// world family carries one combined payload per section that this module
/// re-splits by material (see [`stream_of`]).
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum QuadStream {
    /// The opaque-plus-cutout stream of the two-stream producer contract.
    Terrain,
    /// The translucent water-surface stream.
    Water,
}

/// The Godot surface a quad belongs to, mirroring design decision 5's
/// separate opaque/cutout/water surfaces.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum QuadClass {
    Opaque,
    Cutout,
    Water,
}

/// One expanded vertex in section-local coordinates. `shade` is the
/// daylight-independent lighting product `face_shade * ao_factor *
/// hemi_factor`; `sky` and `block` are the raw light nibbles the Godot
/// material blends with the per-frame daylight uniform. Normals are unit
/// length: `cull.wgsl` only consumes their sign, so normalizing the
/// diagonal normals preserves its semantics while giving Godot well-formed
/// normal attributes.
#[derive(Clone, Copy, Debug, PartialEq)]
pub(crate) struct ExpandedVertex {
    pub(crate) position: [f32; 3],
    pub(crate) normal: [f32; 3],
    pub(crate) uv: [f32; 2],
    pub(crate) layer: u16,
    pub(crate) ao_factor: f32,
    pub(crate) sky: u8,
    pub(crate) block: u8,
    pub(crate) shade: f32,
}

/// One decoded quad expanded to its four vertices.
#[derive(Clone, Copy, Debug, PartialEq)]
pub(crate) struct ExpandedQuad {
    pub(crate) vertices: [ExpandedVertex; 4],
}

/// Triangle indices of one expanded quad, matching the wgpu client's
/// shared quad index buffer (triangle list, counter-clockwise front face,
/// no face culling in the terrain and water pipelines).
pub(crate) const QUAD_INDICES: [u32; 6] = [0, 1, 2, 0, 2, 3];

/// Corner parameter table shared by the shader family: vertex index `i`
/// maps to local `(u, v)` corner `(CU[i], CV[i])`, which is also the
/// ambient-occlusion and corner-height corner order.
const CU: [f32; 4] = [0.0, 1.0, 1.0, 0.0];
const CV: [f32; 4] = [0.0, 0.0, 1.0, 1.0];

/// Per-face directional shade factor, mirroring `terrain.wgsl`
/// `face_shade`: top 1.00, bottom 0.50, X sides 0.68, Z sides 0.84, and
/// the two cross diagonals both 1.00 so the two plant planes show no seam
/// at their intersection.
pub(crate) fn face_shade(face: Face) -> f32 {
    match face {
        Face::PosY | Face::PlantDiagA | Face::PlantDiagB => 1.00,
        Face::NegY => 0.50,
        Face::NegX | Face::PosX => 0.68,
        Face::NegZ | Face::PosZ => 0.84,
    }
}

/// Hemispheric ambient factor, mirroring `terrain.wgsl` `hemi_factor`:
/// ground color 0.38 below, sky 1.0 above, 0.69 at the sides, and a fixed
/// 0.95 for the orientation-less cross diagonals.
pub(crate) fn hemi_factor(face: Face) -> f32 {
    let ny = match face {
        Face::PosY => 1.0,
        Face::NegY => -1.0,
        Face::NegX | Face::PosX | Face::NegZ | Face::PosZ => 0.0,
        Face::PlantDiagA | Face::PlantDiagB => return 0.95,
    };
    0.38 + (1.0 - 0.38) * (ny * 0.5 + 0.5)
}

/// Decode one packed quad into its typed view, fail-closed on every
/// contract-forbidden bit pattern (see the module docs for the exact
/// rejection and tolerance list).
pub(crate) fn decode_quad(packed: u64) -> Result<DecodedQuad, u32> {
    // The rejection order mirrors `UnpackQuad`: the reserved bit first,
    // then the plant-material placement, then the per-shape reserved
    // bits. Every later read is masked to its field width, so no decode
    // below can observe another field's bits.
    if packed >> 63 != 0 {
        return Err(abi::STATUS_INTERNAL);
    }
    let face = match (packed >> SHIFT_FACE) & 7 {
        0 => Face::NegX,
        1 => Face::PosX,
        2 => Face::NegY,
        3 => Face::PosY,
        4 => Face::NegZ,
        5 => Face::PosZ,
        6 => Face::PlantDiagA,
        _ => Face::PlantDiagB,
    };
    let material = ((packed >> SHIFT_MAT) & 0xFFFF) as u16;
    if !face.plant() && plant_material(material) {
        return Err(abi::STATUS_INTERNAL);
    }
    let shape = if face.plant() {
        // Bits 13..19 are reserved for future plant forms; any nonzero
        // value is an encoding error on both sides of the language
        // boundary. Bits 55..62 are deliberately not inspected: the
        // producer writes zero there, Go's boundary ignores them for
        // cross diagonals, and this decode keeps the same acceptance
        // domain.
        if packed & PLANT_RESERVED_MASK != 0 {
            return Err(abi::STATUS_INTERNAL);
        }
        Shape::PlantDiagonal {
            back: (packed >> SHIFT_PLANT_BACK) & 1 == 1,
        }
    } else {
        let corner2 = ((packed >> SHIFT_CORNER2) & 0xF) as u8;
        if corner2 == 0 {
            Shape::Merged {
                w: ((packed >> SHIFT_W) & 0xF) as u8 + 1,
                h: ((packed >> SHIFT_H) & 0xF) as u8 + 1,
            }
        } else {
            Shape::CornerHeights {
                corners: [
                    ((packed >> SHIFT_W) & 0xF) as u8,
                    ((packed >> SHIFT_H) & 0xF) as u8,
                    corner2,
                    ((packed >> SHIFT_CORNER3) & 0xF) as u8,
                ],
            }
        }
    };
    Ok(DecodedQuad {
        x: (packed & 0xF) as u8,
        y: ((packed >> SHIFT_Y) & 0xF) as u8,
        z: ((packed >> SHIFT_Z) & 0xF) as u8,
        face,
        material,
        ao: ((packed >> SHIFT_AO) & 0xFF) as u8,
        light: ((packed >> SHIFT_LIGHT) & 0xFF) as u8,
        shape,
    })
}

/// Classify one decoded quad for surface routing. Water is decided by
/// stream origin alone, mirroring the two-stream producer contract;
/// terrain-stream quads split into cutout by the material set and opaque
/// otherwise. Stream purity itself is a producer invariant that Go never
/// re-validates on upload, so a water-layer quad handed to the terrain
/// stream classifies as opaque rather than being rejected.
pub(crate) fn classify(quad: &DecodedQuad, stream: QuadStream) -> QuadClass {
    match stream {
        QuadStream::Water => QuadClass::Water,
        QuadStream::Terrain => {
            if cutout_material(quad.material) {
                QuadClass::Cutout
            } else {
                QuadClass::Opaque
            }
        }
    }
}

/// Peek which producer stream one packed quad belongs to by material
/// alone. The old client ABI uploads sections as two upload streams
/// before the GPU; the pilot's world family carries one combined payload
/// per section, so the terrain worker re-runs the Go render scheduler's
/// exact split (`q.Mat == assets.LayerWater`) here before classifying.
/// This is a routing peek, not validation: it performs none of
/// `decode_quad`'s rejection work.
pub(crate) fn stream_of(packed: u64) -> QuadStream {
    if ((packed >> SHIFT_MAT) & 0xFFFF) as u16 == WATER_MATERIAL {
        QuadStream::Water
    } else {
        QuadStream::Terrain
    }
}

/// The unit vector of one axis: 0 = X, 1 = Y, 2 = Z, mirroring the
/// shaders' `axis_vec`.
fn axis_vec(axis: usize) -> [f32; 3] {
    match axis {
        0 => [1.0, 0.0, 0.0],
        1 => [0.0, 1.0, 0.0],
        _ => [0.0, 0.0, 1.0],
    }
}

/// Axial-face vertex position: the cell corner offset by the positive-face
/// step, plus the U and V corner parameters scaled by the quad's extent
/// (1x1 for corner-height quads, `w`/`h` for merged quads).
fn axial_position(face: Face, base: [f32; 3], u: f32, v: f32) -> [f32; 3] {
    let axis = face.axis();
    let ua = (axis + 1) % 3;
    let va = (axis + 2) % 3;
    let positive = f32::from(u8::from(face.positive()));
    let mut position = base;
    position[axis] += positive;
    position[ua] += u;
    position[va] += v;
    position
}

/// Cross-diagonal vertex position: U is the horizontal walk along the
/// diagonal and V the vertical rise; diagonal B mirrors the horizontal
/// walk so the plane spans (x+1, z) to (x, z+1).
fn diagonal_position(face: Face, base: [f32; 3], corner: usize) -> [f32; 3] {
    let s = CU[corner];
    let mut position = base;
    position[0] = if face == Face::PlantDiagB {
        base[0] + 1.0 - s
    } else {
        base[0] + s
    };
    position[1] += CV[corner];
    position[2] += s;
    position
}

/// World-space UV of one final vertex position under the wgpu convention:
/// X faces project (z, -y), Y faces (z, x), Z faces (x, -y), and the cross
/// diagonals pin (x, -y) explicitly because their face field carries no
/// axis semantics.
fn face_uv(face: Face, position: [f32; 3]) -> [f32; 2] {
    match face {
        Face::NegX | Face::PosX => [position[2], -position[1]],
        Face::NegY | Face::PosY => [position[2], position[0]],
        Face::NegZ | Face::PosZ => [position[0], -position[1]],
        Face::PlantDiagA | Face::PlantDiagB => [position[0], -position[1]],
    }
}

/// Unit face normal mirroring `cull.wgsl`: axial faces take their axis
/// vector negated on the negative end; the diagonals take the cull pass's
/// raw directions scaled to unit length, negated on the back face. The
/// cull pass consumes only the normal's sign, so scaling preserves it.
fn face_normal(face: Face, back: bool) -> [f32; 3] {
    let unit = std::f32::consts::FRAC_1_SQRT_2;
    let sign = if back { -1.0 } else { 1.0 };
    match face {
        Face::NegX => [-1.0, 0.0, 0.0],
        Face::PosX => [1.0, 0.0, 0.0],
        Face::NegY => [0.0, -1.0, 0.0],
        Face::PosY => [0.0, 1.0, 0.0],
        Face::NegZ => [0.0, 0.0, -1.0],
        Face::PosZ => [0.0, 0.0, 1.0],
        Face::PlantDiagA => [unit * sign, 0.0, -unit * sign],
        Face::PlantDiagB => [unit * sign, 0.0, unit * sign],
    }
}

/// Expand one decoded quad into four section-local vertices, mirroring
/// `terrain.wgsl` and `water.wgsl` vertex construction plus `cull.wgsl`
/// normals; see the module docs for the UV and lighting conventions.
pub(crate) fn expand(quad: &DecodedQuad) -> ExpandedQuad {
    let base = [f32::from(quad.x), f32::from(quad.y), f32::from(quad.z)];
    let shade_base = face_shade(quad.face) * hemi_factor(quad.face);
    let back = matches!(quad.shape, Shape::PlantDiagonal { back: true });
    let mut vertices = [zero_vertex(); 4];
    for (corner, vertex) in vertices.iter_mut().enumerate() {
        let (mut position, raw) = match quad.shape {
            Shape::Merged { w, h } => (
                axial_position(
                    quad.face,
                    base,
                    CU[corner] * f32::from(w),
                    CV[corner] * f32::from(h),
                ),
                0,
            ),
            Shape::CornerHeights { corners } => (
                axial_position(quad.face, base, CU[corner], CV[corner]),
                corners[corner],
            ),
            Shape::PlantDiagonal { .. } => (diagonal_position(quad.face, base, corner), 0),
        };
        // Corner-height lift: a nonzero raw height places the vertex on
        // the cell's top layer at (raw + 1) / 16; zero leaves the
        // axis-face position, which is the block base for base corners.
        if raw != 0 {
            position[1] = f32::from(quad.y) + (f32::from(raw) + 1.0) / 16.0;
        }
        let ao_level = f32::from((quad.ao >> (corner * 2)) & 0x3);
        let ao_factor = 0.55 + 0.45 * (ao_level / 3.0);
        *vertex = ExpandedVertex {
            uv: face_uv(quad.face, position),
            normal: face_normal(quad.face, back),
            position,
            layer: quad.material,
            ao_factor,
            sky: (quad.light >> 4) & 0xF,
            block: quad.light & 0xF,
            shade: shade_base * ao_factor,
        };
    }
    ExpandedQuad { vertices }
}

/// The expanded geometry of one surface class of one section.
#[derive(Default, Debug, PartialEq)]
pub(crate) struct SurfaceGeometry {
    pub(crate) vertices: Vec<ExpandedVertex>,
    pub(crate) indices: Vec<u32>,
}

/// The three-surface expansion of one section mesh payload.
#[derive(Default, Debug, PartialEq)]
pub(crate) struct SectionGeometry {
    pub(crate) opaque: SurfaceGeometry,
    pub(crate) cutout: SurfaceGeometry,
    pub(crate) water: SurfaceGeometry,
}

/// Decode and expand one section payload from one explicit producer
/// stream. Fail-closed: an empty payload (the presentation contract
/// requires at least one quad per upsert), a payload above the frozen
/// per-section quad limit, or any single invalid quad rejects the whole
/// section without producing partial geometry.
pub(crate) fn decode_section(stream: QuadStream, quads: &[u64]) -> Result<SectionGeometry, u32> {
    decode_split(quads, |_| stream)
}

/// Decode and expand one section payload of the pilot's combined world
/// family layout, splitting each quad to its producer stream with
/// [`stream_of`] before classification. This is the entry point the
/// terrain worker will use for the pulled per-section payloads.
pub(crate) fn decode_pilot_section(quads: &[u64]) -> Result<SectionGeometry, u32> {
    decode_split(quads, stream_of)
}

/// Shared per-section decode loop: every quad is decoded fail-closed,
/// classified through the caller's stream router, and expanded into the
/// matching surface. Indices restart per surface, so each surface is an
/// independent Godot mesh; the whole section decodes or none of it does.
fn decode_split(quads: &[u64], route: impl Fn(u64) -> QuadStream) -> Result<SectionGeometry, u32> {
    if quads.is_empty() {
        return Err(abi::STATUS_INTERNAL);
    }
    if quads.len() > abi::MAX_SECTION_MESH_QUADS as usize {
        return Err(abi::STATUS_INTERNAL);
    }
    let mut geometry = SectionGeometry::default();
    for &packed in quads {
        let quad = decode_quad(packed)?;
        let expanded = expand(&quad);
        let surface = match classify(&quad, route(packed)) {
            QuadClass::Opaque => &mut geometry.opaque,
            QuadClass::Cutout => &mut geometry.cutout,
            QuadClass::Water => &mut geometry.water,
        };
        let base = surface.vertices.len() as u32;
        surface.vertices.extend(expanded.vertices);
        surface
            .indices
            .extend(QUAD_INDICES.map(|index| index + base));
    }
    Ok(geometry)
}

/// Check one world batch's operation and packed-quad totals against the
/// frozen presentation limits before any per-section decode spends work:
/// at least one and at most [`abi::MAX_WORLD_BATCH_OPERATIONS`] sections,
/// and at most [`abi::MAX_WORLD_BATCH_QUADS`] packed quads in total.
pub(crate) fn check_world_batch(sections: usize, packed_quads: usize) -> Result<(), u32> {
    if sections == 0 || sections > abi::MAX_WORLD_BATCH_OPERATIONS as usize {
        return Err(abi::STATUS_INTERNAL);
    }
    if packed_quads > abi::MAX_WORLD_BATCH_QUADS as usize {
        return Err(abi::STATUS_INTERNAL);
    }
    Ok(())
}

/// The all-zero vertex used to initialize `expand`'s output array before
/// every slot is overwritten in the corner loop.
fn zero_vertex() -> ExpandedVertex {
    ExpandedVertex {
        position: [0.0; 3],
        normal: [0.0; 3],
        uv: [0.0; 2],
        layer: 0,
        ao_factor: 0.0,
        sky: 0,
        block: 0,
        shade: 0.0,
    }
}

/// Build one packed quad bit-exactly, mirroring `mesh.Quad.Pack` and the
/// engine's `Quad::pack`. Test-shared fixture in the `test_status_record`
/// style: the decode tests below construct raw words through it so the
/// bit-layout knowledge is written once, independent of the decode under
/// test.
#[cfg(test)]
#[derive(Clone, Copy)]
struct TestQuad {
    x: u8,
    y: u8,
    z: u8,
    w: u8,
    h: u8,
    corners: [u8; 4],
    back: bool,
    face: u8,
    material: u16,
    ao: u8,
    light: u8,
}

#[cfg(test)]
impl Default for TestQuad {
    fn default() -> Self {
        Self {
            x: 0,
            y: 0,
            z: 0,
            w: 1,
            h: 1,
            corners: [0; 4],
            back: false,
            face: 3,
            material: 0,
            ao: 0,
            light: 0,
        }
    }
}

#[cfg(test)]
impl TestQuad {
    /// Pack with the producer's exact three-way layout: cross diagonals
    /// borrow bits 12..19 for the front/back flag, corner-height quads for
    /// corners 0 and 1, and merged quads store `w-1`/`h-1`.
    fn pack(self) -> u64 {
        let (low, high) = if self.face >= 6 {
            (u64::from(self.back) << SHIFT_PLANT_BACK, 0)
        } else if self.corners == [0; 4] {
            (
                u64::from(self.w - 1) << SHIFT_W | u64::from(self.h - 1) << SHIFT_H,
                0,
            )
        } else {
            (
                u64::from(self.corners[0]) << SHIFT_W | u64::from(self.corners[1]) << SHIFT_H,
                u64::from(self.corners[2]) << SHIFT_CORNER2
                    | u64::from(self.corners[3]) << SHIFT_CORNER3,
            )
        };
        u64::from(self.x) << SHIFT_X
            | u64::from(self.y) << SHIFT_Y
            | u64::from(self.z) << SHIFT_Z
            | low
            | u64::from(self.face) << SHIFT_FACE
            | u64::from(self.material) << SHIFT_MAT
            | u64::from(self.ao) << SHIFT_AO
            | u64::from(self.light) << SHIFT_LIGHT
            | high
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BED_MATERIAL_FIRST, BED_MATERIAL_LAST, CUTOUT_MATERIAL_GLASS, CUTOUT_MATERIAL_LEAVES,
        DOOR_MATERIAL, DecodedQuad, FARMLAND_MATERIAL_FIRST, FARMLAND_MATERIAL_LAST, Face,
        PLANT_MATERIAL_FIRST, PLANT_MATERIAL_LAST, PLANT_MATERIAL_SAPLING,
        PLANT_MATERIAL_SHORT_GRASS, PLANT_RESERVED_MASK, QUAD_BYTES, QUAD_INDICES, QuadClass,
        QuadStream, SHIFT_AO, SHIFT_CORNER2, SHIFT_CORNER3, SHIFT_FACE, SHIFT_H, SHIFT_LIGHT,
        SHIFT_MAT, SHIFT_W, SHIFT_X, SHIFT_Y, SHIFT_Z, Shape, TORCH_MATERIAL, TestQuad,
        WATER_MATERIAL, check_world_batch, classify, corner_height_material, cutout_material,
        decode_pilot_section, decode_quad, decode_section, expand, face_shade, hemi_factor,
        plant_material, stream_of,
    };
    use crate::abi;

    const QUAD_GO: &str = include_str!("../../../../client/mesh/quad.go");
    const ASSETS_GO: &str = include_str!("../../../../client/assets/blocks.go");
    const POS_GO: &str = include_str!("../../../../shared/core/pos.go");
    const PRESENTATION_GO: &str = include_str!("../../../../client/presentation/world.go");
    const CORE_WORLD_GO: &str = include_str!("../../../../client/cmd/mornlea-godot-core/world.go");
    const SCHEDULER_GO: &str = include_str!("../../../../client/render/section_scheduler.go");
    const TERRAIN_WGSL: &str = include_str!("../../mornlea_client/shaders/terrain.wgsl");
    const WATER_WGSL: &str = include_str!("../../mornlea_client/shaders/water.wgsl");
    const CULL_WGSL: &str = include_str!("../../mornlea_client/shaders/cull.wgsl");
    const RENDER_MOD_RS: &str = include_str!("../../mornlea_client/src/render/mod.rs");

    /// Parse one Go const declaration whose value is a decimal literal,
    /// typed (`Name uint16 = 31`) or untyped (`shiftX = 0`). Mirrors the
    /// `status_decode` source-parsing pattern.
    fn go_const(source: &str, name: &str) -> u32 {
        for raw in source.lines() {
            let Some((head, tail)) = raw.split_once('=') else {
                continue;
            };
            let mut parts = head.trim().trim_start_matches("const").split_whitespace();
            if parts.next() != Some(name) {
                continue;
            }
            let digits: String = tail
                .trim()
                .chars()
                .take_while(|character| character.is_ascii_digit())
                .collect();
            assert!(!digits.is_empty(), "Go const {name} carries no literal");
            return digits.parse().expect("decimal Go const value");
        }
        panic!("Go const {name} was not found");
    }

    /// Look up one material layer's value in the Go asset registry's
    /// layer enumeration. The enumeration is a plain `iota` ladder from
    /// `LayerStone`; every frozen terrain layer name is a bare member
    /// before the first explicitly valued line, so the parser covers
    /// exactly that prefix and stops at the first `=` line.
    fn go_layer(source: &str, name: &str) -> u16 {
        let mut value: u16 = 1;
        let mut in_enum = false;
        for raw in source.lines() {
            let line = raw.trim();
            if !in_enum {
                if line.starts_with("LayerStone uint16 = iota") {
                    // The seed line itself consumes iota value 0.
                    in_enum = true;
                    if name == "LayerStone" {
                        return 0;
                    }
                }
                continue;
            }
            if line == ")" {
                break;
            }
            if line.is_empty() || line.starts_with("//") {
                continue;
            }
            if line.contains('=') {
                // Explicitly valued members end the plain-iota prefix this
                // parser can evaluate.
                break;
            }
            let member = line.split_whitespace().next().expect("layer member");
            if member == name {
                return value;
            }
            value += 1;
        }
        panic!("layer {name} is not a plain member of the enumeration");
    }

    /// Collapse runs of whitespace so `contains` pins survive formatting.
    fn normalized(source: &str) -> String {
        source.split_whitespace().collect::<Vec<_>>().join(" ")
    }

    fn approx(got: f32, want: f32) {
        assert!(
            (got - want).abs() < 1e-6,
            "value {got:.9} is not within 1e-6 of {want:.9}"
        );
    }

    /// Hand-computed per-corner position and UV tables for the geometry
    /// pinning tests.
    type PositionTable = [[f32; 3]; 4];
    type UvTable = [[f32; 2]; 4];

    fn decoded(test: TestQuad) -> DecodedQuad {
        decode_quad(test.pack()).expect("fixture quad must decode")
    }

    // ------------------------------------------------------------------
    // Source pins: bit layout, material sets, limits, and semantics.
    // ------------------------------------------------------------------

    #[test]
    fn quad_decode_pins_the_bit_layout_against_quad_go() {
        for (name, value) in [
            ("shiftX", SHIFT_X),
            ("shiftY", SHIFT_Y),
            ("shiftZ", SHIFT_Z),
            ("shiftW", SHIFT_W),
            ("shiftH", SHIFT_H),
            ("shiftFace", SHIFT_FACE),
            ("shiftMat", SHIFT_MAT),
            ("shiftAO", SHIFT_AO),
            ("shiftLight", SHIFT_LIGHT),
            ("shiftCorner2", SHIFT_CORNER2),
            ("shiftCorner3", SHIFT_CORNER3),
        ] {
            assert_eq!(go_const(QUAD_GO, name), value, "bit offset {name}");
        }
        // The plant back flag borrows the width byte, and the reserved
        // mask is exactly the width byte minus that one bit.
        let quad_go = normalized(QUAD_GO);
        assert!(quad_go.contains("shiftPlantBack = shiftW"));
        assert!(quad_go.contains("plantReservedMask = 0xFF<<shiftW ^ 1<<shiftPlantBack"));
        assert_eq!(PLANT_RESERVED_MASK, 0xFF << SHIFT_W ^ 1 << SHIFT_W);
        // The packed element stays eight bytes on the wire.
        assert_eq!(QUAD_BYTES, 8);
        assert!(normalized(CORE_WORLD_GO).contains("packedQuads*8"));
    }

    #[test]
    fn quad_decode_pins_the_plant_and_door_sets_against_quad_go() {
        for (name, value) in [
            ("PlantMaterialFirst", u32::from(PLANT_MATERIAL_FIRST)),
            ("PlantMaterialLast", u32::from(PLANT_MATERIAL_LAST)),
            (
                "PlantMaterialShortGrass",
                u32::from(PLANT_MATERIAL_SHORT_GRASS),
            ),
            ("PlantMaterialSapling", u32::from(PLANT_MATERIAL_SAPLING)),
            ("DoorMaterial", u32::from(DOOR_MATERIAL)),
        ] {
            assert_eq!(go_const(QUAD_GO, name), value, "material set {name}");
        }
        // The set is discontinuous: the crop range, then two discrete
        // points after the door, workbench, torch, and bed layers.
        assert!(plant_material(PLANT_MATERIAL_FIRST));
        assert!(plant_material(PLANT_MATERIAL_LAST));
        assert!(!plant_material(PLANT_MATERIAL_FIRST - 1));
        assert!(!plant_material(PLANT_MATERIAL_LAST + 1));
        assert!(plant_material(PLANT_MATERIAL_SHORT_GRASS));
        assert!(plant_material(PLANT_MATERIAL_SAPLING));
        assert!(!plant_material(PLANT_MATERIAL_SAPLING - 1));
        assert!(!plant_material(PLANT_MATERIAL_SAPLING + 1));
    }

    #[test]
    fn quad_decode_pins_layer_numbers_against_the_go_asset_registry() {
        for (name, value) in [
            ("LayerLeaves", u32::from(CUTOUT_MATERIAL_LEAVES)),
            ("LayerGlass", u32::from(CUTOUT_MATERIAL_GLASS)),
            ("LayerWater", u32::from(WATER_MATERIAL)),
            ("LayerFarmlandDry", u32::from(FARMLAND_MATERIAL_FIRST)),
            ("LayerFarmlandWet", u32::from(FARMLAND_MATERIAL_LAST)),
            ("LayerDoor", u32::from(DOOR_MATERIAL)),
            ("LayerTorch", u32::from(TORCH_MATERIAL)),
            ("LayerBedFootSouth", u32::from(BED_MATERIAL_FIRST)),
            ("LayerBedHeadEast", u32::from(BED_MATERIAL_LAST)),
            ("LayerShortGrass", u32::from(PLANT_MATERIAL_SHORT_GRASS)),
        ] {
            assert_eq!(u32::from(go_layer(ASSETS_GO, name)), value, "layer {name}");
        }
    }

    #[test]
    fn quad_decode_pins_the_shader_material_gates_against_terrain_wgsl() {
        assert!(TERRAIN_WGSL.contains("return mat >= 29u && mat <= 30u;"));
        assert!(TERRAIN_WGSL.contains("return mat == 59u;"));
        assert!(TERRAIN_WGSL.contains("return mat >= 60u && mat <= 67u;"));
        assert_eq!(FARMLAND_MATERIAL_FIRST, 29);
        assert_eq!(FARMLAND_MATERIAL_LAST, 30);
        assert_eq!(TORCH_MATERIAL, 59);
        assert_eq!(BED_MATERIAL_FIRST, 60);
        assert_eq!(BED_MATERIAL_LAST, 67);
        // The material routing vocabulary mirrors the shader's gates;
        // routing correctness itself is carried by the decode-time
        // corner-2 discriminator, so these sets exist to alarm on
        // cross-language layer drift, exactly as in the shader.
        assert!(corner_height_material(FARMLAND_MATERIAL_FIRST));
        assert!(corner_height_material(FARMLAND_MATERIAL_LAST));
        assert!(corner_height_material(TORCH_MATERIAL));
        assert!(corner_height_material(BED_MATERIAL_FIRST));
        assert!(corner_height_material(BED_MATERIAL_LAST));
        assert!(!corner_height_material(0));
        assert!(!corner_height_material(DOOR_MATERIAL));
        // The routing set is completed by the structural discriminator.
        assert!(TERRAIN_WGSL.contains("|| corner_height(lo, hi, 2u) != 0u) {"));
        // The expansion semantics mirrored below, pinned in place.
        assert!(TERRAIN_WGSL.contains("local = vec3f(px, y + cv[vi], z + s);"));
        assert!(TERRAIN_WGSL.contains("px = x + 1.0 - s;"));
        assert!(TERRAIN_WGSL.contains("uv = vec2f(world.x, -world.y);"));
        for shader in [TERRAIN_WGSL, WATER_WGSL] {
            assert!(shader.contains("local.y = y + (f32(raw) + 1.0) / 16.0;"));
            assert!(shader.contains("if (axis == 0u) { return vec2f(world.z, -world.y); }"));
            assert!(shader.contains("let ao_factor = 0.55 + 0.45 * (ao_level / 3.0);"));
            assert!(shader.contains("let sky = f32((light >> 4u) & 0xFu) / 15.0;"));
            assert!(shader.contains("let block = f32(light & 0xFu) / 15.0;"));
        }
    }

    #[test]
    fn quad_decode_pins_shading_factors_against_the_shaders() {
        assert!(TERRAIN_WGSL.contains("case 3u: { return 1.00; }"));
        assert!(TERRAIN_WGSL.contains("case 2u: { return 0.50; }"));
        assert!(TERRAIN_WGSL.contains("case 0u, 1u: { return 0.68; }"));
        assert!(TERRAIN_WGSL.contains("case 6u, 7u: { return 1.00; }"));
        assert!(TERRAIN_WGSL.contains("default: { return 0.84; }"));
        assert!(TERRAIN_WGSL.contains("if (face >= 6u) { return 0.95; }"));
        assert!(TERRAIN_WGSL.contains("return mix(0.38, 1.0, ny * 0.5 + 0.5);"));
        for (face, shade, hemi) in [
            (Face::NegX, 0.68, 0.69),
            (Face::PosX, 0.68, 0.69),
            (Face::NegY, 0.50, 0.38),
            (Face::PosY, 1.00, 1.00),
            (Face::NegZ, 0.84, 0.69),
            (Face::PosZ, 0.84, 0.69),
            (Face::PlantDiagA, 1.00, 0.95),
            (Face::PlantDiagB, 1.00, 0.95),
        ] {
            approx(face_shade(face), shade);
            approx(hemi_factor(face), hemi);
        }
    }

    #[test]
    fn quad_decode_pins_the_cutout_set_against_the_go_registry() {
        // The Go registry owns the cutout classification for terrain
        // materials: exactly the layers `isCutoutLayer` lists that can
        // appear as quad materials are cutout here. The door layer is the
        // documented counter-example: it is named by the mesh contract but
        // its texture is fully opaque, so Go does not list it and this
        // decode classifies it as opaque.
        let assets = normalized(ASSETS_GO);
        for member in [
            "int(LayerLeaves)",
            "int(LayerGlass)",
            "int(LayerWheat0)",
            "int(LayerCarrot7)",
            "int(LayerTorch)",
            "int(LayerShortGrass)",
            "int(LayerSapling)",
        ] {
            assert!(assets.contains(member), "cutout set is missing {member}");
        }
        assert!(
            !assets.contains("int(LayerDoor)"),
            "the door layer is not a cutout material"
        );
        assert!(cutout_material(CUTOUT_MATERIAL_LEAVES));
        assert!(cutout_material(CUTOUT_MATERIAL_GLASS));
        assert!(cutout_material(TORCH_MATERIAL));
        assert!(cutout_material(PLANT_MATERIAL_FIRST));
        assert!(cutout_material(PLANT_MATERIAL_LAST));
        assert!(cutout_material(PLANT_MATERIAL_SHORT_GRASS));
        assert!(cutout_material(PLANT_MATERIAL_SAPLING));
        assert!(!cutout_material(DOOR_MATERIAL));
        assert!(!cutout_material(WATER_MATERIAL));
        assert!(!cutout_material(0));
    }

    #[test]
    fn quad_decode_pins_the_water_split_against_the_go_scheduler() {
        // The producer's two-stream split is exactly the water layer: the
        // render scheduler routes on the material, and the pilot's
        // combined payload re-runs that same split with `stream_of`.
        assert!(SCHEDULER_GO.contains("q.Mat == assets.LayerWater"));
        assert_eq!(WATER_MATERIAL, 28);
    }

    #[test]
    fn quad_decode_pins_winding_and_diagonal_normals_against_the_wgpu_client() {
        // The shared quad index buffer of the terrain and water passes
        // fixes the winding this expansion must reproduce.
        assert!(RENDER_MOD_RS.contains("u32s_to_bytes(&[0, 1, 2, 0, 2, 3])"));
        assert_eq!(QUAD_INDICES, [0, 1, 2, 0, 2, 3]);
        // Cross-diagonal normals and the back-flag negation come from the
        // GPU cull pass, the only consumer of face normals today.
        assert!(CULL_WGSL.contains("var n = vec3f(1.0, 0.0, -1.0);"));
        assert!(CULL_WGSL.contains("n = vec3f(1.0, 0.0, 1.0);"));
        assert!(CULL_WGSL.contains("if (((lo >> 12u) & 1u) == 1u) {"));
    }

    #[test]
    fn quad_decode_pins_the_frozen_batch_limits_against_presentation_go() {
        let presentation = normalized(PRESENTATION_GO);
        let core = normalized(POS_GO);
        assert!(presentation.contains("MaxSectionMeshQuads = 6 * core.BlocksPerSection"));
        assert!(core.contains("SectionSize = 16"));
        assert!(core.contains("BlocksPerSection = SectionSize * SectionSize * SectionSize"));
        assert_eq!(
            abi::MAX_SECTION_MESH_QUADS,
            6 * 16 * 16 * 16,
            "section quad limit must stay the mesher worst case"
        );
        assert_eq!(go_const(PRESENTATION_GO, "MaxWorldBatchOperations"), 4096);
        assert_eq!(abi::MAX_WORLD_BATCH_OPERATIONS, 4096);
        assert!(presentation.contains("MaxWorldBatchMeshBytes = 4 * 1024 * 1024"));
        assert!(presentation.contains("MaxWorldBatchPackedQuads = MaxWorldBatchMeshBytes / 8"));
        assert_eq!(abi::MAX_WORLD_BATCH_QUADS, 4 * 1024 * 1024 / 8);
    }

    // ------------------------------------------------------------------
    // Decode: layout round-trips and domain boundaries.
    // ------------------------------------------------------------------

    #[test]
    fn quad_decode_decodes_one_merged_quad_bit_exactly() {
        // Hand-assembled word independent of the fixture packer, pinning
        // the whole layout in one place (mirrors the engine's own
        // `pack_matches_go_layout`).
        let want = 3u64
            | 4u64 << SHIFT_Y
            | 5u64 << SHIFT_Z
            | 5u64 << SHIFT_W
            | 6u64 << SHIFT_H
            | 3u64 << SHIFT_FACE
            | 0x1234u64 << SHIFT_MAT
            | 0xa5u64 << SHIFT_AO
            | 0xbcu64 << SHIFT_LIGHT;
        assert_eq!(
            TestQuad {
                x: 3,
                y: 4,
                z: 5,
                w: 6,
                h: 7,
                face: 3,
                material: 0x1234,
                ao: 0xa5,
                light: 0xbc,
                ..TestQuad::default()
            }
            .pack(),
            want
        );
        assert_eq!(
            decode_quad(want).expect("layout word must decode"),
            DecodedQuad {
                x: 3,
                y: 4,
                z: 5,
                face: Face::PosY,
                material: 0x1234,
                ao: 0xa5,
                light: 0xbc,
                shape: Shape::Merged { w: 6, h: 7 },
            }
        );
    }

    #[test]
    fn quad_decode_decodes_the_atlas_layer_domain_boundaries() {
        // The material layer carries the full u16 domain; the quad
        // contract does not bound it by the atlas size. 65535 shares no
        // bit with the reserved bit 63.
        for material in [
            0u16,
            WATER_MATERIAL,
            FARMLAND_MATERIAL_FIRST,
            FARMLAND_MATERIAL_LAST,
            PLANT_MATERIAL_FIRST,
            PLANT_MATERIAL_LAST,
            DOOR_MATERIAL,
            TORCH_MATERIAL,
            BED_MATERIAL_FIRST,
            BED_MATERIAL_LAST,
            PLANT_MATERIAL_SHORT_GRASS,
            PLANT_MATERIAL_SAPLING,
            u16::MAX,
        ] {
            // Plant-set materials travel on a cross diagonal; the rest on
            // an axial face, exactly as the producer emits them.
            let face = if plant_material(material) { 6 } else { 3 };
            let quad = decoded(TestQuad {
                face,
                material,
                ..TestQuad::default()
            });
            assert_eq!(quad.material, material, "material {material}");
        }
    }

    #[test]
    fn quad_decode_decodes_the_full_ao_and_light_domain() {
        for value in 0..=u8::MAX {
            let quad = decoded(TestQuad {
                ao: value,
                light: value,
                ..TestQuad::default()
            });
            assert_eq!(quad.ao, value);
            assert_eq!(quad.light, value);
            let expanded = expand(&quad);
            for (index, vertex) in expanded.vertices.iter().enumerate() {
                let level = f32::from((value >> (index * 2)) & 0x3);
                approx(vertex.ao_factor, 0.55 + 0.45 * (level / 3.0));
                assert_eq!(vertex.sky, value >> 4);
                assert_eq!(vertex.block, value & 0xF);
            }
        }
        // One patterned byte pins each two-bit slot and each nibble.
        let quad = decoded(TestQuad {
            ao: 0xE4,
            light: 0xA5,
            ..TestQuad::default()
        });
        let expanded = expand(&quad);
        let levels: [f32; 4] = expanded.vertices.map(|vertex| vertex.ao_factor);
        approx(levels[0], 0.55);
        approx(levels[1], 0.55 + 0.45 * (1.0 / 3.0));
        approx(levels[2], 0.55 + 0.45 * (2.0 / 3.0));
        approx(levels[3], 1.00);
        assert_eq!(expanded.vertices[0].sky, 10);
        assert_eq!(expanded.vertices[0].block, 5);
    }

    // ------------------------------------------------------------------
    // Expansion geometry: every face, hand-computed against the shaders.
    // ------------------------------------------------------------------

    #[test]
    fn quad_decode_expands_every_axial_face() {
        // Merged quad x=2 y=3 z=5 w=4 h=6 on each axial face; the vertex
        // tables are hand-derived from terrain.wgsl's construction
        // (axis_vec / ua / va with the cu/cv corner table).
        let cases: [(u8, PositionTable, [f32; 3], UvTable); 6] = [
            // Face 0 (NegX): U along +Y scaled by w, V along +Z by h.
            (
                0,
                [
                    [2.0, 3.0, 5.0],
                    [2.0, 7.0, 5.0],
                    [2.0, 7.0, 11.0],
                    [2.0, 3.0, 11.0],
                ],
                [-1.0, 0.0, 0.0],
                [[5.0, -3.0], [5.0, -7.0], [11.0, -7.0], [11.0, -3.0]],
            ),
            (
                1,
                [
                    [3.0, 3.0, 5.0],
                    [3.0, 7.0, 5.0],
                    [3.0, 7.0, 11.0],
                    [3.0, 3.0, 11.0],
                ],
                [1.0, 0.0, 0.0],
                [[5.0, -3.0], [5.0, -7.0], [11.0, -7.0], [11.0, -3.0]],
            ),
            // Face 2 (NegY): U along +Z scaled by w, V along +X by h.
            (
                2,
                [
                    [2.0, 3.0, 5.0],
                    [2.0, 3.0, 9.0],
                    [8.0, 3.0, 9.0],
                    [8.0, 3.0, 5.0],
                ],
                [0.0, -1.0, 0.0],
                [[5.0, 2.0], [9.0, 2.0], [9.0, 8.0], [5.0, 8.0]],
            ),
            (
                3,
                [
                    [2.0, 4.0, 5.0],
                    [2.0, 4.0, 9.0],
                    [8.0, 4.0, 9.0],
                    [8.0, 4.0, 5.0],
                ],
                [0.0, 1.0, 0.0],
                [[5.0, 2.0], [9.0, 2.0], [9.0, 8.0], [5.0, 8.0]],
            ),
            // Face 4 (NegZ): U along +X scaled by w, V along +Y by h.
            (
                4,
                [
                    [2.0, 3.0, 5.0],
                    [6.0, 3.0, 5.0],
                    [6.0, 9.0, 5.0],
                    [2.0, 9.0, 5.0],
                ],
                [0.0, 0.0, -1.0],
                [[2.0, -3.0], [6.0, -3.0], [6.0, -9.0], [2.0, -9.0]],
            ),
            (
                5,
                [
                    [2.0, 3.0, 6.0],
                    [6.0, 3.0, 6.0],
                    [6.0, 9.0, 6.0],
                    [2.0, 9.0, 6.0],
                ],
                [0.0, 0.0, 1.0],
                [[2.0, -3.0], [6.0, -3.0], [6.0, -9.0], [2.0, -9.0]],
            ),
        ];
        for (face, positions, normal, uvs) in cases {
            let quad = decoded(TestQuad {
                x: 2,
                y: 3,
                z: 5,
                w: 4,
                h: 6,
                face,
                material: 0,
                ..TestQuad::default()
            });
            let expanded = expand(&quad);
            for index in 0..4 {
                assert_eq!(
                    expanded.vertices[index].position, positions[index],
                    "face {face} vertex {index} position"
                );
                assert_eq!(
                    expanded.vertices[index].normal, normal,
                    "face {face} vertex {index} normal"
                );
                assert_eq!(
                    expanded.vertices[index].uv, uvs[index],
                    "face {face} vertex {index} uv"
                );
                approx(
                    expanded.vertices[index].shade,
                    face_shade(quad.face)
                        * expanded.vertices[index].ao_factor
                        * hemi_factor(quad.face),
                );
            }
        }
    }

    #[test]
    fn quad_decode_expands_both_plant_diagonals_with_back_flags() {
        // Diagonal A walks (x, z) -> (x+1, z+1); diagonal B walks
        // (x+1, z) -> (x, z+1); the back flag never changes geometry,
        // only the normal's sign. The diagonals' normals are the cull
        // pass's raw directions scaled to unit length.
        let unit = std::f32::consts::FRAC_1_SQRT_2;
        let cases: [(u8, bool, PositionTable, [f32; 3]); 4] = [
            (
                6,
                false,
                [
                    [2.0, 3.0, 5.0],
                    [3.0, 3.0, 6.0],
                    [3.0, 4.0, 6.0],
                    [2.0, 4.0, 5.0],
                ],
                [unit, 0.0, -unit],
            ),
            (
                6,
                true,
                [
                    [2.0, 3.0, 5.0],
                    [3.0, 3.0, 6.0],
                    [3.0, 4.0, 6.0],
                    [2.0, 4.0, 5.0],
                ],
                [-unit, 0.0, unit],
            ),
            (
                7,
                false,
                [
                    [3.0, 3.0, 5.0],
                    [2.0, 3.0, 6.0],
                    [2.0, 4.0, 6.0],
                    [3.0, 4.0, 5.0],
                ],
                [unit, 0.0, unit],
            ),
            (
                7,
                true,
                [
                    [3.0, 3.0, 5.0],
                    [2.0, 3.0, 6.0],
                    [2.0, 4.0, 6.0],
                    [3.0, 4.0, 5.0],
                ],
                [-unit, 0.0, -unit],
            ),
        ];
        for (face, back, positions, normal) in cases {
            let quad = decoded(TestQuad {
                x: 2,
                y: 3,
                z: 5,
                face,
                back,
                material: PLANT_MATERIAL_FIRST,
                ..TestQuad::default()
            });
            assert_eq!(
                quad.shape,
                Shape::PlantDiagonal { back },
                "face {face} back {back}"
            );
            let expanded = expand(&quad);
            for (vertex, want) in expanded.vertices.iter().zip(positions) {
                assert_eq!(vertex.position, want, "face {face} back {back} vertex");
                assert_eq!(vertex.normal, normal);
                // Cross diagonals pin the wgpu UV override (world.x,
                // -world.y) rather than the axial face_uv projection.
                assert_eq!(vertex.uv[0], want[0], "diagonal u is the local x");
                assert_eq!(vertex.uv[1], -want[1], "diagonal v is the negated local y");
            }
        }
    }

    #[test]
    fn quad_decode_expands_water_corner_heights() {
        // A water top face with four distinct raw heights: the axis-face
        // position is computed first, then each nonzero corner lifts the
        // vertex to y + (raw + 1) / 16. Real height (8+1)/16 etc. is
        // exact in f32, so equality pinning is safe.
        let quad = decoded(TestQuad {
            x: 1,
            y: 2,
            z: 3,
            face: 3,
            material: WATER_MATERIAL,
            corners: [8, 12, 10, 15],
            ..TestQuad::default()
        });
        assert_eq!(
            quad.shape,
            Shape::CornerHeights {
                corners: [8, 12, 10, 15]
            }
        );
        let expanded = expand(&quad);
        let positions: [[f32; 3]; 4] = [
            [1.0, 2.0 + 9.0 / 16.0, 3.0],
            [1.0, 2.0 + 13.0 / 16.0, 4.0],
            [2.0, 2.0 + 11.0 / 16.0, 4.0],
            [2.0, 3.0, 3.0],
        ];
        let uvs: [[f32; 2]; 4] = [[3.0, 1.0], [4.0, 1.0], [4.0, 2.0], [3.0, 2.0]];
        for index in 0..4 {
            assert_eq!(expanded.vertices[index].position, positions[index]);
            assert_eq!(expanded.vertices[index].uv, uvs[index]);
            assert_eq!(expanded.vertices[index].normal, [0.0, 1.0, 0.0]);
            assert_eq!(expanded.vertices[index].layer, WATER_MATERIAL);
        }
    }

    #[test]
    fn quad_decode_expands_a_side_face_with_base_and_lifted_corners() {
        // A wall-side corner-height quad (torch-slab shape): the two top
        // corners carry raw heights while the two base corners stay at
        // the block bottom, pinning the lift's selective application.
        let quad = decoded(TestQuad {
            x: 4,
            y: 6,
            z: 8,
            face: 5,
            material: TORCH_MATERIAL,
            corners: [0, 0, 14, 14],
            ..TestQuad::default()
        });
        let expanded = expand(&quad);
        let positions: [[f32; 3]; 4] = [
            [4.0, 6.0, 9.0],
            [5.0, 6.0, 9.0],
            [5.0, 6.0 + 15.0 / 16.0, 9.0],
            [4.0, 6.0 + 15.0 / 16.0, 9.0],
        ];
        let uvs: [[f32; 2]; 4] = [
            [4.0, -6.0],
            [5.0, -6.0],
            [5.0, -(6.0 + 15.0 / 16.0)],
            [4.0, -(6.0 + 15.0 / 16.0)],
        ];
        for index in 0..4 {
            assert_eq!(expanded.vertices[index].position, positions[index]);
            assert_eq!(expanded.vertices[index].uv, uvs[index]);
            assert_eq!(expanded.vertices[index].normal, [0.0, 0.0, 1.0]);
        }
    }

    #[test]
    fn quad_decode_expands_farmland_constant_corners() {
        // Farmland lifts all four corners of its top face to the same raw
        // 14 (real height 15/16, the collision-box height).
        let quad = decoded(TestQuad {
            x: 9,
            y: 1,
            z: 2,
            face: 3,
            material: FARMLAND_MATERIAL_FIRST,
            corners: [14, 14, 14, 14],
            ..TestQuad::default()
        });
        let expanded = expand(&quad);
        let positions: PositionTable = [
            [9.0, 1.0 + 15.0 / 16.0, 2.0],
            [9.0, 1.0 + 15.0 / 16.0, 3.0],
            [10.0, 1.0 + 15.0 / 16.0, 3.0],
            [10.0, 1.0 + 15.0 / 16.0, 2.0],
        ];
        for (vertex, want) in expanded.vertices.iter().zip(positions) {
            assert_eq!(vertex.position, want);
        }
        // The wet variant takes the same corner path.
        assert_eq!(
            decoded(TestQuad {
                material: FARMLAND_MATERIAL_LAST,
                corners: [14, 14, 14, 14],
                ..TestQuad::default()
            })
            .shape,
            Shape::CornerHeights {
                corners: [14, 14, 14, 14]
            }
        );
    }

    #[test]
    fn quad_decode_routes_structural_corner_heights_outside_the_material_sets() {
        // Bed side boards read the oak-planks layer the registry shares
        // with full blocks: the material gate cannot catch them, and the
        // corner-2 structural discriminator must. The geometry is the
        // 1x1 corner-height expansion, not a merged slab.
        let quad = decoded(TestQuad {
            x: 0,
            y: 0,
            z: 0,
            face: 3,
            material: 20,
            corners: [8, 8, 8, 8],
            ..TestQuad::default()
        });
        assert_eq!(
            quad.shape,
            Shape::CornerHeights {
                corners: [8, 8, 8, 8]
            }
        );
        let expanded = expand(&quad);
        for vertex in expanded.vertices {
            assert_eq!(vertex.position[1], 9.0 / 16.0);
        }
        // The bed surface layers route by material into the same path.
        assert_eq!(
            decoded(TestQuad {
                material: BED_MATERIAL_FIRST,
                corners: [8, 8, 8, 8],
                ..TestQuad::default()
            })
            .shape,
            Shape::CornerHeights {
                corners: [8, 8, 8, 8]
            }
        );
    }

    #[test]
    fn quad_decode_keeps_merged_width_and_height_semantics() {
        // A 16x16 merged quad must not be mistaken for corner heights:
        // bits 12..19 read as w-1/h-1 with the corner-2 bits staying 0.
        let quad = decoded(TestQuad {
            w: 16,
            h: 16,
            face: 3,
            material: 0,
            ..TestQuad::default()
        });
        assert_eq!(quad.shape, Shape::Merged { w: 16, h: 16 });
        let expanded = expand(&quad);
        assert_eq!(
            expanded.vertices[2].position,
            [16.0, 1.0, 16.0],
            "the far corner of a 16x16 top face spans the section"
        );
    }

    // ------------------------------------------------------------------
    // Classification and stream routing.
    // ------------------------------------------------------------------

    #[test]
    fn quad_decode_classifies_stream_and_material_combinations() {
        let opaque = decoded(TestQuad {
            material: 0,
            ..TestQuad::default()
        });
        assert_eq!(classify(&opaque, QuadStream::Terrain), QuadClass::Opaque);
        // The door layer is named by the mesh contract but opaque.
        let door = decoded(TestQuad {
            material: DOOR_MATERIAL,
            ..TestQuad::default()
        });
        assert_eq!(classify(&door, QuadStream::Terrain), QuadClass::Opaque);
        for material in [
            CUTOUT_MATERIAL_LEAVES,
            CUTOUT_MATERIAL_GLASS,
            TORCH_MATERIAL,
            PLANT_MATERIAL_FIRST,
            PLANT_MATERIAL_SHORT_GRASS,
            PLANT_MATERIAL_SAPLING,
        ] {
            let cutout = decoded(TestQuad {
                material,
                face: if plant_material(material) { 6 } else { 3 },
                ..TestQuad::default()
            });
            assert_eq!(
                classify(&cutout, QuadStream::Terrain),
                QuadClass::Cutout,
                "material {material}"
            );
        }
        // Water comes from the stream origin, whatever the material.
        let surface = decoded(TestQuad {
            material: WATER_MATERIAL,
            corners: [15, 15, 15, 15],
            ..TestQuad::default()
        });
        assert_eq!(classify(&surface, QuadStream::Water), QuadClass::Water);
        // A water-layer quad on the terrain stream is a producer stream
        // violation that Go never re-validates; the decode classifies by
        // material set only, which yields opaque for the water layer.
        assert_eq!(classify(&surface, QuadStream::Terrain), QuadClass::Opaque);
    }

    #[test]
    fn quad_decode_splits_the_combined_pilot_payload_by_material() {
        let terrain = TestQuad {
            material: 0,
            ..TestQuad::default()
        }
        .pack();
        let cutout = TestQuad {
            material: CUTOUT_MATERIAL_LEAVES,
            ..TestQuad::default()
        }
        .pack();
        let water = TestQuad {
            material: WATER_MATERIAL,
            corners: [8, 8, 8, 8],
            ..TestQuad::default()
        }
        .pack();
        assert_eq!(stream_of(terrain), QuadStream::Terrain);
        assert_eq!(stream_of(cutout), QuadStream::Terrain);
        assert_eq!(stream_of(water), QuadStream::Water);
        let geometry =
            decode_pilot_section(&[terrain, cutout, water]).expect("mixed payload must decode");
        assert_eq!(geometry.opaque.vertices.len(), 4);
        assert_eq!(geometry.opaque.indices, QUAD_INDICES);
        assert_eq!(geometry.cutout.vertices.len(), 4);
        assert_eq!(geometry.cutout.indices, QUAD_INDICES);
        assert_eq!(geometry.water.vertices.len(), 4);
        assert_eq!(geometry.water.indices, QUAD_INDICES);
    }

    #[test]
    fn quad_decode_decodes_sections_per_stream() {
        let stone = TestQuad::default().pack();
        let leaves = TestQuad {
            material: CUTOUT_MATERIAL_LEAVES,
            w: 3,
            h: 2,
            ..TestQuad::default()
        }
        .pack();
        let geometry = decode_section(QuadStream::Terrain, &[stone, leaves])
            .expect("terrain stream must decode");
        assert_eq!(geometry.opaque.vertices.len(), 4);
        assert_eq!(geometry.cutout.vertices.len(), 4);
        assert!(geometry.water.vertices.is_empty());
        // Indices restart per surface class.
        assert_eq!(geometry.opaque.indices, QUAD_INDICES);
        assert_eq!(geometry.cutout.indices, QUAD_INDICES);

        let water = TestQuad {
            material: WATER_MATERIAL,
            corners: [7, 9, 11, 13],
            ..TestQuad::default()
        }
        .pack();
        let water_geometry =
            decode_section(QuadStream::Water, &[water, water]).expect("water stream must decode");
        assert_eq!(water_geometry.water.vertices.len(), 8);
        assert_eq!(
            water_geometry.water.indices,
            [0, 1, 2, 0, 2, 3, 4, 5, 6, 4, 6, 7]
        );
        assert!(water_geometry.opaque.vertices.is_empty());
    }

    // ------------------------------------------------------------------
    // Frozen-limit batch decode.
    // ------------------------------------------------------------------

    #[test]
    fn quad_decode_decodes_a_maximum_section() {
        // The mesher worst case: six faces over 4096 blocks.
        let quads = vec![TestQuad::default().pack(); abi::MAX_SECTION_MESH_QUADS as usize];
        let geometry = decode_section(QuadStream::Terrain, &quads)
            .expect("a section at the frozen limit must decode");
        assert_eq!(
            geometry.opaque.vertices.len(),
            abi::MAX_SECTION_MESH_QUADS as usize * 4
        );
        // One quad past the limit is a contract violation.
        let over = vec![0u64; abi::MAX_SECTION_MESH_QUADS as usize + 1];
        assert_eq!(
            decode_section(QuadStream::Terrain, &over),
            Err(abi::STATUS_INTERNAL)
        );
    }

    #[test]
    fn quad_decode_decodes_a_maximum_world_batch() {
        // 4096 sections carrying 128 quads each: the operation and packed
        // quad totals sit exactly on the frozen batch limits. Geometry is
        // held one section at a time, matching the worker's bounded
        // per-section expansion budget.
        let section: Vec<u64> = (0..128)
            .map(|index| {
                let material = match index % 4 {
                    0 => 0,
                    1 => CUTOUT_MATERIAL_LEAVES,
                    2 => WATER_MATERIAL,
                    _ => PLANT_MATERIAL_FIRST,
                };
                // Plant-set materials only ever travel on the cross
                // diagonals, exactly as the producer packs them.
                let face = if plant_material(material) {
                    6 + (index % 2) as u8
                } else {
                    (index % 6) as u8
                };
                TestQuad {
                    x: (index % 16) as u8,
                    y: ((index / 16) % 16) as u8,
                    z: 0,
                    w: ((index % 15) + 1) as u8,
                    h: ((index % 13) + 1) as u8,
                    face,
                    material,
                    ..TestQuad::default()
                }
                .pack()
            })
            .collect();
        let packed = section.clone();
        let terrain: Vec<u64> = packed
            .iter()
            .copied()
            .filter(|packed| stream_of(*packed) == QuadStream::Terrain)
            .collect();
        let water: Vec<u64> = packed
            .iter()
            .copied()
            .filter(|packed| stream_of(*packed) == QuadStream::Water)
            .collect();
        assert_eq!(
            check_world_batch(4096, 4096 * 128),
            Ok(()),
            "a batch exactly on both limits must pass"
        );
        let mut expanded = 0usize;
        for _ in 0..4096 {
            let geometry = decode_pilot_section(&section).expect("section must decode");
            expanded += geometry.opaque.vertices.len()
                + geometry.cutout.vertices.len()
                + geometry.water.vertices.len();
            assert_eq!(geometry.water.vertices.len(), water.len() * 4);
            assert_eq!(
                geometry.opaque.vertices.len() + geometry.cutout.vertices.len(),
                terrain.len() * 4
            );
        }
        assert_eq!(expanded, 524_288 * 4);
    }

    #[test]
    fn quad_decode_rejects_batches_outside_the_frozen_limits() {
        assert_eq!(check_world_batch(0, 1), Err(abi::STATUS_INTERNAL));
        assert_eq!(
            check_world_batch(abi::MAX_WORLD_BATCH_OPERATIONS as usize + 1, 1),
            Err(abi::STATUS_INTERNAL)
        );
        assert_eq!(
            check_world_batch(1, abi::MAX_WORLD_BATCH_QUADS as usize + 1),
            Err(abi::STATUS_INTERNAL)
        );
        assert_eq!(
            check_world_batch(
                abi::MAX_WORLD_BATCH_OPERATIONS as usize,
                abi::MAX_WORLD_BATCH_QUADS as usize
            ),
            Ok(())
        );
    }

    // ------------------------------------------------------------------
    // Fail-closed rejection of contract-forbidden bits.
    // ------------------------------------------------------------------

    #[test]
    fn quad_decode_rejects_the_reserved_bit_63() {
        let valid = TestQuad::default().pack();
        assert_eq!(valid >> 63, 0);
        assert_eq!(decode_quad(valid), Ok(decoded(TestQuad::default())));
        assert_eq!(
            decode_quad(valid | 1 << 63),
            Err(abi::STATUS_INTERNAL),
            "bit 63 must stay clear so the quad stays eight bytes"
        );
        // Corner 3's field tops out at bit 62; setting its high bit lands
        // exactly on the reserved bit.
        let corner = TestQuad {
            corners: [0, 0, 1, 8],
            ..TestQuad::default()
        }
        .pack();
        assert_eq!(decode_quad(corner | 1 << 63), Err(abi::STATUS_INTERNAL));
    }

    #[test]
    fn quad_decode_rejects_plant_materials_on_axial_faces() {
        for material in [
            PLANT_MATERIAL_FIRST,
            PLANT_MATERIAL_LAST,
            PLANT_MATERIAL_SHORT_GRASS,
            PLANT_MATERIAL_SAPLING,
        ] {
            for face in 0..6u8 {
                let packed = TestQuad {
                    face,
                    material,
                    w: 5,
                    h: 4,
                    ..TestQuad::default()
                }
                .pack();
                assert_eq!(
                    decode_quad(packed),
                    Err(abi::STATUS_INTERNAL),
                    "plant material {material} on face {face}"
                );
            }
            // The same materials are legal on the cross diagonals.
            for face in 6..8u8 {
                let packed = TestQuad {
                    face,
                    material,
                    ..TestQuad::default()
                }
                .pack();
                assert!(
                    decode_quad(packed).is_ok(),
                    "plant material {material} on face {face}"
                );
            }
        }
    }

    #[test]
    fn quad_decode_rejects_nonzero_plant_reserved_bits() {
        for bit in 13..20u32 {
            let packed = TestQuad {
                face: 6,
                material: PLANT_MATERIAL_FIRST,
                ..TestQuad::default()
            }
            .pack()
                | 1 << bit;
            assert_eq!(
                decode_quad(packed),
                Err(abi::STATUS_INTERNAL),
                "plant reserved bit {bit}"
            );
        }
        // Both the front and the back flag forms stay legal, and the
        // corner-2 area is tolerated on cross diagonals to keep this
        // decode's acceptance domain identical to Go's boundary.
        for back in [false, true] {
            let packed = TestQuad {
                face: 7,
                back,
                material: PLANT_MATERIAL_SAPLING,
                ..TestQuad::default()
            }
            .pack();
            assert_eq!(
                decode_quad(packed).expect("diagonal must decode").shape,
                Shape::PlantDiagonal { back }
            );
            assert_eq!(
                decode_quad(packed | 1 << SHIFT_CORNER2),
                decode_quad(packed),
                "bits 55..62 are don't-care on cross diagonals, as in Go"
            );
        }
    }

    #[test]
    fn quad_decode_rejects_a_whole_section_on_one_invalid_quad() {
        let good = TestQuad::default().pack();
        let bad = TestQuad::default().pack() | 1 << 63;
        assert_eq!(
            decode_section(QuadStream::Terrain, &[good, bad, good]),
            Err(abi::STATUS_INTERNAL)
        );
        assert_eq!(
            decode_pilot_section(&[good, bad]),
            Err(abi::STATUS_INTERNAL)
        );
        // The empty payload is not a section: the presentation contract
        // requires a drop instead.
        assert_eq!(
            decode_section(QuadStream::Terrain, &[]),
            Err(abi::STATUS_INTERNAL)
        );
        assert_eq!(
            decode_section(QuadStream::Water, &[]),
            Err(abi::STATUS_INTERNAL)
        );
    }
}
