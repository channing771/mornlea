# Executable Rust domain corpus execution plan

This packet implements repair area 5 from `design.md`. The exact input,
constructor, normalization, and rejection contracts are frozen in
[`03-domain-adapters.md`](03-domain-adapters.md); workers implement those tables
rather than choosing mappings locally. All nodes touch the shared
`mornlea_domain` test target, so nodes 3.1–3.10 run serially. Every dispatch
uses Superpowers `subagent-driven-development`; every worker uses
`using-superpowers`, `test-driven-development`, `systematic-debugging` when a
failure is unexpected, and `verification-before-completion`. Each node receives
independent review through `requesting-code-review`, and the controller applies
`receiving-code-review` before acceptance.

Workers never edit `tasks.md`, `ledger.md`, production Rust sources, corpus case
assets, or another topic module. Node 3.1 alone may correct one manifest
consumer metadata value as specified below. After node 3.1, each topic file has
one exclusive serial owner.

## Node 3.1: Create the closed domain corpus dispatcher skeleton

**Deliverable and prerequisites**

After node 2.2, create a compiling integration target that loads the corpus
once, proves the corrected consumer partition, and routes all 376
`mornlea_domain` cases exactly once into eight bounded topic modules. This node
does not claim behavioral parity.

**Editable files**

- `testdata/runtime-migration/contracts.json`, only the consumer of
  `domain.input/45/session-sequence-arrival`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/support.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/identity_text.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/values.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/command_control.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/command_inventory.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_player.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_world.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_inventory.rs`
- new `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_people.rs`
- `packages/engine/crates/mornlea_domain/AGENTS.md`

The manifest edit changes only that one case from `mornlea_domain` to the
closed consumer `external:runtime-authority`. The input and expectation files,
digests, family, case ID, versions, and every other manifest entry remain byte
identical. The reason is architectural: the expectation contains Go authority
admission and world effects that `mornlea_domain::order_commands` cannot
produce. The focused handwritten `command_order` suite remains the Rust
ordering proof.

**Frozen support API**

`corpus_domain.rs` includes `../../../tests/runtime_corpus.rs` and mounts the
nine sibling modules. `support.rs` defines:

```rust
#[derive(Debug)]
pub enum DispatchError {
    NotImplemented { case_id: String, rule: String },
    InvalidCase { case_id: String, message: String },
}

pub struct ExecutedCase {
    pub case: FrozenCase,
    pub actual: serde_json::Value,
}

pub type OwnsCase = fn(&FrozenCase) -> bool;
pub type ExecuteCase = fn(&FrozenCase) -> Result<serde_json::Value, DispatchError>;

pub fn execute_topic(
    expected_count: usize,
    owns: OwnsCase,
    execute: ExecuteCase,
) -> Result<Vec<ExecutedCase>, DispatchError>;

pub fn assert_domain_normalized(executed: &ExecutedCase);
```

`execute_topic` loads `CorpusConsumer::Domain` once, moves each owned
`FrozenCase` into its `ExecutedCase`, rejects duplicate IDs, and requires the
exact nonzero count. `assert_domain_normalized` clones the expectation only for
comparison. For accepted `domain.command_control` and
`domain.command_inventory` cases it removes exactly `fields.wire`; it removes
nothing else and never exposes expected values to executors. Every other case
compares the complete normalized value.

Support also freezes this complete shared parsing/normalization API; topic
workers may compose it but may not add a second permissive reader or redesign
its absence policy:

```rust
pub type JsonMap = serde_json::Map<String, serde_json::Value>;

pub fn invalid_case(case: &FrozenCase, message: impl Into<String>) -> DispatchError;
pub fn input_object(case: &FrozenCase) -> Result<&JsonMap, DispatchError>;

pub fn required_object<'a>(case: &FrozenCase, object: &'a JsonMap, key: &str)
    -> Result<&'a JsonMap, DispatchError>;
pub fn optional_object<'a>(case: &FrozenCase, object: &'a JsonMap, key: &str)
    -> Result<Option<&'a JsonMap>, DispatchError>;
pub fn required_array<'a>(case: &FrozenCase, object: &'a JsonMap, key: &str)
    -> Result<&'a [serde_json::Value], DispatchError>;
