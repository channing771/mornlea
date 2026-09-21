# Protocol family implementation plan

**Goal:** all 59 packets and framing have callable, bounded, fallible, Go-compatible adapters. **Architecture:** retain versioned DTOs at the wire boundary; convert current shared values into domain; use caller buffers and an exclusively owned compression context. **Tech stack:** Rust std + existing zstd; independent Go codec corpus. **Spec:** coverage, admission, invalid publication, bounded work. **Global constraints:** `../execution-contract.md` and every field/rule in `../packet-contracts.md`. **Review focus:** public-field mutation, real-container validation, inbound/outbound asymmetry, exact counted limits and no destination writes before success.

`PROTOCOL` is `packages/engine/crates/mornlea_protocol`; `ORACLE` and `CORPUS` are as defined in 01-evidence. Every packet row below is **one task**, not a batch instruction. Controller integrates lib.rs, registry.rs, test module registrations and manifest fragments serially. QuickMoveStack/DropStack/MoveStackPartial share one existing source file and must execute in row order. No worker owns all remaining encoders.

<a id="node-3-1"></a>

## 3.1 — Primitive caller-buffer and framing boundary

Dependencies: 1.1. Own PROTOCOL `src/{bytes,varint,frame,batch,error}.rs`, tests `runtime_contract/framing_buffers.rs`, new `tests/allocation_contract.rs` with a single-thread allocation counter. Keep existing tests; add topic modules rather than another copy of all golden tests.

Implement exact FrameRef/write_frame_into/encoded_len contracts from execution-contract. Canonical u32 varint lengths are1 for0..127,2 for128..16383,3 for16384..2097151,4 for2097152..268435455,5 otherwise; fifth byte<=15 and no redundant zero groups. Frame body length includes ID varint, max2097152, min1. Borrow only after proving prefix/body boundaries. Return consumed bytes for the first frame, leave coalesced successor bytes unread. Integer size arithmetic is checked. Fixed-array decoder fills a fixed `[ItemStack::EMPTY;N]` or primitive stack array using indexed checked reads, with no intermediate Vec or unstable array API. Domain ItemStack is Copy. No toolchain upgrade.

`publish_packet` restricts writes to a validated exact-length prefix. `SliceWriter` has private `put_u8/i8/u16/u32/i32/u64/f32/bool/bytes/varint` operations and a cursor; writes do not perform fallible value validation. Their exact sizes are constants except bytes/varint. Callers run complete semantic validation and checked size calculation before writer creation. All branch paths must be covered by size-vs-produced-length tests so user data cannot reach an indexing panic. No public unchecked writer.

