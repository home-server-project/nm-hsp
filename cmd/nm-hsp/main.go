// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/home-server-project/nm-hsp/internal/networkmanager"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--snapshot" {
		fmt.Println("NetworkManager-HSP development build")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := networkmanager.NewSystem(ctx)
	if err != nil {
		exitf("nm-hsp: %v", err)
	}
	defer client.Close()

	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		exitf("nm-hsp: %v", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		exitf("nm-hsp: encode snapshot: %v", err)
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
