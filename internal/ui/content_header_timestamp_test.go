package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

const contentHeaderTimestampX = 22

func renderContentHeader(state *State) *Canvas {
	canvas := NewCanvas(60, 20)
	New(state).contentPanel(canvas, 0, 0, 60, 20)
	return canvas
}

func TestContentHeaderShowsRemoteRevisionInFieldGray(t *testing.T) {
	state := setup(t)
	state.Items[0].Raw["revisionDate"] = "2026-10-19T14:37:52.123Z"
	state.OpenItem()
	canvas := renderContentHeader(state)
	if row := stringCellRow(canvas, 0); !strings.Contains(row, "Last modified: 2026-10-19T14:37") {
		t.Fatalf("unexpected content header: %q", row)
	}
	cell := canvasAt(canvas, contentHeaderTimestampX, 0)
	if cell.FG != CWhite || !cell.Dim {
		t.Fatalf("remote timestamp does not match dim field content: %+v", cell)
	}
}

func TestContentHeaderShowsLocalCreationInGreen(t *testing.T) {
	state := setup(t)
	state.CreateItem(vault.Login, "Created locally")
	var createdID string
	for _, item := range state.Items {
		if item.Name() == "Created locally" {
			createdID = item.ID()
			break
		}
	}
	change := state.Stage.Get(vault.Key("item", createdID))
	if change == nil || change.ChangedAt.IsZero() {
		t.Fatal("new item has no local creation timestamp")
	}
	state.SelectedID = createdID
	state.Focus = PaneContent
	state.loadFields()
	canvas := renderContentHeader(state)
	expected := change.ChangedAt.Format(minuteTimestampLayout)
	if row := stringCellRow(canvas, 0); !strings.Contains(row, "Last modified: "+expected) {
		t.Fatalf("unexpected new-item header: %q", row)
	}
	cell := canvasAt(canvas, contentHeaderTimestampX, 0)
	if cell.FG != CGreen || cell.Dim {
		t.Fatalf("local creation timestamp is not green: %+v", cell)
	}

	created := state.SelectedItem()
	created.Raw["favorite"] = true
	state.Stage.Record("item", created.Raw, vault.TypeNames[created.Type()], state.Originals)
	updated := state.Stage.Get(vault.Key("item", createdID))
	if !updated.ChangedAt.Equal(change.ChangedAt) {
		t.Fatal("editing a new item replaced its local creation timestamp")
	}
}

func TestContentHeaderShowsLocalModificationInYellow(t *testing.T) {
	state := setup(t)
	state.Items[0].Raw["revisionDate"] = "2025-01-02T03:04:05Z"
	state.Items[0].Raw["name"] = "Locally modified"
	state.Stage.Record("item", state.Items[0].Raw, "Login", state.Originals)
	change := state.Stage.Get(vault.Key("item", state.Items[0].ID()))
	if change == nil || change.Kind != vault.Modified || change.ChangedAt.IsZero() {
		t.Fatal("modified item has no local modification timestamp")
	}
	state.OpenItem()
	canvas := renderContentHeader(state)
	expected := change.ChangedAt.Format(minuteTimestampLayout)
	if row := stringCellRow(canvas, 0); !strings.Contains(row, "Last modified: "+expected) {
		t.Fatalf("unexpected modified-item header: %q", row)
	}
	cell := canvasAt(canvas, contentHeaderTimestampX, 0)
	if cell.FG != CYellow || cell.Dim {
		t.Fatalf("local modification timestamp is not yellow: %+v", cell)
	}
}
