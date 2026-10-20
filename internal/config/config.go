// Package config handles the LazyBW JSON/JSONC configuration files.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Authentication struct {
	ClientEnvPath string   `json:"client_env_path"`
	DefaultServer string   `json:"default_server"`
	BackendMode   []string `json:"backend_mode"`
}
type Local struct {
	SaveVault       bool   `json:"save_vault"`
	VaultPath       string `json:"vault_path"`
	VaultEncryption string `json:"vault_encryption"`
}
type Network struct {
	USPingURL string `json:"us_ping_url"`
	EUPingURL string `json:"eu_ping_url"`
}
type Icons struct {
	Enabled         bool                         `json:"enabled"`
	Authentication  map[string]string            `json:"authentication"`
	Layout          map[string]string            `json:"layout"`
	Items           map[string]string            `json:"items"`
	Contents        map[string]string            `json:"contents"`
	Dialogs         map[string]string            `json:"dialogs"`
}

type Visual struct {
	Icons  Icons             `json:"icons"`
	Colors map[string]string `json:"colors"`
}

type Config struct {
	Authentication Authentication                 `json:"authentication"`
	Local          Local                          `json:"local_replication"`
	Network        Network                        `json:"network"`
	Visual         Visual                         `json:"visual"`
	// Colors is retained for callers while the on-disk setting lives in visual.colors.
	Colors         map[string]string              `json:"-"`
	Keybinds       map[string]map[string][]string `json:"-"`
	Directory      string                         `json:"-"`
}

// Icon returns a configured icon. Disabled icons are rendered as empty text.
func (c Config) Icon(section, name string) string {
	iconsConfig := c.Visual.Icons
	// Preserve the defaults for in-memory configurations created by older
	// callers; a configured icons object with enabled=false disables glyphs.
	if !iconsConfig.Enabled {
		if iconsConfig.Authentication == nil && iconsConfig.Layout == nil && iconsConfig.Items == nil && iconsConfig.Contents == nil && iconsConfig.Dialogs == nil {
			iconsConfig = DefaultIcons
		} else {
			return ""
		}
	}
	var icons map[string]string
	switch section {
	case "authentication":
		icons = iconsConfig.Authentication
	case "layout":
		icons = iconsConfig.Layout
	case "items":
		icons = iconsConfig.Items
	case "contents":
		icons = iconsConfig.Contents
	case "dialogs":
		icons = iconsConfig.Dialogs
	}
	return icons[name]
}

// FavouriteIcon uses an ASCII marker when visual icons are disabled.
func (c Config) FavouriteIcon(section string) string {
	icons := c.Visual.Icons
	if !icons.Enabled && (icons.Authentication != nil || icons.Layout != nil || icons.Items != nil || icons.Contents != nil || icons.Dialogs != nil) {
		return "f"
	}
	return c.Icon(section, "favourite")
}

// DefaultColors contains both terminal palette overrides and UI-specific roles.
// Values accept ANSI names, #RRGGBB, and 256-color palette indices.
var DefaultColors = map[string]string{
	"background": "transparent",
	"surface":    "ansi_black 35%", "panel": "transparent",
	"foreground": "ansi_white", "muted": "ansi_bright_black",
	"primary": "ansi_bright_blue", "primary_text": "ansi_bright_blue",
	"accent": "ansi_blue", "info": "ansi_bright_cyan",
	"selection": "ansi_black", "selection_text": "ansi_white",
	"auth_tip_background": "ansi_black", "auth_tip_text": "ansi_white",
	"hover": "ansi_bright_black 12%", "border": "ansi_blue",
	"border_focus": "ansi_bright_blue", "error": "ansi_bright_red",
	"warning": "ansi_bright_cyan", "success": "ansi_bright_green",
	"button": "ansi_bright_black 8%", "button_hover": "ansi_bright_black 28%",
	"title_background": "#0c1634", "find_background": "#222427",
	"black": "ansi_black", "red": "ansi_red", "green": "ansi_green",
	"yellow": "ansi_yellow", "blue": "ansi_blue", "magenta": "ansi_magenta",
	"cyan": "ansi_cyan", "white": "ansi_white",
	"bright_black": "ansi_bright_black", "bright_red": "ansi_bright_red",
	"bright_green": "ansi_bright_green", "bright_yellow": "ansi_bright_yellow",
	"bright_blue": "ansi_bright_blue", "bright_magenta": "ansi_bright_magenta",
	"bright_cyan": "ansi_bright_cyan", "bright_white": "ansi_bright_white",
	"hint_key": "ansi256:250", "hint_action": "ansi256:244",
	"subtitle": "ansi256:249",
}

