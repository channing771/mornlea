# Foundation task index — Superpowers revision

Baseline: `60c476645ee6dae1f6392336a7f3c593d2163ae3`. Implementation remains partial and **not accepted**. Only crate registration2.1 retains acceptance. Existing source/previous completed attempts are preserved; an unchecked node means its revised contract still needs implementation/verification. This planning revision runs no runtime repair. Historical ledger entries retain their original task IDs; use the migration table below when reading them.

The main Agent owns the complete architecture and functionality in [design](design.md), [shared execution contract](execution-contract.md), [packet fields](packet-contracts.md), [storage formats](storage-contracts.md), [kernel evidence](kernel-contracts.md) and the six linked plans. Every unchecked node now has a frozen implementation packet. There is no interface-design assignment left to workers. Dependency waits mean accepted prerequisites are needed, not that the design is unfinished. Workers report source discrepancies; the main Agent revises the affected packet before dependent implementation.

This is the **sole checkbox/status source**. Each node links its exact files, interfaces, algorithm, concrete failing/boundary cases, commands, exclusions, integration and rollback. Test names/targets introduced by the plan are prospective until registered with nonzero cases; do not claim they ran during planning. Add the named failing case first, implement, run its focused gates, submit actual before/after evidence and make a scoped commit after controller acceptance. Source/result SHA, case IDs and executed/discoveredcounts belong in ledger.md.

Dispatch at most one coherent node per worker. Controller serializes shared exports, Cargo.lock, manifest fragments, common test roots and FFI integration. Independent files do not authorize independent architecture decisions. Run tests at the lowest proportionate tier; full gates remain6.6. No online runtime,sourcefixture rewrite,foregroundgame,destructive Git or external publication is included.

Execution order follows direct dependencies, not numeric headings: evidence primitives → domain values and narrow storage repairs → per-packet/per-save nodes →6.1/6.2 foundation contract gate → numerical families/pathfinding →6.3–6.6. Full numerical inventory is not a prerequisite for numerical implementation.

## 1. Executable evidence

