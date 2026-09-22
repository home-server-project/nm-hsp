// SPDX-License-Identifier: Apache-2.0

package ui

import "charm.land/lipgloss/v2"

var (
	colorAccent  = lipgloss.Color("#89B4FA")
	colorGood    = lipgloss.Color("#A6E3A1")
	colorWarning = lipgloss.Color("#F9E2AF")
	colorError   = lipgloss.Color("#F38BA8")
	colorMuted   = lipgloss.Color("#A6ADC8")
	colorSurface = lipgloss.Color("#313244")

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

	helpStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)
)
