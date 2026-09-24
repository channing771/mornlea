//! The closed v45 packet registry: direction, state, key and typed dispatch.
//!
//! [`Direction`], [`State`] and [`PacketKey`] are the three-part key the Go
//! registry freezes. Numeric IDs collide across direction and state by design
//! — C/Play/4 is a keep alive reply while S/Play/4 is a command rejection — so
//! the complete key, never the bare ID, is what identifies a family.
//!
//! [`ClientPacket`] and [`ServerPacket`] are exhaustive enums over exactly the
//! 59 keys the Go v45 registry publishes: 23 client keys (C/Handshake/0,
//! C/Login/0 and the 21 registered C/Play IDs) and 36 server keys
//! (S/Handshake/0..1, S/Login/0..1 and the 32 registered S/Play IDs). Framing
//! stays outside both enums, because the registry owns packet keys and not the
//! frame envelope. There is no catch-all variant, no `Box<dyn Packet>` and no
//! string-keyed module lookup, so adding a packet means adding a variant plus a
//! match arm, and a missing arm is a compile error rather than a silent
//! fallback.
//!
//! Decoding dispatches over the complete `(state, id)` pair. An unregistered
//! key — including the retired C/Play/1, the unassigned C/Play/22 and the
//! unassigned S/Play/32 — is [`ProtocolError::UnknownPacket`] before a DTO is
//! published. Encoding is the inverse: the variant names its own key, and
//! because the output path carries no state argument a record cannot be
//! encoded under a state it was not decoded under, so wrong-state encoding is
//! unrepresentable rather than something to refuse.
//!
//! The two inbound negotiation variants carry the raw records
//! ([`InboundHello`], [`InboundLoginStart`]) rather than the strict outbound
//! types, because a peer running another version has to receive the negotiated
//! mismatch answer instead of a bare decode failure; see [`crate::admission`].
//! Their `encode_into` re-publishes the raw bytes the decoder admitted, which
//! is why the version rule and the canonical display-name rule stay in
//! `validate_hello` / `admit_login` and in the outbound [`ClientHello`] /
//! [`LoginStart`] instead of in a raw record's gate.
//!
//! Every other variant holds its concrete packet type, and each concrete
//! `encode_into` already runs the crate-wide `validate` → checked
//! `encoded_len` → capacity → publish chain. Dispatch therefore adds no second
//! validation policy: the registry decides which family a key names, and the
//! family decides whether the record it carries is publishable.
//!
//! The Go `registry.go` table is the read-only source of this map. The runtime
//! never reads Go source or the test manifest; `tests/protocol_registry.rs`
//! pins the map against the reviewed Go-produced payloads.

use crate::block_changes::BlockChanges;
use crate::bone_meal::BoneMeal;
use crate::chat_command::ChatCommand;
use crate::chat_event::ChatEvent;
use crate::chest_state::ChestState;
use crate::chunk_snapshot::ChunkSnapshot;
use crate::client_hello::{ClientHello, InboundHello};
use crate::close_container::CloseContainer;
use crate::codec::ProtocolCodec;
use crate::collect_water::CollectWater;
use crate::combat_hit::CombatHit;
use crate::command_rejected::CommandRejected;
use crate::companion_despawn::CompanionDespawn;
use crate::companion_spawn::CompanionSpawn;
use crate::companion_states::CompanionStates;
use crate::container_closed::ContainerClosed;
use crate::crafting_state::CraftingState;
use crate::disconnect::Disconnect;
use crate::drop_selected_item::DropSelectedItem;
use crate::equip_armor::EquipArmor;
use crate::error::ProtocolError;
use crate::forget_chunks::ForgetChunks;
use crate::furnace_state::FurnaceState;
use crate::handshake_reject::HandshakeReject;
use crate::hostile_despawn::HostileDespawn;
use crate::hostile_spawn::HostileSpawn;
use crate::hostile_state::HostileState;
use crate::inventory_state::InventoryState;
use crate::item_drop_removes::ItemDropRemoves;
use crate::item_drop_upserts::ItemDropUpserts;
use crate::keep_alive::KeepAlive;
use crate::keep_alive_reply::KeepAliveReply;
use crate::login_reject::LoginReject;
use crate::login_start::{InboundLoginStart, LoginStart, MAX_SMALL_PAYLOAD_BYTES};
use crate::login_success::LoginSuccess;
use crate::move_container_stack::MoveContainerStack;
use crate::move_crafting_stack::MoveCraftingStack;
use crate::move_inventory_stack::MoveInventoryStack;
use crate::move_stack_partial::{DropStack, MoveStackPartial, QuickMoveStack};
use crate::open_container::OpenContainer;
use crate::passive_despawn::PassiveDespawn;
use crate::passive_spawn::PassiveSpawn;
use crate::passive_state::PassiveState;
use crate::place_block::PlaceBlock;
use crate::place_block_succeeded::PlaceBlockSucceeded;
use crate::place_water::PlaceWater;
use crate::player_input::PlayerInput;
use crate::player_state::PlayerState;
use crate::projectile_despawn::ProjectileDespawn;
use crate::projectile_spawn::ProjectileSpawn;
use crate::projectile_state::ProjectileState;
use crate::remote_player_despawn::RemotePlayerDespawn;
use crate::remote_player_spawn::RemotePlayerSpawn;
use crate::remote_player_states::RemotePlayerStates;
use crate::request_chunk_resync::RequestChunkResync;
use crate::select_hotbar::SelectHotbar;
use crate::server_hello::ServerHello;
use crate::take_crafting_output::TakeCraftingOutput;
use crate::till_soil::TillSoil;

