# Kernel compatibility evidence

Read-only extraction at repository HEAD `60c476645ee6dae1f6392336a7f3c593d2163ae3`, 2026-09-20. No repository file was edited and no tests were run. Source root below is `packages/engine/crates/mornlea_engine/src/` in `/Users/chen/work/mornlea`. These are the verified source observations used by the controller for `plans/05-kernel.md`; that plan resolves target APIs and safety behavior. Newly observed gaps below are static findings, not additional executed regressions.

## Cross-cutting facts

- `lib.rs:1–14` declares every module with private `mod`; the useful core functions and structs are `pub(crate)` or private. There is presently no public typed Rust kernel API. Many safe Rust functions rely on prior byte validation and can panic if called directly with invalid byte slices/constructed records. Exporting them alone would not add validation.
- No `format.rs` was found anywhere in the repository by `rg --files | rg '(^|/)format\\.rs$'`. Near-mesh packing lives in `quad.rs`; other formats live alongside algorithms or in `ffi.rs`.
- Engine ABI is 11 (`ffi.rs:54`). Status values (`ffi.rs:65–74`): OK=0, ABI_VERSION=1, INVALID_ARGUMENT=2, INPUT=3, SCRATCH=4, REGISTRY=5, EMISSION=6, OUTPUT_OVERFLOW=7, QUEUE_OVERFLOW=8, PANIC=9. The release profile uses `panic="unwind"` (`packages/engine/Cargo.toml:12`).
- Byte buffers are read as little endian without typed pointer casts. FFI checks nulls, `length <= isize::MAX`, address-add overflow, relevant alignment, and overlaps before creating slices. `output_len`/count metadata pointers require `usize` alignment. These checks do not prove memory validity; callers still own valid allocations/lifetimes.
- Collision, physics, chunk, probe, tree, LOD, eval and rescan stage complete results locally before copying output. Raycast stages cursor and output together. Mesh is different: it writes directly into caller output and only publishes the count on success (`ffi.rs:248–260,348–376`; `greedy/mod.rs:166–187`). Thus mesh count publication is atomic, but late mesh overflow/panic can leave overwritten output bytes with count zero.
- Mesh, LOD, fluid-eval and rescan clear a valid `output_len` before checking its overlap with buffers (`ffi.rs:291–298`, `764–771`, `1108–1115`, `1244–1251`). If metadata itself aliases a buffer, its zero write occurs before overlap rejection. Raycast checks metadata↔metadata and metadata↔fixed buffer ranges before clearing either scalar (`ffi.rs:893–924,977–996`), and tests this distinction. Claims that *all* rejected aliased buffers stay byte-for-byte unchanged would exceed current behavior.

## 1. Collision

| Concern | Current fact and evidence |
|---|---|
| Typed internals | Private `Bounds { minimum:[f32;3], maximum:[f32;3] }`, `MoveResult { position:[f32;3], clipped:[bool;3], on_ground:bool, hit_unknown:bool }`, `CollisionInput<'a> { bytes:&'a [u8], position:[f32;3], displacement:[f32;3], began_grounded:bool, step_height:f32, origin:[i32;3], dimensions:[u32;3] }`, `Cell<'a> { bytes:&'a [u8], position:[i32;3] }` (`collision.rs:10–40`). |
| Core entry points | `resolve_collision(&[u8]) -> [u8;16]`; `resolve_collision_parts(position,displacement,began_grounded,step_height,origin,dimensions,cells:&[u8]) -> [u8;16]` (`collision.rs:148–173`). Neither validates its own inputs. |
| Input layout | 64-byte header, `MGC1`, layout 1; position at 8/12/16, displacement 20/24/28, began-grounded byte32, reserved33..35=0, step height36, origin40/44/48, dimensions52/56/60. Exactly `64 + 196*N` bytes (`ffi.rs:132–194`). |
| Cell layout | loaded u8 0/1, box-count u8 0..8, two zero reserved bytes, eight 24-byte box slots (minimum xyz then maximum xyz, all f32); only used slots checked for finite components. Cells indexed `((y*dim_x)+x)*dim_z+z` (`collision.rs:93–105,111–134`). Loaded=false is accepted and treated as a closed unit cube; inactive box slots are not validated. |
| Scalar/cap validation | All position/displacement/step-height floats finite. Every dimension >0; checked product <=4096; exact length; each origin+(dimension−1) checked for i32 overflow. No step-height sign bound, no box minimum<=maximum check and no restriction of box coordinates to [0,1] (`ffi.rs:132–194`; layout test deliberately includes −0.25 and 1.5 at `ffi.rs:1422–1434`). |
| Required coverage | Prism must cover swept player box and step/ground probe. x/z min=`min(pos,pos+disp)−0.3−1e−5`, max=`max(pos,pos+disp)+0.3+1e−5`; y min=`pos_y+min(0,disp_y,step_height)−1e−4−1e−5`; y max=`pos_y+max(0,disp_y,step_height)+1.8+1e−5`. Bounds finite, floored required coordinates within i32, prism contains them (`ffi.rs:198–237`). |
| Output/capacity | Exactly 16 bytes: position[3] f32, xyz clipped bitmask byte12, on_ground13, used_step14, hit_unknown15 (`collision.rs:500–513`). Output<16 ⇒ OVERFLOW; >16 ⇒ INVALID_ARGUMENT. Input violation⇒INPUT; panic⇒PANIC. Local result copied only on success (`ffi.rs:401–443`). Max input 802,880 bytes. |
| Existing tests | `collision.rs:636 collision_resolves_in_y_x_z_order`, `:667 collision_treats_unknown_as_closed`, `:689 rejected_step_unknown_does_not_reach_selected_result`; `ffi.rs:1361 collision_layout_v1_is_stable`, `:1456 collision_panic_through_publish_path_keeps_caller_output_unchanged`, `:2284 malformed_collision_input_keeps_output_unchanged`. |

