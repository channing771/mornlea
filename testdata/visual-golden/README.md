---
doc_id: visual-golden-index
doc_revision: 2026-09-15.1
language: en
counterpart: README.zh.md
---

# Visual baseline index

[Chinese](README.zh.md)

This directory is the shared home for visual regression baselines. Every file in its baseline classes is a binary test fixture.

- `world/`: 31 windowless world-scene PNG baselines corresponding to `captureScenes` in `cmd/mornlea/capture/capture.go`.
- `motion/`: 11 process GIF baselines (4 passive-cow scripts and 7 motion demos). They verify presentation only and do not participate in comparison. Their producers are `passiveDeathGIFScripts` in `cmd/mornlea/capture/passive_death_scripts.go` and the individual motion-demo entry points, which capture frames by tick and encode with the standard library's `image/gif` package.
- `ui/`: 31 frontend UI-component baselines corresponding to `fixtureNames` in `packages/engine/crates/mornlea_client/frontend/visual/fixture-names.ts`.

The former `cmd/mornlea/capture/testdata/golden/` and `engine/crates/mornlea_client/frontend/visual/golden/` locations have been emptied and retain only empty directories. Producers no longer write to them.

## world (31 images)

Each filename is the scene name with a `.png` suffix. `captureScenes` is authoritative for scene definitions and order.

| Baseline file | Scene name | Description |
|---|---|---|
| `terrain-noon.png` | `terrain-noon` | A distant view of spawn terrain under fixed noon lighting, locking the brightest phase of the day/night pipeline. |
| `avatar-nametag.png` | `avatar-nametag` | Perspective and glyph rendering for a remote player and bilingual display-name tag in the noon world. |
| `debug-panel.png` | `debug-panel` | The high-altitude world backdrop while panel state is visible, proving that the headless path adds no panel pixels. |
| `skylight-tunnel.png` | `skylight-tunnel` | Sunlight descending through a skylight shaft and the light-to-dark transition across its walls. |
| `block-light-room.png` | `block-light-room` | Pure block-light attenuation in a sealed stone room at midnight. |
| `torch-night.png` | `torch-night` | Warm illumination and flame sprites from floor and wall torches in a midnight stone room. |
| `bed-night.png` | `bed-night` | Lit bed surfaces and half-height silhouettes for complete beds facing all four directions in a midnight stone room. |
| `materials-showcase.png` | `materials-showcase` | A material comparison of every block on a display stand under noon sunlight. |
| `target-block-feedback.png` | `target-block-feedback` | Highlight outline and positional feedback for the block hit directly in front of the camera. |
| `grass-closeup.png` | `grass-closeup` | Crossed quads and ground contact for rows of short grass on a nearby turf strip, providing a recognizable short-grass baseline. |
| `oak-grove.png` | `oak-grove` | Combined rendering of canopies, trunks, and the forest floor in an oak grove. |
| `sapling-growth.png` | `sapling-growth` | A nearby sapling and a distant runtime oak on the same grass support surface: the sapling uses the four-quad cutout path (crossed diagonal planes, open upper edge, and ground contact), while runtime tree geometry from `worldgen.TreeBlocks` produces a distinguishable trunk and canopy. |
| `ai-companion.png` | `ai-companion` | An AI companion's follow position and presentation state in the noon world. |
| `sword-combat.png` | `sword-combat` | Combat feedback combining a sword attack pose and an authoritative hit marker in the same frame. |
| `hostile-mob.png` | `hostile-mob` | Positions and hit/chase states of nocturnal mobs at the edge of a torch-lit pool on midnight grassland. |
| `ranged-mob.png` | `ranged-mob` | A bone-white bipedal bone thrower attacking the target player on midnight grassland: two bone spikes are fixed mid-flight and oriented along their initial velocities, with the target player's back and name tag in frame. |
| `passive-herd.png` | `passive-herd` | Positions and drop phase for three textured cows and one textured raw-beef drop on noon grassland. |
| `passive-graze.png` | `passive-graze` | Pose comparison between a grazing cow and a normal cow on noon grassland, plus the single block changed from grass to dirt in front of the grazing cow's muzzle. |
| `water-surface-slope.png` | `water-surface-slope` | A top-down view of water-surface height slopes and pool-floor material visible through the water. |
| `bucket-pond.png` | `bucket-pond` | One source-water block, one empty block, and one hydrated-farmland block on a nearby turf strip as visual evidence before and after bucket actions. |
| `mining-crack-early.png` | `mining-crack-early` | An early-stage world-space mining crack on the same target brick block. |
| `mining-crack-heavy.png` | `mining-crack-heavy` | The heaviest crack stage on the same target brick block, visibly deeper than the early stage. |
| `rain-noon.png` | `rain-noon` | A fixed summer-solstice noon rain fixture whose compensated display phase is exactly 6000 and whose sky and sunlight are byte-identical to the solstice baseline: rain particles, gray sky, and dimmed outdoor light share the frame; precipitation form is derived per particle from the temperature formula, and every visible particle is rain. |
| `camera-third-back.png` | `camera-third-back` | Rear third-person view from the shared camera: the player's body and back are visible and no viewmodel is present. |
| `camera-third-front.png` | `camera-third-front` | Front third-person view from the shared camera: the player's body and face are visible and no viewmodel is present. |
| `snow-cover.png` | `snow-cover` | A fixed midwinter-noon snow fixture (`Winter/128` plus a 9454 day-arc phase compensation, yielding seasonal phase 6000): four pre-laid snow-depth zones, snow-form precipitation particles, and the cold winter sky tint share the frame; precipitation form is derived per particle from the temperature formula, and every visible particle is snow. |
| `main-menu.png` | `main-menu` | The main-menu panorama backdrop, showing only the panoramic world at a fixed rotation time. |
| `settings-menu.png` | `settings-menu` | The settings panorama backdrop at a different rotation time in the same panoramic world. |
| `avatar-detail.png` | `avatar-detail` | Front, side, and rear views of the original traveler together, validating clothing material and static proportions. |
| `far-horizon.png` | `far-horizon` | A high-altitude distant view composed of near terrain, the far-ring shell, fog transition, and sky. |
| `water-underwater.png` | `water-underwater` | A viewpoint with the camera eye submerged, showing both the water-color overlay and attenuation through water. |

