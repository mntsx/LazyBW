package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

var errInvalidConfigValue = errors.New("invalid configuration value")

func objectField(object map[string]json.RawMessage, name string) (map[string]json.RawMessage, bool) {
	raw, exists := object[name]
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	var value map[string]json.RawMessage
	return value, json.Unmarshal(raw, &value) == nil && value != nil
}

func stringField(object map[string]json.RawMessage, name string, nullable bool) (string, bool) {
	raw, exists := object[name]
	if !exists {
		return "", false
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nullable
	}
	var value string
	return value, json.Unmarshal(raw, &value) == nil
}

func boolField(object map[string]json.RawMessage, name string) bool {
	raw, exists := object[name]
	if !exists {
		return false
	}
	var value bool
	return json.Unmarshal(raw, &value) == nil
}

func validHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func validColor(value string) bool {
	fields := strings.Fields(value)
	if len(fields) == 0 || len(fields) > 2 {
		return false
	}
	base := strings.ToLower(fields[0])
	valid := base == "transparent" || base == "default" || base == "white" || base == "black"
	if strings.HasPrefix(base, "ansi256:") {
		index, err := strconv.Atoi(strings.TrimPrefix(base, "ansi256:"))
		valid = err == nil && index >= 0 && index <= 255
	}
	if strings.HasPrefix(base, "#") && len(base) == 7 {
		_, err := strconv.ParseUint(base[1:], 16, 32)
		valid = err == nil
	}
	if strings.HasPrefix(base, "ansi_") {
		name := strings.TrimPrefix(base, "ansi_")
		valid = map[string]bool{
			"black": true, "red": true, "green": true, "yellow": true,
			"blue": true, "magenta": true, "cyan": true, "white": true,
			"bright_black": true, "bright_red": true, "bright_green": true,
			"bright_yellow": true, "bright_blue": true, "bright_magenta": true,
			"bright_cyan": true, "bright_white": true,
		}[name]
	}
	if !valid {
		return false
	}
	if len(fields) == 2 {
		opacity := strings.TrimSuffix(fields[1], "%")
		amount, err := strconv.ParseFloat(opacity, 64)
		return strings.HasSuffix(fields[1], "%") && err == nil && amount >= 0 && amount <= 100
	}
	return true
}

// validateSettings returns the dotted path of the first invalid setting.
func validateSettings(settings map[string]json.RawMessage) string {
	authentication, ok := objectField(settings, "authentication")
	if !ok {
		return "authentication"
	}
	if _, ok := stringField(authentication, "client_env_path", true); !ok {
		return "authentication.client_env_path"
	}
	server, ok := stringField(authentication, "default_server", false)
	if !ok || (server != "us" && server != "eu") {
		return "authentication.default_server"
	}
	local, ok := objectField(settings, "local_replication")
	if !ok {
		return "local_replication"
	}
	if !boolField(local, "save_vault") {
		return "local_replication.save_vault"
	}
	if _, ok := stringField(local, "vault_path", false); !ok {
		return "local_replication.vault_path"
	}
	algorithm, ok := stringField(local, "vault_encryption", false)
	if !ok || (algorithm != "aes-256-gcm" && algorithm != "aes-256-gcm-siv" && algorithm != "xchacha20-poly1305") {
		return "local_replication.vault_encryption"
	}
	network, ok := objectField(settings, "network")
	if !ok {
		return "network"
	}
	for _, field := range []string{"us_ping_url", "eu_ping_url"} {
		value, ok := stringField(network, field, false)
		if !ok || !validHTTPSURL(value) {
			return "network." + field
		}
	}
	colors, ok := objectField(settings, "colors")
	if !ok {
		return "colors"
	}
	for name := range colors {
		value, ok := stringField(colors, name, false)
		if !ok || !validColor(value) {
			return "colors." + name
		}
	}
	return ""
}

// validateKeybinds returns the dotted path of the first invalid binding.
func validateKeybinds(keys map[string]json.RawMessage) string {
	for section := range DefaultKeybinds {
		name := section
		raw, exists := keys[section]
		if !exists && section == "edits" {
			raw, exists = keys["changes"]
			name = "changes"
		}
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return name
		}
		var actions map[string]json.RawMessage
		if json.Unmarshal(raw, &actions) != nil || actions == nil {
			return name
		}
		for action, binding := range actions {
			if _, known := DefaultKeybinds[section][action]; !known &&
				!(section == "global" && action == "save_all") {
				return name + "." + action
			}
			if _, err := decodeBindings(binding); err != nil {
				return name + "." + action
			}
		}
	}
	return ""
}
