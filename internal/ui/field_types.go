package ui

import "github.com/miguelmnts/lazybw-go/internal/vault"

// customFieldOption describes a field-picker option. Website is a special
// login-only operation that appends a URI to login.uris, not a custom field.
type customFieldOption struct {
	Name          string
	Icon          string
	BitwardenType int
	InitialValue  string
}

var customFieldOptions = schemaCustomFieldOptions()

// schemaCustomFieldOptions builds the field picker from the Bitwarden dictionary.
func schemaCustomFieldOptions() []customFieldOption {
	result := make([]customFieldOption, 0)
	for _, entry := range vault.Schema().CustomFieldTypes {
		result = append(result, customFieldOption{
			Name: entry.Name, Icon: entry.Icon,
			BitwardenType: entry.ID, InitialValue: entry.InitialValue,
		})
	}
	return result
}

var websiteFieldOption = customFieldOption{
	Name:          vault.Schema().WebsiteField.Name,
	Icon:          vault.Schema().WebsiteField.Icon,
	BitwardenType: vault.Schema().WebsiteField.ID,
}

// fieldOptions returns the options actually supported by the selected item.
func fieldOptions(item *vault.Item) []customFieldOption {
	options := make([]customFieldOption, 0, len(customFieldOptions)+1)
	for _, option := range customFieldOptions {
		if option.BitwardenType == 3 && (item == nil || item.Type() != vault.Login) {
			continue
		}
		options = append(options, option)
	}
	if item != nil && item.Type() == vault.Login {
		options = append(options, websiteFieldOption)
	}
	return options
}
