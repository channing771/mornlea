/// Failures that reject a framed record before it is published.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ProtocolError {
    EmptyFrame,
    FrameTooLarge,
    Truncated,
    NonCanonicalUvarint,
    InvalidUvarint,
    UnsupportedVersion,
    TrailingBytes,
    InvalidEnum,
    InvalidString,
    InvalidIdentity,
    InvalidRange,
    InvalidFloat,
    /// A packet ID or registry key this crate does not publish.
    UnknownPacket,
    /// The caller's destination buffer cannot hold the encoded packet.
    OutputTooSmall {
        /// Bytes the validated packet needs.
        needed: usize,
        /// Bytes the caller supplied.
        available: usize,
    },
    /// A checked length computation or bounded reservation failed, so no
    /// record can be sized or published.
    Allocation,
}
