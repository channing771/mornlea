//! Caller-owned framing boundary for `mornlea_protocol`.
//!
//! This suite pins the borrowed framing primitives the packet encoders build
//! on: `read_frame_ref` borrows one bounded frame out of the caller's buffer
//! without allocating, and `write_frame_into` publishes a complete frame into
//! a caller-owned slice, leaving a short destination byte-for-byte unchanged.
//! The allocating `read_frame` / `write_frame` wrappers stay the compatibility
//! surface and must keep delegating to these paths, so the frame behaviour in
//! `tests/runtime_contract.rs` and `tests/protocol_corpus.rs` remains the same
//! contract seen from a different entry point.
//!
//! The boundary rules mirror the Go compatibility sources
//! `packages/shared/network/codec/frame.go` and
//! `packages/shared/network/codec/codec_primitives.go`: a canonical uvarint
//! length prefix that excludes itself, a frame body inside
//! `MAX_FRAME_BYTES`, and a packet ID that is itself a canonical uvarint.

use mornlea_protocol::{
    MAX_FRAME_BYTES, ProtocolError, decode_uvarint, encode_uvarint, read_frame, read_frame_ref,
    write_frame, write_frame_into,
};

/// A frame whose body is a two-byte packet ID varint and a one-byte payload,
/// followed by trailing bytes the first record must not consume.
const COALESCED_HEAD: [u8; 6] = [2, 0, 45, 2, 1, 46];

#[test]
fn frame_ref_borrows_the_payload_from_the_caller_buffer() {
    let frame = read_frame_ref(&COALESCED_HEAD).expect("borrowed frame");
    assert_eq!(frame.packet_id, 0);
    assert_eq!(frame.payload, [45]);
    assert_eq!(frame.consumed, 3);
    assert!(
        std::ptr::eq(frame.payload.as_ptr(), COALESCED_HEAD[2..].as_ptr()),
        "payload must alias the caller buffer at the original offset"
    );
    assert_eq!(
        &COALESCED_HEAD[frame.consumed..],
        [2, 1, 46],
        "the first record must consume exactly its own bytes"
    );
}

#[test]
fn frame_ref_accepts_a_packet_id_without_a_payload() {
    let frame = read_frame_ref(&[1, 7]).expect("packet id only frame");
    assert_eq!(frame.packet_id, 7);
    assert!(frame.payload.is_empty(), "payload must stay empty");
    assert_eq!(frame.consumed, 2);
}

#[test]
fn frame_ref_reads_the_largest_canonical_packet_id() {
    let mut wire = vec![5u8];
    wire.extend_from_slice(&[0xff, 0xff, 0xff, 0xff, 0x0f]);
    let frame = read_frame_ref(&wire).expect("five byte id varint");
    assert_eq!(frame.packet_id, u32::MAX);
    assert!(frame.payload.is_empty());
    assert_eq!(frame.consumed, 6);

    let mut with_payload = vec![6u8];
    with_payload.extend_from_slice(&[0xff, 0xff, 0xff, 0xff, 0x0f, 0x2a]);
    let frame = read_frame_ref(&with_payload).expect("five byte id varint with payload");
    assert_eq!(frame.packet_id, u32::MAX);
    assert_eq!(frame.payload, [0x2a]);
}

#[test]
fn frame_ref_rejects_malformed_prefixes_and_bodies() {
    for (wire, expected) in [
        (&[0u8][..], ProtocolError::EmptyFrame),
        (&[0x81, 0x00], ProtocolError::NonCanonicalUvarint),
        (&[0x80], ProtocolError::Truncated),
        (
            &[0x80, 0x80, 0x80, 0x80, 0x80, 0],
            ProtocolError::InvalidUvarint,
        ),
        (
            &[0xff, 0xff, 0xff, 0xff, 0x1f],
            ProtocolError::InvalidUvarint,
        ),
        (
            &[0xff, 0xff, 0xff, 0xff, 0x0f],
            ProtocolError::FrameTooLarge,
        ),
        (&[0x81, 0x80, 0x80, 0x01], ProtocolError::FrameTooLarge),
        (&[3, 0, 0], ProtocolError::Truncated),
        (&[1, 0x80], ProtocolError::Truncated),
        (&[2, 0x81, 0x00], ProtocolError::NonCanonicalUvarint),
        (&[2, 0xff, 0xff, 0xff, 0xff, 0x1f], ProtocolError::Truncated),
        (
            &[7, 0xff, 0xff, 0xff, 0xff, 0x7f, 0, 0],
            ProtocolError::InvalidUvarint,
        ),
    ] {
        let err = read_frame_ref(wire)
            .err()
            .unwrap_or_else(|| panic!("accepted malformed frame {wire:?}"));
        assert_eq!(err, expected, "wrong rejection for {wire:?}");
    }
}

