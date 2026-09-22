# Node 2.1: Fail-closed Godot validator prerequisites

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: none.
- Deliverable: project validation rejects an absent `rg` before scanning and cannot print a success result after the prerequisite failure.
- Required sub-skills: `superpowers:test-driven-development`, `superpowers:systematic-debugging`, and `superpowers:verification-before-completion`.

## File ownership

- Modify: `scripts/godot/validate-project.sh`.
- Create: `packages/audit/godot_validator_prerequisite_test.go`.
- Read-only authority: `scripts/AGENTS.md`; `packages/audit/AGENTS.md`; `packages/audit/godot_project_closure_test.go`; `packages/audit/godot_project_test.go`; every validator caller returned by `rg -n 'validate-project\.sh|godot-project-check' .`.
- Excluded: changing any validation rule, weakening a fixture, installing tools inside the validator, or changing Godot project content.

## Interfaces and behavior

- Preserve every existing command-line mode and exit code: no arguments, `--script-ownership`, `--desktop-only-fixtures`, and `--python-isolation-fixtures` remain valid; unsupported usage remains exit 2.
- Immediately after `set -euo pipefail`, check `command -v rg`. When missing, write exactly `missing required executable: rg` to stderr and exit 1.
- The check occurs before repository/project discovery and before any mode-specific work so every mode has the same dependency contract.
- Missing `rg` output must not contain `validation passed`, `fixture rejected`, or any other success marker.
- The validator does not call Homebrew, apt, curl, or a network installer.

## Test-first steps

1. Add `TestGodotProjectValidatorRequiresRipgrepBeforeValidation` in the new audit test file. Create a temporary `PATH` containing only a symlink named `bash` to the current Bash executable, execute `scripts/godot/validate-project.sh`, and assert:

   ```go
   if err == nil { t.Fatal("validator accepted an environment without rg") }
   if !strings.Contains(output, "missing required executable: rg") {
       t.Fatalf("missing dependency diagnostic:\n%s", output)
   }
   if strings.Contains(output, "validation passed") {
       t.Fatalf("validator reported success after a dependency failure:\n%s", output)
   }
   ```

   Resolve Bash with `exec.LookPath("bash")`; do not assume `/bin/bash`. The script's `/usr/bin/env bash` must still start, while `command -v rg` must fail before any other external command is needed.

2. Run:

   ```bash
   go test ./packages/audit -run '^TestGodotProjectValidatorRequiresRipgrepBeforeValidation$' -count=1
   ```

   Expected baseline result: fail because output is not the required diagnostic or because later commands run before the dependency error.

3. Add the prerequisite check at the prescribed location in `validate-project.sh` with a concise English comment explaining fail-closed ordering.

4. Re-run the prerequisite test; expected: pass with nonzero validator status and no success output.

5. Run existing positive and mutation coverage:

   ```bash
   go test ./packages/audit -run 'TestGodot(ProjectClosure|ProjectValidator|ProjectLayout|ScriptOwnership)' -count=1
   scripts/godot/validate-project.sh
   ```

   Expected: all pass when `rg` is present, and the final command prints `Godot project closure validation passed.` exactly once.

## Closure

- Re-run `rg -n 'validate-project\.sh|godot-project-check' .` and confirm all callers inherit this one prerequisite contract; no caller contains a fallback that ignores failure.
- Run `make test-race-changed RACE_BASE="$task_base"`.
- Commit only the owned files with `fix(godot): fail closed without project validator tools`.
- Rollback unit: this commit alone. Do not restore the fail-open behavior in a later workflow task.
- Report red/green evidence, consumer enumeration, and the commit SHA. The controller updates `tasks.md` and `ledger.md`.
