# Architecture

agent_soup is an LLM-agent lab. Milestone 1 delivers a terminal chat with a
single OpenAI-compatible model, built on the Google Agent Development Kit
(ADK) for Go and the charmbracelet TUI stack. This page is the system
overview; each package has its own deep-dive page under `docs/`.

## Package map

| Package | Responsibility |
| --- | --- |
| `cmd/agent_soup` | `main`: flags, config → chat → TUI wiring, context lifecycle |
| `internal/config` | Loads `models.yml` (omp-compatible subset), resolves one provider+model |
| `internal/openaicompat` | ADK `model.LLM` implementation over the OpenAI Chat Completions API |
| `internal/chat` | ADK agent + runner wiring; one conversation with in-memory session |
| `internal/tui` | bubbletea model: chat viewport, input line, waiting spinner |

## Data flow

```
┌──────────────────────────── internal/tui (bubbletea) ─────────────────────┐
│  input line ──Enter──> entry log ──submitCmd (goroutine)                  │
│      ▲                                     │                              │
│   View()                             agentReplyMsg                        │
└──────────┬──────────────────────────────────▲─────────────────────────────┘
           │ Send(ctx, text)                  │ reply / error
┌──────────▼──────────────────── internal/chat ─────────────────────────────┐
│  runner.Run(user, session, content) — iterates session.Events             │
│  accumulates Role=="model" && !Partial text                               │
│      │                                                                    │
│  ADK runner.Runner (NewInMemory) ── llmagent "chat_agent"                 │
│      │ in-memory session service = multi-turn memory                      │
└──────▼────────────────────────────────────────────────────────────────────┘
┌── internal/openaicompat (model.LLM) ──────────────────────────────────────┐
│  GenerateContent: genai contents → chat-completions messages              │
│  POST {baseUrl}/chat/completions (openai-go v3 client)                    │
└──────▼────────────────────────────────────────────────────────────────────┘
   https://llm-proxy.t-tech.team/v1/chat/completions  (or any compat gateway)
```

One user turn = exactly one HTTP request in M1 (non-streaming): Enter in the
TUI runs `chat.Send` on a goroutine; `Send` appends the user content to the
ADK in-memory session, the runner invokes the agent, which calls
`openaicompat.Model.GenerateContent`; the single final response is stored by
the session service and returned as the reply.

## Why a custom model package

ADK's official `model/openaimodel` targets the OpenAI **Responses API**
(`POST /v1/responses`). The t-tech llm-proxy answers that endpoint with
`400 {"error":"unsupported endpoint"}` — it only implements
`/v1/chat/completions`. `internal/openaicompat` exists to bridge ADK's genai
request/response shapes onto chat completions. See
[adk-chat-completions-model.md](adk-chat-completions-model.md).

## Configuration

`models.yml` in the repo root (gitignored; `models.example.yml` is the
committed template) selects the provider, endpoint, key, and models in the
same format as omp's `~/.omp/agent/models.yml`. Only
`api: openai-completions` providers are supported. See
[configuration.md](configuration.md).

## Testing strategy

- All automated tests are **offline**: every HTTP-touching package
  (`openaicompat`, `chat`) is tested against `httptest.Server` with canned
  completions; `config` reads fixtures from `testdata/`; `tui` tests drive
  `Update`/`View` directly with no terminal.
- The multi-turn memory guarantee has a regression test: the second `Send`'s
  captured request body must contain the first turn's user and assistant
  messages (`internal/chat/chat_test.go`, `TestMultiTurnMemory`).
- The real proxy is exercised only by the manual end-to-end run described in
  `README.md`.

## Toolchain / dependencies

`go.mod` is the single source of truth for the Go toolchain and dependency
versions; policy is to track the latest stable majors (decision
[8](decisions.md#8-dependency-versions-track-latest-majors-log-only-exceptions)).
This section lists only non-trivial exceptions: deliberate pins to an
older major, or known blocking issues in a newer version (each with a
decision-log entry). None currently.
