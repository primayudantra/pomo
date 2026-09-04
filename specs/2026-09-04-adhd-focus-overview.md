# Pomo — ADHD Focus Layer: Design Overview

Date: 2026-09-04
Status: Approved for planning
Owner: techwithprima@gmail.com

## 1. Problem

Pomo today tracks sessions but does nothing while a session runs and offers no
review loop. The target user (an ADHD developer) hits two failure modes:

- **C — Drift mid-session.** The timer runs while the user is on Chrome / Slack /
  YouTube. The session "completes" but no work happened.
- **D — No review loop.** Sessions pile up, never reviewed, no learning, guilt
  accumulates.

This design adds a background daemon that detects drift and nudges gently, plus a
slash-command review/insight/chat surface inside the existing TUI.

## 2. Persona

**ADHD developer.** Needs: low activation energy, external structure,
non-judgmental feedback, gentle re-engagement on drift. Does *not* need: team
features, gamification pressure, heavyweight project management.

## 3. Non-goals (v1)

- Cloud sync, accounts, multi-device.
- Cross-terminal pause/resume as a headline feature (the daemon makes it possible
  later; not built now).
- Windows support for foreground-app watching (interface stub only).
- Proper browser-tab attribution when the OS denies automation permission
  (degrade to app-level, backlog the rest).
- Persisted chat history.

## 4. Scope of change

New internal packages: `watch`, `nudge`, `notify`, `ai`, `ipc`, `report`.
New commands: `daemon` (start/stop/status), `review` (`--json`), `digest`.
New TUI screens: `screenPrompt` (slash palette), `screenChat`.
New DB tables: `drift_events`. New columns on `sessions`: `repo_path`,
`repo_branch`. New config keys (see schema spec).

Existing behaviour is unchanged when the daemon is not running: the timer works
exactly as today, no errors, no drift features.

## 5. Architecture

```
pomo daemon  ──(launchd/systemd unit, or `pomo daemon start`)
   │  tick every `daemon.tick` (default 15s):
   │   • idle: poll database.LastRunningSession()
   │   • active (a session is running):
   │       - internal/watch  → foreground app + fs activity
   │       - score drift, write drift_events episodes
   │       - internal/nudge  → escalation state machine
   │       - internal/ai     → nudge line (haiku) or canned fallback
   │       - internal/notify → OS notification
   │       - push checkpoint / nudge over IPC to a connected timer
   │   • idle + Monday + last week's digest missing → run digest
   │
   └─ IPC: Unix socket ~/.pomo/daemon.sock, newline-delimited JSON

pomo (TUI)   ── connects to the socket if the daemon is up:
   • announces session start {id, repo_path}
   • receives checkpoint / nudge pushes, renders inline
   • sends checkpoint answers and nudge-action keypresses back
   • slash palette: /review /recap /insights /drift /chat /settings /start
     /note /skip /help /quit

pomo review --json   → internal/report aggregation, no TUI (cron/editor)
pomo digest          → writes ~/.pomo/reviews/YYYY-Www.md
```

**Source-of-truth rule:** the daemon owns "is this session drifting". The TUI is a
display + input client. No daemon → no drift data, timer unaffected.

## 6. Component specs

| Area | File |
|---|---|
| Daemon lifecycle, watch, drift scoring | `2026-09-04-daemon-drift-detection.md` |
| Nudge escalation + AI provider layer | `2026-09-04-nudge-and-ai.md` |
| Slash palette + `/chat` | `2026-09-04-slash-palette-and-chat.md` |
| `/review`, `/insights`, `pomo digest`, `internal/report` | `2026-09-04-review-and-digest.md` |
| Schema, config keys, migration, wiring | `2026-09-04-schema-config-migration.md` |
| Testing strategy | `2026-09-04-testing.md` |

## 7. Glossary

- **Tick** — one daemon loop iteration.
- **Signal** — a single reading: foreground app class, fs staleness, checkpoint
  answer.
- **Drifting tick** — a tick whose signals cross the drift threshold.
- **Drift episode** — a contiguous run of drifting ticks; one `drift_events` row.
- **Nudge** — a user-facing notification (OS + inline) asking the user to
  refocus; has an escalation level 1–3.
- **Checkpoint** — a once-per-session self-report prompt (`on task? [y/n]`).
- **BYOK** — bring your own key; user supplies an Anthropic or OpenRouter API key.

## 8. Build order (feeds the implementation plan)

1. Schema + config additions + `internal/report` refactor (no behaviour change).
2. `internal/ipc` socket server/client + `pomo daemon` skeleton (idle loop only).
3. `internal/watch` (darwin first) + drift scoring + `drift_events` writes.
4. `internal/notify` + `internal/nudge` escalation + canned nudges.
5. `internal/ai` provider layer; wire AI nudge lines.
6. TUI: `screenPrompt` slash palette + `/review` `/insights` `/drift`.
7. TUI: `screenChat` streaming.
8. `pomo digest` + daemon weekly auto-write.
9. OS unit install/uninstall in `pomo daemon start/stop`.
10. linux `watch` implementation.