var DefaultIcons = Icons{
	Enabled: true,
	Authentication: map[string]string{
		"server": "\U000F0379", "backend": "\U0000F013", "encryption": "\U000F0780", "local_vault": "\U000F01DA",
	},
	Layout: map[string]string{
		"vault": "\U000F10E5", "items": "\U000F10E5", "folders": "\U000F10E5", "edits": "\U0000F440", "connection": "\U000F0780",
	},
	Items: map[string]string{
		"all": "\U0000EB94", "favourite": "\U0000F005", "login": "\U000F059F", "secure_note": "\U0000F24A", "card": "\U0000F09D", "identity": "\U0000F2BB", "ssh_key": "\U000F0306", "folder": "\U000F0256", "no_folder": "\U000F19F9", "vault": "\U0000EDCA", "user": "\U0000F007",
	},
	Contents: map[string]string{
		"web": "\U000F059F", "favourite": "\U0000F005", "last_modified": "\U0000F0CE", "edit": "\U0000F044", "text": "\U000F0284", "hidden": "\U000F07F5", "checkbox": "\U000F0135", "linked": "\U000F0339",
	},
	Dialogs: map[string]string{"cancel": "\U000F073A", "linked_field": "\U000F0339"},
}

// A list is ordered: its first key is displayed in footer hints.
// Existing JSONC files containing one string per action remain supported.
var DefaultKeybinds = map[string]map[string][]string{
	"global": {
		"quit": {"ctrl+q"}, "sync": {"ctrl+s"},
		"push_all":       {"ctrl+p"},
		"discard_all":    {"ctrl+d"},
		"focus_my_vault": {"1"}, "focus_item_types": {"2"},
		"focus_folders": {"3"}, "focus_changes": {"4"},
		"focus_items": {"5"}, "focus_content": {"6"},
		"hint_next_page": {"?"},
	},
	"authentication": {
		"next": {"down"}, "previous": {"up"},
		"button_left": {"left"}, "button_right": {"right"},
		"submit": {"enter"}, "reveal_secret": {"ctrl+r"},
		"server": {"ctrl+s"}, "backend": {"ctrl+b"}, "quit": {"ctrl+q"},
	},
	"master_password": {
		"next": {"down"}, "previous": {"up"},
		"button_left": {"left"}, "button_right": {"right"},
		"submit": {"enter"}, "reveal_password": {"ctrl+r"},
		"server": {"ctrl+s"}, "backend": {"ctrl+b"}, "quit": {"ctrl+q"},
	},
	"authentication_offline": {
		"button_left": {"left"}, "button_right": {"right"},
		"submit": {"enter"}, "server": {"ctrl+s"}, "backend": {"ctrl+b"}, "quit": {"ctrl+q"},
	},
	"server_picker": {
		"next": {"down"}, "previous": {"up"},
		"confirm": {"right", "enter"}, "cancel": {"escape"},
	},
	"backend_picker": {
		"next": {"down"}, "previous": {"up"},
		"confirm": {"right", "enter"}, "cancel": {"escape"},
	},
	"filters": {
		"next": {"down"}, "previous": {"up"},
		"select": {"right", "enter"}, "return_to_items": {"escape"},
	},
	"folders": {
		"next": {"down"}, "previous": {"up"}, "select": {"right", "enter"},
		"new": {"n"}, "delete": {"d"}, "return_to_items": {"escape"},
	},
	"items": {
		"next": {"down"}, "previous": {"up"},
		"open": {"right", "enter"}, "search": {"f"},
		"delete": {"d"}, "new": {"n"},
	},
	"search": {
		"leave": {"escape"}, "apply": {"enter"},
	},
	"edits": {
		"save": {"s", "enter"}, "discard": {"d"},
		"next": {"down"}, "previous": {"up"},
	},
	"content": {
		"next": {"down"}, "previous": {"up"}, "back": {"left"},
		"new_field": {"n"}, "delete_field": {"d"},
		"copy": {"c"}, "reveal": {"r"},
		"edit":     {"e", "right", "enter"},
		"switch":   {"s", "right", "enter"},
		"favorite": {"f"},
	},
	"detail": {
		"back": {"escape"}, "save": {"enter"},
		"newline": {"ctrl+enter"}, "reveal": {"ctrl+r"},
		"wrap": {"ctrl+w"}, "undo": {"ctrl+z"}, "redo": {"ctrl+y"},
		"copy": {"ctrl+c"}, "paste": {"ctrl+v"},
	},
	"popup": {
		"cancel": {"escape"}, "next": {"down"},
		"previous": {"up"}, "button_left": {"left"},
		"button_right": {"right"}, "confirm": {"enter"},
	},
	"confirmation_popup": {
		"confirm": {"enter"}, "cancel": {"escape", "c"},
		"accept": {"y"}, "reject": {"n"},
		"button_left": {"left"}, "button_right": {"right"},
	},
	"sync_popup": {
		"soft_sync": {"s"}, "hard_sync": {"h"},
		"abort": {"a", "escape"}, "confirm": {"enter"},
		"button_left": {"left"}, "button_right": {"right"},
		"next_page": {"?"},
	},
	"create_item_popup": {
		"next": {"down"}, "previous": {"up"},
		"confirm": {"right", "enter"}, "cancel": {"escape"},
	},
	"create_field_popup": {
		"next": {"down"}, "previous": {"up"},
		"confirm": {"right", "enter"}, "cancel": {"escape"},
	},
	"linked_field_popup": {
		"next": {"down"}, "previous": {"up"},
		"confirm": {"right", "enter"}, "cancel": {"escape"},
	},
	"folder_selector_popup": {
		"next": {"down"}, "previous": {"up"},
		"switch": {"right", "s"}, "confirm": {"enter"}, "cancel": {"escape"},
	},
	"card_brand_popup": {
		"next": {"down"}, "previous": {"up"},
		"confirm": {"right", "enter"}, "cancel": {"escape"},
	},
	"identity_title_popup": {
		"next": {"down"}, "previous": {"up"},
		"confirm": {"right", "enter"}, "cancel": {"escape"},
	},
	"website_popup": {
		"next": {"down"}, "previous": {"up"},
		"confirm": {"right", "enter"}, "cancel": {"escape"},
	},
	"reprompt_popup": {"confirm": {"enter"}, "cancel": {"escape"},
		"reveal_password": {"ctrl+r"}},
	"name_popup": {
		"next": {"down"}, "previous": {"up"},
		"button_left": {"left"}, "button_right": {"right"},
		"confirm": {"enter"}, "cancel": {"escape"},
	},
	"folder_popup": {
		"next": {"down"}, "previous": {"up"},
		"button_left": {"left"}, "button_right": {"right"},
		"confirm": {"enter"}, "cancel": {"escape"},
	},
}

