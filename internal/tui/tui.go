// Package tui implements the agent_soup chat terminal UI on bubbletea:
// a chat-history viewport plus a single user input line. The history shows
// only user queries, agent replies, and errors.
package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/konart/agent_soup/internal/chat"
)

type entryKind int

const (
	entryUser entryKind = iota
	entryAgent
	entryError
)

type entry struct {
	kind entryKind
	text string
}

// agentReplyMsg is delivered asynchronously when a Send completes.
type agentReplyMsg struct {
	reply string
	err   error
}

// Model is the bubbletea model. Run with tea.NewProgram(New(...)); the view
// itself opts into the alternate screen via View.AltScreen.
type Model struct {
	chat          *chat.Chat
	ctx           context.Context
	providerModel string

	vp      viewport.Model
	input   textinput.Model
	spin    spinner.Model
	entries []entry
	waiting bool
	width   int
	height  int
}

// New builds the TUI model. providerModel is shown in the header (e.g.
// "tbank_openai_compat/tgpt/text.instant.sota").
func New(c *chat.Chat, ctx context.Context, providerModel string) Model {
	in := textinput.New()
	in.Placeholder = "Type a message…"
	in.Focus()

	vp := viewport.New()

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	return Model{
		chat:          c,
		ctx:           ctx,
		providerModel: providerModel,
		vp:            vp,
		input:         in,
		spin:          sp,
	}
}

// Init focuses the input and starts the cursor blink.
func (m Model) Init() tea.Cmd {
	return m.input.Focus()
}

// Update drives the waiting-state machine:
// Enter (idle, non-empty input) -> waiting=true + spinner + async Send;
// agentReplyMsg -> waiting=false + rendered entry.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.SetWidth(msg.Width)
		m.vp.SetHeight(msg.Height - 4) // header, spacer, waiting line, input
		m.renderEntries()
		m.vp.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
		return m, func() tea.Msg { return tea.Quit() }
		case "enter":
			if m.waiting || m.input.Value() == "" {
				return m, nil
			}
			text := m.input.Value()
			m.entries = append(m.entries, entry{kind: entryUser, text: text})
			m.input.SetValue("")
			m.waiting = true
			m.renderEntries()
			m.vp.GotoBottom()
			submit := func() tea.Msg {
				reply, err := m.chat.Send(m.ctx, text)
				return agentReplyMsg{reply: reply, err: err}
			}
			return m, tea.Batch(m.spin.Tick, submit)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case agentReplyMsg:
		m.waiting = false
		if msg.err != nil {
			m.entries = append(m.entries, entry{kind: entryError, text: msg.err.Error()})
		} else {
			m.entries = append(m.entries, entry{kind: entryAgent, text: msg.reply})
		}
		m.renderEntries()
		m.vp.GotoBottom()
		return m, nil

	case spinner.TickMsg:
		if !m.waiting {
			return m, nil // stop the chain once the reply arrived
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders header, chat history viewport, waiting line, and input line.
func (m Model) View() tea.View {
	header := headerStyle.Render(fmt.Sprintf("agent_soup · %s · ctrl+c quit", m.providerModel))

	waitLine := ""
	if m.waiting {
		waitLine = fmt.Sprintf("%s thinking…", m.spin.View())
	}

	v := tea.NewView(header + "\n" + m.vp.View() + "\n" + waitLine + "\n" + m.input.View())
	v.AltScreen = true
	return v
}

// renderEntries rebuilds the viewport content from the entry log.
func (m *Model) renderEntries() {
	w := 0
	if m.width > 2 {
		w = m.width - 2
	}
	var parts []string
	for _, e := range m.entries {
		var style lipgloss.Style
		var prefix string
		switch e.kind {
		case entryUser:
			style = userStyle
			prefix = "You ▸ "
		case entryAgent:
			style = agentStyle
			prefix = "Agent ▸ "
		default:
			style = errorStyle
			prefix = "error ▸ "
		}
		if w > 0 {
			style = style.Width(w)
		}
		parts = append(parts, style.Render(prefix+e.text))
	}
	m.vp.SetContent(strings.Join(parts, "\n\n"))
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	userStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5FD7FF"))
	agentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5FD7"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))
)
