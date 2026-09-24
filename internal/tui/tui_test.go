package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newModel() Model {
	return New(nil, context.Background(), "prov/model-x")
}

func key(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func update(m Model, msg tea.Msg) (Model, tea.Cmd) {
	mm, cmd := m.Update(msg)
	return mm.(Model), cmd
}

// sized returns a model that has received a WindowSizeMsg so the viewport
// actually renders lines in View().
func sized() Model {
	m := newModel()
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func TestTypingAndSubmit(t *testing.T) {
	m := sized()
	m, _ = update(m, key("Say OK"))
	if m.input.Value() != "Say OK" {
		t.Fatalf("input = %q", m.input.Value())
	}
	m2, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter must return a cmd batch")
	}
	if len(m2.entries) != 1 || m2.entries[0].kind != entryUser || m2.entries[0].text != "Say OK" {
		t.Fatalf("entries = %+v", m2.entries)
	}
	if !m2.waiting {
		t.Error("waiting not set")
	}
	if m2.input.Value() != "" {
		t.Errorf("input not cleared: %q", m2.input.Value())
	}
	if !strings.Contains(m2.View().Content, "You ▸ Say OK") {
		t.Error("user entry not rendered")
	}
	if !strings.Contains(m2.View().Content, "thinking…") {
		t.Error("waiting line not rendered")
	}
}

func TestAgentReply(t *testing.T) {
	m := sized()
	m, _ = update(m, key("hi"))
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, agentReplyMsg{reply: "hello there"})
	if m.waiting {
		t.Error("waiting not cleared")
	}
	if len(m.entries) != 2 || m.entries[1].kind != entryAgent || m.entries[1].text != "hello there" {
		t.Fatalf("entries = %+v", m.entries)
	}
	if !strings.Contains(m.View().Content, "Agent ▸ hello there") {
		t.Error("agent entry not rendered")
	}
}

func TestErrorReply(t *testing.T) {
	m := sized()
	m, _ = update(m, key("hi"))
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, agentReplyMsg{err: errors.New("HTTP 500: boom")})
	if len(m.entries) != 2 || m.entries[1].kind != entryError {
		t.Fatalf("entries = %+v", m.entries)
	}
	if !strings.Contains(m.View().Content, "error ▸ HTTP 500: boom") {
		t.Error("error entry not rendered")
	}
	if m.waiting {
		t.Error("waiting not cleared after error")
	}
}

func TestEnterWhileWaitingIsNoop(t *testing.T) {
	m := sized()
	m, _ = update(m, key("first"))
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = update(m, key("second"))
	m2, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("Enter while waiting must not return a cmd")
	}
	if len(m2.entries) != 1 {
		t.Fatalf("entries = %+v, want only the first user entry", m2.entries)
	}
}

func TestEnterOnEmptyIsNoop(t *testing.T) {
	m := newModel()
	m2, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || len(m2.entries) != 0 {
		t.Errorf("empty Enter: cmd=%v entries=%d", cmd, len(m2.entries))
	}
}

func TestCtrlCQuits(t *testing.T) {
	m := newModel()
	_, cmd := update(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c must return a cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c cmd = %T, want tea.QuitMsg", cmd())
	}
}

func TestWindowSize(t *testing.T) {
	m := newModel()
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.vp.Width() != 80 || m.vp.Height() != 20 {
		t.Errorf("viewport = %dx%d, want 80x20", m.vp.Width(), m.vp.Height())
	}
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.vp.Width() != 100 || m.vp.Height() != 26 {
		t.Errorf("viewport = %dx%d, want 100x26", m.vp.Width(), m.vp.Height())
	}
}

func TestViewHeader(t *testing.T) {
	m := newModel()
	if !strings.Contains(m.View().Content, "agent_soup · prov/model-x · ctrl+c quit") {
		t.Errorf("header missing: %q", m.View().Content)
	}
}