// NormalizeJSONC strips comments and trailing commas outside quoted strings.
func NormalizeJSONC(src []byte) ([]byte, error) {
	out := append([]byte(nil), src...)
	inString, escaped := false, false
	for i := 0; i < len(out); i++ {
		if inString {
			if escaped {
				escaped = false
			} else if out[i] == '\\' {
				escaped = true
			} else if out[i] == '"' {
				inString = false
			}
			continue
		}
		if out[i] == '"' {
			inString = true
			continue
		}
		if out[i] == '/' && i+1 < len(out) && out[i+1] == '/' {
			for i < len(out) && out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
				i++
			}
			continue
		}
		if out[i] == '/' && i+1 < len(out) && out[i+1] == '*' {
			j := i + 2
			for j+1 < len(out) && !(out[j] == '*' && out[j+1] == '/') {
				j++
			}
			if j+1 >= len(out) {
				return nil, errors.New("unterminated JSONC block comment")
			}
			for k := i; k < j+2; k++ {
				if out[k] != '\n' && out[k] != '\r' {
					out[k] = ' '
				}
			}
			i = j + 1
		}
	}
	inString = false
	escaped = false
	for i := 0; i < len(out); i++ {
		if inString {
			if escaped {
				escaped = false
			} else if out[i] == '\\' {
				escaped = true
			} else if out[i] == '"' {
				inString = false
			}
			continue
		}
		if out[i] == '"' {
			inString = true
			continue
		}
		if out[i] == ',' {
			j := i + 1
			for j < len(out) && (out[j] == ' ' || out[j] == '\n' || out[j] == '\t' || out[j] == '\r') {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				out[i] = ' '
			}
		}
	}
	return expandLongUnicodeEscapes(out)
}

