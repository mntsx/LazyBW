package ui

import (
	"testing"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestOfflineRemoteCheckboxEdits checks staged switches without connectivity.
func TestOfflineRemoteCheckboxEdits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reprompt bool
	}{
		{name: "custom checkbox"},
		{name: "master password reprompt", reprompt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setup(t)
			item := s.Items[0]
			if !tc.reprompt {
				item.Raw["fields"] = []any{map[string]any{
					"name": "Enabled", "type": 2, "value": "false",
				}}
			}
			s.Originals[item.ID()] = vault.Clone(item.Raw)
			s.OpenItem()
			for i, field := range s.Fields {
				if tc.reprompt && field.Reprompt ||
					!tc.reprompt && field.Custom && field.FieldType == 2 {
					s.FieldIndex = i
					break
				}
			}
			s.ServerConnected = false
			a := New(s)
			if err := a.char('s'); err != nil {
				t.Fatal(err)
			}
			if s.Stage.Len() != 1 {
				t.Fatal("offline switch should produce a staged edit")
			}
		})
	}
}
