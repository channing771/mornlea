# Frozen packet field and compatibility table

Controller-reviewed source map for the Superpowers plan, baseline `60c476645ee6dae1f6392336a7f3c593d2163ae3`. Fields, packet keys and bounds below are the compatibility contract consumed by the domain/protocol task nodes. Target ownership and APIs are specified in `execution-contract.md` and the subsystem plans. Newly identified gaps here were inspected statically, not claimed as additional executed reproductions.

Primary sources: `packages/shared/network/protocol/{packet,registry,message_*,snapshot}.go`, `packages/shared/network/codec/{codec_client,codec_server,codec_primitives,chunk_codec}.go`; `packages/server/sim/contract/contract.go`; `packages/server/server/{session_ingress,companion_chat,publication*,*_publication}.go`; `packages/server/sim/runtime/engine_step.go`; `packages/engine/crates/mornlea_protocol/src/*.rs`. All paths below are repository-relative unless absolute.

## Registry and common rules

Protocol version = 45. Go State is `uint8`: Handshake=1, Login=2, Play=3. The registry key is **direction + state + numeric packet ID**, not ID alone. C→S Play ID 1 is unassigned. Next C→S Play ID is 22; next S→C Play ID is 32. Unknown state/type/ID fails closed. Client/server packet and message interfaces are sealed.

| Direction/state | ID → packet |
|---|---|
| C→S Handshake | 0 ClientHello |
| S→C Handshake | 0 ServerHello; 1 HandshakeReject |
| C→S Login | 0 LoginStart |
| S→C Login | 0 LoginSuccess; 1 LoginReject |
| C→S Play | 0 PlayerInput; 2 PlaceBlock; 3 RequestChunkResync; 4 KeepAliveReply; 5 SelectHotbar; 6 MoveInventoryStack; 7 MoveCraftingStack; 8 OpenContainer; 9 MoveContainerStack; 10 CloseContainer; 11 DropSelectedItem; 12 ChatCommand; 13 TillSoil; 14 BoneMeal; 15 TakeCraftingOutput; 16 CollectWater; 17 PlaceWater; 18 EquipArmor; 19 MoveStackPartial; 20 QuickMoveStack; 21 DropStack |
| S→C Play | 0 ChunkSnapshot; 1 BlockChanges; 2 ForgetChunks; 3 PlayerState; 4 CommandRejected; 5 KeepAlive; 6 Disconnect; 7 RemotePlayerSpawn; 8 RemotePlayerDespawn; 9 RemotePlayerStates; 10 InventoryState; 11 ItemDropUpserts; 12 ItemDropRemoves; 13 FurnaceState; 14 ContainerClosed; 15 ChestState; 16 ChatEvent; 17 CompanionSpawn; 18 CompanionStates; 19 CompanionDespawn; 20 PlaceBlockSucceeded; 21 CraftingState; 22 HostileSpawn; 23 HostileState; 24 HostileDespawn; 25 CombatHit; 26 PassiveSpawn; 27 PassiveState; 28 PassiveDespawn; 29 ProjectileSpawn; 30 ProjectileState; 31 ProjectileDespawn |

Wire fixed integers/floats are little endian; booleans are one byte exactly 0 or 1; UUIDs are 16 raw bytes; length/count varints must be canonical; strings must be valid UTF-8; truncated/trailing bytes fail. All floats explicitly called “finite” reject NaN and either infinity. Go protocol validation happens before outbound encoding and after inbound Play decoding; raw inbound Handshake/Login deliberately preserve semantic-invalid records for negotiation/admission.

Frame maximum is 2 MiB for packet-ID varint + payload, excluding the frame-length prefix. Small-payload cap is 64 KiB. Snapshot compressed cap is 1 MiB; decoded cap is 2 MiB. These are wire resource limits, not limits on a logical semantic event type. Published messages and slices are immutable after successful cross-goroutine send.

## Reusable values and exceptions