/// Which side of the connection publishes a packet.
///
/// The direction is part of the key rather than a property of the packet,
/// because the same numeric ID names different records in the two directions.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Direction {
    /// The client publishes the record to the server.
    ClientToServer,
    /// The server publishes the record to the client.
    ServerToClient,
}

/// The connection phase a packet belongs to.
///
/// The three states are the closed Go set. A peer cannot name a fourth phase,
/// so an unknown state byte is a Go-side refusal this crate makes
/// unrepresentable rather than a variant it has to reject.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum State {
    /// Version negotiation, before any identity is known.
    Handshake,
    /// Identity and view-distance admission.
    Login,
    /// Steady-state play traffic.
    Play,
}

/// The complete three-part key that identifies one packet family.
///
/// Direction, state and ID are all required: the ID alone is ambiguous across
/// the other two, which is why a per-struct packet ID cannot serve as the
/// registry key.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PacketKey {
    /// The side that publishes the record.
    pub direction: Direction,
    /// The connection phase the record belongs to.
    pub state: State,
    /// The frozen numeric ID inside that direction and state.
    pub id: u32,
}

/// One client-to-server packet, over exactly the 23 registered client keys.
///
/// The two negotiation variants hold the raw inbound records; every other
/// variant holds its concrete packet type. The enum is closed: a consumer
/// matches on it exhaustively, and a new client packet is a new variant rather
/// than a value an existing arm has to guess about.
#[derive(Clone, Debug, PartialEq)]
pub enum ClientPacket {
    /// C/Handshake/0: the peer's declared protocol version.
    ClientHello(InboundHello),
    /// C/Login/0: the peer's raw identity, display name and view distance.
    LoginStart(InboundLoginStart),
    /// C/Play/0: the highest-frequency movement and held-action record.
    PlayerInput(PlayerInput),
    /// C/Play/2: a placement intent resolved by the authority's ray cast.
    PlaceBlock(PlaceBlock),
    /// C/Play/3: a request for a chunk the client already holds.
    RequestChunkResync(RequestChunkResync),
    /// C/Play/4: the reply to a server keep alive.
    KeepAliveReply(KeepAliveReply),
    /// C/Play/5: the selected hotbar slot.
    SelectHotbar(SelectHotbar),
    /// C/Play/6: an inventory-to-inventory stack move.
    MoveInventoryStack(MoveInventoryStack),
    /// C/Play/7: a crafting-grid stack move.
    MoveCraftingStack(MoveCraftingStack),
    /// C/Play/8: a request to open the container the ray cast hit.
    OpenContainer(OpenContainer),
    /// C/Play/9: a container-view stack move.
    MoveContainerStack(MoveContainerStack),
    /// C/Play/10: the end of a viewed container.
    CloseContainer(CloseContainer),
    /// C/Play/11: a request to drop the selected stack.
    DropSelectedItem(DropSelectedItem),
    /// C/Play/12: a chat instruction the authority interprets.
    ChatCommand(ChatCommand),
    /// C/Play/13: a request to till the fluid-adjacent dirt the ray cast hit.
    TillSoil(TillSoil),
    /// C/Play/14: a request to fertilize the plant the ray cast hit.
    BoneMeal(BoneMeal),
    /// C/Play/15: a request to take the crafting output.
    TakeCraftingOutput(TakeCraftingOutput),
    /// C/Play/16: a request to collect the water source the ray cast hit.
    CollectWater(CollectWater),
    /// C/Play/17: a request to place the held bucket's water.
    PlaceWater(PlaceWater),
    /// C/Play/18: a request to equip the selected armor piece.
    EquipArmor(EquipArmor),
    /// C/Play/19: a partial stack move inside one view.
    MoveStackPartial(MoveStackPartial),
    /// C/Play/20: a quick move to the view's fixed destination.
    QuickMoveStack(QuickMoveStack),
    /// C/Play/21: a full-stack drop from one view.
    DropStack(DropStack),
}

