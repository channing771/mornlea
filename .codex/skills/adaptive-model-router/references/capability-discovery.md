# Capability discovery

Use this reference only when model availability and reasoning-effort support cannot be read from one authoritative native response.

## Source priority

Treat the invocation boundary as the source of truth for what can run now:

1. Inspect native `spawn`, `delegate`, `worker`, `agent`, or model-request tool metadata. Enumerated model and effort values are authoritative for that call surface.
2. For the repository-owned Z Code worker, resolve the selected `adaptive-model-router` skill root and run its packaged `scripts/zcode-agent.mjs probe`. Before a live route, also run the same skill-local executable with `live-probe --cwd /absolute/worktree` so private app-server protocol drift fails before inference. These redacted results are authoritative only for the configured `GLM-5.3` external surface; they do not extend the native model enum.
3. Call a read-only capability or model inventory operation if another host provides one.
4. If a provider's list-models response contains only identifiers, consult current official documentation or machine-readable model metadata for reasoning effort, modalities, context, and tool support.
5. Use local CLI help or a dry-run command only when it is provider-supported and does not modify configuration or incur inference cost.

Never scrape account secrets, print credentials, or use an unrelated account to fill gaps. Community tables and remembered model-family behavior are hints for where to look, not authoritative capability evidence.

## High-level design gate

Classify architecture, feature-design, contract, ownership, lifecycle, concurrency, and other durable multi-component tasks before quota scoring. These tasks MUST use the highest eligible native OpenAI model under the project ceiling with `high` or `max` reasoning. If no compliant advanced OpenAI configuration is exposed, report an unavailable route; do not substitute Z Code or another non-OpenAI backend. Ambiguous tasks that could change durable boundaries are classified as high-level.

## Quota discovery

Quota is a routing signal separate from model capability. Apply it only after the high-level gate and local-time filter. During the host's local `14:00–18:00` disabled window, omit Z Code before scoring. Outside that window, prefer a current, read-only usage snapshot exposed by the provider or the Z Code desktop host. If that surface is unavailable, accept a non-secret snapshot supplied by the host with `used`, `limit`, `remaining`, and `observed_at`; an optional `reset_at` helps decide when a depleted entitlement can be retried. Keep the raw response and credentials out of task output.

Treat a snapshot as unknown when it is malformed, older than 15 minutes, or has no positive `limit`. Unknown quota uses the neutral routing value rather than zero. A confirmed zero remaining quota or a provider rate-limit response is different: it temporarily removes Z Code from the candidate set until reset or a fresh positive snapshot. Quota discovery must remain read-only and must not send a paid model request.

## Normalize heterogeneous hosts

Build a temporary record for each available option:

```yaml
model_id: exact-provider-identifier
availability_source: native-tool | provider-api | local-cli | official-docs
backend: native | zcode-cli | other-external
reasoning:
  values: [exact, supported, labels]
  default: exact-label-or-unknown
  omission: inherit | provider-default | unknown
capabilities:
  modalities: []
  tools: []
  context_window: unknown
  max_output: unknown
constraints:
  regions: []
  service_tiers: []
  policy_ceiling:
    model: none
    reasoning_effort: none
signals:
  capability: unknown
  latency: unknown
  cost: unknown
```

Omit unsupported fields or mark them `unknown`; do not manufacture comparable numeric scores from qualitative marketing descriptions.

## Reconcile conflicting sources

- Native runtime rejection or a current invocation schema overrides general documentation for immediate availability.
- A successful Z Code bridge probe proves only that bridge's external worker; it never proves native delegation support.
- A successful provider probe does not prove live supervision compatibility; the installed app-server must also pass the bridge's behavioral contract check.
- Account-scoped inventory overrides a public catalog for access.
- Official per-model documentation overrides family-name inference for reasoning-effort support.
- A product-specific policy ceiling filters otherwise available options; availability never overrides the ceiling.
- When two current authoritative sources conflict, avoid the disputed option unless the user explicitly wants a safe, authorized probe.

## No discovery interface

If the host has no model inventory, no provider documentation access, and no override fields, retain the current or inherited configuration and label it as such. The router must not claim it selected the best model.

If override fields exist but their accepted values are undiscoverable, use a user-specified exact value when available. Otherwise, keep the default rather than guessing and causing a failed or billable invocation.

## Example judgments

- A tool schema lists four models and effort values per model: use that schema; do not browse for a larger public catalog the tool cannot invoke.
- A `/models` API lists identifiers but no reasoning metadata: intersect the returned identifiers with current official capability documentation.
- A local agent exposes only presets such as `fast`, `balanced`, and `deep`: treat these as atomic configurations rather than pretending model and effort were chosen independently.
- A child worker inherits the parent unless an override is supplied: omission is inheritance, not adaptive routing. Supply an override only when the task justifies it and the host accepts it.
- Any host, API, or gateway exposes an OpenAI model above `gpt-5.6-sol` or an effort above `max`: record the option as available but ineligible under the preset OpenAI ceiling, then select downward.
