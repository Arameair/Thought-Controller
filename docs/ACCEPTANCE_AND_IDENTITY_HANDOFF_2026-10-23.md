# Acceptance checklist and Identity handoff — what the team must deliver

**Owner instruction (2026-10-23):** Make Thought Controller **usable as a real, observable test bench**, then build the separate Identity project. This page augments the detailed [V1 repair plan](V1_REPAIR_AND_FIRST_LIVE_TEST_2026-10-23.md) and [Issue #3](https://github.com/Arameair/Thought-Controller/issues/3).

**Stage:** design/acceptance notes. **No repairs or live tests performed by this document.**

**Baseline under discussion:** `experiment/first-thoughts-v1-20261023`, SHA `daaa0c3cb4f58aa0a4f7fdf83f86f0946c04f6e4`, not Alpha. The team reports 14 Go tests and a browser-based test against a **fake site**. Actual chatjimmy.ai integration and useful reasoning **not verified**. Repair candidates must remain isolated; owner manually approves Alpha promotion.

## A. The minimum usable controller (Gate: owner can TEST)

The owner must be able to **start it through Project Controller**, view and operate the real application without special terminal-only steps, and see trustworthy outcomes. For each checkbox, report PASS, FAIL, BLOCKED or NOT RUN, *with reproducible evidence*. Tests in a fixture/CI alone are not a substitute for an owner run.

- [ ] **Launch and independence:** choose exact branch/SHA in Project Controller, review effective build/run commands, Play → reachable interactive UI. Display chosen and **actually running** version when verifiable; don't confuse page reachable with model ready. An isolated per-branch `-data` location and port must prevent two candidates overwriting each other's thoughts. Stop Thought Controller leaves independent Chat-Jimmy alone. Stop/restart Thought Controller exits/returns cleanly, with no lingering controller-owned operations.
- [ ] **Visible and real controls:** Add Event, Add Thought, Think Once, a bounded Run N, Stop, choose mock/live worker, Reset with confirmation, Sleep/Wake/Resolve, settings, thoughts board, activity timeline, metrics and readable error messages. Each control must work or state NOT IMPLEMENTED. No hidden automatic model calls. This stage does **not** require an unattended continuous heartbeat.
- [ ] **Worker readiness:** Explicitly distinguish `Mock SIMULATED`, `worker reachable but not ready`, `ready`, `busy`, `blocked`, `uncertain`, and `unavailable`. A readiness check sends **zero messages**. Require a true readiness response, not merely HTTP success. Propagate any underlying `simulated` flag all the way to the UI/trace. A real Chrome browser connected to a fake site is **still fake-site evidence**.
- [ ] **One real thought, end to end:** Add a known event, press Think Once, show the **exact prompt and model reply**, thought ID, source event, parser output (recognized vs ignored), applied/rejected operations and reasons, context revision, request ID, status and timing. Mark a malformed reply UNREADABLE and keep unprocessed events pending. No invented conclusions or silent success labels.
- [ ] **Thought development and lifecycle:** Continue a thought across multiple manual beats, inspect all notes and parent/child relationships, manually sleep/wake/resolve, and observe bounded work/activation/expiry. Explain **why** each transition happened. Thought lifetime in V1 is measured in beats, not elapsed clock time: label that clearly.
- [ ] **Input changes and safety:** A newly added event while a request is in flight invalidates a now-stale result. Reset, worker change, manual thought state/settings edits and Stop cannot allow an obsolete response to update current state. Handle both single Think Once and Run N; test via UI and HTTP to prevent relying solely on disabled buttons. Never automatically replay a possibly-sent request. Model output can propose SAY; **never automatically send** to Discord, GitHub or other external services.
- [ ] **Persistence and failure handling:** Stop/restart the *same* candidate with the *same* data directory and prove thoughts, IDs, last revision, notes and trace survive. Reset preserves an archived trace safely or reports the failure. Disk-full / denied-write / unreadable-state errors become visible and cannot be called Saved. A different branch/data folder must not corrupt these records.
- [ ] **Copyable diagnosis:** As in Project Controller, provide **Copy Thought Context / Copy Diagnostics / Copy Logs** (or one equivalent bounded shareable report). Include source project, branch/SHA/run ID if available, worker name/mode, ready state, thought and event IDs, context revision, accepted/rejected/stale, model timings, errors and actual next action. Separate verified observations from app claims and unknowns. **Scrub sensitive content** from copied reports and protect full local prompts, responses and event logs. Don't leak tokens even to a private repo.
- [ ] **Full reproducible owner check:** mock test, live-provider readiness without a send, **one** authorized live-site manual Think Once, a couple of manual follow-ups, stale-input demonstration, clean Stop/Restart and state restoration. Report exact full SHAs and local environment (Windows/WSL), mode, call count, time, behavioral result, failures and unknowns. Do **not** begin with Run 20 on an unverified real provider.

### Fix-first blockers and test gaps

See Issue #3 for detail; in source review: direct `/api/step` bypasses batch-run guard in Reset/worker switch, manual thought changes don't invalidate the revision, unreadable heartbeats can mark events as consumed, worker Find can accept `ready=false`, simulated response provenance is lost in `jimmyWorker`, and `store.go` swallows write/reset errors. Read-only trace endpoints expose complete prompt/reply data on loopback; fix data-access policy **before Identity or real accounts**. Add regression tests for every fix.

Potential source-specific failure worth checking: `worker.go` caches a worker client; reconnect after a restarted worker/changed port must not silently keep using a stale client. Status probe alone cannot prove the downstream chat site is operational.

## B. Practical first test report — fill in, don't guess

| Field | Record in handoff |
| --- | --- |
| Controller branch + full SHA | Exact tested checkout |
| Chat-Jimmy branch + full SHA | Exact independent worker |
| Environment | Windows native / WSL / Debian VM / fixture |
| Test type | Controller mock / browser fake site / **actual chatjimmy.ai** |
| Data and network isolation | Controller data folder; ports; process ownership |
| Owner sequence | Launch, add event, Think Once, inspect, update, restart |
| Model/worker evidence | Readiness, actual request count, ID, response, parser status |
| Behavior evidence | Which thought changed, how, why; relevance vs guess |
| Timings and resources | Per call, total, errors/retries; CPU/RAM if available |
| Verdict | PASS / FAIL / BLOCKED / UNVERIFIED for each acceptance item |
| Follow-up | What is broken, source of failure, copyable context, test branch |
| Promotion | **Owner click required**; never automated |

A PASS requires a real observation of that particular behavior. Passing `go test`, compiling, or receiving HTTP 200 alone is not an owner-visible end-to-end PASS.

## C. What to preserve for **Identity**, the NEXT separate project

**Identity is not personality.** It is a stable, reusable identity/entity and account/capability registry. An entity can be a companion, person, business, team, project, or other participant. Not every entity thinks or needs a personality.

Future Identity setup flow:

1. On launch, **restore** the existing entity across code branches/restarts, or walk a new user through **creating their own**, with a stable entity ID; another installation must not inherit another person's identity.
2. Every entity associates with a **private GitHub repository** as its *nonsecret environment/manifest home*. Guide GitHub signup **through GitHub's official page** if the user has no account; alternatively link an existing account and create/select a private repository. A separate GitHub login for every entity is *not* mandatory.
3. Identity owns account setup, credentials and permissions. GitHub/Discord access tokens/passwords/MFA codes are **never stored in GitHub or in Thought Controller traces**. Use a supported protected credential store and least-privilege, user-approved capabilities; do not pass raw tokens to the controller.
4. Identity knows **what capability an entity is configured for**, its owner, permission scope and resource locator. It does not need to contain each module or copy its data into GitHub. Memory/curiosity/communication/personality/senses are optional, independently testable puzzle pieces.
5. Identity discovers an already-running Thought Controller, registers the entity, and later responds to **controller-initiated requests to add an account or capability** with a guided user approval/setup flow. If the controller isn't running, show the owner the state and offer an explicit approved launch through Project Controller. **No secret cross-app lifecycle control.**

### Minimum controller-side handshake to leave room for

**Don't implement the complete Identity application now.** Once the thought test is reliable, propose and test a small, *versioned* local interface against a fake Identity fixture:

- **Read-only health/capabilities**: actual readiness, protocol version and whether entity registration is supported. Health must never create an entity or send a model request.
- **Register / reconnect entity**: accept authenticated, validated `entity_id` + identity service locator/capability references (no provider tokens); return `registered`, `already registered`, `denied` or `unsupported`. Re-registering the same ID must not create a new entity or duplicate memories.
- **Entity-scoped thoughts**: any thought/event/trace created for one entity belongs only to that ID. One entity's thoughts, memory pointers and permissions cannot be accessed through another entity. For V1, a *single default test entity* is sufficient; multi-entity isolation comes **after** a dedicated test.
- **Capability lookup**: a thought can request e.g. `memory` or `github.read`; controller forwards a request to Identity and receives either a *scoped capability handle* or a user-approval-required/unavailable response. Controller never receives raw GitHub/Discord credentials. **No account creation or remote side effects without owner approval.**
- **Ownership and lifecycle**: Project Controller controls applications' Play/Stop; Thought Controller runs thoughts; Identity manages entity config/accounts. Avoid making a thought able to launch arbitrary software or approve GitHub writes by itself.

**Important sequence:** First demonstrate a trustworthy controller with mock + one real authorized model call and durable thought state. *Then* test an entity-registration contract with a fake Identity module. *Then* build and connect the real Identity application. Do not call the current v1 'Identity ready' until that interface exists and is verified.

## D. What NOT to do in this repair

Do not build Identity, memory, curiosity, personality, auto-started heartbeat, Discord send, GitHub commit/edit capabilities, autonomous code changes, or multi-entity orchestration now. These are later independent experiments. Do not rewrite the working v1 for visual polish before the testability and correctness blockers are fixed. Do not alter/promote `alpha`, `stable` or `main` without the owner's review.

## Pointers

- [Detailed repair plan](V1_REPAIR_AND_FIRST_LIVE_TEST_2026-10-23.md)
- [Tracking issue #3](https://github.com/Arameair/Thought-Controller/issues/3)
- [Initial V1 branch](https://github.com/Arameair/Thought-Controller/tree/experiment/first-thoughts-v1-20261023)
- [Design discussion issue #1](https://github.com/Arameair/Thought-Controller/issues/1)
