# Thought lifecycle — a draft, not a settled model

The purpose of a thought is to make the system's internal work **visible and addressable**, rather than hiding everything inside a long prompt.

## What one thought could contain

A candidate minimal record:

    {
      "thought_id": "t-001",
      "source": "manual-test",
      "trigger_id": "event-001",
      "parent_thought_ids": [],
      "state": "active",
      "question": "What matters about the new information?",
      "context_revision": 3,
      "module": "heartbeat",
      "created_at": "timestamp",
      "last_updated_at": "timestamp",
      "lifetime_policy": "experiment-controlled",
      "evidence_refs": ["event-001"],
      "output": null,
      "result_status": "pending"
    }

This is an **illustrative outline**, not a frozen JSON API. The first tests should determine which fields are useful. A thought's text is a hypothesis or observation with provenance, not automatically a fact or command.

## Candidate states and transitions

    candidate -> active -> resolved
                   │
                   ├-> dormant -> active (new relevant event)
                   ├-> interrupted -> active or cancelled
                   └-> expired / cancelled / failed

A thought can also generate a **new child thought** while retaining a link to its parent. It may be superseded by a corrected interpretation. We must be able to tell the difference between a model failure, an expired thought, and a deliberately abandoned question.

The owner wants thought lifetimes, but **durations and thresholds are intentionally undecided**. Candidates to test: elapsed time, relevance to a goal, missing information, changed context, activation signals, maximum retries, or owner intervention.

## Multiple thoughts are not necessarily multiple model calls

Three active thoughts might each carry state while only one worker request is in flight. Parallel *thought state* is possible even if Chat-Jimmy itself processes one request at a time.

We must test whether thought interactions help or harm. Prevent feedback loops by bounding depth, activation count, queue size, total model calls, runtime, and repeated near-identical thoughts. A result from Thought A can influence Thought B, but the same unsupported guess cannot become an established fact merely by repetition.

## A manually inspectable example

Event: owner sends "I no longer want the feature."

- Thought A: planning a feature; context revision was 4.
- New message becomes revision 5, tied to an authenticated owner source.
- A completes based on revision 4: its proposed next step is marked **stale**.
- Thought B: "reconsider feature work based on the new constraint" may activate.
- No GitHub commit, Discord send, or file edit happens automatically.

## What to log

For each activation or state change, record: timestamp, IDs, module, triggering signal, prior/new state, context revision, model request ID if sent, wall-time, structured verdict, reason for transition, error category, and references to other thoughts. Keep sensitive message text and image bytes out of routine logs.

Useful questions to test: Did the thought use new information? Was it helpful? Was it redundant? Did it trigger a spurious thought? Did pausing prevent new work? Did the system stay within its call and time budget?

The system's operational "I am" could be built from durable identity/history/context components in future work. A persistent data structure is not evidence of felt experience or consciousness.
