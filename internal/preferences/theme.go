// SPDX-License-Identifier: Apache-2.0

package preferences

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	ThemeDark  = "dark"
	ThemeLight = "light"
)

type filePreferences struct {
	TerminalTheme string `json:"terminal_theme"`
}

// LoadTerminalTheme returns the remembered terminal theme when one exists.
// Invalid or incomplete preference content is treated as unset so first-run
// selection can recover without making nm-hsp unusable.
func LoadTerminalTheme() (string, bool, error) {
	path, err := preferencePath()
	if err != nil {
		return "", false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	var prefs filePreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return "", false, nil
	}
	if prefs.TerminalTheme != ThemeDark && prefs.TerminalTheme != ThemeLight {
		return "", false, nil
	}
	return prefs.TerminalTheme, true, nil
}

// SaveTerminalTheme remembers the user's built-in terminal theme selection.
func SaveTerminalTheme(theme string) error {
	if theme != ThemeDark && theme != ThemeLight {
		return errors.New("unsupported terminal theme")
	}

	path, err := preferencePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(filePreferences{TerminalTheme: theme}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func preferencePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nm-hsp", "preferences.json"), nil
}
