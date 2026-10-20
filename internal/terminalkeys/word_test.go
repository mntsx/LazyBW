package terminalkeys

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v2/terminfo"
)

// TestWordSequences checks distinct control sequences and preserves normal keys.
func TestWordSequences(t *testing.T) {
	info := &terminfo.Terminfo{
		KeyLeft:   "\x1bOD",
		KeyRight:  "\x1bOC",
		KeyDelete: "\x1b[3~",
		KeyF5:     "\x1b[15~",
	}
	original := *info
	bindings := registerWordSequences(info)
	if len(bindings) != 7 {
		t.Fatalf("got %d bindings, want 7", len(bindings))
	}
	want := []struct {
		key      tcell.Key
		action   Action
		sequence string
	}{
		{tcell.KeyF52, PreviousWord, "\x1b[1;5D"},
		{tcell.KeyF53, NextWord, "\x1b[1;5C"},
		{tcell.KeyF54, DeleteNextWord, "\x1b[3;5~"},
		{tcell.KeyF55, PreviousBlankLine, "\x1b[1;6A"},
		{tcell.KeyF56, NextBlankLine, "\x1b[1;6B"},
		{tcell.KeyF57, PreviousBlankLine, "\x1b[1;5A"},
		{tcell.KeyF58, NextBlankLine, "\x1b[1;5B"},
	}
	for i, item := range want {
		if bindings[i].Key != item.key || bindings[i].Action != item.action {
			t.Errorf("binding %d: got %+v", i, bindings[i])
		}
	}
	for i, value := range []string{info.KeyF52, info.KeyF53, info.KeyF54, info.KeyF55, info.KeyF56, info.KeyF57, info.KeyF58} {
		if value != want[i].sequence {
			t.Errorf("sequence %d: got %q", i, value)
		}
	}
	if info.KeyLeft != original.KeyLeft || info.KeyRight != original.KeyRight ||
		info.KeyDelete != original.KeyDelete || info.KeyF5 != original.KeyF5 {
		t.Fatal("normal arrows, Delete, or function keys were changed")
	}
}
