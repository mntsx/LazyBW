package ui

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/awesome-gocui/gocui"
)

// keyLabel converts a terminal event into its transport label. It deliberately
// reuses terminalEventBindings rather than defining any user-facing shortcut;
// actions and their defaults are resolved only through Config.Keys.
func keyLabel(key interface{}) string {
	for _, binding := range terminalEventBindings() {
		if binding.key == key {
			return binding.name
		}
	}
	if key == gocui.KeySpace {
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
	// feedback, even when the previous key's feedback is still visible. Resolve
	// the key through configuration; do not place a literal shortcut here.
	if a.isConfiguredKey(key, "global", "hint_next_page") && !textEntry {
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

func (a *App) isConfiguredKey(key, section, action string) bool {
	for _, binding := range a.S.Config.Keys(section, action) {
		if normalizedBindingKey(binding) == normalizedBindingKey(key) {
			return true
		}
	}
	return false
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

// keyAction describes a configured action that is currently available. Key
// combinations are defined solely by the configured keymap; text entry is the
// only generic fallback because it represents input rather than a shortcut.
func (a *App) keyAction(key string) string {
	key = normalizedBindingKey(key)
	for _, binding := range a.configuredBindings()[key] {
		if a.aliasAvailable(binding.section, binding.action) {
			return aliasActionName(binding.section, binding.action)
		}
	}
	if a.textEntryActive() && utf8.RuneCountInString(key) == 1 {
		return "type"
	}
	return "unbound"
}
