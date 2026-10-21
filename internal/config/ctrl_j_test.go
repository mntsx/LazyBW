package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyEnterNewlineBindingsMigrateToCtrlJ(t *testing.T) {
	for _, legacy := range []string{"ctrl+enter", "shift+enter"} {
		t.Run(legacy, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "keybinds.jsonc")
			content := `{"detail":{"newline":["` + legacy + `"]}}`
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}

			cfg, err := Load(directory)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := cfg.Keys("detail", "newline"), []string{"ctrl+j"}; len(got) != 1 || got[0] != want[0] {
				t.Fatalf("legacy newline binding: got %v, want %v", got, want)
			}
		})
	}
}
