package ui

import (
	"context"
	"os/exec"
	"time"

	"github.com/awesome-gocui/gocui"
)

// terminalBackspace resolves legacy BS/DEL keys using the terminal's own
// terminfo entry. When the codes differ, the non-backspace code is interpreted
// as Ctrl+Backspace in a text editor. Ambiguous codes retain normal deletion.
func (a *App) terminalBackspace(key gocui.Key) error {
	if a.textEntryActive() && terminalWordBackspaceKey(key, a.terminalBS) {
		return a.wordEdit(-1, true)
	}
	return a.backspace()
}

// terminalWordBackspaceKey recognizes a separate legacy Ctrl+Backspace code.
// If an emulator uses the same code for both keys, they cannot be distinguished.
func terminalWordBackspaceKey(key gocui.Key, regular byte) bool {
	switch regular {
	case '\x7f':
		return key == gocui.KeyBackspace
	case '\x08':
		return key == gocui.KeyBackspace2
	default:
		return false
	}
}

// terminalBackspaceCode obtains the terminal's native Backspace representation.
// Unknown terminfo settings deliberately leave both BS and DEL as Backspace.
func terminalBackspaceCode() byte {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	value, err := exec.CommandContext(ctx, "tput", "kbs").Output()
	if err != nil || len(value) != 1 {
		return 0
	}
	return value[0]
}