| Value | Go representation and exact rule | Existing Rust representation |
|---|---|---|
| Dimension | `core.DimensionID int32`; world/player/resync/projectile packets accept exactly Overworld=0 or Depths=1; companion/hostile/passive/container packets accept only 0 | `mornlea_domain::Dimension(u8)` validated 0/1 for most world/entity records; ContainerRef uses public `u8` |
| Chunk / block position | `ChunkPos{X,Z int32}` / `BlockPos{X,Y,Z int32}`; coordinates not globally range-limited; only BlockChanges impose Y in [-64,320) and matching chunk. Chunk X/Z = arithmetic shift X/Z by 4 | `(i32,i32)`, split public i32s, or protocol BlockPos |
| Player ID | `[16]byte`, nonzero UUIDv4: byte6 high nibble=4, byte8 & 0xc0=0x80; text parse demands lowercase canonical hyphenated form | `PlayerId([u8;16])`, private validated constructor |
| Companion ID | distinct `[16]byte` type, same UUIDv4 validity. Zero is allowed only by explicit absent-identity unions, particularly ChatEvent invalid-format/unknown-companion rejections | `CompanionId([u8;16])` with public `NONE`; `new` rejects NONE; see gap below |
| Session ID | `contract.SessionID uint64`, attached by server ingress; not carried by any gameplay packet. Recipient 0 is a **chat broadcast routing sentinel**, not a player identity | Not yet present in domain input records |
| Entity ID | hostile/passive/projectile `uint64`, nonzero in all corresponding wire records; these are distinct identity families even though scalar representation matches | public u64 in corresponding records |
| Drop ID | `{Dimension int32, Chunk{X,Z int32}, Slot uint8, Generation uint32}`; Slot<32 and Generation!=0. **No dimension restriction** in `core.DropID.Valid`. Total order is dimension, X, Z, slot, generation | private `DropId` has raw i32 dimension and same checks/order |
| Container reference | `{Dimension int32, Chunk{X,Z int32}, Kind uint8, Slot uint8, Generation uint32}`; kind furnace=0 or chest=1; dimension=0; generation!=0; furnace slot<32, chest slot<16. `FurnaceRef` is an alias, not a distinct shape | public `ContainerRef{dimension:u8,chunk_x:i32,chunk_z:i32,kind:u8,slot:u8,generation:u32}`; same `new` rules |
| Absent container | Exactly the **entire all-zero ContainerRef** is required in non-container stack views; generation 0 is invalid as a real container but necessary here. Negative/foreign dimensions or nonzero coordinates in a “none” reference are invalid | `ContainerRef::NONE`; raw read intentionally permits sentinel, enclosing command must validate |
| ItemStack | `{Item core.ItemID(uint16), Count uint8, Durability uint16}`; empty exactly (0,0,0). Nonempty registered IDs 1..65; first unregistered=66; Count 1..ItemStackLimit. Durable items require durability 1..max; nondurable require 0 | private `ItemStack` duplicates table and same ordinary-stack rules |
| Item limits | Single count IDs: 10,11,12,13,30,31,32,33,47,48,49,50,51,52,55,56,58,59,60,61,62,65. Other registered nonempty IDs allow 1..64 | duplicate `SINGLE_SLOT_ITEMS` in protocol |
| Durability | maxima by item ID: 10→131,11→250,30→131,31→250,47→59,48→131,49→250,58→165,59→240,60→225,61→195,62→120 | duplicate `DURABILITY_MAXIMA` |
| Equipped broken armor | Ordinary ItemStack.Valid rejects durable item at durability 0, but armor-owned state/save slots explicitly allow Count=1 and Durability=0 for a broken piece. It is not valid to globally relax ordinary stacks to match armor slots | preserve this context exception; player-save armor remains raw fidelity data, outside ordinary ItemStack validation |
| Display name | NormalizeDisplayName trims Unicode edge whitespace, then requires valid UTF-8, 1..32 Unicode runes, <=128 bytes, no Unicode control. Remote spawn/chat player names must already equal canonical normalized value. Login admission can normalize incoming name | Rust `valid_display_name` demands already-trimmed; LoginStart currently normalizes in constructor, see admission gap |
| Companion name | canonical display-name rule plus **no Unicode whitespace anywhere** | `valid_companion_name` |
| Command text | valid UTF-8, 1..1024 bytes, no NUL/Unicode control, no leading/trailing Unicode whitespace. No `@` syntax requirement at protocol layer | `ChatCommand` String, validator same intended rules |
| Speech text | same as command text but 1..256 bytes | same intended rules |
| Block ID | uint16, registered IDs <90 (0=air), no holes in current registry | duplicate `BLOCK_ID_MAX=90` |

Current companion capacity is **4 active**, 64 stored (`packages/shared/companion/identity.go`); protocol MaxCompanionStates aliases MaxActive, and Rust MAX_COMPANION_STATES=4. Planning prose referring to 32 active companions does not match current Go. Passive authority cap 32 versus passive wire batch cap 64 is a separate fact.

## All 20 Play intent families

All rows are C→S / Play. `seq` means `Sequence uint64`. Zero is permitted by protocol except TakeCraftingOutput. No packet carries Session or arrival order. All fields listed are the entire packet payload. Rust files use snake_case equivalents under `mornlea_protocol/src/`; structs generally expose public fields, so constructor checks do not protect later mutation.

