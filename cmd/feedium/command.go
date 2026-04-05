package main

import "fmt"

type command int

const (
	commandServe command = iota
	commandMigrate
)

func parseCommand(args []string) (command, error) {
	if len(args) == 0 {
		return commandServe, nil
	}

	if len(args) == 1 && args[0] == "run" {
		return commandServe, nil
	}

	if len(args) == 2 && args[0] == "run" && args[1] == "migrate" {
		return commandMigrate, nil
	}

	return commandServe, fmt.Errorf("unknown command: %v", args)
}
