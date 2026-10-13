package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// addTestWebsites populates a login with realistic Bitwarden URI entries.
func addTestWebsites(s *State) {
	login := s.CurrentItem().Raw["login"].(map[string]any)
	login["uris"] = []any{
		map[string]any{"uri": "https://first.example", "match": float64(0)},
		map[string]any{"uri": "https://second.example", "match": float64(2)},
	}
}

// TestWebsitesAppearAsOneField verifies Bitwarden URI grouping and row sizing.
func TestWebsitesAppearAsOneField(t *testing.T) {
	s := setup(t)
	addTestWebsites(s)
	s.OpenItem()
	if len(s.Fields) != 7 {
		t.Fatalf("wrong field count: %d", len(s.Fields))
	}
	if last := s.Fields[len(s.Fields)-1]; !last.Folders || last.Name != "Folder" {
		t.Fatalf("Folders must be the final field: %#v", last)
	}
	f := s.Fields[3]
	if f.Name != "Websites" || !f.Websites || len(f.Values) != 2 {
		t.Fatalf("incorrect websites group: %#v", f)
	}
	if s.Fields[2].Name != "TOTP" || s.Fields[4].Name != "Notes" {
		t.Fatalf("wrong website position: %#v", s.Fields)
	}
	if fieldRowHeight(f) != 4 {
		t.Fatal("websites must span a label plus two URL rows")
	}
	a := New(s)
	s.FieldIndex = 3
	canvas := NewCanvas(150, 50)
	a.mainRender(canvas)
	output := canvas.Render()
	if !strings.Contains(output, "https://first.example") ||
		!strings.Contains(output, "https://second.example") {
		t.Fatal("the content panel did not display all website values")
	}
	if !strings.Contains(output, "Websites") {
		t.Fatal("the content panel omitted the group label")
	}
}

// TestExistingWebsitePickerAndEdit verifies editing one URI preserves metadata.
func TestExistingWebsitePickerAndEdit(t *testing.T) {
	s := setup(t)
	addTestWebsites(s)
	s.OpenItem()
	s.FieldIndex = 3
	a := New(s)
	if err := a.enter(); err != nil || s.Mode != DialogWebsite {
		t.Fatalf("did not open website picker: %v, mode=%v", err, s.Mode)
	}
	canvas := NewCanvas(100, 40)
	a.createRender(canvas)
	if !strings.Contains(canvas.Render(), "https://second.example") ||
		!strings.Contains(canvas.Render(), websiteFieldOption.Icon) {
		t.Fatal("picker lacks URL values and website icons")
	}
	if err := a.arrow(1, 0); err != nil {
		t.Fatal(err)
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Edit || s.Focus != PaneDetail || s.Input != "https://second.example" {
		t.Fatalf("wrong URI editor: mode=%v input=%s", s.Mode, s.Input)
	}
	s.Input = "https://updated.example"
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	uris := s.SelectedItem().Raw["login"].(map[string]any)["uris"].([]any)
	if len(uris) != 2 || uris[1].(map[string]any)["uri"] != s.Fields[3].Values[1] ||
		uris[1].(map[string]any)["match"] != float64(2) {
		t.Fatal("URI update lost match metadata or changed list size")
	}
	if s.Fields[3].Values[1] != "https://updated.example" {
		t.Fatal("edited URI did not reach the grouped field")
	}
	if s.Stage.Len() != 1 {
		t.Fatal("URI edit was not staged")
	}
	if len(s.SelectedItem().Raw["fields"].([]any)) != 0 {
		t.Fatal("website edit unexpectedly created a custom field")
	}
}

// TestWebsitePickerCancel verifies the picker never stages a cancelled change.
func TestWebsitePickerCancel(t *testing.T) {
	s := setup(t)
	addTestWebsites(s)
	s.OpenItem()
	s.FieldIndex = 3
	a := New(s)
	_ = s.BeginEdit()
	_ = a.arrow(1, 0)
	_ = a.arrow(1, 0)
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Browse || s.Stage.Len() != 0 {
		t.Fatal("cancelling website picker modified the vault")
	}
}

// TestNewWebsiteIsAURI verifies creating and discarding URI values.
func TestNewWebsiteIsAURI(t *testing.T) {
	a, s := newFieldDialogState(t)
	for index := 0; index < len(customFieldOptions); index++ {
		_ = a.arrow(1, 0)
	}
	if fieldOptions(s.SelectedItem())[s.CustomType].Name != "Website" {
		t.Fatal("login field picker lacks Website")
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != Edit || !s.EditNewWebsite || s.Stage.Len() != 0 {
		t.Fatal("new website must open the URL editor before staging")
	}
	_ = a.escape()
	if len(s.SelectedItem().Raw["login"].(map[string]any)["uris"].([]any)) != 0 {
		t.Fatal("cancelling a new website added an empty URI")
	}
	_ = a.char('n')
	for index := 0; index < len(customFieldOptions); index++ {
		_ = a.arrow(1, 0)
	}
	_ = a.enter()
	for _, ch := range "https://new.example" {
		_ = a.char(ch)
	}
	if err := a.enter(); err != nil {
		t.Fatal(err)
	}
	uris := s.SelectedItem().Raw["login"].(map[string]any)["uris"].([]any)
	if len(uris) != 1 || uris[0].(map[string]any)["uri"] != "https://new.example" {
		t.Fatalf("expected an actual Bitwarden URI entry, got %#v", uris)
	}
	if s.Stage.Len() != 1 || s.CurrentField().Name != "Websites" {
		t.Fatal("website creation failed to stage or select the grouped field")
	}
	if len(s.SelectedItem().Raw["fields"].([]any)) != 0 {
		t.Fatal("website creation inserted a custom text field")
	}
}

// TestWebsiteOptionOnlyInLogins ensures other item types cannot add login URIs.
func TestWebsiteOptionOnlyInLogins(t *testing.T) {
	card := vault.NewItem(vault.Card, "card", "card", nil)
	if len(fieldOptions(card)) != len(customFieldOptions)-1 {
		t.Fatal("non-login item offers login websites")
	}
}

// TestBoldNamesAndKeyFeedback verifies the footer and emphasized labels.
func TestBoldNamesAndKeyFeedback(t *testing.T) {
	s := setup(t)
	a := New(s)
	canvas := NewCanvas(120, 44)
	a.mainRender(canvas)
	if !canvasAt(canvas, a.itemX+8, 4).Bold {
		t.Fatal("item name must be bold")
	}
	s.OpenItem()
	canvas = NewCanvas(120, 44)
	a.mainRender(canvas)
	if !canvasAt(canvas, a.contentX+3, 4).Bold {
		t.Fatal("field name must be bold")
	}
	if got := a.keyAction("n"); got != "new" {
		t.Fatalf("wrong key action: %s", got)
	}
	if got := a.keyAction("z"); got != "unbound" {
		t.Fatalf("unmapped key must say unbound, got %s", got)
	}
	a.reportKey("z", "unbound")
	canvas = NewCanvas(120, 44)
	a.mainRender(canvas)
	footerY := canvas.H - 2
	if cell := canvasAt(canvas, 3, footerY); cell.FG != CRed {
		t.Fatalf("unbound action should be normal.red: %+v row=%q", cell,
			stringCellRow(canvas, footerY))
	}
	s.KeyFeedbackExpires = time.Now().Add(-time.Millisecond)
	canvas = NewCanvas(120, 44)
	a.mainRender(canvas)
	if !strings.Contains(stringCellRow(canvas, footerY), "panels") {
		t.Fatal("normal hints did not reappear after one second")
	}
}
