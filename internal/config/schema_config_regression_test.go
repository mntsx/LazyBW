package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedJSONCIsAnnotatedAndListsEveryContext(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := os.ReadFile(filepath.Join(dir, "settings.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settings), "// Allowed values: us, eu") ||
		!strings.Contains(string(settings), "xchacha20-poly1305") ||
		!strings.Contains(string(settings), "ansi256:0..255") {
		t.Fatal("generated settings missing valid-value comments")
	}
	file, err := os.ReadFile(filepath.Join(dir, "keybinds.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"content", "folders", "edits", "sync_popup",
		"folder_selector_popup", "card_brand_popup", "authentication_offline", "offline", "text_input"} {
		if !strings.Contains(string(file), "\""+section+"\": {") {
			t.Errorf("missing keybinding section %s", section)
		}
	}
	if !strings.Contains(string(file), `"switch": [`) ||
		!strings.Contains(string(file), `"right"`) ||
		!strings.Contains(string(file), `"enter"`) {
		t.Fatal("checkbox alternate keys absent")
	}
	if !strings.Contains(string(file), `"select_all": [`) ||
		!strings.Contains(string(file), `"ctrl+a"`) ||
		cfg.Key("detail", "select_all") != "ctrl+a" || cfg.Key("detail", "cut") != "ctrl+x" ||
		cfg.Key("detail", "maximize") != "ctrl+e" {
		t.Fatal("detail select-all default is absent or incorrect")
	}
	if cfg.Key("text_input", "delete_backward") != "backspace" ||
		cfg.Key("offline", "quit") != "enter" || cfg.Keys("global", "hint_next_page")[1] != "ctrl+n" {
		t.Fatal("global text-input or offline default is incorrect")
	}
	if got := cfg.Key("content", "switch"); got != "s" {
		t.Fatalf("wrong hint priority: %q", got)
	}
}

func TestKeybindsAcceptOrderedArraysAndLegacyStrings(t *testing.T) {
	dir := t.TempDir()
	data := `{
      "content": {"switch": ["v", "s", "right", "enter"]},
      "global": {"save_all": "ctrl+y"},
      "changes": {"discard": ["x", "d"]}
    }`
	if err := os.WriteFile(filepath.Join(dir, "keybinds.jsonc"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Key("content", "switch") != "v" || len(cfg.Keys("content", "switch")) != 4 ||
		cfg.Key("global", "push_all") != "ctrl+y" || cfg.Key("edits", "discard") != "x" {
		t.Fatalf("incomplete compatibility: %#v", cfg.Keybinds)
	}
}
