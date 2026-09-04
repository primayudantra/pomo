# Pomo CLI — Personal Productivity Tracker

## 1. Overview

A terminal-based Pomodoro application for personal productivity.

The main purpose is to help the user:

* Start Pomodoro sessions directly from the terminal
* Track what they worked on
* Record completed, skipped, and interrupted sessions
* See historical productivity
* Understand where their focus time goes
* Track daily/weekly/monthly productivity

The application should be **local-first** and require no account or backend for MVP.

---

# 2. Core Philosophy

The CLI should answer three questions:

> **What am I working on?**

> **How much time did I spend on it?**

> **What did I actually accomplish?**

The app should avoid unnecessary productivity features.

No:

* Team management
* Social features
* Chat
* Complex project management
* Calendar
* Cloud sync

At least for the first version.

---

# 3. Basic Workflow

```text
Create / Select Task
        ↓
Start Pomodoro
        ↓
Focus
        ↓
Pomodoro Complete
        ↓
Take Break
        ↓
Repeat
        ↓
Review History
```

Typical usage:

```bash
pomo start
```

or:

```bash
pomo start "Implement account statement"
```

After finishing:

```text
🍅 Pomodoro completed!

Task:
Implement account statement

Duration:
25 minutes

Session #4 today

✓ Saved to history
```

---

# 4. CLI Commands

## Start Pomodoro

```bash
pomo start
```

Interactive task selection:

```text
What are you working on?

1. Account Statement
2. Reconciliation
3. Write Tests
4. Other

> 1
```

Or directly:

```bash
pomo start "Implement account statement"
```

---

## Start With Duration

```bash
pomo start --duration 50
```

Example:

```text
🍅 Focus Session

Task:
Implement account statement

Duration:
50:00

████████████████████████████
```

---

# 5. Timer Controls

During a Pomodoro:

```text
🍅 23:42

Implement account statement

[p] Pause
[r] Resume
[s] Skip
[q] Quit
```

Commands:

```bash
pomo pause
pomo resume
pomo stop
pomo skip
```

---

# 6. Tasks

Tasks don't need to become a full project management system.

Just enough to identify **what the Pomodoro was spent on**.

### Add

```bash
pomo task add "Implement account statement"
```

### List

```bash
pomo task list
```

Output:

```text
TODAY

ID   Task
1    Implement account statement
2    Reconciliation
3    Write test cases
4    Review PR
```

### Complete

```bash
pomo task done 1
```

### Delete

```bash
pomo task delete 1
```

---

# 7. Quick Start

For very fast usage:

```bash
pomo "Implement account statement"
```

This should be equivalent to:

```bash
pomo start "Implement account statement"
```

So the ideal workflow becomes:

```bash
$ pomo "Fix reconciliation"

🍅 Fix reconciliation

25:00
```

This should be the **fastest path** in the application.

---

# 8. Dashboard

Running:

```bash
pomo
```

opens the personal dashboard.

Example:

```text
╭──────────────────────────────────────────╮
│                 POMO                     │
│              Sep 02, 2026                │
├──────────────────────────────────────────┤
│                                          │
│  TODAY                                   │
│                                          │
│  Focus Time              2h 35m           │
│  Pomodoros               6                │
│  Completed Tasks         4                │
│                                          │
├──────────────────────────────────────────┤
│                                          │
│  CURRENT                                 │
│                                          │
│  🍅 Fix reconciliation                   │
│                                          │
│  18:32                                   │
│  ██████████████████░░░                   │
│                                          │
├──────────────────────────────────────────┤
│                                          │
│  TODAY'S ACTIVITY                        │
│                                          │
│  09:10  Account Statement       25m ✓     │
│  10:05  Reconciliation          25m ✓     │
│  11:00  Meeting                  1h       │
│  13:10  Reconciliation          25m ✓     │
│  14:00  Test Cases              25m ✓     │
│  14:40  Reconciliation          25m ▶     │
│                                          │
╰──────────────────────────────────────────╯
```

---

# 9. History

This is one of the most important features.

```bash
pomo history
```

