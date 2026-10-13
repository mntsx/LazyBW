package vault

import (
	"fmt"
	"sync"
	"testing"
)

// TestConcurrentEncryptionWithSelectedBackend checks key access across threads.
func TestConcurrentEncryptionWithSelectedBackend(t *testing.T) {
	c, err := NewCipher()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var workers sync.WaitGroup
	failures := make(chan error, 32)
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			original := fmt.Sprintf("private-%d", index)
			sealed, err := c.Seal(original)
			if err == nil {
				var got string
				got, err = c.Open(sealed)
				if err == nil && got != original {
					err = fmt.Errorf("decryption mismatch for %d", index)
				}
			}
			if err != nil {
				failures <- err
			}
		}(i)
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}
