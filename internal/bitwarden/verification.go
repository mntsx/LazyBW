package bitwarden

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

var ErrRemoteChanged = errors.New("remote vault differs from loaded state")

// remoteDataError describes a verification failure without including server data.
// Its fields must be fixed labels, never raw item IDs, names or values.
type remoteDataError struct {
	entity string
	reason string
}

func (e *remoteDataError) Error() string {
	return "remote verification " + e.entity + ": " + e.reason
}

// RemoteDifference describes one external item or folder change, without plaintext secrets.
type RemoteDifference struct {
	Entity   string
	ID       string
	Name     string
	TypeName string
	Kind     vault.Kind
	Before   map[string]any
	After    map[string]any
}

// RemoteReview returns the synced snapshot and the differences from the loaded baseline.
type RemoteReview struct {
	Items       map[string]map[string]any
	Folders     map[string]map[string]any
	Differences []RemoteDifference
}

// CompareRemoteState synchronizes Bitwarden and captures changed items and folders.
// A failed sync or read returns an error and must prevent remote writes.
func (c *Client) CompareRemoteState(
	ctx context.Context,
	originalItems, originalFolders map[string]map[string]any,
) (*RemoteReview, error) {
	review, err := c.SyncedSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	review.Differences = append(review.Differences, remoteDifferences(
		"folder", originalFolders, review.Folders,
	)...)
	review.Differences = append(review.Differences, remoteDifferences(
		"item", originalItems, review.Items,
	)...)
	return review, nil
}

// SyncedSnapshot reads the canonical CLI lists after a server synchronization.
// The response from bw edit/create is not always identical to later bw list.
func (c *Client) SyncedSnapshot(ctx context.Context) (*RemoteReview, error) {
	if _, err := c.ses(ctx, "sync"); err != nil {
		return nil, fmt.Errorf("remote verification sync: %w", err)
	}
	var items, folders map[string]map[string]any
	var itemErr, folderErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		items, itemErr = c.verificationList(ctx, "items", true)
	}()
	go func() {
		defer wg.Done()
		folders, folderErr = c.verificationList(ctx, "folders", false)
	}()
	wg.Wait()
	if itemErr != nil {
		return nil, itemErr
	}
	if folderErr != nil {
		return nil, folderErr
	}
	return &RemoteReview{Items: items, Folders: folders}, nil
}

// VerifyRemoteState retains the existing guard for callers that only need equality.
func (c *Client) VerifyRemoteState(
	ctx context.Context,
	originalItems, originalFolders map[string]map[string]any,
) error {
	review, err := c.CompareRemoteState(ctx, originalItems, originalFolders)
	if err != nil {
		return err
	}
	if len(review.Differences) != 0 {
		return ErrRemoteChanged
	}
	return nil
}

// remoteDifferences returns stable, complete metadata changes without revealing secrets.
func remoteDifferences(entity string, before, after map[string]map[string]any) []RemoteDifference {
	ids := make(map[string]struct{}, len(before)+len(after))
	for id := range before {
		ids[id] = struct{}{}
	}
	for id := range after {
		ids[id] = struct{}{}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	result := make([]RemoteDifference, 0)
	for _, id := range ordered {
		previous := before[id]
		current := after[id]
		cleanBefore := vault.Clone(previous)
		if cleanBefore != nil {
			delete(cleanBefore, "_owner_name")
		}
		if previous != nil && current != nil && vault.EqualContent(cleanBefore, current) {
			continue
		}
		kind := vault.Modified
		if previous == nil {
			kind = vault.New
		}
		if current == nil {
			kind = vault.Deleted
		}
		display := current
		if display == nil {
			display = previous
		}
		name, _ := display["name"].(string)
		if name == "" {
			name = "(unnamed)"
		}
		typeName := "Folder"
		if entity == "item" {
			typeName = vault.TypeNames[itemType(display)]
		}
		result = append(result, RemoteDifference{
			Entity: entity, ID: id, Name: name, TypeName: typeName,
			Kind: kind, Before: cloneIfPresent(previous), After: cloneIfPresent(current),
		})
	}
	return result
}

// itemType reads Bitwarden's numeric item type without interpreting secrets.
func itemType(raw map[string]any) int {
	switch v := raw["type"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

// decodeValidatedList applies the same strict structure checks on initial
// loading and remote verification. Only an actual empty array is an empty vault.
func decodeValidatedList(raw []byte, entity string) ([]map[string]any, error) {
	values, err := decodeList(raw, "bw list "+entity)
	if err != nil {
		return nil, err
	}
	if values == nil {
		return nil, &remoteDataError{entity: entity, reason: "null list"}
	}
	result := make([]map[string]any, 0, len(values))
	ids := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == nil {
			return nil, &remoteDataError{entity: entity, reason: "null entry"}
		}
		if entity == "folders" && isVirtualNoFolder(value) {
			continue
		}
		id, ok := value["id"].(string)
		if !ok || id == "" {
			return nil, &remoteDataError{entity: entity, reason: "missing ID"}
		}
		if _, exists := ids[id]; exists {
			return nil, &remoteDataError{entity: entity, reason: "duplicate ID"}
		}
		ids[id] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

// verificationList loads remote entries and seals secrets immediately after JSON decoding.
func (c *Client) verificationList(
	ctx context.Context, entity string, sealItems bool,
) (map[string]map[string]any, error) {
	raw, err := c.ses(ctx, "list", entity)
	if err != nil {
		return nil, fmt.Errorf("remote verification %s: %w", entity, err)
	}
	values, err := decodeValidatedList(raw, entity)
	for i := range raw {
		raw[i] = 0
	}
	if err != nil {
		return nil, err
	}
	result := make(map[string]map[string]any, len(values))
	for _, value := range values {
		id := value["id"].(string)
		if sealItems {
			if err := vault.SealItem(value, c.Cipher); err != nil {
				// The original error is deliberately not included: only a fixed
				// category is surfaced, never any raw secret from the response.
				return nil, &remoteDataError{entity: entity, reason: "secret protection unavailable"}
			}
		}
		result[id] = value
	}
	return result, nil
}

// cloneIfPresent preserves nil for added or removed entities.
func cloneIfPresent(v map[string]any) map[string]any {
	if v == nil {
		return nil
	}
	return vault.Clone(v)
}

// VerificationFailure explains which CLI step failed without exposing output
// from Bitwarden (which may contain account or secret data).
func VerificationFailure(err error) string {
	if err == nil {
		return ""
	}
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		step := "Bitwarden CLI"
		switch cliErr.Operation {
		case "bw sync":
			step = "bw sync"
		case "bw list items":
			step = "bw list items"
		case "bw list folders":
			step = "bw list folders"
		}
		return fmt.Sprintf("%s failed (%s, exit %d)", step, cliErr.Category, cliErr.Code)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Remote verification timed out"
	}
	var dataErr *remoteDataError
	if errors.As(err, &dataErr) {
		step := "bw list " + dataErr.entity
		switch dataErr.reason {
		case "null list":
			return step + " returned null instead of a list"
		case "null entry":
			return step + " returned an empty entry"
		case "missing ID":
			return step + " returned an entry without an ID"
		case "duplicate ID":
			return step + " returned duplicate IDs"
		case "secret protection unavailable":
			return "Unable to protect remote secrets in memory"
		}
	}
	return "Remote verification failed due to an unexpected internal error"
}
