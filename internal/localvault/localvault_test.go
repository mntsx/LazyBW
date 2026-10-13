package localvault

import (
	"github.com/miguelmnts/lazybw-go/internal/vault"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSingleFileReplacesDifferentRegionAndPassword(t *testing.T) {
	dir := filepath.Join(t.TempDir(), Filename)
	protector, err := vault.NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	defer protector.Close()
	first, err := NewSession("first-password", protector)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{Items: []map[string]any{{"id": "one", "name": "private", "login": map[string]any{"password": "secret"}}}}
	encoded, err := first.Encode(snapshot, "us", AESGCM, time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteAtomic(dir, encoded); err != nil {
		t.Fatal(err)
	}
	if got, head, err := Decode(dir, "first-password", AESGCM); err != nil || head.Origin != "us" || len(got.Items) != 1 {
		t.Fatalf("first decode failed: %v %v", head, err)
	}
	next, err := NewSession("second-password", protector)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := next.Encode(Snapshot{Items: []map[string]any{{"id": "two"}}}, "eu", XChaCha, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteAtomic(dir, encrypted); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Decode(dir, "first-password", XChaCha); err == nil {
		t.Fatal("old password opened new copy")
	}
	got, header, err := Decode(dir, "second-password", XChaCha)
	if err != nil || header.Origin != "eu" || got.Items[0]["id"] != "two" {
		t.Fatalf("second decode failed: %v", err)
	}
	if _, _, err = Decode(dir, "second-password", AESGCM); err == nil || !strings.Contains(err.Error(), XChaCha) {
		t.Fatal("algorithm mismatch must explain required algorithm")
	}
	first.Close()
	next.Close()
	files, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != Filename {
		t.Fatalf("unexpected backup files: %v", files)
	}
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("unexpected permissions: %v %v", st, err)
	}
}

func TestHeaderTamperingIsRejected(t *testing.T) {
	dir := filepath.Join(t.TempDir(), Filename)
	protector, err := vault.NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	defer protector.Close()
	session, err := NewSession("test-password", protector)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := session.Encode(Snapshot{}, "us", AESGCM, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteAtomic(dir, encrypted); err != nil {
		t.Fatal(err)
	}
	altered := strings.Replace(string(encrypted), `"origin":"us"`, `"origin":"eu"`, 1)
	if altered == string(encrypted) {
		t.Fatal("header not found")
	}
	if err = os.WriteFile(FilePath(dir), []byte(altered), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Decode(dir, "test-password", AESGCM); err == nil {
		t.Fatal("modified metadata accepted")
	}
	session.Close()
}

func TestUnavailableIfDirectoryNotConfigured(t *testing.T) {
	if Available("") {
		t.Fatal("empty directory must not resolve to working directory")
	}
	if err := Validate("", AESGCM); err == nil {
		t.Fatal("empty path must be rejected")
	}
	if err := Validate(t.TempDir(), "invalid"); err == nil {
		t.Fatal("unknown encryption should be rejected")
	}
}
