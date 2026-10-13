package ui

import (
	"fmt"
	"testing"

	"github.com/awesome-gocui/gocui"
	"github.com/gdamore/tcell/v2"
	"github.com/miguelmnts/lazybw-go/internal/terminalkeys"
)

// TestTerminalCtrlWordBindings checks decoded keys in every text editor.
func TestTerminalCtrlWordBindings(t *testing.T) {
	cases := []struct {
		key  tcell.Key
		mode Mode
		want string
		pos  int
	}{
		{tcell.KeyF52, Find, "first second", 0},
		{tcell.KeyF53, Find, "first second", 12},
		{tcell.KeyF54, Find, "first ", 6},
		{tcell.KeyF52, Edit, "first second", 0},
		{tcell.KeyF53, Edit, "first second", 12},
		{tcell.KeyF54, Edit, "first ", 6},
		{tcell.KeyF52, DialogName, "first second", 0},
		{tcell.KeyF53, DialogName, "first second", 12},
		{tcell.KeyF54, DialogName, "first ", 6},
	}
	for _, tc := range cases {
		s := setup(t)
		a := New(s)
		a.nativeWordKeys = []terminalkeys.Binding{
			{Key: tcell.KeyF52, Action: terminalkeys.PreviousWord, Label: "ctrl+←"},
			{Key: tcell.KeyF53, Action: terminalkeys.NextWord, Label: "ctrl+→"},
			{Key: tcell.KeyF54, Action: terminalkeys.DeleteNextWord, Label: "ctrl+delete"},
		}
		s.Mode, s.Input, s.Caret = tc.mode, "first second", 6
		if tc.mode == Find {
			s.Search = s.Input
		}
		callbacks := map[string]func(*gocui.Gui, *gocui.View) error{}
		err := a.bindWordKeys(func(
			key interface{}, mod gocui.Modifier,
			fn func(*gocui.Gui, *gocui.View) error,
		) error {
			callbacks[fmt.Sprintf("%v-%v", key, mod)] = fn
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		fn := callbacks[fmt.Sprintf("%v-%v", gocui.Key(tc.key), gocui.ModNone)]
		if fn == nil {
			t.Fatalf("missing decoded key binding %v", tc.key)
		}
		if err := fn(nil, nil); err != nil {
			t.Fatal(err)
		}
		if s.Input != tc.want || s.Caret != tc.pos {
			t.Errorf("%v/%v: got %q caret=%d; want %q caret=%d",
				tc.mode, tc.key, s.Input, s.Caret, tc.want, tc.pos)
		}
	}
}

// TestTerminalCtrlWordKeysIgnoredWhileBrowsing protects ordinary navigation.
func TestTerminalCtrlWordKeysIgnoredWhileBrowsing(t *testing.T) {
	s := setup(t)
	a := New(s)
	a.nativeWordKeys = []terminalkeys.Binding{
		{Key: tcell.KeyF54, Action: terminalkeys.DeleteNextWord},
	}
	s.Mode, s.Input, s.Caret = Browse, "unchanged", 4
	if err := a.bindWordKeys(func(
		key interface{}, mod gocui.Modifier,
		fn func(*gocui.Gui, *gocui.View) error,
	) error {
		if key == gocui.Key(tcell.KeyF54) && mod == gocui.ModNone {
			return fn(nil, nil)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if s.Input != "unchanged" || s.Caret != 4 {
		t.Fatal("Ctrl+Delete modified text while browsing")
	}
}
