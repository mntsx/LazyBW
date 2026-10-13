package main

import (
	"context"
	"fmt"
	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
	"github.com/miguelmnts/lazybw-go/internal/ui"
	"os"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Configuration error:", err)
		os.Exit(1)
	}
	cli, err := bitwarden.NewClient(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to initialize encrypted vault session")
		os.Exit(1)
	}
	state := ui.NewState(cfg, cli)
	app := ui.New(state)
	if err = app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "LazyBW failed:", err)
		os.Exit(1)
	}
	cli.Close(context.Background())
}
