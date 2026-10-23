# Vision — the idea we're trying to test

**Stage: concept. Not implemented or demonstrated.**

## The owner's idea

Instead of a chatbot that waits for a question, imagine a companion whose internal thought activity continues while it is quiet, speaking, listening, or working on something.

A very fast LLM, initially Chat-Jimmy, could serve as a kind of **interpretive heartbeat**: it examines small summaries of current activity and proposes what deserves attention or further thinking. Several thoughts may fire independently, rather than always following a linear chain. Thoughts can stimulate others, overlap, remain dormant, or expire.

Independent systems—identity, memory, curiosity, goals, perception, communication—can each contribute. The **controller manages activity and relationships**, but we do not assume the controller itself is intelligent. Useful behavior, if it emerges, is an outcome of the interactions.

The companion would ideally develop an operational sense of continuity: "Who am I? What was I thinking about? What matters now? What changed since I started this task?" This is an engineering goal for persistent state, **not evidence of awareness, sentience, or subjective experience**.

## Example to imagine, not a feature claim

You're playing a game. A signal says your character entered an area.

- Observation suggests there is a location change.
- Memory notices you have returned here before.
- Curiosity creates an unresolved question: *Why does this place keep mattering?*
- A current combat concern may suppress that curiosity temporarily.
- Later, when new information arrives, the curiosity thought can resume.

No component needs to speak. And no component needs access to the screen if a reliable text event is sufficient.

Later, while you speak to the companion about a software feature, a separate thought may still be working on your earlier request. If you change the requirement, new information should update the shared context, flag stale work, and let that thought reconsider. An in-flight model response cannot literally be rewritten in place; the system must notice changes and decide whether to accept, revise, or discard the result.

## Guiding principles

1. **Continuity without constant model calls.** Keeping thoughts and state alive does not require the LLM to generate tokens continuously.
2. **Selective senses.** Screenshots, text, audio, and communication are tools to request when needed, subject to approval and availability.
3. **Independent thoughts.** Three or five thoughts should be possible, but not inherently better than one. Their value must be compared experimentally.
4. **Inspectable causality.** Show why a thought started, what influenced it, what changed, and what it triggered.
5. **Real lifetimes.** Active, dormant, interrupted, resumed, resolved, and expired are hypotheses to test, not arbitrary laws we choose now.
6. **Useful outcomes over anthropomorphism.** Measure assistance, recall, responsiveness, safe silence, error correction, and resource costs.
7. **No forced external action.** Thought production and real-world action are separate permission levels.

## Unknowns to be learned, not guessed

- Is the heartbeat primarily time-triggered, event-triggered, or both?
- What information should the LLM see at each activation?
- How many thoughts actually help, and when do they interfere?
- When should a thought expire, sleep, merge, split, or reactivate?
- When should a memory be retained as a fact rather than just a hypothesis?
- Can the system revise a thought appropriately during a live conversation?
- Can we demonstrate a benefit over a simple single-prompt companion?

We should preserve these uncertainties. The project exists to answer them with repeatable experiments.
