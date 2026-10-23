# Decisions and open questions

This page separates **what we've agreed as a working direction** from **details that only experiments can determine**.

## Working direction (subject to owner review)

- The companion should have continuing internal thought activity, not only respond to user prompts.
- A fast LLM such as Chat-Jimmy is a candidate for an **interpretive heartbeat**, not the whole companion mind.
- Multiple thoughts should be possible, including independently active and dormant thoughts that influence one another.
- Independent modules may eventually supply identity, memory, curiosity, goals, perception, and communication.
- Thought Controller manages state, routing, lifecycle, budgets, provenance and observable behavior; no module automatically acquires broad external authority.
- All pieces should be independently runnable, stoppable, replaceable and tested before connection.
- Experiments and owner decisions, not an early abstract design, should determine practical parameters.

These reflect the discussion and **do not imply a chosen implementation or a successful test**.

## Decisions pending

| Topic | Candidates | How we might test |
| --- | --- | --- |
| Heartbeat trigger | timer, event, combination | compare idle useful work / number of calls |
| Heartbeat worker | Chat-Jimmy, rule-only, other permitted model | quality vs latency and reliability |
| Active thoughts | 1, 3, 5, adaptive | correct decisions vs redundant activation |
| Lifetime | time, importance, dependencies, inactivity | dormant thought resumes appropriately |
| Sharing information | direct thought links, event bus, shared context store | traceability and conflict resolution |
| Persistent identity | short state, durable profile, linked memories | continuity without invented facts |
| Relevance threshold | fixed, learned, no threshold initially | false activations and missed opportunities |
| What to perceive | text-first, explicit snapshot, OCR, approved vision | usefulness vs privacy and cost |
| When to speak | only asked, high-priority event, owner-configured | user experience and false interruptions |
| Provider permission | authorized API, local simulator | compliance before unattended use |

## How to record a real decision

Append an entry headed with the UTC date, decision owner, options considered, evidence, selected choice, tradeoffs, and whether it remains experimental. Use [Issue #1](https://github.com/Arameair/Thought-Controller/issues/1) for design review. Don't mark open questions as decided by the model alone.
