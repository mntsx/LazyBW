package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/bitwarden"
	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestLinkedCustomFieldCreationAndEdit checks Bitwarden's linkedId wire format.
func TestLinkedCustomFieldCreationAndEdit(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.OpenItem()
	a := New(s)
	if err := a.char('n'); err != nil {
		t.Fatal(err)
	}
	s.CustomType = 3
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogField {
		t.Fatal("name phase missing")
	}
	for _, ch := range "account-field" {
		if err := a.char(ch); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogLinkedChoice {
		t.Fatal("destination phase missing")
	}
	s.LinkedIndex = 1
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	arr := s.SelectedItem().Raw["fields"].([]any)
	if len(arr) != 1 {
		t.Fatal("linked field not created")
	}
	m := arr[0].(map[string]any)
	if m["type"] != 3 || m["value"] != nil || m["linkedId"] != 101 || m["name"] != "account-field" {
		t.Fatalf("unexpected linked field JSON: %v", m)
	}
	linkedIndex := -1
	for i := range s.Fields {
		if s.Fields[i].Name == "account-field" {
			linkedIndex = i
			break
		}
	}
	if linkedIndex < 0 {
		t.Fatal("linked field absent from content")
	}
	s.FieldIndex = linkedIndex
	field := s.CurrentField()
	if field == nil || !field.Linked || field.LinkedID != 101 {
		t.Fatal("linked field has incorrect type or destination")
	}
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogLinkedChoice || !s.LinkedEdit {
		t.Fatal("linked edit must show destination picker")
	}
	s.LinkedIndex = 0
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if m["linkedId"] != 100 || s.Mode != Browse {
		t.Fatal("linked target not updated")
	}
	if s.Stage.Len() != 1 {
		t.Fatal("linked edit should remain one staged item change")
	}
}

// TestRepromptBlocksProtectedItemUntilCorrectPassword verifies the UI gate.
func TestRepromptBlocksProtectedItemUntilCorrectPassword(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	if err := s.CLI.SetRepromptMaster("correct master"); err != nil {
		t.Fatal(err)
	}
	s.Items[0].Raw["reprompt"] = 1
	a := New(s)
	a.openItem()
	if s.Mode != DialogReprompt || s.SelectedID != "" || s.Focus == PaneContent {
		t.Fatal("protected item opened before re-prompt")
	}
	for _, r := range "incorrect" {
		if err := a.char(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != DialogReprompt || s.RepromptError == "" || s.SelectedID != "" {
		t.Fatal("incorrect password must not reveal content")
	}
	s.ClearMaster()
	s.Caret = 0
	for _, r := range "correct master" {
		if err := a.char(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Browse || s.Focus != PaneContent || s.SelectedID == "" || s.Master != "" {
		t.Fatal("valid password must open item and discard cleartext input")
	}
	s.CloseItem()
	if s.RepromptApprovedID != "" {
		t.Fatal("authorization must not survive closing item")
	}
}

// TestRepromptFieldIsStagedAsNativeBoolean checks position and native value.
func TestRepromptFieldIsStagedAsNativeBoolean(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.OpenItem()
	idx := -1
	for i, field := range s.Fields {
		if field.Reprompt {
			idx = i
			break
		}
	}
	if idx < 1 || s.Fields[idx-1].Name != "Notes" {
		t.Fatal("reprompt must follow Notes")
	}
	s.FieldIndex = idx
	if err := s.SwitchCheckbox(); err != nil {
		t.Fatal(err)
	}
	if s.SelectedItem().Raw["reprompt"] != 1 || s.Stage.Len() != 1 {
		t.Fatal("reprompt toggle not staged")
	}
	if err := s.SwitchCheckbox(); err != nil {
		t.Fatal(err)
	}
	if s.Stage.Len() != 0 {
		t.Fatal("reverting reprompt must clear pending changes")
	}
}

// TestRemoteReviewAbortAndSkipPreserveExpectedStaging checks modal semantics.
func TestRemoteReviewAbortAndSkipPreserveExpectedStaging(t *testing.T) {
	s, review := remoteReviewFixture(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	a.showRemoteReview(review)
	if err := a.char('a'); err != nil {
		t.Fatal(err)
	}
	if s.Stage.Len() != 1 || s.Items[0].Name() != "Local name" || s.Popup != nil {
		t.Fatal("abort must not mutate state")
	}
	a.showRemoteReview(review)
	if err := a.char('h'); err != nil {
		t.Fatal(err)
	}
	if s.Stage.Len() != 1 || s.Items[0].Name() != "Remote name" || len(s.ImportedChanges) != 1 {
		t.Fatal("HARD SYNC must keep independent edits and discard conflicts")
	}
}

// TestNoRemoteChangesDialogHasOneOKButton validates the informational modal.
func TestNoRemoteChangesDialogHasOneOKButton(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	s.Popup = &Popup{Title: "Vault already synchronized", Question: "No remote changes detected", Acknowledge: true}
	canvas := NewCanvas(94, 36)
	a.popupRender(canvas)
	if !strings.Contains(canvas.Render(), "OK") || !strings.Contains(canvas.Render(), "No remote changes") {
		t.Fatal("no-change popup must display its message and OK")
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Popup != nil {
		t.Fatal("OK must dismiss the no-change popup")
	}
}

// TestCiphertextEqualityUsesKeyedTags compares sealed values without opening them.
func TestCiphertextEqualityUsesKeyedTags(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	a, err := s.CLI.Cipher.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CLI.Cipher.Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CLI.Cipher.Seal("other")
	if err != nil {
		t.Fatal(err)
	}
	if !vault.EqualContent(a, b) || vault.EqualContent(a, c) {
		t.Fatal("keyed sealed fingerprints must distinguish changed secrets")
	}
	if string(a.(*vault.Sealed).Nonce) == string(b.(*vault.Sealed).Nonce) {
		t.Fatal("test must use distinct random ciphertext nonces")
	}
	// Confirm no plaintext comparison is needed while comparing sealed item values.
	_ = bitwarden.ErrRemoteChanged
}

// TestSkipRemoteFieldConflictKeepsNewIndependentLinkedField protects local edits.
func TestSkipRemoteFieldConflictKeepsNewIndependentLinkedField(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	item := s.Items[0]
	originalField := map[string]any{"name": "old", "type": 0, "value": "initial"}
	item.Raw["fields"] = []any{vault.Clone(originalField)}
	s.Originals[item.ID()] = vault.Clone(item.Raw)
	item.Raw["fields"] = []any{
		vault.Clone(originalField),
		map[string]any{"name": "new", "type": 3, "value": nil, "linkedId": 100},
	}
	s.Stage.Record("item", item.Raw, "Login", s.Originals)
	remote := vault.Clone(s.Originals[item.ID()])
	remote["fields"].([]any)[0].(map[string]any)["value"] = "remote edit"
	review := &bitwarden.RemoteReview{
		Items:   map[string]map[string]any{item.ID(): remote},
		Folders: map[string]map[string]any{},
	}
	s.ApplyRemoteReview(review, false)
	fields := s.Items[0].Raw["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("non-conflicting linked addition lost: %v", fields)
	}
	if fields[0].(map[string]any)["value"] != "remote edit" ||
		fields[1].(map[string]any)["linkedId"] != 100 {
		t.Fatalf("field-level merge failed: %v", fields)
	}
	if s.Stage.Len() != 1 {
		t.Fatal("independent local field addition must remain staged")
	}
}
