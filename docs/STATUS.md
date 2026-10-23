# Project status and experiment journal

**Last reviewed:** 2026-10-23 (design preparation)  
**Overall:** 🟢 v1 repair prototype on `experiment/v1-repair-20261023` — owner-triggered beats only; P0 fixed and tested here.  
**Owner approval:** pending (owner asked for a first version to start testing, 2026-10-23).  
**Next decision:** owner's first live run with Chat-Jimmy; decide what to keep, change or drop.

## Current state, at a glance

| Area | State | Evidence |
| --- | --- | --- |
| Repository setup | ✅ Bootstrapped | `main` and `alpha` exist |
| Design notes | 🟡 Proposed | this experimental branch, pending owner review |
| Thought Controller executable | ✅ v1 | Go app + `marquee.json`, builds on Linux and Windows |
| First mock thought | ✅ Tested here | Go tests + mock runs (SIMULATED) |
| Chat-Jimmy connection | 🟡 Works end to end against a fake site | real worker → headless Chrome → fake chat site; real chatjimmy.ai not yet |
| Several interacting thoughts | 🟡 First version | follow-ups, wake on related events, budgets; usefulness not measured |
| Lifetimes (sleep / wake / expire) | 🟡 First version | idle-beat rules, adjustable; values are guesses to test |
| Stale replies when context changes | ✅ Tested here | Go test: reply discarded, next heartbeat includes the change |
| Continuous heartbeat | ⚪ Not enabled | only owner-pressed runs of ≤ 20 beats |
| Communication Bridge / ScreenBridge | ⚪ Not connected | Gate 6 |
| Identity / memory / curiosity modules | ⚪ Not built here | Gate 7 |

Legend: ✅ demonstrated from available evidence; 🟡 proposed/in review; ⚪ not started; 🔴 failed/blocked when observed. A project being reachable is not the same as a capability passing its experiment.

## Journal

### 2026-10-23 — v1 repair prototype

**Branch:** `experiment/v1-repair-20261023` (from v1 `daaa0c3`; alpha untouched). Answers the repair notes and Issue #3.

**What changed:** all P0 items (busy guards and Stop/cancel, pending events on unreadable heartbeats, readiness before every send plus source labels, surfaced save errors, safe reset, per-branch data folder). P1 first versions (token on reads, redacted diagnostic, optional text-free trace, stage strip and outcomes, quoted events, echo guard, better duplicate check). A slow mock and Ask directly for the walk-through. Details: [V1_REPAIR_RESULTS](V1_REPAIR_RESULTS_2026-10-23.md).

**What ran:** 31 Go tests (PASS). Walk-through A–F here through the real Chat-Jimmy worker and a fake chat site (PASS, see the results file).

**Not verified:** the real chatjimmy.ai site.

**Decision:** owner to run the walk-through. No promotion requested.

### 2026-10-23 — v1: first runnable thoughts

**Branch:** `experiment/first-thoughts-v1-20261023` (from design-foundation; alpha untouched).

**What changed:**
- A standalone Go app with a timeline page.
- Events with context revisions; thought records with active / dormant / resolved / expired states.
- Beats with at most one worker call: a heartbeat when there are new events, otherwise work on the longest-waiting active thought.
- A strict line format with every line validated. Budgets, duplicate refusal, follow-up depth, a work limit, idle aging and stale-reply rejection.
- Owner controls: Think once, Run 5/20, Stop, Sleep/Wake/Resolve, Settings, Reset.
- A mock worker (SIMULATED) and Chat-Jimmy through its client and worker.json.
- Persistent state and an append-only trace that includes prompts and replies.

**What ran:** 14 Go tests (PASS). Full chain against a fake chat site through the real Chat-Jimmy worker in headless Chrome (PASS). Stop leaves no processes.

**Found and fixed:** commands after chatter on the same line ("Let me think. NOTE: …") were unreadable.

**Not verified:** real chatjimmy.ai following the format; whether several thoughts beat one; the right lifetime values; whether any of it is useful. Those are what the owner's runs are for.

**Decision:** owner to test. No promotion requested.

### 2026-10-23 — Design foundation created

**Goal:** capture the owner's vision so the design is understandable and reviewable before implementation.

**What changed:** a new repository was bootstrapped, `alpha` created as an owner-review baseline, and an experimental **docs-only** proposal prepared. Links and an owner discussion thread live in [Issue #1](https://github.com/Arameair/Thought-Controller/issues/1).

**Tests:** none. No executable controller, heartbeat, or connected LLM was run by these changes.

**Known external foundation:** Chat-Jimmy's [fast-clean-worker experiment](https://github.com/Arameair/chat-jimmy/tree/experiment/fast-clean-worker-20261023) implements local worker discovery and a Go client. This is not yet tested by Thought Controller. Other bridges exist as separate experiments and are not connected here.

**Questions for owner:** Is the described heartbeat accurate? Is 'one mock thought' the correct first gate? What minimum live UI is needed to understand a thought's cause, result and lifetime?

**Decision:** awaiting owner review. No promotion requested or performed.

## How to follow this project

- Read the [README](../README.md) for a map of documents and links.
- Read [Issue #1](https://github.com/Arameair/Thought-Controller/issues/1) to discuss the proposed design and acceptance gates.
- For each future experiment, add a dated entry here with **branch/SHA, what changed, what actually ran, measurements, PASS/FAIL/error evidence, owner decision, and rollback**.
- Keep unfinished items labeled as unfinished; never turn a proposal into an apparent success by updating a title.
- Owner promotes experimentally verified work into `alpha` manually. This journal is not an automated promotion mechanism.
