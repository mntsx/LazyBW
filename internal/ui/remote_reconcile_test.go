package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// remoteReviewFixture stages changes to both a remote-modified and unchanged field.
func remoteReviewFixture(t *testing.T) (*State, *bitwarden.RemoteReview) {
	t.Helper()
	s := setup(t)
	it := s.Items[0]
	it.Raw["name"] = "Local name"
	it.Raw["favorite"] = true
	s.Stage.Record("item", it.Raw, "Login", s.Originals)
	remote := vault.Clone(s.Originals[it.ID()])
	remote["name"] = "Remote name"
	review := &bitwarden.RemoteReview{
		Items:   map[string]map[string]any{it.ID(): remote},
		Folders: map[string]map[string]any{},
		Differences: []bitwarden.RemoteDifference{{
			Entity: "item", ID: it.ID(), Name: "Remote name",
			TypeName: "Login", Kind: vault.Modified,
		}},
	}
	return s, review
}

// TestReconcileSkippingIncomingDiscardsOnlyOverlappingLocalFields checks merge rules.
func TestReconcileSkippingIncomingDiscardsOnlyOverlappingLocalFields(t *testing.T) {
	s, review := remoteReviewFixture(t)
	defer s.CLI.Cipher.Close()
	s.ApplyRemoteReview(review, false)
	item := s.Items[0]
	if item.Name() != "Remote name" || !item.Favorite() {
		t.Fatalf("remote conflict must win while independent favourite survives: %v", item.Raw)
	}
	if len(s.ImportedChanges) != 1 || s.Stage.Len() != 1 {
		t.Fatal("skip must not log incoming changes or erase unrelated staged edits")
	}
	change := s.Stage.Changes()[0]
	if change.After["name"] != "Remote name" || change.Before["name"] != "Remote name" {
		t.Fatal("confirmed baseline must be rebased to current server")
	}
}

// TestReconcileTrackingShowsImportedButNeverStagesIt protects against replaying writes.
func TestReconcileTrackingShowsImportedButNeverStagesIt(t *testing.T) {
	s, review := remoteReviewFixture(t)
	defer s.CLI.Cipher.Close()
	s.ApplyRemoteReview(review, true)
	if s.Items[0].Name() != "Local name" || s.Stage.Len() != 1 {
		t.Fatal("tracking should preserve overlapping staged local values")
	}
	if len(s.ImportedChanges) != 1 || len(s.ChangesList()) != 2 {
		t.Fatal("incoming change must be a distinct, informational entry")
	}
	if !s.ImportedChangeAt(1) || s.ImportedChangeAt(0) ||
		!strings.HasPrefix(s.ImportedChanges[0].Key, "import:") {
		t.Fatal("imported log must not be confused with a staged upload")
	}
	if s.Stage.Changes()[0].Key == s.ImportedChanges[0].Key {
		t.Fatal("imported entry must not become a remote write")
	}
	s.ClearImportedChange(1)
	if len(s.ImportedChanges) != 1 || s.Stage.Len() != 1 {
		t.Fatal("imported ground changes must remain visible for the session")
	}
}

// TestReconcileRemoteDeleteConservesIndependentNewItem checks structural changes.
func TestReconcileRemoteDeleteConservesIndependentNewItem(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.CreateItem(vault.Login, "Unrelated new login")
	originalID := "id-1"
	review := &bitwarden.RemoteReview{
		Items: map[string]map[string]any{}, Folders: map[string]map[string]any{},
		Differences: []bitwarden.RemoteDifference{{
			Entity: "item", ID: originalID, Name: "Example", Kind: vault.Deleted,
		}},
	}
	s.ApplyRemoteReview(review, false)
	if len(s.Items) != 1 || s.Items[0].Name() != "Unrelated new login" ||
		s.Stage.Len() != 1 {
		t.Fatal("remote deletion should remove original but retain unrelated local creation")
	}
}

// TestIncomingDiffPopupExplainsBothChoices checks layout and status markers.
func TestIncomingDiffPopupExplainsBothChoices(t *testing.T) {
	s, review := remoteReviewFixture(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	a.showRemoteReview(review)
	canvas := NewCanvas(100, 34)
	a.popupRender(canvas)
	rendered := canvas.Render()
	for _, word := range []string{"Remote changes detected", "SOFT SYNC", "HARD SYNC", "ABORT SYNC", "Remote name"} {
		if !strings.Contains(rendered, word) {
			t.Fatalf("popup must display %q", word)
		}
	}
	if a.S.Popup.RemoteChoice != 2 {
		t.Fatal("abort must be initially selected")
	}
	if err := a.char('s'); err != nil {
		t.Fatal(err)
	}
	if a.S.Popup != nil || s.Stage.Len() != 1 || len(s.ImportedChanges) != 1 {
		t.Fatal("SOFT SYNC must apply review and record an informational import")
	}
}

// TestRemoteReviewMouseOnlyClicksActualButtons prevents half-dialog hit areas.
func TestRemoteReviewMouseOnlyClicksActualButtons(t *testing.T) {
	s, review := remoteReviewFixture(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	a.showRemoteReview(review)
	a.popupRender(NewCanvas(100, 35))
	buttonY := a.dialogY + a.dialogH - 4
	if err := a.clickAt(a.dialogX+a.dialogW/2+12, buttonY); err != nil {
		t.Fatal(err)
	}
	if s.Popup == nil {
		t.Fatal("clicking empty gap must not confirm import")
	}
	if err := a.clickAt(a.dialogX+(a.dialogW-13)/2+2, buttonY); err != nil {
		t.Fatal(err)
	}
	if s.Popup != nil || len(s.ImportedChanges) != 1 {
		t.Fatal("HARD SYNC mouse button should apply without logging imports")
	}
}
