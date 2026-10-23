# Claude handoff — Thought Controller: first NEW ADMINISTRATOR workspace

**Design for an independent runnable prototype, not a built product.** [Netflix-style workspace concept](IDENTITY_WORKSPACE_START_HERE_2026-10-23.md) · [Proposed entity service protocol](ENTITY_REGISTRY_AND_WORKSPACE_2026-10-23.md) · [TC acceptance tests](MULTI_IDENTITY_ACCEPTANCE_2026-10-23.md) · [TC issue #6](https://github.com/Arameair/Thought-Controller/issues/6) · [Identity team's counterpart](https://github.com/Arameair/identity/blob/experiment/fresh-admin-bootstrap-design-20261023/docs/CLAUDE_FIRST_USER_PROTOTYPE_HANDOFF_2026-10-23.md)

## What the new developer experiences

After starting in **Identity**, the administrator enters the main **Thought Controller dashboard**. It should resemble a calm Netflix/Prime library, but with thought cards grouped under individual identities, NOT movie cards. This is the developer's main daily thought-inspection screen. Project Controller remains the separate tool for building software and controlling experimental branches.

**First working visual prototype** (no real AI required):
- Identity profile tiles: TASS (AI companion) and Example Workshop (Business); a separate System Services area for the Thought Controller service identity. A business does NOT require a personality or automatically get thoughts.
- First-time state shows **No thoughts yet**. Test-only **Load sample thought** creates an unmistakably **SIMULATED** thought in a selected profile.
- Start with rows **Continue Thinking, Active, Sleeping/Waiting, Resolved/History**. Do not permanently impose unanswered categories like goals, questions and research.
- Opening a thought shows its origin, timestamps, notes, links, lifecycle changes and worker/call evidence when available. A mocked thought cannot claim a real model responded. Show logs/metrics on request and Copy Diagnostics.
- After creating one TASS thought, switch to Business: zero TASS thoughts visible. Add a Business thought and switch back: TASS's history and count unchanged. Restart with the same sandbox: stable IDs and thoughts. An unrelated sandbox must show different IDs and no inherited data.

## Controller has its OWN identity

Identity provisions one stable controller-service entity ID bound to one installation ID. TASS and the business each have DIFFERENT entity IDs and thought collections. The code is derived from the owner's reference Thought Controller design, but another developer's installation does not inherit the owner's service ID, thought history, account access or permissions. Service identity does NOT require its own thoughts.

For this FIRST experiment only, use a clearly labeled FAKE service handshake/launcher and sample thoughts if actual Identity registry/launcher do not yet exist. Its return shape should include protocol version, explicit SIMULATED status, service identity, installation binding, source full SHA and readiness. Use the SAME test IDs from Identity's onboarding, rather than quietly manufacturing another admin/service identity.

**Important:** old repaired Thought Controller V1 is NOT an approved Alpha and does not have multi-entity authenticated registration or server-side isolation. UI profile switching alone is not authorization. The next separate integration phase must implement authenticated registration, per-entity server-side data isolation, legacy migration with backups and actual service verification; do not mislabel mock evidence as real safety.

## New user path in conjunction with Identity

1. Open new, isolated Identity profile; obtain administrator/installation/controller service IDs.
2. Identity checks missing controller, asks once for automatic startup permission (option B) for a specific pinned test version, then simulates starting the controller through fake launcher.
3. Open **simulated dashboard**, showing TASS and business profiles and correct system identity.
4. Add demo thought to TASS, inspect and switch. Test empty business state; optionally add business thought.
5. Stop/restart both simulated services in the same isolated test profile and verify unchanged IDs/state and no duplicate starts.
6. Show that GitHub is NOT CONNECTED without forcing login. No Chat-Jimmy calls, code modification, actual repo writes or secret storage.
7. Test second clean admin sandbox: distinct service ID and no thought sharing; copy accurate diagnostics.

## Branch and handoff

Review current Alpha, the old design notes and repaired V1 experiment, but branch implementation from current OWNER-APPROVED Alpha. Explicitly propose a traceable port of existing tested behavior if needed—no silent merge from experiment or promotion. Keep Project Controller Play/Stop, PC_BRANCH, PC_DATA, PORT, detached SHA, safe local state and no orphan processes. Preserve previous thought engine safety constraints if integrating later.

Hand back executable branch/full SHA, UI screenshots or copyable context, actual vs simulated service mode, test status PASS/FAIL/BLOCKED/NOT RUN, separate identity counts, lifecycle evidence and rollback. The owner must be able to act like a new user through Identity without terminal fiddling in the normal UI flow.

**Not in this first mock:** real Thought Controller download/updates, OAuth, provider credentials, unauthorized multi-admin hosting, live AI heartbeat, background thinking or automatic Alpha/main changes.