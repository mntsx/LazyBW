package ui

import (
	"strconv"
	"strings"
)

// A palette index maps to terminal ANSI colors; custom #RRGGBB values are
// emitted using truecolor escapes by Canvas.Render.
func ParseColor(value string, fallback int) int {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return fallback
	}
	raw := strings.ToLower(fields[0])
	if raw == "transparent" || raw == "default" {
		return CDefault
	}
	if strings.HasPrefix(raw, "ansi256:") {
		n, err := strconv.Atoi(strings.TrimPrefix(raw, "ansi256:"))
		if err == nil && n >= 0 && n <= 255 {
			return n
		}
	}
	if strings.HasPrefix(raw, "#") && len(raw) == 7 {
		n, err := strconv.ParseUint(raw[1:], 16, 32)
		if err == nil {
			return int(n) | 1<<24
		}
	}
	names := map[string]int{"ansi_black": CBlack, "ansi_red": CRed, "ansi_green": CGreen, "ansi_yellow": CYellow, "ansi_blue": CBlue, "ansi_magenta": CMagenta, "ansi_cyan": CCyan, "ansi_white": CWhite, "white": CWhite, "black": CBlack, "ansi_bright_black": CBBlack, "ansi_bright_red": CBRed, "ansi_bright_green": CBGreen, "ansi_bright_yellow": CBYellow, "ansi_bright_blue": CBBlue, "ansi_bright_magenta": CBMagenta, "ansi_bright_cyan": CBCyan, "ansi_bright_white": CBWhite}
	if v, ok := names[raw]; ok {
		return v
	}
	return fallback
}
func (c *Canvas) Theme(colors map[string]string) {
	mapping := map[int]string{
		CBlack: "black", CRed: "red", CGreen: "green", CYellow: "yellow",
		CBlue: "blue", CMagenta: "magenta", CCyan: "cyan", CWhite: "foreground",
		CBBlack: "muted", CBRed: "bright_red", CBGreen: "bright_green",
		CBYellow: "bright_yellow", CBBlue: "primary", CBMagenta: "bright_magenta",
		CBCyan: "bright_cyan", CBWhite: "bright_white",
		CTitle: "title_background", CFindBG: "find_background",
		CHintKey: "hint_key", CHintAction: "hint_action", CSubtitle: "subtitle",
	}
	c.remap = map[int]int{}
	for base, key := range mapping {
		if raw, ok := colors[key]; ok {
			c.remap[base] = ParseColor(raw, base)
		}
	}
}
