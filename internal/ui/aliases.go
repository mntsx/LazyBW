package ui

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/awesome-gocui/gocui"
	"github.com/miguelmnts/lazybw-go/internal/config"
)

type configuredBinding struct {
	section, action string
	fn              func() error
}

func normalizedBindingKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.NewReplacer("↑", "up", "↓", "down", "←", "left", "→", "right").Replace(name)
	switch name {
	case "esc":
		return "escape"

	}
	return name
}

// configuredBindings groups every configured action by key. Unlike the old
// alias layer, configured keys replace defaults instead of being merely added
// around a permanently registered hard-coded keymap.
func (a *App) configuredBindings() map[string][]configuredBinding {
	byKey := make(map[string][]configuredBinding)
	for section, actions := range config.DefaultKeybinds {
		for action := range actions {
			keys := a.S.Config.Keys(section, action)
			handler := a.configuredShortcut(section, action)
			if handler == nil {
				continue
			}
			for _, name := range keys {
				name = normalizedBindingKey(name)
				if name != "" {
					byKey[name] = append(byKey[name], configuredBinding{section, action, handler})
				}
			}
		}
	}
	return byKey
}

func (a *App) dispatchConfiguredKey(name string) (bool, error) {
	name = normalizedBindingKey(name)
	for _, binding := range a.configuredBindings()[name] {
		if !a.aliasAvailable(binding.section, binding.action) {
			continue
		}
		before := a.feedbackState()
		deferredSubmit := name == "enter" && configuredSubmitAction(binding.action) &&
			a.keyboardSubmitTarget() != submitFlashNone && a.gui != nil
		var err error
		if name == "enter" && configuredSubmitAction(binding.action) {
			err = a.keyboardEnter()
		} else {
			err = binding.fn()
		}
		if a.exitRequested.Load() {
			return true, a.quitResult(err)
		}
		action := aliasActionName(binding.section, binding.action)
		if action == "next page" {
			a.S.KeyFeedbackExpires = time.Time{}
			a.feedbackActive.Store(false)
			return true, err
		}
		if err != nil || !deferredSubmit && feedbackRequiresChange(action) && a.feedbackState() == before {
			action = "unbound"
		}
		a.reportKey(hintLabel(name), action)
		return true, err
	}
	return false, nil
}

func configuredSubmitAction(action string) bool {
	switch action {
	case "submit", "confirm", "select", "apply", "open", "save":
		return true
	}
	return false
}

// bindAliases registers configured keys that have no native editing callback.
// Native keys call dispatchConfiguredKey themselves before their text fallback.
func (a *App) bindAliases(g *gocui.Gui) error {
	native := map[string]bool{
		"up": true, "down": true, "left": true, "right": true,
		"enter": true, "ctrl+j": true, "escape": true, "esc": true, "backspace": true,
		"delete": true, "home": true, "end": true, "tab": true,
		"ctrl+q": true, "ctrl+s": true, "ctrl+p": true, "ctrl+d": true,
		"ctrl+r": true, "ctrl+b": true, "ctrl+w": true, "ctrl+n": true,
		"ctrl+z": true, "ctrl+y": true, "ctrl+c": true, "ctrl+v": true,
	}
	for name := range a.configuredBindings() {
		if native[name] {
			continue
		}
		key, ok := parseKey(name)
		if !ok {
			continue
		}
		label := name
		if err := g.SetKeybinding("", key, gocui.ModNone,
			func(_ *gocui.Gui, _ *gocui.View) error {
				handled, err := a.dispatchConfiguredKey(label)
				if !handled && !a.textEntryActive() {
					a.reportKey(hintLabel(label), "unbound")
				}
				return err
			}); err != nil {
			return err
		}
	}
	return nil
}

