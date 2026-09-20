// Near-field terrain: opaque surface class of the three-way decode split.
//
// Ownership boundary (design decision 5 of the Godot client migration):
// Python owns bounded lifecycle and configuration only; the Rust renderer
// owns packed-quad expansion, vertex buffers, RIDs, and bulk submission.
// This shader defines the presentation contract the Rust side fills, and
// no Python script ever creates terrain geometry or a fallback mesh.
//
// Rust-side vertex contract (mornlea_godot quad_decode ExpandedVertex):
//   VERTEX   section-local position of one expanded quad corner; the mesh
//            instance transform carries the section world origin.
//   NORMAL   unit face normal (cull.wgsl directions, normalized).
//   UV       wgpu-convention world-space repeating UV, passed through.
//   CUSTOM0  RGBA float attribute: x = shade, y = sky nibble, z = block
//            nibble, w = atlas layer. shade is the decode's
//            daylight-independent product face_shade * ao_factor *
//            hemi_factor; sky and block are the raw 0..15 light words this
//            shader blends with the per-frame daylight uniform below.
//            CUSTOM0 is readable only in the vertex stage, so the values
//            cross to fragment() through the terrain_data varying.
// A mesh that omits CUSTOM0 renders visibly broken (layer 0, unlit) rather
// than silently degrading, so a Rust-side contract drift fails loudly.
//
// UV convention pin (wgpu -> Godot). The Rust expansion emits the decode's
// wgpu-convention UVs verbatim: v = 0 selects the image top row (the first
// row of each atlas.rgba8 layer), X faces project (z, -y), Y faces (z, x),
// Z faces (x, -y), and the plant cross diagonals pin (x, -y) so the texture
// top row lands at the cell top. Sources of truth: terrain.wgsl face_uv and
// its cross-diagonal override, water.wgsl face_uv. Godot's spatial texture
// origin is also top-left, so the Godot-side adaptation owned by this
// material layer is the identity mapping: UV is sampled unchanged. If a
// future renderer path ever samples bottom-up, the one-line negation of the
// vertical component belongs here, never in the Rust decode that owns the
// wgpu convention.
//
// Sampler pins (wgpu source: mornlea_client render/mod.rs "block-sampler"):
// Repeat addressing on both axes, because world-space UVs tile the atlas
// once per block; nearest filtering for the pinned blocky magnification;
// mipmapping over the atlas's five precomputed levels. The wgpu sampler
// splits mag=Nearest / min=Linear / mipmap=Linear, a split the Godot
// sampler hint granularity cannot express, so filter_nearest_mipmap pins
// nearest magnification with nearest-within-level mipmapped minification,
// and that coarsening is recorded for the terrain visual gate to review.
//
// Color-space pin: the atlas is authored as sRGB bytes but uploaded as a
// plain RGBA8 data texture (wgpu Rgba8Unorm, no sRGB view), so the shader
// decodes with pow(2.2) exactly like terrain.wgsl/water.wgsl, applies the
// Narkowicz ACES approximation with exposure pinned to 1.0, and writes
// linear light; Godot's built-in linear-to-sRGB output encode replaces the
// wgpu linear_to_srgb tail.
//
// Pipeline pins (wgpu source: render/mod.rs make_render_pipeline for the
// terrain pass): no face culling (cull_mode None) and depth writing for
// opaque geometry. The wgpu depth comparison is Less while Godot's opaque
// depth test is LESS_EQUAL; the two differ only for coincident geometry,
// which single-draw-per-quad terrain never produces. The material stays
// opaque by never writing ALPHA, which reproduces the wgpu REPLACE blend;
// the opaque class never discards, because fully-opaque atlas layers are
// what classify opaque in the decode, so the wgpu discard belongs to the
// cutout class only.

shader_type spatial;
render_mode unshaded, cull_disabled, depth_draw_always;

// Raw pixel data of the generated atlas; the Rust renderer builds the
// layered texture from assets/generated/atlas.rgba8 and assigns it here,
// because the raw bytes are not a Godot texture resource and Python must
// not embed one.
uniform sampler2DArray atlas : filter_nearest_mipmap, repeat_enable;

// Per-frame daylight in 0..1, defaulting to noon (1.0) where both night
// coefficients are 1.0 and the blend degenerates to the daylight-only
// formula; the Rust frame driver updates it from the environment snapshot.
uniform float daylight : hint_range(0.0, 1.0) = 1.0;

varying vec4 terrain_data;

void vertex() {
	terrain_data = CUSTOM0;
}

void fragment() {
	vec4 texel = texture(atlas, vec3(UV, terrain_data.w));
	// Daylight blend completion, pinned against terrain.wgsl vs_main: night
	// gating fully active at daylight <= 0.15, absent at >= 0.5, with the
	// smoothstep-style cubic in between; night ambient 0.3, night exposure
	// 0.5; sky base rises from the 0.08 floor with the sky nibble while the
	// block nibble never falls below its own contribution.
	float level = clamp(daylight, 0.0, 1.0);
	float gate = clamp((level - 0.15) / 0.35, 0.0, 1.0);
	float day_t = gate * gate * (3.0 - 2.0 * gate);
	float night_amb = mix(0.3, 1.0, day_t);
	float night_expo = mix(0.5, 1.0, day_t);
	float sky = terrain_data.y / 15.0;
	float block = terrain_data.z / 15.0;
	float base = max((0.08 + sky * (level - 0.08)) * night_amb, block);
	vec3 linear = pow(texel.rgb, vec3(2.2)) * (terrain_data.x * base * night_expo);
	ALBEDO = clamp(
		(linear * (2.51 * linear + 0.03)) / (linear * (2.43 * linear + 0.59) + 0.14),
		vec3(0.0),
		vec3(1.0)
	);
}
