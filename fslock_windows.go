// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package fslock

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
	procCreateEventW = modkernel32.NewProc("CreateEventW")
)

const lockfileExclusiveLock = 2

// Lock implements cross-process locks using syscalls.
// This implementation is based on LockFileEx syscall.
type Lock syscall.Handle

// New returns a new lock around the given file.
func New(filename string) (Lock, error) {
	name, err := syscall.UTF16PtrFromString(filename)
	if err != nil {
		return 0, err
	}

	// Open for asynchronous I/O so that we can timeout waiting for the lock.
	// Also open shared so that other processes can open the file (but will
	// still need to lock it).
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.CREATE_NEW, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return 0, err
	}
	return Lock(handle), nil
}

// Lock locks the lock.  This call will block until the lock is available.
func (l Lock) Lock() error {
	ol, err := newOverlapped()
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(ol.HEvent)
	// this is asynchronous because we opened the file for async I/O.
	if err := lockFileEx(syscall.Handle(l), lockfileExclusiveLock, 0, 1, 0, ol); err != nil {
		return err
	}
	_, err = syscall.WaitForSingleObject(ol.HEvent, syscall.INFINITE)
	return err
}

// Unlock unlocks the lock.
func (l Lock) Unlock() error {
	return unlockFileEx(syscall.Handle(l), 0, 1, 0, nil)
}

// LockWithTimeout tries to lock the lock until the timeout expires.
func (l Lock) LockWithTimeout(timeout time.Duration) error {
	ol, err := newOverlapped()
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(ol.HEvent)
	// this is asynchronous because we opened the file for async I/O.
	if err := lockFileEx(syscall.Handle(l), lockfileExclusiveLock, 0, 1, 0, ol); err != nil {
		return err
	}
	_, err = syscall.WaitForSingleObject(ol.HEvent, uint32(timeout.Nanoseconds()/1000))
	return err
}

// newOverlapped creates a structure used to track asynchronous
// I/O requests that have been issued.
func newOverlapped() (*syscall.Overlapped, error) {
	event, err := createEvent(nil, true, true, nil)
	if err != nil {
		return nil, err
	}
	return &syscall.Overlapped{HEvent: event}, nil
}

func lockFileEx(h syscall.Handle, flags, reserved, locklow, lockhigh uint32, ol *syscall.Overlapped) (err error) {
	r1, _, e1 := syscall.Syscall6(procLockFileEx.Addr(), 6, uintptr(h), uintptr(flags), uintptr(reserved), uintptr(locklow), uintptr(lockhigh), uintptr(unsafe.Pointer(ol)))
	if r1 == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func unlockFileEx(h syscall.Handle, reserved, locklow, lockhigh uint32, ol *syscall.Overlapped) (err error) {
	r1, _, e1 := syscall.Syscall6(procUnlockFileEx.Addr(), 5, uintptr(h), uintptr(reserved), uintptr(locklow), uintptr(lockhigh), uintptr(unsafe.Pointer(ol)), 0)
	if r1 == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func createEvent(sa *syscall.SecurityAttributes, manualReset bool, initialState bool, name *uint16) (handle syscall.Handle, err error) {
	var _p0 uint32
	if manualReset {
		_p0 = 1
	} else {
		_p0 = 0
	}
	var _p1 uint32
	if initialState {
		_p1 = 1
	} else {
		_p1 = 0
	}
	r0, _, e1 := syscall.Syscall6(procCreateEventW.Addr(), 4, uintptr(unsafe.Pointer(sa)), uintptr(_p0), uintptr(_p1), uintptr(unsafe.Pointer(name)), 0, 0)
	handle = syscall.Handle(r0)
	if handle == syscall.InvalidHandle {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}
