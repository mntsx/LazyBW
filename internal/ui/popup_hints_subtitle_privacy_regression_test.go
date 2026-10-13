package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestRepromptPreviewRequiresApproval hides list previews until verification.
func TestRepromptPreviewRequiresApproval(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	item := s.Items[0]
	item.Raw["reprompt"] = 1
	s.Focus = PaneItems
	a := New(s)

	before := NewCanvas(120, 44)
	a.mainRender(before)
	if row := stringCellRow(before, 5); strings.Contains(row, "alice") || !strings.Contains(row, "•••••") {
		t.Fatalf("reprompt subtitle must use dots: %q", row)
	}

	s.RepromptApprovedID = item.ID()
	after := NewCanvas(120, 44)
	a.mainRender(after)
	if row := stringCellRow(after, 5); !strings.Contains(row, "alice") {
		t.Fatalf("approved subtitle missing: %q", row)
	}

	s.SelectedID = item.ID()
	s.CloseItem()
	afterClosing := NewCanvas(120, 44)
	a.mainRender(afterClosing)
	if row := stringCellRow(afterClosing, 5); strings.Contains(row, "alice") {
		t.Fatalf("closing content did not revoke subtitle approval: %q", row)
	}
}

// TestSecureNoteNeverPreviewsNotes checks list drawing as well as the model.
func TestSecureNoteNeverPreviewsNotes(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	note := vault.NewItem(vault.SecureNote, "Note", "note-id", s.CLI.Cipher)
	note.Raw["notes"] = "sensitive-preview-must-not-appear\nother"
	s.Loaded([]*vault.Item{note}, nil)
	s.Focus = PaneItems
	a := New(s)
	c := NewCanvas(120, 44)
	a.mainRender(c)
	if row := stringCellRow(c, 5); !strings.Contains(row, "sensitive-preview-must-not-appear") {
		t.Fatalf("unprotected secure note must display a preview: %q", row)
	}
	note.Raw["reprompt"] = 1
	if err := vault.SealItem(note.Raw, s.CLI.Cipher); err != nil {
		t.Fatal(err)
	}
	locked := NewCanvas(120, 44)
	a.mainRender(locked)
	if row := stringCellRow(locked, 5); strings.Contains(row, "sensitive-preview-must-not-appear") || !strings.Contains(row, "••••") {
		t.Fatalf("reprompt-protected secure note was not masked: %q", row)
	}
	s.RepromptApprovedID = note.ID()
	approved := NewCanvas(120, 44)
	a.mainRender(approved)
	if row := stringCellRow(approved, 5); !strings.Contains(row, "sensitive-preview-must-not-appear") {
		t.Fatalf("approved secure note preview absent: %q", row)
	}
}

// TestPopupFooterKeepsBothKeyAndActionLeftAligned checks workspace footer hints.
func TestPopupFooterKeepsBothKeyAndActionLeftAligned(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.Popup = &Popup{Title: "Message", Acknowledge: true}
	a := New(s)
	c := NewCanvas(120, 44)
	const x, y, w = 1, 42, 118
	a.footer(c, x, y, w)

	assertFooterHintPairs(t, c, a.hints(), x, y)
}

// TestDialogFooterCentersBothKeyAndAction checks the dialog footer layout.
func TestDialogFooterCentersBothKeyAndAction(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.Mode = DialogCreate
	a := New(s)
	c := NewCanvas(120, 44)
	const x, y, w = 1, 42, 118
	a.footer(c, x, y, w)

	hints := PaginateHints(a.hints(), max(18, 3*w/4))[0]
	total := 0
	for _, hint := range hints {
		total += utf8.RuneCountInString(hint) + 3
	}
	if total > 0 {
		total -= 3
	}
	start := x + max(0, (max(18, 3*w/4)-total)/2)
	assertFooterHintPairs(t, c, hints, start, y)
	if cell := canvasAt(c, x, y); cell.R != ' ' {
		t.Fatalf("dialog footer is not centered: %#v", cell)
	}
}

// assertFooterHintPairs checks that each action follows its key without overlap.
func assertFooterHintPairs(t *testing.T, c *Canvas, hints []string, start, y int) {
	t.Helper()
	offset := 0
	for _, hint := range hints {
		parts := strings.SplitN(hint, " ", 2)
		keyX := start + offset
		for j, r := range []rune(parts[0]) {
			cell := canvasAt(c, keyX+j, y)
			if cell.R != r || cell.FG != CHintKey {
				t.Fatalf("key %q at x=%d: %#v", parts[0], keyX+j, cell)
			}
		}
		if len(parts) == 2 {
			actionX := keyX + utf8.RuneCountInString(parts[0]) + 1
			for j, r := range []rune(parts[1]) {
				cell := canvasAt(c, actionX+j, y)
				if cell.R != r || cell.FG != CHintAction {
					t.Fatalf("action %q at x=%d: %#v", parts[1], actionX+j, cell)
				}
			}
		}
		offset += utf8.RuneCountInString(hint) + 3
	}
}
