// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestOptionsThemeChangesOnlyOnSelectionAndSaves(t *testing.T) {
	var saved ThemeMode
	screen := newOptionsScreen(ThemeDark, func(mode ThemeMode) error {
		saved = mode
		return nil
	})

	screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if screen.theme != ThemeDark {
		t.Fatalf("cursor movement changed theme to %q", screen.theme)
	}

	screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	if screen.theme != ThemeLight || currentTheme != ThemeLight {
		t.Fatalf("selected theme=%q current=%q, want light", screen.theme, currentTheme)
	}
	if saved != ThemeLight {
		t.Fatalf("saved theme=%q, want light", saved)
	}
}

func TestOptionsBackClosesScreen(t *testing.T) {
	screen := newOptionsScreen(ThemeDark, nil)
	screen.cursor = 2
	if !screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})) {
		t.Fatal("Back should close options")
	}
}
