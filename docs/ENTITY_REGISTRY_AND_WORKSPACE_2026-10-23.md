# Entity-scoped Thought Controller — service identity and registration contract

**Design, not implemented.** [User experience](IDENTITY_WORKSPACE_START_HERE_2026-10-23.md) · [Tests](MULTI_IDENTITY_ACCEPTANCE_2026-10-23.md) · [Identity bootstrap design](https://github.com/Arameair/identity/blob/experiment/fresh-admin-bootstrap-design-20261023/docs/NEW_ADMIN_BOOTSTRAP_2026-10-23.md)

## Separation of identities

One Identity installation has:
- administrator principal/entity ID — sets permissions and consents
- installation ID — binds local trust and durable state
- Thought Controller **service entity ID** — names the controller and its verified instance, *not* the identity whose thoughts are being processed
- managed entity IDs — TASS, Example Workshop, others, each with isolated thought/event/trace namespaces

The reference Thought Controller repo supplies **code/provenance**, not a shared ID, GitHub token, thought database or permission grant. Installing the same code elsewhere creates a DIFFERENT service ID. A fork can diverge while selectively adopting later approved improvements.

## Minimal negotiated contract — version 1 proposal, not live routes

These are examples for the teams to agree on, **not claims that existing V1 supports them**.

- GET health: returns service name, protocol version, real ready/blocked status, running build origin/ref/full SHA, supported features, safe actual service instance identity and installation binding. Health must not send an AI prompt.
- POST authenticated entity registration: takes request_id, entity_id, owner_installation_id, type and approved registration proof/capabilities. Repeat registrations for same entity return ALREADY_REGISTERED, not a duplicated thought namespace; wrong owner/type is DENIED/CONFLICT.
- GET scoped entity list and per-entity status: only identities the caller is authorized to inspect. No global anonymous list of all entities, no tokens in responses.
- Create/view/act on thoughts: a verified entity scope is REQUIRED on **all** entry points, including status, trace export, diagnostics, baseline and manual controls. Reject cross-identity reads/writes; no fallback to a global default.
- Capability request: entity_id, desired capability, action and purpose are sent to Identity for approval. A model thought may request, not grant. Only Identity returns a limited handle/denial; controller never receives raw GitHub/Discord credentials.

**Compatibility negotiation:** versioned requests/responses; INCOMPATIBLE is a valid honest status. Read-only discovery and identity verification come before registration. A controller seen at an unexpected localhost port is not trusted merely because it returns HTTP 200.

## Data: schema and migration

Each event, thought, note, relationship, worker call and trace row belongs to exactly one entity_id and one authorized installation. Store/backup/delete/extract by that boundary. No third party can read TASS's thoughts by selecting Example Workshop. Distinguish the stable service identity from process PID and selected code branch.

**Legacy V1 records** are not automatically assigned to TASS or a service identity. The older single-namespace state may need an owner-confirmed **import/migration** with backup, explicit target entity, dry-run counts and rollback. Isolated experiments must not alter the original state files. Storage failures may never be reported as saved.

Each entity is allowed different model/worker configuration and later permissions, but first prototype can share one independent worker process as long as requests are isolated, serialized as necessary and never misattributed.

## Dashboard minimum

1. Identity rail/profile tiles: display name, type, current status, number of active/dormant/resolved thoughts. **System Services** in separate administrative view, not assumed to have ordinary thoughts.
2. For selected authorized identity: 'Continue Thinking', 'Active', 'Sleeping/Waiting', 'Resolved/History'. Avoid speculative goal/category tagging at first.
3. Thought tile opens actual source, timestamps, notes, links, state history, model provenance, accepted/rejected/stale, diagnostics and owner actions.
4. No-thoughts/worker-not-ready/errors have truthful empty/error UI; actual counts and metrics on demand.
5. One expanded thought view at a time, stable selection, calm scrolling/cards and normal developer-readable logs and copy context.
6. Protect reads, exports, cross-entity calls and control endpoints. Identity authorization cannot be bypassed by requesting another ID or modifying a front-end selector.

## Prototype handoffs

**TC Gate 0: mocked collection UI.** From owner-approved Alpha on a NEW experiment branch (not the unpromoted repaired V1 without approval), build a small standalone mock-profile browser showing separate TASS and business thought collections. No forced model calls, no GitHub, no registration claim. Exercise selected identity and empty state, inspect, logs and metrics.

**TC Gate 1: actual service interface.** Once the owner accepts UI, implement authenticated local service handshake and idempotent registration with an Identity fake adapter, persistent entity-scoped records and tests that unauthorized IDs cannot read/write/act. Integrate with the existing thought engine after agreeing on a safe path from the unpromoted repaired V1; preserve all its regression tests and manual owner controls.

**Integration Gate 2:** connect real Identity prototype, real Thought Controller and trusted launch path. Confirm no duplicate startup, service ID stable, admin identity independent, restart recovery and accurate protocol/version. GitHub account management remains in Identity and is a separate gated test.

**Never claim multi-entity security from UI separation alone.** Verify server-side checks, auth, storage isolation and trace/export paths.

## Acceptance evidence

Hand back exact repo/branch/SHA, data folder per entity, protocol version, registered IDs, launch owner, selected identity, local worker mode, mock vs real evidence, operation counts, independent lifecycle, every denial test and safe copied diagnostics. Owner makes Alpha promotion decision. No automatic merging, installation of newer upstream experiments, external GitHub writes, or undisclosed API calls.