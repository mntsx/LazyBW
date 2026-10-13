package vault

import "errors"

// secretKeySource stores an ephemeral per-process key outside the Cipher.
type secretKeySource interface {
	read() ([64]byte, error)
	close()
	name() string
}

type memoryKeySource struct {
	material [64]byte
	closed   bool
}

func (m *memoryKeySource) read() ([64]byte, error) {
	if m.closed {
		return [64]byte{}, errors.New("vault session is closed")
	}
	return m.material, nil
}
func (m *memoryKeySource) close() {
	for i := range m.material {
		m.material[i] = 0
	}
	m.closed = true
}
func (*memoryKeySource) name() string { return "Memory" }

// zeroMaterial is used for temporary key copies where Go exposes their bytes.
func zeroMaterial(material *[64]byte) {
	for i := range material {
		material[i] = 0
	}
}
