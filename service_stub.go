//go:build !darwin && !linux

package main

import (
	"fmt"
	"os"
)

func handleServiceCommand(args []string) {
	if len(args) >= 1 && args[0] == "run" {
		runServer(args[1:])
		return
	}
	fmt.Fprintf(os.Stderr, "service management is not supported on this platform\n")
	fmt.Fprintf(os.Stderr, "only 'service run' is available\n")
	os.Exit(1)
}
