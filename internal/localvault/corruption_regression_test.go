package localvault

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestBackupChecksumDistinguishesAccidentalCorruption checks integrity errors.
func TestBackupChecksumDistinguishesAccidentalCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), Filename)
	protector, err := vault.NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	defer protector.Close()
	session, err := NewSession("original password", protector)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	data, err := session.Encode(Snapshot{Items: []map[string]any{
		{"name": "test item"},
	}}, "us", AESGCM, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Decode(path, "wrong password", AESGCM); err == nil ||
		errors.Is(err, ErrCorrupted) {
		t.Fatalf("wrong password was classified as corrupted: %v", err)
	}
	var record file
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	// Always corrupt the base64 text, even when its first character is A.
	replacement := "A"
	if record.Ciphertext[0] == 'A' {
		replacement = "B"
	}
	record.Ciphertext = replacement + record.Ciphertext[1:]
	damaged, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Decode(path, "original password", AESGCM); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("modified backup not classified as corrupted: %v", err)
	}
}
