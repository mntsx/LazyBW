package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsInvalidSettingsValues(t *testing.T) {
	cases := []struct {
		name, content string
	}{
		{"server enum", `{"authentication":{"default_server":"moon"}}`},
		{"client path type", `{"authentication":{"client_env_path":42}}`},
		{"local boolean", `{"local_replication":{"save_vault":"yes"}}`},
		{"encryption enum", `{"local_replication":{"vault_encryption":"rot13"}}`},
		{"network URL", `{"network":{"us_ping_url":"http://example.test"}}`},
		{"color value", `{"colors":{"primary":"definitely-not-a-color"}}`},
		{"object structure", `{"authentication":"invalid"}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "settings.jsonc"), []byte(test.content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(directory); !errors.Is(err, errInvalidConfigValue) || !strings.Contains(err.Error(), "settings.jsonc:") {
				t.Fatalf("invalid settings error: %v", err)
			}
		})
	}
}

func TestLoadRejectsNonBooleanCacheClientLogin(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "settings.jsonc"), []byte(`{"authentication":{"cache_client_login":"yes"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(directory)
	if !errors.Is(err, errInvalidConfigValue) || !strings.Contains(err.Error(), "authentication.cache_client_login") {
		t.Fatalf("invalid cache client login error: %v", err)
	}
}

func TestLoadRejectsInvalidKeybindValues(t *testing.T) {
	cases := []struct {
		name, content string
	}{
		{"binding type", `{"items":{"search":true}}`},
		{"binding list element", `{"items":{"search":["f",3]}}`},
		{"null binding", `{"items":{"search":null}}`},
		{"empty string binding", `{"items":{"search":"   "}}`},
		{"unknown action", `{"items":{"seach":"f"}}`},
		{"section structure", `{"items":null}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "keybinds.jsonc"), []byte(test.content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(directory); !errors.Is(err, errInvalidConfigValue) || !strings.Contains(err.Error(), "keybinds.jsonc:") {
				t.Fatalf("invalid keybinds error: %v", err)
			}
		})
	}
}
