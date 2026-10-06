// Package labstatus renders finite, read-only local lab telemetry.
package labstatus

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/szibis/claude-escalate/internal/clientcontrol"
)

const maxBytes = 65536

type Runtime struct {
	ModelLoaded bool               `json:"model_loaded"`
	Model       string             `json:"model"`
	Stats       map[string]float64 `json:"stats"`
	Memory      map[string]any     `json:"memory,omitempty"`
	Endpoint    string             `json:"endpoint,omitempty"`
}
type Snapshot struct {
	SampleTime        float64             `json:"sample_time"`
	RunID             string              `json:"run_id"`
	Gateway           bool                `json:"gateway"`
	Runtimes          map[string]*Runtime `json:"runtimes"`
	RecentCorrections int                 `json:"recent_corrections"`
	RecentErrors      int                 `json:"recent_errors"`
	LastSpeed         map[string]float64  `json:"last_speed"`
	Rates             map[string]float64  `json:"rates,omitempty"`
}

func readFile(path string, limit int64) ([]byte, error) {
	parent, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(parent)
	if err != nil || real != parent {
		return nil, errors.New("file parent must be canonical without symlinks")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("expected regular file without symlinks")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("file exceeds limit")
	}
	return raw, nil
}
func atomicJSON(path string, value any) error {
	parent := filepath.Dir(path)
	if err := clientcontrol.ValidateDirectory(parent); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(parent)
	if err != nil || real != parent {
		return errors.New("directory must be canonical without symlinks")
	}
	if info, e := os.Lstat(path); e == nil && !info.Mode().IsRegular() {
		return errors.New("target must be a regular file")
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(parent, ".status-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	_, err = file.Write(append(raw, '\n'))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, path)
}
func install(root, executable string) error {
	if !filepath.IsAbs(executable) {
		return errors.New("executable must be absolute")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "claude", "settings.json")
	raw, err := readFile(path, 1024*1024)
	if err != nil {
		return err
	}
	var settings map[string]any
	if err = json.Unmarshal(raw, &settings); err != nil {
		return err
	}
	if settings == nil {
		return errors.New("settings must be a JSON object")
	}
	if current, ok := settings["statusLine"]; ok {
		line, isObject := current.(map[string]any)
		command, _ := line["command"].(string)
		project := os.Getenv("SENTINEL_PROJECT_ROOT")
		if project == "" {
			project = filepath.Dir(filepath.Dir(executable))
		}
		project, err = filepath.Abs(project)
		if err != nil {
			return err
		}
		if !isObject || line["type"] != "command" || !ownedLegacyCommand(command, project, root) {
			return nil
		}
		// Preserve any additional preferences on the owned line as well.
		line["command"] = clientcontrol.ShellQuote(executable) + " statusline --root " + clientcontrol.ShellQuote(root)
		return atomicJSON(path, settings)
	}
	settings["statusLine"] = map[string]any{"type": "command", "command": clientcontrol.ShellQuote(executable) + " statusline --root " + clientcontrol.ShellQuote(root), "refreshInterval": 5}
	return atomicJSON(path, settings)
}
func statusClient() *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 600 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}
}
func ownedLegacyCommand(command, project, root string) bool {
	// Deliberately recognize only literal words, never shell operators or expansions.
	if strings.ContainsAny(command, "$`\\;|&<>\n\r") {
		return false
	}
	var fields []string
	var word strings.Builder
	var quote rune
	active := false
	for _, r := range command {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			active = true
		case ' ', '\t':
			if active {
				fields = append(fields, word.String())
				word.Reset()
				active = false
			}
		default:
			word.WriteRune(r)
			active = true
		}
	}
	if quote != 0 {
		return false
	}
	if active {
		fields = append(fields, word.String())
	}
	if len(fields) > 0 && fields[0] == "rtk" {
		fields = fields[1:]
	}
	if len(fields) != 4 {
		return false
	}
	interpreter := filepath.Base(fields[0])
	if interpreter != "python" && interpreter != "python3" && !strings.HasPrefix(interpreter, "python3.") {
		return false
	}
	return fields[1] == filepath.Join(project, "scripts", "sentinel_statusline.py") && fields[2] == "--root" && fields[3] == root
}
func fetch(client *http.Client, endpoint string) []byte {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	var object map[string]json.RawMessage
	if err != nil || len(raw) > maxBytes || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil
	}
	return raw
}
func tail(path string) string {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	offset := max(int64(0), info.Size()-maxBytes)
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return ""
	}
	text := string(raw)
	if index := strings.LastIndex(text, "--- Lab start "); index >= 0 {
		text = text[index+len("--- Lab start "):]
	}
	return text
}

var errorPattern = regexp.MustCompile(`Claude adapter HTTP [45]\d\d:`)
var speedPattern = regexp.MustCompile(`Inference complete tokens=\d+ tok_per_s=([\d.]+)`)

