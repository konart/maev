// Package chat wires the ADK runner to the openaicompat model, providing a
// minimal multi-turn chat conversation with in-memory session state.
package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/genai"

	"github.com/konart/agent_soup/internal/config"
	"github.com/konart/agent_soup/internal/openaicompat"
)

// userID identifies the human side of the conversation for the ADK runner.
const userID = "user"

// Chat is one conversation with one resolved provider/model. It owns an ADK
// in-memory session, so conversation memory lives for the lifetime of the
// process (no persistence).
type Chat struct {
	runner    *runner.Runner
	sessionID string
}

// New builds a Chat from a resolved config: an openaicompat model wrapped in
// a single llm_agent run by an in-memory runner with a fresh UUID session.
func New(r config.Resolved) (*Chat, error) {
	m := openaicompat.NewModel(openaicompat.Options{
		Model:   r.Model,
		BaseURL: r.BaseURL,
		APIKey:  r.APIKey,
		Headers: r.Headers,
	})
	agent, err := llmagent.New(llmagent.Config{
		Name:        "chat_agent",
		Model:       m,
		Description: "Simple chat agent",
		Instruction: "You are a helpful assistant.",
	})
	if err != nil {
		return nil, fmt.Errorf("build agent: %w", err)
	}
	run, err := runner.NewInMemory("agent_soup", agent)
	if err != nil {
		return nil, fmt.Errorf("build runner: %w", err)
	}
	return &Chat{runner: run, sessionID: uuid.NewString()}, nil
}

// Send appends one user turn and returns the agent's reply text. Multi-turn
// memory comes from the runner's in-memory session: every Send sees the full
// prior conversation.
func (c *Chat) Send(ctx context.Context, text string) (string, error) {
	msg := &genai.Content{Role: "user", Parts: []*genai.Part{{Text: text}}}
	var sb strings.Builder
	for ev, err := range c.runner.Run(ctx, userID, c.sessionID, msg, agent.RunConfig{StreamingMode: agent.StreamingModeNone}) {
		if err != nil {
			return "", fmt.Errorf("run agent: %w", err)
		}
		if ev.Content == nil || ev.Content.Role != "model" || ev.Partial {
			continue
		}
		for _, p := range ev.Content.Parts {
			if p != nil && p.Text != "" {
				sb.WriteString(p.Text)
			}
		}
	}
	reply := sb.String()
	if reply == "" {
		return "", fmt.Errorf("model returned empty response")
	}
	return reply, nil
}
