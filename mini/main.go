// The smallest repro: bubbletea only. A four-line menu; on ctrl-c the model marks itself done, so its view
// goes empty (as a finished huh form's does), and it ends with tea.Interrupt (what huh's CancelCmd is) or
// tea.Quit, chosen at build time:
//
//	go build -ldflags "-X main.mode=interrupt" .
//	go build -ldflags "-X main.mode=quit" .
//
// With BT_TRACE set to a path, it writes there whether View ran before the first WindowSizeMsg.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

var mode = "interrupt"

var sawSize, viewed, viewBeforeSize bool

type model struct{ done bool }

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		sawSize = true
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.done = true
			if mode == "quit" {
				return m, tea.Quit
			}
			return m, tea.Interrupt
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	if !viewed {
		viewed, viewBeforeSize = true, !sawSize
	}
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView("What now?\n> Build\n  Test\n  Ship")
}

func main() {
	_, err := tea.NewProgram(model{}).Run()
	if p := os.Getenv("BT_TRACE"); p != "" {
		_ = os.WriteFile(p, []byte(fmt.Sprintf("view-before-size=%v\n", viewBeforeSize)), 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
