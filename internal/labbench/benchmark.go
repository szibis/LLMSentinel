// Package labbench measures fixed synthetic requests through a strict-local gateway.
package labbench

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const scope = "synthetic-benchmark"
const maxBody = 2 * 1024 * 1024

type Config struct {
	Endpoint      string
	StatsEndpoint string
	Roles         []string
	Pairs         int
	Timeout       time.Duration
}
type Snapshot struct {
	Source    string   `json:"source"`
	Error     string   `json:"error,omitempty"`
	Model     string   `json:"model,omitempty"`
	Requests  *float64 `json:"requests"`
	Uptime    *float64 `json:"uptime_s"`
	Reused    *float64 `json:"reused_tokens"`
	Processed *float64 `json:"processed_tokens"`
	Cached    *float64 `json:"cached_prompt_tokens"`
	TTFT      *float64 `json:"ttft_ms"`
	Decode    *float64 `json:"decode_tps"`
	Native    bool     `json:"native_generation_metadata"`
}
type CacheEvidence struct {
	Status         string   `json:"status"`
	Provenance     string   `json:"provenance"`
	Reason         string   `json:"reason,omitempty"`
	ReusedDelta    *float64 `json:"reused_tokens_delta"`
	ProcessedDelta *float64 `json:"processed_tokens_delta"`
	CachedTokens   *float64 `json:"cached_tokens"`
}
type Sample struct {
	Pair            int           `json:"pair"`
	Phase           string        `json:"phase"`
	Role            string        `json:"role"`
	ObservedRole    string        `json:"observed_role,omitempty"`
	Protocol        string        `json:"protocol"`
	StartedAt       time.Time     `json:"started_at"`
	Status          string        `json:"status"`
	HTTPStatus      int           `json:"http_status"`
	Error           string        `json:"error,omitempty"`
	ResponseModel   string        `json:"response_model,omitempty"`
	Output          string        `json:"output,omitempty"`
	OutputTruncated bool          `json:"output_truncated,omitempty"`
	TotalMS         float64       `json:"total_ms"`
	InputTokens     *float64      `json:"input_tokens"`
	OutputTokens    *float64      `json:"output_tokens"`
	UsageSource     string        `json:"usage_source"`
	TTFT            *float64      `json:"ttft_ms"`
	NativeDecode    *float64      `json:"native_decode_tps"`
	Before          Snapshot      `json:"before"`
	After           Snapshot      `json:"after"`
	Cache           CacheEvidence `json:"cache"`
}
type Summary struct {
	Successful int      `json:"successful_requests"`
	P50        *float64 `json:"total_p50_ms"`
	P95        *float64 `json:"total_p95_ms"`
}
type Report struct {
	RunID             string    `json:"run_id,omitempty"`
	Version           int       `json:"schema_version"`
	Scope             string    `json:"scope"`
	Timestamp         time.Time `json:"timestamp"`
	Endpoint          string    `json:"endpoint"`
	PromptSet         string    `json:"prompt_set"`
	SessionProvenance string    `json:"session_provenance,omitempty"`
	Notes             []string  `json:"notes"`
	Samples           []Sample  `json:"samples"`
	Summary           Summary   `json:"summary"`
}
type activity struct {
	Queued int               `json:"queued"`
	Active []json.RawMessage `json:"active"`
	Last   *struct {
		ID       string    `json:"attempt_id"`
		Role     string    `json:"role"`
		Started  time.Time `json:"started_at"`
		Finished time.Time `json:"finished_at"`
		Accepted bool      `json:"accepted"`
	} `json:"last_completed"`
}

