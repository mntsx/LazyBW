package ui

import (
	"strings"
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestPushClearsOnlyEditedGroundPaths preserves unrelated remote indicators.
func TestPushClearsOnlyEditedGroundPaths(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	id := s.Items[0].ID()
	original := vault.Clone(s.Originals[id])
	incoming := vault.Clone(original)
	incoming["name"] = "Remote name"
	incoming["favorite"] = true
	s.InitialGrounds = map[string]map[string]any{id: original}
	s.Originals[id] = vault.Clone(incoming)
	prior := vault.Clone(incoming)
	edited := vault.Clone(incoming)
	edited["name"] = "Local name"
	remote := vault.Clone(edited)
	// CommitRemote installs the confirmed remote state before acknowledgement.
	s.Originals[id] = vault.Clone(remote)
	s.acknowledgePushedGround(id, id, prior, edited, remote)
	if s.GroundFieldChanged(id, []any{"name"}) {
		t.Fatal("pushed field kept its remote ground indicator")
	}
	if !s.GroundFieldChanged(id, []any{"favorite"}) {
		t.Fatal("unrelated remote ground indicator disappeared")
	}
}

// TestFolderHintsAdvertiseUnselect distinguishes an applied active filter.
func TestFolderHintsAdvertiseUnselect(t *testing.T) {
	s := setup(t)
	defer s.CLI.Cipher.Close()
	s.Focus = PaneFolders
	s.Filters[2] = 0
	s.FilterFolder = 0
	a := New(s)
	if !strings.Contains(strings.Join(a.hints(), " "), "ENTER unselect") {
		t.Fatal("active folder must advertise ENTER unselect")
	}
	s.FilterFolder = -1
	if !strings.Contains(strings.Join(a.hints(), " "), "ENTER select") {
		t.Fatal("inactive folder must advertise ENTER select")
	}
}
