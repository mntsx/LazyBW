package bitwarden

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServeVersionSupported(t *testing.T) {
	for value, want := range map[string]bool{
		"2025.8.9":  false,
		"2025.9.0":  true,
		"v2025.9.0": true,
		"2025.10.0": true,
		"2026.1.0":  true,
		"unknown":   false,
	} {
		if got := serveVersionSupported(value); got != want {
			t.Errorf("serveVersionSupported(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestNormalizeServeListResponse(t *testing.T) {
	payload := []byte(`{"object":"list","data":[{"id":"one"}]}`)
	result, err := normalizeServeResponse([]string{"list", "items"}, payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `[{"id":"one"}]` {
		t.Fatalf("normalized list = %s", result)
	}
	for _, malformed := range [][]byte{
		[]byte(`{"object":"list","data":null}`),
		[]byte(`{"object":"item","data":[]}`),
		[]byte(`{"object":"list","data":{}}`),
	} {
		if _, err := normalizeServeResponse([]string{"list", "items"}, malformed); err == nil {
			t.Fatalf("malformed list response was accepted: %s", malformed)
		}
	}
}

func TestServeDirectoryIsPrivateAndSocketSafe(t *testing.T) {
	directory, err := secureServeDirectory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	if len(filepath.Join(directory, "bw.sock")) >= 104 {
		t.Fatalf("Unix socket path is too long: %q", directory)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("directory permissions = %o, want 0700", info.Mode().Perm())
	}
}
