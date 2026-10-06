package lab

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
)

type runState struct {
	RunID      string            `json:"run_id"`
	Phase      string            `json:"phase"`
	StartID    string            `json:"start_id"`
	ModelPath  string            `json:"model_path,omitempty"`
	Runtime    string            `json:"runtime"`
	Gateway    string            `json:"gateway"`
	Upstream   string            `json:"upstream"`
	Models     []runtimeItem     `json:"models"`
	Roles      map[string]string `json:"roles"`
	Supervisor string            `json:"supervisor,omitempty"`
}

var ownershipID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (e *environment) runRunner(args []string) error {
	if len(args) == 0 || len(args) > 2 || (len(args) == 2 && (args[0] != "logs" || args[1] != "--follow")) {
		return errors.New("expected runner start|run|restart|stop|status|logs [--follow]|ask")
	}
	switch args[0] {
	case "start":
		return e.start()
	case "run":
		return e.supervise()
	case "restart":
		if err := e.stop(true); err != nil {
			return err
		}
		return e.start()
	case "stop":
		return e.stop(true)
	case "status":
		return e.status()
	case "logs":
		return e.logs(len(args) == 2)
	case "ask":
		return e.ask()
	default:
		return errors.New("unknown runner command")
	}
}
func preflightPorts(ports []int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		listeners := []net.Listener{}
		var failure error
		failedPort := 0
		for _, port := range ports {
			listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				failure = err
				failedPort = port
				break
			}
			listeners = append(listeners, listener)
		}
		for _, listener := range listeners {
			listener.Close()
		}
		if failure == nil {
			return nil
		}
		if errors.Is(failure, syscall.EADDRINUSE) && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return fmt.Errorf("lab port %d could not be checked or is in use; existing services were not stopped: %w", failedPort, failure)
	}
}
func (e *environment) stop(wait bool) error {
	if !running(e.root) {
		fmt.Fprintln(e.out, "No owned lab is running.")
		return nil
	}
	deadline := time.Now().Add(2 * time.Second)
	var state runState
	for {
		if !running(e.root) {
			return nil
		}
		err := readJSON(filepath.Join(e.root, "state.json"), &state)
		if err == nil && (state.Phase == "starting" || state.Phase == "running" || state.Phase == "stopping") {
			break
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("lab still preparing; retry stop shortly")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ownershipID.MatchString(state.RunID) {
		return errors.New("invalid lab ownership state; no stop request sent")
	}
	if err := seedFile(filepath.Join(e.root, "stop-"+state.RunID), nil); err != nil {
		return err
	}
	fmt.Fprintln(e.out, "Stopping owned lab processes…")
	if wait {
		deadline = time.Now().Add(30 * time.Second)
		for running(e.root) {
			if time.Now().After(deadline) {
				return errors.New("lab did not stop within 30s; no unrelated process signaled; inspect logs")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}
func (e *environment) start() error {
	if err := e.prepare(); err != nil {
		return err
	}
	control, err := acquireLock(filepath.Join(e.root, "control"))
	if err != nil {
		return err
	}
	defer control.Close()
	if running(e.root) {
		fmt.Fprintln(e.out, "Lab already running; inspect make lab-status or explicitly rebuild.")
		return nil
	}
	startID := uuid.NewString()
	env := allowedEnvironment("HOME", "PATH", "LANG", "LC_ALL", "MODEL_PATH", "SMALL_MODEL_PATH", "MLX_FLASH_BIN", "ATTACH_RUNTIME")
	env = append(env, "SENTINEL_LAB_START_ID="+startID, "SENTINEL_PROJECT_ROOT="+e.project, "SENTINEL_LAB_ROOT="+e.root)
	path := filepath.Join(e.root, "supervisor.log")
	var offset int64
	if info, err := os.Stat(path); err == nil {
		offset = info.Size()
	}
	process, err := e.spawn([]string{e.executable, "runner", "run"}, "supervisor.log", env)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	var acknowledged time.Time
	for time.Now().Before(deadline) {
		if process.exited() {
			data, _ := readTail(path, 3000, offset)
			return fmt.Errorf("lab startup failed: %v\n%s\nSee make lab-logs", process.err, data)
		}
		var state runState
		if readJSON(filepath.Join(e.root, "state.json"), &state) == nil && state.StartID == startID && state.Phase == "running" {
			if acknowledged.IsZero() {
				acknowledged = time.Now()
			} else if time.Since(acknowledged) >= 500*time.Millisecond {
				fmt.Fprintf(e.out, "Lab server started in background: %s\nModel readiness is separate; inspect make lab-status.\n", endpoint)
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The supervisor cleans three independent child groups sequentially, each
	// with a four-second TERM grace and a three-second reaping budget.
	if err := stopProcessGrace(process, 25*time.Second); err != nil {
		return fmt.Errorf("startup timed out; owned supervisor cleanup: %w", err)
	}
	return errors.New("lab startup timed out; inspect make lab-logs")
}
func (e *environment) supervise() (result error) {
	if err := e.prepare(); err != nil {
		return err
	}
	lock, err := acquireLock(e.root)
	if err != nil {
		return err
	}
	defer lock.Close()
	var settings runtimeSettings
	path := filepath.Join(e.root, "runtime.json")
	if err = readJSON(path, &settings); err != nil && !os.IsNotExist(err) {
		return err
	}
	if model := os.Getenv("MODEL_PATH"); model != "" {
		settings = runtimeSettings{ModelPath: model, SmallModelPath: os.Getenv("SMALL_MODEL_PATH"), Executable: os.Getenv("MLX_FLASH_BIN")}
		if settings.Executable == "" {
			settings.Executable = "mlx-flash"
		}
	} else if small := os.Getenv("SMALL_MODEL_PATH"); small != "" {
		settings.SmallModelPath = small
	}
	if os.Getenv("ATTACH_RUNTIME") == "1" {
		settings = runtimeSettings{}
	}
	plan, err := e.runtimePlan(settings)
	if err != nil {
		return err
	}
	gateway := e.gatewayCommand(settings)
	if err = nonemptyFile(gateway[0]); err != nil {
		return errors.New("missing gateway binary; run make lab-run to build first")
	}
	ports := []int{19090}
	for _, item := range plan {
		ports = append(ports, item.Port)
	}
	if err = preflightPorts(ports, 3*time.Second); err != nil {
		return err
	}
	if len(plan) > 0 {
		if err = writeJSON(path, settings); err != nil {
			return err
		}
	}
	state := runState{RunID: uuid.NewString(), StartID: os.Getenv("SENTINEL_LAB_START_ID"), Phase: "starting", ModelPath: settings.ModelPath, Runtime: "attached", Gateway: endpoint, Upstream: "http://127.0.0.1:19091/v1", Models: plan, Roles: map[string]string{}, Supervisor: "go"}
	if len(plan) > 0 {
		state.Runtime = "owned"
	}
	if settings.SmallModelPath != "" {
		state.Roles = map[string]string{"haiku": "small", "sonnet": "large", "opus": "large-thinking"}
	}
	if err = writeJSON(filepath.Join(e.root, "state.json"), state); err != nil {
		return err
	}
	processes := []*ownedProcess{}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	env := append(allowedEnvironment("HOME", "PATH", "LANG", "LC_ALL"), "TMPDIR="+filepath.Join(e.root, "tmp"), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1")
	defer func() {
		state.Phase = "stopping"
		writeJSON(filepath.Join(e.root, "state.json"), state)
		for i := len(processes) - 1; i >= 0; i-- {
			if err := stopProcess(processes[i]); err != nil {
				result = errors.Join(result, err)
			}
		}
		os.Remove(filepath.Join(e.root, "stop-"+state.RunID))
		state.Phase = "stopped"
		if err := writeJSON(filepath.Join(e.root, "state.json"), state); err != nil {
			result = errors.Join(result, err)
		}
		fmt.Fprintln(e.out, "Owned lab stopped; attached services left running.")
	}()
	for _, item := range plan {
		log := "runtime.log"
		if item.Name == "small" {
			log = "runtime-small.log"
		}
		process, err := e.spawn(item.Command, log, env)
		if err != nil {
			return err
		}
		processes = append(processes, process)
	}
	process, err := e.spawn(gateway, "gateway.log", env)
	if err != nil {
		return err
	}
	processes = append(processes, process)
	state.Phase = "running"
	if err = writeJSON(filepath.Join(e.root, "state.json"), state); err != nil {
		return err
	}
	fmt.Fprintln(e.out, "Lab processes started:", endpoint, "; inspect readiness before generation.")
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-signals:
			return nil
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(e.root, "stop-"+state.RunID)); err == nil {
				return nil
			}
			for _, process := range processes {
				if process.exited() {
					return fmt.Errorf("owned process exited: %v; inspect make lab-logs", process.err)
				}
			}
		}
	}
}
func (e *environment) status() error {
	fmt.Fprintln(e.out, "Owned lab running:", running(e.root))
	if data, err := os.ReadFile(filepath.Join(e.root, "state.json")); err == nil {
		fmt.Fprintln(e.out, string(data))
	}
	for _, path := range []string{"/health", "/sentinel/status"} {
		value, err := fetchJSON(endpoint+path, 3*time.Second)
		if err != nil {
			fmt.Fprintln(e.out, "Local health unavailable:", err)
			if path == "/health" {
				return err
			}
			continue
		}
		data, _ := json.Marshal(value)
		fmt.Fprintln(e.out, path, string(data))
	}
	return nil
}
func readTail(path string, limit, minimum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	offset := info.Size() - limit
	if offset < minimum {
		offset = minimum
	}
	if offset < 0 {
		offset = 0
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(file, limit))
}
func (e *environment) logs(follow bool) error {
	names := []string{"supervisor.log", "gateway.log", "runtime.log", "runtime-small.log"}
	offsets := map[string]int64{}
	for _, name := range names {
		fmt.Fprintln(e.out, "---", name, "---")
		path := filepath.Join(e.root, name)
		data, err := readTail(path, 16384, 0)
		if err != nil {
			fmt.Fprintln(e.out, "No log yet.")
			continue
		}
		fmt.Fprint(e.out, string(data))
		if info, err := os.Stat(path); err == nil {
			offsets[name] = info.Size()
		}
	}
	if !follow {
		return nil
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-signals:
			return nil
		case <-ticker.C:
			for _, name := range names {
				path := filepath.Join(e.root, name)
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				if info.Size() < offsets[name] {
					offsets[name] = 0
				}
				file, err := os.Open(path)
				if err != nil {
					continue
				}
				file.Seek(offsets[name], io.SeekStart)
				data, err := io.ReadAll(io.LimitReader(file, 65536))
				file.Close()
				if err != nil {
					return err
				}
				if len(data) > 0 {
					fmt.Fprintf(e.out, "[%s] %s", name, data)
					offsets[name] += int64(len(data))
				}
			}
		}
	}
}
func (e *environment) ask() error {
	prompt := os.Getenv("PROMPT")
	if prompt == "" {
		prompt = "Reply with a short greeting."
	}
	model := os.Getenv("MODEL_ROLE")
	if model == "" {
		model = "local"
	}
	data, _ := json.Marshal(map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream": true, "max_tokens": 512})
	request, err := http.NewRequest(http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	fmt.Fprintln(e.stderr, "Sending to local model; first output may wait for model loading and buffered generation.")
	response, err := localHTTP(300 * time.Second).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("local generation HTTP %d; inspect logs", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	complete := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if body == "[DONE]" {
			break
		}
		if body == "" {
			continue
		}
		var event struct {
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(body), &event); err != nil {
			return err
		}
		if len(event.Error) > 0 && string(event.Error) != "null" {
			return errors.New("runtime reported a generation error")
		}
		if len(event.Choices) > 0 {
			choice := event.Choices[0]
			for _, char := range choice.Delta.Content {
				if unicode.IsPrint(char) || char == '\n' || char == '\t' {
					fmt.Fprint(e.out, string(char))
				}
			}
			if choice.Finish == "stop" {
				complete = true
			} else if choice.Finish != "" {
				return fmt.Errorf("incomplete response: %s", choice.Finish)
			}
		}
	}
	fmt.Fprintln(e.out)
	if err := scanner.Err(); err != nil {
		return err
	}
	if !complete {
		return errors.New("stream ended without successful completion; output is partial")
	}
	return nil
}
