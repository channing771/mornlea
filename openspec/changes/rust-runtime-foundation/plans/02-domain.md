# Shared domain design and implementation plan

**Goal:** one valid-value model and complete immutable semantic messages. **Architecture:** dependency-free domain values are consumed by protocol/storage; transport decoding and historical save DTOs remain outside domain. **Tech stack:** Rust std, private fields, checked constructors, fixed arrays and compact palettes. **Spec:** shared coverage, ordering and aggregate validity. **Global constraints:** `../execution-contract.md` and the entire field/rule table in `../packet-contracts.md`. **Review focus:** a wire-valid request is not yet a successful authoritative action; no new movement, crafting, visibility or chat authority is implemented here.

`DOMAIN` is `packages/engine/crates/mornlea_domain`. Every node modifies its named source file, adds a corresponding `DOMAIN/tests/runtime_contract/<topic>.rs`, and adds the module declaration to the existing integration root through the controller. `DOMAIN/src/lib.rs` exports and `DOMAIN/AGENTS.md` have one serial integration owner. This topic directory inherits the crate guide; it does not own a new runtime boundary. Run from repository root:

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract --locked
rustup run 1.97.1 cargo clippy --manifest-path packages/engine/Cargo.toml -p mornlea_domain --all-targets --locked -- -D warnings
```

For each node also run this test command with its named topic filter, and discover that filter with `-- --list`. Every node adds the listed cases before implementation. Existing compiling tests that already satisfy a case are retained as regression evidence; a missing API/import is registration evidence only. Corpus comparison is required before closing a behavior node. Use commit `feat(domain): <node's concrete behavior>` or `fix(domain): ...`, one English line; rollback removes that node and its consumers together, preserving prerequisite values and frozen fixtures.


Every node2.2–2.13 also owns `packages/tools/cmd/runtime-oracle/domain_<topic>_test.go` and `testdata/runtime-migration/cases/domain/<topic>/`, where `<topic>` is its explicitly named Rust filter (identity_values,items_locations,command_control,command_inventory,command_order,event_player,event_world,event_inventory,event_people,event_mobs,event_objects,event_chat). Add Go `TestDomainOracle_<topic>` calling current core identity/item validation or current protocol DTO Validate/codec APIs and normalizing by the common field map below. Ordering additionally uses the actual package-local Step producer in2.6. This means each node can close on independent evidence without waiting for a later protocol conversion node. Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestDomainOracle_<topic>$' -count=1` with that literal topic substituted; do not register empty placeholder producers. The2.3 existing named TestDomainValuesOracle can remain as a wrapper to the same table, not a second source of expected results.

## Common representation and constructor contract

Types named below are new unless explicitly existing. All invariant-bearing records have private fields, `try_new(parts: <Type>Parts) -> Result<Self,DomainError>` and read-only getters named after fields. Their corresponding `Parts` has public fields of exactly the types listed below; validation happens once on construction. Plain coordinates and grouped boolean controls need no private wrapper. Owning collections use `Box<[T]>`; fixed cardinalities use `[T;N]`. Constructors accept owned storage, inspect count before copying, and retain the allocation without cloning. No unchecked public constructor, mutable slice getter or serde derive. All three scalar entity IDs are distinct types wrapping nonzero u64. `Tick`, `SessionId`, `Sequence` and `ArrivalIndex` are u64 aliases; they are not universally nonzero.

The wire-to-domain field naming rule is fixed: CamelCase source fields become snake_case; replace `{X,Y,Z}` with `BlockPos`, `{X,Z}` with `ChunkPos`, validated UUID with the appropriate ID, all ordinary `(Item,Count,Durability)` triples with `ItemStack`, and every finite position/velocity triple with `FiniteVec3`. Do not add/drop fields unless a row below explicitly groups them. Counts, varints, reserved bytes and wire sentinel storage are not fields in semantic records. This is a mechanical field map over the complete table, not permission for a worker to redesign messages.

<a id="node-2-2"></a>

## 2.2 — Identity, text and scalar values

Dependencies: 1.5. Own `identity.rs`, `values.rs`, new `text.rs`, tests `identity_values.rs`. Add `PlayerId` and `CompanionId` checked UUIDv4 wrappers; `HostileId`, `PassiveId`, `ProjectileId` checked nonzero u64 wrappers; `Dimension` 0/1; `HotbarSlot` 0..8; `FiniteVec3([f32;3])`; `LookAngles {yaw,pitch}` finite only; `DisplayName`, `CompanionName`, `CommandText`, `SpeechText` owned String wrappers. Existing `Identities` version bookkeeping remains until replay helpers move; it must not become event payload data.

