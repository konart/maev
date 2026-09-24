# agent_soup

LLM-agent lab. Milestone 1: a terminal chat (`agent_soup`) that talks to one
OpenAI-compatible model through the Google Agent Development Kit (ADK) for
Go, configured via an omp-style `models.yml`.

Status: **milestone 1 complete** — TUI chat, multi-turn memory, offline test
suite, agent-facing docs. No streaming, scrollback, persistence, or tools yet
(see AGENTS.md milestones).

## Quickstart

```bash
cp models.example.yml models.yml   # then edit: set apiKey or export TTECH_LLM_KEY
go run ./cmd/agent_soup
```

- Header shows the active pair, e.g. `agent_soup · tbank_openai_compat/tgpt/text.instant.sota · ctrl+c quit`.
- Type a message + `enter`; the agent's reply appears in the history.
- `ctrl+c` quits.

Flags:

```text
-config string    path to the models config file (default "models.yml")
-provider string  provider key (default: alphabetically first in the file)
-model string     model id (default: the provider's first listed model)
```

Full config reference: [docs/configuration.md](docs/configuration.md).

## Architecture (short)

```
TUI (bubbletea) → internal/chat (ADK runner + in-memory session)
  → internal/openaicompat (ADK model.LLM over POST /chat/completions)
    → OpenAI-compatible gateway (llm-proxy)
```

`internal/config` loads omp-compatible `models.yml`. Deep dive:
[docs/architecture.md](docs/architecture.md).

## Development

```bash
go build ./... && go vet ./...
go test ./...    # fully offline (httptest); no network needed
```

- `models.yml` is gitignored — never commit keys.
- Docs live in flat pages under `docs/`; any behavior change updates the
  relevant page and `AGENTS.md`.

Agents (human or LLM) continuing this repo: read [AGENTS.md](AGENTS.md)
first.
