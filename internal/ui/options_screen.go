// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type themePreferenceSaver func(ThemeMode) error

type optionsScreen struct {
	cursor    int
	theme     ThemeMode
	saveTheme themePreferenceSaver
	err       error
}

func newOptionsScreen(theme ThemeMode, saveTheme themePreferenceSaver) *optionsScreen {
	return &optionsScreen{theme: theme, saveTheme: saveTheme}
}

func (s *optionsScreen) update(msg tea.Msg) bool {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return false
	}

	switch key.String() {
	case "q", "esc":
		return true
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j", "tab":
		if s.cursor < 2 {
			s.cursor++
		}
	case "shift+tab":
		if s.cursor > 0 {
			s.cursor--
		}
	case "space", "enter":
		switch s.cursor {
		case 0:
			s.setTheme(ThemeDark)
		case 1:
			s.setTheme(ThemeLight)
		case 2:
			return true
		}
	}

	return false
}

func (s *optionsScreen) setTheme(theme ThemeMode) {
	s.theme = theme
	ApplyTheme(theme)
	s.err = nil
	if s.saveTheme != nil {
		s.err = s.saveTheme(theme)
	}
}

func (s *optionsScreen) render(width int) string {
	if width < 32 {
		width = 32
	}

	var out strings.Builder
	header := titleStyle.Render("Options") + "  " + mutedStyle.Render("NetworkManager-HSP")
	out.WriteString(lipgloss.NewStyle().Width(width).Padding(0, 1).Render(header))
	out.WriteString("\n\n")
	out.WriteString(lipgloss.NewStyle().Width(width).Padding(0, 1).Render(titleStyle.Render("Appearance")))
	out.WriteString("\n")

	options := []struct {
		label string
		theme ThemeMode
	}{
		{label: "Dark terminal", theme: ThemeDark},
		{label: "Light terminal", theme: ThemeLight},
	}
	for i, option := range options {
		style := cardStyle
		marker := "  "
		if i == s.cursor {
			style = selectedCardStyle
			marker = "› "
		}
		radio := "( )"
		if s.theme == option.theme {
			radio = "(●)"
		}
		out.WriteString("\n")
		out.WriteString(style.Width(cardContentWidth(width)).Render(marker + radio + " " + option.label))
	}

	backStyle := cardStyle
	backMarker := "  "
	if s.cursor == 2 {
		backStyle = selectedCardStyle
		backMarker = "› "
	}
	out.WriteString("\n")
	out.WriteString(backStyle.Width(cardContentWidth(width)).Render(backMarker + "Back"))

	if s.err != nil {
		out.WriteString("\n\n  " + errorStyle.Render("Could not save appearance preference: "+s.err.Error()))
	}

	out.WriteString("\n\n")
	out.WriteString(helpStyle.Width(width).Render("↑/↓ or j/k move   Enter/Space select   Esc/q back"))
	return out.String()
}
