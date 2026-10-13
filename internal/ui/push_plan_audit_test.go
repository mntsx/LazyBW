package ui

import (
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestPushPlanOrdersNewFoldersBeforeItems prevents invalid local IDs remotely.
func TestPushPlanOrdersNewFoldersBeforeItems(t *testing.T) {
	staged := vault.NewStage()
	item := map[string]any{"id": "local-11", "name": "Entry", "folderId": "local-12"}
	folder := map[string]any{"id": "local-12", "name": "Folder"}
	staged.Record("item", item, "Login", nil)
	staged.Record("folder", folder, "Folder", nil)
	result, ok := planPush(staged, staged.Changes())
	if !ok || len(result) != 2 || result[0].Entity != "folder" || result[1].Entity != "item" {
		t.Fatalf("folder must be registered before dependent item: %#v", result)
	}
	if _, ok := planPush(staged, staged.Changes()[:1]); ok {
		t.Fatal("single item push must reject a not-yet-pushed folder")
	}
}

// TestPushPlanPreservesIndependentSelectedChanges checks single-entry push.
func TestPushPlanPreservesIndependentSelectedChanges(t *testing.T) {
	staged := vault.NewStage()
	staged.Record("item", map[string]any{"id": "local-41", "name": "Entry"}, "Login", nil)
	staged.Record("item", map[string]any{"id": "local-42", "name": "Other"}, "Login", nil)
	chosen := staged.Changes()[1:2]
	actual, ok := planPush(staged, chosen)
	if !ok || len(actual) != 1 || actual[0].Key != chosen[0].Key {
		t.Fatal("single independent item must keep its original selection")
	}
}
