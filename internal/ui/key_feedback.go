package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/awesome-gocui/gocui"
)

// keyLabel converts terminal special keys into readable footer labels.
func keyLabel(key interface{}) string {
	switch key {
	case gocui.KeyArrowUp:
		return "↑"
	case gocui.KeyArrowDown:
		return "↓"
	case gocui.KeyArrowLeft:
		return "←"
	case gocui.KeyArrowRight:
		return "→"
	case gocui.KeyEnter:
		return "enter"
	case gocui.KeyEsc:
		return "esc"
	case gocui.KeyCtrlQ:
		return "ctrl+q"
	case gocui.KeyCtrlS:
		return "ctrl+s"
	case gocui.KeyCtrlP:
		return "ctrl+p"
	case gocui.KeyCtrlD:
		return "ctrl+d"
	case gocui.KeyCtrlR:
		return "ctrl+r"
	case gocui.KeyCtrlJ:
		return "ctrl+j"
	case gocui.KeyCtrlW:
		return "ctrl+w"
	case gocui.KeyCtrlZ:
		return "ctrl+z"
	case gocui.KeyCtrlY:
		return "ctrl+y"
	case gocui.KeyCtrlC:
		return "ctrl+c"
	case gocui.KeyCtrlV:
		return "ctrl+v"
	case gocui.KeyCtrlN:
		return "ctrl+n"
	case gocui.KeyBackspace, gocui.KeyBackspace2:
		return "backspace"
	case gocui.KeyDelete:
		return "delete"
	case gocui.KeyHome:
		return "home"
	case gocui.KeyEnd:
		return "end"
	case gocui.KeyTab:
		return "tab"
	case gocui.KeySpace:
		return "space"
	}
	if r, ok := key.(rune); ok && r >= 32 {
		return string(r)
	}
	return fmt.Sprint(key)
}

// reportKey makes input feedback visible for one second after each key press.
func (a *App) reportKey(key, action string) {
	if action == "" {
		action = "unbound"
	}
	a.S.KeyFeedback = key
	a.S.KeyFeedbackAction = action
	a.S.KeyFeedbackExpires = time.Now().Add(time.Second)
	a.feedbackActive.Store(true)
}

// handleKey reports the action that was actually available and carried out.
func (a *App) handleKey(key string, action func() error) error {
	textEntry := a.textEntryActive()
	// Hint pagination should change pages immediately without showing input
	// feedback, even when the previous key's feedback is still visible.
	if key == "?" && !textEntry {
		err := action()
		a.S.KeyFeedbackExpires = time.Time{}
		a.feedbackActive.Store(false)
		return err
	}
	label := "unbound"
	var before feedbackSnapshot
	if !textEntry {
		label = a.keyAction(key)
		before = a.feedbackState()
	}
	err := action()
	if a.exitRequested.Load() {
		// Returning ErrQuit directly avoids waiting for another UI update or
		// displaying a one-second footer hint before the terminal closes.
		return a.quitResult(err)
	}
	if textEntry {
		a.S.KeyFeedbackExpires = time.Time{}
		a.feedbackActive.Store(false)
		return err
	}
	if err != nil || label != "unbound" &&
		a.feedbackState() == before && feedbackRequiresChange(label) {
		label = "unbound"
	}
	a.reportKey(key, label)
	return err
}

// feedbackSnapshot contains only non-secret navigation metadata. Do not copy
// AuthSecret, Input, MasterText, or decrypted vault values into footer tracking.
type feedbackSnapshot struct {
	Screen                                Screen
	Mode                                  Mode
	Focus                                 Pane
	AuthFocus, FormFocus, Caret           int
	ItemIndex, FieldIndex, ChangeIndex    int
	FilterVault, FilterType, FilterFolder int
	Filters                               [4]int
	SelectedID                            string
	InputLength, SearchLength             int
	Busy, AuthVisible                     bool
	Popup                                 *Popup
	PopupYes                              bool
	PopupChoice                           int
	PopupPage                             int
	StageLength, ImportedLength           int
	Favourite                             bool
	ServerChoice, NewType, CustomType     int
	HintPage                              int
	Revealed                              bool
}

