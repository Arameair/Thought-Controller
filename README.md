# Thought Controller — first new-administrator dashboard (SIMULATED prototype)

> **This branch: `experiment/first-user-dashboard-20261023`**, started from `alpha` @ `02fc27f` for [Issue #6](https://github.com/Arameair/Thought-Controller/issues/6) (Stage A) and the [coordinated first-user handoff](https://github.com/Arameair/Thought-Controller/blob/experiment/multi-identity-controller-design-20261023/docs/CLAUDE_FIRST_USER_PROTOTYPE_HANDOFF_2026-10-23.md). **Not approved, not promoted.** `alpha`, `main` and every earlier experiment are untouched.
>
> Its partner is Identity's `experiment/first-admin-wizard-20261023`. Start there: Identity is the first app a new administrator opens. Shared test results and the owner checklist for both apps live on the Identity branch: [FIRST_USER_PROTOTYPE_RESULTS_2026-10-23.md](https://github.com/Arameair/identity/blob/experiment/first-admin-wizard-20261023/docs/FIRST_USER_PROTOTYPE_RESULTS_2026-10-23.md). (Kept there so this branch's tip stays exactly the commit Identity pins.)

A Netflix/Prime-style **thought dashboard**: profile tiles for each identity (TASS, Example Workshop) plus **System Services** for the controller's own service identity, then rows of thought cards — **Continue Thinking · Active · Sleeping / Waiting · Resolved / History**. Click a card for its details, development history and diagnostics.

**Everything here is a test prototype.** The program is a **SIMULATED HOST**: it never launches anything, never calls an AI model, never contacts GitHub, and never thinks on its own. Every thought is made by an owner test button and marked **SIMULATED**.

## Run it in Project Controller

