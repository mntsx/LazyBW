package vault

import (
	"strings"
	"testing"
)

func TestEmbeddedBitwardenDictionaryMatchesFieldModel(t *testing.T) {
	schema := Schema()
	if schema.Version != 1 || len(schema.CreationOrder) != 5 ||
		schema.CreationOrder[0] != Login || schema.CreationOrder[1] != Card {
		t.Fatalf("unexpected item order: %v", schema.CreationOrder)
	}
	fields := FieldDefinitions(Identity)
	if len(fields) == 0 || fields[0].Name != "Title" ||
		strings.Join(FieldOptions(Identity, "Title"), ",") != "Mr,Mrs,Ms,Mx,Dr" {
		t.Fatalf("identity title options missing from JSON dictionary: %#v", fields)
	}
	if len(FieldOptions(Card, "Brand")) != 10 || len(schema.CustomFieldTypes) != 4 {
		t.Fatal("card or custom field metadata missing")
	}
	if TypeNames[Login] != "Login" || len(TypeIcons[Login]) == 0 {
		t.Fatal("item types not initialized from JSON dictionary")
	}
}
