package ui

import (
	"strings"
	"testing"
)

// TestRemoteReviewExplainsTheActiveContext prevents registration wording in manual sync.
func TestRemoteReviewExplainsTheActiveContext(t *testing.T) {
	for _, tc := range []struct {
		name, expected, forbidden string
		registrationBlocked       bool
	}{
		{
			name:      "manual synchronization",
			expected:  "Bitwarden remote has changes not yet shown in LazyBW",
			forbidden: "Cannot push edits to Bitwarden",
		},
		{
			name:                "registration blocked",
			expected:            "Cannot push edits to Bitwarden: remote vault has changed",
			forbidden:           "Bitwarden remote has changes not yet shown in LazyBW",
			registrationBlocked: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, review := remoteReviewFixture(t)
			defer s.CLI.Cipher.Close()
			a := New(s)
			a.showRemoteReviewWithContext(review, tc.registrationBlocked)
			canvas := NewCanvas(120, 36)
			a.popupRender(canvas)
			shown := canvas.Render()
			if !strings.Contains(shown, tc.expected) || strings.Contains(shown, tc.forbidden) {
				t.Fatalf("incorrect context in remote review: %q", shown)
			}
			for _, phrase := range []string{
				"Both options import remote data; neither uploads local edits",
				"SOFT SYNC", "HARD SYNC", "ABORT SYNC",
			} {
				if !strings.Contains(shown, phrase) {
					t.Fatalf("missing %q in popup", phrase)
				}
			}
		})
	}
}

// TestRemoteReviewListHasOneBlankRow checks compact diff-to-explanation spacing.
func TestRemoteReviewListHasOneBlankRow(t *testing.T) {
	s, review := remoteReviewFixture(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	a.showRemoteReview(review)
	canvas := NewCanvas(120, 36)
	a.popupRender(canvas)
	row := a.dialogY + 6
	for _, x := range []int{a.dialogX + 5, a.dialogX + 12, a.dialogX + 30} {
		if canvas.Cells[(row+1)*canvas.W+x].R != ' ' {
			t.Fatalf("expected a blank line between changes and instructions at x=%d", x)
		}
	}
	if canvas.Cells[(row+2)*canvas.W+a.dialogX+3].R != 'B' {
		t.Fatal("explanation must start exactly one blank row after the last change")
	}
}

// TestRemoteReviewButtonsMatchMouseHitAreas checks symmetric text and geometry.
func TestRemoteReviewButtonsMatchMouseHitAreas(t *testing.T) {
	for _, width := range []int{50, 86, 120} {
		buttons := remoteReviewButtons(4, width)
		for i, button := range buttons {
			if button.width-len(button.label) < 2 ||
				(button.width-len(button.label))%2 != 0 {
				t.Fatalf("button %d at width %d is not symmetric: %+v", i, width, button)
			}
			if button.x < 4 || button.x+button.width > 4+width {
				t.Fatalf("button %d is outside the dialog", i)
			}
			if i > 0 && buttons[i-1].x+buttons[i-1].width >= button.x {
				t.Fatalf("buttons overlap at dialog width %d", width)
			}
		}
	}
}

// TestRemoteReviewShortcutLabelsMatchActions checks the new modal shortcuts.
func TestRemoteReviewShortcutLabelsMatchActions(t *testing.T) {
	s, review := remoteReviewFixture(t)
	defer s.CLI.Cipher.Close()
	a := New(s)
	a.showRemoteReview(review)
	hints := strings.Join(a.hints(), " ")
	for _, hint := range []string{"s soft sync", "h hard sync", "a abort sync"} {
		if !strings.Contains(hints, hint) {
			t.Fatalf("missing shortcut hint %q in %q", hint, hints)
		}
	}
	if err := a.char('h'); err != nil {
		t.Fatal(err)
	}
	if s.Popup != nil || len(s.ImportedChanges) != 1 || s.Items[0].Name() != "Remote name" {
		t.Fatal("hard sync should import server state and discard conflicting edits")
	}
}
