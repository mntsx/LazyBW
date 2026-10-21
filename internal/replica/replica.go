// Package replica creates encrypted, read-only copies of the personal vault.
package replica

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
	"github.com/miguelmnts/lazybw-go/internal/localvault"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// Options selects the destination and API credentials for one replication.
type Options struct {
	Path           string
	ClientPath     string
	Password       string
	ClientOverride bool
	Progress       func(string)
}

func (options *Options) report(message string) {
	if options.Progress != nil {
		options.Progress(message)
	}
}

// Run authenticates, downloads the latest server snapshot, and atomically
// replaces the encrypted local replica. It never includes organization items.
func Run(ctx context.Context, cfg config.Config, cli *bitwarden.Client, options *Options) error {
	if cli == nil {
		return errors.New("Bitwarden client is unavailable")
	}
	if options == nil || options.Password == "" {
		return errors.New("master password is empty")
	}
	defer func() { options.Password = "" }()
	options.report("Validating replica settings...")
	if len(cfg.Authentication.BackendMode) > 0 {
		if _, err := cli.SelectAvailableBackend(ctx, cfg.Authentication.BackendMode); err != nil {
			return fmt.Errorf("select Bitwarden backend: %w", err)
		}
	}
	if err := localvault.Validate(options.Path, cfg.Local.VaultEncryption); err != nil {
		return err
	}
	region := config.NormalizeServer(cfg.Authentication.DefaultServer)
	options.report("Checking Bitwarden session...")
	status, err := cli.ConfigureRegionStatus(ctx, cfg.Directory, region)
	if err != nil {
		return fmt.Errorf("configure Bitwarden region: %w", err)
	}
	state := fmt.Sprint(status["status"])
	if state != "unauthenticated" && state != "locked" && state != "unlocked" {
		return errors.New("Bitwarden returned an unknown session status")
	}
	if state == "unauthenticated" || options.ClientOverride {
		options.report("Signing in with API credentials...")
		id, secret, readErr := bitwarden.ReadEnvCredentials(options.ClientPath)
		if readErr != nil {
			return errors.New("cannot read the client credentials file")
		}
		if id == "" || secret == "" {
			return errors.New("client credentials file lacks BW_CLIENTID or BW_CLIENTSECRET")
		}
		if state == "unauthenticated" {
			err = cli.LoginUnauthenticated(ctx, id, secret)
		} else {
			err = cli.Login(ctx, id, secret)
		}
		if err != nil {
			return fmt.Errorf("sign in to Bitwarden: %w", err)
		}
	}
	options.report("Unlocking Bitwarden vault...")
	password := options.Password
	err = cli.Unlock(ctx, password)
	if err != nil {
		options.Password = ""
		password = ""
		return fmt.Errorf("unlock Bitwarden vault: %w", err)
	}
	options.report("Preparing local encryption...")
	session, err := localvault.NewSession(password, cli.Cipher)
	options.Password = ""
	password = ""
	if err != nil {
		return fmt.Errorf("prepare local encryption: %w", err)
	}
	defer session.Close()
	options.report("Synchronizing and downloading vault...")
	items, folders, _, err := cli.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("download Bitwarden vault: %w", err)
	}
	data := localvault.Snapshot{Items: make([]map[string]any, 0, len(items)), Folders: folders}
	for _, item := range items {
		if organization := item.Raw["organizationId"]; organization != nil && organization != "" {
			continue
		}
		data.Items = append(data.Items, item.Raw)
	}
	options.report("Encrypting and writing local replica...")
	return Write(ctx, data, region, options.Path, cfg.Local.VaultEncryption, session, cli.Cipher)
}

// Write decrypts only a short-lived serialization copy, encrypts the complete
// snapshot, and atomically replaces the destination after all steps succeed.
func Write(
	ctx context.Context,
	data localvault.Snapshot,
	region, path, algorithm string,
	session *localvault.Session,
	cipher *vault.Cipher,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	plain := localvault.Snapshot{
		Items:   make([]map[string]any, 0, len(data.Items)),
		Folders: data.Folders,
	}
	for _, item := range data.Items {
		mapped, err := vault.PlainMap(cipher, item)
		if err != nil {
			return err
		}
		delete(mapped, "_owner_name")
		plain.Items = append(plain.Items, mapped)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	sealed, err := session.Encode(plain, region, algorithm, time.Now())
	if err != nil {
		return err
	}
	defer func() {
		for index := range sealed {
			sealed[index] = 0
		}
	}()
	return localvault.WriteAtomicContext(ctx, path, sealed)
}
