# internal/chat — ADK runtime wiring

`internal/chat` owns the agent-side runtime: it wraps the
`internal/openaicompat` model in an ADK `llmagent`, runs it through an
in-memory `runner.Runner`, and exposes a two-method conversation API.

## Construction

```go
func New(r config.Resolved) (*Chat, error)
```

- Model: `openaicompat.NewModel` built from the resolved config
  (model id, baseUrl, key, headers).
- Agent: `llmagent.New(llmagent.Config{`
  `Name: "chat_agent", Model: m, Description: "Simple chat agent",`
  `Instruction: "You are a helpful assistant."})`.
  No tools, no sub-agents, no callbacks.
- Runner: `runner.NewInMemory("agent_soup", agent)` — ADK wires
  in-memory session, artifact, and memory services and sets
  `AutoCreateSession`. The session is created lazily with
  `sessionID = uuid.NewString()`; userID is the constant `"user"`.
  Nothing is persisted: conversation state lives for the process lifetime
  only.

## Send

```go
func (c *Chat) Send(ctx context.Context, text string) (string, error)
```

1. Builds `&genai.Content{Role: "user", Parts: []*genai.Part{{Text: text}}}`.
2. Iterates `c.runner.Run(ctx, "user", sessionID, content,`
   `agent.RunConfig{StreamingMode: agent.StreamingModeNone})` — an
   `iter.Seq2[*session.Event, error]`.
3. Accumulates text from `ev.Content.Parts[*].Text` for events where
   `ev.Content != nil && ev.Content.Role == "model" && !ev.Partial`.
   The `Partial` guard matters only once streaming exists (M1 never
   produces partials); the role guard skips the runner's echo of the
   user's own message.
4. Iterator error → returned wrapped as `run agent: <err>` (after
   discarding partial accumulation from that turn).
5. Empty accumulation → `model returned empty response`.

## Multi-turn memory

The in-memory session service stores every user content and every final
model response. On each `Send`, the runner hands the agent the full session
contents, so the chat-completions request for turn *n* replays turns
`1..n-1` (user messages as `user`, prior replies as `assistant` — see the
mapping in
[adk-chat-completions-model.md](adk-chat-completions-model.md)).

This is regression-tested offline: `internal/chat/chat_test.go`
`TestMultiTurnMemory` asserts the second request body captured by an
`httptest` server contains `first question`, `reply one` (as role
`assistant`), and `second question`.

## Context semantics

`Send` runs under the caller's `context.Context`. In the TUI binary,
`main` creates `ctx, cancel := context.WithCancel(...)` and `cancel()`s
after the bubbletea program returns, aborting any in-flight HTTP request
when the UI exits.

## What is deliberately absent (M1)

- No persistence: killing the process drops the conversation.
- No session listing/resume; one Chat = one UUID session.
- No streaming surface: `Send` returns one string per turn.
