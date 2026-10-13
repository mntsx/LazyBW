package vault

import (
	"errors"
	"strings"
	"testing"
)

// TestAutomaticMemoryFallback tests encryption when the kernel disallows keys.
func TestAutomaticMemoryFallback(t *testing.T) {
	c, err := newCipherWithKernel(func(_ *[64]byte) (secretKeySource, error) {
		return nil, errors.New("keyring not supported")
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Backend() != "Memory" {
		t.Fatalf("fallback backend: %s", c.Backend())
	}
	secret, err := c.Seal("test-private-data")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(secret.(*Sealed).Data), "test-private-data") {
		t.Fatal("plaintext leaked to ciphertext")
	}
	value, err := c.Open(secret)
	if err != nil || value != "test-private-data" {
		t.Fatalf("roundtrip: %q %v", value, err)
	}
	store := c.source.(*memoryKeySource)
	c.Close()
	if c.Backend() != "Unavailable" || !store.closed {
		t.Fatal("fallback key was not destroyed")
	}
	for _, b := range store.material {
		if b != 0 {
			t.Fatal("fallback key material was not zeroed")
		}
	}
	if _, err := c.Open(secret); err == nil {
		t.Fatal("closed cipher decrypted secret")
	}
}

// TestSelectedBackendRoundtrip checks the real runtime backend and key lifecycle.
func TestSelectedBackendRoundtrip(t *testing.T) {
	c, err := NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	if c.Backend() != "Memory" && c.Backend() != "Kernel Keyring" {
		t.Fatalf("unrecognized backend: %s", c.Backend())
	}
	t.Logf("active backend: %s", c.Backend())
	value, err := c.Seal("secret password")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Open(value)
	if err != nil || got != "secret password" {
		t.Fatalf("keyring decryption: %q %v", got, err)
	}
	c.Close()
}
