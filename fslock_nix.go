// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// +build darwin dragonfly freebsd linux netbsd openbsd

package fslock

import (
	"syscall"
	"time"
)

// Lock implements cross-process locks using syscalls.
// This implementation is based on flock syscall.
type Lock int

// New returns a new lock around the given file.
func New(filename string) (Lock, error) {
	fd, err := syscall.Open(filename, syscall.O_CREAT|syscall.O_RDONLY, 0600)
	if err != nil {
		return 0, err
	}
	return Lock(fd), nil
}

// Lock locks the lock.  This call will block until the lock is available.
func (l Lock) Lock() error {
	return syscall.Flock(int(l), syscall.LOCK_EX)
}

// Unlock unlocks the lock.
func (l Lock) Unlock() error {
	return syscall.Flock(int(l), syscall.LOCK_UN)
}

// LockWithTimeout tries to lock the lock until the timeout expires.
func (l Lock) LockWithTimeout(timeout time.Duration) error {
	var t time.Time
	for {
		if t.IsZero() {
			t = time.Now()
		} else if timeout > 0 && time.Since(t) > timeout {
			return ErrTimeout
		}

		err := syscall.Flock(int(l), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		} else if err != syscall.EWOULDBLOCK {
			return err
		}

		// Wait for a bit and try again.
		time.Sleep(50 * time.Millisecond)
	}
}
