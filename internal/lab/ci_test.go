//go:build darwin || linux || freebsd

package lab

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCIPauseRefusesBusyOrUnknownActivity(t *testing.T) {
	for _, activity := range []map[string]any{
		{}, {"queued": float64(1), "active": []any{}}, {"queued": float64(0), "active": []any{map[string]any{"request_id": "active"}}}, {"queued": float64(0), "active": nil},
	} {
		if idleForCI(activity) == nil {
			t.Fatalf("unsafe activity accepted: %v", activity)
		}
	}
	if err := idleForCI(map[string]any{"queued": float64(0), "active": []any{}}); err != nil {
		t.Fatal(err)
	}
}

func TestUserStopWaitsForCIRestoration(t *testing.T) {
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &environment{root: filepath.Join(project, ".sentinel-lab"), out: io.Discard, stderr: io.Discard}
	state := runState{RunID: "owned-run", Phase: "ci-paused"}
	if err := writeJSON(filepath.Join(e.root, "state.json"), state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(e.ciLeasePath(), ciLease{Token: "owned-token", RunID: state.RunID, Resume: true}); err != nil {
		t.Fatal(err)
	}
	entered, release, restored := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		restored <- e.resumeCIWith("owned-token", func() error { close(entered); <-release; return nil })
	}()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- e.stop(true) }()
	select {
	case err := <-stopped:
		t.Fatalf("stop returned before restoration: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-restored; err != nil {
		t.Fatal(err)
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}

func TestCIPausePreflightsRestorationHelper(t *testing.T) {
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(project, ".sentinel-lab")
	if _, err := PauseForCI(project, root, io.Discard); err == nil {
		t.Fatal("missing restoration helper accepted")
	}
	if err := os.MkdirAll(filepath.Join(project, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(project, "bin", "sentinel-tools")
	if err := os.WriteFile(binary, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PauseForCI(project, root, io.Discard); err == nil {
		t.Fatal("nonexecutable restoration helper accepted")
	}
	if err := os.Chmod(binary, 0700); err != nil {
		t.Fatal(err)
	}
	resume, err := PauseForCI(project, root, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if err := resume(); err != nil {
		t.Fatal(err)
	}
	if running(root) {
		t.Fatal("previously stopped lab was started")
	}
}

func TestCIPauseRestoresOnlyItsOwnedIdleLab(t *testing.T) {
	for _, scenario := range []string{"restore", "busy", "attached", "stop-failure", "state-failure", "manual-stop", "wrong-token", "changed-owner", "already-off"} {
		t.Run(scenario, func(t *testing.T) {
			project, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(project, ".sentinel-lab")
			e := &environment{root: root, project: project, out: io.Discard, stderr: io.Discard}
			state := runState{RunID: "owned-run", Phase: "running", Runtime: "owned", Supervisor: "go", Gateway: endpoint}
			if scenario == "attached" {
				state.Runtime = "attached"
			}
			if err := writeJSON(filepath.Join(root, "state.json"), state); err != nil {
				t.Fatal(err)
			}
			lock, err := acquireLock(root)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if scenario == "already-off" {
				lock.Close()
			}
			stops, starts := 0, 0
			token, err := e.pauseCIWith(func() (map[string]any, error) {
				active := []any{}
				if scenario == "busy" {
					active = append(active, "active")
				}
				return map[string]any{"queued": float64(0), "active": active}, nil
			}, func() error {
				stops++
				if scenario == "stop-failure" {
					return errors.New("stop failed")
				}
				if scenario == "state-failure" {
					if err := os.Remove(filepath.Join(root, "state.json")); err != nil {
						return err
					}
					if err := os.Mkdir(filepath.Join(root, "state.json"), 0700); err != nil {
						return err
					}
				}
				return lock.Close()
			})
			if scenario == "state-failure" {
				if err == nil || stops != 1 || token != "" || e.checkCIPause() == nil {
					t.Fatal("failed state write lost actionable recovery lease")
				}
				return
			}
			if scenario == "busy" || scenario == "attached" || scenario == "stop-failure" {
				if err == nil {
					t.Fatal("unsafe pause succeeded")
				}
				if _, err := os.Stat(e.ciLeasePath()); scenario != "stop-failure" && !os.IsNotExist(err) {
					t.Fatalf("failed pause left lease: %v", err)
				}
				if scenario == "stop-failure" && e.checkCIPause() == nil {
					t.Fatal("incomplete shutdown lost recovery lease")
				}
				if scenario != "stop-failure" && stops != 0 {
					t.Fatal("stopped a busy or attached lab")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "already-off" {
				if token == "" || stops != 0 || e.checkCIPause() == nil {
					t.Fatal("changed stopped lab")
				}
				if err := e.resumeCIWith(token, func() error { t.Fatal("stopped lab started"); return nil }); err != nil {
					t.Fatal(err)
				}
				return
			}
			if token == "" || stops != 1 {
				t.Fatal("missing owned pause")
			}
			if e.checkCIPause() == nil {
				t.Fatal("new lab can start during CI")
			}
			if scenario == "manual-stop" {
				if err := e.stop(true); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "changed-owner" {
				state.RunID = "different-run"
				writeJSON(filepath.Join(root, "state.json"), state)
			}
			resumeToken := token
			if scenario == "wrong-token" {
				resumeToken = "another-token"
			}
			err = e.resumeCIWith(resumeToken, func() error { starts++; return nil })
			if scenario == "wrong-token" || scenario == "changed-owner" {
				if err == nil || starts != 0 {
					t.Fatal("restored wrong owner")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := 1
			if scenario == "manual-stop" {
				expected = 0
			}
			if starts != expected {
				t.Fatalf("starts=%d want=%d", starts, expected)
			}
			if e.checkCIPause() != nil {
				t.Fatal("lease was not released")
			}
		})
	}
}

func TestCIPauseRejectsSymlinkLease(t *testing.T) {
	root := t.TempDir()
	e := &environment{root: root, out: io.Discard}
	target := filepath.Join(root, "other")
	if err := os.WriteFile(target, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, e.ciLeasePath()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.readCILease(); err == nil {
		t.Fatal("symlink CI lease accepted")
	}
}

func TestCIPausedRestartPreservesRestoration(t *testing.T) {
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &environment{project: project, root: filepath.Join(project, ".sentinel-lab"), out: io.Discard, stderr: io.Discard}
	state := runState{RunID: "owned-run", Phase: "ci-paused"}
	if err := writeJSON(filepath.Join(e.root, "state.json"), state); err != nil {
		t.Fatal(err)
	}
	lease := ciLease{Token: "owned-token", RunID: state.RunID, Resume: true}
	if err := writeJSON(e.ciLeasePath(), lease); err != nil {
		t.Fatal(err)
	}
	if err := e.runRunner([]string{"restart"}); err == nil {
		t.Fatal("restart during CI was accepted")
	}
	after, err := e.readCILease()
	if err != nil || after != lease {
		t.Fatalf("restart changed CI restoration: %#v %v", after, err)
	}
	if _, err := os.Stat(filepath.Join(e.root, "stop-"+state.RunID)); !os.IsNotExist(err) {
		t.Fatal("restart sent stop during CI")
	}
}

func TestRestartWaitsForCIPauseControlThenPreservesLease(t *testing.T) {
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &environment{project: project, root: filepath.Join(project, ".sentinel-lab"), out: io.Discard, stderr: io.Discard}
	control, err := acquireLock(filepath.Join(e.root, "control"))
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	finished := make(chan error, 1)
	go func() { finished <- e.runRunner([]string{"restart"}) }()
	select {
	case err := <-finished:
		t.Fatalf("restart bypassed CI control: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	lease := ciLease{Token: "owned-token", RunID: "owned-run", Resume: true}
	if err := writeJSON(e.ciLeasePath(), lease); err != nil {
		t.Fatal(err)
	}
	control.Close()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("restart accepted pending CI lease")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("restart did not finish")
	}
	after, err := e.readCILease()
	if err != nil || after != lease {
		t.Fatalf("restart canceled concurrent CI restoration: %#v %v", after, err)
	}
}
