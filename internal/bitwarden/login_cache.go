package bitwarden

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const profileDataFile = "data.json"
const cachedLoginMetadataFile = ".lazybw.json"
const cachedLoginDisabledFile = ".disabled"

type cachedLoginMetadata struct {
	Generation string `json:"generation"`
	ClientID   string `json:"client_id"`
	UserID     string `json:"user_id"`
	DataHash   string `json:"data_hash"`
}

func (c *Client) ConfigureClientLoginCache(configDir string, enabled bool, clientID string) {
	c.mu.Lock()
	c.cacheConfigDir = configDir
	c.cacheClientLogin = enabled
	c.cacheExpectedClientID = clientID
	if c.loginGenerations == nil {
		c.loginGenerations = make(map[string]string)
	}
	c.mu.Unlock()
	if enabled {
		c.enableCachedLogins()
	} else {
		c.clearCachedLogins()
	}
}

func (c *Client) enableCachedLogins() {
	_, _, root, _ := c.profilePaths("us")
	if root == "" {
		return
	}
	directory := filepath.Join(root, "cached-logins")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return
	}
	lock, err := lockSession(directory, false)
	if err != nil {
		return
	}
	defer releaseSessionLock(lock)
	_ = os.Remove(filepath.Join(directory, cachedLoginDisabledFile))
}

func (c *Client) profilePaths(region string) (string, string, string, bool) {
	c.mu.Lock()
	base, enabled := c.cacheConfigDir, c.cacheClientLogin
	c.mu.Unlock()
	if base == "" {
		return "", "", "", false
	}
	root := filepath.Join(base, "bitwarden")
	return filepath.Join(root, "devices", region+".json"), filepath.Join(root, "cached-logins", region), root, enabled
}

func (c *Client) clearCachedLogins() {
	_, _, root, _ := c.profilePaths("us")
	if root == "" {
		return
	}
	directory := filepath.Join(root, "cached-logins")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return
	}
	lock, err := lockSession(directory, false)
	if err != nil {
		return
	}
	defer releaseSessionLock(lock)
	entries, err := os.ReadDir(directory)
	if err == nil {
		for _, entry := range entries {
			if entry.Name() != ".lock" {
				_ = os.RemoveAll(filepath.Join(directory, entry.Name()))
			}
		}
	}
	_ = writePrivateFile(filepath.Join(directory, cachedLoginDisabledFile), []byte("disabled\n"))
}

func (c *Client) restoreCachedLogin(ctx context.Context, profile, region string) (map[string]any, bool) {
	_, directory, root, enabled := c.profilePaths(region)
	c.mu.Lock()
	expectedID := c.cacheExpectedClientID
	c.mu.Unlock()
	if !enabled || expectedID == "" || root == "" {
		return nil, false
	}
	if err := os.MkdirAll(filepath.Join(root, "cached-logins"), 0700); err != nil {
		return nil, false
	}
	lock, err := lockSession(filepath.Join(root, "cached-logins"), false)
	if err != nil {
		return nil, false
	}
	defer releaseSessionLock(lock)
	if _, err := os.Stat(filepath.Join(root, "cached-logins", cachedLoginDisabledFile)); err == nil {
		return nil, false
	}
	metadata, data, err := readCachedLogin(directory)
	if err != nil || metadata.ClientID != expectedID || metadata.UserID != strings.TrimPrefix(expectedID, "user.") {
		return nil, false
	}
	if err := writePrivateFile(filepath.Join(profile, profileDataFile), data); err != nil {
		return nil, false
	}
	c.mu.Lock()
	c.loginGenerations[region] = metadata.Generation
	c.mu.Unlock()
	b, err := c.rawCLI(ctx, "status")
	if err != nil {
		_ = os.Remove(filepath.Join(profile, profileDataFile))
		return nil, false
	}
	status, err := decodeObject(b, "bw status")
	if err != nil || status["status"] != "locked" || fmt.Sprint(status["userId"]) != metadata.UserID {
		_ = os.Remove(filepath.Join(profile, profileDataFile))
		return nil, false
	}
	return status, true
}