// feedbackState captures UI changes without retaining additional secret copies.
func (a *App) feedbackState() feedbackSnapshot {
	s := a.S
	snap := feedbackSnapshot{
		Screen: s.Screen, Mode: s.Mode, Focus: s.Focus,
		AuthFocus: s.AuthFocus, FormFocus: s.FormFocus, Caret: s.Caret,
		ItemIndex: s.ItemIndex, FieldIndex: s.FieldIndex,
		ChangeIndex: s.ChangeIndex, FilterVault: s.FilterVault,
		FilterType: s.FilterType, FilterFolder: s.FilterFolder,
		Filters: s.Filters, SelectedID: s.SelectedID,
		InputLength: len(s.Input), SearchLength: len(s.Search),
		Busy: s.Busy, AuthVisible: s.AuthVisible, Popup: s.Popup,
		StageLength: s.Stage.Len(), ImportedLength: len(s.ImportedChanges), ServerChoice: s.ServerChoice,
		NewType: s.NewType, CustomType: s.CustomType,
		HintPage: s.HintPage, Revealed: s.Revealed[s.RevealKey()],
	}
	if s.Focus == PaneContent && s.SelectedItem() != nil {
		snap.Favourite = s.SelectedItem().Favorite()
	}
	if s.Popup != nil {
		snap.PopupYes = s.Popup.Yes
		snap.PopupChoice = s.Popup.RemoteChoice
		snap.PopupPage = s.Popup.Page
	}
	return snap
}

// feedbackRequiresChange marks operations where an unchanged state means
// no action was performed. Copy, quit and text input have external effects.
func feedbackRequiresChange(action string) bool {
	switch action {
	case "move", "choose", "panels", "return", "open", "edit", "find", "favourite",
		"new", "delete", "push", "discard", "select", "cancel",
		"apply", "reveal", "confirm":
		return true
	}
	return false
}

// textEntryActive includes all textboxes, even modal name and login inputs.
func (a *App) textEntryActive() bool {
	s := a.S
	if s.Popup != nil || s.Screen == AuthChecking || s.Mode == DialogServer || s.Mode == DialogBackend {
		return false
	}
	if s.Screen != Workspace {
		return s.AuthFocus < 2
	}
	switch s.Mode {
	case Find, Edit, DialogReprompt:
		return true
	case DialogName, DialogField, DialogFolder:
		return s.FormFocus == 0
	}
	return false
}

