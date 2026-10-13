package ui

import (
	"strings"
	"testing"
)

// TestRepromptRevealToggle verifies that Ctrl+R changes only input visibility.
func TestRepromptRevealToggle(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	s.Mode = DialogReprompt
	if err := s.SetMasterText("sample-secret"); err != nil {
		t.Fatal(err)
	}
	if s.AuthVisible {
		t.Fatal("password must initially be masked")
	}
	if err := a.toggleAuthVisibility(); err != nil {
		t.Fatal(err)
	}
	if !s.AuthVisible || s.MasterText() != "sample-secret" {
		t.Fatal("reveal should not change password contents")
	}
	if a.keyAction("ctrl+r") != "reveal" {
		t.Fatal("Ctrl+R must be reported as reveal in the reprompt popup")
	}
	hints := strings.Join(a.baseHints(), " ")
	if !strings.Contains(hints, "ctrl+r reveal") {
		t.Fatalf("missing reprompt reveal hint: %s", hints)
	}
	if err := a.toggleAuthVisibility(); err != nil {
		t.Fatal(err)
	}
	if s.AuthVisible || s.MasterText() != "sample-secret" {
		t.Fatal("second toggle must mask the same password")
	}
}
