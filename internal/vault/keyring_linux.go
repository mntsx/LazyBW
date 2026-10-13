//go:build linux

package vault

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	keyctlRevoke  uintptr = 3
	keyctlSetperm uintptr = 5
	keyctlRead    uintptr = 11
	keyPosAll     uintptr = 0x3f000000
)

// kernelKeySource owns a dedicated OS thread to preserve key possession.
// Go goroutines can migrate between threads, whose kernel credentials can
// reference different process keyrings and may not possess the same key.
type kernelKeySource struct {
	requests chan kernelRequest
}

type kernelRequest struct {
	stop   bool
	answer chan kernelResponse
}

type kernelResponse struct {
	material [64]byte
	err      error
}

// keyctl invokes the Linux key management syscall without an external process.
func keyctl(op, a, b, c, d uintptr) (uintptr, error) {
	value, _, errno := syscall.Syscall6(syscall.SYS_KEYCTL, op, a, b, c, d, 0)
	if errno != 0 {
		return 0, errno
	}
	return value, nil
}

func createKernelSource(material *[64]byte) (secretKeySource, error) {
	// Copy key creation to the pinned worker, then wipe the caller's copy.
	requests := make(chan kernelRequest)
	ready := make(chan error, 1)
	go kernelKeyWorker(*material, requests, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return &kernelKeySource{requests: requests}, nil
}

// kernelKeyWorker stores and retrieves the key from the same kernel context.
func kernelKeyWorker(material [64]byte, requests <-chan kernelRequest, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer zeroMaterial(&material)
	var randomID [16]byte
	if _, err := io.ReadFull(rand.Reader, randomID[:]); err != nil {
		ready <- err
		return
	}
	description := []byte("lazybw-" + hex.EncodeToString(randomID[:]) + "\x00")
	keyType := []byte("user\x00")
	var processRing int64 = -2
	serialValue, _, errno := syscall.Syscall6(syscall.SYS_ADD_KEY,
		uintptr(unsafe.Pointer(&keyType[0])),
		uintptr(unsafe.Pointer(&description[0])),
		uintptr(unsafe.Pointer(&material[0])), uintptr(len(material)),
		uintptr(processRing), 0)
	runtime.KeepAlive(keyType)
	runtime.KeepAlive(description)
	if errno != 0 {
		ready <- fmt.Errorf("kernel keyring unavailable: %w", errno)
		return
	}
	serial := int32(serialValue)
	// Only the possessing thread can read the key; do not grant owner-read.
	if _, err := keyctl(keyctlSetperm, uintptr(serial), keyPosAll, 0, 0); err != nil {
		_, _ = keyctl(keyctlRevoke, uintptr(serial), 0, 0, 0)
		ready <- fmt.Errorf("kernel key permissions: %w", err)
		return
	}
	// No plaintext copy should persist while the worker is idle.
	zeroMaterial(&material)
	ready <- nil
	for req := range requests {
		if req.stop {
			_, _ = keyctl(keyctlRevoke, uintptr(serial), 0, 0, 0)
			req.answer <- kernelResponse{}
			return
		}
		var answer kernelResponse
		value, err := keyctl(keyctlRead, uintptr(serial),
			uintptr(unsafe.Pointer(&answer.material[0])),
			uintptr(len(answer.material)), 0)
		if err != nil || value != uintptr(len(answer.material)) {
			zeroMaterial(&answer.material)
			if err == nil {
				err = errors.New("unexpected key length")
			}
			answer.err = fmt.Errorf("could not read kernel key: %w", err)
		}
		req.answer <- answer
		zeroMaterial(&answer.material)
	}
}

func (k *kernelKeySource) read() ([64]byte, error) {
	answer := make(chan kernelResponse, 1)
	k.requests <- kernelRequest{answer: answer}
	result := <-answer
	return result.material, result.err
}
func (k *kernelKeySource) close() {
	answer := make(chan kernelResponse, 1)
	k.requests <- kernelRequest{stop: true, answer: answer}
	<-answer
}
func (*kernelKeySource) name() string { return "Kernel Keyring" }
