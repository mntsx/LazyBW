package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

func TestOfflineWorkspaceRendersOnlyTypesFolders(t *testing.T) {
	s := setup(t)
	s.LocalMode = true
	s.Server = "local"
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	left := strings.Join([]string{stringCellRow(c, 1), stringCellRow(c, a.sideY[2])}, "\n")
	if a.sideH[0] != 0 || a.sideH[3] != 0 || a.sideH[1] == 0 || a.sideH[2] == 0 {
		t.Fatalf("incorrect visible side panels: %v", a.sideH)
	}
	if !strings.Contains(left, "Folders") || !strings.Contains(left, "Items") || strings.Contains(left, "Changes") {
		t.Fatalf("unexpected left panels %q", left)
	}
	footer := stringCellRow(c, 42)
	if !strings.Contains(footer, "Local") || strings.Contains(footer, "Bitwarden CLI") {
		t.Fatalf("wrong offline footer: %q", footer)
	}
}

func TestOfflineReadOnlyPreventsKeyboardAndStateMutations(t *testing.T) {
	s := setup(t)
	s.Server = "local"
	s.LocalMode = true
	a := New(s)
	s.Focus = PaneItems
	for _, key := range "nd" {
		if err := a.char(key); err != nil {
			t.Fatal(err)
		}
	}
	if s.Mode != Browse || s.Stage.Len() != 0 || len(s.Items) != 1 {
		t.Fatal("items were modified in local mode")
	}
	s.CreateItem(vault.Login, "other")
	s.DeleteItem()
	s.CreateFolder("new-folder")
	s.OpenItem()
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if err := a.arrow(0, 1); err != nil {
		t.Fatal(err)
	}
	if s.Mode == Edit || s.Focus == PaneDetail || s.Stage.Len() != 0 || len(s.Items) != 1 {
		t.Fatal("content editing must be inaccessible from local mode")
	}
	if err := s.BeginEdit(); err == nil {
		t.Fatal("state edit path allowed offline mutation")
	}
	for _, key := range "14" {
		if err := a.char(key); err != nil {
			t.Fatal(err)
		}
		if s.Focus == PaneVault || s.Focus == PaneChanges {
			t.Fatal("inaccessible side panel focused")
		}
	}
	a.confirmStage(true, true)
	if s.Popup != nil {
		t.Fatal("offline data unexpectedly registered")
	}
}

func TestConfirmedSnapshotUsesOnlyCommittedPersonalItems(t *testing.T) {
	s := setup(t)
	s.Items[0].Raw["name"] = "staged"
	s.Stage.Record("item", s.Items[0].Raw, "Login", s.Originals)
	s.Originals["org-item"] = map[string]any{"id": "org-item", "organizationId": "org-1"}
	a := New(s)
	snapshot := a.confirmedSnapshot()
	if len(snapshot.Items) != 1 || snapshot.Items[0]["name"] != "Example" {
		t.Fatalf("pending changes or organization items leaked: %v", snapshot.Items)
	}
}
