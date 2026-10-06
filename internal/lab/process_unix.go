//go:build darwin || linux || freebsd

package lab

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func acquireLock(root string) (*os.File, error) {
	if err := privateDirectory(root); err != nil {
		return nil, err
	}
	if err := regularDestination(filepath.Join(root, "run.lock")); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, "run.lock"), os.O_CREATE|os.O_RDWR, 0600) // #nosec G703 -- fixed lock filename under the selected private lab directory.
	if err != nil {
		return nil, err
	}
	if err = file.Chmod(0600); err != nil {
		file.Close()
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("lab already running or lock unavailable: %w", err)
	}
	return file, nil
}
func running(root string) bool {
	file, err := acquireLock(root)
	if err != nil {
		return true
	}
	file.Close()
	return false
}
func replaceProcess(binary string, args []string, dir string, env []string, _ io.Reader, _, _ io.Writer) error {
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return syscall.Exec(binary, append([]string{binary}, args...), env) // #nosec G204 -- binary is the verified isolated client, arguments are fixed; no shell is used.
}

type ownedProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	log  *os.File
}

func (e *environment) spawn(command []string, logName string, env []string) (*ownedProcess, error) {
	if err := privateDirectory(e.root); err != nil {
		return nil, err
	}
	if err := regularDestination(filepath.Join(e.root, logName)); err != nil {
		return nil, err
	}
	log, err := os.OpenFile(filepath.Join(e.root, logName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if err = log.Chmod(0600); err != nil {
		log.Close()
		return nil, err
	}
	fmt.Fprintf(log, "\n--- Lab start %s ---\n", time.Now().Format("2006-01-02 15:04:05"))
	cmd := exec.Command(command[0], command[1:]...) // #nosec G204 -- executable is the explicit local runtime or owned Go binary; arguments are separately passed and no shell is used.
	cmd.Dir = filepath.Join(e.root, "workspace")
	cmd.Env = env
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = cmd.Start(); err != nil {
		log.Close()
		return nil, err
	}
	process := &ownedProcess{cmd: cmd, done: make(chan struct{}), log: log}
	go func() { process.err = cmd.Wait(); log.Close(); close(process.done) }()
	return process, nil
}
func (process *ownedProcess) exited() bool {
	select {
	case <-process.done:
		return true
	default:
		return false
	}
}
func stopProcess(process *ownedProcess) error {
	return stopProcessGrace(process, 4*time.Second)
}
func stopProcessGrace(process *ownedProcess, grace time.Duration) error {
	pid := process.cmd.Process.Pid
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if errors.Is(err, syscall.EPERM) && process.exited() {
			return nil
		}
		return err
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		err := syscall.Kill(-pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if errors.Is(err, syscall.EPERM) {
			if process.exited() {
				return nil
			}
			select {
			case <-process.done:
				return nil
			case <-time.After(time.Until(deadline)):
				return err
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, 0); err == nil {
		if err = syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	select {
	case <-process.done:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("owned process did not exit")
	}
}