Red cases: `[2,0,45]` borrows payload offset2 with no allocation; concatenate it twice and consumed=3; empty frame, prefix6 bytes, ID overflow, noncanonical prefix and max+1 reject; exactly max body accepted; short destination remains0xa5. Varint writes touch only their prefix. Precedence invalid field→size→capacity. Register allocation tests, but do not claim packet paths zero-allocation before3.16/3.47. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked framing_buffers`; same command `--test allocation_contract`. Commit `refactor(protocol): add checked caller-buffer framing`.

<a id="node-3-2"></a>

## 3.2 — Shared-value integration without raw format loss

Dependencies: 2.3, 2.8, 3.1. Own PROTOCOL `src/{player_id,entity_id,item_stack,drop_id,container_ref,block,chunk_snapshot}.rs` and mechanical imports/getter call sites, tests `runtime_contract/shared_values.rs`. Reexport domain PlayerId/CompanionId/ItemStack/DropId and item tables. Keep raw `ContainerRef` DTO with all six wire fields including **i32** dimension; convert to `Option<domain::ContainerRef>` only after exact all-zero detection or full validation. Keep SectionData as wire DTO for Y/storage tags; checked conversion borrows/moves the compact domain section without expanding/repacking. No duplicate item tables or unchecked CompanionId::NONE remain. Chat raw absent companion becomes `Option<CompanionId>` (raw zero only in permitted branches). Mechanical encode call updates do not opportunistically redesign packets.

Tests: protocol and storage ordinary stacks use the same maxima; raw absent container exact zeros; dimension256 must not narrow to0; companion zero is representable only by None in Chat DTO; indexed section retains exact palette order/words. Run full protocol runtime_contract and domain runtime_contract; node uses only current tests plus these value cases and does not need packet completion. Commit `refactor(protocol): consume validated shared values`.

<a id="node-3-3"></a>

## 3.3 — Structural login and pure admission

Dependencies: 2.2, 3.1, 1.5. Own PROTOCOL `src/{client_hello,login_start,admission}.rs`, exports, tests `runtime_contract/admission.rs`; new package-local `packages/server/server/login_admission_oracle_test.go` using existing login-driver test setup. No network listener: use the established in-memory fake transport. This Go producer runs actual login-driver policy with structurally valid requests and records rejection category/normalized identity; it is not a reimplementation of admission.

Use exact InboundHello/InboundLoginStart/AdmittedLogin signatures and error order from execution-contract. Structural login validates bytes/UTF-8/full-consumption and the65536 total payload limit only. Admission validates UUID, then trims Go-defined whitespace and validates canonical name, then distance2..64. Outbound LoginStart validates UUID, **raw name bytes<=128**, normalized name validity and distance, but encodes the original supplied name including permitted edge spaces. Strict `LoginStart::decode` is an outbound-record convenience; typed inbound dispatch must call decode_inbound. ServerHello remains strict current-version.

Cases: hello44 yields version rejection, not decode failure; UUID0 + distance1 yields InvalidIdentity; valid ID/name + distance1 yields ProtocolViolation;128 leading spaces+Alice inbound admits Alice if total<=65536, outbound rejects;32/33 multibyte scalars; all-control, UTF-8 invalid, trailing bytes, missing distance; distance1/2/64/65. Side-effect checks: no accepted session for rejection and no clamping distance. Run protocol filter `admission`; `go test ./packages/server/server -run '^TestLoginAdmissionOracle' -count=1`. Preserve its existing fixture setup and no live saves. Commit `fix(protocol): separate inbound parsing from login admission`.

<a id="node-3-4"></a>

## 3.4 — Typed direction/state registry closure

Dependencies: all packet nodes3.10–3.68 and3.3. Own PROTOCOL `src/registry.rs`, `src/lib.rs`, tests `runtime_contract/registry.rs`. Define ClientPacket and ServerPacket enums with one variant per packet in the registry table; client hello/login variants contain raw inbound types. `decode_client(state:State,id:u32,payload:&[u8])->Result<ClientPacket,ProtocolError>` matches the complete key. `ProtocolCodec::decode_server(&mut self,state:State,id:u32,payload:&[u8])->Result<ServerPacket,ProtocolError>` dispatches compressed snapshot through its reusable context; `ProtocolCodec::encode_server_into(&mut self,packet:&ServerPacket,dst:&mut[u8])->Result<usize,ProtocolError>` dispatches bounded encoders. Each enum has `key()->PacketKey`; all key collisions across direction/state are legal but duplicate complete keys are not. Fixed packets remain inline enum variants; no per-dispatch Box allocation. Unknown state, wrong direction and C/Play/1 fail closed.

Register container_closed publicly. Test external import and actual decode/encode for every key using the named corpus seed. Mutate registry by removing S/Play/14 in a test-owned dispatch map, duplicating one complete key and using Play ID0 under Login; require expected typed variant or missing-key failure, not just count comparison. Run protocol filter `registry` and full runtime_contract. Commit `feat(protocol): dispatch packets by direction and state`.

<a id="node-3-5"></a>

## 3.5 — Semantic adapter closure

Dependencies: 2.14,3.4. Own new PROTOCOL `src/semantic.rs`, tests `runtime_contract/semantic.rs`. `pub enum PlayIntent { Sequenced {sequence:u64,command:domain::Command}, Chat(domain::ChatIntent), KeepAliveReply {token:u64} }`; `TryFrom<ClientPacket> for PlayIntent` rejects non-Play control. `TryFrom<ServerPacket> for domain::Event` accepts the30 semantic variants only; consuming conversion moves collections, never clones them. Reverse `TryFrom<domain::Event> for ServerPacket` applies wire-specific batch/size caps. Named matches implement all RejectReason wire mappings; sentinel and chat union conversions follow domain plan. No raw cast of a differently numbered enum.

Test all19+1 intents, keepalive separation, all30 events, all15 reject reasons, absent-container union, chat single text slot, raw dimension rules and source order preservation. Domain values larger than a wire batch fail conversion instead of truncation. Run protocol `semantic`, full domain runtime_contract and `go test ./packages/tools/cmd/runtime-oracle -run '^TestDomainEventsOracle' -count=1`. Commit `feat(protocol): adapt complete semantic commands and events`.

## Packet-node execution contract

Each numbered row owns `PROTOCOL/src/<module>.rs`, `PROTOCOL/tests/runtime_contract/<module>.rs`, `ORACLE/protocol_<module>_test.go`, and `CORPUS/cases/protocol/<PacketName>/`. The three colocated stack families use distinct test modules named after each packet. All depend on1.5,3.1,3.2; ClientHello and LoginStart also depend on3.3. A row may add one private reusable helper only if no new public contract is created; shared helper changes return to the controller. The field list and Go validation are the **entire matching row(s)** of packet-contracts plus the control fields below. Keep existing wire byte order and lengths; do not infer order from domain struct layout.

Exact steps for every row:

1. Add Go test `TestProtocolOracle<PacketName>` constructing the seed below, invoke actual codec encode/decode for its state/direction, and emit `<family>/45/valid`, `<family>/45/<negative-label>` cases. Family IDs are existing `protocol.client.<PacketName>` or `protocol.server.<PacketName>`. Commit reviewed generated bytes and normalized fields through the controller. Byte-decode cases mutate the encoded seed, not the Rust expected result.
2. Add Rust topic test `<snake>_corpus` loading that exact valid case and each listed negative case. Decode/compare every field and re-encode for noncompressed exact bytes. For fixed payloads<=256 bytes, check every proper truncation. For larger/variable payloads, check cuts at0,1, each header-field boundary, first/last record-field boundaries and length−1, deduplicated; keep these boundary recipes within the manifest case budget. Add one trailing zero byte. Test bool values2, noncanonical count/string prefixes and oversized counts for every applicable field before allocation.
3. Add the node's invalid constructed-value test before replacing its encoder. When private domain values prevent a construction, mutate the raw packet aggregate or decode bytes; do not add unsafe constructors to make a test convenient. Demonstrate the baseline panic/invalid bytes for reviewed regressions; already-correct boundary cases are retained without inventing a red result.
4. Implement validate, checked encoded_len, encode_into and fallible encode using execution-contract. For an existing packet constructor with more than7 parameters, replace it with `try_from_fields(fields: Self) -> Result<Self,ProtocolError>` that calls validate and returns the value; update that family's test/caller sites together. Smaller constructors may remain but delegate validate. This keeps all fields explicit without a clippy too_many_arguments waiver or a second parameter DTO duplicating the wire struct. Reuse one validator for constructor/decode/encode except explicit inbound admission split. Variable array preflight checks count, checked minimum bytes and available bytes **before reserve**; validate whole input before publish. Fixed arrays decode without intermediate Vec. Strings use byte+scalar bounds from the table. Keep negative-zero float bits and input order.
5. Test destination exactly n, n+3 with untouched suffix, n−1 unchanged, invalid value plus short destination yielding the value error. Independently verify encoded_len equals Go payload length. Run the exact commands under the node; use `-- --list` with its Rust filter and require nonzero tests. Submit case IDs, byte/value comparisons and allocation changes for controller review; commit `fix(protocol): validate <snake> before encoding` (or feat when registering a missing adapter).

Seed abbreviations are fully defined: P1=`00112233-4455-4677-8899-aabbccddeeff`, P2=`10213243-5465-4768-899a-abbccddeeff0`; ZERO3 three +0.0 f32s, ZERO_POS(0,0,0), EMPTY=(0,0,0); F={dimension0,chunk(−1,2),kind furnace0,slot31,generation1}; C={dimension0,chunk(−1,2),kind chest1,slot15,generation1}; NONE is all six raw fields zero; D={dimension−1,chunk(−1,2),slot31,generation1}. Repeated entity records for maximum batches use IDs1..N, or UUID prefix `00112233-4455-4677-8899-aabbccdd` with the final16 bits set big-endian to1..N, preserving UUID v4 bits. Duplicate/order negatives replace/reverse adjacent IDs. No unspecified seed fields are permitted.

Control packets not detailed elsewhere: ClientHello/ServerHello contain u32 protocol_version varint; HandshakeReject has server_protocol_version u32 varint, code u8=1, message UTF-8<=256 bytes/scalars; LoginSuccess has16-byte PlayerId then world_seed u64; LoginReject has code u8=1..7 then message<=256; Disconnect code1..5 then message<=256; empty rejection/disconnect messages are valid. LoginStart raw wire is16 UUID bytes, canonical length varint+raw name bytes, distance u8.

<a id="node-3-10"></a>

## 3.10 — ClientHello (C/Handshake/0)

Dependencies: 1.5, 3.1, 3.2, 3.3. Source module `client_hello.rs`; test/Go producer stem `client_hello`. Consume the shared packet-node steps above and the exact `ClientHello` field/rule row in packet-contracts. Valid seed: `protocol_version=45`.

Required cases: Inbound44 is structurally decoded and rejected by validate_hello with server_version45; outbound44 fails; varint overflow/noncanonical/trailing fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleClientHello$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked client_hello_corpus`.

