//go:build !linux

// Package terminalsecurity applies best-effort protections around secret input.
package terminalsecurity

func HardenProcess() {}

func Exclusive(uintptr) func() { return func() {} }

func ProtectTTY() func() { return func() {} }

func RestoreOnTermination(func()) func() { return func() {} }