| ID / intent / Rust file | Exact fields | Go protocol validation | Go ingress/authority mapping |
|---|---|---|---|
| 0 PlayerInput / player_input.rs | seq; MoveX,MoveZ int8; Jump bool; Yaw,Pitch f32; Mining,Eating,Sprinting,Sneaking bool | yaw/pitch finite only; full int8 axes and finite pitch outside normal look range pass protocol | CommandPlayerInput; all controls copied; held Mining now means primary action, with mining/attack chosen by authority. Entity validPlayerInput later checks axes [-1,1] and pitch ±f32(pi/2−0.01); yaw finite, later normalized |
| 2 PlaceBlock / place_block.rs | seq; Yaw,Pitch f32; Slot u8 | finite angles, Slot 0..8 | CommandPlaceBlock; authority raycasts, chooses target and resulting block, checks world/player/inventory; slot intent retained |
| 3 RequestChunkResync / request_chunk_resync.rs | seq; Dimension i32; Chunk X,Z i32; HaveRevision u64 | dimension 0/1; no revision or coordinate bounds | CommandResync; produces routed ResyncRequest, does not declare authoritative revision |
| 5 SelectHotbar / select_hotbar.rs | seq; Slot u8 | Slot 0..8 | CommandSelectHotbar |
| 6 MoveInventoryStack / move_inventory_stack.rs | seq; From,To u8 | each 0..35; distinct | CommandMoveInventoryStack, From→Slot, To→ToSlot; authority owns contents |
| 7 MoveCraftingStack / move_crafting_stack.rs | seq; From,To u8 | each 0..44; distinct; cannot both be >=9 | CommandMoveCraftingStack, From→Slot, To→ToSlot. Grid 0..8, inventory 9..44; personal-grid unused cells 4..8 rejected later by sim |
| 8 OpenContainer / open_container.rs | seq; Yaw,Pitch f32 | finite angles | legacy internal name CommandOpenFurnace; authority determines hit kind, identity and reach. No kind or coordinate supplied |
| 9 MoveContainerStack / move_container_stack.rs | seq; Container ref; From,To u8 | valid real ref; distinct; furnace each 0..38 and To!=38; chest each 0..62 | legacy CommandMoveFurnaceStack, Container→Furnace, From→Slot, To→ToSlot. Furnace view 0..35 inventory, 36 input,37 fuel,38 output; chest 0..35 inventory,36..62 chest |
| 10 CloseContainer / close_container.rs | seq | none | legacy CommandCloseFurnace; viewed identity remains server-owned |
| 11 DropSelectedItem / drop_selected_item.rs | seq | none | CommandDropSelectedItem; authority selected hotbar item, **one item**, feet position |
| 12 ChatCommand / chat_command.rs | Text string | command text above | separate incomingChat{sessionID,generation,command}; FIFO chat drain; **no command Sequence**, not translated to contract.Command. `/warp` namespace intercepted before companion addressing; then `@name command` parsed |
| 13 TillSoil / till_soil.rs | seq; Yaw,Pitch f32 | finite angles | CommandTillSoil; target, held tool and mutation server-owned |
| 14 BoneMeal / bone_meal.rs | seq; Yaw,Pitch f32 | finite angles | CommandBoneMeal; target and held bone meal server-owned |
| 15 TakeCraftingOutput / take_crafting_output.rs | seq | **seq!=0** | CommandTakeCraftingOutput; recipe/output/material consumption/capacity derived by authority |
| 16 CollectWater / collect_water.rs | seq; Yaw,Pitch f32 | finite angles | CommandCollectWater; target/source and held bucket server-owned |
| 17 PlaceWater / place_water.rs | seq; Yaw,Pitch f32 | finite angles | CommandPlaceWater; target and held bucket server-owned |
| 18 EquipArmor / equip_armor.rs | seq | none | CommandEquipArmor; authority selected item determines armor slot; success via InventoryUpdate, no dedicated success packet |
| 19 MoveStackPartial / move_stack_partial.rs | seq; Container ref; View u8; From,To u8; Single bool | View=0 inventory/1 crafting/2 container; view0 exact zero ref and indices 0..35; view1 exact zero ref and indices 0..44; view2 valid real ref and kind-dependent indices 0..38 or 0..62; distinct | CommandMoveStackPartial, explicit enum map View→StackView, ref→Furnace, From→Slot, To→ToSlot. Single=false means ceil(sourceCount/2), true means 1. **Protocol does not reject two inventory-region crafting indices or furnace-output destination**; authority applies item/slot rules |
| 20 QuickMoveStack / move_stack_partial.rs | seq; Container ref; View u8; From u8 | same view/ref/source bound as partial; no To or distinct test | CommandQuickMoveStack; ref→Furnace, From→Slot; full source stack, deterministic server destination order, no supplied count/destination |
| 21 DropStack / move_stack_partial.rs | seq; Container ref; View u8; Slot u8 | same view/ref/source bound as partial | CommandDropStack; entire source stack; feet position and count derived by authority |

Excluded transport control C→S Play/4 KeepAliveReply is Token u64!=0. Server endpointReader consumes it before command ingress.

