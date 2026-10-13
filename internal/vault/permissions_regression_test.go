package vault

import "testing"

// TestItemPermissions checks explicit rights and fail-closed organization metadata.
func TestItemPermissions(t *testing.T) {
	cases := []struct {
		name       string
		raw        map[string]any
		edit, view bool
	}{
		{"personal", map[string]any{}, true, true},
		{"organization unknown", map[string]any{"organizationId": "org"}, false, false},
		{"view only", map[string]any{"organizationId": "org", "edit": false, "viewPassword": true}, false, true},
		{"edit no secrets", map[string]any{"organizationId": "org", "edit": true, "viewPassword": false}, true, false},
		{"fully editable", map[string]any{"organizationId": "org", "edit": true, "viewPassword": true}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := &Item{Raw: tc.raw}
			if item.CanEdit() != tc.edit || item.CanViewSecrets() != tc.view {
				t.Fatalf("permissions mismatch: edit %v, view %v", item.CanEdit(), item.CanViewSecrets())
			}
		})
	}
}

// TestHiddenPasswordPermissions prevents writes to protected fields during push.
func TestHiddenPasswordPermissions(t *testing.T) {
	before := map[string]any{
		"id": "1", "type": 1, "organizationId": "org", "edit": true,
		"viewPassword": false,
		"login":        map[string]any{"username": "a", "password": "secret"},
	}
	item := &Item{Raw: before}
	fields := item.Fields()
	if fields[0].Restricted || !fields[0].Editable {
		t.Fatal("visible username should remain editable")
	}
	if !fields[1].Restricted || fields[1].Editable {
		t.Fatal("password must be restricted")
	}
	visible := Clone(before)
	visible["login"].(map[string]any)["username"] = "b"
	if !CanPushItemChange(before, visible) {
		t.Fatal("visible edit incorrectly blocked")
	}
	secret := Clone(before)
	secret["login"].(map[string]any)["password"] = "changed"
	if CanPushItemChange(before, secret) {
		t.Fatal("protected edit incorrectly allowed")
	}
}

// TestCheckboxPermissions distinguishes switch permissions from text editing.
func TestCheckboxPermissions(t *testing.T) {
	cases := []struct {
		name    string
		edit    bool
		allowed bool
	}{
		{"editable", true, true},
		{"view only", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := &Item{Raw: map[string]any{
				"type":           Login,
				"organizationId": "org",
				"edit":           tc.edit,
				"viewPassword":   true,
				"fields": []any{map[string]any{
					"name": "Active", "type": 2, "value": "false",
				}},
			}}
			for _, field := range item.Fields() {
				if field.Name == "Active" {
					if field.Editable != tc.allowed {
						t.Fatalf("checkbox switch permission: got %v, want %v", field.Editable, tc.allowed)
					}
					return
				}
			}
			t.Fatal("checkbox field missing")
		})
	}
}
