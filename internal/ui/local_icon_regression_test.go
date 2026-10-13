package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/localvault"
)

// TestLocalServerIcon uses the requested long Unicode glyph in both auth UIs.
func TestLocalServerIcon(t *testing.T) {
	s := setup(t)
	s.LocalMode = true
	s.Server = "local"
	s.Screen = AuthMaster
	a := New(s)
	c := NewCanvas(120, 44)
	a.authRender(c)
	icon := canvasAt(c, 2, 1)
	if icon.R != '\U000F0379' || icon.FG != CBlue {
		t.Fatalf("local auth icon mismatch: %+v", icon)
	}

	path := filepath.Join(t.TempDir(), localvault.Filename)
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	s.Config.Local.VaultPath = path
	s.Mode = DialogServer
	s.ServerChoice = 2
	c = NewCanvas(120, 44)
	a.serverPickerRender(c)
	x, y, _, _ := serverPickerGeometry(120, 44)
	icon = canvasAt(c, x+4, y+5)
	if icon.R != '\U000F0379' || icon.FG != CBlue {
		t.Fatalf("local picker icon mismatch: %+v", icon)
	}
}

// TestLocalFooterUsesNormalRed confirms the offline status isn't bright red.
func TestLocalFooterUsesNormalRed(t *testing.T) {
	s := setup(t)
	s.LocalMode = true
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	for x := 0; x < c.W-2; x++ {
		item := canvasAt(c, x, 42)
		if item.R == '•' && canvasAt(c, x+2, 42).R == 'L' {
			if item.FG != CRed {
				t.Fatalf("offline dot should be normal red: %+v", item)
			}
			return
		}
	}
	t.Fatal("offline status missing")
}
