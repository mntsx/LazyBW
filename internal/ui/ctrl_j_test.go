package ui

import "testing"

func TestCtrlJInsertsNewlineInDetailEditor(t *testing.T) {
	s := setup(t)
	s.OpenItem()
	s.FieldIndex = 0
	if err := s.BeginTextEdit(); err != nil {
		t.Fatal(err)
	}
	s.Input = "beforeafter"
	s.Caret = len("before")
	a := New(s)

	handled, err := a.dispatchConfiguredKey("ctrl+j")
	if err != nil || !handled {
		t.Fatalf("Ctrl+J dispatch: handled=%t err=%v", handled, err)
	}
	if got, want := s.Input, "before\nafter"; got != want {
		t.Fatalf("Ctrl+J input: got %q, want %q", got, want)
	}
	if got, want := s.Caret, len("before\n"); got != want {
		t.Fatalf("Ctrl+J caret: got %d, want %d", got, want)
	}
}
