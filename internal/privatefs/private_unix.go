//go:build linux || darwin

package privatefs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func mkdirNative(path string) error {
	return os.MkdirAll(path, 0700)
}

func openNative(path string, flags int, directory bool) (*os.File, error) {
	fd, err := unix.Open(path, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

func protect(f *os.File, mode os.FileMode) error {
	var stat unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return errors.New("private path is not owned by current user")
	}
	return f.Chmod(mode)
}