Identity methods: `try_from_bytes([u8;16])`, `bytes() -> [u8;16]`; scalar ID `try_new(u64)`, `get()`. Text methods: `try_from_canonical(String)`, `as_str()`. Display-name normalization is **not** implicit in this canonical constructor; admission performs trim before it. Pin Go Unicode whitespace/control behavior: use an explicit whitespace predicate for U+0009..000D,0020,0085,00A0,1680,2000..200A,2028,2029,202F,205F,3000 and control U+0000..001F/007F..009F. This avoids drifting with language Unicode versions. Non-whitespace format characters remain as accepted by Go; no NFC normalization.

Tests: UUID zero/wrong v4/wrong variant/valid `00112233-4455-4677-8899-aabbccddeeff`; type distinction at compile time; 0/1/2 dimension; slot8/9; ±infinity/NaN and negative zero bit preservation; canonical `Alice`, 32/33 scalars, 128/129 bytes, whitespace at either edge, U+0085, U+200B, embedded control; `CompanionName("A B")` fails while display name allows its interior space. Command1024/1025 bytes and speech256/257. No NONE companion identity. Topic filter `identity_values`.

<a id="node-2-3"></a>

## 2.3 — Item, drop and container values

Dependencies: 2.2, 1.5. Own new `items.rs`, `locations.rs`, tests `items_locations.rs`, and new Go `packages/tools/cmd/runtime-oracle/domain_values_test.go` (after 1.2 for its imports). Add exact tables from `packet-contracts.md`: `item_stack_limit(u16)->Option<u8>`, `durability_max(u16)->Option<u16>`, `smelting_output(u16)->Option<u16>`, `is_smelting_product(u16)->bool`; one table is authoritative in Rust. `ItemStack::try_new(item:u16,count:u8,durability:u16)`, `EMPTY` and getters. `DropId::try_new(dimension:i32,chunk:ChunkPos,slot:u8,generation:u32)` retains arbitrary raw dimension, slot<32, generation>0. `ContainerKind {Furnace,Chest}` and `ContainerRef::try_new(chunk:ChunkPos,kind:ContainerKind,slot:u8,generation:u32)` are inherently overworld; protocol conversion validates raw dimension before construction. Absence is `Option<ContainerRef>`, never an invalid domain reference.

Tests exhaust IDs0..66, all count boundaries and durable maxima from the source table; empty must be exactly(0,0,0). Broken equipped armor is outside this type. Test drop dimension−1 is accepted, slot31/32, generation0/1; furnace slot31/32, chest15/16; ID ordering uses dimension/chunk/slot/generation. Go adapter invokes current core item/ID validators for every table row, producing `domain.values/current/<label>`. Topic `items_locations`; also `go test ./packages/tools/cmd/runtime-oracle -run '^TestDomainValuesOracle' -count=1`.

<a id="node-2-4"></a>

## 2.4 — Movement and ray command payloads

Dependencies: 2.3. Own `input.rs`, new `input/control.rs`, tests `command_control.rs`. Replace the ten-argument PlayerInput constructor with grouped `Movement {move_x:i8,move_z:i8,jump:bool}`, `HeldActions {primary:bool,eating:bool,sprinting:bool,sneaking:bool}`, and `PlayerControlParts {movement:Movement,look:LookAngles,actions:HeldActions}`. `primary` maps exactly to Go Mining; it does not imply that mining wins over combat. `PlayerControl` accepts full i8 axes because authority validates −1..1 later.

The `Command` variants introduced here are `PlayerInput(PlayerControl)`, `PlaceBlock(PlacementIntent {look:LookAngles,slot:HotbarSlot})`, `Resync(ResyncIntent {dimension:Dimension,chunk:ChunkPos,have_revision:u64})`, `SelectHotbar(HotbarSlot)`, `OpenContainer(LookAngles)`, `TillSoil(LookAngles)`, `BoneMeal(LookAngles)`, `CollectWater(LookAngles)`, `PlaceWater(LookAngles)`. Remove the old domain PlayerInput/PlaceBlock/SelectHotbar constructor wrappers and sequence-bearing payloads in this node, updating their existing domain tests. This node additionally owns the mechanical validation call in `packages/engine/crates/mornlea_protocol/src/player_input.rs`: replace DomainPlayerInput::new with domain LookAngles::try_new(yaw,pitch), mapping failure to ProtocolError::InvalidFloat. Its wire DTO and codec layout stay unchanged. PlaceBlock/SelectHotbar protocol already use HotbarSlot directly. This keeps consumers compiling without a temporary ten-argument domain constructor or a clippy exemption. Keep the old SemanticInput/order_inputs test facade until2.6 replaces its last tests; it may wrap the new payload plus sequence locally, not duplicate validation. No target cell, hit entity, placed block, consumed item or outcome is client supplied.

