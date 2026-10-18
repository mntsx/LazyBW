package ui

import (
	"strings"
	"testing"
)

func TestDetailSecretRevealAndHint(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	s.OpenItem()
	s.FieldIndex = 1 // Password field.
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Edit || s.Focus != PaneDetail || !s.InputSecret {
		t.Fatal("password editor not opened")
	}
	if err := a.toggleAuthVisibility(); err != nil {
		t.Fatal(err)
	}
	if !s.AuthVisible || s.Input != "secret" {
		t.Fatal("Ctrl+R must reveal without changing the detail editor input")
	}
	if !strings.Contains(strings.Join(a.baseHints(), " "), "ctrl+r reveal") {
		t.Fatal("secret detail editor must advertise Ctrl+R reveal")
	}
	if err := a.toggleAuthVisibility(); err != nil {
		t.Fatal(err)
	}
	if s.AuthVisible {
		t.Fatal("second Ctrl+R must hide the secret again")
	}
	s.ExitEdit()
	s.FieldIndex = 0 // Username field.
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	if err := a.toggleAuthVisibility(); err != nil {
		t.Fatal(err)
	}
	if s.AuthVisible {
		t.Fatal("Ctrl+R must not toggle visibility for ordinary text fields")
	}
}

func TestPopupDoesNotRecenterWorkspaceFooter(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	s.Screen = Workspace
	s.Popup = &Popup{Title: "Confirm", Question: "Proceed?", Acknowledge: true}
	c := NewCanvas(120, 40)
	a.footer(c, 1, 38, 118)
	if !strings.HasPrefix(stringCellRow(c, 38), " ENTER") {
		t.Fatalf("workspace footer should remain left aligned with popup: %q", stringCellRow(c, 38))
	}
}

func TestEmptyItemsAreaReceivesMouseFocus(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	s.Screen = Workspace
	s.Focus = PaneVault
	a.mainRender(NewCanvas(120, 44))
	if err := a.clickAt(a.itemX+4, 30); err != nil {
		t.Fatal(err)
	}
	if s.Focus != PaneItems {
		t.Fatal("clicking unused items-panel space must focus Items")
	}
}

func TestEmptyItemsAreaLeavesFindMode(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	s.Screen = Workspace
	s.Focus = PaneItems
	s.Mode = Find
	s.Search = "query"
	s.Input = s.Search
	a.mainRender(NewCanvas(120, 44))

	if err := a.clickAt(a.itemX+4, 30); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Browse || s.Focus != PaneItems {
		t.Fatalf("empty-area click left mode=%v focus=%v; want Browse/Items", s.Mode, s.Focus)
	}
	if s.Search != "query" {
		t.Fatal("leaving Find by mouse must preserve the active query")
	}
}
