# Thought Controller — dashboard on the real Thought engine

> **This branch: `experiment/real-thought-dashboard-20261023`**, built on `experiment/first-user-dashboard-20261023` @ `a6b9dfa` at the owner's request ("fix it so things are not simulated"). **Not approved, not promoted.** `alpha`, `main` and the earlier dashboard branch are untouched.
>
> Results and owner checklist: [docs/REAL_DASHBOARD_RESULTS_2026-10-23.md](docs/REAL_DASHBOARD_RESULTS_2026-10-23.md).
>
> **Update:** the engine to use with Chat-Jimmy is now **[Arameair/Thought-engine](https://github.com/Arameair/Thought-engine)** (`experiment/thought-engine-v1-20261023`): real observations, no TEST mode, Chat-Jimmy as its AI worker. This dashboard reads it unchanged and adds a **Think once** button (shown when the engine offers it). The earlier Arameair/Thought prototype still works too.

Thought Controller is the developer dashboard for Thought activity. It now shows **real data only**: the Sparks, Traces and Thoughts that a running **Thought engine** ([Arameair/Thought](https://github.com/Arameair/Thought), branch `experiment/spark-trace-thought-v1-20261023`) has actually produced, organized by identity, Netflix-style.

What changed from the previous dashboard:

| Before (first-user-dashboard) | Now |
|---|---|
| A "SIMULATED HOST" with its own made-up thought store | No thought store at all. Every Spark, Trace and Thought is read live from the Thought engine |
| Test thoughts from **Add simulated thought** / **Load sample set** buttons | Removed. Thoughts form in the engine from events, by the engine's rules |
| State changes written by the dashboard itself | **Sleep / Wake / Resolve / Reopen** are sent to the engine, which checks that identity's permission and the Thought's state and saves the change — or refuses, and the dashboard shows why |
| Simulated lifecycle / handshake (`tc-sim-handshake/0.1`) and a pretend "service start" | Removed. The dashboard is simply running or not; it connects to the engine and reports the engine's real state |
| Fake identities passed in by Identity's simulated launcher | Identities, permissions and counts come from the engine |

## Run it in Project Controller

1. **Play Arameair/Thought** on `experiment/spark-trace-thought-v1-20261023` (the engine). Note its port.
2. **Play Arameair/Thought-Controller** on `experiment/real-thought-dashboard-20261023`.
3. If the dashboard says **NOT REACHABLE**, press **Find on this computer** → **Use**, or type the engine's port. The address is saved.
4. Generate activity on the **engine's** page (enable TASS, send events). The dashboard updates by itself every 2 seconds.

Standalone:
```sh
# Thought checkout:            go build -o thought . && PORT=7798 ./thought -data ~/thought-test
# Thought-Controller checkout: go build -o thought-controller . && PORT=7797 ./thought-controller -data ~/tc-test
# open http://127.0.0.1:7797/  (7798 is the default engine address; -thought 127.0.0.1:NNNN or THOUGHT_ADDR to change it)
```

Data folder: `-data`, else `TC_DATA`, else `$PC_DATA/thought-controller`, else `~/.thought-controller-dashboard/<branch>`. It holds only `instance.json` (this dashboard's own instance ID), `settings.json` (the engine address) and `events.jsonl` (log). Thought data stays in the engine.

## What you'll see

- A **LIVE** bar naming the engine (address, profile, branch @ commit) and the engine's **own** mode label. Today the engine reports `TEST · SIMULATED EVENTS · NO AI WORKER`, because its events come from its buttons and its permissions are TEST grants. The dashboard shows that honestly and invents nothing on top.
- **Who's thinking?** — one tile per identity in the engine, greyed with **THOUGHT NOT ENABLED** when the engine's permission is off; plus **System Services**.
- **Identity view** — counts; rows **Continue Thinking · Active · Sleeping / Waiting · Resolved / History**; **Recent evidence** (Traces, click for source event and reason); **Sparks** from the engine's current run, glowing then faded. An identity with nothing yet says **No thoughts yet** and where thoughts come from.
- **Thought detail** — why it formed, **Evidence** (each Trace and its source event, FORMED IT vs LATER), **Development history**, **Diagnostics**, and the change buttons.
- **System Services** — this dashboard's instance, build and data folder; the engine's service ID, installation, profile, version, build, rule and traffic; **Identity: NOT CONNECTED**.
- **NOT REACHABLE / NOT A THOUGHT ENGINE / INCOMPATIBLE** screens when the engine is stopped, the address points at another program, or the engine version isn't understood. No stale data is shown.
- **Copy Diagnostics** and **Logs**.

## Boundaries

- The dashboard never thinks, never writes thought data, never launches programs, never calls AI models or GitHub. Its only outbound requests go to the configured loopback engine address, and only after the reply is verified to be a compatible Thought engine (`thought-spark-trace-v0.x`).
- IDs are validated before anything is forwarded, and only four actions exist. Every read and change is answered by the engine for one named identity: asking for TASS's Thought through the Business returns the engine's **404**; changing a Thought while that identity's permission is off returns the engine's **403 DENIED**.
- **No authentication** yet, on this dashboard or on the engine. Loopback-only listening, a Host check and a same-origin check stop other web pages, but any local program can call either API.
- **Identity is not connected.** The dashboard no longer speaks the old simulated protocol, so Identity's `experiment/first-admin-wizard-20261023` (pinned to the old dashboard commit) will correctly report this build as **INCOMPATIBLE**. The old dashboard branch still works with it.

## Tests

`go test ./...` — 5 tests, including a real-process Stop/Play test (instance ID and engine address survive; same-folder second copy refused).
`THOUGHT_SRC=/path/to/Thought go test -run RealThought -v` — runs the **real engine and the real dashboard together**: not-reachable → connected, engine data shown, evidence, cross-identity 404, Sleep made in the engine, engine refusals passed through, engine stop/start, dashboard restart, diagnostics without names.

## Rollback

Stop using this branch; the previous dashboard (`experiment/first-user-dashboard-20261023` @ `a6b9dfa`) and `alpha` are untouched. Delete the data folder shown under System Services, or remove the experiment: `git push origin --delete experiment/real-thought-dashboard-20261023`.