func collect(root string, client *http.Client) Snapshot {
	var state struct {
		RunID  string `json:"run_id"`
		Models []struct {
			Name string          `json:"name"`
			Port json.RawMessage `json:"port"`
		} `json:"models"`
	}
	if raw, e := readFile(filepath.Join(root, "state.json"), maxBytes); e == nil {
		_ = json.Unmarshal(raw, &state)
	}
	endpoints := map[string]string{"gateway": "http://127.0.0.1:19090/health"}
	if len(state.Models) == 0 {
		endpoints["large"] = "http://127.0.0.1:19091/status"
	}
	for _, model := range state.Models {
		var port int
		if (model.Name == "large" || model.Name == "small") && json.Unmarshal(model.Port, &port) == nil && port >= 1 && port <= 65535 {
			endpoints[model.Name] = fmt.Sprintf("http://127.0.0.1:%d/status", port)
		}
	}
	snapshot := Snapshot{SampleTime: float64(time.Now().UnixNano()) / 1e9, RunID: state.RunID, Runtimes: map[string]*Runtime{}, LastSpeed: map[string]float64{}}
	var mutex sync.Mutex
	var group sync.WaitGroup
	for name, endpoint := range endpoints {
		group.Add(1)
		go func() {
			defer group.Done()
			raw := fetch(client, endpoint)
			mutex.Lock()
			defer mutex.Unlock()
			if name == "gateway" {
				var value struct {
					Status string `json:"status"`
				}
				snapshot.Gateway = json.Unmarshal(raw, &value) == nil && value.Status == "ok"
				return
			}
			var runtime *Runtime
			if json.Unmarshal(raw, &runtime) != nil {
				runtime = nil
			}
			if runtime != nil {
				runtime.Endpoint = endpoint
			}
			snapshot.Runtimes[name] = runtime
		}()
	}
	group.Wait()
	log := tail(filepath.Join(root, "gateway.log"))
	snapshot.RecentCorrections = strings.Count(log, "attempting one format correction")
	snapshot.RecentErrors = len(errorPattern.FindAllString(log, -1))
	for name := range snapshot.Runtimes {
		filename := "runtime.log"
		if name == "small" {
			filename = "runtime-small.log"
		}
		matches := speedPattern.FindAllStringSubmatch(tail(filepath.Join(root, filename)), -1)
		if len(matches) > 0 {
			var speed float64
			if _, err := fmt.Sscan(matches[len(matches)-1][1], &speed); err == nil && !math.IsInf(speed, 0) && !math.IsNaN(speed) {
				snapshot.LastSpeed[name] = speed
			}
		}
	}
	return snapshot
}
func addRates(current *Snapshot, previous Snapshot) {
	current.Rates = map[string]float64{}
	elapsed := current.SampleTime - previous.SampleTime
	if elapsed <= 0 || elapsed > 30 || current.RunID != previous.RunID || len(current.Runtimes) == 0 || len(current.Runtimes) != len(previous.Runtimes) {
		return
	}
	tokens, requests := 0.0, 0.0
	seen := map[string]bool{}
	for name, runtime := range current.Runtimes {
		old := previous.Runtimes[name]
		if runtime == nil || old == nil || runtime.Endpoint != old.Endpoint {
			return
		}
		for _, key := range []string{"uptime_s", "tokens_generated", "requests"} {
			now, ok := runtime.Stats[key]
			before, exists := old.Stats[key]
			if !ok || !exists || math.IsNaN(now) || math.IsNaN(before) || math.IsInf(now, 0) || math.IsInf(before, 0) || now < before {
				return
			}
		}
		identity := runtime.Endpoint
		if identity == "" {
			identity = name
		}
		if seen[identity] {
			continue
		}
		seen[identity] = true
		tokens += runtime.Stats["tokens_generated"] - old.Stats["tokens_generated"]
		requests += runtime.Stats["requests"] - old.Stats["requests"]
	}
	current.Rates = map[string]float64{"tokens_per_s": tokens / elapsed, "requests_per_min": requests * 60 / elapsed}
}
func clean(value any) string {
	var b strings.Builder
	count := 0
	for _, r := range fmt.Sprint(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_ .%+:/-", r) {
			b.WriteRune(r)
			count++
			if count >= 80 {
				break
			}
		}
	}
	return b.String()
}
func modelName(value string) string {
	if index := strings.Index(value, "models--"); index >= 0 {
		value = strings.Split(value[index+8:], "/")[0]
		parts := strings.Split(value, "--")
		value = parts[len(parts)-1]
	} else {
		value = filepath.Base(value)
		if value == "." {
			value = ""
		}
	}
	return clean(value)
}
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func render(session map[string]any, snapshot Snapshot) string {
	label := "Claude"
	if model, ok := session["model"].(map[string]any); ok {
		if value, ok := model["display_name"].(string); ok && value != "" {
			label = value
		} else if value, ok := model["id"].(string); ok && value != "" {
			label = value
		}
	}
	label = strings.NewReplacer("sentinel-sonnet", "Sonnet", "sentinel-haiku", "Haiku", "sentinel-opus", "Opus").Replace(clean(label))
	state := "gateway offline"
	if snapshot.Gateway {
		state = "Sentinel online"
	}
	first := []string{"LAB", label, state}
	if context, ok := session["context_window"].(map[string]any); ok {
		if used, ok := context["used_percentage"].(float64); ok && !math.IsNaN(used) && !math.IsInf(used, 0) {
			first = append(first, fmt.Sprintf("ctx %.0f%%", used))
		}
	}
	ready := len(snapshot.Runtimes) > 0
	requests, tokens := 0.0, 0.0
	seen := map[string]bool{}
	var memory map[string]any
	for _, name := range sortedKeys(snapshot.Runtimes) {
		runtime := snapshot.Runtimes[name]
		if runtime == nil {
			first = append(first, name+" unavailable")
			ready = false
			continue
		}
		if len(memory) == 0 && len(runtime.Memory) > 0 {
			memory = runtime.Memory
		}
		if !runtime.ModelLoaded {
			first = append(first, name+" loading")
			ready = false
			continue
		}
		artifact := modelName(runtime.Model)
		if artifact != "" {
			artifact = " " + artifact
		}
		first = append(first, name+artifact+" ready")
		req, hasReq := runtime.Stats["requests"]
		tok, hasTok := runtime.Stats["tokens_generated"]
		if !hasReq || !hasTok || math.IsNaN(req) || math.IsNaN(tok) || math.IsInf(req, 0) || math.IsInf(tok, 0) {
			ready = false
			continue
		}
		identity := runtime.Endpoint
		if identity == "" {
			identity = name
		}
		if !seen[identity] {
			seen[identity] = true
			requests += req
			tokens += tok
		}
	}
	var second []string
	if ready {
		second = append(second, fmt.Sprintf("%g MLX req", requests), fmt.Sprintf("%g tok", tokens))
	}
	if speed, ok := snapshot.Rates["tokens_per_s"]; ok {
		second = append(second, fmt.Sprintf("%.1f tok/s interval", speed), fmt.Sprintf("%.1f req/min", snapshot.Rates["requests_per_min"]))
	}
	for _, name := range sortedKeys(snapshot.LastSpeed) {
		second = append(second, fmt.Sprintf("%s last %.1f tok/s", name, snapshot.LastSpeed[name]))
	}
	var third []string
	if len(memory) > 0 {
		for _, field := range []struct{ key, suffix string }{{"available_gb", "GB available"}, {"swap_used_gb", "GB swap"}} {
			if value, ok := memory[field.key]; ok {
				third = append(third, clean(value)+" "+field.suffix)
			}
		}
		pressure := "unknown"
		if value, ok := memory["pressure"]; ok {
			pressure = clean(value)
		}
		third = append(third, "pressure "+pressure)
	}
	third = append(third, fmt.Sprintf("Format corrections %d / errors %d (log tail)", snapshot.RecentCorrections, snapshot.RecentErrors))
	rows := []string{strings.Join(first, " | ")}
	if len(second) > 0 {
		rows = append(rows, strings.Join(second, " | "))
	}
	rows = append(rows, strings.Join(third, " | "))
	return strings.Join(rows, "\n")
}

