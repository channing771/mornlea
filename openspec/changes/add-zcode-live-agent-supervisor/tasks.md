## 1. Protocol Adapter

- [x] 1.1 Add failing fixture tests in `.codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs` for app-server framing, capability handshake, session creation/subscription, v4 intervention delivery, stop, malformed frames, unsupported methods, child exit, sequence regression, and credential redaction; validate with `node --test .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs`.
- [x] 1.2 Implement the bounded stdio protocol adapter and normalized event model in `.codex/skills/adaptive-model-router/scripts/zcode-live-agent.mjs` until `node --test .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs` passes.

## 2. Persistent Supervisor Lifecycle

- [x] 2.1 Add failing tests in `.codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs` for private runtime paths, on-demand daemon startup, stable worker IDs, editing ownership assertions, cursor-ordered waits, delta coalescing, timeout snapshots, stop, close, stale metadata, and conservative recovery; validate with `node --test .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs`.
- [x] 2.2 Implement the Unix-socket supervisor, per-worker app-server ownership, bounded event ring, wait registration, sanitized metadata, and cleanup in `.codex/skills/adaptive-model-router/scripts/zcode-live-agent.mjs` until `node --test .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs` passes.

## 3. Public Bridge Commands and Compatibility

- [x] 3.1 Add failing integration tests in `.codex/skills/adaptive-model-router/scripts/zcode-agent.test.mjs` for `start`, `wait`, `status`, `steer`, `stop`, and `close`, including machine-readable errors and preservation of the existing `probe`, `run`, and `send` shapes; validate with `node --test .codex/skills/adaptive-model-router/scripts/zcode-agent.test.mjs .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs`.
- [x] 3.2 Extend the skill-local `scripts/zcode-agent.mjs` with live command parsing and supervisor client calls, then validate with the focused Node suites in the selected skill package.
- [x] 3.3 Run a side-effect-free compatibility smoke check against the installed CLI with the selected skill's `scripts/zcode-agent.mjs probe` and live handshake diagnostic, recording the installed identity and result in `openspec/changes/add-zcode-live-agent-supervisor/ledger.md` without issuing a model prompt.

## 4. Routing Policy

- [x] 4.1 Add the validation-aware scoring rubric, conservative `GLM-5.3` capability prior, live command workflow, progress-filtering rules, and high-risk fallback policy to both `.codex/skills/adaptive-model-router/` and `.claude/skills/adaptive-model-router/`; validate with the skill-local focused Node suites, `cmp` for every synchronized router file, and `go test ./packages/audit -run 'Test.*Orchestration|Test.*Governance' -count=1`.

## 5. Review and Closeout

- [x] 5.1 Review the implementation against every scenario in `openspec/changes/add-zcode-live-agent-supervisor/specs/zcode-live-agent-supervision/spec.md`, record review rulings and focused validation in `ledger.md`, and rerun the skill-local focused Node suites.
- [x] 5.2 Format changed Node sources with the repository-available formatter if configured, confirm `git diff --check`, run `make dev-check`, and record the commands and results in `ledger.md`.
- [x] 5.3 Run the six-module race gate with `make test-race` and strict specification validation with `openspec validate --all --strict --no-interactive`, recording exact results in `ledger.md`.
- [x] 5.4 Perform the round-end architecture and router retrospectives, synchronize any verified reusable policy updates, or record `Architecture skill: no change` and `Model router: no change` with reasons in `ledger.md`.

## 6. Skill Runtime Packaging

- [x] 6.1 Add a failing architecture audit that requires the bridge, supervisor, and focused tests under both synchronized `adaptive-model-router/scripts/` directories, requires those files to remain byte-identical, and rejects root `scripts/agents/zcode-*.mjs` copies; validate the expected red state with `go test ./packages/audit -run 'TestProjectAdaptiveModelRouterSkillsMatch|TestProjectRouterOwnsZCodeBridgeRuntime' -count=1`.
- [x] 6.2 Move the four Z Code runtime/test files into both skill packages, update router instructions and maintained references to resolve the selected skill root, and validate with the focused Node suites, both skill validators, and the focused audit.
- [x] 6.3 Re-run the inference-free installed CLI probes, strict OpenSpec validation, and scoped diff checks; record the migration and round-end rulings in `ledger.md` before creating the user-requested scoped Git commit.
