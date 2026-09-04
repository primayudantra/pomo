# Nudge Escalation + AI Provider Layer

Date: 2026-09-04
Parent: `2026-09-04-adhd-focus-overview.md`

## 1. `internal/nudge`

Per-session state machine, held in the daemon's `activeState`. No DB persistence
(nudges are ephemeral; the drift record in `drift_events` is the durable part).

```go
package nudge

type Level int // 1 soft, 2 firm, 3 offer-exit

type State struct {
    Level        Level
    Count        int
    LastAt       time.Time
    lastLines    []string // for de-dup against AI output
}

type Context struct {
    Task        string
    Tag         string
    DistractApp string        // "Google Chrome" or "" 
    DistractDetail string     // "reddit.com" or ""
    DriftMinutes int
    SessionElapsed time.Duration
    Level       Level
    Hour        int           // 0-23 local
}

type Decision struct {
    Fire   bool
    Level  Level
    Text   string             // resolved line (AI or canned)
    Actions []string          // key hints: ["b","r","d"] etc.
}

func (s *State) Evaluate(episodeOpen bool, driftMinutes int, cfg Config, n Nudger) Decision
```

### Rules (`Evaluate`)

- Fire only when a drift episode is currently open.
- Gate: `time.Since(s.LastAt) >= cfg.MinGap` (default 5m) **and**
  `s.Count < cfg.MaxPerSession` (default 4).
- Level selection: `s.Level = min(3, s.Count+1)` at fire time (first fire →
  level 1, etc.). A checkpoint answered `y` (delivered to the daemon via IPC)
  calls `s.Reset()` → `Level = 1`, `Count = 0` (but `MinGap` still applies).
- On fire: build `Context`, call `n.Line(ctx)`; if it errors or returns a line in
  `s.lastLines`, use `canned(level, ctx)`. Append to `lastLines` (keep last 3).
  Set `LastAt`, `Count++`.
- Actions by level:
  - 1: `["snooze","drifted"]` → keys `[s.a snooze = +MinGap] [d]`
  - 2: `["refocus","drifted","break"]`
  - 3: `["break","refocus"]`

### Canned templates

```
L1: "still on %s?"                                   // %s task
L2: "%d min on %s — %s still the plan?"              // drift, app, task
L3: "rough stretch. want a real break or a reset?"
```
(no distract app known → L2 falls back to `"%d min drifting — %s still the plan?"`)

### Actions handled by the TUI (via IPC round-trip)

- `d` "I drifted" → daemon force-closes the open episode with `trigger` unchanged,
  `detail += " (self-reported)"`, resets `distractSince`; no session state change.
- `r` "refocus" → same as `d` plus `nudge.State.Reset()`.
- `b` "break" → TUI ends the current focus session as `interrupted` and starts a
  break (existing timer flow); daemon sees the session row change on its next
  tick and drops `activeState`.
- `snooze` → daemon bumps `LastAt` forward by `cfg.MinGap`.

## 2. `internal/notify`

```go
package notify
func Send(title, body string) error   // best effort, never blocks
```

- darwin: `terminal-notifier -title <t> -message <b> -group pomo` if on PATH,
  else `osascript -e 'display notification ...'`. **Never** `display dialog`
  (blocking — forbidden by the harness rules and bad UX).
- linux: `notify-send -a pomo <t> <b>`.
- Missing binary → return nil (the inline IPC banner is the fallback path).
- 2s exec timeout.

## 3. `internal/ai`

One package, three interfaces, one shared HTTP client, provider switch.

```go
package ai

type Provider string // "", "anthropic", "openrouter"

type Config struct {
    Provider Provider
    Key      string
    Model    string        // default per provider
    baseURL  string        // test override
}

type NudgeContext = nudge.Context   // or a local mirror to avoid import cycle

type Nudger  interface { Line(ctx NudgeContext) (string, error) }
type Recapper interface { Recap(ctx RecapContext) (string, error) }
type Chatter  interface { Stream(ctx context.Context, msgs []Msg, out func(delta string)) error }

func New(cfg Config) (Nudger, Recapper, Chatter)
```

- `New` with `Provider == ""` returns canned/no-op implementations:
  - `Nudger` → returns `ErrNoProvider` so `nudge` uses canned templates.
  - `Recapper` → returns `""` , `ErrNoProvider`.
  - `Chatter` → `Stream` returns `ErrNoProvider` immediately.
- Otherwise returns HTTP-backed implementations sharing:
  - `http.Client{Timeout: 0}` (per-call `context.WithTimeout`).
  - Default model `claude-haiku-4-5` (anthropic) / `anthropic/claude-3.5-haiku`
    (openrouter), overridable by `ai.model`.

### Anthropic transport

`POST https://api.anthropic.com/v1/messages`
Headers: `x-api-key: <key>`, `anthropic-version: 2023-06-01`,
`content-type: application/json`.
Body: `{model, max_tokens, system, messages:[{role:"user",content}], stream}`.
Non-stream response: `.content[0].text`. Stream: SSE, `content_block_delta`
events, `.delta.text`.

### OpenRouter transport

`POST https://openrouter.ai/api/v1/chat/completions`
Headers: `Authorization: Bearer <key>`, `content-type: application/json`,
`HTTP-Referer: https://github.com/<owner>/pomo`, `X-Title: pomo`.
Body: OpenAI shape `{model, max_tokens, messages:[{role:"system"...},{role:"user"...}], stream}`.
Non-stream: `.choices[0].message.content`. Stream: SSE, `.choices[0].delta.content`,
terminates on `data: [DONE]`.

### Per-call parameters

| Call | max_tokens | timeout | stream | system prompt |
|---|---|---|---|---|
| `Line` (nudge) | 120 | 3s | no | "You are a terse, non-judgmental focus buddy for a developer. Reply with ONE sentence, 15 words max, no emoji, no exclamation marks." |
| `Recap` | 250 | 8s | no | "You summarise a developer's focus session data. 3 short sentences, then one line starting with '→ Try:' with a concrete suggestion. No praise, no fluff." |
| `Stream` (chat) | 400 | 30s | yes | "You are a terse focus coach for a developer. Ground every answer in the supplied session and drift data. 4 sentences max. Be concrete." |

User-message payloads are compact `key: value` lines built from the context
structs — never raw session notes unless the caller passed `--include-notes`
(review only).

### Failure behaviour

- Any transport error, non-2xx, or timeout → return the typed error; callers
  degrade (canned nudge / skip recap block / show `[error: …]` in chat with any
  partial text already streamed).
- The daemon calls `Line` with its 3s context on the tick goroutine; a slow AI
  cannot stall the loop beyond 3s. If this proves noticeable, move the call to a
  short-lived goroutine whose result is applied on the next tick (noted as a
  follow-up, not built now).

## 4. Config keys

`ai.provider`, `ai.key`, `ai.model`, `nudge.enabled`, `nudge.max_per_session`,
`nudge.min_gap`. Full table in the schema spec.

## 5. Security note

`ai.key` is stored in the `config` table in `~/.pomo/pomo.db` in plaintext
(consistent with all other config). `pomo config` output and the `/settings`
screen print a one-line warning: `ai.key is stored locally in plaintext at
~/.pomo/pomo.db`. Accepted for v1. `pomo config` masks the value on display
(`sk-…last4`).
