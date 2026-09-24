# Configuration (models.yml)

`agent_soup` reads a single YAML file (default `./models.yml`, override with
`-config <path>`) in the same format as omp's `~/.omp/agent/models.yml`,
restricted to the subset agent_soup supports: OpenAI-compatible
(chat completions) providers. `models.example.yml` in the repo root is the
committed template; real `models.yml` is gitignored and never committed.

## File format

```yaml
providers:
  tbank_openai_compat:
    baseUrl: "https://llm-proxy.t-tech.team/v1"
    api: openai-completions
    apiKey: ${TTECH_LLM_KEY}   # literal key or ${ENV_VAR}
    auth: apiKey
    headers:
      X-Custom: value
    models:
      - id: "tgpt/text.instant.sota"
        name: "tgpt/text.instant.sota"
```

### Keys

| Key | Type | Meaning |
| --- | --- | --- |
| `providers` | map | required, must be non-empty; each key is a provider name |
| `baseUrl` | string | required, non-empty; the OpenAI-compatible API root (e.g. `https://host/v1`) |
| `api` | string | `""` or `openai-completions`; anything else is rejected |
| `apiKey` | Secret | required, non-empty after resolution; see below |
| `auth` | string | `""` or `apiKey`; anything else is rejected |
| `authHeader` | bool | parsed but **ignored** (omp anthropic-only flag) |
| `disableStrictTools` | bool | parsed but **ignored** (omp anthropic-only flag) |
| `headers` | map | extra HTTP headers sent on every request to this provider |
| `models` | list | required, non-empty; ordered |
| `models[].id` | string | model id sent as the chat-completions `model` field |
| `models[].name` | string | display name (parsed; agent_soup currently shows `id` everywhere) |

Parsing is strict: unknown keys fail with
`field <name> not found in type config.Provider`.

### The `Secret` type (apiKey)

`apiKey` accepts either

- a **plain scalar** (literal key or `${ENV_VAR}` reference), or
- a **one-element flow sequence** — omp writes `apiKey: [Secrets33]`; the
  element is unwrapped and used as the scalar.

At resolution time:

1. If the value matches `Secrets<n>` / `[Secrets<n>]` (omp's secret-store
   placeholders), resolution fails: omp's secret store is not accessible to
   agent_soup.
2. Otherwise the value goes through `os.ExpandEnv`: `${VAR}` references are
   replaced from the environment (`$$` escapes a literal `$`).
3. The expanded value must be non-empty.

### Provider/model selection

`agent_soup -provider P -model M` resolves:

- Empty `-provider` → the **alphabetically-first** provider key (map order
  in YAML is irrelevant; agent_soup sorts keys).
- Empty `-model` → the provider's **first listed** model (`models` list
  order).
- Non-empty names must match exactly.

## Exact error messages

All errors exit the binary with status 1 on stderr before the TUI starts:

| Condition | Error (substrings) |
| --- | --- |
| file missing/unreadable | `read config <path>: ...` |
| YAML syntax / unknown field | `parse config <path>: ...` / `field <x> not found ...` |
| empty `providers` | `config <path>: no providers defined` |
| unknown provider | `unknown provider "P" (available: [a b c])` |
| unsupported `api` | `provider P: api "X" not supported, only "openai-completions"` |
| unsupported `auth` | `provider P: auth "X" not supported, only "apiKey"` |
| empty `baseUrl` | `provider P: baseUrl is empty` |
| no models | `provider P: no models defined` |
| unknown model | `provider P: unknown model "M" (available: [m1 m2])` |
| omp placeholder | `provider P: omp secret store placeholder "Secrets33" is not supported; use a literal key or ${ENV_VAR}` |
| empty key | `provider P: apiKey is empty` |

## Environment expansion example

```yaml
apiKey: ${TTECH_LLM_KEY}
```

with `TTECH_LLM_KEY` exported in the shell resolves to the key's value;
an unset variable expands to the empty string and then fails the
`apiKey is empty` check.