## ui (31 images)

Each filename is the fixture name with a `.png` suffix. `fixtureNames` in `fixture-names.ts` is authoritative for the list, and the registry in `fixtures.tsx` is authoritative for the corresponding component.

| Baseline file | Fixture name | Component or state |
|---|---|---|
| `panel-inventory.png` | `panel-inventory` | Personal inventory with a valid 2×2 stone-brick recipe. |
| `panel-inventory-empty.png` | `panel-inventory-empty` | Empty inventory. |
| `panel-inventory-full.png` | `panel-inventory-full` | Inventory filled to item stack limits, with tools represented as valid single items at half durability. |
| `items-all.png` | `items-all` | Every item icon and name in the production registry. |
| `hud-hotbar-first.png` | `hud-hotbar-first` | First hotbar slot selected with adjacent-slot lift. |
| `hud-hotbar-last.png` | `hud-hotbar-last` | Last hotbar slot selected with adjacent-slot lift. |
| `panel-workbench.png` | `panel-workbench` | Valid 3×3 stone-hoe ingredients and result. |
| `panel-chest.png` | `panel-chest` | Chest and unified inventory. |
| `panel-furnace.png` | `panel-furnace` | Three furnace slots plus smelting and burning progress. |
| `panel-character.png` | `panel-character` | Read-only character page. |
| `panel-inventory-narrow.png` | `panel-inventory-narrow` | Scrolling inventory in a small window. |
| `panel-main-menu.png` | `panel-main-menu` | Full-screen `MainMenu`. |
| `panel-settings.png` | `panel-settings` | Full-screen `SettingsPanel`. |
| `panel-pause.png` | `panel-pause` | Full-screen `PauseMenu`. |
| `panel-loading.png` | `panel-loading` | Full-screen `LoadingScreen`. |
| `panel-debug.png` | `panel-debug` | Complete row set for `DebugPanel`. |
| `button-default.png` | `button-default` | Enabled main-menu state for `PixelButton`. |
| `button-disabled.png` | `button-disabled` | Disabled main-menu state for `PixelButton`. |
| `button-pressed.png` | `button-pressed` | Selected window-preset state for `PixelButton`. |
| `input-text.png` | `input-text` | Read-only texture-pack path in `PixelInput`. |
| `preset-group.png` | `preset-group` | Three-button window-preset group using `PixelButton`. |
| `slider.png` | `slider` | Volume slider for `input.settings-slider`. |
| `debug-rows.png` | `debug-rows` | Minimal four-row set for `DebugPanel`. |
| `error-line.png` | `error-line` | Error line rendered by `p.menu-error`. |
| `hud-hotbar.png` | `hud-hotbar` | `HudRoot` with only the hotbar. |
| `hud-status.png` | `hud-status` | Complete `HudRoot` status stack for health, hunger, oxygen, and the eating track. |
| `hud-armor.png` | `hud-armor` | Half-step armor bar in `HudRoot`: an authoritative value of 7 displays three full icons and one half icon, with the row immediately above the heart row. |
| `hud-progress.png` | `hud-progress` | Eating-progress track in `ProgressTrack`. |
| `hud-popup-crosshair.png` | `hud-popup-crosshair` | `HudRoot` item popup, crosshair, and hit marker in one frame. |
| `hud-chat.png` | `hud-chat` | Multi-line chat in `HudRoot`. |
| `hud-container-open.png` | `hud-container-open` | Reversed `HudRoot` composition while a container is open. |

