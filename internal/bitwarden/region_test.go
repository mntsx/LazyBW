package bitwarden

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type regionCall struct {
	args []string
	env  []string
}
type regionRunner struct {
	calls  []regionCall
	server map[string]string
	status map[string]string
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
			server = "https://vault.bitwarden.com"
		}
		status := r.status[profile]
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
	eu := filepath.Join(dir, "bitwarden", "eu")
	us := filepath.Join(dir, "bitwarden", "us")
	if runner.server[eu] != "https://vault.bitwarden.eu" {
		t.Fatalf("EU endpoint not configured: %v", runner.server)
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
	if got := envValue(runner.calls[len(runner.calls)-1].env, "BITWARDENCLI_APPDATA_DIR"); got != us {
		t.Fatalf("US command went to %q", got)
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
	// Existing correctly configured profiles must not be rewritten.
	if len(runner.calls[before:]) != 1 ||
		strings.Join(runner.calls[before].args, " ") != "status" {
		t.Fatal("re-entering a configured region must only check status")
	}
	if info, err := os.Stat(us); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("profile permissions: %v %v", info, err)
	}
	if err := client.ConfigureRegion(ctx, dir, "custom"); err == nil {
		t.Fatal("unknown regions must be rejected")
	}
}

func TestMismatchedProfileLogsOutWithinProfileOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	profile := filepath.Join(dir, "bitwarden", "eu")
	runner := &regionRunner{
		server: map[string]string{profile: "https://vault.bitwarden.com"},
		status: map[string]string{profile: "locked"},
	}
	client, _ := NewClient(runner)
	if err := client.ConfigureRegion(ctx, dir, "eu"); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 3 || strings.Join(runner.calls[1].args, " ") != "logout" ||
		strings.Join(runner.calls[2].args, " ") != "config server https://vault.bitwarden.eu" {
		t.Fatalf("incorrect profile repair commands: %+v", runner.calls)
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
