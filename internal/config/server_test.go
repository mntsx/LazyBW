package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultServerConfiguration(t *testing.T) {
	for _, test := range []struct {
		name, value, want string
	}{
		{"missing", `"client_env_path":"/tmp/creds"`, "us"},
		{"null", `"client_env_path":"/tmp/creds", "default_server":null`, "us"},
		{"invalid", `"client_env_path":"/tmp/creds", "default_server":"other"`, "us"},
		{"number", `"client_env_path":"/tmp/creds", "default_server":4`, "us"},
		{"eu", `"client_env_path":"/tmp/creds", "default_server":"eu"`, "eu"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "settings.jsonc"), []byte(`{"authentication":{`+test.value+`}}`), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(dir)
			if err != nil || cfg.Authentication.DefaultServer != test.want ||
				cfg.Authentication.ClientEnvPath != "/tmp/creds" {
				t.Fatalf("config: %#v, err: %v", cfg.Authentication, err)
			}
		})
	}
	dir := t.TempDir()
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "settings.jsonc"))
	if err != nil || !strings.Contains(string(b), `"default_server": "us"`) {
		t.Fatalf("generated defaults: %s, %v", b, err)
	}
}
