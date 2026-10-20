package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBackendModeDefaultsAndStringCompatibility(t *testing.T) {
	directory := t.TempDir()
	cfg, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"bw serve", "bw"}; !reflect.DeepEqual(cfg.Authentication.BackendMode, want) {
		t.Fatalf("default backend modes = %v, want %v", cfg.Authentication.BackendMode, want)
	}
	path := filepath.Join(directory, "settings.jsonc")
	if err := os.WriteFile(path, []byte(`{
"authentication":{"client_env_path":null,"default_server":"us","backend_mode":"bw serve"},
"local_replication":{"save_vault":false,"vault_path":"/tmp/vault","vault_encryption":"aes-256-gcm"},
"network":{"us_ping_url":"https://api.bitwarden.com","eu_ping_url":"https://api.bitwarden.eu"},"colors":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"bw serve"}; !reflect.DeepEqual(cfg.Authentication.BackendMode, want) {
		t.Fatalf("string backend mode = %v, want %v", cfg.Authentication.BackendMode, want)
	}
}

func TestBackendModeRejectsUnknownEmptyAndDuplicates(t *testing.T) {
	for _, value := range []string{`[]`, `["bw","bw"]`, `["rbw"]`, `1`} {
		t.Run(value, func(t *testing.T) {
			directory := t.TempDir()
			settings := `{"authentication":{"client_env_path":null,"default_server":"us","backend_mode":` + value + `},"local_replication":{"save_vault":false,"vault_path":"/tmp/vault","vault_encryption":"aes-256-gcm"},"network":{"us_ping_url":"https://api.bitwarden.com","eu_ping_url":"https://api.bitwarden.eu"},"colors":{}}`
			if err := os.WriteFile(filepath.Join(directory, "settings.jsonc"), []byte(settings), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(directory)
			if !errors.Is(err, errInvalidConfigValue) || !strings.Contains(err.Error(), "authentication.backend_mode") {
				t.Fatalf("backend mode %s error = %v", value, err)
			}
		})
	}
}
