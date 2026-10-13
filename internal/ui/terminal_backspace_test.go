package ui

import (
	"testing"

	"github.com/awesome-gocui/gocui"
)

// TestTerminalWordBackspaceKey protects regular Backspace on each terminal.
func TestTerminalWordBackspaceKey(t *testing.T) {
	cases := []struct {
		regular byte
		key     gocui.Key
		word    bool
	}{
		{'\x7f', gocui.KeyBackspace, true},
		{'\x7f', gocui.KeyBackspace2, false},
		{'\x08', gocui.KeyBackspace, false},
		{'\x08', gocui.KeyBackspace2, true},
		{0, gocui.KeyBackspace, false},
		{0, gocui.KeyBackspace2, false},
	}
	for _, tc := range cases {
		if got := terminalWordBackspaceKey(tc.key, tc.regular); got != tc.word {
			t.Errorf("regular=%#x key=%v: got %v, want %v",
				tc.regular, tc.key, got, tc.word)
		}
	}
}

// TestLegacyBackspaceSeparatesTextAndNavigation checks word deletion in Find.
func TestLegacyBackspaceSeparatesTextAndNavigation(t *testing.T) {
	s := setup(t)
	a := New(s)
	a.terminalBS = '\x7f'
	s.Mode, s.Input, s.Search, s.Caret = Find, "first second", "first second", 6
	if err := a.terminalBackspace(gocui.KeyBackspace); err != nil {
		t.Fatal(err)
	}
	if s.Input != "second" || s.Caret != 0 || s.Search != "second" {
		t.Fatalf("ctrl+backspace: input=%q cursor=%d search=%q",
			s.Input, s.Caret, s.Search)
	}
	s.Input, s.Search, s.Caret = "first second", "first second", 6
	if err := a.terminalBackspace(gocui.KeyBackspace2); err != nil {
		t.Fatal(err)
	}
	if s.Input != "firstsecond" || s.Caret != 5 {
		t.Fatalf("ordinary backspace: input=%q cursor=%d", s.Input, s.Caret)
	}
}