## 2. Physics step

`StepInput<'a>` (`step.rs:39–81`) contains:

- `bytes:&'a [u8]`; `position,velocity:[f32;3]`; `on_ground,jump:bool`; `move_x,move_z:i8`.
- `yaw_sin,yaw_cos,fixed_delta_seconds,step_height,walk_speed,ground_acceleration,ground_deceleration,air_acceleration,jump_speed,gravity,terminal_fall_speed:f32`.
- `body_in_fluid,sprinting,sneaking:bool`.
- `fluid_gravity,fluid_sink_speed,fluid_ascend_speed,fluid_horizontal_drag,sprint_speed_multiplier,sneak_speed_multiplier:f32`.
- `sweep_min,sweep_max:[f32;3]`; `origin:[i32;3]`; `dimensions:[u32;3]`.

| Concern | Current fact and evidence |
|---|---|
| Entry points | `StepInput::decode(&[u8])->Self`; `step_input_is_valid(&[u8])->bool`; `integrate(&StepInput)->([f32;3],[f32;3])`; `physics_step(&[u8])->Result<[u8;32],StepError>`, only error `DisplacementOutOfBounds` (`step.rs:84,141,271,337–348`). `physics_step` decodes without calling validator. |
| Layout/cap | `MGP1`, layout4, header160 bytes, identical 196-byte cell records; exactly `160+196*N`, N<=4096, nonzero dimensions and checked origin upper coordinates. Max input802,976 bytes (`step.rs:10–17,141–209`). |
| Value validation | Booleans at bytes32,33,128,129,130 ≤1; move values −1..1; byte131 and156..159 zero. All position,velocity,yaw,dt,tunables,sweep and multipliers finite; each sweep min<=max. Used AABB floats finite, boxcount<=8, loaded<=1 and cellreserved zero. No positivity/range checks for dt, speeds, gravity, drag, multipliers; no unit-circle check for yaw; no ordered/local AABB check (`step.rs:141–209`). |
| Coverage distinction | Unlike standalone collision validation, `step_input_is_valid` does **not** verify prism covers movement/player/step/ground-probe volume. `physics_step` only checks integrated displacement per axis against `[sweep_min.next_down(),sweep_max.next_up()]` (one-ULP allowance), then directly calls collision parts (`step.rs:348–372`). |
| Output | 32 bytes: position[3] at0, velocity[3] at12, clipped24,on_ground25,used_step26,hit_unknown27,reserved28..31 zero; clipped velocity components zeroed (`step.rs:373–393`). Exactly32 bytes required; shorter⇒OVERFLOW, larger⇒INVALID_ARGUMENT; validator or displacement failure⇒INPUT. FFI local stage/copy (`ffi.rs:458–500`). |
| Tests | Validator matrix `step.rs:449–614`; `:641 sneaking_slows_to_sneak_multiplier`, `:655 sneaking_takes_priority_over_sprinting`, `:685 jump_uses_jump_speed`, `:694 gravity_clamps_to_terminal`, `:755 physics_step_rejects_displacement_outside_sweep_bounds`, `:766 physics_step_allows_one_ulp_outside_sweep_bounds`, `:781 physics_step_rejects_two_ulp_outside_sweep_bounds`, `:798 physics_step_encodes_output_layout`, `:816 physics_step_lands_on_floor_and_clips_velocity`. |

## 3. Raycast batches

