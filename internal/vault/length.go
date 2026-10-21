package vault

import "encoding/base64"

// EncryptedStringLength returns the size of a Bitwarden type-2 encrypted
// string for plaintext. The value includes its fixed encryption metadata and
// Base64-encoded, PKCS#7-padded ciphertext.
func EncryptedStringLength(plaintext string) int {
	plaintextBytes := len(plaintext)
	paddedBytes := (plaintextBytes/16 + 1) * 16
	return 72 + base64.StdEncoding.EncodedLen(paddedBytes)
}

// MaxPlaintextBytes returns the largest UTF-8 byte length whose type-2
// encrypted string representation fits encryptedLimit characters.
func MaxPlaintextBytes(encryptedLimit int) int {
	maxBytes := 0
	for paddedBytes := 16; 72+base64.StdEncoding.EncodedLen(paddedBytes) <= encryptedLimit; paddedBytes += 16 {
		maxBytes = paddedBytes - 1
	}
	return maxBytes
}

// PlaintextByteLimit derives the editable UTF-8 capacity from an encrypted
// Bitwarden field limit. A zero limit means the value has no text counter.
func PlaintextByteLimit(encryptedLimit int) int {
	if encryptedLimit <= 0 {
		return 0
	}
	return MaxPlaintextBytes(encryptedLimit)
}
