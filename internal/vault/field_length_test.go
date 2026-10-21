package vault

import (
	"strings"
	"testing"
)

func TestFieldDefinitionsCarryBitwardenEncryptedLimits(t *testing.T) {
	limits := map[int]map[string]int{
		Login: {
			"login.username": 1000, "login.password": 5000, "login.totp": 1000,
			"login.uris": 10000, "notes": 10000,
		},
		SecureNote: {"notes": 10000},
		Card: {
			"card.cardholderName": 1000, "card.brand": 1000, "card.number": 1000,
			"card.expMonth": 1000, "card.expYear": 1000, "card.code": 1000,
			"notes": 10000,
		},
		Identity: {
			"identity.title": 1000, "identity.firstName": 1000, "identity.middleName": 1000,
			"identity.lastName": 1000, "identity.username": 1000, "identity.company": 1000,
			"identity.ssn": 1000, "identity.passportNumber": 1000, "identity.licenseNumber": 1000,
			"identity.email": 1000, "identity.phone": 1000, "identity.address1": 1000,
			"identity.address2": 1000, "identity.address3": 1000, "identity.city": 1000,
			"identity.state": 1000, "identity.postalCode": 1000, "identity.country": 1000,
			"notes": 10000,
		},
		SSHKey: {
			"sshKey.privateKey": 5000, "sshKey.publicKey": 5000,
			"sshKey.fingerprint": 1000, "notes": 10000,
		},
	}
	for itemType, expected := range limits {
		definitions := FieldDefinitions(itemType)
		if len(definitions) != len(expected) {
			t.Fatalf("item type %d field count: got %d, want %d", itemType, len(definitions), len(expected))
		}
		for _, definition := range definitions {
			path := ""
			for index, part := range definition.Path {
				if index > 0 {
					path += "."
				}
				path += part
			}
			if got, want := definition.EncryptedLimit, expected[path]; got != want {
				t.Errorf("item type %d %s encrypted limit: got %d, want %d", itemType, path, got, want)
			}
		}
	}
}

func TestDynamicTextFieldsCarryEncryptedLimits(t *testing.T) {
	if got := CustomFieldEncryptedLimit(0); got != 5000 {
		t.Fatalf("text custom field limit: got %d, want 5000", got)
	}
	if got := CustomFieldEncryptedLimit(1); got != 5000 {
		t.Fatalf("hidden custom field limit: got %d, want 5000", got)
	}
	if got := CustomFieldEncryptedLimit(2); got != 0 {
		t.Fatalf("checkbox custom field limit: got %d, want 0", got)
	}
	if got := CustomFieldEncryptedLimit(3); got != 0 {
		t.Fatalf("linked custom field limit: got %d, want 0", got)
	}
	if got := Schema().WebsiteField.EncryptedLimit; got != 10000 {
		t.Fatalf("website limit: got %d, want 10000", got)
	}

	item := NewItem(Login, "example", "id", nil)
	item.Raw["fields"] = []any{
		map[string]any{"name": "Text", "type": 0, "value": ""},
		map[string]any{"name": "Hidden", "type": 1, "value": ""},
		map[string]any{"name": "Checkbox", "type": 2, "value": "false"},
		map[string]any{"name": "Linked", "type": 3, "linkedId": 100},
	}
	item.Raw["login"].(map[string]any)["uris"] = []any{map[string]any{"uri": "https://example.test"}}

	fields := item.Fields()
	limits := map[string]int{}
	for _, field := range fields {
		limits[field.Name] = field.EncryptedLimit
	}
	for name, want := range map[string]int{
		"Websites": 10000, "Text": 5000, "Hidden": 5000, "Checkbox": 0, "Linked": 0,
	} {
		if got := limits[name]; got != want {
			t.Errorf("%s encrypted limit: got %d, want %d", name, got, want)
		}
	}
}

func TestOverlongFieldsUsePlaintextByteCapacities(t *testing.T) {
	item := NewItem(Login, "example", "id", nil)
	login := item.Raw["login"].(map[string]any)
	login["username"] = strings.Repeat("a", 687)
	if item.HasOverlongField() {
		t.Fatal("username at its UTF-8 byte capacity is overlong")
	}
	login["username"] = strings.Repeat("a", 688)
	if !item.HasOverlongField() {
		t.Fatal("username exceeding its UTF-8 byte capacity was not detected")
	}
	login["username"] = ""
	login["uris"] = []any{map[string]any{"uri": strings.Repeat("a", 7440)}}
	if !item.HasOverlongField() {
		t.Fatal("overlong individual website URI was not detected")
	}
}

func TestNonContentEditableTextLimitsAreDeclared(t *testing.T) {
	limits := Schema().EditableTextLimits
	if limits.ItemName != 1000 || limits.CustomFieldName != 1000 || limits.FolderName != 1000 ||
		limits.AttachmentFilename != 1000 || limits.AttachmentKey != 1000 {
		t.Fatalf("unexpected non-content limits: %#v", limits)
	}
}