// keyAction describes the key operation in the current context.
func (a *App) keyAction(key string) string {
	s := a.S
	if s.Screen == AuthChecking || s.Busy {
		return "unbound"
	}
	if key == "ctrl+s" {
		if s.Screen == Workspace && !s.LocalMode && s.ServerConnected &&
			s.Popup == nil && !s.Busy && (s.Mode == Browse || s.Mode == Find) {
			return "synchronize"
		}
		return "unbound"
	}

	if s.Popup != nil {
		if s.Popup.Blocking {
			return "unbound"
		}
		if s.Popup.Acknowledge {
			if key == "enter" {
				return "confirm"
			}
			if key == "esc" {
				return "cancel"
			}
			return "unbound"
		}
		switch key {
		case "←", "→", "↑", "↓":
			return "choose"
		case "enter", "y", "n", "s", "h", "a":
			if s.Popup.RemoteReview != nil && (key == "y" || key == "n") {
				return "unbound"
			}
			if s.Popup.RemoteReview == nil && (key == "s" || key == "h" || key == "a") {
				return "unbound"
			}
			return "confirm"
		case "c", "esc":
			return "cancel"
		case "?":
			pageSize := 10
			if s.Popup.RemoteReview != nil {
				pageSize = a.reviewPageSize()
			}
			if len(s.Popup.Lines) > pageSize {
				return "next page"
			}
		}
		return "unbound"
	}
	if s.Mode == DialogServer || s.Mode == DialogBackend {
		switch key {
		case "↑", "↓":
			return "move"
		case "enter":
			return "select"
		case "esc":
			return "cancel"
		}
		return "unbound"
	}
	if s.Mode == DialogReprompt && key == "ctrl+r" {
		return "reveal"
	}
	if s.Screen != Workspace {
		switch key {
		case "ctrl+s":
			return "server"
		case "ctrl+b":
			return "backend"
		case "↑", "↓", "←", "→":
			return "move"
		case "enter":
			return "select"
		case "esc":
			return "quit"
		case "backspace", "delete", "ctrl+w":
			return "delete"
		case "ctrl+r":
			return "reveal"
		}
		if utf8.RuneCountInString(key) == 1 && s.AuthFocus < 2 {
			return "type"
		}
		return "unbound"
	}
	if strings.HasPrefix(key, "alt+") {
		return "unbound"
	}
	// Global controls are only actions when the current context permits them.
	// Ctrl+R is an authentication-only reveal key, not a workspace shortcut.
	if key == "ctrl+q" && !a.modalOpen() {
		return "quit"
	}
	if (key == "ctrl+p" || key == "ctrl+d") &&
		!a.modalOpen() && s.Stage.Len() > 0 {
		if key == "ctrl+p" {
			if s.ServerConnected {
				return "push"
			}
			return "unbound"
		}
		return "discard"
	}
	if s.Mode == DialogCreate || s.Mode == DialogFieldType || s.Mode == DialogWebsite || s.Mode == DialogLinkedChoice {
		switch key {
		case "↑", "↓":
			return "move"
		case "enter":
			return "select"
		case "esc":
			return "cancel"
		}
		return "unbound"
	}
	if s.Mode == DialogName || s.Mode == DialogField || s.Mode == DialogFolder {
		switch key {
		case "enter":
			return "select"
		case "esc":
			return "cancel"
		case "←", "→", "↑", "↓":
			return "move"
		case "backspace", "delete", "ctrl+delete", "ctrl+backspace":
			return "delete"
		}
		if utf8.RuneCountInString(key) == 1 && s.FormFocus == 0 {
			return "type"
		}
		return "unbound"
	}
	if s.Mode == Edit || s.Mode == Find {
		if key == "↑" && s.Mode == Find {
			return "return"
		}
		switch key {
		case "enter":
			return "apply"
		case "esc":
			return "cancel"
		case "←", "→", "ctrl+←", "ctrl+→", "home", "end":
			return "move"
		case "backspace", "delete", "ctrl+backspace", "ctrl+delete", "ctrl+w":
			return "delete"
		case "ctrl+j":
			return "newline"
		}
		if utf8.RuneCountInString(key) == 1 || key == "space" {
			return "type"
		}
		return "unbound"
	}
	if key == "esc" || key == "←" && s.Focus != PaneItems {
		if s.Focus != PaneItems {
			return "return"
		}
		return "unbound"
	}
	if key == "↑" || key == "↓" {
		return "move"
	}
	if key == "→" {
		if s.Focus == PaneItems && len(s.Filtered()) > 0 {
			return "open"
		}
		if s.Focus == PaneContent && s.CurrentField() != nil &&
			!isCheckboxField(s.CurrentField()) {
			return "edit"
		}
	}
	if key == "enter" {
		switch s.Focus {
		case PaneItems:
			if len(s.Filtered()) > 0 {
				return "open"
			}
		case PaneContent:
			if s.CurrentField() != nil && !isCheckboxField(s.CurrentField()) {
				return "edit"
			}
		case PaneChanges:
			if s.Stage.Len() > 0 && s.ServerConnected && !s.ImportedChangeAt(s.ChangeIndex) {
				return "push"
			}
		default:
			return "select"
		}
	}
	if len(key) == 1 && key[0] >= '1' && key[0] <= '7' {
		pane := paneForNumber(s, int(key[0]-'0'))
		if pane == 0 {
			return "unbound"
		}
		if pane == s.Focus {
			return "return"
		}
		if pane <= PaneItems || pane == PaneContent && s.SelectedID != "" {
			return "panels"
		}
		return "unbound"
	}
	switch key {
	case "?":
		return "next page"
	case "f":
		if s.Focus == PaneItems {
			return "find"
		}
		if s.Focus == PaneContent && !s.LocalMode && s.SelectedItem() != nil {
			return "favourite"
		}
	case "n":
		if s.Focus == PaneItems || s.Focus == PaneFolders ||
			s.Focus == PaneContent && s.SelectedItem() != nil {
			return "new"
		}
	case "d":
		if s.Focus == PaneItems && len(s.Filtered()) > 0 ||
			s.Focus == PaneChanges && len(s.ChangesList()) > 0 ||
			s.Focus == PaneFolders && s.FilterFolder > 0 &&
				s.FilterFolder <= len(s.Folders) ||
			s.Focus == PaneContent && s.CurrentField() != nil && s.CurrentField().Custom {
			return "delete"
		}
	case "e":
		if s.Focus == PaneContent && s.CurrentField() != nil &&
			!isCheckboxField(s.CurrentField()) {
			return "edit"
		}
	case "c":
		if s.Focus == PaneContent && s.CurrentField() != nil && !s.CurrentField().Restricted {
			return "copy"
		}
	case "r":
		if s.Focus == PaneContent && s.CurrentField() != nil &&
			s.CurrentField().Hidden && !s.CurrentField().Restricted {
			return "reveal"
		}
	case "s":
		if s.Focus == PaneChanges && s.Stage.Len() > 0 && s.ServerConnected && !s.ImportedChangeAt(s.ChangeIndex) {
			return "push"
		}
		if s.Focus == PaneContent && isCheckboxField(s.CurrentField()) {
			return "switch"
		}
	}
	return "unbound"
}
