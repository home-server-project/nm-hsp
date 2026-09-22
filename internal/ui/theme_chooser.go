// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ThemeChooserModel is the first-run terminal appearance chooser.
type ThemeChooserModel struct {
	cursor    int
	selected  ThemeMode
	remember  bool
	confirmed bool
	cancelled bool
}

// NewThemeChooser creates the neutral first-run theme chooser. It intentionally
// uses terminal-native colors so it remains readable before a theme is chosen.
func NewThemeChooser() ThemeChooserModel {
	return ThemeChooserModel{
		selected: ThemeDark,
	}
}

func (m ThemeChooserModel) Init() tea.Cmd { return nil }

// Update handles first-run theme selection.
func (m ThemeChooserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j", "tab":
			if m.cursor < 3 {
				m.cursor++
			}
		case "shift+tab":
			if m.cursor > 0 {
				m.cursor--
			}
		case " ", "enter":
			switch m.cursor {
			case 0:
				m.selected = ThemeDark
			case 1:
				m.selected = ThemeLight
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

// View renders a color-neutral chooser using only terminal-native foreground,
// reverse video, and text markers.
func (m ThemeChooserModel) View() tea.View {
	var out strings.Builder

	title := lipgloss.NewStyle().Bold(true).Render("NetworkManager-HSP")
	out.WriteString(title)
	out.WriteString("\n\n")
	out.WriteString("Choose your terminal appearance")
	out.WriteString("\n")
	out.WriteString("This controls built-in contrast only. You can customize colors later.")
	out.WriteString("\n\n")

	rows := []string{
		radioLine(m.selected == ThemeDark, "Dark terminal"),
		radioLine(m.selected == ThemeLight, "Light terminal"),
		checkboxLine(m.remember, "Don't show this again"),
		"[ OK ]",
	}

	for i, row := range rows {
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			prefix = "› "
			style = style.Bold(true).Reverse(true)
		}
		out.WriteString(style.Render(prefix + row))
		out.WriteString("\n")
	}

	out.WriteString("\n")
	out.WriteString("↑/↓ or j/k move   Enter/Space select   Esc/q cancel")

	view := tea.NewView(out.String())
	view.AltScreen = true
	return view
}

func radioLine(selected bool, label string) string {
	if selected {
		return "(●) " + label
	}
	return "( ) " + label
}

func checkboxLine(checked bool, label string) string {
	if checked {
		return "[x] " + label
	}
	return "[ ] " + label
}

// SelectedTheme returns the chosen built-in terminal theme.
func (m ThemeChooserModel) SelectedTheme() ThemeMode { return m.selected }

// RememberChoice reports whether the selection should be persisted.
func (m ThemeChooserModel) RememberChoice() bool { return m.remember }

// Confirmed reports whether the user activated OK.
func (m ThemeChooserModel) Confirmed() bool { return m.confirmed }

// Cancelled reports whether the chooser was exited without confirmation.
func (m ThemeChooserModel) Cancelled() bool { return m.cancelled }
