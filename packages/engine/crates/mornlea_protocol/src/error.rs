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
}
