// Package openaicompat implements the ADK model.LLM interface on top of the
// OpenAI Chat Completions API (/chat/completions).
//
// ADK's built-in model/openaimodel targets the OpenAI Responses API
// (/responses), which many OpenAI-compatible gateways (including the
// t-tech llm-proxy used by this project) do not implement. This package is
// the project's reusable extension point for such gateways.
//
// Milestone 1 limitations (see docs/adk-chat-completions-model.md):
//   - stream is accepted but ignored: every call performs one non-streaming
//     request and yields exactly one final LLMResponse.
//   - non-text genai parts are skipped (no tools in M1).
//   - GenerateContentConfig fields other than SystemInstruction are ignored.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

// Options configures NewModel.
type Options struct {
	Model      string
	BaseURL    string
	APIKey     string
	Headers    map[string]string
	HTTPClient *http.Client
}

// Model is a model.LLM backed by an OpenAI-compatible chat completions
// endpoint. Construct with NewModel.
type Model struct {
	name   string
	client openai.Client
}

// compile-time interface check.
var _ model.LLM = (*Model)(nil)

// NewModel builds a Model from opts.
func NewModel(opts Options) *Model {
	clientOpts := []option.RequestOption{
		option.WithAPIKey(opts.APIKey),
		option.WithBaseURL(opts.BaseURL),
	}
	for k, v := range opts.Headers {
		clientOpts = append(clientOpts, option.WithHeader(k, v))
	}
	if opts.HTTPClient != nil {
		clientOpts = append(clientOpts, option.WithHTTPClient(opts.HTTPClient))
	}
	return &Model{
		name:   opts.Model,
		client: openai.NewClient(append(clientOpts, option.WithMiddleware(normalizeErrorBodies))...),
	}
}

// normalizeErrorBodies rewrites non-standard gateway error bodies so the
// SDK's *openai.Error parsing succeeds. OpenAI-compatible gateways often
// return {"error":"plain string"} (or no error object at all) where the
// OpenAI API returns {"error":{"message":...}}; without this, openai-go
// fails to decode the body and the caller loses the status code and
// message.
func normalizeErrorBodies(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	resp, err := next(req)
	if err != nil || resp == nil || resp.StatusCode < 400 {
		return resp, err
	}
	const maxBody = 64 << 10
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if rewritten := normalizeErrorJSON(body); !bytes.Equal(rewritten, body) {
		resp.Body = io.NopCloser(bytes.NewReader(rewritten))
		resp.Header.Set("Content-Type", "application/json")
	} else {
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}
	return resp, nil
}

// normalizeErrorJSON maps an error body to {"error":{"message":...}} unless
// it already has an error object. It returns the input unchanged on
// unparseable input.
func normalizeErrorJSON(body []byte) []byte {
	var probe struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return body
	}
	if len(probe.Error) > 0 {
		var obj map[string]json.RawMessage
		if json.Unmarshal(probe.Error, &obj) == nil && obj != nil {
			return body // already an object: standard-ish shape
		}
		var msg string
		if json.Unmarshal(probe.Error, &msg) != nil {
			msg = string(probe.Error)
		}
		out, _ := json.Marshal(map[string]any{"error": map[string]any{"message": msg}})
		return out
	}
	out, _ := json.Marshal(map[string]any{"error": map[string]any{"message": string(body)}})
	return out
}

// Name returns the configured model id.
func (m *Model) Name() string { return m.name }

// GenerateContent maps an ADK LLMRequest to one chat-completions request and
// yields a single final LLMResponse. stream is ignored in M1.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		params := openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(m.Name()),
			Messages: m.toMessages(req),
		}
		resp, err := m.client.Chat.Completions.New(ctx, params)
		if err != nil {
			yield(nil, m.wrapErr(err))
			return
		}
		if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
			yield(nil, fmt.Errorf("openai-compat %s: empty response", m.Name()))
			return
		}
		yield(&model.LLMResponse{
			Content: &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{{Text: resp.Choices[0].Message.Content}},
			},
			FinishReason: genai.FinishReasonStop,
		}, nil)
	}
}

// toMessages maps genai contents to chat-completions messages: system
// instruction first, then the conversation. Role "model" maps to assistant,
// everything else to user. Non-text parts are skipped (M1: no tools).
func (m *Model) toMessages(req *model.LLMRequest) []openai.ChatCompletionMessageParamUnion {
	var msgs []openai.ChatCompletionMessageParamUnion
	if req.Config != nil && req.Config.SystemInstruction != nil {
		var sb strings.Builder
		for _, p := range req.Config.SystemInstruction.Parts {
			if p != nil && p.Text != "" {
				sb.WriteString(p.Text)
				sb.WriteString("\n")
			}
		}
		if s := strings.TrimRight(sb.String(), "\n"); s != "" {
			msgs = append(msgs, openai.SystemMessage(s))
		}
	}
	for _, c := range req.Contents {
		if c == nil {
			continue
		}
		var sb strings.Builder
		for _, p := range c.Parts {
			if p != nil && p.Text != "" {
				sb.WriteString(p.Text)
				sb.WriteString("\n")
			}
		}
		s := strings.TrimRight(sb.String(), "\n")
		if s == "" {
			continue
		}
		if c.Role == "model" {
			msgs = append(msgs, openai.AssistantMessage(s))
		} else {
			msgs = append(msgs, openai.UserMessage(s))
		}
	}
	return msgs
}

// wrapErr decorates transport/API errors with the model name and, for HTTP
// error statuses, the status code. *openai.Error's own message already
// includes the method, URL, status text, and body.
func (m *Model) wrapErr(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf("openai-compat %s: HTTP %d: %w", m.Name(), apiErr.StatusCode, err)
	}
	return fmt.Errorf("openai-compat %s: %w", m.Name(), err)
}