// aliasAvailable checks the selected panel and the current item/field before
// describing a configured shortcut as an available action.
func (a *App) aliasAvailable(section, action string) bool {
	s := a.S
	if s.Busy {
		return false
	}
	if s.Popup != nil {
		if s.Popup.Blocking {
			return false
		}
		if section == "sync_popup" {
			return s.Popup.RemoteReview != nil
		}
		if section == "confirmation_popup" {
			return s.Popup.RemoteReview == nil
		}
		return false
	}
	if section == "folder_selector_popup" {
		return s.Mode == DialogFoldersChoice
	}
	if section == "authentication" {
		return s.Screen == AuthClient && s.ServerConnected
	}
	if section == "master_password" {
		return s.Screen == AuthMaster && s.ServerConnected
	}
	if section == "text_input" {
		return a.inputMode() && s.Mode != Edit && !a.creationTextInputLocked()
	}
	if section == "authentication_offline" {
		return (s.Screen == AuthClient || s.Screen == AuthMaster) && !s.ServerConnected
	}
	if section == "offline" {
		return s.Screen == AuthOffline
	}
	if section == "server_picker" {
		return s.Mode == DialogServer
	}
	if section == "backend_picker" {
		return s.Mode == DialogBackend
	}
	if section == "name_popup" {
		return s.Mode == DialogName || s.Mode == DialogField
	}
	modalSection := map[string]Mode{
		"create_item_popup": DialogCreate, "create_field_popup": DialogFieldType,
		"linked_field_popup": DialogLinkedChoice, "card_brand_popup": DialogCardBrand,
		"identity_title_popup": DialogIdentityTitle, "website_popup": DialogWebsite,
		"reprompt_popup": DialogReprompt, "folder_popup": DialogFolder,
		"search": Find, "detail": Edit,
	}
	if mode, ok := modalSection[section]; ok {
		return s.Mode == mode
	}
	if section == "global" && action == "sync" {
		return s.Screen == Workspace && s.Mode != Edit && !s.LocalMode && s.ServerConnected
	}
	if section == "global" && action == "hint_next_page" {
		return true
	}
	if section == "global" && action == "quit" {
		return s.Mode != Edit && !a.modalOpen()
	}
	if s.Screen != Workspace || s.Mode != Browse {
		return false
	}
	if s.LocalMode {
		if section == "global" && (action == "push_all" || action == "discard_all" || action == "focus_my_vault" || action == "focus_changes") {
			return false
		}
		if section == "items" && action != "search" {
			return false
		}
		if section == "edits" {
			return false
		}
		if section == "content" && action != "copy" && action != "reveal" {
			return false
		}
	}
	switch section {
	case "global":
		switch action {
		case "push_all", "discard_all":
			return s.Stage.Len() > 0 && (action != "push_all" || s.ServerConnected)
		case "focus_my_vault", "focus_item_types", "focus_folders",
			"focus_changes", "focus_items", "focus_content", "hint_next_page":
			return true
		}
	case "filters":
		return s.Focus == PaneVault || s.Focus == PaneTypes || s.Focus == PaneFolders
	case "folders":
		return s.Focus == PaneFolders && !s.LocalMode &&
			(action != "delete" || s.Filters[2] > 0 && s.Filters[2] <= len(s.Folders))
	case "items":
		if s.Focus != PaneItems {
			return false
		}
		if action == "delete" {
			return s.CurrentItem() != nil && s.CurrentItem().CanEdit()
		}
		return true
	case "edits":
		if s.Focus == PaneChanges && action == "discard" && s.ImportedChangeAt(s.ChangeIndex) {
			return true
		}
		return s.Focus == PaneChanges && s.Stage.Len() > 0 && !s.ImportedChangeAt(s.ChangeIndex) &&
			(action != "save" || s.ServerConnected)
	case "content":
		if s.Focus != PaneContent {
			return false
		}
		f := s.CurrentField()
		switch action {
		case "next", "previous", "back":
			return true
		case "new_field":
			return s.SelectedItem() != nil && s.SelectedItem().CanEdit()
		case "favorite":
			return s.SelectedItem() != nil && s.SelectedItem().CanEdit()
		case "open_detail":
			return f != nil && f.Editable
		case "copy":
			return f != nil && !f.Restricted
		case "reveal":
			return f != nil && f.Hidden && !f.Restricted
		case "edit":
			return f != nil && f.Editable && !isCheckboxField(f)
		case "switch":
			return f != nil && f.Editable && isCheckboxField(f)
		case "delete_field":
			return f != nil && f.Custom && f.Editable
		}
	}
	return false
}

