package ui

import (
	"fmt"
	"testing"

	"github.com/awesome-gocui/gocui"
)

// TestWordKeyBindingsConnectToActiveEditors exercises actual registered handlers.
func TestWordKeyBindingsConnectToActiveEditors(t *testing.T) {
	for _, mode := range []Mode{Find, Edit, DialogName} {
		for _, tc := range []struct {
			key  interface{}
			mod  gocui.Modifier
			want string
			pos  int
		}{
			{gocui.KeyArrowLeft, gocui.ModMouseCtrl, "first second", 0},
			{gocui.KeyArrowRight, gocui.ModMouseCtrl, "first second", 12},
			{gocui.KeyBackspace2, gocui.ModMouseCtrl, "second", 0},
			{gocui.KeyDelete, gocui.ModMouseCtrl, "first ", 6},
			{gocui.KeyArrowLeft, gocui.ModAlt, "first second", 0},
			{gocui.KeyArrowRight, gocui.ModAlt, "first second", 12},
			{gocui.KeyBackspace2, gocui.ModAlt, "second", 0},
			{gocui.KeyDelete, gocui.ModAlt, "first ", 6},
			{'b', gocui.ModAlt, "first second", 0},
			{'f', gocui.ModAlt, "first second", 12},
			{'d', gocui.ModAlt, "first ", 6},
		} {
			s := setup(t)
			a := New(s)
			s.Mode, s.Input, s.Caret = mode, "first second", 6
			if mode == Find {
				s.Search = s.Input
			}
			handlers := map[string]func(*gocui.Gui, *gocui.View) error{}
			err := a.bindWordKeys(func(
				key interface{}, mod gocui.Modifier,
				handler func(*gocui.Gui, *gocui.View) error,
			) error {
				handlers[fmt.Sprintf("%v-%v", key, mod)] = handler
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(handlers) != len(wordKeyBindings()) {
				t.Fatalf("registered %d handlers; expected %d", len(handlers),
					len(wordKeyBindings()))
			}
			k := fmt.Sprintf("%v-%v", tc.key, tc.mod)
			handler := handlers[k]
			if handler == nil {
				t.Fatalf("missing binding %s", k)
			}
			if err := handler(nil, nil); err != nil {
				t.Fatal(err)
			}
			if s.Input != tc.want || s.Caret != tc.pos {
				t.Fatalf("%v %s: input=%q caret=%d; want %q caret=%d",
					mode, k, s.Input, s.Caret, tc.want, tc.pos)
			}
			if mode == Find && s.Search != tc.want {
				t.Fatalf("find was not recomputed: %q", s.Search)
			}
		}
	}
}

// TestPasswordWordBindings checks the same event path while unlocking the vault.
func TestPasswordWordBindings(t *testing.T) {
	for _, tc := range []struct {
		key  interface{}
		mod  gocui.Modifier
		want string
		pos  int
	}{
		{gocui.KeyArrowLeft, gocui.ModMouseCtrl, "first second", 0},
		{gocui.KeyArrowRight, gocui.ModMouseCtrl, "first second", 12},
		{gocui.KeyBackspace2, gocui.ModMouseCtrl, "second", 0},
		{gocui.KeyDelete, gocui.ModMouseCtrl, "first ", 6},
		{'b', gocui.ModAlt, "first second", 0},
		{'f', gocui.ModAlt, "first second", 12},
		{'d', gocui.ModAlt, "first ", 6},
	} {
		s := setup(t)
		a := New(s)
		s.Screen, s.AuthFocus, s.Master, s.Caret = AuthMaster, 0,
			"first second", 6
		var handler func(*gocui.Gui, *gocui.View) error
		err := a.bindWordKeys(func(
			key interface{}, mod gocui.Modifier,
			fn func(*gocui.Gui, *gocui.View) error,
		) error {
			if key == tc.key && mod == tc.mod {
				handler = fn
			}
			return nil
		})
		if err != nil || handler == nil {
			t.Fatalf("binding failed: %v", err)
		}
		if err := handler(nil, nil); err != nil {
			t.Fatal(err)
		}
		if s.Master != tc.want || s.Caret != tc.pos {
			t.Fatalf("key %v: got %q at %d, want %q at %d", tc.key,
				s.Master, s.Caret, tc.want, tc.pos)
		}
	}
}

// TestWordEventsDoNotEditItemsInBrowse protects the navigation panel.
func TestWordEventsDoNotEditItemsInBrowse(t *testing.T) {
	s := setup(t)
	a := New(s)
	s.Mode, s.Input, s.Caret = Browse, "original", 3
	err := a.bindWordKeys(func(
		key interface{}, mod gocui.Modifier,
		fn func(*gocui.Gui, *gocui.View) error,
	) error {
		if key == gocui.KeyDelete && mod == gocui.ModMouseCtrl {
			if err := fn(nil, nil); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	})
	if err != nil || s.Input != "original" || s.Caret != 3 {
		t.Fatalf("Browse mutated: %v, %q, %d", err, s.Input, s.Caret)
	}
}