**Ordering facts:** `incomingCommand` carries Session, session Generation and Command. `drainIncoming` drops stale generation and replaces Command.Session with envelope Session. Engine `sort.SliceStable` compares Session first, Sequence second and preserves arrival for exact ties; it then ignores Sequence<=session.lastSequence and updates lastSequence **before** entity command execution. Duplicate sequence winner is earliest arrival for that session, even across different kinds; invalid authoritative commands can consume the sequence. No kind-name tiebreaker exists. Zero sequence is wire-valid but normally stale against initial lastSequence=0. Chat uses separate bounded channel FIFO (up to channel length captured at drain start, inputCapacity=256); no global merge order with sequenced commands is encoded in contract.Command. Session generation is runtime ingress lifecycle data; lexical kind sorting cannot substitute for arrival.

## Outcomes and control payloads

| S→C Play ID | Entire wire-semantic fields / validation | Go semantic source and transformation |
|---|---|---|
| 4 CommandRejected | Sequence u64; Reason protocol RejectReason string mapped to byte1..15; zero seq allowed | `Rejection{Session SessionID,Sequence u64,Reason contract.RejectReason u8}`; removes Session for route. Internal enum is **0..14**, wire enum is 1..15; explicit named mapping, not raw cast |
| 20 PlaceBlockSucceeded | Sequence u64, including 0; no other fields | `PlacementSuccess{Session,Sequence}`; owned-session only; confirms atomic block placement and exactly-one inventory decrement, not generic success |
| 25 CombatHit | ServerTick u64!=0; Damage u8 in 1..20; TargetKind u8 player=1,hostile=2,passive=3 | `CombatHit{Session,Damage,TargetKind}`; publication injects TickResult.Tick, routes attacker-only, after own PlayerState; no target ID or sequence on wire |
| 5 KeepAlive | Token u64!=0 | session transport, not simulation outcome |
| 6 Disconnect | Code u8=1 protocol violation,2 timeout,3 shutdown,4 slow client,5 internal; Message string wire<=256 bytes/runes, UTF-8 (empty legal) | session transport; Go protocol Validate checks code, codec checks string |

Rejection mapping in exact order: 0/1 invalid_ray; 1/2 no_target; 2/3 chunk_not_ready; 3/4 protected_block; 4/5 invalid_block; 5/6 occupied; 6/7 invalid_input; 7/8 player_not_ready; 8/9 invalid_slot; 9/10 hotbar_full; 10/11 drop_capacity; 11/12 container_capacity; 12/13 not_fluid_source; 13/14 bucket_mismatch; 14/15 not_armor. Each pair is internal/wire ID. Unknown internal reason causes publication failure/session closure.

## World payloads

| S→C Play ID | Entire fields / validation | Go semantic difference |
|---|---|---|
| 0 ChunkSnapshot | Dimension i32=0/1; Chunk{X,Z i32}; Revision u64!=0; Sections []SectionData exactly24 in Y index order0..23 | Wire snapshot constructed from engine CloneReadyChunk, **not present as a TickResult snapshot event**; logical world data and compressed SectionData are distinct concerns |
| 1 BlockChanges | Dimension0/1; Chunk X,Z i32; BaseRevision u64 in1..MAX−1; NewRevision=Base+1; Changes []{Position X,Y,Z i32; Block u16}, count0..4096 | contract.ChunkChangeBatch has same logical fields with its own BlockChange type; publication copies slice into network records, checks subscriber and contiguous per-session revision before send. Empty changes allowed as drop-only revision barrier |
| 2 ForgetChunks | Dimension0/1; Chunks []{X,Z i32}, count1..4096, unique; **protocol does not demand sorted order** | TickResult.Forget is map[SessionID][]ChunkKey; publication groups dimension, sorts dimension/X/Z, copies and splits into <=4096 chunks |

Each BlockChanges position must have Y in[-64,320), correct chunk, registered block, and strictly increasing chunk-local index. Index = sectionIndex*4096 + localY*256 + localZ*16 + localX; coordinates use floor via arithmetic shifts. Ordering implies no duplicates.

SectionData exact raw fields: Y int32; Storage u8(Single=0,Indexed=1,Direct=2); Single BlockID(u16); Bits u8; Palette []u16; Packed []u64. Single requires Bits=0, empty Palette/Packed, registered Single. Indexed requires Single=0, Bits=4 or8, palette length1..2^Bits, unique registered entries, Packed length ceil(4096/floor(64/Bits)) (256 or512), every packed index<palette length. Direct requires Single=0, empty Palette, Bits=15,1024 packed words, each word high4 bits zero and all4096 decoded values registered. Snapshot compression/envelope/palette layout belongs to current protocol representation; the Go simulation change records do not use packed sections.

TickResult additionally carries Acquire []ChunkKey, Generate []ChunkKey, Ready []ChunkKey, Resync []ResyncRequest. ResyncRequest={Session,Sequence,Dimension,Chunk,HaveRevision}; Ready is publication wake-up for globally-ready transition **or a new subscriber to an already-ready chunk**, not raw chunk contents. Acquire/Generate are worker lifecycle requests. GeneratedChunk/AcquiredChunk contain *world.Chunk, errors, revision/recovery/load metadata, and are worker ingress DTOs, not immutable by-value world observations merely because they live in contract.go.

