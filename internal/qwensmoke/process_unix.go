//go:build darwin || linux

package qwensmoke

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func machineLock(ctx context.Context, path string, timeout time.Duration) (func(), error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, errors.New("cannot open shared Qwen CI lock")
	}
	f := os.NewFile(uintptr(fd), "qwen-ci-lock")
	info, err := f.Stat()
	var stat unix.Stat_t
	if err != nil || !info.Mode().IsRegular() || unix.Fstat(fd, &stat) != nil || int64(stat.Uid) != int64(os.Getuid()) || stat.Nlink != 1 || info.Mode().Perm()&0077 != 0 {
		_ = f.Close()
		return nil, errors.New("CI lock must be a private owned regular file")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = f.Close()
			return nil, errors.New("cannot acquire shared Qwen CI lock")
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, errors.New("timed out waiting for shared Qwen CI lock")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type ownedProcess struct {
	command *exec.Cmd
	done    chan struct{}
	log     *os.File
}

func startOwned(args []string, path string, env []string) (*ownedProcess, error) {
	if len(args) == 0 {
		return nil, errors.New("owned server command is empty")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G703 -- Fixed log basename in a private temporary directory; O_EXCL rejects existing paths.
	if err != nil {
		return nil, errors.New("cannot create private server log")
	}
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 G702 -- Operator-selected executable with fixed argument arrays; no shell or client input.
	cmd.Env = env
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = cmd.Start(); err != nil {
		_ = f.Close()
		return nil, errors.New("cannot start owned server")
	}
	p := &ownedProcess{command: cmd, done: make(chan struct{}), log: f}
	go func() { _ = cmd.Wait(); close(p.done) }()
	return p, nil
}
func (p *ownedProcess) close() error {
	defer func() { _ = p.log.Close() }()
	if err := syscall.Kill(-p.command.Process.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return errors.New("cannot terminate owned server group")
	}
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
	}
	// The group may still contain descendants after its leader has exited.
	if err := syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return errors.New("cannot kill owned server group")
	}
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		return errors.New("owned server did not exit after cleanup")
	}
	return nil
}
