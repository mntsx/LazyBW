package ui

import "strings"

type hintAction struct{ section, action string }

// hintLabel formats the preferred binding while preserving special key names.
func hintLabel(key string) string {
	switch key {
	case "enter":
		return "ENTER"
	case "escape", "esc":
		return "esc"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	default:
		return key
	}
}

// hints uses the first configured key of each action in the footer.
func (a *App) hints() []string {
	hints := a.baseHints()
	s := a.S
	section := ""
	if s.Popup != nil {
		if s.Popup.RemoteReview != nil {
			section = "sync_popup"
		} else {
			section = "confirmation_popup"
		}
	} else {
		switch s.Mode {
		case DialogFoldersChoice:
			section = "folder_selector_popup"
		case DialogCreate:
			section = "create_item_popup"
		case DialogFieldType:
			section = "create_field_popup"
		case DialogLinkedChoice:
			section = "linked_field_popup"
		case DialogCardBrand:
			section = "card_brand_popup"
		case DialogIdentityTitle:
			section = "identity_title_popup"
		case DialogWebsite:
			section = "website_popup"
		case DialogReprompt:
			section = "reprompt_popup"
		case DialogFolder:
			section = "folder_popup"
		case DialogName, DialogField:
			section = "name_popup"
		case Find:
			section = "search"
		case Edit:
			section = "detail"
		default:
			switch s.Focus {
			case PaneVault, PaneTypes:
				section = "filters"
			case PaneFolders:
				section = "folders"
			case PaneItems:
				section = "items"
			case PaneContent:
				section = "content"
			case PaneChanges:
				section = "edits"
			}
		}
	}
	actionByHint := map[string]string{
		"ENTER select": "select", "ENTER unselect": "select",
		"ENTER apply": "confirm", "ENTER confirm": "confirm",
		"ENTER OK": "confirm", "ENTER verify": "confirm",
		"ENTER save": "save", "ENTER open": "open",
		"e edit": "edit", "s switch": "switch", "f favourite": "favorite",
		"n new": "new", "d delete": "delete", "f find": "search",
		"c copy": "copy", "r reveal": "reveal", "s push": "save",
		"d discard": "discard", "esc cancel": "cancel", "esc back": "back",
		"esc close": "cancel", "← back": "back", "→ open": "open",
		"s sync": "soft_sync", "s soft sync": "soft_sync",
		"h hard sync": "hard_sync", "a abort sync": "abort",
		"? next page": "next_page",
	}
	for i, label := range hints {
		if label == "↑/↓ move" {
			previous := s.Config.Key(section, "previous")
			next := s.Config.Key(section, "next")
			if previous != "" && next != "" {
				hints[i] = hintLabel(previous) + "/" + hintLabel(next) + " move"
			}
			continue
		}
		if label == "←/→ move" || label == "←/→ choose" {
			previous := s.Config.Key(section, "button_left")
			next := s.Config.Key(section, "button_right")
			if previous != "" && next != "" {
				verb := "choose"
				if label == "←/→ move" {
					verb = "move"
				}
				hints[i] = hintLabel(previous) + "/" + hintLabel(next) + " " + verb
			}
			continue
		}
		if label == "ctrl+q quit" || label == "ctrl+s sync" ||
			label == "ctrl+p push all" || label == "ctrl+d discard all" {
			action := map[string]string{
				"ctrl+q quit": "quit", "ctrl+s sync": "sync",
				"ctrl+p push all": "push_all", "ctrl+d discard all": "discard_all",
			}[label]
			if key := s.Config.Key("global", action); key != "" {
				hints[i] = hintLabel(key) + " " + strings.SplitN(label, " ", 2)[1]
			}
			continue
		}
		action := actionByHint[label]
		if action == "" {
			continue
		}
		if key := s.Config.Key(section, action); key != "" {
			parts := strings.SplitN(label, " ", 2)
			if len(parts) == 2 {
				hints[i] = hintLabel(key) + " " + parts[1]
			}
		}
	}
	return hints
}
