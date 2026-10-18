package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miguelmnts/lazybw-go/internal/localvault"
)

// TestLocalServerIcon uses the requested long Unicode glyph in both auth UIs.
func TestLocalReplicaTimestampFormat(t *testing.T) {
	if got := formatReplicaTimestamp("2026-10-19T14:37:52.123Z"); got != "2026-10-19T14:37" {
		t.Fatalf("replica timestamp = %q", got)
	}
}

func TestSwitchToLocalLoadsReplicaTimestamp(t *testing.T) {
	s := setup(t)
	path := filepath.Join(t.TempDir(), localvault.Filename)
	s.Config.Local.VaultPath = path
	s.Config.Local.VaultEncryption = localvault.AESGCM
	session, err := localvault.NewSession("master password", s.CLI.Cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	created := time.Date(2026, 10, 19, 14, 37, 52, 0, time.UTC)
	encrypted, err := session.Encode(localvault.Snapshot{}, "eu", localvault.AESGCM, created)
	if err != nil {
		t.Fatal(err)
	}
	if err = localvault.WriteAtomic(path, encrypted); err != nil {
		t.Fatal(err)
	}
	New(s).switchToLocal()
	if s.LocalOrigin != "eu" || s.LocalAlgorithm != localvault.AESGCM ||
		s.LocalReplicaCreated != "2026-10-19T14:37" {
		t.Fatalf("local metadata was not loaded: origin=%q algorithm=%q created=%q",
			s.LocalOrigin, s.LocalAlgorithm, s.LocalReplicaCreated)
	}
}

func TestLocalReplicaTimestampReplacesCipher(t *testing.T) {
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		s := setup(t)
		s.LocalMode = true
		s.Server = "local"
		s.Screen = screen
		s.LocalAlgorithm = localvault.AESGCM
		s.LocalReplicaCreated = "2026-10-19T14:37"
		c := NewCanvas(120, 44)
		New(s).authRender(c)
		row := stringCellRow(c, 1)
		if strings.Contains(row, "cipher:") || !strings.Contains(row, "\U000F01DA 2026-10-19T14:37") {
			t.Fatalf("screen %v: unexpected local indicator %q", screen, row)
		}
		label := "\U000F01DA " + s.LocalReplicaCreated
		x := c.W - 2 - len([]rune(label))
		if icon := canvasAt(c, x, 1); icon.R != '\U000F01DA' || icon.FG != CBlue {
			t.Fatalf("screen %v: local replica icon mismatch: %+v", screen, icon)
		}
		if date := canvasAt(c, x+2, 1); date.R != '2' || date.FG != CWhite {
			t.Fatalf("screen %v: local replica date mismatch: %+v", screen, date)
		}
	}
}

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