## Player payloads

| S→C Play ID | Entire fields | Validation / ownership |
|---|---|---|
| 3 PlayerState | ServerTick u64; LastInputSequence u64; Dimension i32; Position,Velocity [3]f32; Yaw,Pitch f32; OnGround,Ready,Reset bool; MiningActive bool; MiningTarget {X,Y,Z i32}; MiningProgressTicks,MiningRequiredTicks u16; MiningHarvestable bool; Health u8; Oxygen u16; Hunger u8; SaturationZero bool; DayPhaseOffset u16; WorldTimeTicks u64; WeatherKind u8; Season u8; SeasonProgress u8; Temperature i8; ArmorPoints u8 | dimension0/1; all position/velocity/angles finite; health0..20; oxygen0..300; hunger0..20; offset0..23999; weather0 clear/1 rain/2 thunder; season0 spring/1 summer/2 autumn/3 winter; armor0..20. **SeasonProgress entire u8 and Temperature entire i8 accepted**; temperature producer clamps[-40,45]. Tick/input sequence/time zero accepted. No player UUID in this private packet |
| 7 RemotePlayerSpawn | PlayerID UUID; DisplayName string; ServerTick u64; Dimension i32; Position[3]f32; Yaw,Pitch f32 | valid UUID/canonical player name; dim0/1; finite pose; **no pitch bound**, zero tick allowed |
| 8 RemotePlayerDespawn | PlayerID UUID | valid UUID |
| 9 RemotePlayerStates | ServerTick u64; Players []{PlayerID UUID;Dimension i32;Position[3]f32;Yaw,Pitch f32;Reset bool} | count1..7; valid IDs, dim0/1, finite pose; strictly increasing raw UUID byte order; tick0 allowed |

PlayerState mining union: inactive requires exact zero target, zero progress/required and false harvestable. Active requires 0<progress<required. No further target-coordinate rule in PlayerState validator. No wire-wide pitch clipping rule should be inferred from companion rules.

Go `PlayerUpdate` adds Session and ViewCenter ChunkPos, replaces Position/Velocity/OnGround with `physics.State`, replaces five mining fields with `MiningUpdate{Active,Target,ProgressTicks,RequiredTicks,Harvestable}`, and otherwise carries LastInputSequence, Dimension, Yaw/Pitch, Ready/Reset, health/oxygen/hunger/saturation flag, day offset, world time, weather/season/progress/temp/armor. Publication injects ServerTick from enclosing TickResult, drops routing/view fields and flattens State/Mining. Remote publication joins session playerID/displayName, drops velocity/ground/private survival/inventory/mining, gates visibility on subscription and sent foot-chunk snapshot, and derives spawn/state/despawn from per-session visibility. Newly spawned remotes skip a same-tick state record.

TickResult's world time/weather/season/progress are authoritative tick outputs; PlayerUpdate copies world scalars plus per-player observation temperature/armor. Saturation/exhaustion quantities and equipped armor stacks do not occur in PlayerState. Oxygen and saturation-zero presentation flag are not persisted.

## Inventory and container payloads

| S→C Play ID | Entire fields / validation | Go semantic source |
|---|---|---|
| 10 InventoryState | Inventory{Hotbar{Selected u8 0..8,Slots[9]ItemStack},Backpack[27]ItemStack}; each ordinary stack valid | InventoryUpdate{Session,Inventory}, route owner only; **no equipped armor array** |
| 21 CraftingState | Size u8=2 or3; Slots[9]ItemStack; Output ItemStack; all valid; Size2 requires indices4..8 exact EMPTY | CraftingUpdate adds Session; full latest state owner-only, never broadcast; output derived by server |
| 13 FurnaceState | Furnace real furnace ref; Input,Fuel,Output ItemStack; ProgressTicks u8<200; BurnTicks u16<=1600 | FurnaceUpdate adds Session; current viewer-only. Input empty or smeltable: IDs6(raw iron),18(sand),27(clay),53(raw beef); fuel empty or5(coal); output empty or7(iron ingot),23(glass),24(brick),54(cooked beef). No additional timer/slot consistency relation in Validate |
| 15 ChestState | Chest real chest ref; Items[27]ItemStack, all valid | ChestUpdate adds Session; current viewer-only |
| 14 ContainerClosed | Container valid real furnace or chest ref | `FurnaceEnd{Session,Furnace}` contains **both kinds** despite name; strips Session |

Publication local order: rejections → placement successes → inventories → craftings → furnaces → chests → container ends → own PlayerState → CombatHits. Data are copied by value; Go fixed arrays preserve owned snapshots. Ordinary inventory permits only intact durable stacks, while separately owned equipped armor may have durability0.

## Entity payload families