Tests: axes−128/127 and pitch4.0 remain representable; NaN fails LookAngles; all four held flags round-trip independently; place slot8 succeeds/9 fails; resync dimension1 and revision0 accepted; ray intents share the same finite angle rule and retain signed zero. Seed input has move_x=1,move_z=−1,jump=true,look=(0.5,−0.25), all held flags true, then toggle each field and assert it appears in normalized output. Topic `command_control`; also run the existing protocol runtime_contract to validate this mechanical consumer update.

<a id="node-2-5"></a>

## 2.5 — Inventory, container and chat intents

Dependencies: 2.3, 2.4. Own new `input/inventory.rs`, `input/chat.rs`, enum integration in `input.rs`, tests `command_inventory.rs`. Add `MoveInventory(InventoryMove {from:u8,to:u8})`, `MoveCrafting(CraftingMove {from:u8,to:u8})`, `MoveContainer(ContainerMove {container:ContainerRef,from:u8,to:u8})`, `CloseContainer`, `DropSelectedItem`, `TakeCraftingOutput`, `EquipArmor`, `MovePartial(PartialMove {view:StackView,from:u8,to:u8,single:bool})`, `QuickMove(StackSource {view:StackView,slot:u8})`, `DropStack(StackSource)`. This yields exactly 19 Command variants. `StackView {Inventory,Crafting,Container(ContainerRef)}` replaces raw view numbers plus sentinel reference. Chat is `ChatIntent {text:CommandText}` outside Command.

Use the exact per-command source/destination bounds in packet-contracts. In particular partial movement in crafting may have both indices>=9, and partial movement may target furnace output38 at this boundary; do not reuse the stricter MoveCrafting/MoveContainer validators for it. TakeCraftingOutput's nonzero sequence is checked by envelope construction in 2.6, since it has no payload sequence. `ChatIntent` performs no addressing, warp, stop or queue policy.

Tests: inventory35/36 and same slot; crafting8→44 accepted,9→10 rejected for MoveCrafting but accepted for PartialMove; furnace38→0 accepted,0→38 rejected for MoveContainer but accepted for PartialMove; chest62/63; split `single=false/true` preserved; malformed real ref rejected before slot checks; non-container raw sentinel acceptance is adapter work. Chat `@Bob 停止` retained verbatim with no fabricated sequence. Topic `command_inventory`.

<a id="node-2-6"></a>

## 2.6 — Envelope ordering and Go authority counterexample

Dependencies: 2.5, 1.5. Own new `input/order.rs`, tests `command_order.rs`, new `packages/server/sim/runtime/command_order_oracle_test.go`. Read that subtree's guide; the Go file reuses the existing engine ordering test setup and calls the actual `Step`, not a copied sort comparator. It exports the admitted first command/outcome and order using the package-local test schema from 1.5.

`CommandEnvelopeParts` has tick,session,sequence,arrival_index:u64 and command:Command. `try_new` rejects sequence0 only for TakeCraftingOutput. `CommandOrderScratch::try_with_capacity(max_commands:usize)->Result<Self,DomainError>` owns a Vec of `(u64,u64,u64)` key slots. `order_commands(commands:&mut [CommandEnvelope],scratch:&mut CommandOrderScratch)->Result<(),DomainError>` first rejects insufficient scratch; fills/sorts key scratch by `(tick,session,arrival_index)` and rejects duplicates **without changing commands**; then `sort_unstable_by_key` by `(tick,session,sequence,arrival_index)`. The validated final key is unique, making unstable sort deterministic. No key uses kind. Warm operation allocates zero; domain neither filters sequences nor checks session generation.

