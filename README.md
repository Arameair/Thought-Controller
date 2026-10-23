# Thought Controller

**Status: design stage — no executable controller yet.**

Thought Controller is the proposed coordination system for an experimental AI companion: ongoing, inspectable thoughts that can activate one another and selectively request memory, identity, curiosity, communication, or perception. A fast language-model worker such as [Chat-Jimmy](https://github.com/Arameair/chat-jimmy) is a candidate heartbeat/interpretation component. We will **test** whether these mechanisms are useful; we are not claiming artificial consciousness.

## Start here

The initial design is being reviewed on [experiment/design-foundation-20261023](https://github.com/Arameair/Thought-Controller/tree/experiment/design-foundation-20261023). Open that branch to read the vision, architecture, thought model, experiment plan, and status notes. That branch contains **notes only**, not a working prototype.

[View proposed design and changes](https://github.com/Arameair/Thought-Controller/compare/alpha...experiment/design-foundation-20261023)

## Project rules

- **main**: repository bootstrap / public-facing baseline when ready.
- **alpha**: latest owner-approved experimental baseline.
- **experiment/***: isolated proposed work based on alpha. The owner alone promotes experiments; no auto-merge.
- Keep Chat-Jimmy, TASS, ScreenBridge, Communication Bridge, and connectors independently runnable and testable.
- Every experiment must show what actually ran, what passed, what failed, and what remains unverified.

The first goal is **one manually initiated thought, one recorded result, and clear evidence**—not autonomous continuous operation.
