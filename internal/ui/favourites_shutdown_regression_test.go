package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestFavouriteToggleStagesOnlyItsOwnField checks toggling, undo and read-only mode.
func TestFavouriteToggleStagesOnlyItsOwnField(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	item := s.SelectedItem()
	if item == nil || item.Favorite() {
		t.Fatal("expected a non-favourite selected item")
	}
	if !s.ToggleFavourite() || !item.Favorite() || s.FavouriteKind(item) != vault.Modified {
		t.Fatal("favourite must stage as a changed item field")
	}
	if s.Stage.Len() != 1 || s.FieldStatus(s.Fields[0]) != "" {
		t.Fatal("favourite must not flag an unrelated field")
	}
	if !s.ToggleFavourite() || item.Favorite() || s.Stage.Len() != 0 {
		t.Fatal("a second toggle must undo the original change")
	}
	s.LocalMode = true
	if s.ToggleFavourite() || item.Favorite() || s.Stage.Len() != 0 {
		t.Fatal("local mode must stay read-only")
	}
}

// TestNewItemFavouriteMarker checks that creation remains an N-stage change.
func TestNewItemFavouriteMarker(t *testing.T) {
	s := setup(t)
	s.CreateItem(vault.Login, "new favourite")
	for _, item := range s.Items {
		if item.Name() != "new favourite" {
			continue
		}
		s.SelectedID = item.ID()
		s.Focus = PaneContent
		if s.FavouriteKind(item) != vault.New {
			t.Fatal("new items must show N beside the favourite star")
		}
		return
	}
	t.Fatal("new item not created")
}

// TestFavouriteRenderingAndHints verifies the two independent visual indicators.
func TestFavouriteRenderingAndHints(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	a := New(s)
	c := NewCanvas(100, 30)
	c.Theme(s.Config.Colors)
	a.contentPanel(c, 40, 1, 58, 20)
	star := c.Cells[1*c.W+40+58-3]
	if star.R != '\uf005' || !star.Dim {
		t.Fatalf("unselected favourite should be dim white: %+v", star)
	}
	if err := a.char('f'); err != nil {
		t.Fatal(err)
	}
	c = NewCanvas(100, 30)
	a.contentPanel(c, 40, 1, 58, 20)
	star = c.Cells[1*c.W+40+58-3]
	if star.R != '\uf005' || star.FG != CBYellow {
		t.Fatalf("favourite star should be bright yellow: %+v", star)
	}
	indicator := c.Cells[1*c.W+40+58-9]
	if indicator.R != 'M' || indicator.FG != CYellow {
		t.Fatalf("modified favourite status should be marked M: %+v", indicator)
	}
	if !strings.Contains(strings.Join(a.hints(), " | "), "f favourite") {
		t.Fatal("content must advertise the favourite shortcut")
	}
	s.LocalMode = true
	if strings.Contains(strings.Join(a.hints(), " | "), "f favourite") {
		t.Fatal("read-only mode must not advertise the favourite shortcut")
	}
}

// TestShutdownProgressColors checks the waiting popup's phase colors.
func TestShutdownProgressColors(t *testing.T) {
	s := setup(t)
	cases := []struct {
		busy, backup, closing bool
		color                 int
	}{
		{true, false, false, CGreen},
		{false, true, false, CBlue},
		{false, false, true, CRed},
	}
	for _, tc := range cases {
		s.Busy, s.BackupBusy = tc.busy, tc.backup
		message, color := shutdownProgress(s, tc.closing)
		if message == "" || color != tc.color {
			t.Fatalf("shutdown progress: %q, %d; want %d", message, color, tc.color)
		}
	}
}

// TestItemListRendersBrightFavouriteStar checks placement after the item name.
func TestItemListRendersBrightFavouriteStar(t *testing.T) {
	s := setup(t)
	item := s.CurrentItem()
	if item == nil {
		t.Fatal("no item in list")
	}
	item.Raw["favorite"] = true
	a := New(s)
	c := NewCanvas(110, 32)
	a.itemsPanel(c, 35, 1, 74, 26)
	x := 35 + 9 + len([]rune(item.Name()))
	y := 1 + 2 + 1
	star := c.Cells[y*c.W+x]
	if star.R != '\uf005' || star.FG != CBYellow {
		t.Fatalf("favourite icon must follow item name in bright yellow: %+v", star)
	}
}