Exact red input, in this arrival order at tick7: session2/seq1/arrival0 SelectHotbar(0); session1/seq9/arrival0 SelectHotbar(2); session1/seq9/arrival1 PlaceBlock(slot0); session1/seq8/arrival2 SelectHotbar(1). Output order is session1 seq8,session1 seq9 arrival0,session1 seq9 arrival1,session2 seq1. Current lexical-kind sort chooses the wrong seq9 winner; Go authority accepts the earlier SelectHotbar and discards the later same-sequence PlaceBlock. Add reverse-arrival case, same-kind duplicates, same sequence across sessions, two ticks, duplicate arrival error with unchanged input, empty batch, exact/short scratch and scratch reuse after failure. Remove the obsolete production SemanticInput/order_inputs compatibility facade and update its remaining tests here. Scope excludes changing Step or session admission.

Run domain `command_order`; `go test ./packages/server/sim/runtime -run 'TestCommandOrderOracle|TestEngineSortsCommandsAndDeduplicatesSequence' -count=1`. A focused Go command involving Rust is preceded by `make rust` on a clean checkout. Case `domain.input/current/session-sequence-arrival` compares full normalized commands and selected outcome.

<a id="node-2-7"></a>

## 2.7 — Command outcomes and player publications

Dependencies: 2.2, 1.5. Own new `event/player.rs`, `event/outcome.rs`, tests `event_player.rs`. Define `CommandRejection {sequence:u64,reason:RejectReason}`, `PlacementSuccess {sequence:u64}`, `CombatHit {server_tick:u64,damage:u8,target:CombatTarget}`. RejectReason variants are exactly the 15 named reasons in packet-contracts; no discriminant cast determines wire values. CombatTarget is Player/Hostile/Passive. Sequence0 valid for rejection/success; combat tick>0 and damage1..20.

Define `MotionState {position:FiniteVec3,velocity:FiniteVec3,on_ground:bool}`, `MiningState {Idle,Active(ActiveMining)}` where ActiveMining has private target:BlockPos,progress:u16,required:u16,harvestable:bool and its checked Parts constructor requires progress>0 and progress<required. `SurvivalState` fields are health:u8,oxygen:u16,hunger:u8,saturation_zero:bool,armor_points:u8 with limits20/300/20/20. `WorldState` fields day_phase_offset:u16,world_time_ticks:u64,weather:Weather,season:Season,season_progress:u8,temperature:i8; Weather=Clear/Rain/Thunder, Season=Spring/Summer/Autumn/Winter, offset<24000. `PlayerStateParts` fields server_tick:u64,last_input_sequence:u64,dimension:Dimension,motion:MotionState,look:LookAngles,ready:bool,reset:bool,mining:MiningState,survival:SurvivalState,world:WorldState. No private inventory or equipped armor added.

Tests: seed all finite vectors zero, health20,oxygen300,hunger20,armor20, offset23999, time0, clear/spring, season_progress255, temperature−128; accepted. Each max+1 rejected except full-range progress/temperature. Inactive wire mining with nonzero target must fail conversion; active1/2 accepted,0/2 and2/2 rejected. Reject/success seq0 preserved. The node's Go producer normalizes existing protocol.PlayerState/CommandRejected/PlaceBlockSucceeded/CombatHit values directly using the frozen field map; semantic tests compare all fields, not a digest. Later protocol nodes add wire-adapter integration, not a prerequisite for this domain node. Topic `event_player`.

<a id="node-2-8"></a>

## 2.8 — Compact chunk observations

Dependencies: 2.2. Own new `sections.rs`, `event/world.rs`, tests `event_world.rs`. Implement PalettedSection exactly as execution-contract; explicit constructors `single(block:u16)`, `indexed(bits:u8,palette:Box<[u16]>,words:Box<[u64]>)`, `direct(words:Box<[u64]>)`, all fallible. Accessors expose slices, never mutable storage. Direct word high four bits must be zero; 4/8-bit formats have no unused high bits or tail entries for4096 cells.

`ChunkSnapshotParts {dimension:Dimension,chunk:ChunkPos,revision:u64,sections:Box<[PalettedSection;24]>}` requires nonzero revision. Y is implicit in array order. `BlockChange {position:BlockPos,block:u16}` validates block<90; `BlockChangesParts {dimension,chunk,base_revision:u64,new_revision:u64,changes:Box<[BlockChange]>}` validates base1..u64::MAX−1, new=base+1, each Y−64..319, matching chunk and strictly increasing local index. Domain owns these semantic relations; the 4096 wire batch limit is applied by protocol. `ForgetChunksParts {dimension,chunks:Box<[ChunkPos]>}` requires unique nonempty positions, preserving order; protocol owns the4096 cap. Do not sort/repack during conversion.

