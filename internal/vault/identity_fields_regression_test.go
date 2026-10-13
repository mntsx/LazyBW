package vault

import "testing"

// TestIdentityFieldOrderAndVisibility verifies Bitwarden identity field mapping.
func TestIdentityFieldOrderAndVisibility(t *testing.T) {
	item := &Item{Raw: map[string]any{
		"type": Identity, "identity": map[string]any{},
	}}
	fields := item.Fields()
	want := []string{
		"Title", "First name", "Middle name", "Last name", "Username",
		"Company", "National Insurance number", "Passport number",
		"License number", "Email", "Phone", "Address 1", "Address 2",
		"Address 3", "City / town", "County", "Postal code", "Country",
		"Notes", "Master password reprompt",
	}
	if len(fields) != len(want) {
		t.Fatalf("field count: got %d, want %d", len(fields), len(want))
	}
	for index, name := range want {
		if fields[index].Name != name {
			t.Errorf("field %d: got %q, want %q", index, fields[index].Name, name)
		}
	}
	for _, field := range fields {
		wantHidden := field.Name == "National Insurance number" ||
			field.Name == "Passport number"
		if field.Hidden != wantHidden {
			t.Errorf("hidden flag for %q: got %v", field.Name, field.Hidden)
		}
	}
	if fields[13].Path[1] != "address3" || fields[15].Path[1] != "state" {
		t.Fatal("address3 and county must use the native Bitwarden keys")
	}
}
