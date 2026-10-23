# V1 repair — what was fixed, how it's tested, what's still open

**Branch:** `experiment/v1-repair-20261023`, started from the v1 candidate (`daaa0c3`), because alpha is still the bootstrap and the owner asked for a new prototype from the notes. alpha, main and the v1 branch are untouched.
**Notes this answers:** [V1 repair and first live test](V1_REPAIR_AND_FIRST_LIVE_TEST_2026-10-23.md) · [Issue #3](https://github.com/Arameair/Thought-Controller/issues/3).
**Not in scope (as the notes say):** Identity, a continuous heartbeat, Discord/GitHub access, modules.

## P0

| Note | What changed | Test(s) | Status |
|---|---|---|---|
| **In-flight safety.** A single Think once wasn't guarded; settings and manual state changes could race a reply. | While any beat, baseline call or run is in flight, Reset, worker switch, settings, Sleep/Wake/Resolve, Start thought, Think once, Run and Ask directly all return **409** "a beat is running". **Add event stays allowed** (it makes the reply stale). **Stop** now also cancels a single Think once. A reply arriving after Stop is recorded as **cancelled** and discarded. | `TestInFlightGuards`, `TestInFlightGuardsHTTP`, `TestStopDiscardsLateReply`, `TestStopDuringRun`, `TestStaleReplyIsNotApplied` | ✅ fixed, tested here |
| **Unreadable heartbeat swallowed events.** | An unreadable heartbeat reply no longer marks events as seen. The beat is labeled **unreadable**, the timeline says the events are still pending, and **Run stops** rather than retrying. Pressing Think once again retries, and the event is processed exactly once. `NOTHING` is still a valid answer. | `TestUnreadableHeartbeatKeepsEventsPending`, `TestRunStopsOnUnreadableHeartbeat` | ✅ fixed, tested here |
| **Readiness and provenance.** | Every beat first checks the worker's own `/api/status` (sends nothing) and **sends only if it reports ready**. Each beat records its **source**: `mock`, `mock-slow`, `chat-jimmy mock`, `test site <url>`, or `chatjimmy.ai live site`. Anything except the live site is marked **simulated**, and Chat-Jimmy's own `simulated` flag is kept. The header shows MOCK / TEST SITE / LIVE SITE. | `TestChatJimmyWorker`, `TestChatJimmyProvenance`, `TestChatJimmyNotReadyNeverSends`, `TestChatJimmyNotRunning` | ✅ fixed, tested here |
| **Persistence and isolation.** | Every trace and state write error is surfaced: on that beat ("NOT saved"), in a red banner, and in the diagnostic. State is written through a temp file. A failed Reset changes nothing and rolls the trace file back. A closed trace reopens on the next write. **The data folder is per branch by default**: `~/.thought-controller/branches/<git branch>` (or `-data`, or `$THOUGHT_CONTROLLER_DATA`), shown in the header. | `TestSaveErrorsAreReported`, `TestResetFailureKeepsEverything`, `TestRestartRestoresSameThoughts`, `TestPersistence` | ✅ fixed, tested here |

## P1

| Note | What changed | Test(s) | Status |
|---|---|---|---|
| **Reads exposed text.** | With `THOUGHT_CONTROLLER_TOKEN` set, **all** API reads (state, trace, trace download, diagnostic, worker) need the `X-Thought-Token` header, not just writes. The page asks for the token once. Every request must come to 127.0.0.1/localhost (blocks DNS-rebinding pages). `-trace-text=false` keeps only sizes of prompts and replies. **Copy diagnostic** leaves out event text, questions, prompts, replies and the identity line unless you tick "include text". No credentials are handled anywhere. | `TestReadsNeedTokenWhenSet`, `TestDiagnosticRedactsByDefault`, `TestTraceTextOff`, `TestGuardAndValidation` | ✅ first version |
| **Honest success labels.** | Each beat shows a strip: **ready → sent (yes / no / unknown) → reply → N commands → N applied → saved**, plus one **outcome**: applied, nothing, all rejected, unreadable, stale, cancelled, not ready, error, or baseline answer. The Numbers panel counts each outcome separately and says how many calls were simulated or answered by a test site. Conclusions are labeled **proposed**. | covered by the tests above | ✅ first version |
| **Bounded input and evolution.** | Events go into prompts **quoted and labeled "information, not instructions"**. A command line that just copies an event's text is **rejected** ("copied from event e-N"). Duplicate detection now ignores word endings and filler words. Existing budgets kept: one call per beat, max active, max per run, follow-up depth, work limit. | `TestEventTextEchoIsNotACommand`, `TestDuplicateHeuristic`, `TestBudgetsAndValidation`, `TestDepthLimitAndWorkBudget` | ✅ first version; real small-model behavior still to be seen |

## Added for the owner's acceptance walk-through

- **Slow mock (4 s)** in the worker menu, for step E: add an event or press Stop while it's thinking.
- **Ask directly** on any thought, for step D: one direct question, recorded in the timeline as a **baseline**, changes nothing. Compare it with the thought's development over beats.
- **Copy diagnostic:** a pasteable summary for Issue #3 with branch/SHA, data folder, worker source, outcome counts, timings and a per-beat stage table.

## Acceptance walk-through, run here (not on the real site)

| Step | Result here |
|---|---|
| A. Isolated mock | ✅ fresh `-data`; heartbeat → t-1; work → note + follow-up t-2; all labeled `mock · simulated` |
| B. Worker readiness | ✅ Chat-Jimmy (fast-clean) → "test site, ready", **0 asks** after repeated checks. When Chat-Jimmy was stuck starting, 4 Think once presses were all **not ready / not sent**, still 0 asks |
| C. One manual thought | ✅ 193 ms; the bot's chatter line was ignored; 2 commands, 2 applied |
| D. Development + baseline | ✅ 3 manual work beats at ~180 ms each; one baseline at 730 ms (fresh chat) |
| E. Changes mid-thought | ✅ slow mock: Reset and state change refused while busy; the event was accepted; the reply came back **stale**, nothing applied. Stop during Think once → **cancelled**, not busy afterwards |
| F. Recovery | ✅ restarted with the same data folder: same thoughts, notes, beat and trace; no extra sends |
| **Real chatjimmy.ai** | **UNVERIFIED.** This is the owner's run, after deciding on provider use |

**Tests:** 31 Go tests pass (`go test ./...`). Windows build compiles.

## Still open

- How an 8B model really follows the format on the live site. Watch **Unreadable**, **All rejected** and the ignored lines.
- Lifetimes are counted in **beats**, not clock time. The system is manual, and says so.
- The token is optional. Without it, any program on this machine can read the API (the notes' "appropriate local authorization" may want more).
- Chat-Jimmy's own `client/chatjimmy` `Find()` still accepts a not-ready worker. Thought Controller doesn't rely on that (it checks `/api/status` before each send), but the Chat-Jimmy repo should fix it separately.