impl ClientPacket {
    /// The complete key this packet publishes under.
    ///
    /// The key is derived from the variant, so it can never disagree with the
    /// arm that decoded the record and with the arm that encodes it.
    pub fn key(&self) -> PacketKey {
        let id = match self {
            ClientPacket::ClientHello(_) | ClientPacket::LoginStart(_) => 0,
            ClientPacket::PlayerInput(_) => 0,
            ClientPacket::PlaceBlock(_) => 2,
            ClientPacket::RequestChunkResync(_) => 3,
            ClientPacket::KeepAliveReply(_) => 4,
            ClientPacket::SelectHotbar(_) => 5,
            ClientPacket::MoveInventoryStack(_) => 6,
            ClientPacket::MoveCraftingStack(_) => 7,
            ClientPacket::OpenContainer(_) => 8,
            ClientPacket::MoveContainerStack(_) => 9,
            ClientPacket::CloseContainer(_) => 10,
            ClientPacket::DropSelectedItem(_) => 11,
            ClientPacket::ChatCommand(_) => 12,
            ClientPacket::TillSoil(_) => 13,
            ClientPacket::BoneMeal(_) => 14,
            ClientPacket::TakeCraftingOutput(_) => 15,
            ClientPacket::CollectWater(_) => 16,
            ClientPacket::PlaceWater(_) => 17,
            ClientPacket::EquipArmor(_) => 18,
            ClientPacket::MoveStackPartial(_) => 19,
            ClientPacket::QuickMoveStack(_) => 20,
            ClientPacket::DropStack(_) => 21,
        };
        let state = match self {
            ClientPacket::ClientHello(_) => State::Handshake,
            ClientPacket::LoginStart(_) => State::Login,
            _ => State::Play,
        };
        PacketKey {
            direction: Direction::ClientToServer,
            state,
            id,
        }
    }
}