Output:

```text
DATE        TIME    TASK                    DURATION   STATUS

Sep 02      09:10   Account Statement       25m        ✓
Sep 02      10:05   Reconciliation          25m        ✓
Sep 02      11:00   Meeting                 60m        -
Sep 02      13:10   Reconciliation          25m        ✓
Sep 02      14:00   Test Cases              25m        ✓
Sep 02      14:40   Reconciliation          25m        ▶
```

---

# 10. History Filters

### Today

```bash
pomo history --today
```

### Yesterday

```bash
pomo history --yesterday
```

### Last 7 days

```bash
pomo history --week
```

### Specific date

```bash
pomo history --date 2026-09-01
```

### Specific task

```bash
pomo history --task "Reconciliation"
```

---

# 11. Statistics

```bash
pomo stats
```

Example:

```text
PRODUCTIVITY

Today
──────────────────────────────

Focus Time        2h 35m
Pomodoros         6
Completed         5
Skipped           1

This Week
──────────────────────────────

Focus Time        14h 20m
Pomodoros         34
Completed         29
Skipped           5

Average Session
                  25m

Longest Focus
                  50m
```

---

# 12. Where Did My Time Go?

This should be one of the most useful features.

```bash
pomo stats --tasks
```

Output:

```text
FOCUS DISTRIBUTION — THIS WEEK

Reconciliation
████████████████████  5h 20m

Account Statement
████████████          3h 10m

Test Cases
████████              2h 05m

Documentation
████                  1h 10m

Other
██                    45m
```

This gives the user a historical answer to:

> "What the hell did I spend my week doing?"

---

# 13. Daily Summary

```bash
pomo today
```

Output:

```text
TODAY — SEP 02

Focus Time
2h 35m

Pomodoros
6

Tasks
────────────────────

Account Statement       2 pomodoros
Reconciliation          3 pomodoros
Test Cases              1 pomodoro

Timeline
────────────────────

09:10  Account Statement    25m
10:05  Reconciliation       25m
13:10  Reconciliation       25m
14:00  Test Cases            25m
14:40  Reconciliation       25m
```

---

# 14. Weekly Summary

```bash
pomo week
```

Output:

```text
WEEK — AUG 31 → SEP 06

Mon   ████████████    3h 10m
Tue   █████████       2h 30m
Wed   ███████████     2h 35m
Thu   ████████████    3h 20m
Fri   ████████        1h 45m

Total Focus
13h 20m

Pomodoros
32

Average / Day
2h 40m
```

---

# 15. Monthly Statistics

```bash
pomo month
```

Example:

```text
SEPTEMBER 2026

Total Focus       42h 15m
Pomodoros         101
Completed         89
Skipped           12

Average / Day     2h 06m

Most Focused Day
September 1       4h 10m

Most Focused Task
Reconciliation    12h 20m
```

---

# 16. Session History Data

Each Pomodoro should record:

```text
id
task
planned_duration
actual_duration
status
started_at
completed_at
pause_duration
created_at
```

Example:

```json
{
  "id": 102,
  "task": "Reconciliation",
  "planned_duration": 1500,
  "actual_duration": 1500,
  "status": "completed",
  "started_at": "2026-09-02T14:40:00+08:00",
  "completed_at": "2026-09-02T15:05:00+08:00"
}
```

---

# 17. Session Status

Use simple statuses:

```text
completed
cancelled
skipped
interrupted
```

Important distinction:

### Completed

User finished the Pomodoro.

### Skipped

User intentionally skipped it.

### Cancelled

User stopped the session.

### Interrupted

Terminal/process unexpectedly closed.

This is useful for understanding actual productivity rather than pretending every timer was completed.

---

# 18. Categories / Tags

Instead of making a complicated project system, allow optional tags.

Example:

```bash
pomo start "Fix reconciliation" --tag work
```

or:

```bash
pomo start "Read system design book" --tag learning
```

Then:

```bash
pomo stats --tag work
```

Output:

```text
WORK

Focus Time       24h 30m
Pomodoros        58
```

Potential tags:

