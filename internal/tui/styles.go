// Package tui implements the bubbletea front-end for Whisperer.
package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#3a3f4b")).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Padding(0, 1)

	statusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#1f2330")).
			Foreground(lipgloss.Color("#cdd6f4")).
			Padding(0, 1)

	gmStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#cba6f7"))

	playerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a6e3a1"))

	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94a3b8")).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f38ba8")).
			Bold(true)

	endingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af")).
			Bold(true).
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

	driftSoftStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fab387"))

	driftHardStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f38ba8")).
			Bold(true)
)