pub fn optional_array<'a>(case: &FrozenCase, object: &'a JsonMap, key: &str)
    -> Result<Option<&'a [serde_json::Value]>, DispatchError>;
pub fn required_string<'a>(case: &FrozenCase, object: &'a JsonMap, key: &str)
    -> Result<&'a str, DispatchError>;
pub fn optional_string<'a>(case: &FrozenCase, object: &'a JsonMap, key: &str)
    -> Result<Option<&'a str>, DispatchError>;
pub fn required_bool(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<bool, DispatchError>;
pub fn optional_bool(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<Option<bool>, DispatchError>;

pub fn required_i8(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<i8, DispatchError>;
pub fn required_u8(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<u8, DispatchError>;
pub fn required_u16(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<u16, DispatchError>;
pub fn required_u32(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<u32, DispatchError>;
pub fn required_i32(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<i32, DispatchError>;
pub fn required_i64(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<i64, DispatchError>;
pub fn required_u64(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<u64, DispatchError>;
pub fn optional_i8(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<Option<i8>, DispatchError>;
pub fn optional_u8(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<Option<u8>, DispatchError>;
pub fn optional_u16(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<Option<u16>, DispatchError>;
pub fn optional_u32(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<Option<u32>, DispatchError>;
pub fn optional_i32(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<Option<i32>, DispatchError>;
pub fn optional_i64(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<Option<i64>, DispatchError>;
pub fn optional_u64(case: &FrozenCase, object: &JsonMap, key: &str)
    -> Result<Option<u64>, DispatchError>;

pub fn value_object<'a>(case: &FrozenCase, path: &str, value: &'a serde_json::Value)
    -> Result<&'a JsonMap, DispatchError>;
pub fn value_array<'a>(case: &FrozenCase, path: &str, value: &'a serde_json::Value)
    -> Result<&'a [serde_json::Value], DispatchError>;
pub fn value_string<'a>(case: &FrozenCase, path: &str, value: &'a serde_json::Value)
    -> Result<&'a str, DispatchError>;
pub fn value_bool(case: &FrozenCase, path: &str, value: &serde_json::Value)
    -> Result<bool, DispatchError>;
pub fn value_i8(case: &FrozenCase, path: &str, value: &serde_json::Value)
    -> Result<i8, DispatchError>;
pub fn value_u8(case: &FrozenCase, path: &str, value: &serde_json::Value)
    -> Result<u8, DispatchError>;
pub fn value_u16(case: &FrozenCase, path: &str, value: &serde_json::Value)
    -> Result<u16, DispatchError>;
pub fn value_u32(case: &FrozenCase, path: &str, value: &serde_json::Value)
    -> Result<u32, DispatchError>;
pub fn value_i32(case: &FrozenCase, path: &str, value: &serde_json::Value)
    -> Result<i32, DispatchError>;
pub fn value_i64(case: &FrozenCase, path: &str, value: &serde_json::Value)
    -> Result<i64, DispatchError>;
pub fn value_u64(case: &FrozenCase, path: &str, value: &serde_json::Value)
    -> Result<u64, DispatchError>;
pub fn exact_array<'a, const N: usize>(
    case: &FrozenCase,
    path: &str,
    values: &'a [serde_json::Value],
) -> Result<&'a [serde_json::Value; N], DispatchError>;

pub fn parse_uuid_hex(case: &FrozenCase, path: &str, text: &str)
    -> Result<[u8; 16], DispatchError>;
pub fn parse_f32_token(case: &FrozenCase, path: &str, text: &str)
    -> Result<f32, DispatchError>;
pub fn normalize_u64(value: u64) -> serde_json::Value;
pub fn normalize_f32(value: f32) -> serde_json::Value;
pub fn normalize_uuid(value: [u8; 16]) -> serde_json::Value;
pub fn normalized_ok(category: &str, fields: JsonMap) -> serde_json::Value;
pub fn normalized_error(
    case: &FrozenCase,
    category: &str,
    rule: &str,
    fields: JsonMap,
) -> Result<serde_json::Value, DispatchError>;
```

Required readers reject missing or JSON `null`; optional readers map both
missing and `null` to `None` while preserving explicit empty strings, false and
zero as `Some`. Integer readers admit only integral JSON numbers losslessly
inside the named Rust width. Array-element readers use the supplied indexed
path in every error; topic modules iterate object arrays with `value_object`
and scalar arrays with the corresponding `value_*` reader. `exact_array`
rejects every length except `N`. `normalized_error` rejects a caller-supplied
reserved `fields.rule` before inserting the exact rule. `normalize_u64` emits a
decimal string, `normalize_f32` emits eight lowercase `to_bits()` hex digits,
and `normalize_uuid` emits 32 lowercase hex digits.

`parse_uuid_hex` accepts exactly 32 lowercase hex characters; semantic UUID
validity remains the identity constructor's job. `parse_f32_token` accepts
finite corpus decimal strings plus exactly `NaN`, `Inf`, `+Inf`, and `-Inf`,
preserving negative-zero bits and rejecting decimal overflow errors. Malformed
JSON, trailing content, missing fields, wrong widths, malformed tokens, wrong
consumers, and unknown rules return `InvalidCase`, not a normalized semantic
rejection.

The closed partition is:

| Module | Families or rules | Count |
| --- | --- | ---: |
| `identity_text` | all `domain.identity_values` | 31 |
| `values` | all `domain.values` | 99 |
| `command_control` | all `domain.command_control` | 28 |
| `command_inventory` | all `domain.command_inventory` | 54 |
| `event_player` | player/outcome event rules | 47 |
| `event_world` | world event rules | 38 |
| `event_inventory` | inventory event rules | 33 |
| `event_people` | remote-player/companion event rules | 46 |

Each module exports a closed `owns` predicate and a compiling `execute` stub
that returns `DispatchError::NotImplemented { case_id, rule }`. Its behavioral
test is present but ignored until the owning node removes the ignore. The
structure test separately requires exactly 376 domain, 1
`external:runtime-authority`, 154 `external:agent-contract`, and 2
`corpus_frame` cases, exact single ownership of every domain ID, JSON input,
and operation `admit`. The external runtime case must be exactly
`domain.input/45/session-sequence-arrival`; it must not match any domain owner.

**Red/green sequence**

1. Add the structure test first and record the missing target/module failure.
2. Make the single manifest consumer correction and test every frozen support
   signature: required missing/null failure; optional missing/null versus
   explicit empty/false/zero; signed/unsigned boundary and overflow for every
   integer width; object and scalar array element paths; exact arrays at
   `N-1/N/N+1`; UUID lowercase/length/hex; negative zero; each of `NaN`, `Inf`,
   `+Inf`, and `-Inf`; decimal overflow; normalized u64/f32/UUID formatting;
   and reserved error-rule rejection.
3. Add the eight predicates, exact counts, typed stubs, and ignored behavioral
   tests. A catch-all predicate is forbidden.
4. Add a comparator test proving a wire-only command difference is ignored but
   a non-wire semantic mutation fails.
5. Update `AGENTS.md`, format, and verify that no case asset changed.

**Expected results and validation**

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain corpus_structure --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain support:: --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
git diff --exit-code -- testdata/runtime-migration/cases
git diff --exit-code -- testdata/runtime-migration/expectations
```

The proposed commit is `test(domain): scaffold closed corpus dispatch`.

**Exclusions and rollback**

Do not regenerate assets, implement a generic interpreter, encode protocol
wire, execute the external authority case, add a catch-all owner, or change
production APIs. Reverting this node removes the target and restores the one
manifest consumer value.

## Node 3.2: Execute identity and text corpus cases

**Deliverable and prerequisites**

After node 3.1, remove the module's ignore and execute all 31
`domain.identity_values` cases through the public Rust identity/text APIs and
the exact adapter table. This node owns only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain/identity_text.rs`

**Implementation and TDD**

First unignore `identity_text_execute_31_cases`; the required red result is a
typed `NotImplemented` containing the first case ID. Implement only the five
rules `player-id`, `display-name`, `companion-name`, `command-text`, and
`speech-text`. Actual accepted values come from constructed Rust values; error
category/rule selection follows the explicit input precedence in the adapter
table. Unknown rules remain `InvalidCase`. Finish with one local semantic
mutation assertion and the existing handwritten identity/value suite.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain identity_text:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test identity_values --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
```

Exactly 31 unique IDs execute once. Proposed commit:
`test(domain): execute identity text corpus`.

Do not edit shared support, relax canonical parsing, or read normalized
expectations in `execute`. Rollback restores only `identity_text.rs` to node
3.1's typed `NotImplemented` stub plus ignored behavioral test; every later
topic and closure node becomes ineligible until 3.2 is accepted again.

## Node 3.3: Execute value and location corpus cases

**Deliverable and prerequisites**

After node 3.2, execute the 99 `domain.values` cases through the public item,
stack, drop, and container APIs. This node owns only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain/values.rs`

Unignore `values_execute_99_cases` and record the typed `NotImplemented` red.
Implement `item-table`, `item-stack`, `drop-id`, and `container-ref` exactly as
the adapter table specifies, including omission defaults and optional
item-table fields. Closed preconstruction failures remain `InvalidCase`.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain values:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test items_locations --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
```

Exactly 99 unique IDs execute once. Proposed commit:
`test(domain): execute value location corpus`.

Do not change production constructors, use expected fields as output, or edit
another topic. Rollback restores only `values.rs` to node 3.1's typed stub plus
ignored behavioral test; nodes 3.4–3.10 remain ineligible until 3.3 is accepted
again.

## Node 3.4: Execute control command corpus cases

**Deliverable and prerequisites**

After node 3.3, execute all 28 `domain.command_control` cases. This node owns
only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain/command_control.rs`

Unignore `command_control_execute_28_cases` and record the typed
`NotImplemented` red. Parse and construct the nine control rules through
`LookAngles`, payload constructors, `Command`, and `CommandEnvelope`. The
producer supplies only sequence; synthesize `tick = 0`, `session = 0`, and
`arrival_index = 0`, preserving the exact sequence. Normalize the complete
semantic projection from Rust; the shared comparator alone omits Go
`fields.wire`. Follow the adapter's field-specific precedence.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain command_control:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test command_control --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
```

Exactly 28 unique IDs execute once. Proposed commit:
`test(domain): execute control command corpus`.

Do not implement a Go wire encoder, expose synthetic metadata in normalized
output, or change `Command`. Rollback restores only `command_control.rs` to
node 3.1's typed stub plus ignored behavioral test; nodes 3.5–3.10 remain
ineligible until 3.4 is accepted again.

## Node 3.5: Execute inventory and chat command corpus cases

**Deliverable and prerequisites**

After node 3.4, execute all 54 `domain.command_inventory` cases. This node owns
only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain/command_inventory.rs`

Unignore `command_inventory_execute_54_cases` and record the typed
`NotImplemented` red. Implement the eleven rules in the adapter table,
including closed preconstruction classification for stack-view and reference
shapes. Sequenced commands synthesize `tick/session/arrival_index = 0` while
retaining sequence. Chat uses `CommandText` and `ChatIntent` outside
`CommandEnvelope`. The shared comparator alone omits `fields.wire`.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain command_inventory:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test command_inventory --test command_order --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
```

Exactly 54 unique IDs execute once, and the handwritten ordering suite remains
the Rust proof for `domain.input` semantics. Proposed commit:
`test(domain): execute inventory command corpus`.

Do not put chat into `Command`, execute the external authority case, invent a
container sentinel policy, or implement protocol encoding. Rollback restores
only `command_inventory.rs` to node 3.1's typed stub plus ignored behavioral
test; nodes 3.6–3.10 remain ineligible until 3.5 is accepted again.

## Node 3.6: Execute player event corpus cases

**Deliverable and prerequisites**

After node 3.5, execute all 47 player/outcome cases. This node owns only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_player.rs`

Unignore `event_player_execute_47_cases` and record the typed
`NotImplemented` red. Implement `player-state`, `command-rejected`,
`place-block-succeeded`, and `combat-hit` in construction order. Use
`PlayerState::new`, not a nonexistent checked constructor. Preserve all
field-context disambiguation named in the adapter table.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain event_player:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test event_player --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
```

Exactly 47 unique IDs execute once. Proposed commit:
`test(domain): execute player event corpus`.

Do not bypass nested constructors or collapse field-specific rejection rules.
Rollback restores only `event_player.rs` to node 3.1's typed stub plus ignored
behavioral test; nodes 3.7–3.10 remain ineligible until 3.6 is accepted again.

## Node 3.7: Execute world event corpus cases

**Deliverable and prerequisites**

After node 3.6, execute all 38 world cases. This node owns only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_world.rs`

Unignore `event_world_execute_38_cases` and record the typed `NotImplemented`
red. Implement `chunk-snapshot`, `block-changes`, and `forget-chunks`, using the
real `PalettedSection` and event constructors. Preserve section/change/chunk
order and the explicit empty block-change barrier.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain event_world:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test event_world --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
```

Exactly 38 unique IDs execute once. Proposed commit:
`test(domain): execute world event corpus`.

Do not impose protocol packet caps, reproduce section validation locally, or
synthesize normalized sections from expectations. Rollback restores only
`event_world.rs` to node 3.1's typed stub plus ignored behavioral test; nodes
3.8–3.10 remain ineligible until 3.7 is accepted again.

## Node 3.8: Execute inventory event corpus cases

**Deliverable and prerequisites**

After node 3.7, execute all 33 inventory events. This node owns only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_inventory.rs`

Unignore `event_inventory_execute_33_cases` and record the typed
`NotImplemented` red. Implement `inventory-state`, `crafting-state`,
`furnace-state`, `chest-state`, and `container-closed`, including exact fixed
array sizes, slot order, and field-context error mapping.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain event_inventory:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test event_inventory --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
```

Exactly 33 unique IDs execute once. Proposed commit:
`test(domain): execute inventory event corpus`.

Do not default absent required arrays or flatten field-specific furnace errors.
Rollback restores only `event_inventory.rs` to node 3.1's typed stub plus
ignored behavioral test; nodes 3.9–3.10 remain ineligible until 3.8 is accepted
again.

## Node 3.9: Execute people event corpus cases

**Deliverable and prerequisites**

After node 3.8, execute all 46 remote-player and companion events. This node
owns only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_people.rs`

Unignore `event_people_execute_46_cases` and record the typed `NotImplemented`
red. Implement all six rules in the adapter table through checked identities,
names, vectors, rotations, lifecycle values, and batch constructors. Preserve
submitted state order; the semantic crate does not adopt protocol maxima 7/4.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain event_people:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test event_people --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
```

Exactly 46 unique IDs execute once. Proposed commit:
`test(domain): execute people event corpus`.

Do not impose wire caps or sort submitted state batches in the adapter.
Rollback restores only `event_people.rs` to node 3.1's typed stub plus ignored
behavioral test; node 3.10 remains ineligible until 3.9 is accepted again.

## Node 3.10: Close domain coverage and mutation detection

**Deliverable and prerequisites**

After node 3.9, integrate the eight already-green modules into one non-ignored
gate that loads the domain corpus once, executes every ID exactly once, and
proves the comparison detects semantic drift. This node owns only:

- `packages/engine/crates/mornlea_domain/tests/corpus_domain.rs`
- `packages/engine/crates/mornlea_domain/tests/corpus_domain/support.rs`

Add `corpus_domain_executes_376_unique_cases`: use the already-frozen predicates
and executors, reject overlaps/gaps/duplicates, require the exact per-topic and
total counts, and call `assert_domain_normalized` for every `ExecutedCase`.
Add a mutation test that changes one non-wire accepted semantic field and must
fail comparison. Add exclusion assertions proving the external authority case
is neither loaded nor dispatched as domain work. No module may reload its case
to recover the expectation; the retained `FrozenCase` is the comparison source.

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --locked
rustup run 1.97.1 cargo clippy --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --all-targets --locked -- -D warnings
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --check
git diff --exit-code -- testdata/runtime-migration/cases
git diff --exit-code -- testdata/runtime-migration/expectations
```

The gate reports exactly 31 + 99 + 28 + 54 + 47 + 38 + 33 + 46 = 376
unique executed IDs. Proposed commit:
`test(domain): close executable corpus coverage`.

Do not add a skipped-case allowlist, make the external authority case a Rust
domain obligation, edit topic modules, or weaken complete normalized equality.
Rollback restores only `corpus_domain.rs` and `support.rs` to their pre-closure
node 3.9 state; all eight topic modules remain green and independently
re-runnable, while the integrated 376-case acceptance claim is withdrawn.