<a id="node-3-11"></a>

## 3.11 — ServerHello (S/Handshake/0)

Dependencies: 1.5, 3.1, 3.2. Source module `server_hello.rs`; test/Go producer stem `server_hello`. Consume the shared packet-node steps above and the exact `ServerHello` field/rule row in packet-contracts. Valid seed: `protocol_version=45`.

Required cases: 44 fails decode and encode; noncanonical version varint fails. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleServerHello$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked server_hello_corpus`.

<a id="node-3-12"></a>

## 3.12 — HandshakeReject (S/Handshake/1)

Dependencies: 1.5, 3.1, 3.2. Source module `handshake_reject.rs`; test/Go producer stem `handshake_reject`. Consume the shared packet-node steps above and the exact `HandshakeReject` field/rule row in packet-contracts. Valid seed: `server_protocol_version=44,code=1,message="version"`.

Required cases: server version44 is allowed; code0/2 fail; empty message allowed;256/257 ASCII-byte boundary. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleHandshakeReject$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked handshake_reject_corpus`.

<a id="node-3-13"></a>

## 3.13 — LoginStart (C/Login/0)

Dependencies: 1.5, 3.1, 3.2, 3.3. Source module `login_start.rs`; test/Go producer stem `login_start`. Consume the shared packet-node steps above and the exact `LoginStart` field/rule row in packet-contracts. Valid seed: `player_id=P1,display_name=" Alice ",view_distance=2`.

Required cases: Outbound preserves edge spaces if raw length<=128; inbound UUID0,name129 edge-spaces plus Alice,distance1 reaches admission; invalid identity takes precedence over distance;2/64 accepted,1/65 rejected by admission. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleLoginStart$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked login_start_corpus`.

<a id="node-3-14"></a>

## 3.14 — LoginSuccess (S/Login/0)

Dependencies: 1.5, 3.1, 3.2. Source module `login_success.rs`; test/Go producer stem `login_success`. Consume the shared packet-node steps above and the exact `LoginSuccess` field/rule row in packet-contracts. Valid seed: `player_id=P1,world_seed=0`.

Required cases: UUID0/wrong variant rejected; seed=u64::MAX accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleLoginSuccess$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked login_success_corpus`.

<a id="node-3-15"></a>

## 3.15 — LoginReject (S/Login/1)

Dependencies: 1.5, 3.1, 3.2. Source module `login_reject.rs`; test/Go producer stem `login_reject`. Consume the shared packet-node steps above and the exact `LoginReject` field/rule row in packet-contracts. Valid seed: `code=1,message="full"`.

Required cases: all codes1..7 accepted;0/8 fail; message256/257 bytes and invalid UTF-8. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleLoginReject$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked login_reject_corpus`.

<a id="node-3-16"></a>

## 3.16 — PlayerInput (C/Play/0)

Dependencies: 1.5, 3.1, 3.2. Source module `player_input.rs`; test/Go producer stem `player_input`. Consume the shared packet-node steps above and the exact `PlayerInput` field/rule row in packet-contracts. Valid seed: `sequence=1,move_x=1,move_z=-1,jump=true,yaw=0.5,pitch=-0.25,mining=true,eating=true,sprinting=true,sneaking=true`.

Required cases: Mutated yaw NaN returns error without panic; all bool offsets reject2; axes-128/127 and pitch4.0 remain valid; exact23-byte payload. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOraclePlayerInput$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked player_input_corpus`.

<a id="node-3-17"></a>

## 3.17 — PlaceBlock (C/Play/2)

Dependencies: 1.5, 3.1, 3.2. Source module `place_block.rs`; test/Go producer stem `place_block`. Consume the shared packet-node steps above and the exact `PlaceBlock` field/rule row in packet-contracts. Valid seed: `sequence=1,yaw=0.5,pitch=-0.25,slot=8`.

Required cases: slot9 fails; infinite yaw/pitch fail;sequence0 valid. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOraclePlaceBlock$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked place_block_corpus`.

<a id="node-3-18"></a>

## 3.18 — RequestChunkResync (C/Play/3)

Dependencies: 1.5, 3.1, 3.2. Source module `request_chunk_resync.rs`; test/Go producer stem `request_chunk_resync`. Consume the shared packet-node steps above and the exact `RequestChunkResync` field/rule row in packet-contracts. Valid seed: `sequence=1,dimension=1,chunk=(-1,2),have_revision=0`.

Required cases: dimension2/-1 fail;have_revision=u64::MAX and arbitrary chunk i32 allowed. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleRequestChunkResync$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked request_chunk_resync_corpus`.

<a id="node-3-19"></a>

## 3.19 — KeepAliveReply (C/Play/4)

Dependencies: 1.5, 3.1, 3.2. Source module `keep_alive_reply.rs`; test/Go producer stem `keep_alive_reply`. Consume the shared packet-node steps above and the exact `KeepAliveReply` field/rule row in packet-contracts. Valid seed: `token=1`.

Required cases: token0 fails; token=u64::MAX accepted; not an authority command. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleKeepAliveReply$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked keep_alive_reply_corpus`.

<a id="node-3-20"></a>

## 3.20 — SelectHotbar (C/Play/5)

Dependencies: 1.5, 3.1, 3.2. Source module `select_hotbar.rs`; test/Go producer stem `select_hotbar`. Consume the shared packet-node steps above and the exact `SelectHotbar` field/rule row in packet-contracts. Valid seed: `sequence=1,slot=8`.

Required cases: slot9/255 fail;sequence0 allowed. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleSelectHotbar$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked select_hotbar_corpus`.

