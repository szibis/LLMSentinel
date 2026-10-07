//go:build darwin || linux

package qwensmoke

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestMetalLockIsNotInheritedByRestoredLab(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metal.lock")
	unlock, err := machineLock(context.Background(), path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	child := exec.Command("sleep", "2")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	unlock()
	other, err := machineLock(context.Background(), path, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("restored lab inherited Metal lock after CI finished: %v", err)
	}
	other()
}
