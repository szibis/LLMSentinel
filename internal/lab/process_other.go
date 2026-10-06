//go:build !darwin && !linux && !freebsd

package lab

import (
	"errors"
	"io"
	"os"
	"time"
)

var errPlatform = errors.New("isolated MLX lab supervision requires macOS or Unix; production client configuration was not changed")

func acquireLock(string) (*os.File, error) { return nil, errPlatform }
func running(string) bool                  { return false }
func replaceProcess(string, []string, string, []string, io.Reader, io.Writer, io.Writer) error {
	return errPlatform
}

type ownedProcess struct {
	err  error
	done chan struct{}
}

func (e *environment) spawn([]string, string, []string) (*ownedProcess, error) {
	return nil, errPlatform
}
func (*ownedProcess) exited() bool                        { return true }
func stopProcess(*ownedProcess) error                     { return errPlatform }
func stopProcessGrace(*ownedProcess, time.Duration) error { return errPlatform }
