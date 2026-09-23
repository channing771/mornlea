//! Versioned framing, negotiation, and packet-family codecs.
//!
//! This crate may depend on `mornlea_domain` and must not depend on save
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Packet families are ported one inventory row at a time. Framing rejects
//! empty, oversized, truncated, and non-canonical length prefixes before
//! publishing a payload.
//!
//! Inbound negotiation is split in two: a packet family decodes a structurally
//! valid record and keeps the peer's raw fields (`*_inbound` decoders), and the
//! pure admission functions in [`admission`] decide whether those raw fields
//! are usable. No admission function owns a session, a deadline or a transport.

#![deny(unsafe_code)]

mod admission;
mod batch;
mod block;
mod block_changes;
mod bone_meal;
mod bytes;
mod chat_command;
mod chat_event;
mod chest_state;
mod chunk_snapshot;
mod client_hello;
mod close_container;
mod codec;
mod collect_water;
mod combat_hit;
mod command_rejected;
mod companion_despawn;
mod companion_spawn;
mod companion_states;
mod container_closed;
mod container_ref;
mod crafting_state;
mod disconnect;
mod drop_id;
mod drop_selected_item;
mod entity_id;
mod equip_armor;
mod error;
mod forget_chunks;
mod frame;
mod furnace_state;
mod handshake_reject;
mod hostile_despawn;
mod hostile_id;
mod hostile_spawn;
mod hostile_state;
mod inventory_state;
mod item_drop_removes;
mod item_drop_upserts;
mod item_stack;
mod keep_alive;
mod keep_alive_reply;
mod login_reject;
mod login_start;
mod login_success;
mod move_container_stack;
mod move_crafting_stack;
mod move_inventory_stack;
mod move_stack_partial;
mod open_container;
mod passive_despawn;
mod passive_spawn;
mod passive_state;
mod place_block;
mod place_block_succeeded;
mod place_water;
mod player_id;
mod player_input;
mod player_state;
mod projectile_despawn;
mod projectile_spawn;
mod projectile_state;
mod remote_player_despawn;
mod remote_player_spawn;
mod remote_player_states;
mod request_chunk_resync;
mod select_hotbar;
mod server_hello;
mod take_crafting_output;
mod till_soil;
mod varint;