## motion (11 files)

Motion-process GIFs verify presentation only and do not participate in automated pixel comparison. `make visual-check` compares only the PNG files in `world/`. GIF scripts have no threshold and are excluded from comparison; comparison-only runs do not generate them by default, and they are generated only when `GIFS=1` is requested explicitly or when baselines are updated. The rules for the 31 `world/` PNG files do not include these GIFs.

| Demo file | Scene | Frames/duration | Generation entry point |
|---|---|---|---|
| `break-burst.gif` | `break-burst-motion`: full mining lifecycle—F0–4 target idle; F5–24 mining ramp, completing crack stages 0→9; F25 destruction in the same frame, clearing the target and mining state and injecting a dirt drop; F25–44 persistent particles and a falling drop, whose three-block gravity integration takes about 9 ticks and lands at F34; F34–49 settled drop retained | 50 frames, 0.13 seconds each, approximately 6.5 seconds per loop | `go run ./packages/client/cmd/mornlea --motion-demo testdata/visual-golden/motion/break-burst.gif` (run from the repository root) |
| `avatar-walk.gif` | Idle → slow walk → fast walk → settled stop; the 40-tick slow walk and 20-tick fast walk each cover 4.3 blocks | 100 frames, 20 Hz, 5 seconds | Add `--motion-scene avatar-walk` to the preceding command and change the output to `avatar-walk.gif` |
| `drop-scatter.gif` | Empty pre-trigger scene → four stacks spawning on their authoritative frame → scattering and falling → landing | 80 frames, 20 Hz, 4 seconds | Add `--motion-scene drop-scatter` to the preceding command and change the output to `drop-scatter.gif` |
| `drop-density.gif` | Empty scene → 1 → 4 → 9 → 16 → 32 stacks → remove half to leave 16 → steady state | 160 frames, 20 Hz, 8 seconds | Add `--motion-scene drop-density` to the preceding command and change the output to `drop-density.gif` |
| `hand-mining.gif` | Iron pickaxe held with a constant early crack stage; the right hand performs 12 sinusoidal swings on the pickaxe's 10-tick period | 120 frames, 20 Hz, 6 seconds | Add `--motion-scene hand-mining` to the preceding command and change the output to `hand-mining.gif` |
| `hand-attack.gif` | Iron sword held; synthesize one confirmation edge every 12 frames (6 swinging frames and 6 neutral frames) for 10 complete swings | 120 frames, 20 Hz, 6 seconds | Add `--motion-scene hand-attack` to the preceding command and change the output to `hand-attack.gif` |
| `graze.gif` | `graze`: before and after grazing—6 normal standing frames followed by 6 grazing frames, with a normal cow as reference | 12 frames, approximately 8 fps | Generated with `make visual-update` by `RunPassiveDeathGIFs` inside `RunCapture` (pixel comparison is retired; generation is retained) |
| `lure.gif` | `lure`: approach while holding wheat—the remote player moves toward a stationary cow frame by frame, with a wheat drop in front of the cow | No more than 48 frames, approximately 8 fps | Same as above |
| `kill.gif` | `kill`: kill event—on frame 4 the death despawn creates a raw-beef drop, followed by a 20-frame retained red-flash and side-fall period | No more than 48 frames, approximately 8 fps | Same as above |
| `beef-drop.gif` | `beef-drop`: raw-beef drop—a single raw-beef item floats and rotates from authoritative-tick-derived state | No more than 48 frames, approximately 8 fps | Same as above |
| `weather-cycle.gif` | `weather-cycle`: complete weather transition—24 clear frames → 24 rain frames → 24 thunderstorm frames including flash frames → 24 clear frames, with particle motion and sky-gray transitions in the same view | 96 frames, 0.13 seconds each, approximately 12.5 seconds per loop | `go run ./packages/client/cmd/mornlea --motion-demo testdata/visual-golden/motion/weather-cycle.gif --motion-scene weather-cycle` (run from the repository root) |

