package vault

import (
	"strings"
	"testing"
)

func TestType2EncryptedLengthCapacities(t *testing.T) {
	tests := []struct {
		encryptedLimit int
		maximum        int
	}{
		{1000, 687},
		{5000, 3695},
		{10000, 7439},
	}
	for _, test := range tests {
		t.Run("limit", func(t *testing.T) {
			if got := PlaintextByteLimit(test.encryptedLimit); got != test.maximum {
				t.Fatalf("capacity for %d: got %d, want %d", test.encryptedLimit, got, test.maximum)
			}
			if got := EncryptedStringLength(strings.Repeat("a", test.maximum)); got > test.encryptedLimit {
				t.Fatalf("%d plaintext bytes produced %d encrypted characters", test.maximum, got)
			}
			if got := EncryptedStringLength(strings.Repeat("a", test.maximum+1)); got <= test.encryptedLimit {
				t.Fatalf("%d plaintext bytes unexpectedly fit in %d encrypted characters", test.maximum+1, test.encryptedLimit)
			}
		})
	}
}

func TestType2EncryptedLengthUsesUTF8Bytes(t *testing.T) {
	value := "hé\n🙂"
	if got, want := len(value), 8; got != want {
		t.Fatalf("UTF-8 byte length: got %d, want %d", got, want)
	}
	if got, want := EncryptedStringLength(value), 96; got != want {
		t.Fatalf("encrypted length: got %d, want %d", got, want)
	}
	if got := PlaintextByteLimit(0); got != 0 {
		t.Fatalf("unlimited field capacity: got %d, want 0", got)
	}
}