| Concern | Current fact and evidence |
|---|---|
| Typed internals | Private `RaycastInput { origin,direction:[f32;3], maximum:f32 }`; private `RaycastCursor { state:u8, cell,step:[i32;3], delta,maximum:[f32;3] }`; crate-visible `RaycastBatch { cursor:[u8;64], output:[u8;1280], count:usize, done:bool }` (`raycast.rs:1–28`). No typed hit record; `write_record` accepts block[3],face:u8,distance:f32. |
| Core API | `raycast_batch(input_bytes:&[u8],cursor_bytes:&[u8])->RaycastBatch`; validation lives in FFI (`raycast.rs:133`; `ffi.rs:832–891`). |
| Input validation | Exactly40 bytes, `MGR1`,layout1,reserved36..39 zero; finite origin/direction; at least one nonzero direction component; finite maximum>0. Direction need not be normalized, origin not limited to i32 range (`ffi.rs:832–848`). |
| Cursor validation | Exactly64 bytes,`MRC1`,layout1,state0/1/2,reserved9..11 and60..63 zero. Fresh state0 requires all bytes12..63 zero. Active/done require step=sign(direction), exact reciprocal-magnitude delta bits (zero direction⇒+infinity), maximum finite or +infinity or documented overflow case. Zero direction additionally requires maximum=+infinity. No cell-progression, maximum monotonicity, same-origin or same-distance-history verification (`ffi.rs:850–891`; `raycast.rs:120–131`). Overflow exceptional maximum accepts −infinity if initial maximum is −infinity, or NaN if initial maximum −infinity and delta+infinity. |
| Bounds/continuation | 64 records per call, 20 bytes each, output1280 exactly. Fresh call includes floor(origin) record with face0xff,distance0. Cell advance uses wrapping_add; floor outside [−2^31,2^31) maps i32::MIN. Strict ties choose X thenY thenZ; exact maximum distance included (`raycast.rs:71–95,133–217`). No total traversal cap; one call bounded64. |
| Output/publication | Records xyz i32,face:u8,reserved3 bytes,distance:f32. Full1280-byte array copied (unused bytes zero), 64-byte cursor copied, count+done published only after complete success. Output<1280⇒OVERFLOW; >1280⇒INVALID_ARGUMENT. Input/cursor lengths invalid⇒INPUT. Metadata valid/no aliases before clearing; failures leave cursor/output unchanged and count=0/done=0, except invalid/aliased metadata remain untouched (`ffi.rs:965–1051`). |
| Tests | `raycast.rs:334 raycast_emits_origin_with_negative_floor`,`:350 raycast_uses_strict_xyz_tie_priority`,`:370 raycast_includes_exact_endpoint_and_rejects_next_float`,`:391 raycast_continues_after_sixty_four_records`,`:405 raycast_wraps_i32_cell_advance`; FFI `:1484 raycast_panic_through_publish_path_is_atomic`,`:1516 raycast_success_publishes_local_cursor_and_output_once`,`:1782 malformed_raycast_input_and_cursor_matrix_is_atomic`,`:2003 invalid_raycast_metadata_pointer_matrix_is_atomic`,`:2014 raycast_metadata_buffer_overlap_matrix_is_atomic`. |

## Shared world-generation parameters

`Materials` has 15 fields, all `u16`: `air, stone, dirt, grass, bedrock, snow, sand, clay, gravel, iron_ore, coal_ore, oak_log, leaves, water, short_grass` (`worldgen.rs:105–123`). `WorldgenParams { seed: i64, materials: Materials, perm: [u8;512] }` (`:174–179`).

`parse_header` (`worldgen.rs:944–990`) requires at least 566 bytes, magic `MGW1`, layout 3, world min Y exactly −64 and max Y exactly 320. Material IDs must be pairwise distinct except `water == air`, which gates water and the LOD sea clamp off. All `u16` values are accepted; there is no registry lookup. The 512 permutation bytes are copied as-is, without checking permutation uniqueness, duplication or correspondence to the seed. Any `i64` seed is accepted. Offsets: seed 8, min Y 16, max Y 20, materials 24..53, perm 54..565.

## 4. Worldgen chunk