```text
work
learning
personal
coding
meeting
admin
```

But tags should remain optional.

---

# 19. Notes

After a Pomodoro, optionally ask:

```text
🍅 Session completed!

Add a note? (optional)

> Fixed the balance calculation and added test cases.
```

Then history can show:

```bash
pomo history --details
```

```text
14:40 — Reconciliation
25 minutes ✓

Note:
Fixed the balance calculation and added test cases.
```

This turns the app into a lightweight **work journal**.

---

# 20. Data Storage

For personal MVP, don't use PostgreSQL.

Use:

**SQLite**

Example:

```text
~/.pomo/
    pomo.db
    config.json
```

SQLite is enough for:

* Tasks
* Pomodoro sessions
* History
* Statistics
* Tags
* Notes

No server required.

---

# 21. Architecture

```text
                 ┌─────────────────┐
                 │     Terminal    │
                 │                 │
                 │      pomo       │
                 └────────┬────────┘
                          │
                          ▼
                 ┌─────────────────┐
                 │   Application   │
                 │                 │
                 │ Timer           │
                 │ Task Manager    │
                 │ Statistics      │
                 │ History         │
                 └────────┬────────┘
                          │
                          ▼
                 ┌─────────────────┐
                 │     SQLite      │
                 │                 │
                 │ tasks           │
                 │ sessions        │
                 │ tags            │
                 │ notes           │
                 └─────────────────┘
```

Everything runs locally.

---

# 22. Recommended Tech Stack

Use **Go**.

```text
Go
├── Cobra       CLI commands
├── Bubble Tea  Interactive terminal dashboard
├── Lip Gloss   Terminal UI
└── SQLite      Local database
```

Build as a single binary:

```bash
pomo
```

No runtime dependency required.

---

# 23. Configuration

```bash
pomo config
```

Example:

```text
POMODORO

Focus                 25m
Short Break            5m
Long Break            15m
Sessions Before Long   4

Auto Start Break       true
Auto Start Focus       false

Sound                  true
Notifications          true
```

Change:

```bash
pomo config set focus 50m
```

---

# 24. MVP Commands

The initial release only needs:

```bash
pomo
pomo start
pomo pause
pomo resume
pomo stop

pomo task add
pomo task list
pomo task done
pomo task delete

pomo history
pomo today
pomo week
pomo month
pomo stats

pomo config
```

And the killer shortcut:

```bash
pomo "whatever I'm working on"
```

---

# 25. Example Real-Life Workflow

Start work:

```bash
$ pomo
```

Check dashboard:

```text
Today: 0h 00m
Pomodoros: 0

No active session.

[Enter] Start Pomodoro
```

Start:

```bash
$ pomo "Work on account statement"
```

25 minutes later:

```text
🍅 Completed!

Work on account statement
25 minutes

Add note? >
```

Next:

```bash
$ pomo "Write test cases"
```

End of day:

```bash
$ pomo today
```

```text
TODAY

Focus Time: 3h 20m
Pomodoros: 8

Account Statement      2h 05m
Test Cases             50m
Reconciliation         25m

Completed: 7
Skipped:   1
```

End of week:

```bash
$ pomo week
```

And you can immediately see:

```text
This week you focused for 17h 40m.

Top activities:

1. Account Statement    6h 20m
2. Reconciliation       4h 15m
3. Test Cases            3h 40m
4. Documentation         2h 10m
```

---

# 26. Definition of Done — MVP

The MVP is complete when the user can:

* Start a Pomodoro from terminal
* Associate it with a task
* Pause/resume/stop it
* Automatically save session history
* See today's sessions
* See historical sessions
* See weekly/monthly focus time
* See focus time grouped by task
* Add notes to completed sessions
* Configure Pomodoro duration
* Close the terminal and not lose historical data
* Run everything locally without an account/server

## Product Success Criteria

The application should make this workflow take **less than 5 seconds**:

```bash
pomo "Fix reconciliation"
```

And at any point, the user should be able to answer:

> **"What did I spend my time on today/this week?"**

without opening a browser or another productivity application.