#[test]
fn frame_ref_admits_the_maximum_body_and_rejects_one_more_byte() {
    let maximum = frame_with_body(MAX_FRAME_BYTES as usize);
    let admitted = read_frame_ref(&maximum).expect("maximum body length admits");
    assert_eq!(admitted.packet_id, 0);
    // A two megabyte body needs a four-byte canonical length prefix, so the
    // whole frame is the prefix plus the body.
    assert_eq!(admitted.consumed, 4 + MAX_FRAME_BYTES as usize);
    assert_eq!(admitted.payload.len(), MAX_FRAME_BYTES as usize - 1);

    let oversized = frame_with_body(MAX_FRAME_BYTES as usize + 1);
    assert_eq!(
        read_frame_ref(&oversized).err(),
        Some(ProtocolError::FrameTooLarge),
        "a body one byte above the ceiling must be refused"
    );

    // A two-byte ID varint spends one payload byte, so the same ceiling is
    // reached one byte earlier.
    let admitted_two_byte_id = frame_with_id_body(128, MAX_FRAME_BYTES as usize - 2);
    let frame = read_frame_ref(&admitted_two_byte_id).expect("maximum body with two byte id");
    assert_eq!(frame.packet_id, 128);
    assert_eq!(
        read_frame_ref(&frame_with_id_body(128, MAX_FRAME_BYTES as usize - 1)).err(),
        Some(ProtocolError::FrameTooLarge),
        "a two byte id plus one payload byte above the ceiling must be refused"
    );
}

#[test]
fn write_frame_into_leaves_a_short_destination_unchanged() {
    let mut dst = [0xa5u8; 5];
    let err = write_frame_into(128, &[1, 2, 3], &mut dst).expect_err("short destination");
    assert_eq!(
        err,
        ProtocolError::OutputTooSmall {
            needed: 6,
            available: 5
        }
    );
    assert_eq!(dst, [0xa5; 5], "a capacity refusal must not touch a byte");
}

#[test]
fn write_frame_into_reports_an_invalid_size_before_capacity() {
    let mut dst = [0x5au8; 4];
    let oversized = vec![0u8; MAX_FRAME_BYTES as usize];
    assert_eq!(
        write_frame_into(0, &oversized, &mut dst).err(),
        Some(ProtocolError::FrameTooLarge),
        "an invalid frame size must win over a short destination"
    );
    assert_eq!(dst, [0x5a; 4], "an invalid size must not touch a byte");
}

#[test]
fn write_frame_into_publishes_a_canonical_frame_into_an_exact_window() {
    let mut dst = [0u8; 6];
    let written = write_frame_into(128, &[1, 2, 3], &mut dst).expect("exact destination");
    assert_eq!(written, 6);
    assert_eq!(dst, [5, 0x80, 0x01, 1, 2, 3]);

    // A larger destination is written only in the exact frame prefix, so the
    // caller's remaining bytes stay under the caller's control.
    let mut roomy = [0xa5u8; 9];
    let written = write_frame_into(128, &[1, 2, 3], &mut roomy).expect("roomy destination");
    assert_eq!(written, 6);
    assert_eq!(&roomy[..6], &[5, 0x80, 0x01, 1, 2, 3]);
    assert_eq!(
        &roomy[6..],
        [0xa5; 3],
        "bytes past the frame must be untouched"
    );

    let frame = read_frame_ref(&roomy[..written]).expect("read back the published frame");
    assert_eq!(frame.packet_id, 128);
    assert_eq!(frame.payload, [1, 2, 3]);
    assert_eq!(frame.consumed, 6);

    let mut empty = [0u8; 2];
    assert_eq!(
        write_frame_into(7, &[], &mut empty).expect("empty payload"),
        2
    );
    assert_eq!(empty, [1, 7]);
    let frame = read_frame_ref(&empty).expect("read back the id only frame");
    assert_eq!(frame.packet_id, 7);
    assert!(frame.payload.is_empty());
}