func localURL(value string, root bool) (string, error) {
	u, err := url.Parse(value)
	if err != nil {
		return "", errors.New("invalid loopback URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "http" || ip == nil || !ip.IsLoopback() || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (root && strings.Trim(u.Path, "/") != "") {
		return "", errors.New("endpoint must be HTTP with a literal loopback address, without credentials/query; gateway endpoint has no path")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func validate(cfg Config) (Config, error) {
	var err error
	cfg.Endpoint, err = localURL(cfg.Endpoint, true)
	if err != nil {
		return cfg, err
	}
	if cfg.StatsEndpoint != "" {
		cfg.StatsEndpoint, err = localURL(cfg.StatsEndpoint, false)
		if err != nil {
			return cfg, err
		}
		if len(cfg.Roles) != 1 {
			return cfg, errors.New("explicit stats endpoint requires exactly one role")
		}
	}
	if cfg.Pairs < 1 || cfg.Pairs > 20 || cfg.Timeout <= 0 || cfg.Timeout > 10*time.Minute {
		return cfg, errors.New("pairs must be 1..20 and timeout must be positive and at most 600 seconds")
	}
	if len(cfg.Roles) < 1 || len(cfg.Roles) > 3 {
		return cfg, errors.New("select one to three unique roles")
	}
	seen := map[string]bool{}
	for _, role := range cfg.Roles {
		if (role != "haiku" && role != "sonnet" && role != "opus") || seen[role] {
			return cfg, errors.New("roles must be unique haiku,sonnet,opus")
		}
		seen[role] = true
	}
	return cfg, nil
}
func object(raw []byte) (map[string]any, error) {
	var value map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	if value == nil || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("expected one JSON object")
	}
	return value, nil
}
func nested(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}
func number(m map[string]any, key string, integer bool) *float64 {
	n, ok := m[key].(json.Number)
	if !ok {
		return nil
	}
	value, err := n.Float64()
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || (integer && value != math.Trunc(value)) {
		return nil
	}
	return &value
}
func readResponse(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, errors.New("response exceeds 2 MiB")
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return body, nil
}
func get(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return readResponse(resp)
}
func snapshot(raw []byte, role, source string) Snapshot {
	s := Snapshot{Source: source}
	m, err := object(raw)
	if err != nil {
		s.Error = "invalid telemetry JSON"
		return s
	}
	if roles := nested(m, "runtime_health"); roles != nil {
		m = nested(roles, role)
	} else if runtimes := nested(m, "runtimes"); runtimes != nil {
		m = nested(runtimes, role)
	} else if health := nested(m, "health"); health != nil {
		m = health
	}
	if m == nil {
		s.Error = "selected role telemetry missing"
		return s
	}
	if m["stale"] == true {
		s.Error = "stale telemetry"
		return s
	}
	s.Model, _ = m["model"].(string)
	stats := nested(m, "stats")
	if stats == nil {
		s.Error = "runtime counters unavailable"
		return s
	}
	s.Requests = number(stats, "requests", true)
	s.Uptime = number(stats, "uptime_s", false)
	cache := nested(m, "prompt_cache")
	if cache == nil {
		cache = nested(stats, "prompt_cache")
	}
	s.Reused = number(cache, "reused_tokens", true)
	s.Processed = number(cache, "processed_tokens", true)
	generation := nested(m, "last_generation")
	if generation == nil {
		generation = nested(stats, "last_generation")
	}
	s.Native, _ = generation["native_generation_metadata"].(bool)
	s.Cached = number(generation, "cached_prompt_tokens", true)
	s.TTFT = number(generation, "ttft_ms", false)
	s.Decode = number(generation, "generation_tps", false)
	return s
}
func sampleStats(ctx context.Context, c *http.Client, endpoint, role string) Snapshot {
	raw, err := get(ctx, c, endpoint)
	if err != nil {
		return Snapshot{Source: endpoint, Error: err.Error()}
	}
	return snapshot(raw, role, endpoint)
}
func sampleActivity(ctx context.Context, c *http.Client, endpoint string) *activity {
	raw, err := get(ctx, c, endpoint+"/sentinel/activity")
	if err != nil {
		return nil
	}
	var a activity
	if _, err := object(raw); err != nil || json.Unmarshal(raw, &a) != nil {
		return nil
	}
	return &a
}
func cacheEvidence(before, after Snapshot, a, b *activity, started time.Time, role string) (CacheEvidence, *float64, *float64, string) {
	e := CacheEvidence{Status: "unknown", Provenance: "unavailable"}
	fail := func(reason string) (CacheEvidence, *float64, *float64, string) {
		e.Reason = reason
		return e, nil, nil, ""
	}
	if before.Error != "" || after.Error != "" || before.Requests == nil || after.Requests == nil || before.Model == "" || after.Model != before.Model || before.Uptime == nil || after.Uptime == nil {
		return fail("missing counters or runtime identity changed")
	}
	if *after.Requests < *before.Requests || *after.Uptime < *before.Uptime {
		return fail("runtime counter reset")
	}
	if *after.Requests-*before.Requests != 1 {
		return fail("request counter window was not isolated")
	}
	if b == nil || b.Last == nil || a == nil || b.Last.ID == "" || (a.Last != nil && a.Last.ID == b.Last.ID) || b.Last.Role != role || !b.Last.Accepted || b.Last.Started.Before(started) || b.Last.Finished.Before(b.Last.Started) || b.Queued != 0 || len(b.Active) != 0 {
		return fail("matching completed runtime role unavailable")
	}
	if before.Reused == nil || after.Reused == nil || before.Processed == nil || after.Processed == nil || *after.Reused < *before.Reused || *after.Processed < *before.Processed {
		return fail("cache counters missing or reset")
	}
	reused, processed := *after.Reused-*before.Reused, *after.Processed-*before.Processed
	e.ReusedDelta = &reused
	e.ProcessedDelta = &processed
	e.Provenance = "isolated_runtime_counter_window"
	if !after.Native || after.Cached == nil {
		e.Reason = "native per-generation cache count unavailable"
		return e, nil, nil, b.Last.Role
	}
	e.CachedTokens = after.Cached
	if *after.Cached > 0 {
		e.Status = "cached_tokens_observed"
	} else {
		e.Status = "zero_cached_tokens_observed"
	}
	return e, after.TTFT, after.Decode, b.Last.Role
}

var readBenchmarkRandom = rand.Read

// Benchmark never resets caches or infers warm/cold state from request order.
func Benchmark(ctx context.Context, cfg Config) (Report, error) {
	report := Report{Version: 1, Scope: scope, Timestamp: time.Now().UTC(), PromptSet: "paired-prefix-v2", SessionProvenance: "isolated_random_run_role_pair_session", Samples: []Sample{}, Notes: []string{"Fixed synthetic prompts; first/repeat labels describe request order, not cache state.", "Each first/repeat pair shares a fresh session, isolated across pairs, roles and runs. Cache residency and memory pressure may still prevent reuse.", "TTFT/decode require native metadata and a matching isolated runtime counter window. Missing values remain null.", "Counter-window attribution cannot exclude traffic invisible to sampled runtime counters. No cost is inferred."}}
	cfg, err := validate(cfg)
	if err != nil {
		return report, err
	}
	report.Endpoint = cfg.Endpoint
	runID := make([]byte, 16)
	if n, err := readBenchmarkRandom(runID); err != nil {
		return report, fmt.Errorf("benchmark session randomness unavailable: %w", err)
	} else if n != len(runID) {
		return report, errors.New("benchmark session randomness incomplete")
	}
	runSession := hex.EncodeToString(runID)
	report.RunID = runSession
	client := &http.Client{Timeout: cfg.Timeout, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	defer client.CloseIdleConnections()
	raw, err := get(ctx, client, cfg.Endpoint+"/health")
	if err != nil {
		return report, err
	}
	health, err := object(raw)
	if err != nil || health["mode"] != "serving" || health["policy"] != "strict-local" {
		return report, errors.New("benchmark requires a serving strict-local gateway")
	}
	var runError error
	for _, role := range cfg.Roles {
		for pair := 1; pair <= cfg.Pairs; pair++ {
			sessionID := fmt.Sprintf("labbench-%s-%s-%d", runSession, role, pair)
			for _, phase := range []string{"first", "repeat"} {
				statsURL := cfg.StatsEndpoint
				if statsURL == "" {
					statsURL = cfg.Endpoint + "/sentinel/status"
				}
				s := Sample{Pair: pair, Phase: phase, Role: role, Protocol: "openai_chat_completions", Status: "error", UsageSource: "unavailable"}
				s.Before = sampleStats(ctx, client, statsURL, role)
				beforeActivity := sampleActivity(ctx, client, cfg.Endpoint)
				prompt := strings.Repeat("Synthetic benchmark reference: Cedar is blue. Birch is green. Oak is red. ", 32) + fmt.Sprintf("\nPair %d: Reply with Cedar's color only.", pair)
				requestBody, _ := json.Marshal(map[string]any{"model": role, "messages": []map[string]string{{"role": "user", "content": prompt}}, "max_tokens": 32, "temperature": 0, "stream": false})
				request, _ := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint+"/v1/chat/completions", bytes.NewReader(requestBody))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("X-Session-ID", sessionID)
				s.StartedAt = time.Now().UTC()
				started := time.Now()
				response, requestErr := client.Do(request)
				var body []byte
				if requestErr == nil {
					s.HTTPStatus = response.StatusCode
					body, requestErr = readResponse(response)
				}
				s.TotalMS = float64(time.Since(started)) / float64(time.Millisecond)
				if requestErr == nil {
					m, decodeErr := object(body)
					if decodeErr != nil {
						requestErr = errors.New("invalid completion JSON")
					} else {
						if m["error"] != nil {
							requestErr = errors.New("completion reported an error")
						}
						choices, ok := m["choices"].([]any)
						if !ok || len(choices) != 1 {
							requestErr = errors.New("expected one completion choice")
						} else {
							choice, _ := choices[0].(map[string]any)
							message := nested(choice, "message")
							s.Output, _ = message["content"].(string)
							if strings.TrimSpace(s.Output) == "" {
								requestErr = errors.New("completion text unavailable")
							}
						}
						s.ResponseModel, _ = m["model"].(string)
						usage := nested(m, "usage")
						s.InputTokens = number(usage, "prompt_tokens", true)
						s.OutputTokens = number(usage, "completion_tokens", true)
						for _, field := range []string{"prompt_tokens", "completion_tokens"} {
							if value, present := usage[field]; present && value != nil && number(usage, field, true) == nil {
								requestErr = errors.New("invalid completion token accounting")
							}
						}
						if s.InputTokens != nil || s.OutputTokens != nil {
							s.UsageSource = "completion_response"
						}
					}
				}
				if len(s.Output) > 4096 {
					s.OutputTruncated = true
					s.Output = s.Output[:4096]
					for !utf8.ValidString(s.Output) {
						s.Output = s.Output[:len(s.Output)-1]
					}
				}
				s.After = sampleStats(ctx, client, statsURL, role)
				afterActivity := sampleActivity(ctx, client, cfg.Endpoint)
				s.Cache, s.TTFT, s.NativeDecode, s.ObservedRole = cacheEvidence(s.Before, s.After, beforeActivity, afterActivity, s.StartedAt, role)
				if requestErr != nil {
					s.Error = requestErr.Error()
					runError = requestErr
				} else {
					s.Status = "success"
				}
				report.Samples = append(report.Samples, s)
				if runError != nil {
					summarize(&report)
					return report, runError
				}
			}
		}
	}
	summarize(&report)
	return report, nil
}
func summarize(r *Report) {
	values := []float64{}
	for _, s := range r.Samples {
		if s.Status == "success" {
			values = append(values, s.TotalMS)
		}
	}
	sort.Float64s(values)
	r.Summary.Successful = len(values)
	if len(values) > 0 {
		p50 := values[int(math.Ceil(.5*float64(len(values))))-1]
		p95 := values[int(math.Ceil(.95*float64(len(values))))-1]
		r.Summary.P50 = &p50
		r.Summary.P95 = &p95
	}
}

// Load returns a bounded, typed report; unrecognized input fields are not exposed.
func Load(root string) (Report, error) {
	return loadReport(filepath.Join(root, "benchmark-latest.json"))
}

func loadReport(path string) (Report, error) {
	var report Report
	info, err := os.Lstat(path)
	if err != nil {
		return report, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBody {
		return report, errors.New("benchmark report must be a bounded regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}
	if _, err := object(raw); err != nil {
		return report, err
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		return report, err
	}
	if report.Version != 1 || report.Scope != scope || report.Timestamp.IsZero() || len(report.Samples) > 120 {
		return Report{}, errors.New("invalid benchmark report identity or sample count")
	}
	for i := range report.Samples {
		s := &report.Samples[i]
		if math.IsNaN(s.TotalMS) || math.IsInf(s.TotalMS, 0) || s.TotalMS < 0 || s.Pair < 1 || s.Pair > 20 {
			return Report{}, errors.New("invalid benchmark sample")
		}
		if (s.Role != "haiku" && s.Role != "sonnet" && s.Role != "opus") || (s.Phase != "first" && s.Phase != "repeat") || s.Protocol != "openai_chat_completions" || (s.Status != "success" && s.Status != "error") {
			return Report{}, errors.New("invalid benchmark sample identity")
		}
		values := []*float64{s.InputTokens, s.OutputTokens, s.TTFT, s.NativeDecode, s.Cache.ReusedDelta, s.Cache.ProcessedDelta, s.Cache.CachedTokens}
		for _, snapshot := range []Snapshot{s.Before, s.After} {
			values = append(values, snapshot.Requests, snapshot.Uptime, snapshot.Reused, snapshot.Processed, snapshot.Cached, snapshot.TTFT, snapshot.Decode)
		}
		for _, n := range values {
			if n != nil && (math.IsNaN(*n) || math.IsInf(*n, 0) || *n < 0) {
				return Report{}, errors.New("invalid benchmark measurement")
			}
		}
		normalizePersistedAttribution(s)
	}
	report.Summary = Summary{}
	summarize(&report)
	return report, nil
}

var writeReport = os.WriteFile

func save(path string, report Report) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("output must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if report.RunID == "" {
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			return err
		}
		report.RunID = hex.EncodeToString(id)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".benchmark-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() { temporary.Close(); os.Remove(name) }()
	if err := writeReport(name, append(raw, '\n'), 0600); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := archiveReport(filepath.Dir(path), report, append(raw, '\n')); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func Run(args []string, _ io.Reader, out, stderr io.Writer) int {
	return RunContext(context.Background(), args, nil, out, stderr)
}

// RunContext lets owned CI lifecycles cancel pending benchmark requests.
func RunContext(ctx context.Context, args []string, _ io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("lab bench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("endpoint", "http://127.0.0.1:19090", "Strict-local gateway loopback HTTP endpoint")
	roles := flags.String("roles", "haiku", "Comma-separated haiku,sonnet,opus roles")
	role := flags.String("role", "", "Single role, overrides --roles")
	pairs := flags.Int("pairs", 3, "Paired fixed-prefix requests per role (1..20)")
	timeout := flags.Int("timeout", 300, "HTTP timeout in seconds (1..600)")
	output := flags.String("output", filepath.Join(".sentinel-lab", "benchmark-latest.json"), "Report file (empty for stdout only)")
	stats := flags.String("stats-endpoint", "", "Optional literal-loopback native telemetry endpoint, single role only")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}
	if *role != "" {
		*roles = *role
	}
	cfg := Config{Endpoint: *endpoint, StatsEndpoint: *stats, Roles: strings.Split(*roles, ","), Pairs: *pairs, Timeout: time.Duration(*timeout) * time.Second}
	if _, err := validate(cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	report, runErr := Benchmark(ctx, cfg)
	if *output != "" {
		if err := save(*output, report); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := json.NewEncoder(out).Encode(report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if runErr != nil {
		fmt.Fprintln(stderr, runErr)
		return 1
	}
	return 0
}

// Persisted native attribution must retain the role and isolated counter evidence
// that the live benchmark required. Unsupported values remain unknown.
func normalizePersistedAttribution(s *Sample) {
	before, after := s.Before, s.After
	clear := func() {
		s.TTFT = nil
		s.NativeDecode = nil
		s.ObservedRole = ""
		s.Cache = CacheEvidence{Status: "unknown", Provenance: "unavailable", Reason: "persisted attribution lacks matching role and native counter evidence"}
	}
	for _, counter := range []*float64{before.Requests, after.Requests, before.Reused, after.Reused, before.Processed, after.Processed, after.Cached, s.Cache.ReusedDelta, s.Cache.ProcessedDelta, s.Cache.CachedTokens} {
		if counter != nil && *counter != math.Trunc(*counter) {
			clear()
			return
		}
	}
	if s.ObservedRole != s.Role || s.StartedAt.IsZero() || before.Error != "" || after.Error != "" || before.Source == "" || before.Source != after.Source || before.Model == "" || before.Model != after.Model || before.Requests == nil || after.Requests == nil || *after.Requests-*before.Requests != 1 || before.Uptime == nil || after.Uptime == nil || *after.Uptime < *before.Uptime || before.Reused == nil || after.Reused == nil || before.Processed == nil || after.Processed == nil || *after.Reused < *before.Reused || *after.Processed < *before.Processed {
		clear()
		return
	}
	equal := func(a, b *float64) bool { return a != nil && b != nil && *a == *b }
	reused, processed := *after.Reused-*before.Reused, *after.Processed-*before.Processed
	if s.Cache.Provenance != "isolated_runtime_counter_window" || !equal(s.Cache.ReusedDelta, &reused) || !equal(s.Cache.ProcessedDelta, &processed) {
		clear()
		return
	}
	if !after.Native || after.Cached == nil {
		s.TTFT = nil
		s.NativeDecode = nil
		s.Cache.Status = "unknown"
		s.Cache.CachedTokens = nil
		s.Cache.Reason = "native per-generation cache count unavailable"
		return
	}
	status := "zero_cached_tokens_observed"
	if *after.Cached > 0 {
		status = "cached_tokens_observed"
	}
	if s.Cache.Status != status || !equal(s.Cache.CachedTokens, after.Cached) {
		clear()
		return
	}
	if !equal(s.TTFT, after.TTFT) {
		s.TTFT = nil
	}
	if !equal(s.NativeDecode, after.Decode) {
		s.NativeDecode = nil
	}
}
