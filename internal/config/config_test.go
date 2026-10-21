package config

import (
	"encoding/json"
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
func TestLongUnicodeEscapesInJSONC(t *testing.T) {
	data, err := NormalizeJSONC([]byte(`{"icon":"\U000F0379"}`))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value["icon"] != "\U000F0379" {
		t.Fatalf("icon = %q", value["icon"])
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
func TestLoadCreatesGitignoreForBitwardenData(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "bitwarden/\n" {
		t.Fatalf(".gitignore = %q, want bitwarden/", contents)
	}
}

func TestLoadPreservesExistingGitignore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("custom/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "custom/\n" {
		t.Fatalf("existing .gitignore was changed: %q", contents)
	}
}

func TestResetRestoresOnlySelectedConfigurationFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, "settings.jsonc")
	keybindsPath := filepath.Join(dir, "keybinds.jsonc")
	if err := os.WriteFile(settingsPath, []byte(`{"network":{"us_ping_url":"https://custom.example"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	originalKeybinds := []byte(`{"items":{"search":["x"]}}`)
	if err := os.WriteFile(keybindsPath, originalKeybinds, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Reset(dir, true, false); err != nil {
		t.Fatal(err)
	}
	keybinds, err := os.ReadFile(keybindsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(keybinds) != string(originalKeybinds) {
		t.Fatalf("keybinds changed while resetting settings: %q", keybinds)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Network.USPingURL != "https://api.bitwarden.com" {
		t.Fatalf("settings were not reset: %q", cfg.Network.USPingURL)
	}
	if got := cfg.Keys("items", "search"); len(got) != 1 || got[0] != "x" {
		t.Fatalf("custom keybind was not preserved: %v", got)
	}
	if err := Reset(dir, false, true); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Keys("items", "search"); len(got) != 1 || got[0] != "f" {
		t.Fatalf("keybinds were not reset: %v", got)
	}
}

func TestKeybindOverridesReplaceDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keybinds.jsonc"), []byte(`{
		"items": {"search": ["x"]},
		"global": {"sync": []}
	}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Keys("items", "search"); len(got) != 1 || got[0] != "x" {
		t.Fatalf("search override was merged with its old default: %v", got)
	}
	if got := cfg.Keys("global", "sync"); len(got) != 0 {
		t.Fatalf("empty binding did not disable sync: %v", got)
	}
	if got := cfg.Keys("filters", "select"); len(got) != 2 || got[0] != "right" || got[1] != "enter" {
		t.Fatalf("unexpected filter selection defaults: %v", got)
	}
}

func TestOptionPopupDefaultsPreferRightAndKeepAlternatives(t *testing.T) {
	for _, testCase := range []struct {
		section, action, alternative string
	}{
		{"server_picker", "confirm", "enter"},
		{"create_item_popup", "confirm", "enter"},
		{"create_field_popup", "confirm", "enter"},
		{"linked_field_popup", "confirm", "enter"},
		{"card_brand_popup", "confirm", "enter"},
		{"identity_title_popup", "confirm", "enter"},
		{"website_popup", "confirm", "enter"},
		{"folder_selector_popup", "switch", "s"},
	} {
		bindings := DefaultKeybinds[testCase.section][testCase.action]
		if len(bindings) != 2 || bindings[0] != "right" || bindings[1] != testCase.alternative {
			t.Errorf("%s.%s = %v", testCase.section, testCase.action, bindings)
		}
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