Tick is u64 in all batched entity messages and may be0 (CombatHit is the exception). Finite pose/velocity rules cover every f32 in the following rows. UUID order is lexicographic raw-byte order; scalar IDs use numeric order. All batches reject duplicates by strict ordering; empty batches are invalid.

| S→C Play IDs / family | Entire fields and batch bounds | Go semantic source / omitted data |
|---|---|---|
| 17 CompanionSpawn | ID UUID; Name string; Tick u64; Dimension i32=0; Position[3]f32; Yaw,Pitch f32; valid UUID/name; finite pose; pitch inclusive[-pi/2,+pi/2] | Joins `companion.Definition{ID,Name,...}` and `CompanionUpdate`; profile/persona are not sent |
| 18 CompanionStates | Tick u64; States[]{ID UUID;Dimension i32=0;Position[3]f32;Yaw,Pitch f32;Reset bool}; count1..4; sorted UUID; same pose/pitch rules | CompanionUpdate={ID,Dimension,State physics.State,Yaw,Pitch,Reset,Mining MiningUpdate}; velocity/ground/mining are not on companion wire states |
| 19 CompanionDespawn | ID UUID valid/nonzero | derived from visibility difference |
| 22 HostileSpawn | ServerTick u64; Spawns[]{ID u64!=0;Dimension i32=0;Position[3]f32;Yaw f32;Health u8 1..20;Kind u8 0 nightwalker/1 bone thrower}; count1..64 sorted | HostileMob projection fields below; spawn does not send velocity, ground or AI state |
| 23 HostileState | ServerTick u64; States[]{ID u64!=0;Position,Velocity[3]f32;Yaw f32;Health u8 1..20;Kind u8 0/1}; count1..64 sorted | no dimension in state; dimension changes require despawn/spawn |
| 24 HostileDespawn | ServerTick u64; IDs[]u64!=0; count1..64 sorted | derived from visibility/liveness difference; no reason |
| 26 PassiveSpawn | ServerTick u64; Spawns[]{ID u64!=0;Dimension i32=0;Position[3]f32;Yaw f32;Health u8 1..20}; count1..64 sorted | authority cap32 is narrower than wire64; do not conflate |
| 27 PassiveState | ServerTick u64; States[]{ID u64!=0;Position,Velocity[3]f32;Yaw f32;Health u8 1..20;Grazing u8 0/1}; count1..64 sorted | PassiveMob.Grazing is bool; publication maps to u8; transient, never saved |
| 28 PassiveDespawn | ServerTick u64; Despawns[]{ID u64!=0;Reason u8 0 vanished/1 died}; count1..64 sorted | reason joins visibility difference with separate engine.PassiveDeaths []u64 |
| 29 ProjectileSpawn | ServerTick u64; Spawns[]{ID u64!=0;Kind u8 0 shard/1 arrow;Dimension i32=0/1;Position,Velocity[3]f32}; count1..128 sorted | ProjectileSnapshot identical individual fields; wire does not impose kind×dimension policy |
| 30 ProjectileState | ServerTick u64; States[]{ID u64!=0;Position[3]f32}; count1..128 sorted | omits immutable kind/dimension and velocity from full snapshot |
| 31 ProjectileDespawn | ServerTick u64; IDs[]u64!=0; count1..128 sorted | derived from visibility/liveness difference; transient entities not saved |
| 11 ItemDropUpserts | ServerTick u64; Drops[]{ID DropID;BlockIndex u32<98304;Item u16;Count u8;Durability u16}; count1..32 sorted DropID; ordinary ItemStack valid | engine DropSnapshot has same individual fields; per-session publication derives upserts/deltas. **Validator permits empty (0,0,0) stack**, though normal authority produces real drops |
| 12 ItemDropRemoves | ServerTick u64; IDs[]DropID; count1..32 sorted | publication-derived removal; raw dimension exception retained |

HostileMob entire Go projection: ID u64; Dimension i32; State physics.State{Position,Velocity[3]f32,OnGround bool}; Yaw f32; Health u8; AttackCooldown,HurtCooldown,BurnCooldown u8; HasTarget bool; PlayerID UUID (may be absent when no target); NextRepathTicks u64; DistantTicks u16; Kind u8; ShootCooldown u8. Cooldowns/target/pathing are simulation/orchestration data, not fields on wire outcomes. PassiveMob entire projection: ID u64; Dimension i32; State physics.State; Yaw f32; Health u8; Grazing bool. ProjectileSnapshot entire projection as in table. None of these structs in contract.go has a standalone Validate method; their producers enforce authority invariants.

Hostiles/projectiles/passives are **not TickResult slices**: publication calls engine.HostileMobs(), engine.Projectiles(), engine.PassiveMobs(), engine.PassiveDeaths() once after tick and shares snapshots across sessions. Drop publication likewise reads engine state. Entity publication gates on per-session interest and foot-chunk snapshot and computes despawn→spawn→state; newly spawned entities skip state that tick. Global publication order is remote despawns, companion despawns, chunk forget, snapshots, block deltas, companion spawn/state, remote spawn/state, hostile group, projectile group, passive group, drops, chats, local results. A raw wire trace therefore contains visibility/transport projections beyond TickResult alone.

