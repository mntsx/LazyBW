package bitwarden

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type optimizationRunner struct {
	mu           sync.Mutex
	calls        []string
	loginFailure bool
	version      string
	help         string
}

func (r *optimizationRunner) Run(_ context.Context, args []string, _ []byte, _ []string) ([]byte, []byte, int, error) {
	call := strings.Join(args, " ")
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
	switch call {
	case "--version":
		if r.version == "" {
			return []byte("2026.10.0"), nil, 0, nil
		}
		return []byte(r.version), nil, 0, nil
	case "serve --help":
		if r.help == "" {
			return []byte("--hostname"), nil, 0, nil
		}
		return []byte(r.help), nil, 0, nil
	case "status":
		return []byte(`{"status":"unauthenticated","serverUrl":"https://vault.bitwarden.com"}`), nil, 0, nil
	case "login --apikey":
		if r.loginFailure {
			return nil, []byte("network"), 1, nil
		}
		return nil, nil, 0, nil
	default:
		return nil, nil, 0, nil
	}
}

func (r *optimizationRunner) commands() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.calls, "|")
}

func TestBackendSelectionCachesServeProbeForPickerAndFooter(t *testing.T) {
	runner := &optimizationRunner{}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := client.SelectAvailableBackend(ctx, []string{string(BackendBWServe)}); err != nil {
		t.Fatal(err)
	}
	if err := client.BackendAvailable(ctx, BackendBWServe); err != nil {
		t.Fatal(err)
	}
	if version := client.Version(ctx); version != "2026.10.0" {
		t.Fatalf("footer version = %q", version)
	}
	if got := runner.commands(); got != "--version|serve --help" {
		t.Fatalf("backend probing was repeated: %s", got)
	}
}

func TestBackendSelectionRejectsIncompatibleServe(t *testing.T) {
	runner := &optimizationRunner{version: "2025.8.9"}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.BackendAvailable(context.Background(), BackendBWServe); err == nil {
		t.Fatal("unsupported bw serve version was accepted")
	}
	if got := runner.commands(); got != "--version" {
		t.Fatalf("incompatible version checked unexpected commands: %s", got)
	}
}

func TestFailedLoginInvalidatesKnownProfileStatus(t *testing.T) {
	runner := &optimizationRunner{loginFailure: true}
	client, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	defer client.Close(ctx)
	if _, err := client.ConfigureRegionStatus(ctx, t.TempDir(), "us"); err != nil {
		t.Fatal(err)
	}
	if err := client.LoginUnauthenticated(ctx, "user.test", "secret"); err == nil {
		t.Fatal("failing login was accepted")
	}
	if _, err := client.Status(ctx); err != nil {
		t.Fatal(err)
	}
	if got := runner.commands(); got != "login --apikey|status" {
		t.Fatalf("failed login did not invalidate status: %s", got)
	}
}

func TestFreshProfilesAvoidStatusAndConfigureOnlyEU(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		region string
		want   string
	}{
		{region: "us", want: ""},
		{region: "eu", want: "config server https://vault.bitwarden.eu"},
	} {
		t.Run(test.region, func(t *testing.T) {
			runner := &optimizationRunner{}
			client, err := NewClient(runner)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close(ctx)
			status, err := client.ConfigureRegionStatus(ctx, t.TempDir(), test.region)
			if err != nil {
				t.Fatal(err)
			}
			if status["status"] != "unauthenticated" {
				t.Fatalf("fresh profile status = %#v", status)
			}
			if _, err := client.Status(ctx); err != nil {
				t.Fatal(err)
			}
			if got := runner.commands(); got != test.want {
				t.Fatalf("fresh %s commands = %q, want %q", test.region, got, test.want)
			}
		})
	}
}
