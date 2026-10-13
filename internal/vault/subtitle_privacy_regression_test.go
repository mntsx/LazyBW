package vault

import (
	"strings"
	"testing"
)

func TestSecureNoteSubtitleUsesFirstLineWhenUnlocked(t *testing.T) {
	c, err := NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	item := &Item{Cipher: c, Raw: map[string]any{"type": SecureNote, "notes": "secret first line\nsecond line"}}
	if err := SealItem(item.Raw, c); err != nil {
		t.Fatal(err)
	}
	if got := item.Subtitle(); got != "secret first line" {
		t.Fatalf("first-line preview: %q", got)
	}
	if got := item.MaskedSubtitle(); !strings.HasPrefix(got, "••••") || strings.Contains(got, "secret") {
		t.Fatalf("protected preview: %q", got)
	}
	login := &Item{Raw: map[string]any{"type": Login, "login": map[string]any{"username": "public@example.test"}}}
	if got := login.Subtitle(); got != "public@example.test" {
		t.Fatal(got)
	}
	if got := login.MaskedSubtitle(); got != strings.Repeat("•", len([]rune("public@example.test"))) {
		t.Fatal(got)
	}
}