// configuredShortcut resolves a keymap action to its existing UI transition.
func (a *App) configuredShortcut(section, action string) func() error {
	switch action {
	case "next", "previous":
		delta := 1
		if action == "previous" {
			delta = -1
		}
		return func() error { return a.arrow(delta, 0) }
	case "button_left":
		return func() error { return a.arrow(0, -1) }
	case "button_right":
		return func() error { return a.arrow(0, 1) }
	case "submit", "confirm", "select", "apply", "open", "save":
		return a.enter
	case "cancel", "leave", "back", "return_to_items":
		return a.escape
	case "quit":
		return func() error { a.requestQuit(); return nil }
	case "server":
		return func() error { a.openServerPicker(); return nil }
	case "backend":
		return func() error { a.openBackendPicker(); return nil }
	case "sync":
		return func() error { a.checkRemote(); return nil }
	case "push_all":
		return func() error { a.confirmStage(true, true); return nil }
	case "discard_all":
		return func() error { a.confirmStage(false, true); return nil }
	case "reveal_secret", "reveal_password":
		return func() error { a.S.AuthVisible = !a.S.AuthVisible; return nil }
	case "hint_next_page":
		return func() error { a.nextHintPage(); return nil }
	case "next_page":
		return func() error { return a.char('?') }
	case "soft_sync":
		return func() error { return a.char('s') }
	case "hard_sync":
		return func() error { return a.char('h') }
	case "abort":
		return func() error { return a.char('a') }
	case "accept":
		return func() error { return a.char('y') }
	case "reject":
		return func() error { return a.char('n') }
	case "favorite":
		return func() error { return a.char('f') }
	case "switch":
		return func() error {
			if section == "content" {
				return a.S.SwitchCheckbox()
			}
			return a.char('s')
		}
	case "new", "new_field":
		return func() error { return a.char('n') }
	case "delete", "delete_field", "discard":
		return func() error { return a.char('d') }
	case "cut":
		if section == "detail" {
			return a.cutEditorSelection
		}
		return nil
	case "copy":
		if section == "detail" {
			return a.copyEditorSelection
		}
		return func() error { return a.char('c') }
	case "reveal":
		if section == "detail" {
			return a.toggleAuthVisibility
		}
		return func() error { return a.char('r') }
	case "edit", "open_detail":
		return func() error { return a.S.BeginEdit() }
	case "search":
		return func() error { return a.char('f') }
	case "newline":
		return func() error { a.S.Insert('\n'); return nil }
	case "maximize":
		return func() error { a.S.DetailMaximized = !a.S.DetailMaximized; return nil }
	case "wrap":
		return func() error { a.S.InputWrap = !a.S.InputWrap; return nil }
	case "undo":
		return func() error { a.S.UndoEdit(); return nil }
	case "redo":
		return func() error { a.S.RedoEdit(); return nil }
	case "paste":
		return a.pasteEditorText
	case "select_all":
		return func() error {
			if section == "detail" {
				a.S.SelectAll()
			}
			return nil
		}
	case "cursor_up":
		return func() error { return a.arrow(-1, 0) }
	case "cursor_down":
		return func() error { return a.arrow(1, 0) }
	case "cursor_left":
		return func() error { return a.arrow(0, -1) }
	case "cursor_right":
		return func() error { return a.arrow(0, 1) }
	case "cursor_start":
		return func() error { a.S.moveCaret(0, false); return nil }
	case "cursor_end":
		return func() error {
			if section == "text_input" && (a.S.Screen != Workspace || a.S.Mode == DialogReprompt) {
				a.S.Caret = utf8.RuneCountInString(a.authValue())
			} else {
				a.S.moveCaret(utf8.RuneCountInString(a.S.Input), false)
			}
			return nil
		}
	case "delete_backward":
		return a.backspace
	case "delete_forward":
		return a.deleteForward
	case "word_previous", "word_next", "delete_word_previous", "delete_word_next":
		return func() error {
			direction := 1
			if action == "word_previous" || action == "delete_word_previous" {
				direction = -1
			}
			remove := action == "delete_word_previous" || action == "delete_word_next"
			if section == "text_input" {
				return a.wordEdit(direction, remove)
			}
			if remove {
				a.S.DeleteWord(direction)
			} else {
				a.S.WordCursor(direction)
			}
			return nil
		}
	case "blank_line_previous", "blank_line_next":
		return func() error {
			direction := 1
			if action == "blank_line_previous" {
				direction = -1
			}
			a.S.CursorBlankLine(direction, false)
			return nil
		}
	case "select_left", "select_right":
		return func() error {
			direction := 1
			if action == "select_left" {
				direction = -1
			}
			a.S.CursorSelect(direction)
			return nil
		}
	case "select_up", "select_down":
		return func() error {
			direction := 1
			if action == "select_up" {
				direction = -1
			}
			if strings.Contains(a.S.Input, "\n") {
				a.S.CursorSelectLine(direction)
			}
			return nil
		}
	case "select_word_previous", "select_word_next":
		return func() error {
			direction := 1
			if action == "select_word_previous" {
				direction = -1
			}
			a.S.moveCaret(WordBoundary(a.S.Input, a.S.Caret, direction), true)
			return nil
		}
	case "select_blank_line_previous", "select_blank_line_next":
		return func() error {
			direction := 1
			if action == "select_blank_line_previous" {
				direction = -1
			}
			a.S.CursorBlankLine(direction, true)
			return nil
		}
	}
	focus := map[string]Pane{
		"focus_my_vault": PaneVault, "focus_item_types": PaneTypes,
		"focus_folders": PaneFolders, "focus_changes": PaneChanges,
		"focus_items": PaneItems, "focus_content": PaneContent,
	}
	if pane, ok := focus[action]; ok {
		return func() error { return a.focusConfiguredPane(pane) }
	}
	return nil
}

