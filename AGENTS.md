# AGENTS.md

Operating manual for LLM agents (and humans) working in this repo. Read this
first. It states the current contract, the decisions behind it, and where
the depth lives.

## Current state — milestone 1 done

- Terminal chat TUI: input line + history showing only user queries, agent
  replies, and errors; waiting spinner; `ctrl+c` quit.
- One provider/model per run, selected by `-config/-provider/-model` from an
  omp-format `models.yml` (OpenAI-completions providers only).
- ADK Go 2.x runtime (`llmagent` + in-memory runner/session) with
  multi-turn conversation memory per process.
- Offline test suite per package (`httptest`); manual e2e against the real
  proxy documented in README.
- Docs: flat pages under `docs/` (one subsystem per page, plus the
  decision log) + README + this file.

## Architecture map

`cmd/agent_soup` (flags/wiring) → `internal/config` (models.yml subset) →
`internal/chat` (ADK runner + in-memory session) → `internal/openaicompat`
(ADK `model.LLM` over chat completions) → gateway `/chat/completions`;
`internal/tui` (bubbletea) drives `chat.Send` and renders entries.

Depth: [docs/architecture.md](docs/architecture.md) (overview),
[docs/configuration.md](docs/configuration.md) (config),
[docs/adk-chat-completions-model.md](docs/adk-chat-completions-model.md)
(custom model), [docs/chat-runtime.md](docs/chat-runtime.md) (ADK wiring),
[docs/tui.md](docs/tui.md) (UI), [docs/decisions.md](docs/decisions.md)
(decision log).

## Design decisions

The decision log lives in [docs/decisions.md](docs/decisions.md) — dated,
numbered, append-only. New non-trivial decisions get an entry there in the
same change that implements them.

## Known issues & compromises

- No streaming; reply appears at once (M2).
- Viewport bottom-locked; no scrollback (M2).
- No persistence; one process = one conversation (M3).
- One provider/model per run, chosen by flags; no picker (M2).
- Empty model reply surfaces as `model returned empty response` error entry.
- Proxy's `reasoning_content` field is ignored.
- `Send` returns the turn's text only; no usage/token accounting displayed.

## Milestones

- [x] **M1** — chat TUI on ADK 2.x, omp-subset config, chat-completions model,
  offline tests, docs (this state).
- [ ] **M2** — SSE streaming into the TUI, viewport scrolling, provider+model
  picker, YAML-driven defaults.
- [ ] **M3 (ideas)** — session persistence, tools/function calling,
  multi-agent setups.

## Conventions

- Tests per package, offline via `httptest`; no test touches the real proxy.
- Every behavior change updates the relevant `docs/` page, the decision
  log ([docs/decisions.md](docs/decisions.md)), and the milestones here in
  the same commit.
- `models.yml` is gitignored; never commit real keys.
- Error strings are user-facing (stderr on startup, red entries in TUI);
  keep them in sync with `docs/configuration.md`'s error table.
