// Near-field terrain: cutout surface class of the three-way decode split.
//
// This shader shares the opaque class's full contract (ownership boundary,
// Rust-side vertex attributes and the terrain_data varying, UV convention
// pin, sampler pins, color-space pin, and daylight blend) as documented in
// terrain_near_opaque.gdshader; the files duplicate the body because Godot
// shader language, like WGSL, has no include, mirroring the
// terrain.wgsl/water.wgsl precedent.
//
// Class-specific pin: binary-alpha layers (leaves, glass, torch, and the
// plant set in the decode's cutout_material routing) discard below the
// alpha scissor threshold 0.5, the exact cutout rule of the wgpu terrain
// pass (terrain.wgsl fs_main: if (c.a < 0.5) { discard; }). Depth stays
// written (the wgpu terrain pipeline enables depth write), so cutout
// geometry occludes later surfaces exactly like the wgpu terrain pass, and
// the material stays opaque by never writing ALPHA, reproducing the wgpu
// REPLACE blend of the combined terrain pass.

shader_type spatial;
render_mode unshaded, cull_disabled, depth_draw_always;

// Raw pixel data of the generated atlas; the Rust renderer builds the
// layered texture from assets/generated/atlas.rgba8 and assigns it here,
// because the raw bytes are not a Godot texture resource and Python must
// not embed one.
uniform sampler2DArray atlas : filter_nearest_mipmap, repeat_enable;

// Per-frame daylight in 0..1, defaulting to noon (1.0) where both night
// coefficients are 1.0; the Rust frame driver updates it per frame.
uniform float daylight : hint_range(0.0, 1.0) = 1.0;

varying vec4 terrain_data;

void vertex() {
	terrain_data = CUSTOM0;
}

void fragment() {
	vec4 texel = texture(atlas, vec3(UV, terrain_data.w));
	if (texel.a < 0.5) {
		discard;
	}
	// Daylight blend completion, pinned against terrain.wgsl vs_main.
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