#[test]
fn write_frame_into_crosses_every_length_prefix_boundary() {
    for (payload_len, prefix_len) in [
        (126usize, 1usize),
        (127, 2),
        (16_382, 2),
        (16_383, 3),
        (2_097_150, 3),
        (2_097_151, 4),
    ] {
        let payload = vec![0x2au8; payload_len];
        let body = 1 + payload_len;
        let mut dst = vec![0u8; prefix_len + body];
        let written = write_frame_into(0, &payload, &mut dst).expect("bounded frame");
        assert_eq!(
            written,
            prefix_len + body,
            "wrong wire size for {payload_len}"
        );
        let (declared, used) = decode_uvarint(&dst).expect("canonical prefix");
        assert_eq!(declared, u32::try_from(body).expect("body fits u32"));
        assert_eq!(used, prefix_len, "wrong prefix length for {payload_len}");
        let frame = read_frame_ref(&dst).expect("read back the boundary frame");
        assert_eq!(frame.consumed, written);
        assert_eq!(frame.payload.len(), payload_len);
    }
}

#[test]
fn write_frame_into_crosses_every_packet_id_varint_boundary() {
    for (packet_id, id_len) in [
        (0u32, 1usize),
        (127, 1),
        (128, 2),
        (16_383, 2),
        (16_384, 3),
        (2_097_151, 3),
        (2_097_152, 4),
        (268_435_455, 4),
        (268_435_456, 5),
        (u32::MAX, 5),
    ] {
        let mut dst = [0u8; 16];
        let written = write_frame_into(packet_id, &[0x2a], &mut dst).expect("bounded frame");
        assert_eq!(
            written,
            1 + id_len + 1,
            "wrong wire size for id {packet_id}"
        );
        let frame = read_frame_ref(&dst[..written]).expect("read back the boundary frame");
        assert_eq!(frame.packet_id, packet_id);
        assert_eq!(frame.payload, [0x2a]);
    }
}

#[test]
fn canonical_uvarint_lengths_and_malformed_vectors_are_pinned() {
    for (value, len) in [
        (0u32, 1usize),
        (127, 1),
        (128, 2),
        (16_383, 2),
        (16_384, 3),
        (2_097_151, 3),
        (2_097_152, 4),
        (268_435_455, 4),
        (268_435_456, 5),
        (u32::MAX, 5),
    ] {
        assert_eq!(
            encode_uvarint(value).len(),
            len,
            "wrong canonical length for {value}"
        );
        let (decoded, used) = decode_uvarint(&encode_uvarint(value)).expect("canonical round trip");
        assert_eq!(decoded, value);
        assert_eq!(used, len);
    }

    for wire in [
        &[0x80u8][..],
        &[0x81, 0x00],
        &[0x80, 0x80, 0x80, 0x80, 0x00],
        &[0xff, 0xff, 0xff, 0xff, 0x7f],
        &[0xff, 0xff, 0xff, 0xff, 0x10],
        &[0x80, 0x80, 0x80, 0x80, 0x80, 0x00],
    ] {
        assert!(
            decode_uvarint(wire).is_err(),
            "accepted malformed uvarint {wire:?}"
        );
    }
    assert_eq!(
        decode_uvarint(&[0xff, 0xff, 0xff, 0xff, 0x0f]).expect("u32 max"),
        (u32::MAX, 5)
    );
}

#[test]
fn allocating_wrappers_match_the_caller_owned_paths() {
    let wire = write_frame(128, &[1, 2, 3]).expect("allocating write");
    assert_eq!(wire, [5, 0x80, 0x01, 1, 2, 3]);
    let mut caller = [0u8; 6];
    assert_eq!(
        write_frame_into(128, &[1, 2, 3], &mut caller).expect("caller write"),
        wire.len()
    );
    assert_eq!(
        &caller[..],
        wire.as_slice(),
        "wrapper and caller path disagree"
    );

    let (id, payload, used) = read_frame(&wire).expect("allocating read");
    let frame = read_frame_ref(&wire).expect("borrowed read");
    assert_eq!(id, frame.packet_id);
    assert_eq!(payload, frame.payload);
    assert_eq!(used, frame.consumed);

    let mut coalesced = write_frame(3, &[1, 2, 3]).expect("first frame");
    coalesced.extend_from_slice(&write_frame(4, &[5, 6]).expect("second frame"));
    let first = read_frame_ref(&coalesced).expect("first record");
    assert_eq!(
        (first.packet_id, first.payload, first.consumed),
        (3, &[1, 2, 3][..], 5)
    );
    let second = read_frame_ref(&coalesced[first.consumed..]).expect("second record");
    assert_eq!((second.packet_id, second.payload), (4, &[5, 6][..]));
    assert_eq!(first.consumed + second.consumed, coalesced.len());
}

