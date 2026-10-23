# Thought Controller — v1 repair prototype

**Status: 🟢 runnable experiment** on `experiment/v1-repair-20261023` (from the v1 candidate; alpha untouched). It fixes every P0 and P1 item in the [repair notes](docs/V1_REPAIR_AND_FIRST_LIVE_TEST_2026-10-23.md). **[What was fixed, test by test →](docs/V1_REPAIR_RESULTS_2026-10-23.md)**

What's different from v1:
- **Busy guards.** While a beat runs, only **Add event** works; everything else says "a beat is running". **Stop** also stops a single Think once, and a late reply is discarded.
- **Pending events.** An unreadable heartbeat **keeps your events pending** and stops a run instead of retrying.
- **Readiness and source.** Each beat checks the worker is ready first (sends nothing), and records where the answer came from: **MOCK / TEST SITE / LIVE SITE**. Only the live site counts as real-model evidence.
- **Evidence strip.** Each beat shows **ready → sent → reply → commands → applied → saved** and one outcome.
- **Saving.** Save errors are shown, never hidden. Each branch gets its **own data folder** (shown under the title).
- **Text handling.** Optional token covers reads too. **Copy diagnostic** leaves text out by default. Event text is quoted and can't act as a command.
- **For the walk-through:** **Slow mock** (for testing Stop/STALE by hand) and **Ask directly** (one-shot baseline to compare against).

## Run it

Project-controller → Arameair/Thought-Controller → this branch → **Play**. (Terminal: `go build -o thought-controller . && ./thought-controller [-data DIR] [-trace-text=false]`, page on `$PORT`, default 7792.) Data goes to `~/.thought-controller/branches/<branch>` unless you pass `-data`.

For live thoughts, also **Play** Chat-Jimmy (`experiment/fast-clean-worker-20261023`) and pick **Chat-Jimmy (live)** at the top right. Thought Controller finds it through `~/.chat-jimmy/worker.json`. It never starts or stops it, and checking readiness never sends a prompt.

## How it works

- **Events** are things that happened ("I entered Goldshire again."). Each one bumps the **context revision**.
- **Thoughts** are records with an ID, the question, where they came from (event, owner, or a parent thought), notes, a conclusion and a state: **active → dormant → expired**, or **resolved**.
- **Beat:** at most **one** worker call.
  - **Heartbeat** when there are new events: the worker reads the events and current thoughts and proposes changes.
  - Otherwise **work** on the active thought that has waited longest, moving it one step.
  - With nothing to think about, there's no call.
- **Worker reply:** the worker answers in short lines (`NEW:`, `WAKE t-3:`, `NOTE:`, `RESOLVE:`, `SLEEP`, `SAY:`, `NOTHING`). The controller checks every line before anything changes, rejects what breaks a rule, and says why.
- **Rules** (adjustable in **Settings** and recorded with every beat):
  - Max 5 active thoughts and 2 new per beat.
  - Follow-up depth 2.
  - A thought sleeps after 6 work beats without resolving.
  - Idle 8 beats → dormant; 30 → expired.
  - Duplicate questions are refused.
