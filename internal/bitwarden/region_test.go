package bitwarden

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type regionCall struct {
	args []string
	env  []string
}
type regionRunner struct {
	calls         []regionCall
	server        map[string]string
	status        map[string]string
	defaultServer string
	defaultStatus string
}

func (r *regionRunner) Run(_ context.Context, args []string, _ []byte, env []string) ([]byte, []byte, int, error) {
	r.calls = append(r.calls, regionCall{append([]string(nil), args...), append([]string(nil), env...)})
	profile := envValue(env, "BITWARDENCLI_APPDATA_DIR")
	if r.server == nil {
		r.server = map[string]string{}
	}
	if r.status == nil {
		r.status = map[string]string{}
	}
	switch strings.Join(args, " ") {
	case "status":
		server := r.server[profile]
		if server == "" {
			server = r.defaultServer
		}
		if server == "" {
			server = "https://vault.bitwarden.com"
		}
		status := r.status[profile]
		if status == "" {
			status = r.defaultStatus
		}
		if status == "" {
			status = "unauthenticated"
		}
		out, _ := json.Marshal(map[string]string{"serverUrl": server, "status": status})
		return out, nil, 0, nil
	case "logout":
		r.status[profile] = "unauthenticated"
		return nil, nil, 0, nil
	}
	if len(args) == 3 && args[0] == "config" && args[1] == "server" {
		r.server[profile] = args[2]
	}
	return []byte("[]"), nil, 0, nil
}

func envValue(env []string, name string) string {
	var result string
	for _, v := range env {
		if strings.HasPrefix(v, name+"=") {
			result = strings.TrimPrefix(v, name+"=")
		}
	}
	return result
}

func TestIsolatedRegionsAndSessions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	runner := &regionRunner{}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	client.SetSession("previous-session")
	if err := client.ConfigureRegion(ctx, dir, "eu"); err != nil {
		t.Fatal(err)
	}
	eu := envValue(runner.calls[len(runner.calls)-1].env, "BITWARDENCLI_APPDATA_DIR")
	if runner.server[eu] != "https://vault.bitwarden.eu" {
		t.Fatalf("EU endpoint not configured: %v", runner.server)
	}
	if filepath.Base(eu) != "eu" || filepath.Dir(filepath.Dir(eu)) != filepath.Join(dir, "bitwarden", "sessions") {
		t.Fatalf("EU profile has unexpected path: %q", eu)
	}
	if client.HasSession() {
		t.Fatal("previous profile session reused")
	}
	if _, err := client.ses(ctx, "list", "items"); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.calls {
		if envValue(call.env, "BITWARDENCLI_APPDATA_DIR") != eu {
			t.Fatalf("command escaped EU profile: %v", call)
		}
		if envValue(call.env, "BW_SESSION") != "" {
			t.Fatalf("command leaked session: %v", call.args)
		}
	}
	if err := client.ConfigureRegion(ctx, dir, "us"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ses(ctx, "list", "folders"); err != nil {
		t.Fatal(err)
	}
	us := envValue(runner.calls[len(runner.calls)-1].env, "BITWARDENCLI_APPDATA_DIR")
	if filepath.Base(us) != "us" || filepath.Dir(us) != filepath.Dir(eu) {
		t.Fatalf("US profile must share the session directory: eu=%q us=%q", eu, us)
	}
	client.SetSession("US-session")
	if _, err := client.ses(ctx, "list", "items"); err != nil {
		t.Fatal(err)
	}
	if got := envValue(runner.calls[len(runner.calls)-1].env, "BW_SESSION"); got != "US-session" {
		t.Fatal("profile session wasn't passed to protected command")
	}
	before := len(runner.calls)
	if err := client.ConfigureRegion(ctx, dir, "us"); err != nil {
		t.Fatal(err)
	}
	// An externally assigned session makes the cached authentication state
	// unreliable. The endpoint still must not be rewritten.
	if len(runner.calls[before:]) != 1 ||
		strings.Join(runner.calls[before].args, " ") != "status" {
		t.Fatal("an invalidated profile must fall back to status without rewriting")
	}
	for _, path := range []string{filepath.Join(dir, "bitwarden", "sessions"), filepath.Dir(us), us} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("directory permissions for %q: %v %v", path, info, err)
		}
	}
	if err := client.ConfigureRegion(ctx, dir, "custom"); err == nil {
		t.Fatal("unknown regions must be rejected")
	}
	sessionDir := filepath.Dir(us)
	client.Close(ctx)
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatalf("session directory was not removed: %v", err)
	}
}

func TestSessionStartupCleansOnlyInactiveOldDirectories(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	base := filepath.Join(dir, "bitwarden", "sessions")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(base, "old")
	fresh := filepath.Join(base, "fresh")
	active := filepath.Join(base, "active")
	for _, path := range []string{old, fresh, active} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	activeLock, err := lockSession(active, false)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSessionLock(activeLock)
	oldTime := time.Now().Add(-abandonedSessionAge - time.Hour)
	for _, path := range []string{old, active} {
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}

	client, err := NewClient(&regionRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(ctx)
	if err := client.ConfigureRegion(ctx, dir, "us"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("inactive old session was not removed: %v", err)
	}
	for _, path := range []string{fresh, active} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("session %q was removed: %v", path, err)
		}
	}
}

func TestMismatchedProfileLogsOutWithinProfileOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	runner := &regionRunner{
		defaultServer: "https://vault.bitwarden.com",
		defaultStatus: "locked",
	}
	client, _ := NewClient(runner)
	if err := client.ConfigureRegion(ctx, dir, "eu"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 ||
		strings.Join(runner.calls[0].args, " ") != "config server https://vault.bitwarden.eu" {
		t.Fatalf("a new EU profile must be configured directly: %+v", runner.calls)
	}
}

func TestCredentialsFileExpandsHomeVariable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	filename := filepath.Join(home, "api-key.env")
	err := os.WriteFile(filename, []byte(
		"export BW_CLIENTID=user.test\nexport BW_CLIENTSECRET=test-secret\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	id, secret, err := ReadEnvCredentials("$HOME/api-key.env")
	if err != nil || id != "user.test" || secret != "test-secret" {
		t.Fatalf("credentials did not expand HOME (error: %v)", err)
	}
}
