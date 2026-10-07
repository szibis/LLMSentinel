//go:build darwin || linux || freebsd

package taskquality

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestCLIProcessTimeoutAndExitedParentLeaveNoChildWork(t *testing.T) {
	for _, command := range []string{
		"(sleep 0.3; printf leaked > leaked) >/dev/null 2>&1 & printf DONE",
		"(sleep 0.3; printf leaked > leaked) & wait",
	} {
		root := canonicalRoot(t)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		started := time.Now()
		_, _, _, _ = runCLIProcess(ctx, "/bin/sh", []string{"-c", command}, []string{"PATH=/usr/bin:/bin"}, root)
		cancel()
		if time.Since(started) > 2*time.Second {
			t.Fatal("CLI cancellation blocked")
		}
		time.Sleep(400 * time.Millisecond)
		if _, err := os.Stat(root + "/leaked"); err == nil {
			t.Fatal("CLI descendant survived completion/cancellation")
		}
	}
}