## Chat semantic union

S→C Play/16 `ChatEvent` entire Go fields: EventID u64; PlayerID UUID; PlayerName string; CompanionID UUID-or-explicit-zero; CompanionName string; Kind u8; RejectReason u8; Command string; Speech string. All events require EventID!=0, valid PlayerID and canonical PlayerName. Only the Speech kind permits nonempty Speech; **one physical wire text slot** contains Command or Speech according to kind. Go's two fields express a mutually exclusive semantic union. Text slot max is1024 (speech256); wire max1328.

| Kind numeric/name | Reason | Companion identity/name | Command/Speech |
|---|---|---|---|
| 1 Accepted |0 None| valid UUID + valid companion name | valid Command; Speech empty |
| 2 Rejected /1 InvalidFormat |1| exact zero UUID + empty name | both empty |
| 2 Rejected /2 UnknownCompanion |2| exact zero UUID + valid companion name | both empty |
| 2 Rejected /4 QueueFull |4| valid UUID + valid name | valid Command; Speech empty |
| 2 Rejected /5 NotFollowing |5| valid UUID + valid name | valid Command; Speech empty |
| 3 TaskStarted,4 TaskProgress,5 TaskCompleted,7 TaskTimedOut,8 TaskStopped |0| valid UUID + valid name | valid original Command; Speech empty |
| 6 TaskFailed |16 PlannerUnavailable,17 InvalidPlan,18 PathUnreachable,19 WorldChanged,20 InventoryFull| valid UUID + valid name | valid original Command; Speech empty |
| 9 CompanionSpeech |0| valid UUID + valid name | Command empty; valid Speech1..256 bytes |

All other kind/reason combinations fail atomically. Rejection reason3 is reserved/unassigned; 6..15 are not accepted chat reasons. Task facts repeat original player command and cannot carry model-generated speech.

There is **no chat event DTO in sim/contract**. Current server builds protocol.ChatEvent directly; `chatDelivery{event network.ChatEvent,recipient SessionID}` is the routing wrapper. recipient0 broadcasts to normal sessions; nonzero targets one session; trusted observer receives no chats. Accepted uses broadcast; synchronous addressing/queue/not-following rejects target issuer. Event IDs come from server.nextChatEventID and overflow is a hard failure. Task lifecycle/speech later reuse same event-ID counter. Exact `停止` after addressing trim bypasses FIFO task enqueue; successful stop later emits TaskStopped, otherwise NotFollowing. `/warp` namespace commands do not produce ChatEvent. Warp rejects are direct `CommandRejected{Sequence:0,Reason:...}` in `server/warp.go`, outside TickResult.Rejected; this is a concrete reason that outcome sequence zero is legal. Exact accepted warp strings are `/warp depths` and `/warp overworld`; all `/warp` or `/warp `-prefixed spelling errors enter warp rejection. These facts prevent inventing a shared client sequence or treating chat event ID as command sequence.

## Baseline gaps covered by the task plan

Corresponding snake_case module exists for every registry family (QuickMoveStack and DropStack are colocated in move_stack_partial.rs). However the existence of a file does not establish usable dispatch/coverage:

1. `container_closed.rs` implements valid server/play/14 but **lib.rs declares neither mod container_closed nor its pub use**, so the file is not in the compiled public codec surface.
2. There is no typed direction/state registry/dispatch counterpart in current Rust lib.rs. Per-struct PACKET_ID constants alone cannot discriminate collisions among direction and state.
3. `move_stack_partial.rs::validate_stack_split` container branch checks only kind and indices; it does **not** call container.validate_any/new. `ContainerRef::read` intentionally returns raw fields. Thus container-view partial/quick/drop decode can pass invalid generation, physical slot or dimension. The non-container branch correctly demands exact NONE.
4. `MoveContainerStack::new` validates indices/kind but not the public ContainerRef's dimension/generation/physical slot. Its `decode` reconstructs ContainerRef::new, so decode is stronger than public construction. This is separate from the partial/quick/drop decode gap.
5. `CompanionId::NONE` is public. CompanionSpawn::valid, CompanionStates::valid and CompanionDespawn::new do not reject NONE when constructed through the public API. Their decoders call validated CompanionId::new. ChatEvent correctly uses names_companion to distinguish real ID from its two legitimate zero-identity rejection branches.
6. Many Rust packet structs have public mutable fields and encode uses `finish().expect(...)` assuming a validated constructor. Post-construction invalid public values can emit unchecked enum/range values or panic for invalid float/string. Ordinary ItemStack and DropId fields are private; aggregates and references often are not. Constructor/decoder validation must not be treated as a total encoder guarantee.
7. Rust domain currently implements only PlayerInput, PlaceBlock, SelectHotbar semantic inputs; `order_inputs` sorts sequence then kind string, lacking Session/arrival and disagreeing with Go stable ordering/dedup semantics. Domain event.rs contains digest-only Observation, not any semantic payload above.
8. Current Rust ClientHello/ServerHello decode checks current version immediately and LoginStart decode uses semantic constructor; Go raw inbound client admission distinguishes structural decode from version/identity/name/distance rejection. This is already recognized by repair planning; it must not be used as a domain-value acceptance rule for raw inbound records.
9. Protocol and storage have duplicate current item/UUID tables. This extraction records protocol values only, not approval of a particular shared API.