| Concern | Current fact and evidence |
|---|---|
| Request/API | No request struct. Parser returns `(WorldgenParams, chunk_x: i32, chunk_z: i32)` from exactly 574 bytes (`worldgen.rs:993–1001`). Core method is `generate_chunk(&self, chunk_x: i32, chunk_z: i32, dense: &mut [u16])` (`:487`). |
| Coordinates | Only shared header and exact length are validated. No coordinate range check. Generation derives base using signed `chunk << 4`; ordinary additions for local columns and tree fringe follow (`worldgen.rs:490–496,575–580`). Extreme derived bases can overflow tree-fringe arithmetic in debug builds. Parser acceptance alone does not establish safe coordinate arithmetic. |
| Output/cap | Fixed 16×16×384 = 98,304 `u16`; layout `[y+64][lz][lx]`; exactly 196,608 output bytes. The core only debug-asserts dense length, then mutates dense in place. FFI allocates exact dense and encoded vectors locally (`worldgen.rs:23–25,487–505`; `ffi.rs:546–562`). |
| Status | Output shorter than 196608 ⇒ OVERFLOW; longer ⇒ INVALID_ARGUMENT. Parse failure ⇒ INPUT; panic ⇒ PANIC. Caller output is copied only after success (`ffi.rs:520–568`). |
| Tests | `worldgen.rs:1145 generate_chunk_is_deterministic`; `:1155 chunk_matches_pointwise_base_block`; `:1232 flooding_only_replaces_air_at_or_below_sea_level`; `:1276 gate_off_leaves_every_floodable_cell_as_air`; `:1329 header_allows_water_equal_to_air_but_rejects_other_duplicates`; `:1618 short_grass_chunk_and_pointwise_parity_spans_boundaries`; `:1680 short_grass_is_independent_of_generation_order`. FFI: `:3066 worldgen_chunk_is_deterministic_and_wrong_abi_is_rejected`; `:3117 worldgen_chunk_invalid_input_leaves_output_untouched`. |

## 5. Worldgen probe

| Concern | Current fact and evidence |
|---|---|
| Typed record/API | `ProbeRecord { mode: u32, wx: i32, wy: i32, wz: i32 }`; parser returns `(WorldgenParams, Vec<ProbeRecord>)`; `run_probe(&WorldgenParams, &[ProbeRecord], &mut [u8])` (`worldgen.rs:1004–1064`). |
| Validation | Shared 566-byte header, u32 count, then 16-byte records. Count 1..=64; exact length `570 + 16*N`. Mode 0 = height, 1 = terrain, 2 = base; larger values rejected. Coordinates are arbitrary i32, including wy outside world height; no radius guard or requirement to zero ignored coordinates (`worldgen.rs:1013–1039`). Height ignores wy. Terrain explicitly returns air outside Y bounds; base continues through tree/decorative queries. Tree lookup uses ordinary x/z ±3 arithmetic (`:448–463`). |
| Output/cap | Exactly `8*N` bytes, maximum 512: height i32, block u16, reserved u16 = 0. Mode 0 leaves block zero; modes 1/2 leave height zero. Core only debug-asserts output length; if bypassing parser, its wildcard mode branch dispatches to base. FFI stages the entire vector. Short output ⇒ OVERFLOW; long ⇒ INVALID_ARGUMENT; bad input ⇒ INPUT (`worldgen.rs:1046–1064`; `ffi.rs:587–630`). |
| Tests | `ffi.rs:3180 worldgen_probe_matches_chunk_and_rejects_bad_records`; chunk/pointwise parity tests above; `worldgen.rs:1644 height_and_terrain_queries_ignore_short_grass`. |

## 6. Runtime tree blocks

| Concern | Current fact and evidence |
|---|---|
| Typed structs/API | `TreeBlocksRequest { seed: i64, x: i32, y: i32, z: i32 }`; `TreeBlock { dx: i8, dy: i8, dz: i8, block: u16 }`; parser returns `Option<TreeBlocksRequest>`; `tree_blocks(&TreeBlocksRequest) -> Option<Vec<TreeBlock>>`; `encode_tree_blocks(&[TreeBlock], &mut [u8])` (`worldgen.rs:751–773,828,863`). |
| Validation | Exactly 28 bytes, `MTB1`, layout 1. Root y in [−64,311], where 311 is `WORLD_MAX_Y−9`; x/z ±2 must fit i32. Any i64 seed. Direct constructed requests are not revalidated by `tree_blocks` (`worldgen.rs:735–793`). |
| Geometry/bound | Ordinary oak only: height 5..7, optional fluffy crown, no rare crown or branches. Order is dy outer, dz middle, dx inner. dx/dz in −2..2; dy in 0..8; blocks 17 (log) or 19 (leaves). Documented actual worst case 65 records; defensive cap 128 (`worldgen.rs:75–85,800–857`). |
| Output/cap | `4 + 8*N` bytes, maximum 1028. Count u32, then dx/dy/dz i8, reserved u8=0, block u16, reserved u16=0. FFI output below 4 bytes ⇒ OVERFLOW immediately; below actual needed bytes ⇒ OVERFLOW. Larger buffers accepted; only used prefix written. No separate required-size metadata. Kernel returns None if cap 128 is exceeded. FFI stages complete output (`ffi.rs:662–709`). |
| Tests | `worldgen.rs:2342 geometry_is_deterministic_and_bounded`; `:2393 geometry_reuses_normal_crown_tiers`; `:2448 geometry_is_independent_of_worldgen_and_short_grass_salts`; `:2461 parse_rejects_bad_input`; `:2546 encode_writes_count_and_fixed_records`; `:2570 runtime_block_ids_match_go_core`; `ffi.rs:3290 tree_blocks_is_deterministic_and_rejects_bad_input`. |

