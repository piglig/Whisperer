// Package tui implements the bubbletea front-end for Whisperer.
package tui

import "github.com/charmbracelet/lipgloss"

var (
	sessionBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#121826")).
			Foreground(lipgloss.Color("#e5e7eb")).
			Bold(true).
			Padding(0, 1)

	paneTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f8fafc")).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22d3ee")).
			Bold(true)

	accentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#facc15")).
			Bold(true)

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64748b"))

	composerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f8fafc")).
			Background(lipgloss.Color("#0f172a")).
			Padding(0, 1)

	gmLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c4b5fd")).
			Bold(true)

	playerLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#86efac")).
				Bold(true)

	gmStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c4b5fd"))

	playerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#86efac"))

	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#38bdf8")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fca5a5")).
			Bold(true)

	endingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fde68a")).
			Bold(true)
)
