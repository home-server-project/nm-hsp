// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/networkmanager"
	"github.com/home-server-project/nm-hsp/internal/preferences"
	"github.com/home-server-project/nm-hsp/internal/ui"
	"github.com/home-server-project/nm-hsp/internal/vpn"
)

type appSource struct {
	*networkmanager.Client
	vpn *vpn.Manager
}

func (s *appSource) Snapshot(ctx context.Context) (model.Snapshot, error) {
	snapshot, err := s.Client.Snapshot(ctx)
	if err != nil {
		return model.Snapshot{}, err
	}
	snapshot.VPN = s.vpn.Snapshot(ctx)
	return snapshot, nil
}

func (s *appSource) VPNAction(
	ctx context.Context,
	id model.VPNProviderID,
	action model.VPNAction,
) (model.VPNActionResult, error) {
	return s.vpn.Action(ctx, id, action)
}

func (s *appSource) VPNWaitAuthentication(
	ctx context.Context,
	id model.VPNProviderID,
	userCode string,
) (model.VPNActionResult, error) {
	return s.vpn.WaitAuthentication(ctx, id, userCode)
}

func main() {
	if len(os.Args) > 2 {
		exitf("usage: nm-hsp [--snapshot]")
	}
	if len(os.Args) == 2 && os.Args[1] != "--snapshot" {
		exitf("usage: nm-hsp [--snapshot]")
	}

	snapshotMode := len(os.Args) == 2
	themeMode := ui.ThemeDark

	if !snapshotMode {
		savedTheme, found, _ := preferences.LoadTerminalTheme()
		if found {
			themeMode = ui.ThemeMode(savedTheme)
		} else {
			program := tea.NewProgram(ui.NewThemeChooser())
			result, err := program.Run()
			if err != nil {
				exitf("nm-hsp: theme chooser: %v", err)
			}

			choice, ok := result.(ui.ThemeChooserModel)
			if !ok {
				exitf("nm-hsp: theme chooser returned unexpected model")
			}
			if !choice.Confirmed() {
				return
			}

			themeMode = choice.SelectedTheme()
			if choice.RememberChoice() {
				if err := preferences.SaveTerminalTheme(string(themeMode)); err != nil {
					exitf("nm-hsp: save terminal theme: %v", err)
				}
			}
		}
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	client, err := networkmanager.NewSystem(startupCtx)
	cancelStartup()
	if err != nil {
		exitf("nm-hsp: %v", err)
	}
	defer client.Close()

	source := &appSource{
		Client: client,
		vpn:    vpn.NewManager(),
	}

	if snapshotMode {
		snapshotCtx, cancelSnapshot := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelSnapshot()

		snapshot, err := source.Snapshot(snapshotCtx)
		if err != nil {
			exitf("nm-hsp: %v", err)
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(snapshot); err != nil {
			exitf("nm-hsp: encode snapshot: %v", err)
		}
		return
	}

	program := tea.NewProgram(ui.NewWithTheme(source, themeMode))
	if _, err := program.Run(); err != nil {
		exitf("nm-hsp: %v", err)
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