- Raw key PNGs from new demos are written to a side review directory formed by appending `-frames/` to the output path. They are not goldens; the GIF is the complete process, and the PNGs only help reviewers confirm encoding fidelity.
- Demo scene values live in `packages/client/cmd/mornlea/capture/motion_break_burst.go`, `motion_experience.go`, `motion_hand_swing.go`, and `motion_weather_cycle.go`. They are not appended to `captureScenes`.
- Encoding uses only the standard library's `image/gif` package, with one adaptive palette shared by the entire animation and no dithering. Fixed input produces byte-identical output.

## Boundaries and selection rules for the three classes

The three families are routed by observable subject and time dimension. Each registry remains authoritative for filenames and counts:

- Class 1, window/UI (`ui/`): component-level PNGs for the window and WebView layers. `fixtureNames` in `fixture-names.ts` is authoritative for the registry. A local Chrome instance captures them, the existing dual-threshold comparison applies, and the check does not run in CI.
- Class 2, stable world (`world/`): one stable frame after convergence from windowless offscreen rendering. `captureScenes` is authoritative for scene definitions and order. `make visual-check` compares them, and `make visual-update` explicitly overwrites them.
- Class 3, process GIF (`motion/`): complete cross-tick state-transition GIFs captured by tick, covering the pre-trigger, outcome, and settled phases rather than only an excerpt. They are bounded human-review evidence, verify presentation only, and participate in no automated pixel-comparison gate. The four passive-cow scripts are generated by `RunPassiveDeathGIFs` inside `RunCapture` during `make visual-update`; the seven motion demos are generated explicitly through `--motion-demo`.

Routing discipline: window chrome belongs only in `ui/`, single world frames belong only in `world/`, and temporal processes belong only in GIFs. World frames must not contain window-chrome pixels, and UI fixtures must not recreate world pixels.

The `avatar-detail` PNG fixes the front/side/rear materials and static proportions, while the `avatar-walk` GIF reviews distance-driven gait and settling. Their responsibilities are distinct. The four former GPU container scenes have moved to same-named frontend `panel-*` fixtures; world images do not carry panel content.

Deduplication discipline: do not store the same behavior as both a PNG and GIF. Existing pairs with distinct responsibilities—such as a gated sample point versus a human-reviewed full process, including the two crack frames and the mining demo—must record the distinction in this section. A feature's own change must consolidate any new overlap according to this section; this index defines the rule but does not modify existing baselines.

## Update entry points and discipline

- World baseline comparison uses `make visual-check`. It compares only `world/` PNG files; GIF scripts are excluded, comparison-only runs do not generate them by default, and `GIFS=1` requests them explicitly. `make visual-update` is the overwrite entry point. Path constants are `captureGoldenDir` in `capture/capture_image.go` and `passiveDeathMotionDir` in `capture/passive_death_gif.go`.
- Component baseline comparison uses `make frontend-visual-check`, or `corepack pnpm visual-check` from `frontend/`. Its overwrite entry point is `make frontend-visual-update`, or `corepack pnpm visual-update`. The `goldenDir` value in `visual/visual.mjs` derives the baseline directory from `repoRoot`.
- Inspect before overwriting: update baselines only after every expected visual change has been reviewed manually. Ordinary validation compares without accepting differences automatically. When pixels drift, inspect the captured and difference images before deciding whether to fix the code or update the baseline. A missing baseline must fail and must never be created silently; an update must be requested explicitly.
- Comparison uses two thresholds. Their definitions and values are owned by the comparison functions in `capture/visual_compare.go` and `visual/visual.mjs`; this document intentionally does not duplicate the numeric values.
