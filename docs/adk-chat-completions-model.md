# internal/openaicompat — ADK model over Chat Completions

This package implements ADK's `model.LLM` interface
(`google.golang.org/adk/v2/model`) on top of the **OpenAI Chat Completions
API** (`POST {baseUrl}/chat/completions`) using `openai-go/v3`.

## Why it exists

ADK ships `model/openaimodel`, but it targets the OpenAI **Responses API**
(`/responses`). The t-tech llm-proxy — like most gateways that call
themselves "OpenAI-compatible" — does not implement it:

```
POST https://llm-proxy.t-tech.team/v1/responses
→ 400 {"error":"unsupported endpoint"}

POST https://llm-proxy.t-tech.team/v1/chat/completions
→ 200, standard choices/usage shape
```

(probed live against the proxy). So agent_soup bridges ADK's genai-shaped
requests onto chat completions itself. This package is the project's
reusable extension point: any future OpenAI-compatible backend plugs in
here.

## Contract

```go
type Options struct {
    Model      string            // model id
    BaseURL    string            // API root, e.g. https://host/v1
    APIKey     string            // sent as Authorization: Bearer <key>
    Headers    map[string]string // extra headers on every request
    HTTPClient *http.Client      // optional (tests inject transports)
}

func NewModel(opts Options) *Model
func (m *Model) Name() string // opts.Model
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error]
```

`*Model` satisfies `model.LLM` (compile-time checked with `var _ model.LLM`).

## Request mapping (genai → chat completions)

| ADK side | Chat-completions side |
| --- | --- |
| `req.Config.SystemInstruction` (text parts, joined with `\n`) | one leading `system` message |
| `req.Contents[i].Role == "model"` | `assistant` message |
| any other role (`user`, …) | `user` message |
| all text parts of one content (joined with `\n`) | message `content` string |

- A content whose text parts are all empty is skipped entirely.
- **Non-text parts are skipped** (no tool/function-call parts exist in M1;
  see "Extending" below).
- `req.Config` fields other than `SystemInstruction` (temperature,
  max output, tools, …) are **ignored** in M1.
- Client options: `option.WithAPIKey`, `option.WithBaseURL`, one
  `option.WithHeader` per configured header, optional
  `option.WithHTTPClient`.

## Response mapping

The HTTP response yields exactly **one** final `*model.LLMResponse`:

```go
&model.LLMResponse{
    Content:      &genai.Content{Role: "model", Parts: []*genai.Part{{Text: choices[0].message.content}}},
    FinishReason: genai.FinishReasonStop,
}
```

## Error semantics

The iterator yields `(nil, err)` — at most one error, always last:

- HTTP error status (openai-go `*openai.Error`):
  `openai-compat <model>: HTTP <code>: <sdk error>` — the SDK error text
  already includes method, URL, status text, and body.
- `choices` empty or first choice content empty:
  `openai-compat <model>: empty response`.
- Any other transport/decode error:
  `openai-compat <model>: <err>`.

### Non-standard gateway error bodies

Compat gateways often return error bodies the OpenAI API never produces:
the t-proxy answers `401 {"error":"fail parse token claims: ..."}` — a
**string** `error` field (OpenAI uses an object). openai-go then fails to
decode the body and the raw JSON error hides the status and message. To
keep HTTP failures informative, `NewModel` installs an
`option.WithMiddleware` (`normalizeErrorBodies`) that rewrites any ≥400
body lacking an `error` object into `{"error":{"message": <message>}}`
(from the string field, or the whole body if absent) before the SDK parses
it. Status code, URL, and message survive; SDK retry behavior for 5xx is
unchanged (the response passes through with its status intact).

## M1 limitations (deliberate)

- **`stream` is accepted but ignored.** Every call performs one
  non-streaming `client.Chat.Completions.New` and yields one final
  response. The ADK runner is configured with
  `agent.StreamingModeNone`, so nothing requests streaming today.
- Non-text genai parts are skipped (no tools in M1).
- The proxy's non-standard `reasoning_content` field on choices is
  ignored.

## Extending

- **Streaming (M2)**: switch to
  `client.Chat.Completions.NewStreaming(ctx, params)` (already available
  in openai-go v3), map each chunk's delta text to a
  `Partial: true` LLMResponse, and finish with a `TurnComplete`/final
  non-partial response. The `stream` parameter becomes meaningful; the
  runner side switches to a streaming `RunConfig`.
- **Tool calls**: map genai function-call parts to
  `openai.ToolMessage`/assistant `tool_calls`, and surface
  `FunctionCall` parts in the response. The skipping logic in
  `toMessages` is the single place to change.
- **Config passthrough**: copy `req.Config.Temperature` etc. into
  `ChatCompletionNewParams` when needed.
