package ui

import (
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestDeleteFolderStagesIndependentItemEdits checks detachment survives undo.
func TestDeleteFolderStagesIndependentItemEdits(t *testing.T) {
	s := setup(t)
	folder := map[string]any{"id": "folder-1", "name": "Work", "object": "folder"}
	s.Folders = []map[string]any{folder}
	s.FolderOriginals = map[string]map[string]any{"folder-1": vault.Clone(folder)}
	s.Items[0].Raw["folderId"] = "folder-1"
	s.Originals[s.Items[0].ID()] = vault.Clone(s.Items[0].Raw)
	s.Filters[2] = 1
	s.DeleteFolder()
	itemEdit := s.Stage.Get(vault.Key("item", s.Items[0].ID()))
	folderEdit := s.Stage.Get(vault.Key("folder", "folder-1"))
	if itemEdit == nil || itemEdit.Kind != vault.Modified {
		t.Fatalf("missing item detachment edit: %+v", itemEdit)
	}
	if folderEdit == nil || folderEdit.Kind != vault.Deleted {
		t.Fatalf("missing folder deletion edit: %+v", folderEdit)
	}
	if s.Items[0].FolderID() != "" {
		t.Fatalf("deleted folder still assigned: %s", s.Items[0].FolderID())
	}
	s.Restore(folderEdit)
	if s.Stage.Get(itemEdit.Key) == nil || s.Items[0].FolderID() != "" {
		t.Fatal("undoing folder deletion also discarded independent item edit")
	}
}
