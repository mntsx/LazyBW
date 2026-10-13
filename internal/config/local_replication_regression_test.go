package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalReplicationConfigKey requires the renamed JSONC section.
func TestLocalReplicationConfigKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.jsonc")
	config := `{"local":{"save_vault":true},"local_replication":{
		"save_vault":true,"vault_path":"/tmp/bw-vault",
		"vault_encryption":"aes-256-gcm-siv"}}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Local.SaveVault || got.Local.VaultPath != "/tmp/bw-vault" ||
		got.Local.VaultEncryption != "aes-256-gcm-siv" {
		t.Fatalf("renamed local replication settings not applied: %+v", got.Local)
	}
	other := t.TempDir()
	generated, err := Load(other)
	if err != nil || generated.Local.SaveVault {
		t.Fatal("invalid default configuration")
	}
	data, err := os.ReadFile(filepath.Join(other, "settings.jsonc"))
	if err != nil || !strings.Contains(string(data), `"local_replication"`) ||
		strings.Contains(string(data), `"local":`) {
		t.Fatal("generated configuration retained the previous key")
	}
}