<a id="node-3-21"></a>

## 3.21 — MoveInventoryStack (C/Play/6)

Dependencies: 1.5, 3.1, 3.2. Source module `move_inventory_stack.rs`; test/Go producer stem `move_inventory_stack`. Consume the shared packet-node steps above and the exact `MoveInventoryStack` field/rule row in packet-contracts. Valid seed: `sequence=1,from=0,to=35`.

Required cases: 35 accepted;36 rejected;from=to rejected. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleMoveInventoryStack$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked move_inventory_stack_corpus`.

<a id="node-3-22"></a>

## 3.22 — MoveCraftingStack (C/Play/7)

Dependencies: 1.5, 3.1, 3.2. Source module `move_crafting_stack.rs`; test/Go producer stem `move_crafting_stack`. Consume the shared packet-node steps above and the exact `MoveCraftingStack` field/rule row in packet-contracts. Valid seed: `sequence=1,from=8,to=44`.

Required cases: 45 rejected;9->10 rejected;8->44 accepted; equal indices rejected. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleMoveCraftingStack$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked move_crafting_stack_corpus`.

<a id="node-3-23"></a>

## 3.23 — OpenContainer (C/Play/8)

Dependencies: 1.5, 3.1, 3.2. Source module `open_container.rs`; test/Go producer stem `open_container`. Consume the shared packet-node steps above and the exact `OpenContainer` field/rule row in packet-contracts. Valid seed: `sequence=1,yaw=0.5,pitch=-0.25`.

Required cases: NaN/infinity angles rejected;sequence0 accepted; no target identity supplied. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleOpenContainer$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked open_container_corpus`.

<a id="node-3-24"></a>

## 3.24 — MoveContainerStack (C/Play/9)

Dependencies: 1.5, 3.1, 3.2. Source module `move_container_stack.rs`; test/Go producer stem `move_container_stack`. Consume the shared packet-node steps above and the exact `MoveContainerStack` field/rule row in packet-contracts. Valid seed: `sequence=1,container=F,from=38,to=0`.

Required cases: F from38 valid/to38 invalid;C slot62 valid/63 invalid; mutate dimension1,generation0,furnace slot32,chest slot16: constructor,encode and decode all reject. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleMoveContainerStack$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked move_container_stack_corpus`.

<a id="node-3-25"></a>

## 3.25 — CloseContainer (C/Play/10)

Dependencies: 1.5, 3.1, 3.2. Source module `close_container.rs`; test/Go producer stem `close_container`. Consume the shared packet-node steps above and the exact `CloseContainer` field/rule row in packet-contracts. Valid seed: `sequence=0`.

Required cases: 0/u64::MAX accepted; trailing or missing sequence bytes fail; no container field added. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleCloseContainer$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked close_container_corpus`.

<a id="node-3-26"></a>

## 3.26 — DropSelectedItem (C/Play/11)

Dependencies: 1.5, 3.1, 3.2. Source module `drop_selected_item.rs`; test/Go producer stem `drop_selected_item`. Consume the shared packet-node steps above and the exact `DropSelectedItem` field/rule row in packet-contracts. Valid seed: `sequence=0`.

Required cases: 0/u64::MAX accepted; exact8 bytes; no count/position supplied. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleDropSelectedItem$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked drop_selected_item_corpus`.

<a id="node-3-27"></a>

## 3.27 — ChatCommand (C/Play/12)

Dependencies: 1.5, 3.1, 3.2. Source module `chat_command.rs`; test/Go producer stem `chat_command`. Consume the shared packet-node steps above and the exact `ChatCommand` field/rule row in packet-contracts. Valid seed: `text="@Bob hello"`.

Required cases: 1024/1025 bytes; empty,leading/trailing whitespace,control,NUL,invalid UTF-8 fail; no sequence bytes. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleChatCommand$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked chat_command_corpus`.

<a id="node-3-28"></a>

## 3.28 — TillSoil (C/Play/13)

Dependencies: 1.5, 3.1, 3.2. Source module `till_soil.rs`; test/Go producer stem `till_soil`. Consume the shared packet-node steps above and the exact `TillSoil` field/rule row in packet-contracts. Valid seed: `sequence=1,yaw=0.5,pitch=-0.25`.

Required cases: NaN/infinity rejected;sequence0 accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleTillSoil$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked till_soil_corpus`.

<a id="node-3-29"></a>

## 3.29 — BoneMeal (C/Play/14)

Dependencies: 1.5, 3.1, 3.2. Source module `bone_meal.rs`; test/Go producer stem `bone_meal`. Consume the shared packet-node steps above and the exact `BoneMeal` field/rule row in packet-contracts. Valid seed: `sequence=1,yaw=0.5,pitch=-0.25`.

Required cases: NaN/infinity rejected;sequence0 accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleBoneMeal$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked bone_meal_corpus`.

<a id="node-3-30"></a>

## 3.30 — TakeCraftingOutput (C/Play/15)

Dependencies: 1.5, 3.1, 3.2. Source module `take_crafting_output.rs`; test/Go producer stem `take_crafting_output`. Consume the shared packet-node steps above and the exact `TakeCraftingOutput` field/rule row in packet-contracts. Valid seed: `sequence=1`.

Required cases: sequence0 rejected,1/u64::MAX accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleTakeCraftingOutput$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked take_crafting_output_corpus`.

<a id="node-3-31"></a>

## 3.31 — CollectWater (C/Play/16)

Dependencies: 1.5, 3.1, 3.2. Source module `collect_water.rs`; test/Go producer stem `collect_water`. Consume the shared packet-node steps above and the exact `CollectWater` field/rule row in packet-contracts. Valid seed: `sequence=1,yaw=0.5,pitch=-0.25`.

Required cases: NaN/infinity rejected;sequence0 accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleCollectWater$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked collect_water_corpus`.

<a id="node-3-32"></a>

## 3.32 — PlaceWater (C/Play/17)

Dependencies: 1.5, 3.1, 3.2. Source module `place_water.rs`; test/Go producer stem `place_water`. Consume the shared packet-node steps above and the exact `PlaceWater` field/rule row in packet-contracts. Valid seed: `sequence=1,yaw=0.5,pitch=-0.25`.

Required cases: NaN/infinity rejected;sequence0 accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOraclePlaceWater$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked place_water_corpus`.

