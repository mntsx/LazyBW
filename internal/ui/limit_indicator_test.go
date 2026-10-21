package ui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func assertOverlongMarker(t *testing.T, canvas *Canvas, name string) {
	t.Helper()
	needle := name + " !"
	for y := 0; y < canvas.H; y++ {
		row := stringCellRow(canvas, y)
		index := strings.Index(row, needle)
		if index < 0 {
			continue
		}
		x := utf8.RuneCountInString(row[:index]) + utf8.RuneCountInString(name) + 1
		if cell := canvasAt(canvas, x, y); cell.FG != CRed || cell.R != '!' {
			t.Fatalf("overlong marker for %q: %+v", name, cell)
		}
		return
	}
	t.Fatalf("missing overlong marker %q", needle)
}

func TestOverlongFieldMarkersAppearInContentItemsAndEdits(t *testing.T) {
	s := setup(t)
	item := s.CurrentItem()
	item.Raw["login"].(map[string]any)["username"] = strings.Repeat("a", 688)
	item.Raw["favorite"] = true
	a := New(s)

	s.Focus = PaneItems
	items := NewCanvas(100, 20)
	a.itemsPanel(items, 0, 0, 100, 20)
	assertOverlongMarker(t, items, "Example")
	favouriteOrder := "Example ! " + s.Config.FavouriteIcon("items")
	foundFavouriteOrder := false
	for y := 0; y < items.H; y++ {
		if strings.Contains(stringCellRow(items, y), favouriteOrder) {
			foundFavouriteOrder = true
			break
		}
	}
	if !foundFavouriteOrder {
		t.Fatalf("warning and favourite icon order missing: %q", favouriteOrder)
	}

	s.OpenItem()
	s.Focus = PaneContent
	contents := NewCanvas(100, 20)
	a.contentPanel(contents, 0, 0, 100, 20)
	assertOverlongMarker(t, contents, "Username")

	s.Stage.Record("item", item.Raw, "Login", s.Originals)
	s.Focus = PaneChanges
	edits := NewCanvas(100, 12)
	a.sidePanel(edits, "Edits", a.icon("layout", "edits"), PaneChanges, 0, 0, 100, 12)
	assertOverlongMarker(t, edits, "Example")
}
