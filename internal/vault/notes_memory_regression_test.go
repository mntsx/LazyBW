package vault

import "testing"

// TestNotesAreSealedInMemory checks transparent editing and remote serialization.
func TestNotesAreSealedInMemory(t *testing.T) {
	c, err := NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	item := NewItem(SecureNote, "Test", "note-1", c)
	item.Raw["notes"] = "sensitive first line"
	if err := SealItem(item.Raw, c); err != nil {
		t.Fatal(err)
	}
	if _, ok := item.Raw["notes"].(*Sealed); !ok {
		t.Fatal("note remained plaintext in the session model")
	}
	fields := item.Fields()
	var notes *Field
	for j := range fields {
		if fields[j].Name == "Notes" {
			notes = &fields[j]
		}
	}
	if notes == nil || notes.Hidden {
		t.Fatal("notes must remain a visible, editable field")
	}
	if got, err := notes.Reveal(c); err != nil || got != "sensitive first line" {
		t.Fatalf("note not readable in Content: %q, %v", got, err)
	}
	if err := item.Set([]any{"notes"}, "updated note"); err != nil {
		t.Fatal(err)
	}
	if _, ok := item.Raw["notes"].(*Sealed); !ok {
		t.Fatal("edited note remained plaintext")
	}
	out, err := PlainMap(c, item.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := out["notes"]; got != "updated note" {
		t.Fatalf("serialization changed note contents: %v", got)
	}
}