<a id="node-3-33"></a>

## 3.33 — EquipArmor (C/Play/18)

Dependencies: 1.5, 3.1, 3.2. Source module `equip_armor.rs`; test/Go producer stem `equip_armor`. Consume the shared packet-node steps above and the exact `EquipArmor` field/rule row in packet-contracts. Valid seed: `sequence=0`.

Required cases: 0/u64::MAX accepted; no requested armor slot or item supplied. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleEquipArmor$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked equip_armor_corpus`.

<a id="node-3-34"></a>

## 3.34 — MoveStackPartial (C/Play/19)

Dependencies: 1.5, 3.1, 3.2. Source module `move_stack_partial.rs`; test/Go producer stem `move_stack_partial`. Consume the shared packet-node steps above and the exact `MoveStackPartial` field/rule row in packet-contracts. Valid seed: `sequence=1,container=NONE,view=Inventory,from=0,to=35,single=true`.

Required cases: View3 fails;NONE with chunk_x1 fails;container view with generation0/dimension1/physical slot overflow fails;crafting9->10 and furnace0->38 allowed here;equal indices fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleMoveStackPartial$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked move_stack_partial_corpus`.

<a id="node-3-35"></a>

## 3.35 — QuickMoveStack (C/Play/20)

Dependencies: 1.5, 3.1, 3.2, 3.34. Source module `move_stack_partial.rs`; test/Go producer stem `quick_move_stack`. Consume the shared packet-node steps above and the exact `QuickMoveStack` field/rule row in packet-contracts. Valid seed: `sequence=1,container=C,view=Container,from=62`.

Required cases: 63 fails;raw real ref invalid dimension/generation/slot fails;inventory/crafting require exact NONE;no destination. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleQuickMoveStack$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked quick_move_stack_corpus`.

<a id="node-3-36"></a>

## 3.36 — DropStack (C/Play/21)

Dependencies: 1.5, 3.1, 3.2, 3.35. Source module `move_stack_partial.rs`; test/Go producer stem `drop_stack`. Consume the shared packet-node steps above and the exact `DropStack` field/rule row in packet-contracts. Valid seed: `sequence=1,container=NONE,view=Crafting,slot=44`.

Required cases: 45 fails;real ref invalid dimension/generation/slot fails;inventory NONE chunk_x1 fails;no count. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleDropStack$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked drop_stack_corpus`.

<a id="node-3-37"></a>

## 3.37 — ChunkSnapshot (S/Play/0)

Dependencies: 1.5, 3.1, 3.2. Source module `chunk_snapshot.rs`; test/Go producer stem `chunk_snapshot`. Consume the shared packet-node steps above and the exact `ChunkSnapshot` field/rule row in packet-contracts. Valid seed: `dimension=1,chunk=(-1,2),revision=1,sections=24 Single(air) in Y0..23 order`.

Required cases: revision0,dimension2,section count23/25,duplicate Y,invalid palette/index/word length rejected before compression;compressed1MiB+1,decoded2MiB+1 and expansion-bomb reject;short dst unchanged. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleChunkSnapshot$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked chunk_snapshot_corpus`.

<a id="node-3-38"></a>

## 3.38 — BlockChanges (S/Play/1)

Dependencies: 1.5, 3.1, 3.2. Source module `block_changes.rs`; test/Go producer stem `block_changes`. Consume the shared packet-node steps above and the exact `BlockChanges` field/rule row in packet-contracts. Valid seed: `dimension=0,chunk=(-1,-1),base_revision=1,new_revision=2,changes=[position(-1,-64,-1),block1]`.

Required cases: empty accepted;4096/4097 count;wrong chunk,Y320/-65,block90,duplicate/unsorted local index fail;base0/MAX and new!=base+1 fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleBlockChanges$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked block_changes_corpus`.

<a id="node-3-39"></a>

## 3.39 — ForgetChunks (S/Play/2)

Dependencies: 1.5, 3.1, 3.2. Source module `forget_chunks.rs`; test/Go producer stem `forget_chunks`. Consume the shared packet-node steps above and the exact `ForgetChunks` field/rule row in packet-contracts. Valid seed: `dimension=1,chunks=[(2,1),(-1,0)]`.

Required cases: Unsorted unique input preserves order;0/4097 count fail;4096 valid;duplicate chunk and dimension2 fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleForgetChunks$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked forget_chunks_corpus`.

<a id="node-3-40"></a>

## 3.40 — PlayerState (S/Play/3)

Dependencies: 1.5, 3.1, 3.2. Source module `player_state.rs`; test/Go producer stem `player_state`. Consume the shared packet-node steps above and the exact `PlayerState` field/rule row in packet-contracts. Valid seed: `server_tick=0,last_input_sequence=0,dimension=1,position=ZERO3,velocity=ZERO3,yaw=0,pitch=4,on_ground=false,ready=true,reset=false,mining_active=false,mining_target=ZERO_POS,mining_progress_ticks=0,mining_required_ticks=0,mining_harvestable=false,health=20,oxygen=300,hunger=20,saturation_zero=false,day_phase_offset=23999,world_time_ticks=0,weather_kind=0,season=0,season_progress=255,temperature=-128,armor_points=20`.

Required cases: Each finite field NaN fails; inactive nonzero mining field fails;active progress1/required2 accepted,0/2 and2/2 fail;health21,oxygen301,hunger21,offset24000,weather3,season4,armor21 fail;progress255/temp127 allowed. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOraclePlayerState$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked player_state_corpus`.

<a id="node-3-41"></a>

## 3.41 — CommandRejected (S/Play/4)

Dependencies: 1.5, 3.1, 3.2. Source module `command_rejected.rs`; test/Go producer stem `command_rejected`. Consume the shared packet-node steps above and the exact `CommandRejected` field/rule row in packet-contracts. Valid seed: `sequence=0,reason=invalid_ray`.