- [x] 1.1 [Manifest and shared consumer with one real framing case](plans/01-evidence.md#node-1-1). Direct prerequisites: 2.1. Execute the linked packet's named red/green and integration commands.
- [x] 1.2 [Narrow oracle dependency and header-provenance gates](plans/01-evidence.md#node-1-2). Direct prerequisites: 1.1. Execute the linked packet's named red/green and integration commands.
- [x] 1.3 [Schema-2 trace completeness](plans/01-evidence.md#node-1-3). Direct prerequisites: 1.1. Execute the linked packet's named red/green and integration commands.
- [x] 1.4 [Exclusive temporary work and atomic export](plans/01-evidence.md#node-1-4). Direct prerequisites: 1.3. Execute the linked packet's named red/green and integration commands.
- [x] 1.5 [Independent operation runners](plans/01-evidence.md#node-1-5). Direct prerequisites: 1.2, 1.3, 1.4. Execute the linked packet's named red/green and integration commands.
- [x] 1.6 [Agent HTTP/MCP contract evidence without a service](plans/01-evidence.md#node-1-6). Direct prerequisites: 1.5. Execute the linked packet's named red/green and integration commands.

## 2. Domain and shared ownership

- [x] 2.1 Register `mornlea_domain`, `mornlea_protocol`, `mornlea_storage`, scoped guides and nonempty existing runtime_contract targets. Retained acceptance from the prior revision; this does not accept behavior. Discovery: `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain -p mornlea_protocol -p mornlea_storage --test runtime_contract --locked -- --list`.
- [x] 2.2 [Identity, text and scalar values](plans/02-domain.md#node-2-2). Direct prerequisites: 1.5. Execute the linked packet's named red/green and integration commands.
- [x] 2.3 [Item, drop and container values](plans/02-domain.md#node-2-3). Direct prerequisites: 2.2, 1.5. Execute the linked packet's named red/green and integration commands.
- [x] 2.4 [Movement and ray command payloads](plans/02-domain.md#node-2-4). Direct prerequisites: 2.3. Execute the linked packet's named red/green and integration commands.
- [ ] 2.5 [Inventory, container and chat intents](plans/02-domain.md#node-2-5). Direct prerequisites: 2.3, 2.4. Execute the linked packet's named red/green and integration commands.
- [ ] 2.6 [Envelope ordering and Go authority counterexample](plans/02-domain.md#node-2-6). Direct prerequisites: 2.5, 1.5. Execute the linked packet's named red/green and integration commands.
- [ ] 2.7 [Command outcomes and player publications](plans/02-domain.md#node-2-7). Direct prerequisites: 2.2, 1.5. Execute the linked packet's named red/green and integration commands.
- [ ] 2.8 [Compact chunk observations](plans/02-domain.md#node-2-8). Direct prerequisites: 2.2. Execute the linked packet's named red/green and integration commands.
- [ ] 2.9 [Inventory and container publications](plans/02-domain.md#node-2-9). Direct prerequisites: 2.3. Execute the linked packet's named red/green and integration commands.
- [ ] 2.10 [Remote-player and companion observations](plans/02-domain.md#node-2-10). Direct prerequisites: 2.2. Execute the linked packet's named red/green and integration commands.
- [ ] 2.11 [Hostile and passive observations](plans/02-domain.md#node-2-11). Direct prerequisites: 2.2. Execute the linked packet's named red/green and integration commands.
- [ ] 2.12 [Projectile and item-drop observations](plans/02-domain.md#node-2-12). Direct prerequisites: 2.3. Execute the linked packet's named red/green and integration commands.
- [ ] 2.13 [Chat tagged union](plans/02-domain.md#node-2-13). Direct prerequisites: 2.2. Execute the linked packet's named red/green and integration commands.
- [ ] 2.14 [Complete semantic event surface and evidence](plans/02-domain.md#node-2-14). Direct prerequisites: 2.6, 2.7, 2.8, 2.9, 2.10, 2.11, 2.12, 2.13, 1.5. Execute the linked packet's named red/green and integration commands.

## 3. Protocol: one concrete packet per node

- [ ] 3.1 [Primitive caller-buffer and framing boundary](plans/03-protocol.md#node-3-1). Direct prerequisites: 1.1. Execute the linked packet's named red/green and integration commands.
- [ ] 3.2 [Shared-value integration without raw format loss](plans/03-protocol.md#node-3-2). Direct prerequisites: 2.3, 2.8, 3.1. Execute the linked packet's named red/green and integration commands.
- [ ] 3.3 [Structural login and pure admission](plans/03-protocol.md#node-3-3). Direct prerequisites: 2.2, 3.1, 1.5. Execute the linked packet's named red/green and integration commands.
- [ ] 3.4 [Typed direction/state registry closure](plans/03-protocol.md#node-3-4). Direct prerequisites: 3.10–3.68, 3.3. Execute the linked packet's named red/green and integration commands.
- [ ] 3.5 [Semantic adapter closure](plans/03-protocol.md#node-3-5). Direct prerequisites: 2.14, 3.4. Execute the linked packet's named red/green and integration commands.
- [ ] 3.10 [ClientHello (C/Handshake/0)](plans/03-protocol.md#node-3-10). Direct prerequisites: 1.5, 3.1, 3.2, 3.3. Execute the linked packet's named red/green and integration commands.
- [ ] 3.11 [ServerHello (S/Handshake/0)](plans/03-protocol.md#node-3-11). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.12 [HandshakeReject (S/Handshake/1)](plans/03-protocol.md#node-3-12). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.13 [LoginStart (C/Login/0)](plans/03-protocol.md#node-3-13). Direct prerequisites: 1.5, 3.1, 3.2, 3.3. Execute the linked packet's named red/green and integration commands.
- [ ] 3.14 [LoginSuccess (S/Login/0)](plans/03-protocol.md#node-3-14). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.15 [LoginReject (S/Login/1)](plans/03-protocol.md#node-3-15). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.16 [PlayerInput (C/Play/0)](plans/03-protocol.md#node-3-16). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.17 [PlaceBlock (C/Play/2)](plans/03-protocol.md#node-3-17). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.18 [RequestChunkResync (C/Play/3)](plans/03-protocol.md#node-3-18). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.19 [KeepAliveReply (C/Play/4)](plans/03-protocol.md#node-3-19). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.20 [SelectHotbar (C/Play/5)](plans/03-protocol.md#node-3-20). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.21 [MoveInventoryStack (C/Play/6)](plans/03-protocol.md#node-3-21). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.22 [MoveCraftingStack (C/Play/7)](plans/03-protocol.md#node-3-22). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.23 [OpenContainer (C/Play/8)](plans/03-protocol.md#node-3-23). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.24 [MoveContainerStack (C/Play/9)](plans/03-protocol.md#node-3-24). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.25 [CloseContainer (C/Play/10)](plans/03-protocol.md#node-3-25). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.26 [DropSelectedItem (C/Play/11)](plans/03-protocol.md#node-3-26). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.27 [ChatCommand (C/Play/12)](plans/03-protocol.md#node-3-27). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.28 [TillSoil (C/Play/13)](plans/03-protocol.md#node-3-28). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.29 [BoneMeal (C/Play/14)](plans/03-protocol.md#node-3-29). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.30 [TakeCraftingOutput (C/Play/15)](plans/03-protocol.md#node-3-30). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.31 [CollectWater (C/Play/16)](plans/03-protocol.md#node-3-31). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.32 [PlaceWater (C/Play/17)](plans/03-protocol.md#node-3-32). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.33 [EquipArmor (C/Play/18)](plans/03-protocol.md#node-3-33). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.34 [MoveStackPartial (C/Play/19)](plans/03-protocol.md#node-3-34). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.35 [QuickMoveStack (C/Play/20)](plans/03-protocol.md#node-3-35). Direct prerequisites: 1.5, 3.1, 3.2, 3.34. Execute the linked packet's named red/green and integration commands.
- [ ] 3.36 [DropStack (C/Play/21)](plans/03-protocol.md#node-3-36). Direct prerequisites: 1.5, 3.1, 3.2, 3.35. Execute the linked packet's named red/green and integration commands.
- [ ] 3.37 [ChunkSnapshot (S/Play/0)](plans/03-protocol.md#node-3-37). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.38 [BlockChanges (S/Play/1)](plans/03-protocol.md#node-3-38). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.39 [ForgetChunks (S/Play/2)](plans/03-protocol.md#node-3-39). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.40 [PlayerState (S/Play/3)](plans/03-protocol.md#node-3-40). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.41 [CommandRejected (S/Play/4)](plans/03-protocol.md#node-3-41). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.42 [KeepAlive (S/Play/5)](plans/03-protocol.md#node-3-42). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.43 [Disconnect (S/Play/6)](plans/03-protocol.md#node-3-43). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.44 [RemotePlayerSpawn (S/Play/7)](plans/03-protocol.md#node-3-44). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.45 [RemotePlayerDespawn (S/Play/8)](plans/03-protocol.md#node-3-45). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.46 [RemotePlayerStates (S/Play/9)](plans/03-protocol.md#node-3-46). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.47 [InventoryState (S/Play/10)](plans/03-protocol.md#node-3-47). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.48 [ItemDropUpserts (S/Play/11)](plans/03-protocol.md#node-3-48). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.49 [ItemDropRemoves (S/Play/12)](plans/03-protocol.md#node-3-49). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.50 [FurnaceState (S/Play/13)](plans/03-protocol.md#node-3-50). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.51 [ContainerClosed (S/Play/14)](plans/03-protocol.md#node-3-51). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.52 [ChestState (S/Play/15)](plans/03-protocol.md#node-3-52). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.53 [ChatEvent (S/Play/16)](plans/03-protocol.md#node-3-53). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.54 [CompanionSpawn (S/Play/17)](plans/03-protocol.md#node-3-54). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.55 [CompanionStates (S/Play/18)](plans/03-protocol.md#node-3-55). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.56 [CompanionDespawn (S/Play/19)](plans/03-protocol.md#node-3-56). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.57 [PlaceBlockSucceeded (S/Play/20)](plans/03-protocol.md#node-3-57). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.58 [CraftingState (S/Play/21)](plans/03-protocol.md#node-3-58). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.59 [HostileSpawn (S/Play/22)](plans/03-protocol.md#node-3-59). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.60 [HostileState (S/Play/23)](plans/03-protocol.md#node-3-60). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.61 [HostileDespawn (S/Play/24)](plans/03-protocol.md#node-3-61). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.62 [CombatHit (S/Play/25)](plans/03-protocol.md#node-3-62). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.63 [PassiveSpawn (S/Play/26)](plans/03-protocol.md#node-3-63). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.64 [PassiveState (S/Play/27)](plans/03-protocol.md#node-3-64). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.65 [PassiveDespawn (S/Play/28)](plans/03-protocol.md#node-3-65). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.66 [ProjectileSpawn (S/Play/29)](plans/03-protocol.md#node-3-66). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.67 [ProjectileState (S/Play/30)](plans/03-protocol.md#node-3-67). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.68 [ProjectileDespawn (S/Play/31)](plans/03-protocol.md#node-3-68). Direct prerequisites: 1.5, 3.1, 3.2. Execute the linked packet's named red/green and integration commands.
- [ ] 3.69 [Allocation and encoder audit closure](plans/03-protocol.md#node-3-69). Direct prerequisites: 3.4, 3.5. Execute the linked packet's named red/green and integration commands.

## 4. Storage repairs and format families

- [ ] 4.1 [Legacy companion queue ownership](plans/04-storage.md#node-4-1). Direct prerequisites: 1.5. Execute the linked packet's named red/green and integration commands.
- [ ] 4.2 [Companion cardinality and membership](plans/04-storage.md#node-4-2). Direct prerequisites: 4.1. Execute the linked packet's named red/green and integration commands.
- [ ] 4.3 [Chunk aggregate validation on every encoder](plans/04-storage.md#node-4-3). Direct prerequisites: 1.5. Execute the linked packet's named red/green and integration commands.
- [ ] 4.4 [Fixed region-bank representation](plans/04-storage.md#node-4-4). Direct prerequisites: 1.5. Execute the linked packet's named red/green and integration commands.
- [ ] 4.5 [Share current values while keeping raw historical DTOs](plans/04-storage.md#node-4-5). Direct prerequisites: 2.3, 2.8, 4.3, 4.4. Execute the linked packet's named red/green and integration commands.
- [ ] 4.6 [Player codec and all nine migrations](plans/04-storage.md#node-4-6). Direct prerequisites: 4.5, 1.5. Execute the linked packet's named red/green and integration commands.
- [ ] 4.7 [Metadata byte fidelity and six versions](plans/04-storage.md#node-4-7). Direct prerequisites: 4.6, 1.5. Execute the linked packet's named red/green and integration commands.
- [ ] 4.8 [Hostile records and canonical output](plans/04-storage.md#node-4-8). Direct prerequisites: 4.6. Execute the linked packet's named red/green and integration commands.
- [ ] 4.9 [Passive records and reserved bytes](plans/04-storage.md#node-4-9). Direct prerequisites: 4.6. Execute the linked packet's named red/green and integration commands.
- [ ] 4.10 [Reusable chunk codec with bounded compression](plans/04-storage.md#node-4-10). Direct prerequisites: 4.3, 4.5, 4.6. Execute the linked packet's named red/green and integration commands.
- [ ] 4.11 [Companion writer and complete version corpus](plans/04-storage.md#node-4-11). Direct prerequisites: 4.1, 4.2, 4.5, 4.6. Execute the linked packet's named red/green and integration commands.
- [ ] 4.12 [Region caller buffers and corruption closure](plans/04-storage.md#node-4-12). Direct prerequisites: 4.4, 4.6. Execute the linked packet's named red/green and integration commands.

## 5. Numerical APIs and Rust-native pathfinding

- [ ] 5.1 [Common surface and FFI publication order](plans/05-kernel.md#node-5-1). Direct prerequisites: 6.2. Execute the linked packet's named red/green and integration commands.
- [ ] 5.2 [Collision typed grid and result](plans/05-kernel.md#node-5-2). Direct prerequisites: 5.1. Execute the linked packet's named red/green and integration commands.
- [ ] 5.3 [Physics with explicit controls and tunables](plans/05-kernel.md#node-5-3). Direct prerequisites: 5.2. Execute the linked packet's named red/green and integration commands.
- [ ] 5.4 [Opaque ray continuation](plans/05-kernel.md#node-5-4). Direct prerequisites: 5.1. Execute the linked packet's named red/green and integration commands.
- [ ] 5.5 [World parameters and chunk generation](plans/05-kernel.md#node-5-5). Direct prerequisites: 5.1. Execute the linked packet's named red/green and integration commands.
- [ ] 5.6 [Typed world probes](plans/05-kernel.md#node-5-6). Direct prerequisites: 5.5. Execute the linked packet's named red/green and integration commands.
- [ ] 5.7 [Runtime tree blocks](plans/05-kernel.md#node-5-7). Direct prerequisites: 5.1. Execute the linked packet's named red/green and integration commands.
- [ ] 5.8 [LOD shell with reusable samples and stage](plans/05-kernel.md#node-5-8). Direct prerequisites: 5.5. Execute the linked packet's named red/green and integration commands.
- [ ] 5.9 [Fluid evaluation with native bounded batch](plans/05-kernel.md#node-5-9). Direct prerequisites: 5.1. Execute the linked packet's named red/green and integration commands.
- [ ] 5.10 [Fluid rescan safe halo and explicit continuation](plans/05-kernel.md#node-5-10). Direct prerequisites: 5.1. Execute the linked packet's named red/green and integration commands.
- [ ] 5.11 [Mesh/light typed views and atomic geometry](plans/05-kernel.md#node-5-11). Direct prerequisites: 5.1. Execute the linked packet's named red/green and integration commands.
- [ ] 5.12 [Immutable path grid and Go corpus](plans/05-kernel.md#node-5-12). Direct prerequisites: 6.2. Execute the linked packet's named red/green and integration commands.
- [ ] 5.13 [Indexed-heap A* preserving Go choices](plans/05-kernel.md#node-5-13). Direct prerequisites: 5.12. Execute the linked packet's named red/green and integration commands.
- [ ] 5.14 [Numerical corpus closure](plans/05-kernel.md#node-5-14). Direct prerequisites: 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 5.9, 5.10, 5.11, 5.13. Execute the linked packet's named red/green and integration commands.

## 6. Controller integration and acceptance

- [ ] 6.1 [Merge domain/protocol/storage evidence](plans/06-acceptance.md#node-6-1). Direct prerequisites: 1.6, 2.14, 3.69, 4.7, 4.8, 4.9, 4.10, 4.11, 4.12. Execute the linked packet's named red/green and integration commands.
- [ ] 6.2 [Foundation contract gate before numerical workers](plans/06-acceptance.md#node-6-2). Direct prerequisites: 6.1. Execute the linked packet's named red/green and integration commands.
- [ ] 6.3 [Executed differential replay and complete inventory](plans/06-acceptance.md#node-6-3). Direct prerequisites: 6.2, 5.14. Execute the linked packet's named red/green and integration commands.
- [ ] 6.4 [Mutation and failure gate](plans/06-acceptance.md#node-6-4). Direct prerequisites: 6.3. Execute the linked packet's named red/green and integration commands.
- [ ] 6.5 [Architecture, guide and ledger review](plans/06-acceptance.md#node-6-5). Direct prerequisites: 6.4. Execute the linked packet's named red/green and integration commands.
- [ ] 6.6 [Final stage gates and release evidence](plans/06-acceptance.md#node-6-6). Direct prerequisites: 6.5. Execute the linked packet's named red/green and integration commands.

## Historical ID migration

| Previous revision | Replacement nodes |
| --- | --- |
|1.1/1.2 inventory/trace acceptance|1.1–1.6,6.1,6.3–6.4|
|1.3 manifest|1.1|
|1.4/1.5 trace identity/isolation|1.3/1.4|
|1.6/1.7 Go protocol/save adapters|1.5 plus each individual3.10–3.68 and4.1–4.12 producer|
|1.8 domain/Agent evidence|1.6,2.2–2.14|
|2.1 crate registration|2.1, retained accepted|
|2.2/2.3/2.4 domain/protocol/storage acceptance|6.1/6.2|
|2.5–2.8 four storage repairs|4.1–4.4|
|2.9 admission|3.3,3.10,3.13|
|2.10/2.11 shared values/migration|2.2/2.3/2.8,3.2,4.5|
|2.12 command design|2.4–2.6|
|2.13 unspecified event design|2.7–2.14; all field/ownership decisions now written|
|2.14 registry|3.4|
|2.15 framing/fixed codecs|3.1,3.16,3.47,3.69|
|2.16 unspecified encoder families|all59 named packet nodes and4.6–4.12|
|3.1/3.4–3.7 unspecified kernel packets|5.1–5.11,5.14; all native signatures/capacities now written|
|3.2/3.8/3.9 pathfinding|5.12/5.13|
|3.3 replay|6.3/6.4|
|3.10 kernel oracle|each5.2–5.13 producer and5.14|
|4.1 audit boundary|1.2|
|4.2/4.3 closeout|6.5/6.6|

Newly source-inspected metadata fidelity is explicitly assigned4.7; it is not claimed as an executed runtime repair. The companion active cap is4, not the erroneous32 in the previous planning text. The checked-in review and historical ledger are retained as evidence; this table prevents reinterpreting their old IDs as new tasks.
