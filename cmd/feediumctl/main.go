package main

import (
	"fmt"
	"os"

	"github.com/4itosik/feedium/cmd/feediumctl/internal/app"
)

func main() {
	if err := app.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, app.FormatError(err))
		os.Exit(1)
	}
}
