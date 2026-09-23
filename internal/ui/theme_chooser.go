// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ThemeChooserModel is the first-run terminal appearance chooser.
type ThemeChooserModel struct {
	width     int
	cursor    int
	selected  ThemeMode
	remember  bool
	confirmed bool
	cancelled bool
}

// NewThemeChooser creates the first-run theme chooser using the same visual
// language as the main nm-hsp dashboard.
func NewThemeChooser() ThemeChooserModel {
	ApplyTheme(ThemeDark)
	return ThemeChooserModel{
		selected: ThemeDark,
	}
}

func (m ThemeChooserModel) Init() tea.Cmd { return nil }

// Update handles first-run theme selection. Choosing Dark or Light immediately
// applies that palette so the chooser itself acts as a live preview.
func (m ThemeChooserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.previewCursorTheme()
			}
		case "down", "j", "tab":
			if m.cursor < 3 {
				m.cursor++
				m.previewCursorTheme()
			}
		case "shift+tab":
			if m.cursor > 0 {
				m.cursor--
				m.previewCursorTheme()
			}
		case " ", "enter":
			switch m.cursor {
			case 0:
				m.selected = ThemeDark
				ApplyTheme(m.selected)
			case 1:
				m.selected = ThemeLight
				ApplyTheme(m.selected)
			case 2:
				m.remember = !m.remember
			case 3:
				m.confirmed = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *ThemeChooserModel) previewCursorTheme() {
	switch m.cursor {
	case 0:
		m.selected = ThemeDark
		ApplyTheme(m.selected)
	case 1:
		m.selected = ThemeLight
		ApplyTheme(m.selected)
	}
}

// View renders the chooser with the same cards, accent, muted text, and help
// lane used by the rest of nm-hsp.
func (m ThemeChooserModel) View() tea.View {
	width := m.width
	if width <= 0 {
		width = 88
	}
	if width < 32 {
		width = 32
	}
	contentWidth := width - 2

	var out strings.Builder
	header := titleStyle.Render("NetworkManager-HSP") + "  " +
		mutedStyle.Render("terminal appearance")
	out.WriteString(lipgloss.NewStyle().
		Width(contentWidth).
		Padding(0, 1).
		Render(header))
	out.WriteString("\n")
	out.WriteString(lipgloss.NewStyle().
		Width(contentWidth).
		Padding(0, 1).
		Render(mutedStyle.Render("Choose the built-in contrast that matches your terminal.")))
	out.WriteString("\n\n")

	options := []struct {
		title       string
		description string
		selected    bool
	}{
		{
			title:       "Dark terminal",
			description: "Optimized for dark terminal backgrounds",
			selected:    m.selected == ThemeDark,
		},
		{
			title:       "Light terminal",
			description: "Optimized for light terminal backgrounds",
			selected:    m.selected == ThemeLight,
		},
	}

	for i, option := range options {
		style := cardStyle
		marker := "  "
		if i == m.cursor {
			style = selectedCardStyle
			marker = "› "
		}
		choice := "( )"
		if option.selected {
			choice = "(●)"
		}
		body := marker + titleStyle.Render(choice+" "+option.title) +
			"\n    " + mutedStyle.Render(option.description)
		out.WriteString(style.Width(cardContentWidth(contentWidth)).Render(body))
		out.WriteString("\n")
	}

	rememberStyle := cardStyle
	rememberMarker := "  "
	if m.cursor == 2 {
		rememberStyle = selectedCardStyle
		rememberMarker = "› "
	}
	check := "[ ]"
	if m.remember {
		check = "[x]"
	}
	out.WriteString(rememberStyle.
		Width(cardContentWidth(contentWidth)).
		Render(rememberMarker + check + " Don't show this again"))
	out.WriteString("\n")

	continueStyle := cardStyle
	continueMarker := "  "
	if m.cursor == 3 {
		continueStyle = selectedCardStyle
		continueMarker = "› "
	}
	out.WriteString(continueStyle.
		Width(cardContentWidth(contentWidth)).
		Render(continueMarker + "Continue"))

	out.WriteString("\n\n")
	out.WriteString(helpStyle.Width(contentWidth).Render(
		"↑/↓ or j/k move   Enter/Space select   Esc/q cancel",
	))

	view := tea.NewView(out.String())
	view.AltScreen = true
	return view
}

// SelectedTheme returns the chosen built-in terminal theme.
func (m ThemeChooserModel) SelectedTheme() ThemeMode { return m.selected }

// RememberChoice reports whether the selection should be persisted.
func (m ThemeChooserModel) RememberChoice() bool { return m.remember }

// Confirmed reports whether the user activated Continue.
func (m ThemeChooserModel) Confirmed() bool { return m.confirmed }

// Cancelled reports whether the chooser was exited without confirmation.
func (m ThemeChooserModel) Cancelled() bool { return m.cancelled }