// expandLongUnicodeEscapes accepts the readable \U000F0000 notation used for
// icons in settings.jsonc before handing the document to the JSON decoder.
func expandLongUnicodeEscapes(data []byte) ([]byte, error) {
	var out strings.Builder
	for index := 0; index < len(data); {
		if data[index] != '\\' || index+1 >= len(data) || data[index+1] != 'U' {
			out.WriteByte(data[index])
			index++
			continue
		}
		end := index + 2
		for end < len(data) && end < index+10 && (data[end] >= '0' && data[end] <= '9' || data[end] >= 'A' && data[end] <= 'F' || data[end] >= 'a' && data[end] <= 'f') {
			end++
		}
		// A previous LazyBW version emitted \U000EDCA (seven digits). Keep it
		// readable while newly written configuration always uses eight digits.
		if end-index-2 != 7 && end-index-2 != 8 {
			return nil, fmt.Errorf("invalid long Unicode escape at byte %d: %q", index, data[index:end])
		}
		value, err := strconv.ParseUint(string(data[index+2:end]), 16, 32)
		if err != nil || value > 0x10FFFF || value >= 0xD800 && value <= 0xDFFF {
			return nil, fmt.Errorf("invalid long Unicode escape at byte %d: %q", index, data[index:end])
		}
		out.WriteRune(rune(value))
		index = end
	}
	return []byte(out.String()), nil
}

// formatLongUnicodeEscapes keeps private-use glyphs readable in JSONC files.
func formatLongUnicodeEscapes(data []byte) []byte {
	var out strings.Builder
	for _, r := range string(data) {
		if r >= 0xE000 {
			fmt.Fprintf(&out, `\U%08X`, r)
		} else {
			out.WriteRune(r)
		}
	}
	return []byte(out.String())
}

func readObject(path string, defaults any) (map[string]json.RawMessage, error) {
	unlock, err := acquireConfigLock(path)
	if err != nil {
		return nil, err
	}
	defer unlock()
	b, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		b, err = json.MarshalIndent(defaults, "", "  ")
		if err != nil {
			return nil, err
		}
		b = formatLongUnicodeEscapes(b)
		b = annotateDefaults(filepath.Base(path), b)
		b = append(b, '\n')
		if err = writeConfigAtomic(path, b, nil, true); err != nil {
			return nil, err
		}
	}
	b = bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})
	normalized, err := NormalizeJSONC(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(normalized, &obj); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", filepath.Base(path), err)
	}
	if obj == nil {
		return nil, fmt.Errorf("%s must be a JSON object", filepath.Base(path))
	}
	if err = addMissingDefaults(path, b, normalized, defaults); err != nil {
		return nil, fmt.Errorf("complete %s defaults: %w", filepath.Base(path), err)
	}
	// Decode the completed file so newly persisted defaults apply immediately,
	// not only after the next process start.
	completed, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	completed = bytes.TrimPrefix(completed, []byte{0xef, 0xbb, 0xbf})
	normalized, err = NormalizeJSONC(completed)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if err = json.Unmarshal(normalized, &obj); err != nil {
		return nil, fmt.Errorf("invalid completed %s: %w", filepath.Base(path), err)
	}
	return obj, nil
}
func preferred(dir, stem string) string {
	p := filepath.Join(dir, stem+".jsonc")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	legacy := filepath.Join(dir, stem+".json")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return p
}
func createGitignore(dir string) error {
	file, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create .gitignore: %w", err)
	}
	defer file.Close()
	if _, err := file.WriteString("bitwarden/\n"); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

