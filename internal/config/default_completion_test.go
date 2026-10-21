package config

import (
	"encoding/json"
	"errors"
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
  "local_replication": {"save_vault": true},
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
	if cfg.Authentication.DefaultServer != "eu" || !cfg.Authentication.CacheClientLogin || cfg.Colors["selection"] != "ansi_red" ||
		!cfg.Local.SaveVault || cfg.Local.VaultPath != "$HOME/.local/share/lazybw/bw-vault" {
		t.Fatalf("existing or first-load default settings are wrong: %+v", cfg)
	}
	content, object := readCompletedObject(t, path)
	if !strings.Contains(content, "// Keep this user comment.") {
		t.Fatal("user JSONC comment was removed")
	}
	authentication := object["authentication"].(map[string]any)
	if _, exists := authentication["client_env_path"]; !exists {
		t.Fatal("missing authentication subfield was not persisted")
	}
	if authentication["cache_client_login"] != true {
		t.Fatalf("cache client login default = %#v, want true", authentication["cache_client_login"])
	}
	colors := object["colors"].(map[string]any)
	if colors["selection"] != "ansi_red" || colors["background"] == nil {
		t.Fatalf("color defaults were not merged safely: %#v", colors)
	}
	if object["local_replication"] == nil || object["network"] == nil ||
		object["custom"].(map[string]any)["keep"] != true {
		t.Fatalf("root defaults or custom fields were lost: %#v", object)
	}
	if _, err = Load(directory); err != nil {
		t.Fatal(err)
	}
	completedAgain, _ := readCompletedObject(t, path)
	if completedAgain != content {
		t.Fatal("settings default completion is not idempotent")
	}
}

func TestLoadPreservesDisabledCacheClientLogin(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.jsonc")
	if err := os.WriteFile(path, []byte(`{"authentication":{"client_env_path":null,"cache_client_login":false}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Authentication.CacheClientLogin {
		t.Fatal("explicitly disabled client-login cache was changed")
	}
	_, object := readCompletedObject(t, path)
	authentication := object["authentication"].(map[string]any)
	if authentication["cache_client_login"] != false {
		t.Fatalf("persisted cache client login = %#v, want false", authentication["cache_client_login"])
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
	detail := object["detail"].(map[string]any)
	textInput := object["text_input"].(map[string]any)
	if items["open"] == nil || items["search"].([]any)[0] != "x" || object["filters"] == nil ||
		detail["select_all"].([]any)[0] != "ctrl+a" ||
		textInput["delete_backward"].([]any)[0] != "backspace" {
		t.Fatalf("missing keybind defaults were not persisted: %#v", object)
	}
	global := object["global"].(map[string]any)
	changes := object["changes"].(map[string]any)
	if global["save_all"].([]any)[0] != "ctrl+y" || global["push_all"] != nil ||
		changes["discard"].([]any)[0] != "x" || changes["save"] == nil || object["edits"] != nil {
		t.Fatalf("legacy aliases were not completed safely: global=%#v changes=%#v", global, changes)
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
	updated := strings.ReplaceAll(secondContent, "ctrl+y", "ctrl+z")
	updated = strings.Replace(updated, `"discard": ["x", "d"]`, `"discard": ["z", "d"]`, 1)
	if err = os.WriteFile(path, []byte(updated), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if third.Key("global", "push_all") != "ctrl+z" || third.Key("edits", "discard") != "z" {
		t.Fatal("later edits to legacy shortcuts stopped taking effect")
	}
}

func TestDefaultCompletionPreservesExistingIndentation(t *testing.T) {
	for name, indentation := range map[string]string{
		"four spaces": "    ",
		"tabs":        "\t",
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "settings.jsonc")
			original := "{\n" +
				indentation + "\"authentication\": {\n" +
				indentation + indentation + "\"default_server\": \"eu\"\n" +
				indentation + "},\n" +
				indentation + "\"colors\": {}\n" +
				"}\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(directory); err != nil {
				t.Fatal(err)
			}
			content, _ := readCompletedObject(t, path)
			if !strings.Contains(content, "\n"+indentation+indentation+"\"client_env_path\"") {
				t.Fatalf("nested defaults did not retain %s indentation:\n%s", name, content)
			}
			if !strings.Contains(content, "\n"+indentation+"\"local_replication\"") {
				t.Fatalf("root defaults did not retain %s indentation:\n%s", name, content)
			}
		})
	}
}

func TestAtomicCompletionRefusesConcurrentChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.jsonc")
	original := []byte(`{"authentication":{}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	err := writeConfigAtomic(path, []byte(`{"replacement":true}`), []byte(`{"stale":true}`), false)
	if !errors.Is(err, errConfigChanged) {
		t.Fatalf("concurrent change error = %v", err)
	}
	current, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(current) != string(original) {
		t.Fatal("stale completion overwrote the current configuration")
	}
}

func TestDefaultCompletionPreservesConfigurationSymlinks(t *testing.T) {
	directory := t.TempDir()
	targetDirectory := t.TempDir()
	target := filepath.Join(targetDirectory, "settings.jsonc")
	if err := os.WriteFile(target, []byte("{\n  // managed target\n  \"authentication\": {}\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "settings.jsonc")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Load(directory); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("configuration symlink was replaced: mode=%v", info.Mode())
	}
	content, object := readCompletedObject(t, target)
	if !strings.Contains(content, "// managed target") || object["local_replication"] == nil {
		t.Fatal("symlink target was not completed while preserving comments")
	}
}
