//go:build darwin || linux

package clientcapture

import (
	"errors"
	"os"
	"syscall"
)

func openPrivate(path string, appendMode bool) (*os.File, error) {
	flags := os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if appendMode {
		flags = os.O_WRONLY | os.O_APPEND | os.O_CREATE | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	}
	// #nosec G304 -- canonical explicit user path, no-follow open, ownership/type/link checks before writing.
	file, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	native, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(native.Uid) != os.Getuid() || (appendMode && native.Nlink != 1) {
		_ = file.Close()
		return nil, errors.New("private file type or owner mismatch")
	}
	return file, nil
}

func lockFile(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_EX) }
