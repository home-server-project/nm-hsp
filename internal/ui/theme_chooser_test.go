// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestThemeChooserSelectsLightAndRemember(t *testing.T) {
	m := NewThemeChooser()

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updated.(ThemeChooserModel)
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(ThemeChooserModel)
	if m.SelectedTheme() != ThemeLight {
		t.Fatalf("selected theme = %q, want light", m.SelectedTheme())
	}
	if currentTheme != ThemeLight {
		t.Fatalf("live preview theme = %q, want light", currentTheme)
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updated.(ThemeChooserModel)
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(ThemeChooserModel)
	if !m.RememberChoice() {
		t.Fatal("remember choice should be checked")
	}

	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updated.(ThemeChooserModel)
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(ThemeChooserModel)
	if !m.Confirmed() || cmd == nil {
		t.Fatal("Continue should confirm and quit")
	}
}

func TestThemeChooserRenderUsesPlainChoiceMarkers(t *testing.T) {
	m := NewThemeChooser()
	output := m.View().Content
	for _, want := range []string{
		"Dark terminal",
		"Light terminal",
		"Don't show this again",
		"Continue",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("chooser missing %q", want)
		}
	}
}

func TestThemePalettesAreDistinctAndComplete(t *testing.T) {
	dark := paletteFor(ThemeDark)
	light := paletteFor(ThemeLight)
	if dark == light {
		t.Fatal("light and dark palettes must differ")
	}
	for name, value := range map[string]string{
		"dark accent":   dark.accent,
		"dark good":     dark.good,
		"dark warning":  dark.warning,
		"dark error":    dark.err,
		"dark muted":    dark.muted,
		"dark surface":  dark.surface,
		"light accent":  light.accent,
		"light good":    light.good,
		"light warning": light.warning,
		"light error":   light.err,
		"light muted":   light.muted,
		"light surface": light.surface,
	} {
		if value == "" {
			t.Fatalf("%s must not be empty", name)
		}
	}
}
