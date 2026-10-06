// Package labstatus renders finite, read-only local lab telemetry.
package labstatus

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
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

// RuntimeStats keeps numeric counters while tolerating native metadata extensions.
type RuntimeStats map[string]float64

func (s *RuntimeStats) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	counters := RuntimeStats{}
	for name, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		var number float64
		if json.Unmarshal(value, &number) == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
			counters[name] = number
		}
	}
	*s = counters
	return nil
}

type Runtime struct {
	LastGeneration map[string]any `json:"last_generation,omitempty"`
	SampleTime     float64        `json:"sample_time,omitempty"`
	Stale          bool           `json:"stale,omitempty"`
	ModelLoaded    bool           `json:"model_loaded"`
	Model          string         `json:"model"`
	Stats          RuntimeStats   `json:"stats"`
	Memory         map[string]any `json:"memory,omitempty"`
	Endpoint       string         `json:"endpoint,omitempty"`
}

func (r *Runtime) UnmarshalJSON(raw []byte) error {
	type plain Runtime
	var value plain
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	var nested struct {
		Stats struct {
			LastGeneration map[string]any `json:"last_generation"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(raw, &nested); err != nil {
		return err
	}
	if value.LastGeneration == nil {
		value.LastGeneration = nested.Stats.LastGeneration
	}
	*r = Runtime(value)
	return nil
}

type Snapshot struct {
	RuntimeEndpoints   map[string]string   `json:"runtime_endpoints,omitempty"`
	Activity           map[string]any      `json:"activity,omitempty"`
	ActivityStale      bool                `json:"activity_stale,omitempty"`
	ActivitySampleTime float64             `json:"activity_sample_time,omitempty"`
	GatewayStale       bool                `json:"gateway_stale,omitempty"`
	GatewaySampleTime  float64             `json:"gateway_sample_time,omitempty"`
	SampleTime         float64             `json:"sample_time"`
	RunID              string              `json:"run_id"`
	Gateway            bool                `json:"gateway"`
	Runtimes           map[string]*Runtime `json:"runtimes"`
	RecentCorrections  int                 `json:"recent_corrections"`
	RecentErrors       int                 `json:"recent_errors"`
	LastSpeed          map[string]float64  `json:"last_speed"`
	Rates              map[string]float64  `json:"rates,omitempty"`
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
	endpoints := map[string]string{"gateway": "http://127.0.0.1:19090/health", "activity": "http://127.0.0.1:19090/sentinel/activity"}
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
	snapshot.RuntimeEndpoints = map[string]string{}
	for name, endpoint := range endpoints {
		if name != "gateway" && name != "activity" {
			snapshot.RuntimeEndpoints[name] = endpoint
		}
	}
	var mutex sync.Mutex
	var group sync.WaitGroup
	for name, endpoint := range endpoints {
		group.Add(1)
		go func() {
			defer group.Done()
			raw := fetch(client, endpoint)
			mutex.Lock()
			defer mutex.Unlock()
			if name == "activity" {
				if json.Unmarshal(raw, &snapshot.Activity) == nil && snapshot.Activity != nil {
					snapshot.ActivitySampleTime = snapshot.SampleTime
				}
				return
			}
			if name == "gateway" {
				var value struct {
					Status string `json:"status"`
				}
				snapshot.Gateway = json.Unmarshal(raw, &value) == nil && value.Status == "ok"
				if snapshot.Gateway {
					snapshot.GatewaySampleTime = snapshot.SampleTime
				}
				return
			}
			var runtime *Runtime
			if json.Unmarshal(raw, &runtime) != nil {
				runtime = nil
			}
			if runtime != nil {
				runtime.Endpoint = endpoint
				runtime.SampleTime = snapshot.SampleTime
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

// retainRecent tolerates brief poll failures without calling retained data healthy.
func retainRecent(current *Snapshot, previous Snapshot) {
	if current.RunID != previous.RunID {
		return
	}
	for name, runtime := range current.Runtimes {
		old := previous.Runtimes[name]
		if runtime == nil || old == nil {
			continue
		}
		uptime, has := runtime.Stats["uptime_s"]
		before, had := old.Stats["uptime_s"]
		if runtime.Endpoint != old.Endpoint || (has && had && uptime < before) {
			return
		}
	}
	recent := func(stamp float64) bool { age := current.SampleTime - stamp; return stamp > 0 && age >= 0 && age <= 15 }
	for name, runtime := range current.Runtimes {
		if runtime != nil {
			continue
		}
		old := previous.Runtimes[name]
		if old == nil {
			continue
		}
		if expected := current.RuntimeEndpoints[name]; expected != "" && expected != old.Endpoint {
			continue
		}
		stamp := old.SampleTime
		if stamp == 0 {
			stamp = previous.SampleTime
		}
		if !recent(stamp) {
			continue
		}
		copy := *old
		copy.Stale = true
		copy.SampleTime = stamp
		current.Runtimes[name] = &copy
	}
	stamp := previous.GatewaySampleTime
	if stamp == 0 && previous.Gateway {
		stamp = previous.SampleTime
	}
	if !current.Gateway && recent(stamp) {
		current.GatewayStale = true
		current.GatewaySampleTime = stamp
	}
	stamp = previous.ActivitySampleTime
	if stamp == 0 && previous.Activity != nil {
		stamp = previous.SampleTime
	}
	if current.Activity == nil && recent(stamp) {
		current.Activity = previous.Activity
		current.ActivityStale = true
		current.ActivitySampleTime = stamp
	}
}
func number(value any) (float64, bool) {
	n, ok := value.(float64)
	return n, ok && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0
}
func activityLabel(snapshot Snapshot) string {
	if snapshot.Activity == nil {
		return "activity unknown"
	}
	route := func(attempt map[string]any) string {
		role := clean(attempt["role"])
		if attempt["role"] == nil || role == "" {
			role = "unknown role"
		}
		upstream, _ := attempt["upstream"].(string)
		target := "unknown runtime"
		if u, err := url.Parse(upstream); err == nil && u.Host != "" {
			for _, name := range sortedKeys(snapshot.RuntimeEndpoints) {
				endpoint, err := url.Parse(snapshot.RuntimeEndpoints[name])
				if err == nil && endpoint.Host == u.Host {
					target = name
					break
				}
			}
			for _, name := range sortedKeys(snapshot.Runtimes) {
				runtime := snapshot.Runtimes[name]
				if runtime == nil {
					continue
				}
				endpoint, err := url.Parse(runtime.Endpoint)
				if err == nil && endpoint.Host == u.Host {
					target = name
					break
				}
			}
			if target == "unknown runtime" {
				target = clean(u.Host)
			}
		}
		return role + "→" + target
	}
	suffix := ""
	if snapshot.ActivityStale {
		suffix = fmt.Sprintf(" (stale %.0fs)", snapshot.SampleTime-snapshot.ActivitySampleTime)
	}
	if active, ok := snapshot.Activity["active"].([]any); ok && len(active) > 0 {
		var routes []string
		for _, item := range active {
			if attempt, ok := item.(map[string]any); ok {
				routes = append(routes, route(attempt))
			}
		}
		if len(routes) > 0 {
			prefix := "active "
			if snapshot.ActivityStale {
				prefix = "last observed active "
			}
			return prefix + strings.Join(routes, ", ") + suffix
		}
	}
	if last, ok := snapshot.Activity["last_completed"].(map[string]any); ok {
		return "idle; last " + route(last) + suffix
	}
	if _, known := snapshot.Activity["active"].([]any); !known {
		return "activity unknown" + suffix
	}
	return "idle" + suffix
}

// Color only the final human-facing output; telemetry JSON remains plain.
func colorize(text string) string {
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	color := func(code, word string) { text = strings.ReplaceAll(text, word, "\x1b["+code+"m"+word+"\x1b[0m") }
	color("36", "LAB")
	color("32", "Sentinel online")
	for _, word := range []string{"gateway offline", "unavailable", "pressure critical"} {
		color("31", word)
	}
	for _, word := range []string{"stale", "loading", "pressure warning"} {
		color("33", word)
	}
	color("32", "ready")
	return text
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
		if runtime == nil || old == nil || runtime.Stale || old.Stale || runtime.Endpoint != old.Endpoint {
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
	if snapshot.GatewayStale {
		state = fmt.Sprintf("gateway unavailable; last seen %.0fs ago", snapshot.SampleTime-snapshot.GatewaySampleTime)
	}
	first := []string{"LAB", "selected " + label, state, activityLabel(snapshot)}
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
			if runtime.Stale {
				first = append(first, fmt.Sprintf("%s stale %.0fs (was loading)", name, snapshot.SampleTime-runtime.SampleTime))
			} else {
				first = append(first, name+" loading")
			}
			ready = false
			continue
		}
		artifact := modelName(runtime.Model)
		if artifact != "" {
			artifact = " " + artifact
		}
		if runtime.Stale {
			first = append(first, fmt.Sprintf("%s%s stale %.0fs", name, artifact, snapshot.SampleTime-runtime.SampleTime))
			ready = false
		} else {
			first = append(first, name+artifact+" ready")
		}
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
	for _, name := range sortedKeys(snapshot.Runtimes) {
		runtime := snapshot.Runtimes[name]
		if runtime == nil {
			continue
		}
		metadata := runtime.LastGeneration
		if speed, ok := number(metadata["generation_tps"]); ok {
			text := fmt.Sprintf("%s last %.1f decode tok/s", name, speed)
			if duration, ok := number(metadata["time_s"]); ok {
				text += fmt.Sprintf(" / %.1fs generation", duration)
			}
			if stamp, ok := number(metadata["timestamp"]); ok && stamp <= snapshot.SampleTime {
				text += fmt.Sprintf(" (%.0fs ago)", snapshot.SampleTime-stamp)
			} else {
				text += " (age unknown)"
			}
			second = append(second, text)
		} else if speed, ok := snapshot.LastSpeed[name]; ok {
			second = append(second, fmt.Sprintf("%s legacy log %.1f tok/s (age unknown)", name, speed))
		}
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
	// Even a young cache cannot cross a known lab restart.
	var currentState struct {
		RunID string `json:"run_id"`
	}
	if raw, e := readFile(filepath.Join(absolute, "state.json"), maxBytes); e == nil && json.Unmarshal(raw, &currentState) == nil && currentState.RunID != previous.RunID {
		previous = Snapshot{}
	}
	age := float64(time.Now().UnixNano())/1e9 - previous.SampleTime
	if age < 0 || age >= 4 || previous.SampleTime == 0 {
		client := statusClient()
		snapshot = collect(absolute, client)
		client.CloseIdleConnections()
		retainRecent(&snapshot, previous)
		addRates(&snapshot, previous)
		_ = atomicJSON(cache, snapshot)
	}
	if *jsonFlag {
		err = json.NewEncoder(out).Encode(snapshot)
	} else {
		_, err = fmt.Fprintln(out, colorize(render(session, snapshot)))
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