Required cases: all15 named reasons map wire1..15;wire0/16 fail;internal0 is not wire0. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleCommandRejected$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked command_rejected_corpus`.

<a id="node-3-42"></a>

## 3.42 — KeepAlive (S/Play/5)

Dependencies: 1.5, 3.1, 3.2. Source module `keep_alive.rs`; test/Go producer stem `keep_alive`. Consume the shared packet-node steps above and the exact `KeepAlive` field/rule row in packet-contracts. Valid seed: `token=1`.

Required cases: token0 fails;u64::MAX accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleKeepAlive$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked keep_alive_corpus`.

<a id="node-3-43"></a>

## 3.43 — Disconnect (S/Play/6)

Dependencies: 1.5, 3.1, 3.2. Source module `disconnect.rs`; test/Go producer stem `disconnect`. Consume the shared packet-node steps above and the exact `Disconnect` field/rule row in packet-contracts. Valid seed: `code=1,message=""`.

Required cases: codes1..5 valid;0/6 fail;message256/257 bytes;invalid UTF-8 fails. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleDisconnect$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked disconnect_corpus`.

<a id="node-3-44"></a>

## 3.44 — RemotePlayerSpawn (S/Play/7)

Dependencies: 1.5, 3.1, 3.2. Source module `remote_player_spawn.rs`; test/Go producer stem `remote_player_spawn`. Consume the shared packet-node steps above and the exact `RemotePlayerSpawn` field/rule row in packet-contracts. Valid seed: `player_id=P1,display_name="Alice",server_tick=0,dimension=1,position=ZERO3,yaw=0,pitch=4`.

Required cases: UUID0,noncanonical name,dimension2,nonfinite pose fail;pitch4 remains valid. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleRemotePlayerSpawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked remote_player_spawn_corpus`.

<a id="node-3-45"></a>

## 3.45 — RemotePlayerDespawn (S/Play/8)

Dependencies: 1.5, 3.1, 3.2. Source module `remote_player_despawn.rs`; test/Go producer stem `remote_player_despawn`. Consume the shared packet-node steps above and the exact `RemotePlayerDespawn` field/rule row in packet-contracts. Valid seed: `player_id=P1`.

Required cases: UUID0/wrong version/variant fail;exact16 bytes. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleRemotePlayerDespawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked remote_player_despawn_corpus`.

<a id="node-3-46"></a>

## 3.46 — RemotePlayerStates (S/Play/9)

Dependencies: 1.5, 3.1, 3.2. Source module `remote_player_states.rs`; test/Go producer stem `remote_player_states`. Consume the shared packet-node steps above and the exact `RemotePlayerStates` field/rule row in packet-contracts. Valid seed: `server_tick=0,players=[P1 dimension1 positionZERO3 yaw0 pitch4 resetfalse]`.

Required cases: count1/7 valid,0/8 invalid;duplicate/reversed IDs;NaN pose;pitch4 valid. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleRemotePlayerStates$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked remote_player_states_corpus`.

<a id="node-3-47"></a>

## 3.47 — InventoryState (S/Play/10)

Dependencies: 1.5, 3.1, 3.2. Source module `inventory_state.rs`; test/Go producer stem `inventory_state`. Consume the shared packet-node steps above and the exact `InventoryState` field/rule row in packet-contracts. Valid seed: `selected=8,hotbar=[EMPTY;9],backpack=[EMPTY;27]`.

Required cases: selected9/255 fails encode; ordinary durable58 count1 durability0 rejected;fixed181 bytes;last backpack field preserved. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleInventoryState$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked inventory_state_corpus`.

<a id="node-3-48"></a>

## 3.48 — ItemDropUpserts (S/Play/11)

Dependencies: 1.5, 3.1, 3.2. Source module `item_drop_upserts.rs`; test/Go producer stem `item_drop_upserts`. Consume the shared packet-node steps above and the exact `ItemDropUpserts` field/rule row in packet-contracts. Valid seed: `server_tick=0,drops=[id=D,block_index=98303,stack=EMPTY]`.

Required cases: Empty stack allowed;block98304 fails;count0/33 fails,32 valid;duplicate/unsorted DropId;raw dimension-1 accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleItemDropUpserts$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked item_drop_upserts_corpus`.

<a id="node-3-49"></a>

## 3.49 — ItemDropRemoves (S/Play/12)

Dependencies: 1.5, 3.1, 3.2. Source module `item_drop_removes.rs`; test/Go producer stem `item_drop_removes`. Consume the shared packet-node steps above and the exact `ItemDropRemoves` field/rule row in packet-contracts. Valid seed: `server_tick=0,ids=[D]`.

Required cases: count0/33 fails,32 valid;generation0,slot32,duplicate/unsorted fail;dimension-1 allowed. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleItemDropRemoves$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked item_drop_removes_corpus`.

<a id="node-3-50"></a>

## 3.50 — FurnaceState (S/Play/13)

Dependencies: 1.5, 3.1, 3.2. Source module `furnace_state.rs`; test/Go producer stem `furnace_state`. Consume the shared packet-node steps above and the exact `FurnaceState` field/rule row in packet-contracts. Valid seed: `furnace=F,input=(6,1,0),fuel=(5,1,0),output=(7,1,0),progress_ticks=199,burn_ticks=1600`.

Required cases: Input18/27/53,output23/24/54 valid;input1/fuel1/output1 fail;progress200/burn1601 fail;chest ref fails; no additional timer consistency. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleFurnaceState$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked furnace_state_corpus`.

<a id="node-3-51"></a>

## 3.51 — ContainerClosed (S/Play/14)

Dependencies: 1.5, 3.1, 3.2. Source module `container_closed.rs`; test/Go producer stem `container_closed`. Consume the shared packet-node steps above and the exact `ContainerClosed` field/rule row in packet-contracts. Valid seed: `container=C`.

Required cases: Both F/C valid;generation0,dimension1,slot overflow fail;external import must compile and typed registry execute key S/Play/14. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleContainerClosed$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked container_closed_corpus`.

<a id="node-3-52"></a>

## 3.52 — ChestState (S/Play/15)

Dependencies: 1.5, 3.1, 3.2. Source module `chest_state.rs`; test/Go producer stem `chest_state`. Consume the shared packet-node steps above and the exact `ChestState` field/rule row in packet-contracts. Valid seed: `chest=C,items=[EMPTY;27]`.

Required cases: F ref fails;last item65 count1 durability0 valid;nondurable durability1 fails;wrong fixed payload/trailing fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleChestState$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked chest_state_corpus`.

<a id="node-3-53"></a>

