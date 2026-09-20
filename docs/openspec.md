---
doc_id: openspec-workflow
doc_revision: 2026-09-19.1
language: en
counterpart: openspec.zh.md
---
# OpenSpec workflow

Mornlea uses OpenSpec to define change scope, observable requirements, design, implementation tasks, and archival context. Agree on what and how to verify before coding, then preserve stable behavior in canonical specifications.

## Setup

The repository uses the `spec-driven` schema and core profile. Project context and artifact rules are in `openspec/config.yaml`; project rules are in `AGENTS.md`; CI uses `openspec validate --all --strict --no-interactive`.

```bash
npm install -g @fission-ai/openspec@1.7.0
openspec --version
```

## When to use it

Use OpenSpec for new milestones or systems, cross-package features/refactors, protocol/save/concurrency/resource-ownership changes, compatibility or performance-boundary changes, and work expected to last more than two days. Spelling, formatting, and disposable validation scripts may be direct changes.

## Standard flow

Use `explore` when context or requirements are unclear. Proposals must state context, goals, non-goals, observable outcome, affected packages/documents, and compatibility/concurrency/performance impact. Specifications use English normative `SHALL`/`MUST` requirements with decidable Given/When/Then scenarios. Designs describe ownership, dependency direction, boundaries, risks, rollback, rejected alternatives, and validation. Tasks name exact files and commands and remain independently verifiable.

1. Explore relevant code, tests, and history with `$openspec-explore` when requirements are unclear.
2. Propose a focused change with `$openspec-propose <change-name>` containing proposal, delta specs, design, and tasks. Requirements use normative English and decidable Given/When/Then scenarios.
3. Apply with `$openspec-apply-change`, following `tasks.md`, test-first. Main-agent execution is the default; verified OpenAI ChatGPT/Codex has standing delegation authorization. Delegate only for material context isolation, never for parallel speed or unused capacity, and run at most three concurrent subagents. Non-OpenAI or unknown providers use strict `subagent-driven-development` with independent implementation and review.
4. Validate and archive with `$openspec-archive-change` after implementation matches the specifications:

```bash
openspec status --change <change-name>
openspec validate --all --strict --no-interactive
```

Use `$openspec-sync-specs` when delta specifications must be promoted before archival. At the end of each implementation round, promote stable architectural findings to `mornlea-architecture`, or record `Architecture skill: no change`.

Archival merges delta requirements into `openspec/specs/<capability>/spec.md` and moves the complete change to `openspec/changes/archive/`. Do not bulk-rewrite historical changes or plans.

## Daily commands

```bash
openspec list
openspec list --specs
openspec status --change <change-name>
openspec show <change-name>
openspec validate --all --strict --no-interactive
openspec doctor
```

Active and canonical planning artifacts are English. Current explanatory documents use an English `*.md` canonical file with synchronized Chinese `*.zh.md` counterpart. Archived changes remain historical evidence.

## Hook status

Automatic Claude Code and Codex Hooks were removed. The maintained implementation and tests are `scripts/agent-hooks/guard.mjs` and `node --test scripts/agent-hooks/guard.test.mjs`; do not describe installed Hook configurations.
