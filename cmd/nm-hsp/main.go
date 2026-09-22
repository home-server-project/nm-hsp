// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/networkmanager"
	"github.com/home-server-project/nm-hsp/internal/ui"
)

func main() {
	if len(os.Args) > 2 {
		exitf("usage: nm-hsp [--snapshot]")
	}
	if len(os.Args) == 2 && os.Args[1] != "--snapshot" {
		exitf("usage: nm-hsp [--snapshot]")
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	client, err := networkmanager.NewSystem(startupCtx)
	cancelStartup()
	if err != nil {
		exitf("nm-hsp: %v", err)
	}
	defer client.Close()

	if len(os.Args) == 2 {
		snapshotCtx, cancelSnapshot := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelSnapshot()

		snapshot, err := client.Snapshot(snapshotCtx)
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

	program := tea.NewProgram(ui.New(client))
	if _, err := program.Run(); err != nil {
		exitf("nm-hsp: %v", err)
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