## 7. LOD shell

| Concern | Current fact and evidence |
|---|---|
| Typed structs | `LodShellRequest { params: WorldgenParams, tile_x: i32, tile_z: i32, step: u32 }`. `LodQuad { x: i32, z: i32, y: i32, w: u16, d: u16, face: LodFace, material: u16, shade: u8 }`. `LodFace { Top=0, NegX=1, PosX=2, NegZ=3, PosZ=4 }` (`lod.rs:61–107`). |
| API | `parse_lod_input(&[u8]) -> Option<LodShellRequest>`; `lod_shell(&LodShellRequest) -> Vec<LodQuad>`; `encode_shell(&[LodQuad], &mut Vec<u8>)` (`lod.rs:139,393,408`). |
| Validation | Exactly 582 bytes: shared 566-byte header, tile x/z i32, columns u32 exactly 64, step u32 in {2,4,8}. Each tile must pass checked multiplication by 64, checked base+64, and checked base−8. Positive grid alignment also guarantees base+64+step−1 fits; negative check is independent (`lod.rs:137–172`). Despite a comment saying chunk coordinates, actual base is tile×64 (`:233–245`). |
| Dimensions | N=64/step in {32,16,8}; samples (N+2)² windows, each of step² columns. Only N² interior windows produce tops; surrounding ring supplies height comparisons (`lod.rs:118–128,233–255`). |
| Output bound | Actual bytes = 20×Q. Derived conservative bound from loops: Q ≤ N²+2N(N+1) = 3N²+2N (tops plus at most one skirt per X/Z pair). Step 2: 3136 quads / 62720 bytes. Step 4: 800 / 16000. Step 8: 208 / 4160. There is no explicit static cap constant or rejection; kernel builds a Vec (`lod.rs:263–389,403–419`). Encoding order is x, z, y i32; w,d u16; face u8; material u16; shade u8. |
| Publication | Generation and encoding staged locally. Short capacity ⇒ OVERFLOW with exact required bytes in output_len; payload untouched. Any sufficient capacity accepted. Success writes prefix and actual length. Other errors keep output_len zero, subject to shared metadata-alias caveat. Null output is rejected even with capacity zero (`ffi.rs:755–829`). |
| Tests | `lod.rs:536 empty_tile_produces_no_quads`; `:549 uniform_tile_merges_to_single_top_quad`; `:569 height_step_generates_closing_skirt`; `:636 boundary_skirt_owned_by_taller_side`; `:699 parse_rejects_extreme_tiles_at_i32_boundary`; `:729 window_aggregates_match_worldgen`; `:838 sea_level_clamped_water_windows_emit_no_skirts_but_land_does`; `:936 golden_shell_bytes_are_stable`; `:953 lod_shell_ignores_short_grass_decoration`. FFI: `:3651 lod_shell_two_phase_capacity_probe_then_retry_succeeds`; `:3759 lod_shell_panic_is_contained_without_output`. |

## 8. Fluid evaluation

| Concern | Current fact and evidence |
|---|---|
| Typed core/API | No request/result structs. `eval_one(cells: &[u16;7], out: &mut [u8;12])`; `parse_eval_input(&[u8]) -> Option<usize>`; `read_eval_item(&[u8], index: usize) -> [u16;7]` (`fluid_eval.rs:192,258,274`). |
| Validation/cap | Header is u32 layout=1 and u32 item count. Exact length `8 + 14*N`, with checked multiplication/addition. **N=0 accepted; no separate operational batch cap beyond u32 and representable buffer size.** Every u16 block ID is accepted; unknown IDs follow nonfluid/nonreplaceable semantics (`fluid_eval.rs:258–282,126–149`). |
| Output | Input slots: self, above, below, +x, −x, +z, −z. Four output candidates per item, each slot u8 + block u16 LE, total 12 bytes. Used entries are contiguous, unused entries `(0xff,0,0)`. At most four horizontal writes; vertical, death, or source upgrade writes at most one. Required output `12*N`; larger capacities allowed (`fluid_eval.rs:186–251`; `ffi.rs:1148–1175`). |
| Status/publication | Short output ⇒ INVALID_ARGUMENT, **not OVERFLOW**; no size probe. Bad input ⇒ INPUT, panic ⇒ PANIC. All items staged before publication; output_len is needed bytes on success, otherwise zero (metadata caveat). Non-null output required even when N=0 (`ffi.rs:1099–1180`). |
| Tests | `fluid_eval.rs:420 replaceable_table_mirrors_go`; `:453 source_writes_level_one_down_when_below_replaceable`; `:501 level_seven_does_not_spread`; `:516 dying_flowing_cell_writes_air_to_self`; `:701 output_entries_pack_contiguously_with_sentinels`; `:722 infinite_water_two_sources_upgrade`; `:752 infinite_water_vertical_priority_preserved`; `:767 parse_eval_input_validates_layout_and_length`. FFI: `:3952 fluid_eval_short_capacity_is_invalid_argument_not_overflow`; `:4040 fluid_eval_panic_is_contained_without_output`. |

