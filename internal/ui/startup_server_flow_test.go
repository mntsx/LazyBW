package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/awesome-gocui/gocui"
	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/config"
)

type serverFlowRunner struct {
	mu                 sync.Mutex
	calls              []string
	profiles           map[string]string
	credentialsCorrect bool
	servers            map[string]string
}

func (r *serverFlowRunner) Run(_ context.Context, args []string, _ []byte, env []string) ([]byte, []byte, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.profiles == nil {
		r.profiles = make(map[string]string)
	}
	cmd := strings.Join(args, " ")
	profile := ""
	clientID := ""
	secret := ""
	for _, value := range env {
		switch {
		case strings.HasPrefix(value, "BITWARDENCLI_APPDATA_DIR="):
			profile = strings.TrimPrefix(value, "BITWARDENCLI_APPDATA_DIR=")
		case strings.HasPrefix(value, "BW_CLIENTID="):
			clientID = strings.TrimPrefix(value, "BW_CLIENTID=")
		case strings.HasPrefix(value, "BW_CLIENTSECRET="):
			secret = strings.TrimPrefix(value, "BW_CLIENTSECRET=")
		}
	}
	r.calls = append(r.calls, cmd)
	switch cmd {
	case "status":
		state := r.profiles[profile]
		if state == "" {
			state = "unauthenticated"
		}
		server := r.servers[profile]
		if server == "" {
			server = "https://vault.bitwarden.com"
		}
		data, _ := json.Marshal(map[string]string{
			"status": state, "serverUrl": server,
		})
		return data, nil, 0, nil
	case "--version":
		return []byte("2026.10.0"), nil, 0, nil
	case "config server https://vault.bitwarden.com",
		"config server https://vault.bitwarden.eu":
		if r.servers == nil {
			r.servers = make(map[string]string)
		}
		r.servers[profile] = args[2]
		return nil, nil, 0, nil
	case "login --apikey":
		r.credentialsCorrect = clientID == "user.test" && secret == "test-secret" &&
			r.servers[profile] != "https://vault.bitwarden.eu"
		if !r.credentialsCorrect {
			return nil, []byte("invalid_client"), 1, nil
		}
		r.profiles[profile] = "locked"
		return nil, nil, 0, nil
	default:
		return nil, nil, 0, nil
	}
}

// applyStartupUpdateNow runs the same UI callback synchronously for deterministic tests.
// A real gocui.Gui without a running MainLoop only queues its callbacks.
func applyStartupUpdateNow(fn func(*gocui.Gui) error) {
	_ = fn(nil)
}

// Verify startup does not assume that gocui.Update executes callbacks immediately.
func TestStartupSupportsQueuedUIUpdates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api-key.env")
	if err := os.WriteFile(path, []byte("BW_CLIENTID=user.test\nBW_CLIENTSECRET=test-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &serverFlowRunner{}
	client, err := bitwarden.NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(config.Config{
		Directory:      dir,
		Authentication: config.Authentication{ClientEnvPath: path, DefaultServer: "us"},
	}, client)
	app := New(state)
	callbacks := make([]func(*gocui.Gui) error, 0)
	app.startupWithUpdates(context.Background(), func(fn func(*gocui.Gui) error) {
		callbacks = append(callbacks, fn)
	}, true)
	if state.Screen != AuthChecking || !state.Busy || len(callbacks) == 0 {
		t.Fatal("startup must support queued GUI updates")
	}
	for _, callback := range callbacks {
		if err := callback(nil); err != nil {
			t.Fatal(err)
		}
	}
	if state.Screen != AuthMaster || state.Busy || !runner.credentialsCorrect {
		t.Fatalf("queued startup did not finish: screen=%v busy=%v", state.Screen, state.Busy)
	}
}

func TestStartupAuthenticatesWithSavedKeyOnlyAtLaunch(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "api-key.env")
	if err := os.WriteFile(env, []byte("export BW_CLIENTID=user.test\nexport BW_CLIENTSECRET=test-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &serverFlowRunner{}
	client, err := bitwarden.NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(config.Config{
		Directory:      dir,
		Authentication: config.Authentication{ClientEnvPath: env, DefaultServer: "us"},
	}, client)
	app := New(state)
	app.startupWithUpdates(context.Background(), applyStartupUpdateNow, true)
	if state.Screen != AuthMaster || !runner.credentialsCorrect {
		t.Fatalf("automatic login failed: screen=%v, accepted=%v", state.Screen, runner.credentialsCorrect)
	}
	if len(runner.calls) < 4 || strings.Join(runner.calls[:3], "|") !=
		"status|--version|status" {
		t.Fatalf("endpoint wasn't verified before sign-in: %v", runner.calls)
	}
	for _, cmd := range runner.calls {
		if cmd == "login --apikey" {
			return
		}
	}
	t.Fatal("automatic login never invoked")
}

func TestSelectingServerDoesNotAutomaticallyLogin(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "api-key.env")
	if err := os.WriteFile(env, []byte("BW_CLIENTID=user.test\nBW_CLIENTSECRET=test-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &serverFlowRunner{}
	client, err := bitwarden.NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(config.Config{
		Directory:      dir,
		Authentication: config.Authentication{ClientEnvPath: env, DefaultServer: "us"},
	}, client)
	app := New(state)
	app.startupWithUpdates(context.Background(), applyStartupUpdateNow, false)
	if state.Screen != AuthClient || state.Busy {
		t.Fatalf("expected manual API form, screen=%v busy=%v", state.Screen, state.Busy)
	}
	for _, cmd := range runner.calls {
		if cmd == "login --apikey" {
			t.Fatal("server selection must not trigger login")
		}
	}
}

func TestAuthServerIndicatorUsesRowAboveFormerPosition(t *testing.T) {
	for _, region := range []struct {
		code   string
		domain string
	}{{"us", "bitwarden.com"}, {"eu", "bitwarden.eu"}} {
		for _, screen := range []Screen{AuthClient, AuthMaster} {
			state := setup(t)
			state.Screen = screen
			state.Server = region.code
			canvas := NewCanvas(100, 32)
			New(state).authRender(canvas)
			indicator := strings.ToUpper(region.code) +
				" (" + region.domain + ")"
			if !strings.Contains(stringCellRow(canvas, 1), indicator) ||
				strings.Contains(stringCellRow(canvas, 2), indicator) {
				t.Fatalf("%s: server indicator must appear on first content row", region.code)
			}
		}
	}
}

func TestManualAuthCredentialsOnlyFromDefaultRegion(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "api-key.env")
	if err := os.WriteFile(env, []byte("BW_CLIENTID=user.test\nBW_CLIENTSECRET=test-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state := NewState(config.Config{
		Authentication: config.Authentication{ClientEnvPath: env, DefaultServer: "us"},
	}, nil)
	app := New(state)
	state.Screen = AuthClient
	id, secret := app.authClientCredentials()
	if id != "user.test" || secret != "test-secret" {
		t.Fatal("AUTH should load saved credentials for selected default region")
	}
	state.Server = "eu"
	id, secret = app.authClientCredentials()
	if id != "" || secret != "" {
		t.Fatal("AUTH should not submit the US key to an EU account")
	}
	state.AuthID = "user.eu-test"
	state.AuthSecret = "eu-test-secret"
	id, secret = app.authClientCredentials()
	if id != "user.eu-test" || secret != "eu-test-secret" {
		t.Fatal("manual credentials must override the default file")
	}
}