// Run implements one bounded telemetry refresh or a non-overwriting installation.
func Run(args []string, in io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("statusline", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".sentinel-lab", "isolated lab root")
	installFlag := flags.Bool("install", false, "install only if no status line exists")
	jsonFlag := flags.Bool("json", false, "print telemetry JSON")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "Unexpected statusline arguments")
		return 1
	}
	absolute, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := clientcontrol.ValidateDirectory(absolute); err != nil {
		fmt.Fprintln(stderr, "Statusline root rejected:", err)
		return 1
	}
	if *installFlag {
		executable, e := os.Executable()
		if e == nil {
			e = install(absolute, executable)
		}
		if e != nil {
			fmt.Fprintln(stderr, "Statusline install failed:", e)
			return 1
		}
		return 0
	}
	session := map[string]any{}
	if !*jsonFlag {
		raw, e := io.ReadAll(io.LimitReader(in, maxBytes+1))
		if e == nil && len(raw) <= maxBytes {
			_ = json.Unmarshal(raw, &session)
		}
	}
	cache := filepath.Join(absolute, "tmp", "statusline.json")
	// The telemetry directory belongs to this utility, unlike the user's settings directory.
	if err := clientcontrol.ValidateDirectory(filepath.Dir(cache)); err == nil {
		if err := os.MkdirAll(filepath.Dir(cache), 0700); err == nil {
			real, e := filepath.EvalSymlinks(filepath.Dir(cache))
			if e == nil && real == filepath.Dir(cache) {
				_ = os.Chmod(real, 0700)
			}
		}
	}
	var previous Snapshot
	if raw, e := readFile(cache, maxBytes); e == nil {
		_ = json.Unmarshal(raw, &previous)
	}
	snapshot := previous
	age := float64(time.Now().UnixNano())/1e9 - previous.SampleTime
	if age < 0 || age >= 4 || previous.SampleTime == 0 {
		client := statusClient()
		snapshot = collect(absolute, client)
		client.CloseIdleConnections()
		addRates(&snapshot, previous)
		_ = atomicJSON(cache, snapshot)
	}
	if *jsonFlag {
		err = json.NewEncoder(out).Encode(snapshot)
	} else {
		_, err = fmt.Fprintln(out, render(session, snapshot))
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
