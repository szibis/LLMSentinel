//go:build !darwin && !linux

package clientcapture

import (
	"errors"
	"os"
)

func openPrivate(string, bool) (*os.File, error) {
	return nil, errors.New("private capture requires supported native no-follow and file locks")
}
func lockFile(*os.File) error { return errors.New("private file lock unavailable") }