1. **Arameair/Thought-Controller** → branch **`experiment/first-user-dashboard-20261023`** → **Play** (build plan: `go build -o thought-controller .`, then `./thought-controller`).
2. The page says **"No controller service is running"**. That's correct: the *host* is up, but no installation's controller has been started.
3. Note the **port** on this pane — you give it to Identity (or Identity's **Find on this computer** finds it).
4. Open **Identity** (its own Play) and follow its first-run steps. When you approve automatic startup, Identity's simulated launcher asks this host to start *your* installation's controller with *your* IDs.
5. **Stop** ends the host completely (no child processes exist). **Play** again: the host waits for Identity again, and Identity's saved approval starts it without asking.

Data folder under Project Controller: `$PC_DATA/thought-controller-first-user` (shown on the page).

## Run it standalone (no Project Controller)

```sh
go build -o thought-controller .
PORT=7797 ./thought-controller -data ~/tc-first-user-test        # Linux / macOS / WSL
```
```powershell
go build -o thought-controller.exe .
$env:PORT=7797; .\thought-controller.exe -data $HOME\tc-first-user-test   # Windows PowerShell
```

Port 7797 is what a new Identity profile looks at by default, so a standalone pair needs no setup. Without `-data`, the folder is `TC_FIRST_USER_DATA`, else `$PC_DATA/thought-controller-first-user`, else `~/.thought-controller-first-user/<branch>`. These names deliberately differ from the repaired V1 experiment's (`THOUGHT_CONTROLLER_DATA`, `~/.thought-controller/…`), so this prototype **cannot open that experiment's state**. A data folder inside the code checkout is refused; a second host on the same folder refuses to start.

## What you can do on the dashboard

- **Who's thinking?** — one tile per identity Identity registered, with thought counts, and a **System Services** tile.
- **Identity view** — the identity's ID (Copy ID), counts per row, **Add simulated thought** (title + row) and **Load sample set** (one SIMULATED thought per row). An identity with no thoughts shows **No thoughts yet** and says why.
- **Thought detail** — *Details* (origin, timestamps, notes, worker calls = 0), owner **Continue / Activate / Sleep / Resolve** buttons and **Add note**; *Development history* (every state change and note); *Diagnostics* (IDs, lifecycle, storage path — no title or note text) with Copy.
- **System Services** — the controller's **service ID**, **installation ID** and **administrator ID** exactly as Identity issued them, the start request and consent reference, the reference build and lifecycle counters. A service identity has no thoughts by design.
- **Header** — service state, **Worker: NOT READY** (no AI worker exists), branch @ commit, **Logs**, **Copy Diagnostics**, **Stop service** (simulated).
- If Identity opened the page for a different installation than the one running, a red banner says so.

## How installations stay apart

```
<data>/installations/<installation_id>/binding.json            service + administrator IDs (written once, never re-bound)
<data>/installations/<installation_id>/events.jsonl
<data>/installations/<installation_id>/entities/<entity_id>/entity.json
<data>/installations/<installation_id>/entities/<entity_id>/thoughts/<thought_id>.json
```

- The host runs **at most one installation's controller service at a time** and only loads that installation's folder. A second installation is refused ("busy") until the first is stopped; then it gets its own folder and sees none of the first one's identities or thoughts.
- Every thought route names its entity (`/api/entities/{entity_id}/thoughts/...`) and is looked up only in that entity's own folder. Reading or changing TASS's thought through the business path returns **404**. An unregistered entity is rejected, never defaulted.
- Each save is written to a temp file, flushed, renamed, then **read back**; only then is it shown as saved. Unreadable files are listed and **left untouched**.

## Real vs simulated

| Piece | Status |
|---|---|
| Dashboard, rows, empty states, detail / history / diagnostics | **Works** (real UI over real files) |
| Per-installation and per-entity storage, restart restore | **Works** (tested with the real binary) |
| Server-side entity scoping (cross-entity read/write → 404) | **Works**, but **no authentication** — any local program can call the API |
| Health / start / stop / register | **SIMULATED handshake** `tc-sim-handshake/0.1`, unauthenticated |
| "Starting" the controller | **SIMULATED**: the host is already running; a start request opens an installation's service inside it. No process is launched |
| Thoughts | **SIMULATED test fixtures** made by buttons; 0 model calls |
| AI worker / Chat-Jimmy | **Not present** (NOT READY) |
| GitHub | **Not present** — Identity owns GitHub status |

## What was reviewed and what was (not) carried over

- **`alpha` @ `02fc27f`** (bootstrap README) — this branch starts here.
- **`experiment/v1-repair-20261023` @ `e943847`** (repaired V1 thought engine) — **not merged and not ported.** Its engine (beats, parser, worker, Chat-Jimmy client) is out of scope for a no-AI visual prototype and is still unapproved. Its vocabulary (active / dormant / resolved) and its no-hidden-send, honest-label and redacted-diagnostics rules shaped this UI. Bringing the engine under per-entity scoping is Stage B and needs an owner-approved plan.
- **Ported by copy, with attribution comments** from Arameair/identity `experiment/identity-local-v1-20261023` @ `2b31e99`: `lock.go`, `lock_unix.go`, `lock_windows.go` (one host per data folder), the data-folder resolution pattern and the loopback / same-origin request guards.

## Tests

`go test ./...` — 10 tests, including **`TestRealProcess`**, which builds and runs the real binary with `PORT`, `PC_BRANCH`, `PC_DATA`: start → bind an installation → add a thought → same-folder second copy refused → SIGTERM (lock removed) → Play again → service waits → start → thought restored. Also covered: request validation, duplicate request IDs, reuse, refusal of a second installation / changed service ID / unapproved commit, entity scoping, restart restore, second-installation isolation, broken-file reporting, diagnostics redaction, Host/Origin/Content-Type guards. Builds for Linux, Windows and macOS.

## Rollback

Nothing was promoted. Stop using this branch — `alpha` (`02fc27f`) is untouched. To clear test data, Stop the host and delete its data folder (shown on the page). To remove the experiment entirely: `git push origin --delete experiment/first-user-dashboard-20261023`.