## 9. Fluid rescan

Exact private fields of `RescanView<'a>` (`fluid_rescan.rs:95–111`): `bytes: &'a [u8]`, `section_offsets: [usize;24]`, `section_uniform: [Option<u16>;24]`, `skirt_offset, metadata_offset: usize`, `center_x, center_z: i32`, `x0, x1, z0, z1, start_section: usize`, `budget: u32`. No typed production section, metadata or summary struct; `RescanBox` is test-only.

| Concern | Current fact and evidence |
|---|---|
| API | `parse_rescan_input(&[u8]) -> Option<RescanView>`; `fluid_rescan(&RescanView) -> Vec<u8>` (`fluid_rescan.rs:171,355`). |
| Dimensions | Fixed 24 sections of 16³ center blocks, total height 384; 68 skirt columns of 384 u16 each; metadata for 9 chunks × 24 sections. Box x/z 0..=17, center columns 1..=16; world y −64..319. No variable dimensions field (`fluid_rescan.rs:51–87`). |
| Layout | Header 26 bytes begins with u32 layout=1. Despite prose name `MFL1`, **there is no 4-byte magic**. Center x/z i32 at 4/8; x0/x1/z0/z1 u16 at 12/14/16/18; start-section u8 at 20; reserved21=0; budget u32 at22. Then 24 section records: uniform `(kind=0,pad=0,id:u16)` =4 bytes, or dense `(kind=1,pad=0,blocks:[u16;4096])` =8194 bytes. Then 52224 skirt bytes and 648 metadata bytes. Total `26 + sum(section_lengths) + 52224 + 648`, min52994, max249554 (`fluid_rescan.rs:168–248`). |
| Validation | Start<24. Closed ranges 0≤x0≤x1≤17 and same for z. Center×16, base+16, base−1 checked. Section kind 0/1, pad0. Metadata flag0/1; flag0 requires ID0. No block ID membership check, no budget cap (any u32 including0), no consistency validation between metadata and center/skirt (`fluid_rescan.rs:171–248`). |
| Admitted vs production range | Parser admits outer skirt scans; production scans only center columns 1..=16. Source water on the outer skirt can read a neighbor outside the box and panic in `skirt_column`, explicitly documented at `fluid_rescan.rs:38–41,134–145,308–311`. Thus not every parser-accepted input produces an ordinary result. |
| Output bound | Position = three i32 LE =12 bytes. Summary =8 bytes: spent u32, done u8, reserved[3]=0. For A=(x1−x0+1)(z1−z0+1), r=24−start: records≤16*A*r; bytes≤12*16*A*r+8. Admitted max 18×18×384 =124416 records /1493000 bytes. Production center max 16×16×384 =98304 records /1179656 bytes. These are derived loop bounds, not explicit count constants (`fluid_rescan.rs:335–408`). |
| Budget behavior | Checks spent≥budget before each section; an entered section completes fully. Uniform nonfluid costs1; uniform sealed source costs1; otherwise each requested cell costs1. Emits fluid cells except sealed sources. Budget0 returns spent0, done=false, summary only. A successful scan may be partial and report done=false; this is unrelated to failure atomicity. Maximum overshoot is4095 cells in production or5183 for admitted 18×18 ranges. Comment “at most4096” at:363 assumes center-only production scans. No next-section field; caller replays accounting (`:349–408`). |
| Publication | Stages full scan Vec. Like LOD, short capacity ⇒ OVERFLOW with exact required bytes, payload unchanged. Other errors give metadata0 subject to alias caveat (`ffi.rs:1235–1307`). |
| Tests | `fluid_rescan.rs:601 mfl1_layout_bytes_are_pinned`; `:618 parse_validates_header_contract`; `:659 parse_validates_section_records_and_total_length`; `:682 parse_validates_metadata_table`; `:785 section_fixed_point_below_uses_section_records_over_metadata`; `:820 unsealed_edge_sources_emit_through_skirt_column`; `:837 mixed_dense_section_emits_in_scan_order_with_world_coords`; `:878 budget_exhaustion_overshoot_and_resume_mirror_go`; `:921 world_bottom_source_reads_barrier_below`. FFI: `:4333 fluid_rescan_two_phase_overflow_is_exact_and_atomic`; `:4390 fluid_rescan_invalid_input_matrix_is_atomic`; `:4520 fluid_rescan_panic_is_contained_without_output`. |

