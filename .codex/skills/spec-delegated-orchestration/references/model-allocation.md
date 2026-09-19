# Model Allocation Guide

This guide gives the main model a consistent default allocation for the candidate roster supplied by the user. It is a planning heuristic, not a claim about provider quality or a guarantee that a host exposes every option.

## Reasoning Bands

Use one of these semantic bands in assignments.yaml. If the execution host has exact effort names, record the host value in resolved_reasoning as well.

| Band | Meaning | Typical host value |
| --- | --- | --- |
| minimal | deterministic extraction, formatting, classification, or a trivial check | none or minimal |
| low | narrow, low-risk work with an obvious answer or strong oracle | low |
| medium | ordinary implementation, synthesis, or multi-step investigation | medium |
| high | ambiguity, interacting constraints, or an independent review | high or xhigh |
| max | the hardest decisions, weak evidence, or high-consequence synthesis | max or the host's highest permitted value |

Host values are examples. The main model must not invent an unsupported effort enum.

## Default Allocation Table

| Task shape | Default model | Default reasoning | Notes |
| --- | --- | --- | --- |
| Mechanical extraction, formatting, small fixture, or narrow lookup | GLM 5.3 Flash | minimal or low | Use when the result is easy for the main model to recognize and reuse. |
| Bounded ordinary implementation or straightforward debugging | GLM 5.3 | medium | Keep the task's scope narrow and independently addressable. |
| Repository exploration, documentation synthesis, alternative proposals, or a second perspective | Muse spark | low or medium | Assign conclusions for the main model to assess, not final contract authority. |
| Spec interpretation, cross-task synthesis, contract review, or high-consequence supporting analysis | a separately launched ChatGPT worker model | high or max | The current main model performs final arbitration; never assign the controller to itself. |
| Long-context investigation, complex bounded implementation, adversarial review, or cross-package issue analysis | Grok 4.6 | high | Use only when the execution environment actually exposes this candidate. |

These defaults are cost-oriented: select the least expensive listed model and band that can plausibly produce a useful result. Do not lower the assignment for architecture, protocol, save, ABI, security, or other durable contract work merely to save tokens; assign supporting analysis to a worker and reserve the final decision for the main model.

The allocation unit is a coherent work package, not necessarily one checkbox. Group adjacent tasks when they share ownership, context, dependencies, and validation; split them when independent failure recovery, review, or lifecycle boundaries make a separate handoff materially useful. Record every covered leaf task in the assignment's `covers` list so coarse allocation never loses traceability.

## Allocation Procedure

For each decomposed task, the main model records:

1. the task's complexity and consequence;
2. the amount of context and independent reasoning it needs;
3. the strength of the answer-checking oracle available to the main model;
4. the selected user-facing model label and reasoning band;
5. an optional fallback model/band if the selected route is unavailable.

Choose the lowest band that covers the first three factors. Raise the band when the task is ambiguous, cross-cutting, weakly checkable, or costly to redo. Keep the main model's assignment explicit even when it is the default owner.

## Assignment Example

    tasks:
      - id: block-contract
        title: extract and reconcile the current packet-field contract
        covers: ["4.1", "4.2", "4.3"]
        model: GLM 5.3 Flash
        reasoning: low
        result_dir: delegation/results/block-contract/
      - id: block-design
        title: compare two bounded implementation approaches
        covers: ["5.1", "5.2"]
        model: Muse spark
        reasoning: medium
        result_dir: delegation/results/block-design/
      - id: block-review
        title: identify contract conflicts for controller review
        covers: ["6.1", "6.2", "6.3"]
        model: ChatGPT worker
        reasoning: high
        result_dir: delegation/results/block-review/

ChatGPT worker is a human-readable placeholder for the concrete separately launched ChatGPT model selected by the host; replace it with the actual user-approved label in a real assignment. The main model should preserve the distinction between the requested label and any host-resolved identifier.
