package replica

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
	"github.com/miguelmnts/lazybw-go/internal/localvault"
)

type replicationRunner struct {
	mu     sync.Mutex
	calls  []string
	status string
}

func (runner *replicationRunner) Run(_ context.Context, args []string, _ []byte, _ []string) ([]byte, []byte, int, error) {
	call := strings.Join(args, " ")
	runner.mu.Lock()
	runner.calls = append(runner.calls, call)
	runner.mu.Unlock()
	switch call {
	case "status":
		status := runner.status
		if status == "" {
			status = "unauthenticated"
		}
		server := `null`
		if status != "unauthenticated" {
			server = `"https://vault.bitwarden.com","userId":"old-account"`
		}
		return []byte(`{"status":"` + status + `","serverUrl":` + server + `}`), nil, 0, nil
	case "login --apikey", "logout", "sync":
		return nil, nil, 0, nil
	case "unlock --raw --passwordenv LAZYBW_MASTER_PASSWORD":
		return []byte("session-token"), nil, 0, nil
	case "list items":
		return []byte(`[
			{"id":"personal","name":"Personal","type":1,"organizationId":null,"notes":"private note","login":{"username":"me","password":"password"}},
			{"id":"organization","name":"Work","type":1,"organizationId":"org-1","login":{"username":"work","password":"work-password"}}
		]`), nil, 0, nil
	case "list folders":
		return []byte(`[{"id":"folder-1","name":"Personal"}]`), nil, 0, nil
	case "list organizations":
		return []byte(`[{"id":"org-1","name":"Employer"}]`), nil, 0, nil
	default:
		return nil, []byte("unexpected command"), 1, nil
	}
}

func TestRunCreatesLatestPersonalReplica(t *testing.T) {
	directory := t.TempDir()
	credentials := filepath.Join(directory, "client.env")
	if err := os.WriteFile(credentials, []byte("BW_CLIENTID=user.test\nBW_CLIENTSECRET=api-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "bw-vault")
	if err := os.WriteFile(destination, []byte("old replica"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Directory: filepath.Join(directory, "config")}
	cfg.Authentication.DefaultServer = "us"
	cfg.Local.VaultEncryption = localvault.AESGCM
	runner := &replicationRunner{}
	cli, err := bitwarden.NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close(context.Background())
	var progress []string
	options := Options{
		Path: destination, ClientPath: credentials, Password: "master password",
		Progress: func(message string) { progress = append(progress, message) },
	}
	if err = Run(context.Background(), cfg, cli, &options); err != nil {
		t.Fatal(err)
	}
	if options.Password != "" {
		t.Fatal("replication retained the plaintext master password")
	}
	expectedProgress := []string{
		"Validating replica settings...",
		"Checking Bitwarden session...",
		"Signing in with API credentials...",
		"Unlocking Bitwarden vault...",
		"Preparing local encryption...",
		"Synchronizing and downloading vault...",
		"Encrypting and writing local replica...",
	}
	if strings.Join(progress, "\n") != strings.Join(expectedProgress, "\n") {
		t.Fatalf("unexpected progress messages: %q", progress)
	}
	snapshot, header, err := localvault.Decode(destination, "master password", localvault.AESGCM)
	if err != nil {
		t.Fatal(err)
	}
	if header.Origin != "us" || len(snapshot.Items) != 1 || snapshot.Items[0]["id"] != "personal" {
		t.Fatalf("unexpected personal snapshot: header=%#v items=%#v", header, snapshot.Items)
	}
	login, ok := snapshot.Items[0]["login"].(map[string]any)
	if !ok || login["password"] != "password" || snapshot.Items[0]["notes"] != "private note" {
		t.Fatalf("secrets were not preserved in encrypted replica: %#v", snapshot.Items[0])
	}
	if len(snapshot.Folders) != 1 || snapshot.Folders[0]["id"] != "folder-1" {
		t.Fatalf("folders were not replicated: %#v", snapshot.Folders)
	}
	runner.mu.Lock()
	calls := strings.Join(runner.calls, "\n")
	runner.mu.Unlock()
	for _, expected := range []string{"login --apikey", "unlock --raw --passwordenv LAZYBW_MASTER_PASSWORD", "sync"} {
		if !strings.Contains(calls, expected) {
			t.Fatalf("missing %q in calls:\n%s", expected, calls)
		}
	}
	if strings.Contains(calls, "status") {
		t.Fatalf("a new replication profile must not recheck its known status:\n%s", calls)
	}
}

func TestExplicitClientOverridesLockedProfile(t *testing.T) {
	directory := t.TempDir()
	credentials := filepath.Join(directory, "override.env")
	if err := os.WriteFile(credentials, []byte("BW_CLIENTID=user.override\nBW_CLIENTSECRET=api-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Directory: filepath.Join(directory, "config")}
	cfg.Authentication.DefaultServer = "us"
	cfg.Local.VaultEncryption = localvault.AESGCM
	runner := &replicationRunner{status: "locked"}
	cli, err := bitwarden.NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close(context.Background())
	options := Options{
		Path: filepath.Join(directory, "bw-vault"), ClientPath: credentials,
		Password: "master password", ClientOverride: true,
	}
	if err = Run(context.Background(), cfg, cli, &options); err != nil {
		t.Fatal(err)
	}
	runner.mu.Lock()
	calls := strings.Join(runner.calls, "\n")
	runner.mu.Unlock()
	if !strings.Contains(calls, "login --apikey") {
		t.Fatalf("explicit credentials did not authenticate the profile:\n%s", calls)
	}
	if strings.Contains(calls, "status") || strings.Contains(calls, "logout") {
		t.Fatalf("a new override profile must not probe or log out first:\n%s", calls)
	}
}

func TestRunValidatesDestinationBeforeAuthentication(t *testing.T) {
	runner := &replicationRunner{}
	cli, err := bitwarden.NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close(context.Background())
	cfg := config.Config{Directory: t.TempDir()}
	cfg.Local.VaultEncryption = localvault.AESGCM
	options := Options{
		Path: filepath.Join(t.TempDir(), "missing", "bw-vault"), Password: "secret",
	}
	err = Run(context.Background(), cfg, cli, &options)
	if err == nil {
		t.Fatal("invalid destination was accepted")
	}
	if options.Password != "" {
		t.Fatal("failed replication retained the plaintext master password")
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 0 {
		t.Fatalf("authentication started before destination validation: %v", runner.calls)
	}
}
