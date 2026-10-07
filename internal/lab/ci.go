package lab

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/szibis/claude-escalate/internal/clientcontrol"
)

type ciLease struct {
	Token            string `json:"token"`
	RunID            string `json:"run_id"`
	Resume           bool   `json:"resume"`
	InitiallyStopped bool   `json:"initially_stopped,omitempty"`
}

func (e *environment) ciLeasePath() string { return filepath.Join(e.root, "ci-pause.json") }
func (e *environment) readCILease() (ciLease, error) {
	var lease ciLease
	if err := regularDestination(e.ciLeasePath()); err != nil {
		return lease, err
	}
	err := readJSON(e.ciLeasePath(), &lease)
	if err == nil && (!ownershipID.MatchString(lease.Token) || !ownershipID.MatchString(lease.RunID)) {
		err = errors.New("invalid CI pause ownership")
	}
	return lease, err
}
func idleForCI(activity map[string]any) error {
	queued, ok := activity["queued"].(float64)
	active, ok2 := activity["active"].([]any)
	if !ok || !ok2 || queued != 0 || len(active) != 0 {
		return errors.New("lab is busy or activity is unknown; CI will not interrupt user requests")
	}
	return nil
}
func (e *environment) pauseCI() (string, error) {
	return e.pauseCIWithReservation(func(token string) (map[string]any, error) {
		if err := reserveGateway(http.MethodPost, token); err != nil {
			if errors.Is(err, errCIGatewayRefused) {
				return nil, err
			}
			if releaseErr := reserveGateway(http.MethodDelete, token); releaseErr != nil {
				return nil, errors.Join(errCIReservationUncertain, err, releaseErr)
			}
			return nil, err
		}
		return map[string]any{"queued": float64(0), "active": []any{}}, nil
	}, func() error { return e.stopFor(true, true) })
}

var errCIReservationUncertain = errors.New("gateway reservation uncertain; inspect private CI lease before recovery")
var errCIGatewayRefused = errors.New("gateway refused atomic idle reservation")

func reserveGateway(method, token string) error {
	req, err := http.NewRequest(method, endpoint+"/sentinel/ci-reservation", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Sentinel-CI-Token", token)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errCIGatewayRefused
	}
	return nil
}

