package ui

import "testing"

func TestClickItemsTitleBarFocusesPanelWithoutOpeningItem(t *testing.T) {
	for _, tc := range []struct {
		name string
		x    func(*App) int
	}{
		{"panel number", func(a *App) int { return a.itemX + 2 }},
		{"Name", func(a *App) int { return a.itemX + 7 }},
		{"Owner", func(a *App) int { return a.itemX + a.itemW - 27 }},
		{"State", func(a *App) int { return a.itemX + a.itemW - 16 }},
		{"right edge", func(a *App) int { return a.itemX + a.itemW - 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setup(t)
			s.Focus = PaneVault
			a := New(s)
			a.mainRender(NewCanvas(120, 44))
			initialIndex := s.ItemIndex
			if err := a.clickAt(tc.x(a), 1); err != nil {
				t.Fatal(err)
			}
			if s.Focus != PaneItems {
				t.Fatalf("clicked %s, focus = %d; want Items", tc.name, s.Focus)
			}
			if s.ItemIndex != initialIndex || s.SelectedID != "" {
				t.Fatal("clicking title bar changed or opened an item")
			}
		})
	}
}

func TestClickOutsideItemsTitleBarDoesNotActivateItems(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y func(*App) int
	}{
		{"left border", func(a *App) int { return a.itemX - 1 }, func(*App) int { return 1 }},
		{"right border", func(a *App) int { return a.itemX + a.itemW }, func(*App) int { return 1 }},
		{"separator", func(a *App) int { return a.itemX + 7 }, func(*App) int { return 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setup(t)
			s.Focus = PaneVault
			a := New(s)
			a.mainRender(NewCanvas(120, 44))
			if err := a.clickAt(tc.x(a), tc.y(a)); err != nil {
				t.Fatal(err)
			}
			if s.Focus != PaneVault {
				t.Fatalf("clicking %s activated Items", tc.name)
			}
		})
	}
}

func TestClickItemsTitleBarFromContentFocusesItems(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	if s.Focus != PaneContent || s.SelectedID == "" {
		t.Fatal("item did not open")
	}
	a := New(s)
	a.mainRender(NewCanvas(120, 44))
	initialID := s.SelectedID
	if err := a.clickAt(a.itemX+7, 1); err != nil {
		t.Fatal(err)
	}
	if s.Focus != PaneItems || s.SelectedID != initialID {
		t.Fatal("click must focus Items without closing or replacing the current item")
	}
}
