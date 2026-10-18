//go:build linux

// Package terminalsecurity applies best-effort protections around secret input.
package terminalsecurity

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// HardenProcess prevents core dumps and ordinary same-user process inspection.
// Privileged processes and the terminal/PTY host remain outside our control.
func HardenProcess() {
	limit := unix.Rlimit{Cur: 0, Max: 0}
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &limit)
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
}

// Exclusive prevents new opens of a terminal while sensitive input is active.
// Existing descriptors, privileged processes, and the PTY host are unaffected.
func Exclusive(fd uintptr) func() {
	if err := unix.IoctlSetInt(int(fd), unix.TIOCEXCL, 0); err != nil {
		return func() {}
	}
	return func() {
		_ = unix.IoctlSetInt(int(fd), unix.TIOCNXCL, 0)
	}
}

// ProtectTTY covers the controlling terminal even when standard input was
// redirected and the TUI framework opened /dev/tty independently.
func ProtectTTY() func() {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return func() {}
	}
	release := Exclusive(tty.Fd())
	return func() {
		release()
		_ = tty.Close()
	}
}

// RestoreOnTermination runs cleanup before restoring the default termination
// behavior. This prevents an interrupted hidden prompt from leaving echo off.
func RestoreOnTermination(cleanup func()) func() {
	signals := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		select {
		case received := <-signals:
			cleanup()
			signal.Stop(signals)
			signal.Reset(received)
			if value, ok := received.(syscall.Signal); ok {
				_ = syscall.Kill(syscall.Getpid(), value)
			}
		case <-done:
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}
