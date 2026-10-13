package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestHintPaginationOmitsUnnecessaryNavigation verifies single-page footers.
func TestHintPaginationOmitsUnnecessaryNavigation(t *testing.T) {
	pages := PaginateHints([]string{"enter select", "n new", "d delete"}, 80)
	if len(pages) != 1 || strings.Contains(strings.Join(pages[0], " "), "? ...") {
		t.Fatalf("unnecessary navigation marker: %#v", pages)
	}
	pages = PaginateHints([]string{"enter select", "n new", "d delete", "esc close"}, 28)
	if len(pages) < 2 || pages[0][len(pages[0])-1] != "? ..." {
		t.Fatalf("missing navigation marker: %#v", pages)
	}
	for index, page := range pages {
		if index > 0 && page[0] != "? ..." {
			t.Fatalf("missing continuation marker on page %d", index)
		}
	}
}

// TestFolderCreationDeletionStagesEdits verifies both operations are staged.
func TestFolderCreationDeletionStagesEdits(t *testing.T) {
	s := setup(t)
	s.CreateFolder("Temporary folder")
	if s.Stage.Len() != 1 || s.Stage.Changes()[0].Kind != vault.New {
		t.Fatalf("folder creation was not staged: %#v", s.Stage.Changes())
	}
	for index, folder := range s.Folders {
		if folder["name"] == "Temporary folder" {
			s.Filters[2] = index + 1
			break
		}
	}
	s.DeleteFolder()
	if s.Stage.Len() != 0 {
		t.Fatalf("deleted unsaved folder must cancel the new edit: %#v", s.Stage.Changes())
	}
}

// TestSyncWithoutEditsUsesSingleChoice ensures soft and hard modes disappear.
func TestSyncWithoutEditsUsesSingleChoice(t *testing.T) {
	s, review := remoteReviewFixture(t)
	defer s.CLI.Cipher.Close()
	for _, change := range s.Stage.Changes() {
		s.Restore(change)
	}
	a := New(s)
	a.showRemoteReview(review)
	hints := strings.Join(a.hints(), " ")
	if !strings.Contains(hints, "s sync") || strings.Contains(hints, "h hard sync") {
		t.Fatalf("wrong no-edits sync shortcuts: %s", hints)
	}
	canvas := NewCanvas(120, 36)
	a.popupRender(canvas)
	shown := canvas.Render()
	if !strings.Contains(shown, "SYNC") || strings.Contains(shown, "SOFT SYNC") ||
		strings.Contains(shown, "HARD SYNC") {
		t.Fatalf("wrong no-edits sync actions")
	}
}
