// SPDX-License-Identifier: Apache-2.0

package preferences

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerminalThemePreferenceRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, found, err := LoadTerminalTheme(); err != nil || found {
		t.Fatalf("initial load found=%v err=%v", found, err)
	}

	if err := SaveTerminalTheme(ThemeLight); err != nil {
		t.Fatal(err)
	}
	theme, found, err := LoadTerminalTheme()
	if err != nil {
		t.Fatal(err)
	}
	if !found || theme != ThemeLight {
		t.Fatalf("theme=%q found=%v", theme, found)
	}

	path, err := preferencePath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("preferences mode = %o, want 600", info.Mode().Perm())
	}
}

func TestInvalidPreferenceFallsBackToUnset(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, err := preferencePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	if theme, found, err := LoadTerminalTheme(); err != nil || found || theme != "" {
		t.Fatalf("theme=%q found=%v err=%v", theme, found, err)
	}
}