## 3.53 — ChatEvent (S/Play/16)

Dependencies: 1.5, 3.1, 3.2. Source module `chat_event.rs`; test/Go producer stem `chat_event`. Consume the shared packet-node steps above and the exact `ChatEvent` field/rule row in packet-contracts. Valid seed: `event_id=1,player_id=P1,player_name="Alice",companion_id=P2,companion_name="Bob",kind=Accepted,reason=None,command="hello",speech=""`.

Required cases: Instantiate every legal kind/reason branch in packet-contracts, including all5 TaskFailed reasons;reject event0,reason3,illegal kind/reason,simultaneous command/speech;zero ID only InvalidFormat/UnknownCompanion;1328 max payload,1024 command and256 speech. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleChatEvent$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked chat_event_corpus`.

<a id="node-3-54"></a>

## 3.54 — CompanionSpawn (S/Play/17)

Dependencies: 1.5, 3.1, 3.2. Source module `companion_spawn.rs`; test/Go producer stem `companion_spawn`. Consume the shared packet-node steps above and the exact `CompanionSpawn` field/rule row in packet-contracts. Valid seed: `id=P2,name="Bob",tick=0,dimension=0,position=ZERO3,yaw=0,pitch=0`.

Required cases: Zero-ID constructed value rejected;dimension1/name with space/NaN fail;pitch±pi/2 accepted,next float outside fails. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleCompanionSpawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked companion_spawn_corpus`.

<a id="node-3-55"></a>

## 3.55 — CompanionStates (S/Play/18)

Dependencies: 1.5, 3.1, 3.2. Source module `companion_states.rs`; test/Go producer stem `companion_states`. Consume the shared packet-node steps above and the exact `CompanionStates` field/rule row in packet-contracts. Valid seed: `tick=0,states=[P2 dimension0 positionZERO3 yaw0 pitch0 resetfalse]`.

Required cases: count1/4 valid,0/5 invalid;zero/duplicate/unsorted IDs;dimension1 and excessive pitch fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleCompanionStates$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked companion_states_corpus`.

<a id="node-3-56"></a>

## 3.56 — CompanionDespawn (S/Play/19)

Dependencies: 1.5, 3.1, 3.2. Source module `companion_despawn.rs`; test/Go producer stem `companion_despawn`. Consume the shared packet-node steps above and the exact `CompanionDespawn` field/rule row in packet-contracts. Valid seed: `id=P2`.

Required cases: Zero-ID constructor/encode/decode rejected;no public NONE valid ID. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleCompanionDespawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked companion_despawn_corpus`.

<a id="node-3-57"></a>

## 3.57 — PlaceBlockSucceeded (S/Play/20)

Dependencies: 1.5, 3.1, 3.2. Source module `place_block_succeeded.rs`; test/Go producer stem `place_block_succeeded`. Consume the shared packet-node steps above and the exact `PlaceBlockSucceeded` field/rule row in packet-contracts. Valid seed: `sequence=0`.

Required cases: 0/u64::MAX accepted;exact8 bytes;no generic success payload added. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOraclePlaceBlockSucceeded$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked place_block_succeeded_corpus`.

<a id="node-3-58"></a>

## 3.58 — CraftingState (S/Play/21)

Dependencies: 1.5, 3.1, 3.2. Source module `crafting_state.rs`; test/Go producer stem `crafting_state`. Consume the shared packet-node steps above and the exact `CraftingState` field/rule row in packet-contracts. Valid seed: `size=Personal,slots=[EMPTY;9],output=EMPTY`.

Required cases: size2/3 valid,1/4 fail;personal slot4 nonempty fails,workbench slot8 allowed; invalid output stack fails. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleCraftingState$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked crafting_state_corpus`.

<a id="node-3-59"></a>

## 3.59 — HostileSpawn (S/Play/22)

Dependencies: 1.5, 3.1, 3.2. Source module `hostile_spawn.rs`; test/Go producer stem `hostile_spawn`. Consume the shared packet-node steps above and the exact `HostileSpawn` field/rule row in packet-contracts. Valid seed: `server_tick=0,spawns=[id1 dimension0 positionZERO3 yaw0 health20 kind0]`.

Required cases: 1/64 valid,0/65 count fail;ID0/duplicate/reversed;dimension1,health0/21,kind2,nonfinite fail;kind1 accepted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleHostileSpawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked hostile_spawn_corpus`.

<a id="node-3-60"></a>

## 3.60 — HostileState (S/Play/23)

Dependencies: 1.5, 3.1, 3.2. Source module `hostile_state.rs`; test/Go producer stem `hostile_state`. Consume the shared packet-node steps above and the exact `HostileState` field/rule row in packet-contracts. Valid seed: `server_tick=0,states=[id1 positionZERO3 velocityZERO3 yaw0 health20 kind1]`.

Required cases: 1/64 valid,0/65 fail;ID0/order/health/kind2/nonfinite fail;no dimension bytes. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleHostileState$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked hostile_state_corpus`.

<a id="node-3-61"></a>

## 3.61 — HostileDespawn (S/Play/24)

Dependencies: 1.5, 3.1, 3.2. Source module `hostile_despawn.rs`; test/Go producer stem `hostile_despawn`. Consume the shared packet-node steps above and the exact `HostileDespawn` field/rule row in packet-contracts. Valid seed: `server_tick=0,ids=[1]`.

Required cases: 1/64 valid,0/65 fail;ID0/duplicate/reversed fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleHostileDespawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked hostile_despawn_corpus`.

<a id="node-3-62"></a>

## 3.62 — CombatHit (S/Play/25)

Dependencies: 1.5, 3.1, 3.2. Source module `combat_hit.rs`; test/Go producer stem `combat_hit`. Consume the shared packet-node steps above and the exact `CombatHit` field/rule row in packet-contracts. Valid seed: `server_tick=1,damage=20,target_kind=Player`.

Required cases: tick0,damage0/21,target0/4 fail;targets1/2/3 valid;exact10 bytes. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleCombatHit$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked combat_hit_corpus`.

<a id="node-3-63"></a>

## 3.63 — PassiveSpawn (S/Play/26)

Dependencies: 1.5, 3.1, 3.2. Source module `passive_spawn.rs`; test/Go producer stem `passive_spawn`. Consume the shared packet-node steps above and the exact `PassiveSpawn` field/rule row in packet-contracts. Valid seed: `server_tick=0,spawns=[id1 dimension0 positionZERO3 yaw0 health20]`.

