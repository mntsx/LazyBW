package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
	"github.com/miguelmnts/lazybw-go/internal/localvault"
	"github.com/miguelmnts/lazybw-go/internal/replica"
	"github.com/miguelmnts/lazybw-go/internal/terminalsecurity"
	"github.com/miguelmnts/lazybw-go/internal/ui"
	"golang.org/x/term"
)

func main() {
	terminalsecurity.HardenProcess()
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "LazyBW failed:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin *os.File, stdout, stderr io.Writer) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	if len(args) == 0 {
		return runTUI(cfg)
	}
	if args[0] != "replicate" {
		return fmt.Errorf("unknown command %q (expected: replicate)", args[0])
	}
	options, prompt, err := parseReplicate(args[1:], cfg, stdout)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if prompt {
		options.Password, err = readPassword(stdin, stderr)
		if err != nil {
			return err
		}
	}
	options.Progress = func(message string) {
		_, _ = fmt.Fprintln(stdout, message)
	}
	cli, err := bitwarden.NewClient(nil)
	if err != nil {
		return errors.New("unable to initialize encrypted vault session")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cli.Close(closeCtx)
	}()
	if err = replica.Run(context.Background(), cfg, cli, &options); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "Local vault replicated")
	return err
}

func runTUI(cfg config.Config) error {
	cli, err := bitwarden.NewClient(nil)
	if err != nil {
		return errors.New("unable to initialize encrypted vault session")
	}
	state := ui.NewState(cfg, cli)
	app := ui.New(state)
	if err = app.Run(); err != nil {
		return err
	}
	cli.Close(context.Background())
	return nil
}

func parseReplicate(args []string, cfg config.Config, output io.Writer) (replica.Options, bool, error) {
	options := replica.Options{
		Path:       cfg.Local.VaultPath,
		ClientPath: cfg.Authentication.ClientEnvPath,
	}
	flags := flag.NewFlagSet("replicate", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&options.Path, "path", options.Path, "destination file without an extension")
	flags.StringVar(&options.ClientPath, "client", options.ClientPath, "Bitwarden API credentials .env file")
	flags.StringVar(&options.Password, "password", "", "Bitwarden master password (less secure than the hidden prompt)")
	flags.Usage = func() {
		fmt.Fprintln(output, "Usage: lazybw replicate [--path FILE] [--client FILE.env] [--password VALUE]")
		fmt.Fprintln(output, "If --password is omitted, LazyBW reads it from the terminal with echo disabled.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return replica.Options{}, false, err
	}
	if flags.NArg() != 0 {
		return replica.Options{}, false, errors.New("replicate does not accept positional arguments")
	}
	provided := map[string]bool{}
	flags.Visit(func(value *flag.Flag) { provided[value.Name] = true })
	if provided["path"] {
		if strings.TrimSpace(options.Path) == "" {
			return replica.Options{}, false, errors.New("--path cannot be empty")
		}
		name := filepath.Base(localvault.FilePath(options.Path))
		if filepath.Ext(name) != "" {
			return replica.Options{}, false, errors.New("--path destination must not have a file extension")
		}
	}
	if provided["client"] {
		options.ClientOverride = true
		if strings.TrimSpace(options.ClientPath) == "" {
			return replica.Options{}, false, errors.New("--client cannot be empty")
		}
		if filepath.Ext(filepath.Base(options.ClientPath)) != ".env" {
			return replica.Options{}, false, errors.New("--client must point to a .env file")
		}
	}
	if provided["password"] && options.Password == "" {
		return replica.Options{}, false, errors.New("--password cannot be empty")
	}
	return options, !provided["password"], nil
}

func readPassword(stdin *os.File, output io.Writer) (string, error) {
	input := stdin
	promptOutput := output
	tty, ttyErr := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if ttyErr == nil {
		defer tty.Close()
		input = tty
		promptOutput = tty
	} else if stdin == nil || !term.IsTerminal(int(stdin.Fd())) {
		return "", errors.New("master password requires an interactive terminal; use --password only if necessary")
	}
	releaseTTY := terminalsecurity.Exclusive(input.Fd())
	state, err := term.GetState(int(input.Fd()))
	if err != nil {
		releaseTTY()
		return "", fmt.Errorf("inspect terminal state: %w", err)
	}
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			_ = term.Restore(int(input.Fd()), state)
			releaseTTY()
		})
	}
	stopSignals := terminalsecurity.RestoreOnTermination(cleanup)
	defer stopSignals()
	defer cleanup()
	if _, err := fmt.Fprint(promptOutput, "Master password: "); err != nil {
		return "", err
	}
	password, err := term.ReadPassword(int(input.Fd()))
	_, _ = fmt.Fprintln(promptOutput)
	if err != nil {
		return "", fmt.Errorf("read master password: %w", err)
	}
	defer func() {
		for index := range password {
			password[index] = 0
		}
	}()
	if len(password) == 0 {
		return "", errors.New("master password is empty")
	}
	return string(password), nil
}
