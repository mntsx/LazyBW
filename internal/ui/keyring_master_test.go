package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestMasterEncryptedBetweenEdits preserves editing without retaining plaintext.
func TestMasterEncryptedBetweenEdits(t *testing.T) {
	s := setup(t)
	s.Screen = AuthMaster
	s.Master = "original-pass"
	if err := s.ProtectMaster(); err != nil {
		t.Fatal(err)
	}
	if s.Master != "" || s.masterSealed == nil {
		t.Fatal("the master password remains in the state as plaintext")
	}
	if s.MasterText() != "original-pass" {
		t.Fatal("could not decrypt stored password")
	}
	a := New(s)
	first := s.masterSealed
	a.setAuthValue("modified-pass")
	for _, b := range first.Data {
		if b != 0 {
			t.Fatal("replaced encrypted password not zeroed")
		}
	}
	if s.Master != "" || a.authValue() != "modified-pass" {
		t.Fatal("editing must preserve encrypted-at-rest state")
	}
	s.Caret = len([]rune(a.authValue()))
	if err := a.wordEdit(-1, true); err != nil || a.authValue() != "modified-" {
		t.Fatalf("protected text editing failed: %v %q", err, a.authValue())
	}
	original := s.masterSealed
	s.ClearMaster()
	if s.MasterText() != "" || s.masterSealed != nil {
		t.Fatal("master password was not discarded")
	}
	for _, b := range original.Data {
		if b != 0 {
			t.Fatal("master ciphertext was not overwritten")
		}
	}
}

// TestAuthEncryptionIndicator verifies both forms display the active method.
func TestAuthEncryptionIndicator(t *testing.T) {
	s := setup(t)
	a := New(s)
	for _, screen := range []Screen{AuthClient, AuthMaster} {
		s.Screen = screen
		c := NewCanvas(120, 42)
		a.authRender(c)
		row := stringCellRow(c, 1)
		label := "AES-256-GCM / " + s.CLI.Cipher.Backend()
		if !strings.Contains(row, "\U000F0780 "+label) || strings.Contains(row, "cipher:") {
			t.Fatalf("%v: unexpected cipher method label: %q", screen, row)
		}
	}
}

// TestAesSealedDoesNotUseMaster checks that both secrets share the AEAD backend.
func TestAesSealedDoesNotUseMaster(t *testing.T) {
	s := setup(t)
	if err := s.ProtectMaster(); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMasterText("temporary"); err != nil {
		t.Fatal(err)
	}
	if _, ok := any(s.masterSealed).(*vault.Sealed); !ok {
		t.Fatal("master plaintext was not sealed")
	}
}
