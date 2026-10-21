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
			boundAction, visible := a.localFocusAction(section, action)
			if !visible {
				continue
			}
			keys := a.S.Config.Keys(section, action)
			handler := a.configuredShortcut(section, boundAction)
			if handler == nil {
				continue
			}
			for _, name := range keys {
				name = normalizedBindingKey(name)
				if name != "" {
					byKey[name] = append(byKey[name], configuredBinding{section, boundAction, handler})
				}
			}
		}
	}
	return byKey
}

// localFocusAction compacts the rendered local panels into the first four
// configured global focus slots. It deliberately remaps semantic actions, not
// key literals, so a user's keybinds.jsonc remains the source of every key.
func (a *App) localFocusAction(section, action string) (string, bool) {
	if !a.S.LocalMode || section != "global" {
		return action, true
	}
	switch action {
	case "focus_my_vault":
		return "focus_item_types", true
	case "focus_item_types":
		return "focus_folders", true
	case "focus_folders":
		return "focus_items", true
	case "focus_changes":
		return "focus_content", true
	case "focus_items", "focus_content":
		return "", false
	default:
		return action, true
	}
}

// localFocusSlotAction returns the configured global slot assigned to a local
// panel target. It is the inverse of localFocusAction and never chooses a key.
func localFocusSlotAction(action string) string {
	switch action {
	case "focus_item_types":
		return "focus_my_vault"
	case "focus_folders":
		return "focus_item_types"
	case "focus_items":
		return "focus_folders"
	case "focus_content":
		return "focus_changes"
	default:
		return ""
	}
}

func (a *App) dispatchConfiguredKey(name string) (bool, error) {
	name = normalizedBindingKey(name)
	for _, binding := range a.configuredBindings()[name] {
		if !a.aliasAvailable(binding.section, binding.action) {
			continue
		}
		before := a.feedbackState()
		// Enter is a terminal transport event with special submit-flash handling.
		// It does not assign an action: binding.action still comes exclusively
		// from Config.Keys / DefaultKeybinds.
		isTerminalEnter := name == terminalEnterLabel
		deferredSubmit := isTerminalEnter && configuredSubmitAction(binding.action) &&
			a.keyboardSubmitTarget() != submitFlashNone && a.gui != nil
		var err error
		if isTerminalEnter && configuredSubmitAction(binding.action) {
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
// The native set below owns gocui event transport only; it must never define
// defaults or behavior for a user-facing shortcut. Those live in DefaultKeybinds.
func (a *App) bindAliases(g *gocui.Gui) error {
	for name := range a.configuredBindings() {
		if nativeTerminalLabel(name) {
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

// configuredShortcut resolves semantic configured actions. Never simulate a
// legacy key here: DefaultKeybinds is the sole source of key combinations.
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
		return a.nextPopupPage
	case "soft_sync":
		return func() error { return a.chooseRemoteSync(0) }
	case "hard_sync":
		return func() error { return a.chooseRemoteSync(1) }
	case "abort":
		return func() error { return a.chooseRemoteSync(2) }
	case "accept":
		return func() error { return a.setPopupConfirmation(true) }
	case "reject":
		return func() error { return a.setPopupConfirmation(false) }
	case "favorite":
		return a.toggleFavorite
	case "switch":
		if section == "content" {
			return a.S.SwitchCheckbox
		}
		return a.toggleFolderChoice
	case "new", "new_field":
		return a.createWorkspaceItem
	case "delete", "delete_field":
		return a.deleteWorkspaceItem
	case "discard":
		return func() error { a.confirmStage(false, false); return nil }
	case "cut":
		if section == "detail" {
			return a.cutEditorSelection
		}
		return nil
	case "copy":
		if section == "detail" {
			return a.copyEditorSelection
		}
		return a.copyCurrentField
	case "reveal":
		if section == "detail" {
			return a.toggleAuthVisibility
		}
		return a.revealCurrentField
	case "edit", "open_detail":
		return func() error { return a.S.BeginEdit() }
	case "search":
		return a.startSearch
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

// parseKey decodes the JSONC key notation into gocui transport events. Its
// key names are syntax, never shortcut defaults or action assignments; those
// are defined only by config.DefaultKeybinds and Config.Keys.
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
