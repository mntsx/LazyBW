package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Sealed stores authenticated ciphertext and the original rune count; its
// string representation never contains the secret value.
type Sealed struct {
	Nonce  []byte
	Data   []byte
	Length int
	Tag    [32]byte
}

func (s *Sealed) String() string { return "<encrypted>" }

type Cipher struct {
	mu     sync.RWMutex
	source secretKeySource
	closed bool
}

// NewCipher selects the kernel-backed key store when available, otherwise
// retains the existing in-process AES-256-GCM key as a portable fallback.
func NewCipher() (*Cipher, error) {
	return newCipherWithKernel(createKernelSource)
}

func newCipherWithKernel(kernel func(*[64]byte) (secretKeySource, error)) (*Cipher, error) {
	var material [64]byte
	if _, err := io.ReadFull(rand.Reader, material[:]); err != nil {
		zeroMaterial(&material)
		return nil, err
	}
	store, err := kernel(&material)
	if err != nil {
		store = &memoryKeySource{material: material}
	}
	zeroMaterial(&material)
	return &Cipher{source: store}, nil
}

// Backend reports the active storage for the session's AES key.
func (c *Cipher) Backend() string {
	if c == nil {
		return "Unknown"
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.source == nil || c.closed {
		return "Unavailable"
	}
	return c.source.name()
}

// withKeys keeps material transient and serializes closure against operations.
func (c *Cipher) withKeys(fn func(cipher.AEAD, []byte) (any, error)) (any, error) {
	if c == nil {
		return nil, errors.New("vault session is closed")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || c.source == nil {
		return nil, errors.New("vault session is closed")
	}
	material, err := c.source.read()
	if err != nil {
		return nil, err
	}
	defer zeroMaterial(&material)
	block, err := aes.NewCipher(material[:32])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return fn(aead, material[32:])
}

func (c *Cipher) Seal(s string) (any, error) {
	if s == "" {
		return "", nil
	}
	return c.withKeys(func(a cipher.AEAD, tagKey []byte) (any, error) {
		n := make([]byte, a.NonceSize())
		if _, err := io.ReadFull(rand.Reader, n); err != nil {
			return nil, err
		}
		tag := hmac.New(sha256.New, tagKey)
		_, _ = tag.Write([]byte(s))
		var digest [32]byte
		copy(digest[:], tag.Sum(nil))
		return &Sealed{Nonce: n, Data: a.Seal(nil, n, []byte(s), nil),
			Length: len([]rune(s)), Tag: digest}, nil
	})
}
func (c *Cipher) Open(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	switch x := v.(type) {
	case string:
		return x, nil
	case *Sealed:
		result, err := c.withKeys(func(a cipher.AEAD, _ []byte) (any, error) {
			b, err := a.Open(nil, x.Nonce, x.Data, nil)
			if err != nil {
				return nil, errors.New("secret decryption failed")
			}
			defer func() {
				for i := range b {
					b[i] = 0
				}
			}()
			return string(b), nil
		})
		if err != nil {
			return "", err
		}
		return result.(string), nil
	default:
		return fmt.Sprint(x), nil
	}
}
func (c *Cipher) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	if c.source != nil {
		c.source.close()
		c.source = nil
	}
}
func Mask(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(*Sealed); ok {
		return strings.Repeat("•", s.Length)
	}
	return strings.Repeat("•", len([]rune(fmt.Sprint(value))))
}

// PlainMap decrypts a short-lived serialization copy for the bw CLI. Never log it.
func PlainMap(c *Cipher, m map[string]any) (map[string]any, error) {
	v, err := plainCopy(c, m)
	if err != nil {
		return nil, err
	}
	return v.(map[string]any), nil
}
func plainCopy(c *Cipher, v any) (any, error) {
	switch x := v.(type) {
	case *Sealed:
		return c.Open(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			p, err := plainCopy(c, val)
			if err != nil {
				return nil, err
			}
			out[k] = p
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for k, val := range x {
			p, err := plainCopy(c, val)
			if err != nil {
				return nil, err
			}
			out[k] = p
		}
		return out, nil
	default:
		return v, nil
	}
}
func Clone(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = clone(v)
	}
	return out
}
func clone(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return Clone(x)
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = clone(v)
		}
		return out
	default:
		return v
	}
}

// SameSecret compares encrypted values without decrypting or exposing them.
func SameSecret(a, b any) bool {
	first, ok1 := a.(*Sealed)
	second, ok2 := b.(*Sealed)
	if ok1 && ok2 {
		return hmac.Equal(first.Tag[:], second.Tag[:])
	}
	if ok1 || ok2 {
		return false
	}
	return a == b
}