## 10. Mesh and light

| Typed item | Exact fields/API |
|---|---|
| `MeshInput<'a>` | `section_origin_y: i32, air_id: u16, barrier_id: u16, blocks: &'a [u8], heights_present: &'a [u8], heights: &'a [u8], registry: RegistryView<'a>` (`input.rs:66–74`). |
| `RegistryView<'a>` | Private fields `entries: &'a [u8], visibility: &'a [u8], count: usize, words_per_row: usize`. Registry entries remain byte views (`input.rs:193–198`). |
| `LightScratch<'a>` | Private fields `levels: &'a mut [u8], queue: &'a mut [u32], tail: usize`. `new` does not validate slice lengths; `at` returns0 outside the volume (`light.rs:24–44`). |
| `Quad` | `x,y,z,w,h: u8, face: Face, material: u16, ao,light: u8, corners: [u8;4], back: bool`. Face values 0..7: NegX, PosX, NegY, PosY, NegZ, PosZ, PlantDiagA, PlantDiagB. `pack(self)->u64`; `unpack` only under cfg(test) (`quad.rs:74–134,188`). |
| Core APIs | `MeshInput::{parse,parse_structural}(&[u8])->Result<Self,InputError>`; `validate_registry(bool)->Result<(),InputError>`; `build_light(&MeshInput,&RegistryView,&mut LightScratch)->Result<(),light::MeshError>`; `mesh_section(&MeshInput,&LightScratch,&mut[u64])->Result<usize,greedy::MeshError>` (`input.rs:77–90,150`; `light.rs:65`; `greedy/mod.rs:62`). InputError: Input/Registry/Emission. Light errors: EmissionOutOfRange/QueueOverflow. Geometry error: OutputOverflow. |

| Concern | Current fact and evidence |
|---|---|
| Input layout | Header16 bytes: `MGM1`, section-origin Y i32 at4, registry count u16 at8, words/row u16 at10, air u16 at12, barrier u16 at14. Then 27×4096 u16 blocks (3³ sections),9 presence bytes,9×256 i16 column heights,20-byte registry entries,visibility words. Exact size `225817 + 20*R + 8*R*ceil(R/64)`; R in1..=96, words/row exactlyceil. Maximum229273 bytes. Center16³, neighbor local coordinates−16..31 (`input.rs:1–3,56,90–147,158–188,368–374`). |
| Registry entry | `id:u16, opaque:u8, emission:u8, material:[u16;6], fluid_height:u8, light_attenuation:u8, block_top_raw:u8, model:u8`, total20 bytes. **Current maximum is96 and model6 (bed) is accepted.** Earlier historical FFI comments saying80 or model6 rejected are stale (`input.rs:4–56,201–266`). |
| Structural validation | Magic, exact size, presence bytes0/1, registry count and row shape. No bound on section-origin Y or signed heights. FFI uniform-air center returns0 after structural parsing and **skips semantic registry validation and light**, but still requires valid scratch and full output capacity (`ffi.rs:291–365`). |
| Semantic validation | Air≠barrier; registry IDs strictly increasing and both sentinel IDs present; opaque≤1; emission≤15; fluid height≤14; attenuation≤1; block-top≤14; fluid height and block-top cannot both be nonzero; model≤6. No requirement that all block IDs be registered; no material/plant/model consistency, visibility consistency, or visibility padding check. Unknown IDs use conservative/default accessors (`input.rs:150–156,201–266,269–365`). Fluid heights1..6 are accepted although production comments expect7..14. |
| Scratch | Light volume48³=110592. Levels u8[L], queue u32[L], zero padding:552960 bytes. FFI requires at least that size and u64 alignment (8). Invalid or overlapping scratch⇒SCRATCH. Queue reused between sky/block passes. `build_light` clears levels and resets tail; errors may leave mutated scratch (`ffi.rs:76–94,263–272,308–329`; `light.rs:4–6,24–75`). |
| Output capacity | FFI requires at least6×4096=24576 u64s (196608 bytes), even for all-air center. Any less⇒OVERFLOW and count0. Output needs u64 alignment; entire declared capacity range checked. Production ordinary/short/fluid geometry≤6 per cell, plants4, standing torches4, wall torches3, beds5 (`ffi.rs:79,311–345`; `greedy/mod.rs:42–59`; `greedy/torch.rs:29–41`; `greedy/bed.rs:27–32`). |
| Bound caveat | The24576 bound assumes production registry consistency. Parser accepts model1 plus plant material. `mesh_plants` checks material independently of model, then `mesh_models` emits4 standing-torch quads, allowing8 per cell for that constructed input (`greedy/mod.rs:194–204,233–263`; `greedy/torch.rs:58–91,99–125`). This is code-derived, not executed here. Plant material on an axial face can also trigger `Quad::pack` assertions after earlier output writes (`quad.rs:134–165`). |
| Publication | Geometry fills caller output progressively, checking capacity per quad; late failure retains earlier writes. FFI catch only keeps output count zero on failure/panic. Earlier parsing errors avoid geometry writes; light may already have changed scratch on later errors. Success count identifies the valid prefix, unused output stays unchanged (`ffi.rs:248–260,350–376`; `greedy/mod.rs:166–187,245–263`). |
| Packing | `pack` asserts w/h1..16; plant faces require unit quads and zero corners; axial faces forbid plant materials and back=true; corner quads require unit size and corners≤15. Does not assert x/y/z≤15 because mesher constructs valid coordinates. Reserved bit63 is checked only by test-only unpack (`quad.rs:133–190`). |

