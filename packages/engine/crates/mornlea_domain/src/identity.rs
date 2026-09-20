/// Failures that reject a domain record before it is published.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DomainError {
    IncompleteIdentity,
    InvalidDimension,
    InvalidHotbarSlot,
    InvalidIdentity,
    InvalidText,
    NonFiniteRotation,
    UnknownId,
}

/// Reports whether the 16 bytes are a non-zero UUIDv4 in wire order.
///
/// The rule is the Go `core.PlayerID.Valid` gate: a zero value, a version
/// nibble other than `4`, or a variant outside RFC-4122 is rejected. Every
/// identity newtype in this crate funnels through it so the player and
/// companion rules cannot drift apart.
fn is_uuid_v4(bytes: [u8; 16]) -> bool {
    bytes != [0; 16] && bytes[6] >> 4 == 4 && bytes[8] & 0xc0 == 0x80
}

/// Stable UUIDv4 player identity.
///
/// The wire layout is the standard big-endian UUID byte order, and the
/// identity is a value type: cloning copies the bytes and the type stays
/// distinct from a companion identity so a record cannot name the wrong
/// subject.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct PlayerId([u8; 16]);

impl PlayerId {
    /// Wraps wire bytes, rejecting a zero, non-v4, or non-RFC-4122 variant.
    pub fn try_from_bytes(bytes: [u8; 16]) -> Result<Self, DomainError> {
        if !is_uuid_v4(bytes) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(bytes))
    }

    pub fn bytes(self) -> [u8; 16] {
        self.0
    }
}

/// Stable UUIDv4 companion identity.
///
/// The byte rule is identical to `PlayerId`, but the type is separate because
/// a companion names a different subject with its own ordering space. This
/// crate publishes no absent identity: a wire-level "no companion" form is a
/// protocol concern and stays out of the domain value set.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct CompanionId([u8; 16]);

impl CompanionId {
    /// Wraps wire bytes, rejecting a zero, non-v4, or non-RFC-4122 variant.
    pub fn try_from_bytes(bytes: [u8; 16]) -> Result<Self, DomainError> {
        if !is_uuid_v4(bytes) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(bytes))
    }

    pub fn bytes(self) -> [u8; 16] {
        self.0
    }
}

/// Reports whether a scalar entity identifier is publishable: zero is the
/// absent form in every entity family, so it is rejected here instead of
/// being interpreted downstream.
fn is_nonzero_entity_id(id: u64) -> bool {
    id != 0
}

/// Nonzero authoritative night-walker identity.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct HostileId(u64);

impl HostileId {
    pub fn try_new(id: u64) -> Result<Self, DomainError> {
        if !is_nonzero_entity_id(id) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(id))
    }

    pub fn get(self) -> u64 {
        self.0
    }
}

/// Nonzero authoritative passive-mob identity.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct PassiveId(u64);

impl PassiveId {
    pub fn try_new(id: u64) -> Result<Self, DomainError> {
        if !is_nonzero_entity_id(id) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(id))
    }

    pub fn get(self) -> u64 {
        self.0
    }
}

/// Nonzero authoritative projectile identity.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct ProjectileId(u64);

impl ProjectileId {
    pub fn try_new(id: u64) -> Result<Self, DomainError> {
        if !is_nonzero_entity_id(id) {
            return Err(DomainError::InvalidIdentity);
        }
        Ok(Self(id))
    }

    pub fn get(self) -> u64 {
        self.0
    }
}

/// Current supported contract versions shared by replay and inventory identity.
///
/// Values must stay aligned with `testdata/runtime-migration/contracts.json`.
/// `current_identities_match_frozen_inventory` fails if either side drifts.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Identities {
    pub protocol: u32,
    pub chunk_schema: u32,
    pub player_schema: u32,
    pub world_metadata: u32,
    pub companions_ai_schema: u32,
    pub hostile_mobs_schema: u32,
    pub passive_mobs_schema: u32,
    pub engine_abi: u32,
    pub region_format: u32,
    pub agent_http: String,
    pub agent_mcp: String,
}

impl Identities {
    pub fn current() -> Self {
        Self {
            protocol: 45,
            chunk_schema: 9,
            player_schema: 9,
            world_metadata: 6,
            companions_ai_schema: 5,
            hostile_mobs_schema: 2,
            passive_mobs_schema: 1,
            engine_abi: 11,
            region_format: 1,
            agent_http: "v1".into(),
            agent_mcp: "v1".into(),
        }
    }

    /// Rejects a zero version or empty agent contract label so incomplete
    /// evidence cannot be counted as agreement.
    pub fn validate(&self) -> Result<(), DomainError> {
        if self.protocol == 0
            || self.chunk_schema == 0
            || self.player_schema == 0
            || self.world_metadata == 0
            || self.companions_ai_schema == 0
            || self.hostile_mobs_schema == 0
            || self.passive_mobs_schema == 0
            || self.engine_abi == 0
            || self.region_format == 0
            || self.agent_http.trim().is_empty()
            || self.agent_mcp.trim().is_empty()
        {
            return Err(DomainError::IncompleteIdentity);
        }
        Ok(())
    }
}

/// Replay identity consumed by later offline differential comparison.
///
/// Construction fails closed on a missing source revision, corpus digest, or
/// incomplete contract version set. The constructor does not write files.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ReplayIdentity {
    pub source_revision: String,
    pub corpus_digest: String,
    pub identities: Identities,
    pub seed: u64,
}

impl ReplayIdentity {
    pub fn new(
        source_revision: impl Into<String>,
        corpus_digest: impl Into<String>,
        identities: Identities,
        seed: u64,
    ) -> Result<Self, DomainError> {
        let source_revision = source_revision.into();
        let corpus_digest = corpus_digest.into();
        if source_revision.trim().is_empty() || corpus_digest.trim().is_empty() {
            return Err(DomainError::IncompleteIdentity);
        }
        identities.validate()?;
        Ok(Self {
            source_revision,
            corpus_digest,
            identities,
            seed,
        })
    }
}
