//! CRC-32C (Castagnoli) used by every save envelope.
//!
//! The Go storage codecs hash with `crc32.MakeTable(crc32.Castagnoli)`, which
//! is the reflected CRC-32C polynomial with init and final XOR of all ones.
//! Save files are compared byte-for-byte, so this table must produce the same
//! value as the Go side for the same input.

const CRC32C_POLY: u32 = 0x82F6_3B78;

const fn crc32c_table() -> [u32; 256] {
    let mut table = [0u32; 256];
    let mut index = 0usize;
    while index < table.len() {
        let mut crc = index as u32;
        let mut bit = 0;
        while bit < 8 {
            crc = if crc & 1 != 0 {
                (crc >> 1) ^ CRC32C_POLY
            } else {
                crc >> 1
            };
            bit += 1;
        }
        table[index] = crc;
        index += 1;
    }
    table
}

static CRC32C_TABLE: [u32; 256] = crc32c_table();

/// Computes the CRC-32C checksum of `data`.
///
/// This is the shared envelope checksum for every `save.*` family; it is
/// public so callers and contract tests can reseal a mutated fixture without
/// reimplementing the polynomial.
pub fn crc32c(data: &[u8]) -> u32 {
    crc32c_join(&[data])
}

/// Computes the CRC-32C checksum over the concatenation of `parts` without
/// materializing it. Save envelopes hash a header slice plus the payload.
pub fn crc32c_join(parts: &[&[u8]]) -> u32 {
    let mut crc = 0xFFFF_FFFFu32;
    for part in parts {
        for &byte in *part {
            let index = ((crc ^ u32::from(byte)) & 0xFF) as usize;
            crc = CRC32C_TABLE[index] ^ (crc >> 8);
        }
    }
    crc ^ 0xFFFF_FFFF
}

#[cfg(test)]
mod tests {
    use super::{crc32c, crc32c_join};

    #[test]
    fn crc32c_matches_reference_vectors() {
        // Reference values from the CRC-32C ("iSCSI") catalogue.
        assert_eq!(crc32c(b""), 0x0000_0000);
        assert_eq!(crc32c(b"a"), 0xC1D0_4330);
        assert_eq!(crc32c(b"123456789"), 0xE306_9283);
        assert_eq!(crc32c(b"\x00\x00\x00\x00"), 0x4867_4BC7);
    }

    #[test]
    fn crc32c_join_matches_single_slice() {
        let joined = crc32c_join(&[b"mornlea", b"-storage"]);
        assert_eq!(joined, crc32c(b"mornlea-storage"));
    }
}
