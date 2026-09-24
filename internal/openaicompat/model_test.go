package openaicompat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

const cannedReply = `{
  "id": "chatcmpl-1",
  "object": "chat.completion",
  "created": 1700000000,
  "model": "test-model",
  "choices": [
    {"index": 0, "message": {"role": "assistant", "content": "canned reply"}, "finish_reason": "stop"}
  ],
  "usage": {"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3}
}`

type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   string
}

func newServer(t *testing.T, status int, reply string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	cap := &capturedRequest{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		cap.Method, cap.Path, cap.Auth, cap.Body = r.Method, r.URL.Path, r.Header.Get("Authorization"), string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func testReq() *model.LLMRequest {
	return &model.LLMRequest{
		Model: "test-model",
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "hello"}}},
			{Role: "model", Parts: []*genai.Part{{Text: "hi there"}}},
			{Role: "user", Parts: []*genai.Part{{Text: "continue"}}},
		},
		Config: &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: "be brief"}}},
		},
	}
}

func TestGenerateContentRequestAndResponse(t *testing.T) {
	srv, cap := newServer(t, 200, cannedReply)
	m := NewModel(Options{
		Model:   "test-model",
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Headers: map[string]string{"X-Custom": "custom-value"},
	})
	if m.Name() != "test-model" {
		t.Fatalf("Name() = %q", m.Name())
	}

	var got []*model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), testReq(), false) {
		if err != nil {
			t.Fatalf("GenerateContent: %v", err)
		}
		got = append(got, resp)
	}
	if len(got) != 1 {
		t.Fatalf("yielded %d responses, want 1", len(got))
	}
	r := got[0]
	if r.Content == nil || r.Content.Role != "model" {
		t.Fatalf("content = %+v, want model role", r.Content)
	}
	if len(r.Content.Parts) != 1 || r.Content.Parts[0].Text != "canned reply" {
		t.Fatalf("parts = %+v", r.Content.Parts)
	}
	if r.FinishReason != genai.FinishReasonStop {
		t.Errorf("finishReason = %v", r.FinishReason)
	}

	// Request assertions.
	if cap.Method != "POST" || cap.Path != "/chat/completions" {
		t.Errorf("request = %s %s", cap.Method, cap.Path)
	}
	if cap.Auth != "Bearer test-key" {
		t.Errorf("authorization = %q", cap.Auth)
	}
	var body struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(cap.Body), &body); err != nil {
		t.Fatalf("decode request body %q: %v", cap.Body, err)
	}
	if body.Model != "test-model" {
		t.Errorf("model = %q", body.Model)
	}
	wantMsgs := [][2]string{
		{"system", "be brief"},
		{"user", "hello"},
		{"assistant", "hi there"},
		{"user", "continue"},
	}
	if len(body.Messages) != len(wantMsgs) {
		t.Fatalf("messages = %+v", body.Messages)
	}
	for i, w := range wantMsgs {
		if body.Messages[i].Role != w[0] || body.Messages[i].Content != w[1] {
			t.Errorf("message[%d] = %+v, want %v", i, body.Messages[i], w)
		}
	}
	// Request body assertions above prove capture; custom headers are
	// asserted in TestCustomHeader.
}

func TestCustomHeader(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedReply))
	}))
	defer srv.Close()
	m := NewModel(Options{Model: "test-model", BaseURL: srv.URL, APIKey: "k", Headers: map[string]string{"X-Custom": "cv"}})
	for _, err := range m.GenerateContent(context.Background(), testReq(), false) {
		if err != nil {
			t.Fatal(err)
		}
	}
	if gotHeader != "cv" {
		t.Errorf("X-Custom = %q", gotHeader)
	}
}

func TestHTTPError(t *testing.T) {
	srv, _ := newServer(t, 401, `{"error":{"message":"bad key","type":"invalid_request_error","code":"401"}}`)
	m := NewModel(Options{Model: "test-model", BaseURL: srv.URL, APIKey: "bad"})
	var lastErr error
	for _, err := range m.GenerateContent(context.Background(), testReq(), false) {
		if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		t.Fatal("want error on 401")
	}
	for _, want := range []string{"401", "test-model", "bad key"} {
		if !strings.Contains(lastErr.Error(), want) {
			t.Errorf("error %q missing %q", lastErr, want)
		}
	}
}

func TestNonStandardErrorBodies(t *testing.T) {
	// Gateways (e.g. t-tech llm-proxy) return {"error":"<string>"} where the
	// OpenAI API returns an object; without normalization openai-go loses
	// the status code and message in a JSON decode error.
	cases := []struct {
		name string
		body string
		want string
	}{
		{"string error", `{"error":"fail parse token claims: token is malformed"}`, "fail parse token claims"},
		{"no error field", `{"kind":"Error","code":"EUNAUTHORIZED"}`, "EUNAUTHORIZED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newServer(t, 401, tc.body)
			m := NewModel(Options{Model: "test-model", BaseURL: srv.URL, APIKey: "bad"})
			var lastErr error
			for _, err := range m.GenerateContent(context.Background(), testReq(), false) {
				if err != nil {
					lastErr = err
				}
			}
			if lastErr == nil {
				t.Fatal("want error on 401")
			}
			for _, want := range []string{"401", "test-model", tc.want} {
				if !strings.Contains(lastErr.Error(), want) {
					t.Errorf("error %q missing %q", lastErr, want)
				}
			}
		})
	}
}

func TestEmptyResponse(t *testing.T) {
	for name, reply := range map[string]string{
		"no choices":    `{"id":"x","object":"chat.completion","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		"empty content": `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := newServer(t, 200, reply)
			m := NewModel(Options{Model: "test-model", BaseURL: srv.URL, APIKey: "k"})
			var lastErr error
			for _, err := range m.GenerateContent(context.Background(), testReq(), false) {
				if err != nil {
					lastErr = err
				}
			}
			if lastErr == nil || !strings.Contains(lastErr.Error(), "empty response") {
				t.Errorf("err = %v, want empty response", lastErr)
			}
		})
	}
}

func TestStreamFlagIgnored(t *testing.T) {
	srv, _ := newServer(t, 200, cannedReply)
	m := NewModel(Options{Model: "test-model", BaseURL: srv.URL, APIKey: "k"})
	n := 0
	for _, err := range m.GenerateContent(context.Background(), testReq(), true) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 1 {
		t.Errorf("stream=true yielded %d responses, want 1 (non-streaming in M1)", n)
	}
}
