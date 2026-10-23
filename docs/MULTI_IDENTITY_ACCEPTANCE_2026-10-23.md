# Thought Controller — developer dashboard and isolation acceptance

[← Overview](IDENTITY_WORKSPACE_START_HERE_2026-10-23.md) · [Service contract](ENTITY_REGISTRY_AND_WORKSPACE_2026-10-23.md) · [Fresh-admin whole-system tests](https://github.com/Arameair/identity/blob/experiment/fresh-admin-bootstrap-design-20261023/docs/NEW_ADMIN_TEST_MATRIX_2026-10-23.md)

| Check | Action | PASS |
| --- | --- | --- |
| T1 | Start a test Thought Controller independent of Identity | Page accessible; current branch/full SHA, service identity/provenance and local data location visible; actual runtime state, not fabricated 'ready' |
| T2 | Register administrator-installation and controller service identity | Correct distinct, stable IDs; no duplicate on repeat/restart; require actual authenticated contract when moving past mocks |
| T3 | Register TASS + Example Workshop | Two distinct scoped collections, business not required to have a personality or start thinking |
| T4 | Add test thought to TASS; browse both identity profiles | TASS tile shows thought, business empty state; both persist and selecting either shows correct counts |
| T5 | Add a different thought to business | Business shows only its thought; TASS shows only its own |
| T6 | Try direct API read/mutate/export with the wrong identity ID or auth | Denied (never silently select first/default); no cross-tenant data/trace exposure |
| T7 | Think Once in TASS, then change selected profile | Response and metrics remain bound to TASS even after UI selection changes; no silent bleed or duplicated worker request |
| T8 | Stale, unreadable, not-ready and Stop cases | Existing V1 repair semantics retained, no unsafe retries or mislabeled applied thoughts |
| T9 | Stop/restart Thought Controller and Identity | Same controller service ID and entity IDs, thoughts, timestamps and histories; no orphan process |
| T10 | Change code version or use local fork | Local data survives with validated migration/rollback, origin/version displayed; no unauthorized upstream changes |
| T11 | No GitHub, offline or no worker | Usable dashboard and honest statuses/empty states; zero surprise provider operations |
| T12 | Fresh admin install from same approved reference | Controller code/provenance may match; **service IDs, thought data and GitHub permissions differ** |
| T13 | Open detail and Copy Diagnostics | Real origin, notes, relationships, source/provenance, exact model calls and timestamps, no passwords/tokens or unrequested private text |
| T14 | Start/stop through approved launcher or Project Controller | Owned lifecycle only, no double-start, proper readiness and no unowned process |
| T15 | Test different branches in Project Controller | Correct branch, pinned SHA and distinct test data; cannot overwrite an approved persistent identity without explicit action |

**First runnable prototype target:** T1 (mock version), T3–T5 using mock thoughts and clear simulated labels, and the empty-state portion of T11. Then T2/T6 and real persistence/handshake. Then end-to-end T7–T15. Each line requires **PASS / FAIL / BLOCKED / NOT RUN**, with full SHA and test evidence.

**No promotion before owner review.** Simulation/one browser fake site is not evidence of live Chat-Jimmy thought quality; real site access must be separately authorized. No automatic worker prompts or background thinking are necessary for these structural tests.