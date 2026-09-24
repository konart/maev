# internal/tui — bubbletea chat UI

The TUI is a single bubbletea `Model` (charm.land bubbletea / bubbles /
lipgloss v2) showing a chat history viewport, a
waiting indicator, and one input line. The history contains **only** user
queries, agent replies, and errors.

## Construction and lifecycle

```go
func New(c *chat.Chat, ctx context.Context, providerModel string) Model
```

- `c` — the conversation; `Send` is called from a bubbletea command
  goroutine, never from `Update` directly.
- `ctx` — cancellation scope owned by `main`; cancelling aborts in-flight
  requests after the program exits.
- `providerModel` — header label, `"provider/model"`.

`main` runs it with `tea.NewProgram(model)`; the alternate screen is opted
into by the view itself (`View.AltScreen = true` on the returned
`tea.View`). `Init()` focuses the input (returns the cursor-blink command
from `input.Focus()`).

## State

```go
type entryKind int // entryUser, entryAgent, entryError

type Model struct {
    chat          *chat.Chat
    ctx           context.Context
    providerModel string
    vp            viewport.Model  // chat history (bubbles v2: resize via SetWidth/SetHeight)
    input         textinput.Model // prompt "> ", placeholder "Type a message…"
    spin          spinner.Model   // spinner.Dot
    entries       []entry         // kind + text log, source of viewport content
    waiting       bool            // a Send is in flight
    width, height int
}
```

## Message flow

```
tea.KeyPressMsg Enter (idle, non-empty input)
  └─ append entryUser, clear input, waiting=true
  └─ tea.Batch(spinner start, submitCmd)          // goroutine
       submitCmd = chat.Send(ctx, text) → agentReplyMsg{reply, err}

agentReplyMsg
  └─ waiting=false
  └─ append entryAgent (reply) or entryError (err.Error())
  └─ re-render entries, viewport GotoBottom

spinner.TickMsg
  └─ waiting: advance spinner, re-tick (spinner.Update chains the next tick)
  └─ !waiting: dropped — the chain stops after the reply arrives

ctrl+c → tea.Quit
WindowSizeMsg → vp.SetWidth(w), vp.SetHeight(h-4), re-render, GotoBottom
```

Enter while `waiting` or with an empty input is a no-op (returns no cmd).
Typing continues to work while waiting — only submission is gated.

## Layout (`View`)

```
agent_soup · <providerModel> · ctrl+c quit     ← header (bold white)
<viewport: rendered entries>                   ← height = height-4
<spinner> thinking…                            ← only while waiting (row always reserved)
> <input with cursor>                          ← textinput, prompt "> "
```

The `height-4` viewport budget = header + spacer + waiting row + input.
The waiting row is rendered empty when idle so the input line never jumps.

## Entry rendering

Entries are re-rendered wholesale (`renderEntries`) on every append/resize:
each entry is `prefix + text` in its color, wrapped to `width-2` via
lipgloss `Width(n)`, entries separated by one blank line.

| Kind | Prefix | Color |
| --- | --- | --- |
| user | `You ▸ ` | `#5FD7FF` (cyan) |
| agent | `Agent ▸ ` | `#FF5FD7` (magenta) |
| error | `error ▸ ` | `#FF5F5F` (red) |

## Keybindings

| Key | Action |
| --- | --- |
| printable keys | edit input (standard textinput: cursor movement, word jumps, delete) |
| `enter` | submit input as a user turn (ignored while waiting/empty) |
| `ctrl+c` | quit |

## Testing

`internal/tui/tui_test.go` drives `Update`/`View` directly with no
terminal (see `docs/architecture.md` for the project testing strategy).

## M1 limitations

- Viewport is **bottom-locked**: no PgUp/PgDn scrollback (all keys but
  `enter`/`ctrl+c` go to the input). Scrolling is milestone 2.
- No streaming display — the reply appears in full when `Send` returns.
- No color-scheme or prefix configuration.
