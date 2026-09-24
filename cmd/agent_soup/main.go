// Command agent_soup is a terminal chat UI for a single OpenAI-compatible
// model configured in models.yml.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/konart/agent_soup/internal/chat"
	"github.com/konart/agent_soup/internal/config"
	"github.com/konart/agent_soup/internal/tui"
)

func main() {
	configPath := flag.String("config", "models.yml", "path to the models config file")
	providerName := flag.String("provider", "", "provider key in the config (default: alphabetically first)")
	modelName := flag.String("model", "", "model id under the provider (default: first listed)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: agent_soup [-config path] [-provider name] [-model id]\n\n")
		fmt.Fprintf(os.Stderr, "Starts a terminal chat with one provider/model from the config file.\n")
		fmt.Fprintf(os.Stderr, "Empty -provider/-model select the alphabetically-first provider\n")
		fmt.Fprintf(os.Stderr, "and that provider's first listed model.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	f, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	r, err := f.Resolve(*providerName, *modelName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	c, err := chat.New(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := tea.NewProgram(tui.New(c, ctx, r.Provider+"/"+r.Model))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// cancel() via defer: aborts any in-flight request once the UI is gone.
}
