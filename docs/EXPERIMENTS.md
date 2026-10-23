# Evidence-first experiment plan

**No gate is approved or completed.** One gate at a time, branch from the latest owner-approved `alpha`, preserve rollback to alpha, and ask the owner to promote only after their own tests. These are suggested experiments, not implementation instructions to execute now.

## What every test must show

- **Starting conditions:** branch + exact SHA, mock/live provider, independent processes and ports, permissions, relevant context revision.
- **Trigger:** the owner button or named event and the exact expected number of model calls.
- **Observed trace:** each thought's state transition and causal link, timings (submit, first text, total where measurable), outcome, errors and whether the worker was really contacted.
- **Result:** PASS, FAIL, ERROR, BLOCKED, UNSUPPORTED, UNCERTAIN, or SIMULATED as appropriate. Never count SIMULATED as live.
- **Failure recovery:** Pause/Stop, no orphan browser or unacknowledged retry, no automatic promotion, no silent loss of state.
- **Decision:** Keep, revise, revert, or gather more data; record who approved and why.

### Gate 0 — Design review (current)

Read [Vision](VISION.md), [Architecture](ARCHITECTURE.md), [Thoughts](THOUGHTS.md), and this plan. Record disagreements in [Issue #1](https://github.com/Arameair/Thought-Controller/issues/1). **Success:** owner agrees what the first runnable test is and what it would establish. **Current status: proposed/reviewing.**

### Gate 1 — First controlled thought, MOCK ONLY

A standalone Go app in Project Controller (its own `marquee.json`) with one button: "Trigger thought". A deterministic fake worker produces a known response.

**Success:** exactly one thought created; visible request/context version, start/end, response and state; no calls to Chat-Jimmy; Stop exits cleanly. Test repeated clicks and busy behavior. This validates controller mechanics, not intelligence.

### Gate 2 — One thought through a running Chat-Jimmy worker

Use its existing local discovery and Go client; do not start or stop Chat-Jimmy from Thought Controller. Only one explicit owner-triggered request, after provider-access rules are satisfied.

**Success:** readiness check doesn't send a prompt; one request returns one response or a clearly categorized failure; context revision and latency are recorded; busy is not retried silently; uncertain outcomes are never double-sent. Compare one-line and multiline inputs and verify real result, not only fake-site latency. No heartbeat yet.

### Gate 3 — Three independent thought records

Start three thought records from known input. Try unrelated and related thoughts. Initially **serialize** Chat-Jimmy requests because the worker is one-at-a-time, but let thoughts maintain independent lifecycles.

**Success:** unique IDs, independent results, traceable cross-links, bounded queue, no infinite recursion, stable behavior when one thought fails. Compare single-thought versus three-thought usefulness and cost. Try both 'no interaction' and 'signal between thoughts' conditions.

### Gate 4 — Thought lifetimes and controlled heartbeat

Test activation and dormancy with fake events and a low, visible, owner-enabled schedule. Record every proposed signal. Expiry vs reactivation is a measured parameter, not a hard-coded philosophical decision.

**Success:** pause stops new activation; running work is handled predictably; dormant thoughts reactivate only on defined signals; failures don't cause call storms; fixed test budget is obeyed. Do not introduce unattended live public-demo traffic without authorized access.

### Gate 5 — Context changes while thinking

One thought starts on context revision N. An authenticated new message arrives during its work; mark earlier result stale if the relevant fact changed. Let a later thought reconcile.

**Success:** no obsolete action occurs; event ordering and revision trace are visible; the system avoids discarding unrelated work.

### Gate 6 — One bridge at a time

Start with read-only, text-only events from Communication Bridge (its identity, origin, message IDs and delivery rules remain authoritative). Later test ScreenBridge metadata, then a manually approved single screenshot through a suitable vision/OCR adapter. Keep signals and permissions separate from outward replies.

**Success:** no duplicate Discord posts; no screen automatically captured; no false claims that Chat-Jimmy can see images. Every permission is visible and revocable.

### Gate 7 — Independent memory, identity, and curiosity

Add only one module per experiment. Give each a narrow contract and specific benchmark. Ask whether it improves longer conversations, recall, prioritization, safe silence and correction.

**Success:** repeatable improvement over the simpler baseline, or honest evidence that the module is unnecessary/harmful. Never treat coherent language as proof of consciousness.

## Limits we haven't selected

Heartbeat rate, default thought count, activation thresholds, how many historical thoughts to retain, token budget, decay rules, merge/split rules, evaluation benchmark, and useful duration are **open variables**. Record the experimental settings in every run; do not select them based on intuition alone.

## Current recommended next move

Discuss and review Gate 0. If approved, implement **Gate 1 mock only** in a new experimental branch with a tiny status/test/log interface. Do **not** start integrating bridges or issuing background Chat-Jimmy calls just because the code can reach them.
