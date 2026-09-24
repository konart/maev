# Decision log

Dated, numbered, append-only record of non-trivial project decisions:
anything a future contributor would need explained (why X exists, why not
the obvious alternative, why a version is pinned). Trivial changes don't
get an entry; new decisions are appended in the same change that
implements them.

## 1. Custom `model.LLM` over Chat Completions

2026-09-24 — `internal/openaicompat`

t-proxy answers `POST /v1/responses` with `400 {"error":"unsupported endpoint"}`;
ADK's `openaimodel` needs the Responses API. Compat gateways generally
implement only `/chat/completions` (probed: 200 with standard
choices/usage). One file owns the mapping, so future gateways plug in there.

## 2. Non-streaming in M1

2026-09-24

`GenerateContent(..., stream)` ignores `stream` and yields one final
response; runner uses `StreamingModeNone`. Keeps M1 trivially correct;
streaming is M2 via `client.Chat.Completions.NewStreaming` + `Partial`
events.

## 3. Non-text genai parts are skipped

2026-09-24

No tools exist in M1, so there is nothing to map; the skip point is the
single place to extend.

## 4. omp config subset with env expansion

2026-09-24

Same YAML shape as `~/.omp/agent/models.yml`; `apiKey` accepts scalar or
omp's one-element flow list; `${ENV_VAR}` expanded at use; omp
`[SecretsN]` placeholders rejected (no access to omp's store);
`authHeader`/`disableStrictTools` parsed-but-ignored (anthropic-only
flags).

## 5. ADK in-memory session per process

2026-09-24

`runner.NewInMemory` + UUID session gives multi-turn memory with zero
infra; no persistence (accepting: conversation dies with the process).

## 6. Module `github.com/konart/agent_soup`, single binary `cmd/agent_soup`

2026-09-24

No remote exists yet; path rename is mechanical.

## 7. omp-style flat `docs/` pages

2026-09-24

Kebab-case, one subsystem per page, README/AGENTS link in. Rule: any
subsystem gaining nontrivial behavior gets/updates its page in the same
change.

## 8. Dependency versions: track latest majors, log only exceptions

2026-09-24

`go.mod` is the single source of truth for the Go toolchain and dependency
versions; docs never mirror version lists (they drift immediately and
review can't catch it). Default policy: stay on the latest stable major
versions (`go get -u` + `go mod tidy`, keep code migrated).

`docs/architecture.md`'s toolchain/dependency section is reserved for
non-trivial cases only: a deliberate pin to an older major (project not
ready to migrate) or a known blocking issue in a newer version. Each such
exception gets a decision-log entry here explaining why. None currently.
