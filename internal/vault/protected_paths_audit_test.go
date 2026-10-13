package vault

import "testing"

// TestLicenseNumberRespectsHiddenSecretWritePermission closes a missing guard.
func TestLicenseNumberRespectsHiddenSecretWritePermission(t *testing.T) {
	before := map[string]any{
		"id": "id", "organizationId": "org", "edit": true,
		"viewPassword": false,
		"identity":     map[string]any{"licenseNumber": "protected", "firstName": "Alice"},
	}
	after := Clone(before)
	after["identity"].(map[string]any)["licenseNumber"] = "changed"
	if CanPushItemChange(before, after) {
		t.Fatal("hidden license number cannot be modified without secret access")
	}
	after = Clone(before)
	after["identity"].(map[string]any)["firstName"] = "Bob"
	if !CanPushItemChange(before, after) {
		t.Fatal("visible identity fields should remain editable")
	}
}
