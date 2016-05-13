// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package fslock_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	stdtesting "testing"
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/fslock"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

const (
	shortWait = 50 * time.Millisecond
	longWait  = 10 * time.Second
)

type fslockSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&fslockSuite{})

func (s *fslockSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

func (s *fslockSuite) TestNewWithExistingDir(c *gc.C) {
	dir := c.MkDir()
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, gc.IsNil)
	_, err = fslock.New(filepath.Join(dir, "special"))
	c.Assert(err, gc.IsNil)
}

func (s *fslockSuite) TestLockBlocks(c *gc.C) {
	dir := c.MkDir()
	lock1, err := fslock.New(filepath.Join(dir, "testing"))
	c.Assert(err, gc.IsNil)
	lock2, err := fslock.New(filepath.Join(dir, "testing"))
	c.Assert(err, gc.IsNil)

	acquired := make(chan struct{})
	err = lock1.Lock()
	c.Assert(err, gc.IsNil)

	go func() {
		lock2.Lock()
		close(acquired)
	}()

	// Waiting for something not to happen is inherently hard...
	select {
	case <-acquired:
		c.Fatalf("Unexpected lock acquisition")
	case <-time.After(shortWait):
		// all good
	}

	err = lock1.Unlock()
	c.Assert(err, gc.IsNil)

	select {
	case <-acquired:
		// all good
	case <-time.After(longWait):
		c.Fatalf("Expected lock acquisition")
	}
}

func (s *fslockSuite) TestLockWithTimeoutUnlocked(c *gc.C) {
	dir := c.MkDir()
	lock, err := fslock.New(filepath.Join(dir, "testing"))
	c.Assert(err, gc.IsNil)

	err = lock.LockWithTimeout(shortWait)
	c.Assert(err, gc.IsNil)
}

func (s *fslockSuite) TestLockWithTimeoutLocked(c *gc.C) {
	dir := c.MkDir()
	lock1, err := fslock.New(filepath.Join(dir, "testing"))
	c.Assert(err, gc.IsNil)
	lock2, err := fslock.New(filepath.Join(dir, "testing"))
	c.Assert(err, gc.IsNil)

	err = lock1.Lock()
	c.Assert(err, gc.IsNil)

	err = lock2.LockWithTimeout(shortWait)
	c.Assert(err, gc.Equals, fslock.ErrTimeout)
}

func (s *fslockSuite) TestStress(c *gc.C) {
	const lockAttempts = 200
	const concurrentLocks = 10

	var counter = new(int64)
	// Use atomics to update lockState to make sure the lock isn't held by
	// someone else. A value of 1 means locked, 0 means unlocked.
	var lockState = new(int32)

	var wg sync.WaitGroup

	dir := c.MkDir()

	var stress = func(name string) {
		defer wg.Done()
		lock, err := fslock.New(filepath.Join(dir, "testing"))
		if err != nil {
			c.Errorf("Failed to create a new lock")
			return
		}
		for i := 0; i < lockAttempts; i++ {
			err = lock.Lock()
			c.Assert(err, gc.IsNil)
			state := atomic.AddInt32(lockState, 1)
			c.Assert(state, gc.Equals, int32(1))
			// Tell the go routine scheduler to give a slice to someone else
			// while we have this locked.
			runtime.Gosched()
			// need to decrement prior to unlock to avoid the race of someone
			// else grabbing the lock before we decrement the state.
			atomic.AddInt32(lockState, -1)
			err = lock.Unlock()
			c.Assert(err, gc.IsNil)
			// increment the general counter
			atomic.AddInt64(counter, 1)
		}
	}

	for i := 0; i < concurrentLocks; i++ {
		wg.Add(1)
		go stress(fmt.Sprintf("Lock %d", i))
	}
	wg.Wait()
	c.Assert(*counter, gc.Equals, int64(lockAttempts*concurrentLocks))
}
