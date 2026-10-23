# Thought Controller's relationship to Identity and the new Thought module

**Owner design update, 2026-10-23 — not implemented.** This note supersedes earlier architectural wording that Thought Controller IS the thinking capability or that simply having an entity automatically grants thinking. [Identity card / feature grant decision](https://github.com/Arameair/identity/blob/experiment/fresh-admin-bootstrap-design-20261023/docs/FEATURE_CARDS_AND_THOUGHT_MODULE_2026-10-23.md) · [New Thought repo plan](https://github.com/Arameair/identity/blob/experiment/fresh-admin-bootstrap-design-20261023/docs/THOUGHT_REPOSITORY_PLAN_2026-10-23.md) · [existing dashboard spec](IDENTITY_WORKSPACE_START_HERE_2026-10-23.md).

## Clean three-way separation

**Identity** = central account + GitHub controller, secure permission authority, persistent ID cards, feature catalog and grants.

**Thought** = a separate proposed **Arameair/Thought** repository and registered system-feature identity. It does the actual Spark → Trace → Thought work for identities that have Thought Processing explicitly enabled and authorized.

**Thought Controller** = visual developer dashboard and operational/testing controls, Netflix/Prime-style thought collections organized by the selected owning identity. It does not run a second account manager or independently grant abilities to other entities. It observes Thought activity via authorized, versioned interfaces and may request owner-approved processing actions.

An identity like Example Workshop that has not been granted Thought Processing **can exist, own resources, use other capabilities and appear on the dashboard, but MUST NOT generate sparks, traces or thoughts**. It shows “Thought Processing not enabled” with an action to request enablement from Identity rather than processing a prompt anyway.

## System cards vs records

The local Identity installation has its own **System ID Card** with available features. Thought is the first installable/default-available feature *only when actually verified*. The Thought module is itself a stable **service identity**, not TASS's ID and not a thought record. “Default available” is not “default enabled on all identities.”

Do not give an individual spark/trace/thought its own full Identity card: those are small records owned by the selected authorized entity, stored by Thought. One thought may relate to many traces but may never silently cross another entity's authorization boundary.

The reference Thought Controller and Thought code repositories are **origin/provenance for new installations**, not the actual system card or an authorized user's GitHub account. Installations get new independent IDs, grants, data, and modifications.

## Proposed dashboard behavior

- Profile rail: TASS (Thought enabled), Business (Thought disabled), System Services (Thought module instance/health). These are EXAMPLE cards; actual UI must use verified status and ID.
- For each enabled identity, show **Sparks (brief activity)**, **Traces (recent evidence/residue)** and **Thoughts** with Continue Thinking / Active / Sleeping / Resolved sections. First prototype may use just an activity strip plus thought cards; no need to animate thousands of sparks or invent unobservable thought contents.
- Clicking Thought displays contributing trace IDs, their original event references, state changes, reinforcement, wake reasons, worker provenance, accepted/rejected actions and errors.
- Clicking a trace can show source/reference and linked thought if access permitted. Privacy-aware retention; don't keep raw chat/screen contents in logs by default.
- Disabled identity: no Think Once action, no Create Spark, no worker prompt; instead display **Enable Thought Processing on ID Card** (opens Identity request/feature management).
- “Enable” action on card makes Identity run install/verify/approval/grant/register checks; Thought Controller reads resulting status, not granting it unilaterally.
- Thought Controller's startup/identity health is distinct from Thought module health, worker health and GitHub availability. Each shows actual state (installed/enabled/ready/offline/simulated/denied), not one generic green badge.

## Contracts the teams should discuss

Identity issues narrowly scoped capability grants with installation_id, entity_id, feature ID (thought.processing), actions, policy/version, expiry and revocation. Thought service validates them at every read/write/action boundary. Thought Controller may get read/manage rights for an identity only as authorized. In first mocked prototypes, grants and service calls are explicit **SIMULATED** and should be testable as deny/allow/revoke, but do not claim secure live integration.

One clear event can create a momentary spark; some sparks create traces; related traces may promote to a sustained thought. Thresholds/TTL/quota are provisional and adjustable; tests determine what helps. A live fast-model worker is LATER and cannot be called just because the dashboard opens.

The original Thought Controller repaired V1 has some thought state and worker logic, but **no approved standalone Thought module, no real Identity grant broker, no verified cross-identity data isolation**. Do not silently port or merge experimental code; negotiate an adapter, and explicitly choose what code migrates from controller to new Thought repo.

## Immediate prototype acceptance

1. Identity card shows installed/available Thought feature (simulated), but TASS and Business initially disabled.
2. Simulate enabling for TASS through Identity; its grant authorizes one spark→trace→thought stream. Business remains empty and rejected at the mocked service boundary.
3. Controller shows the correct TASS trace trail and why thought was formed. It cannot fake an authorization by switching profiles.
4. Switch between TASS and Business; never leak records or worker runs. Enable Business separately if testing isolation.
5. Disable TASS and verify new processing stops. Restore and confirm same entity IDs, stored evidence and grant/revoke state in the same test sandbox.
6. System Services shows separate Thought service ID and source/health, not just the dashboard's process PID.
7. Copied diagnostics label MOCK vs LIVE, feature grant decision, scope, counts, dropped traces, errors and full branch/SHA. No GitHub credentials or raw private content.

This is the **next design direction** for Claude if it is currently coding the earlier two-repo new-user prototype: finish/checkpoint existing independently testable work, review scope changes, and discuss a subsequent explicit Thought-module experiment. Do not silently rewrite all three apps or promote Alpha.