func (e *environment) pauseCIWith(activityFn func() (map[string]any, error), stopFn func() error) (string, error) {
	return e.pauseCIWithReservation(func(string) (map[string]any, error) { return activityFn() }, stopFn)
}
func (e *environment) pauseCIWithReservation(activityFn func(string) (map[string]any, error), stopFn func() error) (string, error) {
	control, err := acquireLock(filepath.Join(e.root, "control"))
	if err != nil {
		return "", err
	}
	defer control.Close()
	if _, err := e.readCILease(); err == nil {
		return "", errors.New("lab already reserved by CI")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if !running(e.root) {
		reservation, err := acquireLock(e.root)
		if err != nil {
			return "", err
		}
		defer reservation.Close()
		lease := ciLease{Token: uuid.NewString(), RunID: uuid.NewString(), InitiallyStopped: true}
		return lease.Token, writeJSON(e.ciLeasePath(), lease)
	}
	var state runState
	if err := readJSON(filepath.Join(e.root, "state.json"), &state); err != nil {
		return "", err
	}
	if state.Phase != "running" || state.Runtime != "owned" || state.Supervisor != "go" || state.Gateway != endpoint || !ownershipID.MatchString(state.RunID) {
		return "", errors.New("CI can pause only a running owned Go lab")
	}
	lease := ciLease{Token: uuid.NewString(), RunID: state.RunID, Resume: true}
	if err := writeJSON(e.ciLeasePath(), lease); err != nil {
		return "", err
	}
	activity, err := activityFn(lease.Token)
	if err != nil {
		if !errors.Is(err, errCIReservationUncertain) {
			_ = os.Remove(e.ciLeasePath())
		}
		return "", err
	}
	if err := idleForCI(activity); err != nil {
		_ = os.Remove(e.ciLeasePath())
		return "", err
	}
	if err := stopFn(); err != nil {
		return "", fmt.Errorf("lab shutdown incomplete: %w; inspect private ci-pause.json for recovery after the job ends", err)
	}
	state.Phase = "ci-paused"
	if err := writeJSON(filepath.Join(e.root, "state.json"), state); err != nil {
		return "", fmt.Errorf("lab paused but state update failed: %w; inspect private ci-pause.json for recovery", err)
	}
	return lease.Token, nil
}
func (e *environment) resumeCI(token string) error {
	return e.resumeCIWith(token, e.startLocked)
}
func (e *environment) resumeCIWith(token string, startFn func() error) error {
	if token == "" {
		return nil
	}
	control, err := acquireLock(filepath.Join(e.root, "control"))
	if err != nil {
		return err
	}
	defer control.Close()
	lease, err := e.readCILease()
	if err != nil {
		control.Close()
		return err
	}
	if token != lease.Token {
		control.Close()
		return errors.New("CI resume token does not own pause")
	}
	if lease.InitiallyStopped {
		return os.Remove(e.ciLeasePath())
	}
	var state runState
	if err := readJSON(filepath.Join(e.root, "state.json"), &state); err != nil {
		control.Close()
		return err
	}
	if state.RunID != lease.RunID {
		control.Close()
		return errors.New("lab ownership changed during CI; refusing restoration")
	}
	if lease.Resume && running(e.root) {
		if err := reserveGateway(http.MethodPost, lease.Token); err != nil {
			return err
		}
		if err := e.stopFor(true, true); err != nil {
			return err
		}
	}
	if err := os.Remove(e.ciLeasePath()); err != nil {
		control.Close()
		return err
	}
	if !lease.Resume {
		state.Phase = "stopped"
		return writeJSON(filepath.Join(e.root, "state.json"), state)
	}
	return startFn()
}

func (e *environment) cancelCIResume() error {
	control, err := e.stopControlLock()
	if err != nil {
		return err
	}
	defer control.Close()
	lease, err := e.readCILease()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	lease.Resume = false
	return writeJSON(e.ciLeasePath(), lease)
}

// A user stop waits for an in-progress pause or restoration, then cancels or
// stops the resulting owned run. It cannot report success before restoration.
func (e *environment) stopControlLock() (*os.File, error) {
	deadline := time.Now().Add(45 * time.Second)
	for {
		lock, err := acquireLock(filepath.Join(e.root, "control"))
		if err == nil || !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			return lock, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (e *environment) checkCIPause() error {
	if _, err := e.readCILease(); err == nil {
		return errors.New("lab paused for hardware CI; restart after the CI lease is released")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ciEnvironment(project, root string, out io.Writer) (*environment, error) {
	if !filepath.IsAbs(project) || !filepath.IsAbs(root) {
		return nil, errors.New("CI lab project and root must both be absolute paths")
	}
	if err := privateDirectory(root); err != nil {
		return nil, err
	}
	if err := clientcontrol.ValidateDirectory(filepath.Join(project, "bin")); err != nil {
		return nil, err
	}
	executable := filepath.Join(project, "bin", "sentinel-tools")
	if err := regularDestination(executable); err != nil {
		return nil, err
	}
	info, err := os.Stat(executable)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0111 == 0 {
		return nil, errors.New("CI lab restoration helper must be executable")
	}
	return &environment{project: project, root: root, executable: executable, out: out, stderr: out, in: os.Stdin}, nil
}

// PauseForCI is opt-in and must be called while holding the shared Metal lock.
// A previously stopped lab stays stopped. Active or attached labs are refused.
func PauseForCI(project, root string, out io.Writer) (func() error, error) {
	if project == "" && root == "" {
		return func() error { return nil }, nil
	}
	e, err := ciEnvironment(project, root, out)
	if err != nil {
		return nil, err
	}
	token, err := e.pauseCI()
	if err != nil {
		return nil, err
	}
	return func() error { return e.resumeCI(token) }, nil
}

// RunCI lets other runtimes use the same Go-owned lab coordination while they
// hold the shared machine lock. JSON stdout is separate from lifecycle logs.
func RunCI(args []string, _ io.Reader, out, stderr io.Writer) int {
	if len(args) == 0 {
		return 2
	}
	flags := flag.NewFlagSet("ci-lab", flag.ContinueOnError)
	flags.SetOutput(stderr)
	project := flags.String("project", "", "absolute lab project")
	root := flags.String("root", "", "absolute lab root")
	token := flags.String("token", "", "pause ownership token")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || !filepath.IsAbs(*project) || !filepath.IsAbs(*root) {
		return 2
	}
	e, err := ciEnvironment(*project, *root, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "CI lab coordination:", err)
		return 1
	}
	switch args[0] {
	case "pause":
		*token, err = e.pauseCI()
		if err == nil {
			err = json.NewEncoder(out).Encode(map[string]any{"paused": *token != "", "token": *token})
		}
	case "resume":
		err = e.resumeCI(*token)
	default:
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "CI lab coordination:", err)
		return 1
	}
	return 0
}