/// One server-to-client packet, over exactly the 36 registered server keys.
///
/// The compressed snapshot variant is the one family whose payload length is
/// not precomputed, so its encoder lives on [`ProtocolCodec`] beside the
/// context that owns the compression scratch. Every other variant holds its
/// concrete packet type.
#[derive(Clone, Debug, PartialEq)]
pub enum ServerPacket {
    /// S/Handshake/0: the server's declared protocol version.
    ServerHello(ServerHello),
    /// S/Handshake/1: the version-mismatch answer.
    HandshakeReject(HandshakeReject),
    /// S/Login/0: the admitted identity and world seed.
    LoginSuccess(LoginSuccess),
    /// S/Login/1: the login refusal.
    LoginReject(LoginReject),
    /// S/Play/0: the compressed chunk column.
    ChunkSnapshot(ChunkSnapshot),
    /// S/Play/1: the authoritative block changes of one revision step.
    BlockChanges(BlockChanges),
    /// S/Play/2: the chunks the client may forget.
    ForgetChunks(ForgetChunks),
    /// S/Play/3: the owning session's private body, survival and world state.
    PlayerState(PlayerState),
    /// S/Play/4: the refusal of one sequenced command.
    CommandRejected(CommandRejected),
    /// S/Play/5: the liveness probe.
    KeepAlive(KeepAlive),
    /// S/Play/6: the reason the connection is ending.
    Disconnect(Disconnect),
    /// S/Play/7: a peer session that became visible.
    RemotePlayerSpawn(RemotePlayerSpawn),
    /// S/Play/8: a peer session that stopped being visible.
    RemotePlayerDespawn(RemotePlayerDespawn),
    /// S/Play/9: the visible peer sessions' poses.
    RemotePlayerStates(RemotePlayerStates),
    /// S/Play/10: the owning session's inventory.
    InventoryState(InventoryState),
    /// S/Play/11: the dropped stacks that became visible.
    ItemDropUpserts(ItemDropUpserts),
    /// S/Play/12: the dropped stacks that stopped being visible.
    ItemDropRemoves(ItemDropRemoves),
    /// S/Play/13: the owning session's viewed furnace.
    FurnaceState(FurnaceState),
    /// S/Play/14: the container view that ended.
    ContainerClosed(ContainerClosed),
    /// S/Play/15: the owning session's viewed chest.
    ChestState(ChestState),
    /// S/Play/16: one confirmed chat fact.
    ChatEvent(ChatEvent),
    /// S/Play/17: a companion that became visible.
    CompanionSpawn(CompanionSpawn),
    /// S/Play/18: the visible companions' poses.
    CompanionStates(CompanionStates),
    /// S/Play/19: a companion that stopped being visible.
    CompanionDespawn(CompanionDespawn),
    /// S/Play/20: the acknowledgement of one placement.
    PlaceBlockSucceeded(PlaceBlockSucceeded),
    /// S/Play/21: the owning session's crafting grid.
    CraftingState(CraftingState),
    /// S/Play/22: the hostile mobs that became visible.
    HostileSpawn(HostileSpawn),
    /// S/Play/23: the visible hostile mobs' poses.
    HostileState(HostileState),
    /// S/Play/24: the hostile mobs that stopped being visible.
    HostileDespawn(HostileDespawn),
    /// S/Play/25: the private confirmation of one melee hit.
    CombatHit(CombatHit),
    /// S/Play/26: the passive mobs that became visible.
    PassiveSpawn(PassiveSpawn),
    /// S/Play/27: the visible passive mobs' poses.
    PassiveState(PassiveState),
    /// S/Play/28: the passive mobs that stopped being visible.
    PassiveDespawn(PassiveDespawn),
    /// S/Play/29: the projectiles that became visible.
    ProjectileSpawn(ProjectileSpawn),
    /// S/Play/30: the visible projectiles' positions.
    ProjectileState(ProjectileState),
    /// S/Play/31: the projectiles that stopped being visible.
    ProjectileDespawn(ProjectileDespawn),
}

impl ServerPacket {
    /// The complete key this packet publishes under.
    ///
    /// The key is derived from the variant, so it can never disagree with the
    /// arm that decoded the record and with the arm that encodes it.
    pub fn key(&self) -> PacketKey {
        let (state, id) = match self {
            ServerPacket::ServerHello(_) => (State::Handshake, 0),
            ServerPacket::HandshakeReject(_) => (State::Handshake, 1),
            ServerPacket::LoginSuccess(_) => (State::Login, 0),
            ServerPacket::LoginReject(_) => (State::Login, 1),
            ServerPacket::ChunkSnapshot(_) => (State::Play, 0),
            ServerPacket::BlockChanges(_) => (State::Play, 1),
            ServerPacket::ForgetChunks(_) => (State::Play, 2),
            ServerPacket::PlayerState(_) => (State::Play, 3),
            ServerPacket::CommandRejected(_) => (State::Play, 4),
            ServerPacket::KeepAlive(_) => (State::Play, 5),
            ServerPacket::Disconnect(_) => (State::Play, 6),
            ServerPacket::RemotePlayerSpawn(_) => (State::Play, 7),
            ServerPacket::RemotePlayerDespawn(_) => (State::Play, 8),
            ServerPacket::RemotePlayerStates(_) => (State::Play, 9),
            ServerPacket::InventoryState(_) => (State::Play, 10),
            ServerPacket::ItemDropUpserts(_) => (State::Play, 11),
            ServerPacket::ItemDropRemoves(_) => (State::Play, 12),
            ServerPacket::FurnaceState(_) => (State::Play, 13),
            ServerPacket::ContainerClosed(_) => (State::Play, 14),
            ServerPacket::ChestState(_) => (State::Play, 15),
            ServerPacket::ChatEvent(_) => (State::Play, 16),
            ServerPacket::CompanionSpawn(_) => (State::Play, 17),
            ServerPacket::CompanionStates(_) => (State::Play, 18),
            ServerPacket::CompanionDespawn(_) => (State::Play, 19),
            ServerPacket::PlaceBlockSucceeded(_) => (State::Play, 20),
            ServerPacket::CraftingState(_) => (State::Play, 21),
            ServerPacket::HostileSpawn(_) => (State::Play, 22),
            ServerPacket::HostileState(_) => (State::Play, 23),
            ServerPacket::HostileDespawn(_) => (State::Play, 24),
            ServerPacket::CombatHit(_) => (State::Play, 25),
            ServerPacket::PassiveSpawn(_) => (State::Play, 26),
            ServerPacket::PassiveState(_) => (State::Play, 27),
            ServerPacket::PassiveDespawn(_) => (State::Play, 28),
            ServerPacket::ProjectileSpawn(_) => (State::Play, 29),
            ServerPacket::ProjectileState(_) => (State::Play, 30),
            ServerPacket::ProjectileDespawn(_) => (State::Play, 31),
        };
        PacketKey {
            direction: Direction::ServerToClient,
            state,
            id,
        }
    }
}

