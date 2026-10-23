# Architecture proposal — small independent pieces

**Proposal only. None of this controller architecture has been implemented in this repository.**

## Separate five concepts

| Concept | Role | Does it need an LLM? |
| --- | --- | --- |
| Trigger clock / event | Offers an opportunity to process something | No |
| Fast LLM heartbeat | Interprets a bounded context snapshot and *proposes* a thought or activation | Yes, when invoked; Chat-Jimmy is the first candidate |
| Thought record | Carries state, evidence, history, and links to other thoughts | No |
| Thought Controller | Schedules, validates, routes, budgets, pauses, logs and displays thought activity | No intrinsic reasoning required |
| Specialist module | Applies its own logic to memory, identity, curiosity, goals, perception, etc. | Depends on module |

This preserves the owner's idea: intelligence is **not hard-coded in one master agent**. But a robust coordinator still needs deterministic validation and safeguards.

## Intended data flow

    Trigger: owner button / elapsed time / accepted event
          │
          ▼
    Controller takes versioned context snapshot
          │ (bounded, provenance and budget attached)
          ▼
    Optional Chat-Jimmy heartbeat: suggest an activation
          │
          ▼
    Controller validates output; malformed/late/unsafe => rejected, logged
          │
          ├──► create or update Thought A
          ├──► wake Thought B (if rules permit)
          └──► leave Thought C dormant
                    │
       Modules evaluate/produce observations or candidate ideas
                    │
       Controller merges evidence / marks conflicts / stores new version
                    │
       MAY ask a bridge, produce a message proposal, or stay silent

These are **candidate transitions**, not guaranteed behavior. No module is allowed to present its speculation as verified observation.

## Project boundaries

**Thought Controller owns:** thought IDs and lifecycle, context revision numbers, causal links, event queue, bounded concurrent work, worker requests and status, stale-result rejection, logs, visible timeline, Stop/Pause, policy gates.

**Chat-Jimmy owns:** one text request at a time, browser interaction with its site, its own response/status/timing. It does not own a companion's identity or persistent thought memory. Its auto-fresh-chat behavior means controller context must be self-contained when continuity matters.

**Communication Bridge owns:** receipt, deduplication, source IDs, delivery and Discord/local transport. Thought Controller is not a new Discord client.

**ScreenBridge owns:** explicit selection and capture of a window, integrity metadata, image bytes. It is not a visual understanding model. Image interpretation needs a separate, permitted vision/OCR component.

**TASS/Connector own:** existing companion and action interfaces; Thought Controller should not directly steal browser profiles, take screenshots, click, or type.

**Project Controller owns:** applications' Play/Stop/build/preview processes, not thoughts.

## First possible integration: Chat-Jimmy

Use the documented running worker, without embedding its browser:

- Locate `~/.chat-jimmy/worker.json` (or explicit local URL).
- Check `GET /api/heartbeat` and require a **true ready** status, not merely HTTP 200.
- Use the versioned `POST /api/ask` interface, including a source label and unique request ID.
- Record success, busy, blocked, unavailable, uncertain, error, simulated; response timing and context revision.
- A worker request may be uncertain after a timeout: **never silently retry**. One active request per Chat-Jimmy worker.
- Require a locally configured access token and protect its file; don't put it in logs. The current client can use `CHAT_JIMMY_TOKEN`, but security and readiness issues should be checked before live integration.
- Worker discovery is specific to the same machine/user home environment. WSL vs Debian VM or Windows may need explicit, approved connection configuration.
- The fast-clean branch currently has mock-site speed evidence and reported real-site observations from its owner; the Thought Controller connection itself is **untested**.

**External-service boundary:** a public chat demo being accessible does not automatically grant rights for continuous/unattended programmatic use. Validate Taalas's applicable terms/permission first. Until then, keep automatic heartbeat experiments in a mock/local mode or explicitly authorized environment.

## External events and changes during a thought

A long model call cannot be magically changed after submission. Instead:

1. Record the context revision included in the request.
2. New events enter a bounded queue and update the current world model.
3. On completion, compare request revision to current revision.
4. If an important dependency changed, mark the result **stale/reconsider**, never auto-act on it.
5. A separate thought can propose a revised action. Keep the previous result visible for debugging.

Do not treat mere arrival of any message as reason to restart everything.

## Simplicity and reliability

Initial version: one manual test; no background poller; no actions; no persistent secret prompts. Later experiments can introduce a bounded scheduler, multiple thoughts, and internal event-triggered activity. Always keep process health distinct from model readiness and from successful thought completion.

Proposed UI: **Status**, **Trigger one thought**, **Active/Dormant/Resolved thoughts**, **Timeline/Logs**, **Metrics**, **Pause/Stop**. Only show controls implemented and verified at that gate.
