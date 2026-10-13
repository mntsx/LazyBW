package ui

import (
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestContentAndDetailsUseTheSameRevealState checks both entry directions.
func TestContentAndDetailsUseTheSameRevealState(t *testing.T) {
	for _, initiallyRevealed := range []bool{false, true} {
		t.Run(map[bool]string{false: "hidden", true: "revealed"}[initiallyRevealed],
			func(t *testing.T) {
				s := setup(t)
				defer s.CLI.Cipher.Close()
				s.Screen = Workspace
				s.OpenItem()
				s.FieldIndex = 1 // Login password.
				a := New(s)
				if initiallyRevealed {
					s.Reveal() // r reveal in Content.
				}
				key := s.RevealKey()
				if err := s.BeginEdit(); err != nil {
					t.Fatal(err)
				}
				if s.AuthVisible != initiallyRevealed {
					t.Fatal("Details did not inherit the password visibility")
				}
				if err := a.toggleAuthVisibility(); err != nil {
					t.Fatal(err)
				}
				if s.AuthVisible != !initiallyRevealed ||
					s.Revealed[key] != !initiallyRevealed {
					t.Fatal("Details reveal did not update Content visibility")
				}
				s.ExitEdit()
				if s.Revealed[key] != !initiallyRevealed {
					t.Fatal("leaving Details lost the shared visibility")
				}
				s.Reveal() // r reveal in Content.
				if s.Revealed[key] != initiallyRevealed {
					t.Fatal("Content reveal did not toggle the same field")
				}
				if err := s.BeginEdit(); err != nil {
					t.Fatal(err)
				}
				if s.AuthVisible != initiallyRevealed {
					t.Fatal("reopening Details failed to inherit Content visibility")
				}
			})
	}
}

// TestFolderCursorDefaultsToNoFolder confirms a highlight without filtering.
func TestFolderCursorDefaultsToNoFolder(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	if s.Filters[2] != 0 || s.FilterFolder != -1 {
		t.Fatalf("incorrect initial folder state: %d, %d", s.Filters[2],
			s.FilterFolder)
	}
	folder := map[string]any{"id": "folder-a", "name": "A", "object": "folder"}
	s.Folders = []map[string]any{folder}
	s.FolderOriginals = map[string]map[string]any{"folder-a": vault.Clone(folder)}
	s.Filters[2] = 1
	s.DeleteFolder()
	if s.Filters[2] != 0 || s.FilterFolder != -1 {
		t.Fatalf("deleting folder must highlight No Folder but not filter: %d, %d",
			s.Filters[2], s.FilterFolder)
	}
}

// TestBlankSidebarClickPreservesAppliedFilter tests empty-pane hitboxes.
func TestBlankSidebarClickPreservesAppliedFilter(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pane     Pane
		prepare  func(*State)
		selected func(*State) int
		applied  func(*State) int
	}{
		{"vault", PaneVault, func(s *State) { s.Filters[0] = 0 },
			func(s *State) int { return s.Filters[0] },
			func(s *State) int { return s.FilterVault }},
		{"types", PaneTypes, func(s *State) { s.FilterType = 2; s.Filters[1] = 5 },
			func(s *State) int { return s.Filters[1] },
			func(s *State) int { return s.FilterType }},
		{"folders-applied", PaneFolders, func(s *State) {
			s.Folders = []map[string]any{{"id": "a", "name": "A"}}
			s.FilterFolder = 1
			s.Filters[2] = 0
		}, func(s *State) int { return s.Filters[2] },
			func(s *State) int { return s.FilterFolder }},
		{"folders-unapplied", PaneFolders, func(s *State) {
			s.Folders = []map[string]any{{"id": "a", "name": "A"}}
			s.FilterFolder = -1
			s.Filters[2] = 1
		}, func(s *State) int { return s.Filters[2] },
			func(s *State) int { return -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setup(t)
			defer s.CLI.Cipher.Close()
			s.Screen = Workspace
			s.Focus = PaneItems
			tc.prepare(s)
			a := New(s)
			a.mainRender(NewCanvas(120, 60))
			index := int(tc.pane) - 1
			x := 6
			y := a.sideY[index] + a.sideH[index] - 2
			if err := a.clickAt(x, y); err != nil {
				t.Fatal(err)
			}
			if s.Focus != tc.pane {
				t.Fatalf("blank sidebar click did not focus %d", tc.pane)
			}
			expected := tc.applied(s)
			if expected < 0 {
				expected = 0
			}
			if tc.selected(s) != expected {
				t.Fatalf("selected %d, expected %d", tc.selected(s), expected)
			}
		})
	}
}

// TestRepromptStartsMaskedAfterDetailReveal keeps master input independent.
func TestRepromptStartsMaskedAfterDetailReveal(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.Screen = Workspace
	s.OpenItem()
	s.FieldIndex = 1
	if err := s.BeginEdit(); err != nil {
		t.Fatal(err)
	}
	a := New(s)
	if err := a.toggleAuthVisibility(); err != nil {
		t.Fatal(err)
	}
	if !s.AuthVisible {
		t.Fatal("password detail did not reveal")
	}
	s.ExitEdit()
	if s.AuthVisible {
		t.Fatal("secret editor visibility carried past editor exit")
	}
	a.startReprompt(s.SelectedID)
	if s.AuthVisible {
		t.Fatal("master password reprompt must start masked")
	}
}
