# Thought Controller — V1 repair notes and first real-worker test

**Date:** 2026-10-23  
**Document status:** proposed for owner and team review; **no code or tests changed in this branch**.  
**Source being reviewed:** [First Thoughts V1](https://github.com/Arameair/Thought-Controller/tree/experiment/first-thoughts-v1-20261023) @ `daaa0c3cb4f58aa0a4f7fdf83f86f0946c04f6e4`.  
**Follow progress and report evidence:** [Issue #3 — repair and acceptance](https://github.com/Arameair/Thought-Controller/issues/3).  
**Next project after approval and the first working live test:** Identity.

## Goal, in plain language

Make one real thought through an **already-running** Chat-Jimmy worker **reliable, observable, and safe**. Then demonstrate that the thought can be revisited and changed without losing its history. We are deliberately postponing continuous autonomous thinking, memory, personality, curiosity, GitHub/Discord access and other modules.

A complete workflow we should eventually *see*, not merely claim:

1. Owner launches Thought Controller from Project Controller, which names the exact run and its data folder.
2. Owner sees which worker is connected and whether it is **actually ready**, with MOCK / FAKE SITE / VERIFIED LIVE SITE provenance where known.
3. Owner adds an event, manually presses Think once, and sees the **exact context sent, model output, accepted/rejected proposals, state changes, elapsed time, and current thought record**.
4. Owner manually continues that thought. The system remembers what happened between beats.
5. Owner changes information while a call is running. The earlier answer is marked **STALE** and cannot overwrite the newer state.
6. Owner stops and restarts; the stored thought remains, and a trace can explain what happened. No secret is exposed by a diagnostic report.

## Current evidence and limits

The team reports **14 Go tests passing** and a full integration path through the **real Chat-Jimmy worker and a fake chat website** (~205 ms per beat in that environment). This is useful protocol/integration evidence. It does **not** demonstrate behavior, format compliance, or latency on the actual chatjimmy.ai site. No Thought Controller v1 live-site success has been verified here.

Current V1 already has valuable foundations: event revisions; a single request per beat; active/dormant/resolved/expired thought records; parent links; follow-up limits; visible timeline; an opt-in `Would say (never sent)` list; persisted state; mock worker; and manual controls.

**Avoid redesigning those parts prematurely.** First close the trust/reliability gaps and gather actual evidence.

## Repair order

| Priority | What must be repaired | What we should see as proof |
| --- | --- | --- |
| **P0** | In-flight safety: Reset/worker switching/manual state/settings during single Think once and during Run 5/20; Stop and cancellation behavior | One accepted request or explicit 409; event updates mark obsolete replies stale; no late or obsolete result applied |
| **P0** | Unreadable heartbeat output must not silently consume new events | Malformed reply clearly reported; event remains pending; next deliberately requested beat can process it |
| **P0** | Worker readiness and simulation provenance | `ready=false` not treated as connected; fake or simulated replies never presented as real-site evidence |
| **P0** | Persistence and branch isolation | Disk/rename errors surfaced without panic or false saved claim; unique data folders per experimental run; restart resumes reliably |
| **P1** | Trace/data access and redaction | Local reads are access-controlled appropriately; full prompts/replies protected; diagnostics redact secrets and identify unknowns |
| **P1** | Clear model-quality vs transport-quality measurements | Separate successful send, parse, valid proposal, meaningful conclusion, and saved state; show unreadable/rejected/stale counters |
| **P2** | Thought quality, lifecycle tuning and repeated heartbeat | Only after basic live-model capability is measured; compare against a one-prompt baseline, keep budgets |

### Why the P0 findings matter

**In-flight mutation:** `server.go` blocks Reset and worker switches while `run.running` is true, but `/api/step` does not use that run flag. Manual thought state changes and settings do not bump the event revision. This permits an old model result to apply after a manual change unless the server either explicitly blocks those operations during a beat or invalidates that beat. Keep adding a new **event** possible during an in-flight beat, because testing that stale reply is essential.

**Unprocessed events:** `mind.go` sets `LastSeenRev=p.rev` for a nonstale heartbeat, even when the reply is malformed. In that case a user event may disappear from further heartbeat context. Retain the event and show **protocol/unreadable — pending**, without automatically retrying an uncertain send.

**Readiness and provenance:** `internal/chatjimmy/chatjimmy.go` does not require the heartbeat's ready flag to be true in `Find()`. `WorkerReply` in `worker.go` omits the underlying Chat-Jimmy `simulated` flag. Thus a mock/fake response can be reported as a nonsimulated Chat-Jimmy beat. Preserve provenance throughout the run and test false readiness.

**Storage:** `store.go` ignores write/rename failures; after a failed trace rotation `tf` may be nil. A realistic test injects file-system failures and verifies graceful recovery. `main.go` defaults to shared `~/.thought-controller`; use the `-data` flag or equivalent per-workspace isolation so comparing two branches never mutates one test's state.

**Read access:** `server.go` applies the optional token check to POST endpoints, not the GET state/trace endpoints; those may expose full prompts, replies, and user events. Before attaching personal or business entities, choose an appropriate local authorization model and enforce redaction/retention. A private GitHub repository is **not** a credentials vault.

### Also important, but not first-gate blockers

- Thought lifetimes are currently measured in **beats**, not wall-clock elapsed time. The system is **manual**, not continuously thinking; retain that truthful label.
- A `NEW` question and a `RESOLVE` line are *proposals from a model*, not independently verified conclusions.
- The basic duplicate similarity function is heuristic. Test false duplicates and missed near-duplicates; don't promote a repeated guess into a fact.
- Keep only one active Chat-Jimmy request per worker; preserve `busy`, `blocked`, `unavailable` and `uncertain` as separate failure categories. Never silently replay uncertain requests.
- An in-flight request can finish after the owner presses Stop. Decide and test a clear rule about discarding or recording that reply before the next live test.
- Preserve Project Controller's independence: it owns Play/Stop of programs; Thought Controller owns **thought** lifecycle. Worker readiness != answer received != useful thought.

## Acceptance walk-through (manual, owner-visible)

**A. Isolated mock** — fresh `-data` path, worker=mock. Add `I entered Goldshire again.`. Run **Think once**, then 2–3 more manual beats. Inspect created thought IDs, causes, notes, transitions and trace. Result must say `SIMULATED`.

**B. Worker check** — independently Play the [fast-clean Chat-Jimmy branch](https://github.com/Arameair/chat-jimmy/tree/experiment/fast-clean-worker-20261023). In Thought Controller, select it and verify **ready** without sending a question. If not truly ready, stop here and capture diagnostics.

**C. Live-site test** — only after applicable authorization to use the provider for this test. Add the same event and press **Think once** (avoid `Run 20` initially). Inspect raw reply, recognized commands, errors and duration. Repeat only manually after reviewing the prior trace.

**D. Development quality** — follow an active thought for a few beats. Does it add relevant information, detect missing evidence, resolve appropriately, or simply repeat? Compare with the same question answered in one simple Chat-Jimmy request. Record latency and call count.

**E. Changes mid-thought** — use a deliberately slow fake worker first, inject a newer event during the request, and confirm old text is discarded. Only after that, try owner-provided input during a real request. Also test Reset/Stop while a single step runs.

**F. Recovery** — Stop, restart same exact branch/SHA with same isolated data folder. The thought records and safe trace should reappear unchanged. There must be no orphan processes or unexpected sends.

**Owner report:** chosen branch/SHA, Chat-Jimmy branch/SHA, mode (mock/fake-site/real-site), test data dir, number of calls, prompt/reply sizes, durations, meaningful outcomes, failure categories, context/trace capture, and **PASS / FAIL / BLOCKED / UNVERIFIED**. Preserve exact evidence, avoid automatic promotion.

## How to make progress visible in the UI

Keep the calm workspace view. In the top header show **worker + target provenance**, **ready/blocked**, **active run / stopped**, **current context revision** and **thought count**. Each thought tile should show **ID, state, question, source/parent, last changed, work count**, and a way to inspect **all notes and why it changed**. A timeline entry should show **what caused a beat, the model request ID, the exact instructions supplied, accepted/rejected proposals and the result classification**.

Add a **Copy diagnostic / Copy thought context** control after safety/redaction is verified, patterned on Project Controller. Don't show a green `PASS` merely because HTTP answered or a model returned some text.

## After acceptance: the next puzzle piece is Identity, not another thinking feature

Identity should be an **independent reusable entity registry and account setup system**, not an AI-personality module. An entity could be a companion, person, organization/business, project or other participant.

The intended first Identity experiment:

1. On first launch, create a new entity or restore one from local persistent state. Maintain a stable unique ID across restarts and software branches.
2. Guide the owner to an existing GitHub account or GitHub's **official signup page**; guide creation/selection of a private GitHub repository as that entity's configuration home. Don't require a separate GitHub login for every entity.
3. Store **no account passwords, MFA codes or provider tokens in GitHub**. Use protected local credential storage, limited authorizations and explicit user approval.
4. Discover an already-running Thought Controller and request registration. If absent, offer an **approved** launch via Project Controller or another clear lifecycle owner; no covert launches.
5. Identity registers its **entity ID, capability handles, permissions and resource locators**, not raw credentials. Later Thought Controller can request an account connection; Identity owns the authorization UI and supplies a scoped capability to the requesting entity.
6. Keep memory, curiosity, personality, perception and communication separate optional components; the entity's repository can record nonsecret installation/configuration references as pieces are connected.

**Not in the current repair scope:** implementation of Identity, registration APIs, Discord, GitHub actions, continuous live heartbeat, autonomous code edits, or broad account permissions.

## Branch and review rules

`alpha` is still the bootstrap baseline; V1 is on an experiment built from `experiment/design-foundation-20261023`. **No one should pretend V1 is Alpha.** Before starting a fixing-code branch, let the owner choose the safe route that preserves the policy to work from **latest approved alpha** (after accepting a verified baseline) or expressly approve an isolated V1-derived correction experiment.

This document is **notes only**. The next team should link each repair PR back to [Issue #3](https://github.com/Arameair/Thought-Controller/issues/3) and provide verified tests, trace evidence, rollback instructions and unknowns.