Tests: Single air; Indexed4 palette[0,1] with one cell1; Indexed8 90 registered IDs; Direct15 block89 at last cell; counts255/256/257 for 4-bit words; duplicate palette, palette90, absent index, direct high bits, section23 preserved; changes empty revision barrier accepted; negative chunk(−1,−1) and world(−1,−64,−1) local index; duplicate/unsorted changes; revision overflow; forget reversed unique order preserved. Checked Box conversion avoids a large stack array. Topic `event_world`.

<a id="node-2-9"></a>

## 2.9 — Inventory and container publications

Dependencies: 2.3. Own new `event/inventory.rs`, tests `event_inventory.rs`. `InventoryStateParts {selected:HotbarSlot,hotbar:[ItemStack;9],backpack:[ItemStack;27]}`. `CraftingStateParts {size:CraftingSize,slots:[ItemStack;9],output:ItemStack}` where size Personal/Workbench and Personal slots4..8 EMPTY. `FurnaceStateParts {container:ContainerRef,input:ItemStack,fuel:ItemStack,output:ItemStack,progress_ticks:u8,burn_ticks:u16}` requires furnace kind, progress<200,burn<=1600,input empty or IDs6/18/27/53,fuel empty or5,output empty or7/23/24/54. `ChestStateParts {container:ContainerRef,items:[ItemStack;27]}` requires chest. `ContainerClosed {container:ContainerRef}` permits either kind. No timer consistency rule beyond Go validation.

Tests: all-empty selected8 accepted; personal slot4 nonempty rejected; workbench slot8 allowed; each smelting product; fuel stone rejected; progress199/200,burn1600/1601; chest closure accepted despite historical Go FurnaceEnd name; no equipped armor slots added. Topic `event_inventory`.

<a id="node-2-10"></a>

## 2.10 — Remote-player and companion observations

Dependencies: 2.2. Own new `event/people.rs`, tests `event_people.rs`. Types exactly follow RemotePlayerSpawn/Despawn/States and CompanionSpawn/Despawn/States rows in packet-contracts using the common naming map; batch record types `RemotePlayerState`, `CompanionState` omit the enclosing tick. Spawn names use DisplayName/CompanionName, and batched records own ID,dimension,position,look,reset. Batch structs own `server_tick:u64` and `states:Box<[...]>`; source Tick is renamed server_tick. Spawn flattened yaw/pitch becomes look. Despawn is a checked ID. Domain requires nonempty strictly increasing IDs; packet adapters enforce7/4 batch maxima. Companion dimensions0 and pitch inclusive±pi/2 remain a per-record invariant; remote pitch4.0 is valid.

Tests use UUIDs ending ff and fe sorted lexicographically; reverse/duplicate IDs fail; zero tick works; companion dimension1,zero-ID,spaced name,pitch next float above pi/2 fail; remote corresponding pitch accepted; no profile/persona/mining/velocity leaked into companion records. Topic `event_people`.

<a id="node-2-11"></a>

## 2.11 — Hostile and passive observations

Dependencies: 2.2. Own new `event/mobs.rs`, tests `event_mobs.rs`. Types and entire fields are HostileSpawnRecord, HostileStateRecord, HostileDespawn IDs, PassiveSpawnRecord, PassiveStateRecord, PassiveDespawnRecord from packet-contracts. Replace Kind with `HostileKind {Nightwalker,BoneThrower}`, Grazing with bool, passive reason with `PassiveDespawnReason {Vanished,Died}`. Single records enforce nonzero typed IDs, dimension0 where present, finite pose/velocity, health1..20. Batch records own server_tick and boxed records, enforce nonempty/strict numeric IDs; **domain does not impose authority32 or wire64**. Protocol supplies cap64. No cooldowns,target IDs or path state are inserted.

Tests health0/1/20/21, kind0/1/2, grazing raw0/1/2 adapter conversion, reason0/1/2, IDs1/2 vs2/1, dimensions1 rejected only on spawn, no dimension synthesized in state. Topic `event_mobs`.

<a id="node-2-12"></a>

## 2.12 — Projectile and item-drop observations

Dependencies: 2.3. Own new `event/objects.rs`, tests `event_objects.rs`. Define ProjectileSpawnRecord/StateRecord/Despawn and ItemDrop/ItemDropUpserts/ItemDropRemoves with exact fields in packet-contracts. ProjectileKind=Shard/Arrow; spawn dimension0/1; state has ID and position only. ItemDrop groups Item/Count/Durability into ItemStack and retains `block_index:u32<98304`; DropId uses its raw-dimension rule. Batches require nonempty strictly increasing respective IDs, with no reordering; packet adapters apply128/32 limits.

