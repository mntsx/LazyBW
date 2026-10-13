// Package terminalkeys configures word-editing key recognition independently
// from the GUI toolkit's key handling.
package terminalkeys

import (
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v2/terminfo"
)

// Action identifies a word-editing operation without depending on the UI.
type Action uint8

const (
	PreviousWord Action = iota + 1
	NextWord
	DeleteNextWord
)

// Binding associates a terminal-decoded key with an editing operation.
type Binding struct {
	Key    tcell.Key
	Action Action
	Label  string
}

// Register makes Ctrl+Left, Ctrl+Right and Ctrl+Delete distinguishable even
// when a GUI toolkit discards the Ctrl modifier from regular arrow keys.
// It changes only an in-process copy of the terminfo entry.
func Register() []Binding {
	name := os.Getenv("TERM")
	if name == "" {
		return nil
	}
	original, err := terminfo.LookupTerminfo(name)
	if err != nil {
		return nil
	}
	patched := *original
	bindings := registerWordSequences(&patched)
	terminfo.AddTerminfo(&patched)
	return bindings
}

// registerWordSequences assigns xterm CSI control sequences to private keys.
// The highest non-mouse function-key slots are used only for decoding inside
// LazyBW. These capabilities are not persisted or changed in the terminal.
func registerWordSequences(info *terminfo.Terminfo) []Binding {
	info.KeyF52 = "\x1b[1;5D"
	info.KeyF53 = "\x1b[1;5C"
	info.KeyF54 = "\x1b[3;5~"
	return []Binding{
		{Key: tcell.KeyF52, Action: PreviousWord, Label: "ctrl+←"},
		{Key: tcell.KeyF53, Action: NextWord, Label: "ctrl+→"},
		{Key: tcell.KeyF54, Action: DeleteNextWord, Label: "ctrl+delete"},
	}
}
