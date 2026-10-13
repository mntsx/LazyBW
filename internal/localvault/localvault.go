// Package localvault stores authenticated, password-protected offline snapshots.
package localvault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ericlagergren/siv"
	"github.com/miguelmnts/lazybw-go/internal/vault"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	Filename     = "bw-vault"
	AESGCM       = "aes-256-gcm"
	AESGCMSIV    = "aes-256-gcm-siv"
	XChaCha      = "xchacha20-poly1305"
	maxFileSize  = 128 << 20
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 2
)

// Snapshot contains only personal items and folders confirmed on the server.
type Snapshot struct {
	Items   []map[string]any `json:"items"`
	Folders []map[string]any `json:"folders"`
}

type Header struct {
	Version   int    `json:"version"`
	Origin    string `json:"origin"`
	Created   string `json:"created"`
	Algorithm string `json:"algorithm"`
	KDF       string `json:"kdf"`
	Salt      string `json:"salt"`
	Memory    uint32 `json:"memory"`
	Time      uint32 `json:"time"`
	Threads   uint8  `json:"threads"`
}

type file struct {
	Header     Header `json:"header"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	Checksum   string `json:"checksum,omitempty"`
}

// ErrCorrupted means the encrypted file failed structural or checksum validation.
// Authentication failure alone cannot distinguish a wrong password from tampering.
var ErrCorrupted = errors.New("local vault file is corrupted")

// addChecksum covers the entire serialized record excluding the checksum itself.
func addChecksum(record *file) error {
	record.Checksum = ""
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	record.Checksum = base64.StdEncoding.EncodeToString(digest[:])
	return nil
}

// verifyChecksum rejects accidental edits before deriving the password key.
// Older version-one files without a checksum remain readable.
func verifyChecksum(record file) error {
	if record.Checksum == "" {
		if record.Header.Version >= 2 {
			return ErrCorrupted
		}
		return nil
	}
	expected, err := base64.StdEncoding.DecodeString(record.Checksum)
	if err != nil || len(expected) != sha256.Size {
		return ErrCorrupted
	}
	record.Checksum = ""
	data, err := json.Marshal(record)
	if err != nil {
		return ErrCorrupted
	}
	actual := sha256.Sum256(data)
	if subtle.ConstantTimeCompare(expected, actual[:]) != 1 {
		return ErrCorrupted
	}
	return nil
}

// Session stores the derived key for a single authenticated online session.
// Call Close as soon as the session ends.
type Session struct {
	key       *vault.Sealed
	protector *vault.Cipher
	salt      [16]byte
	closed    bool
}

// ValidAlgorithm reports whether the algorithm can encrypt a local snapshot.
func ValidAlgorithm(name string) bool {
	return name == AESGCM || name == AESGCMSIV || name == XChaCha
}

// FilePath expands the configured full path of the backup file.
func FilePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

// Available verifies that there is a regular, readable local vault file.
func Available(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	info, err := os.Lstat(FilePath(dir))
	return err == nil && info.Mode().IsRegular()
}

// Validate checks a backup destination before the vault is unlocked.
func Validate(dir, algorithm string) error {
	if !ValidAlgorithm(algorithm) {
		return fmt.Errorf("unsupported local encryption algorithm: %s", algorithm)
	}
	if strings.TrimSpace(dir) == "" {
		return errors.New("vault_path is empty")
	}
	path := FilePath(dir)
	info, err := os.Stat(filepath.Dir(path))
	if err != nil || !info.IsDir() {
		return errors.New("vault_path parent directory does not exist")
	}
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() {
			return errors.New("bw-vault is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot access bw-vault")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".bw-vault-check-*")
	if err != nil {
		return errors.New("vault_path is not writable")
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// NewSession derives an ephemeral encryption key from the master password.
func NewSession(password string, protector *vault.Cipher) (*Session, error) {
	if protector == nil {
		return nil, errors.New("missing session cipher")
	}
	s := &Session{protector: protector}
	if _, err := io.ReadFull(rand.Reader, s.salt[:]); err != nil {
		return nil, err
	}
	key := argon2.IDKey([]byte(password), s.salt[:], argonTime, argonMemory, argonThreads, 32)
	defer zero(key)
	encrypted, err := protector.Seal(string(key))
	if err != nil {
		return nil, err
	}
	s.key = encrypted.(*vault.Sealed)
	return s, nil
}

// Close overwrites the session key and salt where practical.
func (s *Session) Close() {
	if s == nil {
		return
	}
	if s.key != nil {
		zero(s.key.Nonce)
		zero(s.key.Data)
		s.key = nil
	}
	for i := range s.salt {
		s.salt[i] = 0
	}
	s.closed = true
}

func aeadFor(algorithm string, key []byte) (cipher.AEAD, error) {
	switch algorithm {
	case AESGCM:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	case AESGCMSIV:
		if len(key) != 32 {
			return nil, errors.New("AES-256-GCM-SIV requires a 32-byte key")
		}
		return siv.NewGCM(key)
	case XChaCha:
		return chacha20poly1305.NewX(key)
	default:
		return nil, errors.New("unsupported local encryption algorithm")
	}
}

func headerAAD(h Header) ([]byte, error) { return json.Marshal(h) }

// Encode encrypts a complete, coherent snapshot with authenticated metadata.
func (s *Session) Encode(snapshot Snapshot, origin, algorithm string, now time.Time) ([]byte, error) {
	if s == nil || s.closed || s.key == nil || s.protector == nil {
		return nil, errors.New("local vault key unavailable")
	}
	rawKey, err := s.protector.Open(s.key)
	if err != nil {
		return nil, err
	}
	key := []byte(rawKey)
	defer zero(key)
	if origin != "us" && origin != "eu" {
		return nil, errors.New("invalid vault origin")
	}
	a, err := aeadFor(algorithm, key)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	defer zero(data)
	salt := base64.StdEncoding.EncodeToString(s.salt[:])
	head := Header{Version: 2, Origin: origin, Created: now.UTC().Format(time.RFC3339),
		Algorithm: algorithm, KDF: "argon2id", Salt: salt,
		Memory: argonMemory, Time: argonTime, Threads: argonThreads}
	aad, err := headerAAD(head)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	encrypted := a.Seal(nil, nonce, data, aad)
	record := file{Header: head, Nonce: base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(encrypted)}
	if err := addChecksum(&record); err != nil {
		zero(encrypted)
		return nil, err
	}
	result, err := json.Marshal(record)
	zero(encrypted)
	if len(result) > maxFileSize {
		return nil, errors.New("local vault too large")
	}
	return result, err
}

// Inspect returns the public metadata without deriving or revealing secrets.
func Inspect(dir string) (Header, error) {
	f, err := os.Open(FilePath(dir))
	if err != nil {
		return Header{}, err
	}
	defer f.Close()
	var value file
	if err = json.NewDecoder(io.LimitReader(f, maxFileSize+1)).Decode(&value); err != nil {
		return Header{}, err
	}
	return value.Header, nil
}

// Decode opens an offline snapshot with exactly the password that protected it.
func Decode(dir, password, configuredAlgorithm string) (Snapshot, Header, error) {
	path := FilePath(dir)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileSize {
		return Snapshot{}, Header{}, ErrCorrupted
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, Header{}, err
	}
	defer zero(raw)
	var record file
	if err = json.Unmarshal(raw, &record); err != nil {
		return Snapshot{}, Header{}, ErrCorrupted
	}
	if err := verifyChecksum(record); err != nil {
		return Snapshot{}, record.Header, err
	}
	h := record.Header
	if (h.Version != 1 && h.Version != 2) || h.KDF != "argon2id" || !ValidAlgorithm(h.Algorithm) ||
		(h.Origin != "us" && h.Origin != "eu") || h.Time < 1 || h.Time > 10 ||
		h.Memory < 8192 || h.Memory > 128*1024 || h.Threads < 1 || h.Threads > 8 {
		return Snapshot{}, h, ErrCorrupted
	}
	if h.Algorithm != configuredAlgorithm {
		return Snapshot{}, h, fmt.Errorf("local vault was encrypted with %s", h.Algorithm)
	}
	salt, err := base64.StdEncoding.DecodeString(h.Salt)
	if err != nil || len(salt) != 16 {
		return Snapshot{}, h, ErrCorrupted
	}
	key := argon2.IDKey([]byte(password), salt, h.Time, h.Memory, h.Threads, 32)
	defer zero(key)
	a, err := aeadFor(h.Algorithm, key)
	if err != nil {
		return Snapshot{}, h, err
	}
	nonce, err := base64.StdEncoding.DecodeString(record.Nonce)
	if err != nil || len(nonce) != a.NonceSize() {
		return Snapshot{}, h, ErrCorrupted
	}
	ciphertext, err := base64.StdEncoding.DecodeString(record.Ciphertext)
	if err != nil {
		return Snapshot{}, h, ErrCorrupted
	}
	defer zero(ciphertext)
	aad, _ := headerAAD(h)
	plain, err := a.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return Snapshot{}, h, fmt.Errorf("password differs from the one used to encrypt the local vault on %s", h.Created)
	}
	defer zero(plain)
	var snapshot Snapshot
	if err = json.Unmarshal(plain, &snapshot); err != nil {
		return Snapshot{}, h, ErrCorrupted
	}
	return snapshot, h, nil
}

// WriteAtomic replaces the single backup only after a complete durable write.
func WriteAtomic(dir string, encrypted []byte) error {
	return WriteAtomicContext(context.Background(), dir, encrypted)
}

// WriteAtomicContext avoids replacing the previous file after cancellation.
func WriteAtomicContext(ctx context.Context, dir string, encrypted []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := Validate(dir, AESGCM); err != nil {
		return err
	}
	path := FilePath(dir)
	f, err := os.CreateTemp(filepath.Dir(path), ".bw-vault-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err = f.Write(encrypted); err != nil {
		_ = f.Close()
		return err
	}
	if err = ctx.Err(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	if d, e := os.Open(filepath.Dir(path)); e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
