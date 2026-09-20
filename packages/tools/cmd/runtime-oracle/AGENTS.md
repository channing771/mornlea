# Runtime oracle

`packages/tools/cmd/runtime-oracle` is the offline contract inventory and replay
oracle for OpenSpec change `rust-runtime-foundation`. It observes repository
sources and testdata only. Production code must not import live authority,
native ABI, network transports, or storage codecs; the empty allowed import set
is enforced by `packages/audit` `TestInternalDependenciesAreOneWay`.

## Inventory freeze (`inventory.go`, `discover.go`, `root.go`)

- `Discover` reads current protocol, save, kernel, and agent registries from
  files. It does not load production packages.
- `LoadInventory` reads the frozen corpus at `InventoryRelPath`
  (`testdata/runtime-migration/contracts.json`).
- `Reconcile` fails closed on a missing supported family, version drift, an
  extra inventory row, missing provenance, a missing coverage fixture, or
  incomplete identity. It does not infer parity from a covered subset.
- Enforcement: `TestContractInventoryReconcilesFrozenCorpus`,
  `TestContractInventoryRejectsMissingFamily`,
  `TestContractInventoryRejectsVersionMismatch`,
  `TestContractInventoryRejectsMissingCoverageFixture`,
  `TestContractInventoryRejectsIncompleteIdentity`.

## Focused Verification

```bash
go test ./packages/tools/cmd/runtime-oracle -run TestContractInventory -count=1
go test ./packages/tools/cmd/runtime-oracle -list TestContractInventory
go test ./packages/tools/cmd/runtime-oracle -race -count=1
```
