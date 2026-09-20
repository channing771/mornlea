/// Failures that reject a domain record before it is published.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DomainError {
    IncompleteIdentity,
    InvalidDimension,
    InvalidHotbarSlot,
    NonFiniteRotation,
    UnknownId,
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