## Test inputs and evidence worth reusing

The listed files are current sources of concrete values/goldens. Tests were **not run** for this extraction; these are candidate corpus sources, not claimed passing evidence.

| Coverage | Existing evidence |
|---|---|
| State/direction/ID closure | protocol/registry_test.go frozen registry tests; packet_test.go validation matrix and state/error code pins; message_test.go sealed message shapes |
| Small payload bytes and reject enum | codec/codec_golden_test.go TestProtocolV1SmallPacketGolden, TestProtocolV2RemotePlayerGolden, TestSmallPacketCommandRejectedReasonWireValues; codec/codec_invalid_test.go |
| Shared primitives | codec/codec_primitives_test.go canonical varint boundaries, LE integer/float, invalid UTF-8, limits, finite floats, short/trailing input; frame_test.go |
| Snapshot | codec/testdata/chunk-snapshot-v1.bin; chunk_codec_test.go fixtureSnapshot and malformed envelope/logical/zstd-expansion tests; protocol/snapshot_test.go canonical sections, malformed palette/words/block IDs |
| Stack views / zero sentinel / malformed real refs | codec/stack_splitting_test.go TestStackSplitGoldenWireLayout, TestStackSplitZeroContainerRoundTrip, TestStackSplitWireRejectionMatrix; codec/drop_stack_test.go same DropStack families; protocol/message_stack_splitting_test.go and message_drop_stack_test.go |
| Inventory / crafting / furnace / chest | codec/codec_invalid_test.go inventory and crafting bad cells; codec/furnace_test.go fixed lengths, invalid values and wire; codec/container_test.go chest golden; protocol/furnace_output_whitelist_test.go covers all smelting products |
| Player projections and limits | codec/worldtime_test.go, hunger_test.go, weather_test.go, season_test.go, armor_test.go; protocol/worldtime_test.go; server/publication_test.go TestPublishLocalResultMapsCanonicalMiningStateIntoSinglePlayerState, TestRemotePlayerStatesDoNotPublishMiningState |
| Chat identity union/text boundaries | codec/message_companion_test.go TestCompanionMessageGolden, all TaskLifecycle/Speech combinations, invalid UTF8,1024/1025 command,32-rune/128-byte names, radians pitch, max wire lengths, five/duplicate/unsorted companions; message_companion_fuzz_test.go and message_companion_limit_lock_test.go |
| Drops | codec/drop_test.go TestProtocolV4DropGolden, TestProtocolV16ToolDropUsesFiveByteStackWire, count-before-allocation cases; protocol/drop_test.go/message_drop*.go |
| Mobs/projectiles/combat | codec/message_{hostile,passive,projectile,combat}_test.go golden/fixed-size and invalid count/ID/enum/finite cases; protocol equivalent files |
| Ordering/dedup | sim/runtime/engine_test.go TestEngineSortsCommandsAndDeduplicatesSequence; chest_inventory_test.go TestMoveChestMultiPlayerSameTickStableOrder / TestMoveIgnoresStaleSequenceForChest; drop_test.go TestPlaceBeforeDropUsesGlobalSequenceOrder; placement_success_test.go TestPlayerPlacementSuccessesPreserveEverySequence. New same-sequence different-kind reversed-arrival cases are still needed to expose lexical kind ordering |
| Routed outcomes | server/placement_success_publication_test.go; combat_hit_publication_test.go; crafting_publication_test.go owner-only; furnace_publication_test.go and chest_publication_test.go viewer-only |
| Visibility projection/order | server/player_publication_test.go (including seven states / eight sessions), companion_publication_test.go (including four companion capacity), hostile_publication_test.go, passive_publication_test.go, projectile_publication_test.go; Memory/TCP transcript parity tests for hostile/passive |
| Chunk readiness/forget | server/publication_test.go initial snapshot before delta, contiguous revision, resync priority,4097 split; sim/runtime engine/subscription tests for per-session readiness |
| Rust current family fixtures | mornlea_protocol/tests/runtime_contract.rs: family-named *_round_trip_preserves_golden_bytes and *_rejects_* tests; these are source-ported/self-roundtrip evidence until executed against independent Go-produced cases |

This table is change-scoped compatibility evidence. Do not copy its volatile family counts into the architecture skill.
