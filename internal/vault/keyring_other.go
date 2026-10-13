//go:build !linux

package vault

import "errors"

func createKernelSource(_ *[64]byte) (secretKeySource, error) {
	return nil, errors.New("Linux Kernel Keyring is unavailable")
}