pub use admission::{
    AdmittedLogin, HandshakeRejection, LoginAdmissionError, admit_login, validate_hello,
};
pub use block::{
    BLOCKS_PER_SECTION, MAX_CHUNK_BLOCK_INDEX, MAX_Y, MIN_Y, SECTION_SIZE, SECTIONS_PER_CHUNK,
};
pub use block_changes::{BlockChange, BlockChanges, MAX_BLOCK_CHANGES};
pub use bone_meal::BoneMeal;
pub use chat_command::{CHAT_COMMAND_MAX_WIRE_BYTES, CHAT_COMMAND_TEXT_MAX_BYTES, ChatCommand};
pub use chat_event::{
    CHAT_EVENT_ACCEPTED, CHAT_EVENT_COMPANION_SPEECH, CHAT_EVENT_MAX_WIRE_BYTES,
    CHAT_EVENT_REJECTED, CHAT_EVENT_TASK_COMPLETED, CHAT_EVENT_TASK_FAILED,
    CHAT_EVENT_TASK_PROGRESS, CHAT_EVENT_TASK_STARTED, CHAT_EVENT_TASK_STOPPED,
    CHAT_EVENT_TASK_TIMED_OUT, CHAT_REJECT_INVALID_FORMAT, CHAT_REJECT_NONE,
    CHAT_REJECT_NOT_FOLLOWING, CHAT_REJECT_QUEUE_FULL, CHAT_REJECT_UNKNOWN_COMPANION,
    CHAT_SPEECH_TEXT_MAX_BYTES, ChatEvent, TASK_FAIL_INVALID_PLAN, TASK_FAIL_INVENTORY_FULL,
    TASK_FAIL_PATH_UNREACHABLE, TASK_FAIL_PLANNER_UNAVAILABLE, TASK_FAIL_WORLD_CHANGED,
};
pub use chest_state::{CHEST_SLOTS, ChestState};
pub use chunk_snapshot::{
    ChunkSnapshot, MAX_COMPRESSED_SNAPSHOT, MAX_DECODED_SNAPSHOT, SNAPSHOT_ENVELOPE_LENGTH,
    SectionData, SectionStorage, SnapshotEnvelope, compress_logical,
};
pub use client_hello::{ClientHello, InboundHello};
pub use close_container::CloseContainer;
pub use codec::ProtocolCodec;
pub use collect_water::CollectWater;
pub use combat_hit::{
    COMBAT_TARGET_HOSTILE, COMBAT_TARGET_PASSIVE, COMBAT_TARGET_PLAYER, CombatHit, MAX_HEALTH,
};
pub use command_rejected::{
    CommandRejected, REJECT_BUCKET_MISMATCH, REJECT_CHUNK_NOT_READY, REJECT_CONTAINER_CAPACITY,
    REJECT_DROP_CAPACITY, REJECT_HOTBAR_FULL, REJECT_INVALID_BLOCK, REJECT_INVALID_INPUT,
    REJECT_INVALID_RAY, REJECT_INVALID_SLOT, REJECT_NO_TARGET, REJECT_NOT_ARMOR,
    REJECT_NOT_FLUID_SOURCE, REJECT_OCCUPIED, REJECT_PLAYER_NOT_READY, REJECT_PROTECTED_BLOCK,
    reject_reason_from_wire, reject_reason_to_wire,
};
pub use companion_despawn::CompanionDespawn;
pub use companion_spawn::{COMPANION_SPAWN_MAX_WIRE_BYTES, CompanionSpawn};
pub use companion_states::{
    COMPANION_STATE_WIRE_BYTES, COMPANION_STATES_MAX_WIRE_BYTES, CompanionState, CompanionStates,
    MAX_COMPANION_STATES,
};
pub use container_closed::ContainerClosed;
pub use container_ref::{
    CHESTS_PER_CHUNK, CONTAINER_KIND_CHEST, CONTAINER_KIND_FURNACE, ContainerRef,
    FURNACES_PER_CHUNK,
};
pub use crafting_state::{
    CRAFTING_GRID_SIZE_PERSONAL, CRAFTING_GRID_SIZE_WORKBENCH, CraftingState,
};
pub use disconnect::{
    DISCONNECT_INTERNAL_ERROR, DISCONNECT_PROTOCOL_VIOLATION, DISCONNECT_SERVER_SHUTDOWN,
    DISCONNECT_SLOW_CLIENT, DISCONNECT_TIMEOUT, Disconnect,
};
pub use drop_id::{DROP_ID_WIRE_BYTES, DropId, MAX_ITEM_DROP_BATCH};
pub use drop_selected_item::DropSelectedItem;
pub use entity_id::{CompanionId, valid_companion_name, valid_display_name};
pub use equip_armor::EquipArmor;
pub use error::ProtocolError;
pub use forget_chunks::{ForgetChunks, MAX_FORGET_CHUNKS};
pub use frame::{
    FrameRef, MAX_FRAME_BYTES, read_frame, read_frame_ref, write_frame, write_frame_into,
};
pub use furnace_state::{
    FURNACE_BURN_TICKS, FURNACE_SMELT_TICKS, FurnaceState, is_smelting_product,
};
pub use handshake_reject::{HANDSHAKE_VERSION_MISMATCH, HandshakeReject};
pub use hostile_despawn::{HOSTILE_DESPAWN_WIRE_BYTES, HostileDespawn, MAX_HOSTILE_RECORDS};
pub use hostile_id::{HOSTILE_ID_WIRE_BYTES, HostileId};
pub use hostile_spawn::{
    HOSTILE_KIND_BONE_THROWER, HOSTILE_KIND_NIGHTWALKER, HOSTILE_SPAWN_MAX_RECORDS,
    HOSTILE_SPAWN_WIRE_BYTES, HostileSpawn, HostileSpawnRecord,
};
pub use hostile_state::{
    HOSTILE_STATE_MAX_RECORDS, HOSTILE_STATE_WIRE_BYTES, HostileState, HostileStateRecord,
};
pub use inventory_state::{
    BACKPACK_SLOTS, HOTBAR_SLOTS, INVENTORY_STATE_WIRE_BYTES, InventoryState,
};
pub use item_drop_removes::ItemDropRemoves;
pub use item_drop_upserts::{ITEM_DROP_WIRE_BYTES, ItemDrop, ItemDropUpserts};
pub use item_stack::{
    ITEM_COAL, ITEM_ID_MAX, ITEM_NONE, ITEM_STONE, ITEM_STONE_PICKAXE, ItemStack, smelting_output,
    valid_furnace_output,
};
pub use keep_alive::KeepAlive;
pub use keep_alive_reply::KeepAliveReply;
pub use login_reject::{
    LOGIN_ALREADY_ONLINE, LOGIN_INTERNAL_ERROR, LOGIN_INVALID_IDENTITY, LOGIN_PLAYER_DATA_CORRUPT,
    LOGIN_PROTOCOL_VIOLATION, LOGIN_SERVER_FULL, LOGIN_STORE_UNAVAILABLE, LoginReject,
};
pub use login_start::{
    InboundLoginStart, LOGIN_VIEW_DISTANCE_MAX, LOGIN_VIEW_DISTANCE_MIN, LoginStart,
    MAX_SMALL_PAYLOAD_BYTES,
};
pub use login_success::LoginSuccess;
pub use move_container_stack::{
    CHEST_VIEW_SLOTS, FURNACE_OUTPUT_SLOT, FURNACE_VIEW_SLOTS, MoveContainerStack,
};
pub use move_crafting_stack::{CRAFTING_GRID_SLOTS, GRID_CRAFTING_VIEW_SLOTS, MoveCraftingStack};
pub use move_inventory_stack::{INVENTORY_SLOTS, MoveInventoryStack};
pub use move_stack_partial::{
    DropStack, MoveStackPartial, QuickMoveStack, STACK_VIEW_CONTAINER, STACK_VIEW_CRAFTING,
    STACK_VIEW_INVENTORY,
};
pub use open_container::OpenContainer;
pub use passive_despawn::{
    MAX_PASSIVE_RECORDS, PASSIVE_DESPAWN_DIED, PASSIVE_DESPAWN_VANISHED, PassiveDespawn,
    PassiveDespawnRecord,
};
pub use passive_spawn::{
    MAX_PASSIVE_SPAWN_RECORDS, PASSIVE_SPAWN_MAX_WIRE_BYTES, PASSIVE_SPAWN_WIRE_BYTES,
    PassiveSpawn, PassiveSpawnRecord,
};
pub use passive_state::{
    PASSIVE_STATE_MAX_WIRE_BYTES, PASSIVE_STATE_WIRE_BYTES, PassiveState, PassiveStateRecord,
};
pub use place_block::PlaceBlock;
pub use place_block_succeeded::PlaceBlockSucceeded;
pub use place_water::PlaceWater;
pub use player_id::PlayerId;
pub use player_input::PlayerInput;
pub use player_state::{
    BlockPos, DAY_PHASE_TICKS_MAX, MAX_ARMOR_POINTS, MAX_HUNGER, MAX_OXYGEN_TICKS,
    PLAYER_STATE_WIRE_BYTES, PlayerState, SEASON_AUTUMN, SEASON_SPRING, SEASON_SUMMER,
    SEASON_WINTER, WEATHER_CLEAR, WEATHER_RAIN, WEATHER_THUNDER,
};
pub use projectile_despawn::{
    MAX_PROJECTILE_RECORDS, PROJECTILE_DESPAWN_WIRE_BYTES, ProjectileDespawn,
};
pub use projectile_spawn::{
    PROJECTILE_KIND_ARROW, PROJECTILE_KIND_SHARD, PROJECTILE_SPAWN_MAX_WIRE_BYTES,
    PROJECTILE_SPAWN_WIRE_BYTES, ProjectileSpawn, ProjectileSpawnRecord,
};
pub use projectile_state::{
    PROJECTILE_STATE_MAX_WIRE_BYTES, PROJECTILE_STATE_WIRE_BYTES, ProjectileState,
    ProjectileStateRecord,
};
pub use remote_player_despawn::RemotePlayerDespawn;
pub use remote_player_spawn::RemotePlayerSpawn;
pub use remote_player_states::{
    MAX_REMOTE_PLAYER_STATES, REMOTE_PLAYER_STATE_WIRE_BYTES, REMOTE_PLAYER_STATES_MAX_WIRE_BYTES,
    RemotePlayerState, RemotePlayerStates,
};
pub use request_chunk_resync::RequestChunkResync;
pub use select_hotbar::SelectHotbar;
pub use server_hello::ServerHello;
pub use take_crafting_output::TakeCraftingOutput;
pub use till_soil::TillSoil;
pub use varint::{decode_uvarint, encode_uvarint};

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_protocol";

/// Domain crate identity that protocol records share.
pub const DOMAIN_CRATE_NAME: &str = mornlea_domain::CRATE_NAME;