/// Applies the Go codec's per-packet payload ceiling before any field is read.
///
/// The Go decoder refuses an oversized payload before it parses one, so an
/// oversized payload is a size refusal rather than a field failure. The
/// answer is [`ProtocolError::Allocation`], the sentinel
/// `LoginStart::decode_inbound` already publishes for the same bytes, so one
/// condition has one answer whichever arm reaches it first.
fn check_small_payload(payload: &[u8]) -> Result<(), ProtocolError> {
    if payload.len() > MAX_SMALL_PAYLOAD_BYTES {
        return Err(ProtocolError::Allocation);
    }
    Ok(())
}

/// Decodes one client-to-server payload under the requested key.
///
/// The dispatch is an exhaustive match over the complete `(state, id)` pair.
/// Every registered arm calls the family's accepted decoder — the structural
/// `decode_inbound` for the two negotiation families and the bounded
/// `decode` for the rest — and every other pair is
/// [`ProtocolError::UnknownPacket`]. The retired C/Play/1 and the unassigned
/// C/Play/22 therefore never reach a decoder.
pub fn decode_client(state: State, id: u32, payload: &[u8]) -> Result<ClientPacket, ProtocolError> {
    check_small_payload(payload)?;
    match (state, id) {
        (State::Handshake, 0) => Ok(ClientPacket::ClientHello(ClientHello::decode_inbound(
            payload,
        )?)),
        (State::Login, 0) => Ok(ClientPacket::LoginStart(LoginStart::decode_inbound(
            payload,
        )?)),
        (State::Play, 0) => Ok(ClientPacket::PlayerInput(PlayerInput::decode(payload)?)),
        (State::Play, 2) => Ok(ClientPacket::PlaceBlock(PlaceBlock::decode(payload)?)),
        (State::Play, 3) => Ok(ClientPacket::RequestChunkResync(
            RequestChunkResync::decode(payload)?,
        )),
        (State::Play, 4) => Ok(ClientPacket::KeepAliveReply(KeepAliveReply::decode(
            payload,
        )?)),
        (State::Play, 5) => Ok(ClientPacket::SelectHotbar(SelectHotbar::decode(payload)?)),
        (State::Play, 6) => Ok(ClientPacket::MoveInventoryStack(
            MoveInventoryStack::decode(payload)?,
        )),
        (State::Play, 7) => Ok(ClientPacket::MoveCraftingStack(MoveCraftingStack::decode(
            payload,
        )?)),
        (State::Play, 8) => Ok(ClientPacket::OpenContainer(OpenContainer::decode(payload)?)),
        (State::Play, 9) => Ok(ClientPacket::MoveContainerStack(
            MoveContainerStack::decode(payload)?,
        )),
        (State::Play, 10) => Ok(ClientPacket::CloseContainer(CloseContainer::decode(
            payload,
        )?)),
        (State::Play, 11) => Ok(ClientPacket::DropSelectedItem(DropSelectedItem::decode(
            payload,
        )?)),
        (State::Play, 12) => Ok(ClientPacket::ChatCommand(ChatCommand::decode(payload)?)),
        (State::Play, 13) => Ok(ClientPacket::TillSoil(TillSoil::decode(payload)?)),
        (State::Play, 14) => Ok(ClientPacket::BoneMeal(BoneMeal::decode(payload)?)),
        (State::Play, 15) => Ok(ClientPacket::TakeCraftingOutput(
            TakeCraftingOutput::decode(payload)?,
        )),
        (State::Play, 16) => Ok(ClientPacket::CollectWater(CollectWater::decode(payload)?)),
        (State::Play, 17) => Ok(ClientPacket::PlaceWater(PlaceWater::decode(payload)?)),
        (State::Play, 18) => Ok(ClientPacket::EquipArmor(EquipArmor::decode(payload)?)),
        (State::Play, 19) => Ok(ClientPacket::MoveStackPartial(MoveStackPartial::decode(
            payload,
        )?)),
        (State::Play, 20) => Ok(ClientPacket::QuickMoveStack(QuickMoveStack::decode(
            payload,
        )?)),
        (State::Play, 21) => Ok(ClientPacket::DropStack(DropStack::decode(payload)?)),
        _ => Err(ProtocolError::UnknownPacket),
    }
}

