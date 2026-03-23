package main

import (
	"log"
	"os"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if err := runMain(os.Args[1:], runGatewayStartup); err != nil {
		log.Fatalf("command failed: %v", err)
	}
}

func runMain(args []string, startup func([]string)) error {
	if handled, err := handleConfigCommands(args); handled {
		return err
	}
	startup(args)
	return nil
}