func DefaultDirectory() (string, error) {
	if x := os.Getenv("LAZYBW_CONFIG_DIR"); x != "" {
		return x, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lazybw"), nil
}

// Load preserves configured values, persists missing defaults, and gives JSONC precedence.
func Load(dir string) (Config, error) {
	if dir == "" {
		var err error
		dir, err = DefaultDirectory()
		if err != nil {
			return Config{}, err
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Config{}, err
	}
	if err := createGitignore(dir); err != nil {
		return Config{}, err
	}
	settingsPath := preferred(dir, "settings")
	settingsDefaults := map[string]any{
		"authentication":     map[string]any{"client_env_path": nil, "default_server": "us", "backend_mode": []string{"bw serve", "bw"}},
		"local_replication": map[string]any{"save_vault": false, "vault_path": "$HOME/.local/share/lazybw/bw-vault", "vault_encryption": "aes-256-gcm"},
		"network":            map[string]string{"us_ping_url": "https://api.bitwarden.com", "eu_ping_url": "https://api.bitwarden.eu"},
		"visual":             Visual{Icons: DefaultIcons, Colors: DefaultColors},
	}
	// Complete legacy colors only when that former root object already exists.
	if existing, readErr := os.ReadFile(settingsPath); readErr == nil {
		if normalized, normalizeErr := NormalizeJSONC(existing); normalizeErr == nil {
			var object map[string]json.RawMessage
			if json.Unmarshal(normalized, &object) == nil && object["colors"] != nil {
				settingsDefaults["colors"] = DefaultColors
			}
		}
	}
	settings, err := readObject(settingsPath, settingsDefaults)
	if err != nil {
		return Config{}, err
	}
	keys, err := readObject(preferred(dir, "keybinds"), DefaultKeybinds)
	if err != nil {
		return Config{}, err
	}
	if field := validateSettings(settings); field != "" {
		return Config{}, fmt.Errorf("%w in settings.jsonc: %s", errInvalidConfigValue, field)
	}
	if field := validateKeybinds(keys); field != "" {
		return Config{}, fmt.Errorf("%w in keybinds.jsonc: %s", errInvalidConfigValue, field)
	}
	c := Config{Directory: dir, Colors: map[string]string{}, Keybinds: map[string]map[string][]string{}, Visual: Visual{Icons: DefaultIcons, Colors: map[string]string{}}}
	c.Local.VaultEncryption = "aes-256-gcm"
	c.Network.USPingURL = "https://api.bitwarden.com"
	c.Network.EUPingURL = "https://api.bitwarden.eu"
	for k, v := range DefaultColors {
		c.Colors[k], c.Visual.Colors[k] = v, v
	}
	if raw := settings["authentication"]; len(raw) > 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil {
			_ = json.Unmarshal(fields["client_env_path"], &c.Authentication.ClientEnvPath)
			_ = json.Unmarshal(fields["default_server"], &c.Authentication.DefaultServer)
			modes, modeErr := decodeBackendModes(fields["backend_mode"])
			if modeErr != nil {
				return Config{}, fmt.Errorf("%w in settings.jsonc: authentication.backend_mode", errInvalidConfigValue)
			}
			c.Authentication.BackendMode = modes
		}
	}
	if len(c.Authentication.BackendMode) == 0 {
		c.Authentication.BackendMode = []string{"bw serve", "bw"}
	}
	if raw := settings["local_replication"]; len(raw) > 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil {
			_ = json.Unmarshal(fields["save_vault"], &c.Local.SaveVault)
			_ = json.Unmarshal(fields["vault_path"], &c.Local.VaultPath)
			_ = json.Unmarshal(fields["vault_encryption"], &c.Local.VaultEncryption)
		}
	}
	if raw := settings["network"]; len(raw) > 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil {
			_ = json.Unmarshal(fields["us_ping_url"], &c.Network.USPingURL)
			_ = json.Unmarshal(fields["eu_ping_url"], &c.Network.EUPingURL)
		}
	}
	// Missing, null and unknown regions consistently fall back to the US.
	c.Authentication.DefaultServer = NormalizeServer(c.Authentication.DefaultServer)
	if raw := settings["visual"]; len(raw) > 0 {
		if err = json.Unmarshal(raw, &c.Visual); err != nil {
			return Config{}, err
		}
		for k, v := range c.Visual.Colors {
			c.Colors[k] = v
		}
	}
	// Accept the former top-level colors object for existing configurations.
	// It wins while it remains present, so completing visual cannot alter a
	// user's existing palette.
	if raw := settings["colors"]; len(raw) > 0 {
		var override map[string]string
		if err = json.Unmarshal(raw, &override); err != nil {
			return Config{}, err
		}
		for k, v := range override {
			c.Colors[k], c.Visual.Colors[k] = v, v
		}
	}
	for section, defaults := range DefaultKeybinds {
		c.Keybinds[section] = make(map[string][]string)
		for action, bindings := range defaults {
			c.Keybinds[section][action] = append([]string(nil), bindings...)
		}
		raw := keys[section]
		if section == "edits" && len(raw) == 0 {
			raw = keys["changes"]
		}
		if len(raw) > 0 {
			var overrides map[string]json.RawMessage
			if err := json.Unmarshal(raw, &overrides); err != nil {
				return Config{}, err
			}
			for action, value := range overrides {
				if section == "global" && action == "save_all" {
					if _, canonical := overrides["push_all"]; canonical {
						continue
					}
					action = "push_all"
				}
				bindings, err := decodeBindings(value)
				if err != nil {
					return Config{}, fmt.Errorf("keybinds.%s.%s: %w", section, action, err)
				}
				c.Keybinds[section][action] = bindings
			}
		}
	}
	return c, nil
}

