package localvault

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miguelmnts/lazybw-go/internal/vault"
)

// TestAESGCMSIVRFC8452 verifies the AES-256-GCM-SIV implementation against RFC 8452.
func TestAESGCMSIVRFC8452(t *testing.T) {
	key, err := hex.DecodeString("01000000000000000000000000000000" +
		"00000000000000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := hex.DecodeString("030000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	aead, err := aeadFor(AESGCMSIV, key)
	if err != nil {
		t.Fatal(err)
	}
	if aead.NonceSize() != 12 || aead.Overhead() != 16 {
		t.Fatalf("unexpected GCM-SIV parameters: nonce=%d tag=%d",
			aead.NonceSize(), aead.Overhead())
	}
	cases := []struct {
		plaintext string
		expected  string
	}{
		{"", "07f5f4169bbf55a8400cd47ea6fd400f"},
		{"0100000000000000", "c2ef328e5c71c83b843122130f7364b761e0b97427e3df28"},
	}
	for _, tt := range cases {
		plaintext, err := hex.DecodeString(tt.plaintext)
		if err != nil {
			t.Fatal(err)
		}
		actual := aead.Seal(nil, nonce, plaintext, nil)
		if hex.EncodeToString(actual) != tt.expected {
			t.Fatalf("RFC 8452 vector mismatch: got %x; expected %s", actual, tt.expected)
		}
		decrypted, err := aead.Open(nil, nonce, actual, nil)
		if err != nil || !bytes.Equal(decrypted, plaintext) {
			t.Fatalf("RFC 8452 round trip failed: %v", err)
		}
	}
}

// TestAESGCMSIVLocalVaultBackup checks full backup, header and tamper detection.
func TestAESGCMSIVLocalVaultBackup(t *testing.T) {
	if !ValidAlgorithm(AESGCMSIV) {
		t.Fatal("AES-256-GCM-SIV not accepted")
	}
	protector, err := vault.NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	defer protector.Close()
	session, err := NewSession("a strong master password", protector)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	path := filepath.Join(t.TempDir(), Filename)
	now := time.Date(2026, time.October, 11, 12, 0, 0, 0, time.UTC)
	source := Snapshot{
		Items: []map[string]any{{"id": "login", "login": map[string]any{
			"password": "a sensitive password", "uris": []string{"https://example.org"}}}},
		Folders: []map[string]any{{"id": "folder"}},
	}
	encrypted, err := session.Encode(source, "eu", AESGCMSIV, now)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("a sensitive password")) {
		t.Fatal("sensitive plaintext present in encrypted file")
	}
	if err := WriteAtomic(path, encrypted); err != nil {
		t.Fatal(err)
	}
	restored, head, err := Decode(path, "a strong master password", AESGCMSIV)
	if err != nil || head.Algorithm != AESGCMSIV || head.Origin != "eu" ||
		len(restored.Items) != 1 || len(restored.Folders) != 1 {
		t.Fatalf("unable to read GCM-SIV backup: %#v %#v %v", restored, head, err)
	}
	if _, _, err := Decode(path, "a strong master password", AESGCM); err == nil ||
		!strings.Contains(err.Error(), AESGCMSIV) {
		t.Fatalf("algorithm mismatch not explained: %v", err)
	}
	if _, _, err := Decode(path, "incorrect password", AESGCMSIV); err == nil ||
		errors.Is(err, ErrCorrupted) {
		t.Fatalf("incorrect password handling changed: %v", err)
	}
	tampered := bytes.Replace(encrypted, []byte(`"origin":"eu"`),
		[]byte(`"origin":"us"`), 1)
	if bytes.Equal(encrypted, tampered) {
		t.Fatal("failed to build corrupted fixture")
	}
	if err := os.WriteFile(path, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Decode(path, "a strong master password", AESGCMSIV); !errors.Is(err, ErrCorrupted) {
		t.Fatalf("corrupted header was not detected: %v", err)
	}
}

// TestBackupAlgorithmsRemainIndependent ensures the original algorithms remain available.
func TestBackupAlgorithmsRemainIndependent(t *testing.T) {
	for _, algorithm := range []string{AESGCM, XChaCha, AESGCMSIV} {
		t.Run(algorithm, func(t *testing.T) {
			key := bytes.Repeat([]byte{0x01}, 32)
			aead, err := aeadFor(algorithm, key)
			if err != nil {
				t.Fatal(err)
			}
			nonce := make([]byte, aead.NonceSize())
			payload := []byte("vault content")
			aad := []byte("authenticated header")
			ciphertext := aead.Seal(nil, nonce, payload, aad)
			plain, err := aead.Open(nil, nonce, ciphertext, aad)
			if err != nil || !bytes.Equal(plain, payload) {
				t.Fatalf("roundtrip failed: %v", err)
			}
			ciphertext[0] ^= 1
			if _, err := aead.Open(nil, nonce, ciphertext, aad); err == nil {
				t.Fatal("tampered ciphertext accepted")
			}
			if _, err := aeadFor(algorithm, key[:31]); err == nil {
				t.Fatal("non-256-bit key accepted")
			}
		})
	}
}

// TestAESGCMSIVReplacesPreviousBackup checks algorithm and password rotation.
func TestAESGCMSIVReplacesPreviousBackup(t *testing.T) {
	protector, err := vault.NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	defer protector.Close()
	path := filepath.Join(t.TempDir(), Filename)
	previous, err := NewSession("old-password", protector)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := previous.Encode(
		Snapshot{Items: []map[string]any{{"name": "old"}}},
		"us", AESGCM, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, encrypted); err != nil {
		t.Fatal(err)
	}
	previous.Close()

	updated, err := NewSession("new-password", protector)
	if err != nil {
		t.Fatal(err)
	}
	defer updated.Close()
	encrypted, err = updated.Encode(
		Snapshot{Items: []map[string]any{{"name": "new"}}},
		"eu", AESGCMSIV, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, encrypted); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Decode(path, "old-password", AESGCMSIV); err == nil {
		t.Fatal("old master password still unlocks overwritten file")
	}
	data, header, err := Decode(path, "new-password", AESGCMSIV)
	if err != nil || header.Algorithm != AESGCMSIV ||
		header.Origin != "eu" || len(data.Items) != 1 ||
		data.Items[0]["name"] != "new" {
		t.Fatalf("wrong replacement contents: %#v %#v %v", data, header, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != Filename {
		t.Fatalf("unexpected files after replacement: %#v", entries)
	}
}
