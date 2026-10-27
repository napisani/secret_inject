//go:build linux || darwin

package privatefs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func TryLock(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func Unlock(f *os.File) error  { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func LockBusy(err error) bool  { return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) }