- **Stale replies:** if an event arrives while the worker is thinking, its reply is marked **STALE** and nothing from it is applied. The next beat is a heartbeat that includes the new event.
- **SAY lines** go to **Would say (never sent)**. No messages, clicks or file changes, ever.
- **Buttons:** **Think once** (one beat), **Run 5 / Run 20** (stops early when there's nothing left), **Stop**, and Sleep / Wake / Resolve on any thought.
- **Saved data:** `~/.thought-controller/state.json` and `trace.jsonl` (every beat, including the exact prompt and reply) survive restarts. **Reset** keeps the old trace file.

## Try this first

1. Mock worker: add "I entered Goldshire again.", then **Run 20 beats**. Watch thoughts get created, worked and resolved. Everything is labeled SIMULATED.
2. Switch to Chat-Jimmy, **Reset**, and repeat. Open "what the worker saw and said" on a few beats. **Unreadable replies** and **Rejected** show how well an 8B model follows the format.
3. Put a resolved or active thought to sleep, then add a related event. Does the heartbeat wake it?
4. Start a thought ("What should I work on next?"). While a run is going, add "I no longer want the feature." and look for **STALE**.
5. Try **Settings → Fresh chat each call** on and off, and fill in **About the companion**. Compare the numbers.

## Tested here (before your first run)

| Check | Result |
|---|---|
| 14 Go tests: parser (sloppy replies, commands after chatter), heartbeat → work → resolve, budgets, duplicates, depth, work limit, stale reply not applied, busy, idle → dormant → expired, wake, errors not retried, persistence/reset, run cap + Stop, request guard | PASS |
| Chat-Jimmy client against a fake worker: found via worker.json, readiness sends nothing, one ask per beat, "not running" categorized | PASS |
| **Full chain:** Thought Controller → real Chat-Jimmy worker (fast-clean) → headless Chrome → fake chat site with a sloppy bot. Event → 2 thoughts → worked → resolved in 7 beats; sleep + related event → woken + a would-say line; ~205 ms per beat; 0 unreadable after the parser fix; clean Stop | PASS |
| Real chatjimmy.ai answering in this format | **NOT TESTED**. That's your first live run |

That full-chain run found a real problem first: work replies like "Let me think. NOTE: …" were all unreadable, because commands were only recognized at the start of a line. Fixed, with a test.

# Design foundation (unchanged)

*The design notes below are as reviewed. v1 above implements Gates 1–2 and a bounded first look at 3–5.*

> The goal is to experiment with a companion that has ongoing internal activity: multiple thoughts can fire, develop, influence one another, become dormant, and resume. A fast LLM is a candidate heartbeat/interpretation worker. The controller manages the machinery; intelligence, if useful behavior develops, would come from the system working together rather than from one central prompt.

This is a hypothesis to test, not a claim of human cognition or consciousness.

## Read the project in this order

1. [Current status and journal](docs/STATUS.md) — what actually exists, what changed, what to inspect next.
2. [Vision](docs/VISION.md) — the idea in ordinary language, including the companion's possible sense of continuity ("I am").
3. [Architecture](docs/ARCHITECTURE.md) — how the heartbeat, thoughts, independent modules, and other projects might connect.
4. [Thought lifecycle](docs/THOUGHTS.md) — draft states, connections, evidence, lifetimes, and examples.
5. [Experiments](docs/EXPERIMENTS.md) — small, gated tests with observable pass/fail criteria.
6. [Decisions and open questions](docs/DECISIONS.md) — assumptions we have **not** decided yet.

[Discuss the design in Issue #1](https://github.com/Arameair/Thought-Controller/issues/1). The issue is the owner's review thread; documentation is not permission to start implementation.

## Concept, not yet implemented

    Clock / new event ──► Fast LLM heartbeat (Chat-Jimmy candidate)
                                │ candidate thought signals
                                ▼
                         Thought Controller
                         (routing + state + evidence)
                          │       │        │
                        Thought A Thought B Thought C
                          │       │        │
                          └──► independent modules ◄──┘
                                 (later)
                     memory | identity | curiosity | goals
                                 │
                       request a tool only if needed
               Communication Bridge | ScreenBridge | TASS

The arrows are illustrative, **not a fixed pipeline**: thoughts can be concurrent, linked, reactivated, or deliberately left unresolved. Perception, speaking, and acting are optional outputs, not mandatory effects of thinking.

## Projects we intend to reuse, not absorb

- [Chat-Jimmy fast-clean worker](https://github.com/Arameair/chat-jimmy/tree/experiment/fast-clean-worker-20261023): first candidate fast text worker; has worker discovery, a Go client and per-request timing. **Not connected here.** Its mock-site timings are not proof of live-site performance; ongoing automated provider use requires appropriate authorization.
- [Communication Bridge](https://github.com/Arameair/Communication-Bridge): transport for owner messages and replies, not an intelligence module.
- [ScreenBridge](https://github.com/Arameair/ScreenBridge): owner-selected window capture; screenshots are not automatically understood by a text-only model.
- [TASS](https://github.com/Arameair/TASS) / [TASS Connector](https://github.com/Arameair/TASS-Connector): current companion workflow and controlled connection path.

Every project retains its independent Play/Stop lifecycle. Thought Controller must connect to already-running workers, never secretly start or stop them.

## Development rules

- `alpha` = latest owner-approved test baseline; `experiment/*` = candidate started from alpha; `stable` = snapshot of a verified alpha when the owner chooses; `main` = bootstrap/public baseline.
- **The owner alone promotes to alpha.** Never auto-merge or call a simulation a live result.
- One experiment, one question, one observable result, then a deliberate decision.
- The first implementation gate is a **manually triggered, one-thought mock test**, followed by a separately approved one-thought Chat-Jimmy test. No unattended heartbeat or tool actions in the first build.
- Proposed thought outputs are data. Sending messages, controlling a PC, changing code, or modifying files requires separate explicit authorization.
- Keep logs and simple status readable to a person, not just another AI. Record input source, state changes, timing, outcome, and uncertainty without exposing secrets.

**Design review:** [Issue #1](https://github.com/Arameair/Thought-Controller/issues/1) · **Proposed work:** [experiment/design-foundation-20261023](https://github.com/Arameair/Thought-Controller/tree/experiment/design-foundation-20261023)