Useful tests:

- Parser: `input.rs:481 accepts_exactly_max_registry_entries`; `:490 parses_unaligned_little_endian_input_without_typed_casts`; `:550 unknown_model_tags_are_rejected`; `:568 block_top_raw_readback_mutex_and_sentinel`; `:613 rejects_malformed_registry_and_overbright_emission`.
- Light: `light.rs:363 all_sources_fill_exact_queue_without_overflow`; `:371 one_short_queue_reports_overflow`; `:384 emission_sixteen_is_rejected`; `:437 queue_is_reused_between_sky_and_block_passes`; `:469 reused_scratch_clears_old_light_before_a_dark_build`; `:758 sky_queue_enqueues_each_cell_at_most_once_in_mixed_media`.
- Geometry: `greedy/merge_tests.rs:14 isolated_block_produces_six_unit_quads`; `:127 asymmetric_fixture_preserves_complete_face_slice_row_order`; `:186 one_short_output_reports_overflow`; `greedy/plant_tests.rs:244 a_section_full_of_plants_stays_within_the_fixed_bound`; `greedy/torch_tests.rs:330 a_section_full_of_torches_stays_within_the_fixed_bound`; `greedy/bed.rs:182 bed_cell_emits_half_height_slab_quads`. Water/farmland/section-boundary modules cover corresponding specialized geometry.
- FFI: `ffi.rs:2335 uniform_air_returns_without_touching_light_scratch`; `:2362 uniform_air_skips_unused_registry_semantics_and_light`; `:2425 uniform_air_still_rejects_structural_presence_error`; `:2454 ffi_publishes_six_quads_only_after_complete_mesh`; `:2616 invalid_arguments_and_inputs_return_exact_atomic_statuses`; `:2730 null_and_misaligned_buffers_are_rejected_atomically`; `:2878 overlapping_scratch_and_output_are_rejected_atomically`. Several “atomic” tests assert status/count rather than exercising every late failure with an output canary; names do not establish staged payload publication.

## Material distinctions

1. Current validators omit several possible typed invariants: physics prism coverage/tunable ranges; chunk/probe coordinate bounds; mesh registry cross-field consistency; rescan neighbor-safe scan ranges. These are implementation observations, not decisions about new API acceptance.
2. Eval has no small batch cap; raycast/probe cap at64 records; collision/physics at4096 cells; rescan is intrinsically bounded by18×18×384; LOD by64 columns and three steps; chunk by98304 cells; tree by128 records; mesh by production24576 quads and fixed scratch.
3. Rescan starts with layout u32=1, not magic. Mesh has magic but no layout-version field.
4. LOD/rescan report exact required size on overflow. Eval short capacity is INVALID_ARGUMENT. Tree accepts larger buffers without separate length metadata. Collision/step/chunk/probe/raycast require exact output length. Mesh requires a static minimum.
5. Some internal safe functions mutate supplied Rust slices before errors. Nine FFI families stage results; mesh FFI only stages count publication.