/// Encodes one typed client packet into the caller's buffer and returns the
/// bytes written.
///
/// The key is obtained from the variant first, so an arm can never publish a
/// record under a direction the variant does not name. The concrete encoder is
/// the only writer and already revalidates the record, sizes it with checked
/// arithmetic, tests the destination and publishes only `dst[..written]`, so a
/// short or invalid call leaves every destination byte unchanged. No state
/// argument exists on this path, which is what makes encoding a record under a
/// state it was not decoded under unrepresentable.
pub fn encode_client_into(packet: &ClientPacket, dst: &mut [u8]) -> Result<usize, ProtocolError> {
    let key = packet.key();
    debug_assert_eq!(
        key.direction,
        Direction::ClientToServer,
        "every client variant publishes under the client key space"
    );
    match packet {
        ClientPacket::ClientHello(record) => record.encode_into(dst),
        ClientPacket::LoginStart(record) => record.encode_into(dst),
        ClientPacket::PlayerInput(record) => record.encode_into(dst),
        ClientPacket::PlaceBlock(record) => record.encode_into(dst),
        ClientPacket::RequestChunkResync(record) => record.encode_into(dst),
        ClientPacket::KeepAliveReply(record) => record.encode_into(dst),
        ClientPacket::SelectHotbar(record) => record.encode_into(dst),
        ClientPacket::MoveInventoryStack(record) => record.encode_into(dst),
        ClientPacket::MoveCraftingStack(record) => record.encode_into(dst),
        ClientPacket::OpenContainer(record) => record.encode_into(dst),
        ClientPacket::MoveContainerStack(record) => record.encode_into(dst),
        ClientPacket::CloseContainer(record) => record.encode_into(dst),
        ClientPacket::DropSelectedItem(record) => record.encode_into(dst),
        ClientPacket::ChatCommand(record) => record.encode_into(dst),
        ClientPacket::TillSoil(record) => record.encode_into(dst),
        ClientPacket::BoneMeal(record) => record.encode_into(dst),
        ClientPacket::TakeCraftingOutput(record) => record.encode_into(dst),
        ClientPacket::CollectWater(record) => record.encode_into(dst),
        ClientPacket::PlaceWater(record) => record.encode_into(dst),
        ClientPacket::EquipArmor(record) => record.encode_into(dst),
        ClientPacket::MoveStackPartial(record) => record.encode_into(dst),
        ClientPacket::QuickMoveStack(record) => record.encode_into(dst),
        ClientPacket::DropStack(record) => record.encode_into(dst),
    }
}

