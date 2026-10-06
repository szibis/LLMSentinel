//go:build darwin || linux

package qwensmoke

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSmokeProcessHelper(t *testing.T) {
	role := os.Getenv("QWEN_SMOKE_TEST_HELPER")
	if role == "" {
		return
	}
	if role == "child" {
		signal.Ignore(syscall.SIGTERM)
		if err := os.WriteFile(os.Getenv("QWEN_SMOKE_TEST_READY"), []byte("ready"), 0600); err != nil { // #nosec G703 -- Private test fixture path supplied only by the parent Go test.
			os.Exit(2)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSmokeProcessHelper$") // #nosec G204 G702 -- Reexecute this Go test binary for a private descendant-lifecycle fixture.
	cmd.Env = append(os.Environ(), "QWEN_SMOKE_TEST_HELPER=child")
	if err := cmd.Start(); err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(os.Getenv("QWEN_SMOKE_TEST_PID"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil { // #nosec G703 -- Private test fixture path supplied only by the parent Go test.
		os.Exit(4)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(os.Getenv("QWEN_SMOKE_TEST_READY")); err == nil { // #nosec G703 -- Private readiness marker path supplied only by the parent Go test.
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(5)
}

func TestCleanupKillsDescendantAfterLeaderExit(t *testing.T) {
	directory := t.TempDir()
	pidPath := filepath.Join(directory, "child.pid")
	ready := filepath.Join(directory, "ready")
	env := append(os.Environ(), "QWEN_SMOKE_TEST_HELPER=leader", "QWEN_SMOKE_TEST_PID="+pidPath, "QWEN_SMOKE_TEST_READY="+ready)
	process, err := startOwned([]string{os.Args[0], "-test.run=^TestSmokeProcessHelper$"}, filepath.Join(directory, "server.log"), env)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.done:
	case <-time.After(8 * time.Second):
		_ = process.close()
		t.Fatal("fixture leader did not exit")
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		_ = process.close()
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		_ = process.close()
		t.Fatal(err)
	}
	if err = process.close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err = syscall.Kill(pid, 0); err != nil {
			return
		}
		if status, e := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")); e == nil {
			fields := strings.Fields(string(status))
			if len(fields) > 2 && fields[2] == "Z" {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("owned descendant survived cleanup")
}

func TestSharedLockContentionAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metal.lock")
	unlock, err := machineLock(context.Background(), path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if other, e := machineLock(context.Background(), path, 50*time.Millisecond); e == nil {
		other()
		unlock()
		t.Fatal("lock contention accepted")
	}
	unlock()
	other, err := machineLock(context.Background(), path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	other()
	if _, err = os.Stat(path); err != nil {
		t.Fatal("lock inode removed", err)
	}
	symlink := filepath.Join(t.TempDir(), "lock")
	if err = os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	if other, e := machineLock(context.Background(), symlink, time.Second); e == nil {
		other()
		t.Fatal("symlink lock accepted")
	}
}

func TestOwnedProcessCleanupAndReadinessFailure(t *testing.T) {
	process, err := startOwned([]string{"sh", "-c", "sleep 60"}, filepath.Join(t.TempDir(), "server.log"), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err = waitReady(context.Background(), process, "http://127.0.0.1:1/health", 50*time.Millisecond); err == nil || time.Since(start) > time.Second {
		process.close()
		t.Fatal("readiness was unbounded", err)
	}
	pid := process.command.Process.Pid
	process.close()
	if err = syscall.Kill(-pid, syscall.SIGTERM); err == nil {
		t.Fatal("owned group survived cleanup")
	}
	dead, err := startOwned([]string{"sh", "-c", "exit 0"}, filepath.Join(t.TempDir(), "dead.log"), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer dead.close()
	<-dead.done
	if _, err = waitReady(context.Background(), dead, "http://127.0.0.1:1/health", time.Second); err == nil {
		t.Fatal("server death not reported")
	}
}
