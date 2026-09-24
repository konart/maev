package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/konart/agent_soup/internal/config"
)

// replyServer returns one canned assistant reply per request and records
// every request body in order.
type replyServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	status int
}

func newReplyServer(t *testing.T, replies []string) *replyServer {
	t.Helper()
	rs := &replyServer{}
	i := 0
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rs.mu.Lock()
		rs.bodies = append(rs.bodies, string(body))
		idx := i
		i++
		status := rs.status
		rs.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			w.Write([]byte(`{"error":{"message":"boom","type":"server_error","code":"500"}}`))
			return
		}
		text := replies[idx%len(replies)]
		json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test", "object": "chat.completion", "created": 1700000000, "model": "m",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": text},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	t.Cleanup(rs.Close)
	return rs
}

func testChat(t *testing.T, baseURL string) *Chat {
	t.Helper()
	c, err := New(config.Resolved{
		Provider: "p",
		Model:    "m",
		BaseURL:  baseURL,
		APIKey:   "k",
	})
	if err != nil {
		t.Fatalf("chat.New: %v", err)
	}
	return c
}

func TestSendReturnsReply(t *testing.T) {
	srv := newReplyServer(t, []string{"reply one"})
	c := testChat(t, srv.URL)
	got, err := c.Send(context.Background(), "first question")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got != "reply one" {
		t.Errorf("reply = %q", got)
	}
}

func TestMultiTurnMemory(t *testing.T) {
	srv := newReplyServer(t, []string{"reply one", "reply two"})
	c := testChat(t, srv.URL)
	if _, err := c.Send(context.Background(), "first question"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Send(context.Background(), "second question"); err != nil {
		t.Fatal(err)
	}
	if len(srv.bodies) != 2 {
		t.Fatalf("captured %d requests, want 2", len(srv.bodies))
	}
	second := srv.bodies[1]
	for _, want := range []string{"first question", "reply one", "second question"} {
		if !strings.Contains(second, want) {
			t.Errorf("second request body missing %q\nbody: %s", want, second)
		}
	}
	// Turn 1's user message maps to role "user", the agent reply to
	// "assistant" in the replayed history.
	var decoded struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(second), &decoded); err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, m := range decoded.Messages {
		roles[m.Content] = m.Role
	}
	if roles["first question"] != "user" {
		t.Errorf("first question role = %q", roles["first question"])
	}
	if roles["reply one"] != "assistant" {
		t.Errorf("reply one role = %q", roles["reply one"])
	}
}

func TestSendServerError(t *testing.T) {
	srv := newReplyServer(t, []string{"unused"})
	srv.status = 500
	c := testChat(t, srv.URL)
	_, err := c.Send(context.Background(), "hello")
	if err == nil {
		t.Fatal("want error on HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want status code", err)
	}
}