impl ProtocolCodec {
    /// Decodes one server-to-client payload under the requested key.
    ///
    /// The dispatch is an exhaustive match over the complete `(state, id)`
    /// pair, exactly as [`decode_client`] is. The compressed snapshot is the
    /// one family the 64 KiB small-payload ceiling does not bound — its own
    /// compressed and decoded ceilings are checked before the frame is
    /// touched — so the ceiling is applied to every other family, matching the
    /// Go decoder's split between the control decoder and the snapshot codec.
    /// The snapshot arm routes through this codec's owned decompressor, which
    /// is why decoding a server packet needs the context owner and decoding a
    /// client packet does not.
    pub fn decode_server(
        &mut self,
        state: State,
        id: u32,
        payload: &[u8],
    ) -> Result<ServerPacket, ProtocolError> {
        if !matches!((state, id), (State::Play, 0)) {
            check_small_payload(payload)?;
        }
        match (state, id) {
            (State::Handshake, 0) => Ok(ServerPacket::ServerHello(ServerHello::decode(payload)?)),
            (State::Handshake, 1) => Ok(ServerPacket::HandshakeReject(HandshakeReject::decode(
                payload,
            )?)),
            (State::Login, 0) => Ok(ServerPacket::LoginSuccess(LoginSuccess::decode(payload)?)),
            (State::Login, 1) => Ok(ServerPacket::LoginReject(LoginReject::decode(payload)?)),
            (State::Play, 0) => Ok(ServerPacket::ChunkSnapshot(self.decode_snapshot(payload)?)),
            (State::Play, 1) => Ok(ServerPacket::BlockChanges(BlockChanges::decode(payload)?)),
            (State::Play, 2) => Ok(ServerPacket::ForgetChunks(ForgetChunks::decode(payload)?)),
            (State::Play, 3) => Ok(ServerPacket::PlayerState(PlayerState::decode(payload)?)),
            (State::Play, 4) => Ok(ServerPacket::CommandRejected(CommandRejected::decode(
                payload,
            )?)),
            (State::Play, 5) => Ok(ServerPacket::KeepAlive(KeepAlive::decode(payload)?)),
            (State::Play, 6) => Ok(ServerPacket::Disconnect(Disconnect::decode(payload)?)),
            (State::Play, 7) => Ok(ServerPacket::RemotePlayerSpawn(RemotePlayerSpawn::decode(
                payload,
            )?)),
            (State::Play, 8) => Ok(ServerPacket::RemotePlayerDespawn(
                RemotePlayerDespawn::decode(payload)?,
            )),
            (State::Play, 9) => Ok(ServerPacket::RemotePlayerStates(
                RemotePlayerStates::decode(payload)?,
            )),
            (State::Play, 10) => Ok(ServerPacket::InventoryState(InventoryState::decode(
                payload,
            )?)),
            (State::Play, 11) => Ok(ServerPacket::ItemDropUpserts(ItemDropUpserts::decode(
                payload,
            )?)),
            (State::Play, 12) => Ok(ServerPacket::ItemDropRemoves(ItemDropRemoves::decode(
                payload,
            )?)),
            (State::Play, 13) => Ok(ServerPacket::FurnaceState(FurnaceState::decode(payload)?)),
            (State::Play, 14) => Ok(ServerPacket::ContainerClosed(ContainerClosed::decode(
                payload,
            )?)),
            (State::Play, 15) => Ok(ServerPacket::ChestState(ChestState::decode(payload)?)),
            (State::Play, 16) => Ok(ServerPacket::ChatEvent(ChatEvent::decode(payload)?)),
            (State::Play, 17) => Ok(ServerPacket::CompanionSpawn(CompanionSpawn::decode(
                payload,
            )?)),
            (State::Play, 18) => Ok(ServerPacket::CompanionStates(CompanionStates::decode(
                payload,
            )?)),
            (State::Play, 19) => Ok(ServerPacket::CompanionDespawn(CompanionDespawn::decode(
                payload,
            )?)),
            (State::Play, 20) => Ok(ServerPacket::PlaceBlockSucceeded(
                PlaceBlockSucceeded::decode(payload)?,
            )),
            (State::Play, 21) => Ok(ServerPacket::CraftingState(CraftingState::decode(payload)?)),
            (State::Play, 22) => Ok(ServerPacket::HostileSpawn(HostileSpawn::decode(payload)?)),
            (State::Play, 23) => Ok(ServerPacket::HostileState(HostileState::decode(payload)?)),
            (State::Play, 24) => Ok(ServerPacket::HostileDespawn(HostileDespawn::decode(
                payload,
            )?)),
            (State::Play, 25) => Ok(ServerPacket::CombatHit(CombatHit::decode(payload)?)),
            (State::Play, 26) => Ok(ServerPacket::PassiveSpawn(PassiveSpawn::decode(payload)?)),
            (State::Play, 27) => Ok(ServerPacket::PassiveState(PassiveState::decode(payload)?)),
            (State::Play, 28) => Ok(ServerPacket::PassiveDespawn(PassiveDespawn::decode(
                payload,
            )?)),
            (State::Play, 29) => Ok(ServerPacket::ProjectileSpawn(ProjectileSpawn::decode(
                payload,
            )?)),
            (State::Play, 30) => Ok(ServerPacket::ProjectileState(ProjectileState::decode(
                payload,
            )?)),
            (State::Play, 31) => Ok(ServerPacket::ProjectileDespawn(ProjectileDespawn::decode(
                payload,
            )?)),
            _ => Err(ProtocolError::UnknownPacket),
        }
    }

