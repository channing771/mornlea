Every checkbox is a Superpowers task. Before editing, the assigned worker reads
the linked packet and invokes `using-superpowers` plus the packet's named
workflow skills. Behavioral nodes use `test-driven-development`; delegated
nodes use `subagent-driven-development`; unexpected failures use
`systematic-debugging`; every handoff uses `verification-before-completion` and
independent `requesting-code-review`. The controller uses
`receiving-code-review`, owns status, and never treats planning validation as
implementation acceptance.

## 1. Go evidence truthfulness

- [ ] 1.1 [Separate working and complete inventory reconciliation](plans/01-evidence.md#node-11-separate-working-and-complete-inventory-reconciliation). Direct prerequisites: none. Run `go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run 'TestContractInventory(Working|Complete|RejectsUnknownConsumer|RejectsUnsupportedCaseVersion)'`.
- [ ] 1.2 [Build traces only from executed observations](plans/01-evidence.md#node-12-build-traces-only-from-executed-observations). Direct prerequisites: 1.1. Run `go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run 'TestTrace|TestProtocolOracleFrame|TestExecutedObservation'`.

## 2. Corpus publication and loading

- [ ] 2.1 [Remove tracked-corpus rewrite paths](plans/02-corpus.md#node-21-remove-tracked-corpus-rewrite-paths). Direct prerequisites: 1.2. Run `go test ./packages/tools/cmd/runtime-oracle ./packages/shared/companion -race -count=1` and `git diff --exit-code -- testdata/runtime-migration`.
- [ ] 2.2 [Make the Rust corpus loader fail closed](plans/02-corpus.md#node-22-make-the-rust-corpus-loader-fail-closed). Direct prerequisites: 1.1. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_loader --locked` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract corpus_frame --locked`.

## 3. Executable Rust domain corpus

- [ ] 3.1 [Create the closed domain corpus dispatcher skeleton](plans/03-domain-corpus.md#node-31-create-the-closed-domain-corpus-dispatcher-skeleton). Direct prerequisites: 2.2. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain corpus_structure --locked`.
- [ ] 3.2 [Execute identity and text corpus cases](plans/03-domain-corpus.md#node-32-execute-identity-and-text-corpus-cases). Direct prerequisites: 3.1. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain identity_text:: --locked`.
- [ ] 3.3 [Execute value and location corpus cases](plans/03-domain-corpus.md#node-33-execute-value-and-location-corpus-cases). Direct prerequisites: 3.2. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain values:: --locked`.
- [ ] 3.4 [Execute control command corpus cases](plans/03-domain-corpus.md#node-34-execute-control-command-corpus-cases). Direct prerequisites: 3.3. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain command_control:: --locked`.
- [ ] 3.5 [Execute inventory and chat command corpus cases](plans/03-domain-corpus.md#node-35-execute-inventory-and-chat-command-corpus-cases). Direct prerequisites: 3.4. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain command_inventory:: --locked`.
- [ ] 3.6 [Execute player event corpus cases](plans/03-domain-corpus.md#node-36-execute-player-event-corpus-cases). Direct prerequisites: 3.5. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_player:: --locked`.
- [ ] 3.7 [Execute world event corpus cases](plans/03-domain-corpus.md#node-37-execute-world-event-corpus-cases). Direct prerequisites: 3.6. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_world:: --locked`.
- [ ] 3.8 [Execute inventory event corpus cases](plans/03-domain-corpus.md#node-38-execute-inventory-event-corpus-cases). Direct prerequisites: 3.7. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_inventory:: --locked`.
- [ ] 3.9 [Execute people event corpus cases](plans/03-domain-corpus.md#node-39-execute-people-event-corpus-cases). Direct prerequisites: 3.8. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_people:: --locked`.
- [ ] 3.10 [Close domain coverage and mutation detection](plans/03-domain-corpus.md#node-310-close-domain-coverage-and-mutation-detection). Direct prerequisites: 3.9. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain --locked` and require exactly 376 unique executed IDs.

## 4. Bounded domain construction

- [ ] 4.1 [Bound text and semantic batches before work](plans/04-domain-bounds.md#node-41-bound-text-and-semantic-batches-before-work). Direct prerequisites: 3.10. Run `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test resource_bounds --locked` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --locked`.

## 5. Controller closeout

- [ ] 5.1 [Review, validate, sync and archive the extracted baseline](plans/05-closeout.md#node-51-review-validate-sync-and-archive-the-extracted-baseline). Direct prerequisites: 1.2, 2.1, 2.2, 3.10, 4.1. Run `make rust-check`, `make test-race`, `make dev-check`, `openspec validate rust-runtime-foundation-baseline --strict --no-interactive`, and `openspec validate --all --strict --no-interactive` at one result SHA before sync or archive.
