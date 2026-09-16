# Documentation Guide

## Current facts and historical evidence

- Observable current behavior is determined by code, tests, and `openspec/specs/`.
- `docs/architecture.md` describes the current architecture, and `docs/notes/progress.md` records implementation history.
- `docs/superpowers/` and archived changes are historical evidence. Do not modernize them in bulk or use them to override verified current facts.

## Writing rules

- Long-lived baseline documents state current facts only. Keep change-specific non-goals in the corresponding proposal.
- New or substantively revised explanatory and architectural documents use English in canonical `*.md` files and provide synchronized Simplified Chinese `*.zh.md` counterparts. Unchanged pre-policy documents may remain explicitly classified as `legacy` until their first substantive revision. Active OpenSpec artifacts and new or substantively revised canonical OpenSpec prose, project plans, task briefs, ledgers, and machine governance use English. Historical evidence may retain its original language.
- New source-code comments and source-comment examples use English. Existing non-English comments are grandfathered and need not be translated as unrelated work; new or substantively rewritten comments follow the English rule. Preserve exact external API names, identifiers, commands, and established technical terms.
- Verify that a target exists and a command matches the current implementation before changing links or commands.
- Navigation pages link to authoritative documents instead of duplicating the same explanation across README files, indexes, and history.

## Implementation orchestration

The root provider-aware orchestration policy applies without modification. This scoped guide does not impose an additional subagent count, sequence, or review topology.
