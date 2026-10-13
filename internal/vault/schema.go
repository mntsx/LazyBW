package vault

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed bitwarden_schema.json
var schemaSource embed.FS

type FieldDefinition struct {
	Name     string   `json:"name"`
	Path     []string `json:"path"`
	Hidden   bool     `json:"hidden,omitempty"`
	Websites bool     `json:"websites,omitempty"`
	Options  []string `json:"options,omitempty"`
}

type ItemTypeDefinition struct {
	ID     int               `json:"id"`
	Name   string            `json:"name"`
	Icon   string            `json:"icon"`
	Fields []FieldDefinition `json:"fields"`
}

type CustomFieldDefinition struct {
	Name         string `json:"name"`
	ID           int    `json:"id"`
	Icon         string `json:"icon"`
	InitialValue string `json:"initial_value"`
}

type LinkedTargetDefinition struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

type BitwardenSchema struct {
	Version          int                      `json:"schema_version"`
	ItemTypes        []ItemTypeDefinition     `json:"item_types"`
	CreationOrder    []int                    `json:"creation_order"`
	CustomFieldTypes []CustomFieldDefinition  `json:"custom_field_types"`
	WebsiteField     CustomFieldDefinition    `json:"website_field"`
	LinkedTargets    []LinkedTargetDefinition `json:"linked_targets"`
}

var schema = loadSchema()

// loadSchema validates the embedded Bitwarden type dictionary at startup.
func loadSchema() BitwardenSchema {
	data, err := schemaSource.ReadFile("bitwarden_schema.json")
	if err != nil {
		panic(err)
	}
	var result BitwardenSchema
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}
	if result.Version != 1 {
		panic(fmt.Sprintf("unsupported Bitwarden schema version %d", result.Version))
	}
	seen := make(map[int]bool)
	for _, typ := range result.ItemTypes {
		if typ.ID <= 0 || typ.Name == "" || seen[typ.ID] {
			panic("invalid Bitwarden item type")
		}
		seen[typ.ID] = true
		for _, f := range typ.Fields {
			if f.Name == "" || len(f.Path) == 0 {
				panic("invalid Bitwarden field")
			}
		}
	}
	for _, id := range result.CreationOrder {
		if !seen[id] {
			panic("unknown creation type")
		}
	}
	return result
}

// Schema returns the built-in schema; callers must not modify it.
func Schema() BitwardenSchema { return schema }

// FieldDefinitions returns the fields for one Bitwarden item type.
func FieldDefinitions(itemType int) []FieldDefinition {
	for _, typ := range schema.ItemTypes {
		if typ.ID == itemType {
			return typ.Fields
		}
	}
	return nil
}

// FieldOptions returns the selectable values of a default item field.
func FieldOptions(itemType int, name string) []string {
	for _, f := range FieldDefinitions(itemType) {
		if f.Name == name {
			return append([]string(nil), f.Options...)
		}
	}
	return nil
}

func schemaTypeNames() map[int]string {
	names := make(map[int]string)
	for _, typ := range schema.ItemTypes {
		names[typ.ID] = typ.Name
	}
	return names
}
func schemaTypeIcons() map[int]string {
	icons := make(map[int]string)
	for _, typ := range schema.ItemTypes {
		icons[typ.ID] = typ.Icon
	}
	return icons
}
