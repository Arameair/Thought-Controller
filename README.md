# Thought Controller — design foundation

**Status: 🟡 DESIGN ONLY. No Thought Controller executable exists. Nothing runs automatically.**

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
