package bitwarden

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPersistentCapabilitiesReuseAndInvalidate(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	directory := t.TempDir()
	binary := filepath.Join(directory, "bw-test")
	calls := filepath.Join(directory, "calls")
	writeCapabilityScript(t, binary, calls, "2026.10.0")

	first, err := NewClient(ExecRunner{Binary: binary})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.BackendAvailable(context.Background(), BackendBWServe); err != nil {
		t.Fatal(err)
	}
	if got := capabilityCalls(t, calls); got != "--version|serve --help" {
		t.Fatalf("initial capability calls = %q", got)
	}
	second, err := NewClient(ExecRunner{Binary: binary})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.BackendAvailable(context.Background(), BackendBWServe); err != nil {
		t.Fatal(err)
	}
	if got := capabilityCalls(t, calls); got != "--version|serve --help" {
		t.Fatalf("valid persistent cache invoked bw again: %q", got)
	}
	cacheFile := filepath.Join(cache, "lazybw", capabilityCacheFile)
	if err := os.WriteFile(cacheFile, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	third, _ := NewClient(ExecRunner{Binary: binary})
	if err := third.BackendAvailable(context.Background(), BackendBWServe); err != nil {
		t.Fatal(err)
	}
	if got := capabilityCalls(t, calls); got != "--version|serve --help|--version|serve --help" {
		t.Fatalf("corrupt cache was accepted: %q", got)
	}
	writeCapabilityScript(t, binary, calls, "2025.8.9")
	fourth, _ := NewClient(ExecRunner{Binary: binary})
	if err := fourth.BackendAvailable(context.Background(), BackendBWServe); err == nil {
		t.Fatal("updated incompatible executable was accepted from stale cache")
	}
	if got := capabilityCalls(t, calls); !strings.HasSuffix(got, "|--version") {
		t.Fatalf("changed executable did not recheck version: %q", got)
	}
}

func writeCapabilityScript(t *testing.T, path, calls, version string) {
	t.Helper()
	script := "#!/bin/sh\necho \"$@\" >> " + calls + "\nif [ \"$1\" = \"--version\" ]; then echo " + version + "; exit 0; fi\nif [ \"$1\" = \"serve\" ]; then echo --hostname; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
}

func capabilityCalls(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(strings.TrimSpace(strings.ReplaceAll(string(data), "\n", "|")), "|")
}

type persistentLoginRunner struct {
	mu    sync.Mutex
	calls []string
}

func (r *persistentLoginRunner) Run(_ context.Context, args []string, _ []byte, env []string) ([]byte, []byte, int, error) {
	profile := envValue(env, "BITWARDENCLI_APPDATA_DIR")
	call := strings.Join(args, " ")
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
	switch call {
	case "login --apikey":
		data := []byte(`{"appId":"device-id","token":"secret-token"}`)
		if err := os.WriteFile(filepath.Join(profile, profileDataFile), data, 0600); err != nil {
			return nil, nil, 1, err
		}
		return nil, nil, 0, nil
	case "status":
		if _, err := os.Stat(filepath.Join(profile, profileDataFile)); err == nil {
			return []byte(`{"status":"locked","serverUrl":"https://vault.bitwarden.com","userId":"test"}`), nil, 0, nil
		}
		return []byte(`{"status":"unauthenticated","serverUrl":"https://vault.bitwarden.com"}`), nil, 0, nil
	default:
		return nil, nil, 0, nil
	}
}

func (r *persistentLoginRunner) commands() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.calls, "|")
}

func TestPersistentLoginRestoresLockedProfileAndDevice(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	runner := &persistentLoginRunner{}
	first, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	first.ConfigureClientLoginCache(directory, true, "user.test")
	if _, err := first.ConfigureRegionStatus(ctx, directory, "us"); err != nil {
		t.Fatal(err)
	}
	if err := first.LoginUnauthenticated(ctx, "user.test", "secret"); err != nil {
		t.Fatal(err)
	}
	first.Close(ctx)

	cached := filepath.Join(directory, "bitwarden", "cached-logins", "us", profileDataFile)
	data, err := readPrivateFile(cached)
	if err != nil || strings.Contains(string(data), "secret-token") == false {
		t.Fatalf("locked login was not cached: %v", err)
	}
	device, err := readPrivateFile(filepath.Join(directory, "bitwarden", "devices", "us.json"))
	if err != nil || strings.Contains(string(device), "device-id") == false || strings.Contains(string(device), "secret-token") {
		t.Fatalf("device cache leaked data or was missing: %s (%v)", device, err)
	}

	second, err := NewClient(runner)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close(ctx)
	second.ConfigureClientLoginCache(directory, true, "user.test")
	status, err := second.ConfigureRegionStatus(ctx, directory, "us")
	if err != nil {
		t.Fatal(err)
	}
	if status["status"] != "locked" {
		t.Fatalf("cached login status = %#v", status)
	}
	if strings.Count(runner.commands(), "login --apikey") != 1 {
		t.Fatalf("cached login repeated API login: %s", runner.commands())
	}
}

func TestDisablingLoginCacheRemovesLoginsButKeepsDevices(t *testing.T) {
	directory := t.TempDir()
	login := filepath.Join(directory, "bitwarden", "cached-logins", "eu", profileDataFile)
	device := filepath.Join(directory, "bitwarden", "devices", "eu.json")
	if err := writePrivateFile(login, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(device, []byte(`{"appId":"device"}`)); err != nil {
		t.Fatal(err)
	}
	client, _ := NewClient(&persistentLoginRunner{})
	client.ConfigureClientLoginCache(directory, false, "")
	if _, err := os.Stat(login); !os.IsNotExist(err) {
		t.Fatalf("cached login survived disabled setting: %v", err)
	}
	if _, err := os.Stat(device); err != nil {
		t.Fatalf("device cache was removed: %v", err)
	}
}

func TestCachedLoginMetadataNeverContainsUnlockedSession(t *testing.T) {
	metadata := cachedLoginMetadata{Generation: "x", ClientID: "user.test", UserID: "test", DataHash: "hash"}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "BW_SESSION") || strings.Contains(string(encoded), "master") {
		t.Fatalf("metadata retained an unlock secret: %s", encoded)
	}
}