thread_local! {
    static ALLOCATIONS: std::cell::Cell<usize> = const { std::cell::Cell::new(0) };
}

/// Counting allocator installed for this test binary.
///
/// Rust has no equivalent of Go's `testing.AllocsPerRun`, so a per-thread
/// counter layered over the system allocator is the only way to observe the
/// allocation behaviour of a caller-owned framing path. The counter is
/// thread-local rather than global on purpose: cargo runs the cases in this
/// binary in parallel, and a global counter would be polluted by the other
/// cases. The TLS is `const` initialized, so reading it inside the allocator
/// performs no lazy allocation and registers no destructor.
struct CountingAllocator;

fn allocation_count() -> usize {
    ALLOCATIONS.with(|counter| counter.get())
}

fn bump() {
    // `try_with` skips the count once a thread's TLS is gone during teardown.
    let _ = ALLOCATIONS.try_with(|counter| counter.set(counter.get() + 1));
}

// SAFETY: the counter only observes the system allocator; pointer semantics and
// alignment requirements are forwarded unchanged.
unsafe impl std::alloc::GlobalAlloc for CountingAllocator {
    unsafe fn alloc(&self, layout: std::alloc::Layout) -> *mut u8 {
        bump();
        // SAFETY: the layout is guaranteed valid by the caller of the allocator.
        unsafe { std::alloc::System.alloc(layout) }
    }

    unsafe fn dealloc(&self, ptr: *mut u8, layout: std::alloc::Layout) {
        // SAFETY: ptr and layout come from this allocator by contract.
        unsafe { std::alloc::System.dealloc(ptr, layout) }
    }

    unsafe fn alloc_zeroed(&self, layout: std::alloc::Layout) -> *mut u8 {
        bump();
        // SAFETY: same contract as `alloc`.
        unsafe { std::alloc::System.alloc_zeroed(layout) }
    }

    unsafe fn realloc(&self, ptr: *mut u8, layout: std::alloc::Layout, new: usize) -> *mut u8 {
        bump();
        // SAFETY: same contract as `dealloc`, with a caller-checked new size.
        unsafe { std::alloc::System.realloc(ptr, layout, new) }
    }
}

#[global_allocator]
static COUNTING_ALLOCATOR: CountingAllocator = CountingAllocator;

#[test]
fn borrowed_frame_read_does_not_allocate() {
    // Warm up so a first-call lazy setup is never mistaken for a per-read cost.
    for _ in 0..8 {
        read_frame_ref(&COALESCED_HEAD).expect("warm up read");
    }

    let before = allocation_count();
    let mut payload_bytes = 0usize;
    for _ in 0..256 {
        let frame = read_frame_ref(&COALESCED_HEAD).expect("borrowed read");
        assert_eq!(frame.consumed, 3);
        payload_bytes += frame.payload.len();
    }
    let borrowed = allocation_count() - before;
    assert_eq!(
        payload_bytes, 256,
        "the borrowed payload must stay readable"
    );
    assert_eq!(
        borrowed, 0,
        "the borrowed frame read allocated {borrowed} times after warm-up"
    );

    // The contrast keeps the zero-allocation claim honest: the allocating
    // wrapper still publishes an owned payload, so the counter must observe it.
    let before = allocation_count();
    let (id, payload, used) = read_frame(&COALESCED_HEAD).expect("owning read");
    let owned = allocation_count() - before;
    assert!(owned >= 1, "the owning wrapper must allocate, saw {owned}");
    assert_eq!((id, payload.as_slice(), used), (0, &[45][..], 3));
}

/// Builds a frame whose body length is exactly `body`, using a one-byte packet
/// ID varint and the remaining bytes as payload.
fn frame_with_body(body: usize) -> Vec<u8> {
    frame_with_id_body(0, body - 1)
}

/// Builds a frame with the given packet ID varint and `payload_len` payload
/// bytes, so the body length is the ID varint plus the payload.
fn frame_with_id_body(packet_id: u32, payload_len: usize) -> Vec<u8> {
    let id = encode_uvarint(packet_id);
    let body = id.len() + payload_len;
    let mut wire = encode_uvarint(u32::try_from(body).expect("body fits u32"));
    wire.extend_from_slice(&id);
    wire.resize(wire.len() + payload_len, 0x2a);
    wire
}