func (a *App) focusConfiguredPane(pane Pane) error {
	s := a.S
	if pane == s.Focus {
		if pane == PaneContent {
			s.CloseItem()
		} else if pane != PaneItems {
			s.Focus = PaneItems
		}
		return nil
	}
	if pane > PaneItems && (pane != PaneContent || s.SelectedID == "") {
		return nil
	}
	if pane == PaneContent {
		if item := s.SelectedItem(); item != nil &&
			(item.Raw["reprompt"] == 1 || item.Raw["reprompt"] == float64(1)) &&
			s.RepromptApprovedID != item.ID() {
			a.startReprompt(item.ID())
			return nil
		}
	}
	s.Focus = pane
	if pane <= PaneChanges {
		s.RepromptApprovedID = ""
	}
	if pane == PaneChanges {
		s.ChangeIndex = min(s.ChangeIndex, max(0, len(s.ChangesList())-1))
	}
	return nil
}

func parseKey(s string) (interface{}, bool) {
	if strings.HasPrefix(s, "ctrl+") && len(s) == 6 {
		r := s[5]
		if r >= 'a' && r <= 'z' {
			return gocui.Key(int(r - 'a' + 1)), true
		}
	}
	switch strings.ToLower(s) {
	case "up":
		return gocui.KeyArrowUp, true
	case "down":
		return gocui.KeyArrowDown, true
	case "left":
		return gocui.KeyArrowLeft, true
	case "right":
		return gocui.KeyArrowRight, true
	case "enter":
		return gocui.KeyEnter, true
	case "escape", "esc":
		return gocui.KeyEsc, true
	case "tab":
		return gocui.KeyTab, true
	case "ctrl+q":
		return gocui.KeyCtrlQ, true
	case "ctrl+s":
		return gocui.KeyCtrlS, true
	case "ctrl+p":
		return gocui.KeyCtrlP, true
	case "ctrl+d":
		return gocui.KeyCtrlD, true
	case "ctrl+r":
		return gocui.KeyCtrlR, true
	case "ctrl+b":
		return gocui.KeyCtrlB, true

	}
	if utf8.RuneCountInString(s) == 1 {
		r, _ := utf8.DecodeRuneInString(s)
		return r, true
	}
	return nil, false
}

// Defaults documents the actions read from the existing JSONC keymap.
var Defaults = config.DefaultKeybinds

// aliasActionName renders every configured action in footer terminology.
func aliasActionName(section, action string) string {
	if strings.HasPrefix(action, "focus_") {
		return "panels"
	}
	switch action {
	case "previous", "next", "button_left", "button_right":
		return "move"
	case "submit", "select":
		return "select"
	case "confirm", "accept", "reject":
		return "confirm"
	case "cancel":
		return "cancel"
	case "return_to_items", "leave":
		return "return"
	case "back":
		if section == "detail" {
			return "cancel"
		}
		return "return"
	case "apply":
		return "apply"
	case "open":
		return "open"
	case "save":
		if section == "detail" {
			return "apply"
		}
		return "push"
	case "push_all":
		return "push"
	case "discard", "discard_all":
		return "discard"
	case "delete", "delete_field":
		return "delete"
	case "new", "new_field":
		return "new"
	case "search":
		return "find"
	case "favorite":
		return "favourite"
	case "reveal", "reveal_secret", "reveal_password":
		return "reveal"
	case "sync", "soft_sync", "hard_sync":
		return "synchronize"
	case "abort":
		return "cancel"
	case "server":
		return "server"
	case "backend":
		return "backend"
	case "hint_next_page", "next_page":
		return "next page"
	case "newline":
		return "newline"
	case "maximize", "wrap", "undo", "redo", "cut", "paste", "select_all":
		return action
	case "cursor_up", "cursor_down", "cursor_left", "cursor_right",
		"cursor_start", "cursor_end", "word_previous", "word_next",
		"blank_line_previous", "blank_line_next":
		return "move"
	case "delete_backward", "delete_forward", "delete_word_previous", "delete_word_next":
		return "delete"
	case "select_left", "select_right", "select_up", "select_down",
		"select_word_previous", "select_word_next", "select_blank_line_previous", "select_blank_line_next":
		return "select"
	case "switch", "copy", "edit", "quit":
		return action
	}
	return "unbound"
}