// Keys returns every configured binding for an action, in display order.
func (c Config) Keys(section, action string) []string {
	if actions, ok := c.Keybinds[section]; ok {
		if bindings, ok := actions[action]; ok {
			return append([]string(nil), bindings...)
		}
	}
	return append([]string(nil), DefaultKeybinds[section][action]...)
}

// Key returns the preferred binding displayed in the footer.
func (c Config) Key(section, action string) string {
	bindings := c.Keys(section, action)
	if len(bindings) == 0 {
		return ""
	}
	return bindings[0]
}

// decodeBindings accepts the legacy string form and ordered binding arrays.
func decodeBindings(raw json.RawMessage) ([]string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("expected a key or list of keys, got null")
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		one = strings.ToLower(strings.TrimSpace(one))
		if one == "" {
			return nil, errors.New("empty key binding")
		}
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, fmt.Errorf("expected a key or list of keys: %w", err)
	}
	for index, key := range many {
		many[index] = strings.ToLower(strings.TrimSpace(key))
		if many[index] == "" {
			return nil, errors.New("empty key binding")
		}
	}
	return many, nil
}

var defaultEnum = map[string]string{
	"default_server":   "us, eu",
	"backend_mode":     "bw, bw serve",
	"vault_encryption": "aes-256-gcm, aes-256-gcm-siv, xchacha20-poly1305",
}
var defaultLine = regexp.MustCompile(`^(\s*)"([^"]+)":`)

// annotateDefaults adds JSONC hints while keeping generated settings editable.
func annotateDefaults(filename string, jsonData []byte) []byte {
	var out strings.Builder
	if filename == "keybinds.jsonc" {
		out.WriteString("// Each action accepts a list of keys. The first is used in hints.\n")
		out.WriteString("// Examples: [\"s\", \"enter\"], [\"ctrl+p\"]. Existing strings still work.\n")
	} else if filename == "settings.jsonc" {
		out.WriteString("// Visual colors: transparent, ansi_<color>, ansi_bright_<color>, ansi256:0..255, #RRGGBB.\n")
		out.WriteString("// Available colors: black, red, green, yellow, blue, magenta, cyan, white.\n")
	}
	for _, line := range strings.Split(string(jsonData), "\n") {
		if filename == "settings.jsonc" {
			switch line {
			case "    \"icons\": {":
				out.WriteString("    // Icons: set enabled to false to use text-only markers.\n")
			case "      \"authentication\": {":
				out.WriteString("      // Authentication\n")
			case "      \"layout\": {":
				out.WriteString("      // Layout\n")
			case "      \"items\": {":
				out.WriteString("      // Items\n")
			case "      \"contents\": {":
				out.WriteString("      // Contents\n")
			case "      \"dialogs\": {":
				out.WriteString("      // Dialogs\n")
			case "    \"colors\": {":
				out.WriteString("    // Colors\n")
			}
			match := defaultLine.FindStringSubmatch(line)
			if len(match) > 0 {
				if choices, ok := defaultEnum[match[2]]; ok {
					out.WriteString(match[1] + "// Allowed values: " + choices + "\n")
				}
			}
		}
		out.WriteString(line + "\n")
	}
	return []byte(strings.TrimSuffix(out.String(), "\n"))
}

// BackendModes accepts the canonical ordered backend selection values.
func BackendModes(value []string) ([]string, error) {
	if len(value) == 0 {
		return nil, errors.New("backend mode list is empty")
	}
	modes := make([]string, 0, len(value))
	seen := make(map[string]bool, len(value))
	for _, value := range value {
		mode := strings.ToLower(strings.TrimSpace(value))
		if mode != "bw" && mode != "bw serve" || seen[mode] {
			return nil, errors.New("invalid backend mode")
		}
		seen[mode] = true
		modes = append(modes, mode)
	}
	return modes, nil
}

func decodeBackendModes(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []string{"bw", "bw serve"}, nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return BackendModes([]string{one})
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}
	return BackendModes(many)
}

// NormalizeServer accepts the supported Bitwarden cloud regions.
func NormalizeServer(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "eu":
		return "eu"
	case "us":
		return "us"
	default:
		return "us"
	}
}