Tests projectile ID0,kind2,nonfinite velocity; both kind×dimension combinations valid; state does not duplicate kind/velocity; drop dimension−1 remains valid; block98303/98304; **empty drop stack remains representable**, because Go wire accepts it; stale/duplicate ID ordering fails. Topic `event_objects`.

<a id="node-2-13"></a>

## 2.13 — Chat tagged union

Dependencies: 2.2. Own new `event/chat.rs`, tests `event_chat.rs`. `ChatEventParts {event_id:u64,player_id:PlayerId,player_name:DisplayName,body:ChatBody}` requires event_id!=0. `CompanionSpeaker {id:CompanionId,name:CompanionName}`. `ChatBody` variants: `Accepted {companion,command:CommandText}`, `InvalidFormat`, `UnknownCompanion {name:CompanionName}`, `QueueFull {companion,command}`, `NotFollowing {companion,command}`, `Task {companion,command,state:TaskState}`, `Speech {companion,text:SpeechText}`. TaskState variants Started/Progress/Completed/TimedOut/Stopped/Failed(TaskFailure); failures PlannerUnavailable/InvalidPlan/PathUnreachable/WorldChanged/InventoryFull. These variants exhaust the legal kind/reason table. No raw zero companion ID in domain; no simultaneous command and speech fields.

Tests instantiate every legal branch; expected protocol conversion sets exact zero UUID only for InvalidFormat/UnknownCompanion, emits empty text for them, and picks the sole wire text slot for all others. Rejected reason3 and task failure reason15/21 are adapter errors. Original commands remain distinct from model speech. `/warp` sequence0 is a CommandRejection, not a fabricated ChatEvent. Topic `event_chat`.

<a id="node-2-14"></a>

## 2.14 — Complete semantic event surface and evidence

Dependencies: 2.6, 2.7, 2.8, 2.9, 2.10, 2.11, 2.12, 2.13, 1.5. Own `event.rs`, `lib.rs`, new `tests/runtime_contract/event_surface.rs`, new `packages/tools/cmd/runtime-oracle/domain_events_test.go`, test-only observation helpers. Replace digest-only Observation with `Event` variants named exactly: ChunkSnapshot,BlockChanges,ForgetChunks,PlayerState,CommandRejected,RemotePlayerSpawn,RemotePlayerDespawn,RemotePlayerStates,InventoryState,ItemDropUpserts,ItemDropRemoves,FurnaceState,ContainerClosed,ChestState,Chat,CompanionSpawn,CompanionStates,CompanionDespawn,PlaceBlockSucceeded,CraftingState,HostileSpawn,HostileState,HostileDespawn,CombatHit,PassiveSpawn,PassiveState,PassiveDespawn,ProjectileSpawn,ProjectileState,ProjectileDespawn. Each contains the corresponding value from 2.7–2.13 (Chat contains ChatEvent). Exactly30 variants; transport hello/login/reject/keepalive/disconnect excluded. No broad catch-all opaque bytes or digest variant.

Routing is separate `RoutedEvent {recipient:EventRecipient,event:Event}` with `EventRecipient {Session(u64),Broadcast}`. It is an offline/publication envelope only; no session validation or visibility state is implemented. ServerTick fields remain in values that carry them; do not invent a tick for inventory/close/chat events. Acquire/Generate/Ready/Resync worker lifecycle messages and GeneratedChunk pointers are deliberately **outside Event** and belong to later authority/runtime ownership. They are not omitted wire events.

Go event evidence constructs current validated protocol messages and normalizes semantic fields for all30 variants; producer fields are copied from the precise packet table, including current publication joins. It does not claim to replay subscription/visibility logic or the full TickResult. Each earlier event node already supplies its independent Go value producer; protocol nodes add end-to-end wire conversion cases. Move replay Observation/digest/order_observations to test support and delete production use; update foundation tests mechanically.

Test every variant once; mutate a single mining scalar, entity ID, container kind, drop order and chat branch, and require normalized comparison failure. Run domain `event_surface`, `go test ./packages/tools/cmd/runtime-oracle -run '^TestDomainEventsOracle' -count=1`, and the full domain commands above. Completion requires 19 sequenced commands + separate ChatIntent +30 event variants, not a count of source files.
