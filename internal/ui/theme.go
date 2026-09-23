// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"image/color"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// ThemeMode selects one of nm-hsp's two built-in terminal contrast themes.
type ThemeMode string

const (
	ThemeDark  ThemeMode = "dark"
	ThemeLight ThemeMode = "light"
)

type themePalette struct {
	accent  string
	good    string
	warning string
	err     string
	muted   string
	surface string
}

var (
	currentTheme = ThemeDark

	colorAccent  color.Color
	colorGood    color.Color
	colorWarning color.Color
	colorError   color.Color
	colorMuted   color.Color
	colorSurface color.Color

	titleStyle        lipgloss.Style
	mutedStyle        lipgloss.Style
	goodStyle         lipgloss.Style
	warningStyle      lipgloss.Style
	errorStyle        lipgloss.Style
	cardStyle         lipgloss.Style
	selectedCardStyle lipgloss.Style
	helpStyle         lipgloss.Style
)

func init() {
	ApplyTheme(ThemeDark)
}

// ApplyTheme installs one of the two built-in high-contrast terminal themes.
func ApplyTheme(mode ThemeMode) {
	if mode != ThemeLight {
		mode = ThemeDark
	}
	currentTheme = mode

	palette := paletteFor(mode)
	colorAccent = lipgloss.Color(palette.accent)
	colorGood = lipgloss.Color(palette.good)
	colorWarning = lipgloss.Color(palette.warning)
	colorError = lipgloss.Color(palette.err)
	colorMuted = lipgloss.Color(palette.muted)
	colorSurface = lipgloss.Color(palette.surface)

	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent)

	mutedStyle = lipgloss.NewStyle().
		Foreground(colorMuted)

	goodStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorGood)

	warningStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorWarning)

	errorStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorError)

	cardStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface).
		Padding(0, 1)

	selectedCardStyle = cardStyle.
		BorderForeground(colorAccent)

	// The help lane intentionally uses the terminal's own foreground color.
	// Terminal themes choose that foreground to contrast with their background.
	helpStyle = lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1)
}

func paletteFor(mode ThemeMode) themePalette {
	if mode == ThemeLight {
		return themePalette{
			accent:  "#1D4ED8",
			good:    "#166534",
			warning: "#854D0E",
			err:     "#B91C1C",
			muted:   "#4B5563",
			surface: "#6B7280",
		}
	}

	return themePalette{
		accent:  "#89B4FA",
		good:    "#A6E3A1",
		warning: "#F9E2AF",
		err:     "#F38BA8",
		muted:   "#A6ADC8",
		surface: "#6C7086",
	}
}

func applyTextInputTheme(input *textinput.Model) {
	styles := textinput.DefaultStyles(currentTheme == ThemeDark)

	// Typed text follows the terminal's native foreground for maximum
	// compatibility with unusual terminal backgrounds.
	styles.Focused.Text = lipgloss.NewStyle()
	styles.Blurred.Text = lipgloss.NewStyle().Foreground(colorMuted)

	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	styles.Focused.Suggestion = lipgloss.NewStyle().Foreground(colorMuted)
	styles.Blurred.Suggestion = lipgloss.NewStyle().Foreground(colorMuted)
	styles.Cursor.Color = colorAccent

	input.SetStyles(styles)
}