func (c *Client) persistLockedLogin(ctx context.Context, clientID string, knownLocked bool) {
	c.mu.Lock()
	profile := c.profileDir
	region := c.profileRegions[profile]
	c.mu.Unlock()
	if profile == "" || region == "" || clientID == "" {
		return
	}
	userID := strings.TrimPrefix(clientID, "user.")
	if !knownLocked {
		b, err := c.rawCLI(ctx, "status")
		if err != nil {
			return
		}
		status, err := decodeObject(b, "bw status")
		if err != nil || status["status"] != "locked" || fmt.Sprint(status["userId"]) != userID {
			return
		}
		userID = fmt.Sprint(status["userId"])
	}
	data, err := readPrivateFile(filepath.Join(profile, profileDataFile))
	if err != nil {
		return
	}
	c.persistDevice(region, data)
	_, directory, root, enabled := c.profilePaths(region)
	if !enabled || root == "" {
		return
	}
	cacheRoot := filepath.Join(root, "cached-logins")
	if err := os.MkdirAll(cacheRoot, 0700); err != nil {
		return
	}
	lock, err := lockSession(cacheRoot, false)
	if err != nil {
		return
	}
	defer releaseSessionLock(lock)
	if _, err := os.Stat(filepath.Join(cacheRoot, cachedLoginDisabledFile)); err == nil {
		return
	}
	c.mu.Lock()
	expectedGeneration := c.loginGenerations[region]
	c.mu.Unlock()
	if existing, _, err := readCachedLogin(directory); err == nil && expectedGeneration != "" && existing.Generation != expectedGeneration {
		return
	}
	generation, err := randomGeneration()
	if err != nil {
		return
	}
	hash := sha256.Sum256(data)
	metadata := cachedLoginMetadata{
		Generation: generation, ClientID: clientID, UserID: userID,
		DataHash: hex.EncodeToString(hash[:]),
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return
	}
	if err := writePrivateFile(filepath.Join(directory, profileDataFile), data); err != nil {
		return
	}
	encoded, err := json.Marshal(metadata)
	if err != nil || writePrivateFile(filepath.Join(directory, cachedLoginMetadataFile), encoded) != nil {
		return
	}
	c.mu.Lock()
	c.loginGenerations[region] = generation
	c.mu.Unlock()
}

func readCachedLogin(directory string) (cachedLoginMetadata, []byte, error) {
	metadataData, err := readPrivateFile(filepath.Join(directory, cachedLoginMetadataFile))
	if err != nil {
		return cachedLoginMetadata{}, nil, err
	}
	var metadata cachedLoginMetadata
	if json.Unmarshal(metadataData, &metadata) != nil || metadata.Generation == "" || metadata.ClientID == "" || metadata.UserID == "" {
		return cachedLoginMetadata{}, nil, errors.New("invalid cached login metadata")
	}
	data, err := readPrivateFile(filepath.Join(directory, profileDataFile))
	if err != nil {
		return cachedLoginMetadata{}, nil, err
	}
	hash := sha256.Sum256(data)
	if metadata.DataHash == "" || metadata.DataHash != hex.EncodeToString(hash[:]) || !json.Valid(data) {
		return cachedLoginMetadata{}, nil, errors.New("invalid cached login data")
	}
	return metadata, data, nil
}

func readPrivateFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		if err == nil {
			err = errors.New("insecure cached file")
		}
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, 32<<20))
}

func writePrivateFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".lazybw-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func randomGeneration() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func (c *Client) persistDevice(region string, data []byte) {
	device, directory, _, _ := c.profilePaths(region)
	if device == "" {
		return
	}
	var raw any
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	sanitized, ok := onlyAppID(raw)
	if !ok {
		return
	}
	encoded, err := json.Marshal(sanitized)
	if err == nil {
		_ = writePrivateFile(device, encoded)
	}
	_ = directory
}

func onlyAppID(value any) (any, bool) {
	switch current := value.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, nested := range current {
			if key == "appId" {
				if id, ok := nested.(string); ok && id != "" {
					result[key] = id
				}
				continue
			}
			if child, ok := onlyAppID(nested); ok {
				result[key] = child
			}
		}
		return result, len(result) > 0
	case []any:
		return nil, false
	default:
		return nil, false
	}
}

func (c *Client) restoreDevice(profile, region string) {
	device, _, _, _ := c.profilePaths(region)
	if device == "" {
		return
	}
	data, err := readPrivateFile(device)
	if err != nil || !json.Valid(data) {
		return
	}
	// The CLI stores appId in its profile JSON. A device file contains only the
	// matching object path, so it never carries account data, tokens, or keys.
	_ = writePrivateFile(filepath.Join(profile, profileDataFile), data)
}