    /// Encodes one typed server packet into the caller's buffer and returns
    /// the bytes written.
    ///
    /// The key is obtained from the variant first, so an arm can never publish
    /// a record under a direction the variant does not name. The snapshot arm
    /// delegates to this codec's owned compressor, because that family's
    /// payload length is not precomputed; every other arm delegates to the
    /// concrete `encode_into`, which revalidates, sizes with checked
    /// arithmetic, tests the destination and publishes only `dst[..written]`.
    /// No state argument exists on this path, so a record cannot be encoded
    /// under a state it was not decoded under.
    pub fn encode_server_into(
        &mut self,
        packet: &ServerPacket,
        dst: &mut [u8],
    ) -> Result<usize, ProtocolError> {
        let key = packet.key();
        debug_assert_eq!(
            key.direction,
            Direction::ServerToClient,
            "every server variant publishes under the server key space"
        );
        match packet {
            ServerPacket::ChunkSnapshot(record) => self.encode_snapshot_into(record, dst),
            ServerPacket::ServerHello(record) => record.encode_into(dst),
            ServerPacket::HandshakeReject(record) => record.encode_into(dst),
            ServerPacket::LoginSuccess(record) => record.encode_into(dst),
            ServerPacket::LoginReject(record) => record.encode_into(dst),
            ServerPacket::BlockChanges(record) => record.encode_into(dst),
            ServerPacket::ForgetChunks(record) => record.encode_into(dst),
            ServerPacket::PlayerState(record) => record.encode_into(dst),
            ServerPacket::CommandRejected(record) => record.encode_into(dst),
            ServerPacket::KeepAlive(record) => record.encode_into(dst),
            ServerPacket::Disconnect(record) => record.encode_into(dst),
            ServerPacket::RemotePlayerSpawn(record) => record.encode_into(dst),
            ServerPacket::RemotePlayerDespawn(record) => record.encode_into(dst),
            ServerPacket::RemotePlayerStates(record) => record.encode_into(dst),
            ServerPacket::InventoryState(record) => record.encode_into(dst),
            ServerPacket::ItemDropUpserts(record) => record.encode_into(dst),
            ServerPacket::ItemDropRemoves(record) => record.encode_into(dst),
            ServerPacket::FurnaceState(record) => record.encode_into(dst),
            ServerPacket::ContainerClosed(record) => record.encode_into(dst),
            ServerPacket::ChestState(record) => record.encode_into(dst),
            ServerPacket::ChatEvent(record) => record.encode_into(dst),
            ServerPacket::CompanionSpawn(record) => record.encode_into(dst),
            ServerPacket::CompanionStates(record) => record.encode_into(dst),
            ServerPacket::CompanionDespawn(record) => record.encode_into(dst),
            ServerPacket::PlaceBlockSucceeded(record) => record.encode_into(dst),
            ServerPacket::CraftingState(record) => record.encode_into(dst),
            ServerPacket::HostileSpawn(record) => record.encode_into(dst),
            ServerPacket::HostileState(record) => record.encode_into(dst),
            ServerPacket::HostileDespawn(record) => record.encode_into(dst),
            ServerPacket::CombatHit(record) => record.encode_into(dst),
            ServerPacket::PassiveSpawn(record) => record.encode_into(dst),
            ServerPacket::PassiveState(record) => record.encode_into(dst),
            ServerPacket::PassiveDespawn(record) => record.encode_into(dst),
            ServerPacket::ProjectileSpawn(record) => record.encode_into(dst),
            ServerPacket::ProjectileState(record) => record.encode_into(dst),
            ServerPacket::ProjectileDespawn(record) => record.encode_into(dst),
        }
    }
}
