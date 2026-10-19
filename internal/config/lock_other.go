//go:build !linux

package config

func acquireConfigLock(string) (func(), error) { return func() {}, nil }
