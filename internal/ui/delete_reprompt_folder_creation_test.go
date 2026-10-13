package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestProtectedDeletionRequiresFreshPassword verifies the deletion gate.
func TestProtectedDeletionRequiresFreshPassword(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	if err := s.CLI.SetRepromptMaster("correct master"); err != nil {
		t.Fatal(err)
	}
	item := s.CurrentItem()
	item.Raw["reprompt"] = 1
	a := New(s)

	// Even a direct staging call must not bypass re-prompt.
	s.DeleteItem()
	if s.Stage.Len() != 0 || len(s.Items) != 1 {
		t.Fatal("protected item deleted without password")
	}
	if err := a.char('d'); err != nil {
		t.Fatal(err)
	}
	if s.Popup == nil {
		t.Fatal("deletion confirmation missing")
	}
	s.Popup.Yes = true
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogReprompt || s.RepromptDeleteID != item.ID() {
		t.Fatal("confirmed deletion must request master password")
	}
	if err := s.SetMasterText("wrong password"); err != nil {
		t.Fatal(err)
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogReprompt || s.RepromptError == "" ||
		s.Stage.Len() != 0 || len(s.Items) != 1 {
		t.Fatal("invalid password must not delete the protected item")
	}
	if err := s.SetMasterText("correct master"); err != nil {
		t.Fatal(err)
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Browse || s.RepromptDeleteID != "" ||
		s.RepromptApprovedID != "" || len(s.Items) != 0 {
		t.Fatal("verified deletion must remove item and clear authorization")
	}
	changes := s.Stage.Changes()
	if len(changes) != 1 || changes[0].Kind != vault.Deleted {
		t.Fatalf("expected one staged deletion, got %#v", changes)
	}
}

// TestProtectedDeletionCancelDoesNotAuthorize checks both cancel paths.
func TestProtectedDeletionCancelDoesNotAuthorize(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.CurrentItem().Raw["reprompt"] = 1
	a := New(s)
	if err := a.char('d'); err != nil {
		t.Fatal(err)
	}
	if err := a.escape(); err != nil {
		t.Fatal(err)
	}
	if s.Popup != nil || s.Mode != Browse || s.Stage.Len() != 0 {
		t.Fatal("cancelled confirmation changed item state")
	}
	if err := a.char('d'); err != nil {
		t.Fatal(err)
	}
	s.Popup.Yes = true
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if err := a.escape(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Browse || s.RepromptTargetID != "" ||
		s.RepromptDeleteID != "" || s.Stage.Len() != 0 {
		t.Fatal("cancelled re-prompt must clear pending deletion")
	}
	s.DeleteItem()
	if len(s.Items) != 1 {
		t.Fatal("cancelled re-prompt must not authorize a later deletion")
	}
}

// TestCreationUsesAppliedFolder verifies that cursor highlight is not applied.
func TestCreationUsesAppliedFolder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		active int
		cursor int
		want   string
	}{
		{name: "active folder", active: 1, cursor: 2, want: "folder-a"},
		{name: "other active folder", active: 2, cursor: 1, want: "folder-b"},
		{name: "all folders", active: -1, cursor: 1, want: ""},
		{name: "no folder", active: 0, cursor: 1, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setup(t)
			defer s.CLI.Cipher.Close()
			s.Folders = []map[string]any{
				{"id": "folder-a", "name": "A"},
				{"id": "folder-b", "name": "B"},
			}
			s.FilterFolder = tc.active
			s.Filters[2] = tc.cursor
			s.CreateItem(vault.Login, "New entry")
			var created *vault.Item
			for _, it := range s.Items {
				if it.Name() == "New entry" {
					created = it
					break
				}
			}
			if created == nil || created.FolderID() != tc.want {
				t.Fatalf("new item folder: got %v, want %q", created, tc.want)
			}
			changes := s.Stage.Changes()
			if len(changes) != 1 || changes[0].After["folderId"] != created.Raw["folderId"] {
				t.Fatal("staged creation differs from the chosen folder")
			}
		})
	}
}

// TestNewItemDialogUsesActiveFolder covers the keyboard creation path.
func TestNewItemDialogUsesActiveFolder(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.Folders = []map[string]any{{"id": "folder-1", "name": "Work"}}
	s.FilterFolder = 1
	s.Mode = DialogName
	s.Input = "New login"
	s.FormFocus = 0
	a := New(s)
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range s.Items {
		if it.Name() == "New login" {
			found = it.FolderID() == "folder-1"
		}
	}
	if !found || s.Mode != Browse {
		t.Fatal("new-item dialog did not assign applied folder")
	}
}

// TestDeleteRepromptExplainsPurpose verifies the password prompt action label.
func TestDeleteRepromptExplainsPurpose(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.Mode = DialogReprompt
	s.RepromptTargetID = s.Items[0].ID()
	s.RepromptDeleteID = s.Items[0].ID()
	a := New(s)
	canvas := NewCanvas(100, 40)
	a.createRender(canvas)
	if !strings.Contains(canvas.Render(), "delete this item") {
		t.Fatal("delete re-prompt must describe deletion instead of opening")
	}
}
