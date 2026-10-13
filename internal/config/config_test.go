package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONC(t *testing.T) {
	b, err := NormalizeJSONC([]byte(`{"url":"https://foo//bar", /* comment */ "list":[1,2,], // end
 "v":3,}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "https://foo//bar") || strings.Contains(string(b), "comment") || strings.Contains(string(b), "2,") {
		t.Fatalf("incorrect JSONC: %s", b)
	}
}
func TestDefaultFilesAndPrecedence(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"settings.jsonc", "keybinds.jsonc"} {
		if _, err = os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	old := filepath.Join(dir, "settings.json")
	_ = os.WriteFile(old, []byte(`{"colors":{"selection":"ansi_red"}}`), 0600)
	v, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v.Colors["selection"] != "ansi_black" {
		t.Fatal("JSONC must take precedence")
	}
}
func TestLegacyFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"colors":{"selection":"ansi_red"}}`), 0600)
	v, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v.Colors["selection"] != "ansi_red" {
		t.Fatal("legacy JSON not loaded")
	}
	if _, err = os.Stat(filepath.Join(dir, "settings.jsonc")); err == nil {
		t.Fatal("unexpected duplicate")
	}
}

func TestLocalAndNetworkDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Local.SaveVault || cfg.Local.VaultPath != "$HOME/.local/share/lazybw/bw-vault" || cfg.Local.VaultEncryption != "aes-256-gcm" ||
		cfg.Network.USPingURL != "https://api.bitwarden.com" || cfg.Network.EUPingURL != "https://api.bitwarden.eu" {
		t.Fatalf("unexpected backup configuration: %+v %+v", cfg.Local, cfg.Network)
	}
	settings, err := os.ReadFile(filepath.Join(dir, "settings.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settings), "vault_encryption") || !strings.Contains(string(settings), "us_ping_url") {
		t.Fatal("default settings omit local/network fields")
	}
}
