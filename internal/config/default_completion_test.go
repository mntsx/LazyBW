package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readCompletedObject(t *testing.T, path string) (string, map[string]any) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := NormalizeJSONC(content)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal(normalized, &object); err != nil {
		t.Fatalf("completed configuration is invalid: %v\n%s", err, content)
	}
	return string(content), object
}

func TestLoadPersistsMissingSettingsDefaults(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.jsonc")
	original := `{
  // Keep this user comment.
  "authentication": {"default_server": "eu"},
  "colors": {"selection": "ansi_red"},
  "custom": {"keep": true}
}
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Authentication.DefaultServer != "eu" || cfg.Colors["selection"] != "ansi_red" {
		t.Fatalf("existing settings were overwritten: %+v", cfg)
	}
	content, object := readCompletedObject(t, path)
	if !strings.Contains(content, "// Keep this user comment.") {
		t.Fatal("user JSONC comment was removed")
	}
	authentication := object["authentication"].(map[string]any)
	if _, exists := authentication["client_env_path"]; !exists {
		t.Fatal("missing authentication subfield was not persisted")
	}
	colors := object["colors"].(map[string]any)
	if colors["selection"] != "ansi_red" || colors["background"] == nil {
		t.Fatalf("color defaults were not merged safely: %#v", colors)
	}
	if object["local_replication"] == nil || object["network"] == nil ||
		object["custom"].(map[string]any)["keep"] != true {
		t.Fatalf("root defaults or custom fields were lost: %#v", object)
	}
}

func TestLoadPersistsMissingKeybindDefaultsAndLegacyAliases(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "keybinds.jsonc")
	original := `{
  // Keep custom shortcuts and old names.
  "items": {"search": ["x"]},
  "global": {"save_all": ["ctrl+y"]},
  "changes": {"discard": ["x", "d"]}
}
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key("items", "search") != "x" || first.Key("global", "push_all") != "ctrl+y" ||
		first.Key("edits", "discard") != "x" {
		t.Fatalf("legacy shortcuts changed during completion: %#v", first.Keybinds)
	}
	content, object := readCompletedObject(t, path)
	if !strings.Contains(content, "// Keep custom shortcuts and old names.") {
		t.Fatal("keybind JSONC comment was removed")
	}
	items := object["items"].(map[string]any)
	if items["open"] == nil || items["search"].([]any)[0] != "x" || object["filters"] == nil {
		t.Fatalf("missing keybind defaults were not persisted: %#v", object)
	}
	global := object["global"].(map[string]any)
	edits := object["edits"].(map[string]any)
	if global["push_all"].([]any)[0] != "ctrl+y" || edits["discard"].([]any)[0] != "x" {
		t.Fatalf("legacy aliases were not migrated: global=%#v edits=%#v", global, edits)
	}
	firstContent := content
	second, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	secondContent, _ := readCompletedObject(t, path)
	if second.Key("global", "push_all") != "ctrl+y" || second.Key("edits", "discard") != "x" {
		t.Fatal("persisted legacy shortcuts changed on reload")
	}
	if secondContent != firstContent {
		t.Fatal("default completion is not idempotent")
	}
}
