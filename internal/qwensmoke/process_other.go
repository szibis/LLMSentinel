//go:build !darwin && !linux

package qwensmoke

import (
	"context"
	"errors"
	"time"
)

type ownedProcess struct{ done chan struct{} }

func machineLock(context.Context, string, time.Duration) (func(), error) {
	return nil, errors.New("owned smoke process groups require macOS or Linux")
}
func startOwned([]string, string, []string) (*ownedProcess, error) {
	return nil, errors.New("owned smoke process groups require macOS or Linux")
}
func (*ownedProcess) close() error {
	return errors.New("owned smoke process groups require macOS or Linux")
}
