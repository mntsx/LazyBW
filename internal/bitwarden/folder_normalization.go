package bitwarden

import "strings"

// isVirtualNoFolder identifies Bitwarden's synthetic unfiled-items entry.
// Real folders, including user-created folders called "No Folder", have IDs.
func isVirtualNoFolder(folder map[string]any) bool {
	if folder == nil {
		return false
	}
	if id, ok := folder["id"]; ok && id != nil && id != "" {
		return false
	}
	name, ok := folder["name"].(string)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "no folder", "items with no folder":
		return true
	default:
		return false
	}
}

// withoutVirtualNoFolder drops the virtual filter from a CLI folder snapshot.
// Both startup and remote verification must use the same normalization.
func withoutVirtualNoFolder(folders []map[string]any) []map[string]any {
	if folders == nil {
		return nil
	}
	result := make([]map[string]any, 0, len(folders))
	for _, folder := range folders {
		if !isVirtualNoFolder(folder) {
			result = append(result, folder)
		}
	}
	return result
}
