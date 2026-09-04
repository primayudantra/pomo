# Specs

Design specs for the **ADHD Focus Layer** — drift detection + slash-command
review/chat surface for pomo.

Read in this order:

1. [`2026-09-04-adhd-focus-overview.md`](2026-09-04-adhd-focus-overview.md) — problem, persona, architecture, build order. Start here.
2. [`2026-09-04-daemon-drift-detection.md`](2026-09-04-daemon-drift-detection.md) — `pomo daemon`, `internal/watch`, drift scoring, `drift_events`.
3. [`2026-09-04-nudge-and-ai.md`](2026-09-04-nudge-and-ai.md) — nudge escalation, `internal/notify`, `internal/ai` (Anthropic + OpenRouter BYOK).
4. [`2026-09-04-slash-palette-and-chat.md`](2026-09-04-slash-palette-and-chat.md) — `/` palette, `screenChat`, `internal/ipc` protocol.
5. [`2026-09-04-review-and-digest.md`](2026-09-04-review-and-digest.md) — `internal/report`, `/review` / `/insights`, `pomo digest`, weekly auto-write.
6. [`2026-09-04-schema-config-migration.md`](2026-09-04-schema-config-migration.md) — schema deltas, config keys, SQLite concurrency, backward compat.
7. [`2026-09-04-testing.md`](2026-09-04-testing.md) — first tests for the repo, `make test`, manual checklist.

Status: approved for planning (2026-09-04).

## Progress

- Chunk 1 (schema + `internal/report` + `pomo review`): **done** — `plans/2026-09-04-report-foundation.md`.
- Chunk 2 (config keys + `internal/ipc` + `internal/ai`): **done** — `plans/2026-09-04-daemon-prep.md`.
- Chunk 3 (daemon + `internal/watch` + drift scoring + `internal/nudge` + `internal/notify`): **done** — `plans/2026-09-04-drift-daemon.md`.
- Chunk 4 (TUI slash palette + `/chat` + `pomo digest` + daemon wiring): **done** — `plans/2026-09-04-tui-slash-surface.md`.

ADHD focus layer complete. Deferred items (backlog, see the overview spec §3
Non-goals): launchd/systemd unit, Linux foreground watching, browser-tab
attribution under denied automation permission.
