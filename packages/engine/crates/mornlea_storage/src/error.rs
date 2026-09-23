/// Failures that reject a versioned save record.
///
/// `Corrupt` and `FutureVersion` mirror the storage sentinels the Go codecs
/// report today: both reject the record, neither repairs it. `FutureVersion`
/// stays separate so a caller can tell "this file is newer than the reader"
/// apart from "these bytes violate the on-disk contract".
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum StorageError {
    /// The bytes violate the current format contract. The record is rejected
    /// as-is; no field is guessed, repaired, or dropped.
    Corrupt(String),
    /// The record declares a version newer than the reader supports.
    FutureVersion(String),
    /// The caller's output buffer cannot hold the complete canonical record.
    /// Encoding leaves the buffer unchanged in this case.
    OutputTooSmall { needed: usize, available: usize },
}

impl std::fmt::Display for StorageError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Corrupt(detail) => write!(f, "save record is corrupt: {detail}"),
            Self::FutureVersion(detail) => write!(f, "save record is a future version: {detail}"),
            Self::OutputTooSmall { needed, available } => write!(
                f,
                "save output buffer is too small: need {needed} bytes, have {available}"
            ),
        }
    }
}

impl std::error::Error for StorageError {}

/// Convenience alias for save-codec results.
pub type StorageResult<T> = Result<T, StorageError>;

/// Builds a [`StorageError::Corrupt`] with a stable field-scoped detail.
pub(crate) fn corrupt(field: &str, detail: impl std::fmt::Display) -> StorageError {
    StorageError::Corrupt(format!("{field}: {detail}"))
}

/// Builds a [`StorageError::FutureVersion`] for a version above the reader.
pub(crate) fn future_version(field: &str, version: u32) -> StorageError {
    StorageError::FutureVersion(format!("{field}: {version}"))
}