Required cases: count64 valid despite authority32;0/65 fail;ID0/order/health0/21/dimension1/nonfinite fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOraclePassiveSpawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked passive_spawn_corpus`.

<a id="node-3-64"></a>

## 3.64 — PassiveState (S/Play/27)

Dependencies: 1.5, 3.1, 3.2. Source module `passive_state.rs`; test/Go producer stem `passive_state`. Consume the shared packet-node steps above and the exact `PassiveState` field/rule row in packet-contracts. Valid seed: `server_tick=0,states=[id1 positionZERO3 velocityZERO3 yaw0 health20 grazing1]`.

Required cases: grazing0/1 valid,2 fails;count64 valid,0/65 fail;ID/order/health/nonfinite fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOraclePassiveState$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked passive_state_corpus`.

<a id="node-3-65"></a>

## 3.65 — PassiveDespawn (S/Play/28)

Dependencies: 1.5, 3.1, 3.2. Source module `passive_despawn.rs`; test/Go producer stem `passive_despawn`. Consume the shared packet-node steps above and the exact `PassiveDespawn` field/rule row in packet-contracts. Valid seed: `server_tick=0,despawns=[id1 reasonDied]`.

Required cases: reason0/1 valid,2 fails;count0/65,ID0/order fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOraclePassiveDespawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked passive_despawn_corpus`.

<a id="node-3-66"></a>

## 3.66 — ProjectileSpawn (S/Play/29)

Dependencies: 1.5, 3.1, 3.2. Source module `projectile_spawn.rs`; test/Go producer stem `projectile_spawn`. Consume the shared packet-node steps above and the exact `ProjectileSpawn` field/rule row in packet-contracts. Valid seed: `server_tick=0,spawns=[id1 kindArrow dimension1 positionZERO3 velocityZERO3]`.

Required cases: 1/128 valid,0/129 fail;both kinds in both dimensions valid;kind2/dimension2/ID0/order/nonfinite fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleProjectileSpawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked projectile_spawn_corpus`.

<a id="node-3-67"></a>

## 3.67 — ProjectileState (S/Play/30)

Dependencies: 1.5, 3.1, 3.2. Source module `projectile_state.rs`; test/Go producer stem `projectile_state`. Consume the shared packet-node steps above and the exact `ProjectileState` field/rule row in packet-contracts. Valid seed: `server_tick=0,states=[id1 positionZERO3]`.

Required cases: 1/128 valid,0/129 fail;ID0/order/nonfinite fail;no velocity/kind/dimension emitted. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleProjectileState$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked projectile_state_corpus`.

<a id="node-3-68"></a>

## 3.68 — ProjectileDespawn (S/Play/31)

Dependencies: 1.5, 3.1, 3.2. Source module `projectile_despawn.rs`; test/Go producer stem `projectile_despawn`. Consume the shared packet-node steps above and the exact `ProjectileDespawn` field/rule row in packet-contracts. Valid seed: `server_tick=0,ids=[1]`.

Required cases: 1/128 valid,0/129 fail;ID0/duplicate/reversed fail. The seed and each applicable boundary are independent Go-produced operations; constructor validation alone is not evidence for public encode/decode.

Run `go test ./packages/tools/cmd/runtime-oracle -run '^TestProtocolOracleProjectileDespawn$' -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked projectile_despawn_corpus`.

## Snapshot-specific implementation for 3.37

ChunkSnapshot replaces the noncompressed inherent encode_into signature with `SnapshotCodec::encode_into(&mut self,value:&ChunkSnapshot,dst:&mut[u8])->Result<usize,ProtocolError>` and `decode(&mut self,payload:&[u8])->Result<ChunkSnapshot,ProtocolError>`. `SnapshotCodec::try_new()` owns zstd compressor/decompressor and reusable logical/compressed Vec scratch. `validate` and `logical_len` stay on ChunkSnapshot; **there is no encoded_len that pretends compressed size is known before compression**. `encode` is a fallible owned convenience on the context. ProtocolCodec owns one SnapshotCodec.

Algorithm: validate24 ordered sections and all logical values; checked logical size<=2MiB; write logical bytes to reusable scratch; compress to separately bounded scratch with the existing compression parameters; if compressed bytes>1MiB return capacity; build exact existing envelope/checksum; preflight final destination; copy once. Decode checks envelope/version/declared limits/integrity, streams at most decoded_limit+1 bytes and rejects overrun, consumes the complete valid zstd stream, including concatenated/skippable frames accepted by Go DecodeAll, rejects malformed trailing compressed data, and requires the total decoded length to match the envelope, then validates sections into a temporary result. No output publication until complete. Retain input palette ordering and do not canonicalize by dense expansion. A context is exclusively borrowed, never shared behind a global mutex. Warm scratch capacity is retained; cold initialization allocations are reported separately. Compare cross-decodes and **logical** bytes, not compression implementation output bytes.

Tests include24 distinct section sentinels; each palette representation; incompressible legal data; declared-small/actual-large bomb; lying length/checksum; concatenated frames and a skippable frame with the same logical output; context reuse after error; short output and unchanged suffix; correct depth dimension1. Use the same snapshot corpus commands above.

<a id="node-3-69"></a>

## 3.69 — Allocation and encoder audit closure

Dependencies:3.4,3.5. Own `PROTOCOL/tests/allocation_contract.rs` and encoder-audit test module only; any discovered behavior fix returns to its family node. Warm one ProtocolCodec and reusable buffers before enabling the thread-local counting allocator; disable counting during test harness logging. Run one test thread. Assert zero allocations for borrowed frame, PlayerInput encode/decode and InventoryState encode/decode including registry dispatch. Assert no per-primitive allocations in a64-record hostile packet and record unavoidable owned collection allocation separately. Snapshot has bounded retained scratch, not a zero-allocation decoded ownership claim.

Add test-only invalid-constructed seeds for each publicly mutable packet field category, then audit no public packet encoder returns raw Vec or uses caller-reachable expect. Distinguish internal post-preflight invariants from caller validation. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test allocation_contract --locked -- --test-threads=1` and full runtime_contract. Report cold/warm calls, bytes copied and retained scratch; latency informational. Commit `test(protocol): enforce bounded encoding and allocation contracts`.
