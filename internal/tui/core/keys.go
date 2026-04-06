package core

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all key bindings for the TUI.
type KeyMap struct {
	Quit    key.Binding
	Back    key.Binding
	Help    key.Binding
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	New     key.Binding
	History key.Binding
	Tab     key.Binding
	Delete  key.Binding
	Kill    key.Binding
	Retry   key.Binding
	Create  key.Binding
	Edit    key.Binding
	Filter  key.Binding
	Bottom  key.Binding
	Top     key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/down", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new pipeline"),
		),
		History: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "history"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next field"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d", "backspace"),
			key.WithHelp("d", "delete"),
		),
		Kill: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "kill"),
		),
		Retry: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "retry"),
		),
		Create: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "create workflow"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit workflow"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
		Top: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "top"),
		),
	}
}
