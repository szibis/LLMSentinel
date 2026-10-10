// Package clientcapture implements opt-in local CLI conversation evidence capture.
package clientcapture

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxEventBytes      = 1024 * 1024
	maxTranscriptBytes = 8 * 1024 * 1024
	maxRows            = 512
	maxText            = 32768
	maxRecordBytes     = 512 * 1024
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]{8,}|AKIA[A-Z0-9]{16})\b`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|password|secret|authorization)["']?\s*[:=]\s*["']?[^\s,"'}]+`),
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
}

func mapValue(value any) map[string]any { result, _ := value.(map[string]any); return result }
func stringValue(value any) string      { result, _ := value.(string); return result }

func decodeObject(raw []byte) (map[string]any, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("invalid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("expected one JSON object")
	}
	return object, nil
}

func canonicalPath(value string) (string, error) {
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", errors.New("path must be canonical and absolute")
	}
	current, suffix := value, []string{}
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			if resolved != value {
				return "", errors.New("path contains symlink")
			}
			return value, nil
		}
		if !os.IsNotExist(err) || current == filepath.Dir(current) {
			return "", err
		}
		// EvalSymlinks also reports ENOENT for a dangling link. Distinguish
		// that existing unsafe entry from a genuinely absent output filename.
		if info, statErr := os.Lstat(current); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("path contains symlink")
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return "", statErr
		}
		suffix = append(suffix, filepath.Base(current))
		current = filepath.Dir(current)
	}
}

func readTranscript(path string, includeHeader bool) ([]map[string]any, bool, error) {
	canonical, err := canonicalPath(path)
	if err != nil {
		return nil, false, err
	}
	file, err := openPrivate(canonical, false)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	var header []byte
	if includeHeader {
		header, err = io.ReadAll(io.LimitReader(file, 256*1024))
		if err != nil {
			return nil, false, err
		}
		if index := bytes.IndexByte(header, '\n'); index >= 0 {
			header = header[:index]
		}
	}
	start := max(int64(0), info.Size()-maxTranscriptBytes)
	if _, err = file.Seek(start, io.SeekStart); err != nil {
		return nil, false, err
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxTranscriptBytes))
	if err != nil {
		return nil, false, err
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	clipped := start > 0 || len(lines) > maxRows
	if len(lines) > maxRows {
		lines = lines[len(lines)-maxRows:]
	}
	rows := make([]map[string]any, 0, len(lines)+1)
	if len(header) > 0 {
		if metadata, e := decodeObject(header); e == nil && metadata["type"] == "session_meta" {
			rows = append(rows, metadata)
		}
	}
	for _, line := range lines {
		if row, e := decodeObject(line); e == nil {
			rows = append(rows, row)
		}
	}
	return rows, clipped, nil
}

func contentText(content any, kind string) string {
	if text, ok := content.(string); ok && kind == "text" {
		return text
	}
	items, _ := content.([]any)
	parts := []string{}
	for _, item := range items[:min(len(items), maxRows)] {
		block := mapValue(item)
		if block["type"] == kind {
			if text, ok := block["text"].(string); ok {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func integer(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Int64()
	return parsed, err == nil && parsed >= 0
}

func checkedAdd(left, right int64) (int64, bool) {
	if right > math.MaxInt64-left {
		return 0, false
	}
	return left + right, true
}

func codexUsage(value any) map[string]int64 {
	input := mapValue(value)
	result := map[string]int64{}
	for _, key := range []string{"input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens"} {
		number, ok := integer(input[key])
		if !ok {
			return nil
		}
		result[key] = number
	}
	total, ok := checkedAdd(result["input_tokens"], result["output_tokens"])
	if !ok || total != result["total_tokens"] || result["cached_input_tokens"] > result["input_tokens"] || result["reasoning_output_tokens"] > result["output_tokens"] {
		return nil
	}
	return result
}

func enrichCodex(record, event map[string]any) error {
	rows, clipped, err := readTranscript(stringValue(event["transcript_path"]), true)
	if err != nil {
		return err
	}
	record["truncated"] = clipped
	metadata := map[string]any{}
	for _, row := range rows {
		if row["type"] == "session_meta" {
			metadata = mapValue(row["payload"])
			break
		}
	}
	record["capture_metadata"] = map[string]any{"cli_version": metadata["cli_version"], "model_provider": metadata["model_provider"]}
	session, turn := stringValue(record["session_id"]), stringValue(record["turn_id"])
	if metadata["cli_version"] != "0.160.1" || metadata["id"] != session || session == "" || turn == "" {
		return nil
	}
	scoped := []map[string]any{}
	current := ""
	for _, row := range rows {
		payload := mapValue(row["payload"])
		if payload == nil {
			continue
		}
		if row["type"] == "event_msg" && (payload["type"] == "task_started" || payload["type"] == "turn_started") {
			current = stringValue(payload["turn_id"])
		} else if row["type"] == "turn_context" {
			current = stringValue(payload["turn_id"])
			if current == turn && stringValue(payload["model"]) != "" {
				record["model"] = payload["model"]
			}
		}
		if current == turn {
			scoped = append(scoped, row)
		}
	}
	texts := map[string][]string{"user": {}, "assistant": {}}
	seen := map[string]bool{}
	for _, row := range scoped {
		payload := mapValue(row["payload"])
		role := stringValue(payload["role"])
		if row["type"] != "response_item" || payload["type"] != "message" || (role != "user" && role != "assistant") {
			continue
		}
		kind := "input_text"
		if role == "assistant" {
			kind = "output_text"
		}
		text := contentText(payload["content"], kind)
		key := role + "\x00" + text
		if text != "" && !seen[key] {
			texts[role] = append(texts[role], text)
			seen[key] = true
		}
	}
	if len(texts["user"]) > 0 {
		record["inputs"] = texts["user"]
	}
	if len(texts["assistant"]) > 0 {
		record["outputs"] = texts["assistant"]
	}
	responses := map[string]map[string]int64{}
	metaSession := metadata["session_id"]
	if metaSession == nil {
		metaSession = metadata["id"]
	}
	for _, row := range rows {
		payload := mapValue(row["payload"])
		if row["type"] == "token_usage_record" && payload["thread_id"] == session && payload["session_id"] == metaSession && payload["turn_id"] == turn && stringValue(payload["response_id"]) != "" {
			if usage := codexUsage(payload["usage"]); usage != nil {
				responses[stringValue(payload["response_id"])] = usage
			}
		}
	}
	var usage map[string]int64
	source, scope := "codex_rollout_token_count", "last_call"
	if len(responses) > 0 {
		usage = map[string]int64{}
		for _, response := range responses {
			for key, value := range response {
				sum, ok := checkedAdd(usage[key], value)
				if !ok {
					return nil
				}
				usage[key] = sum
			}
		}
		source, scope = "codex_rollout_token_usage_record", "observed_turn_calls"
	} else {
		var info map[string]any
		for _, row := range scoped {
			payload := mapValue(row["payload"])
			if row["type"] == "event_msg" && payload["type"] == "token_count" {
				info = mapValue(payload["info"])
			}
		}
		usage = codexUsage(info["last_token_usage"])
	}
	if usage != nil {
		target := mapValue(record["usage"])
		for _, key := range []string{"input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens"} {
			target[key] = usage[key]
		}
		target["cache_read_input_tokens"] = usage["cached_input_tokens"]
		target["source"], target["scope"] = source, scope
		target["reported_calls"] = nil
		if len(responses) > 0 {
			target["reported_calls"] = len(responses)
		}
		target["coverage"], target["provenance"] = "reported_calls_only", "local_cli_record"
	}
	return nil
}

func enrichClaude(record, event map[string]any) error {
	rows, clipped := []map[string]any{}, false
	if path := stringValue(event["transcript_path"]); path != "" {
		var err error
		rows, clipped, err = readTranscript(path, false)
		if err != nil {
			return err
		}
	}
	record["truncated"] = clipped
	start := -1
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i]["type"] == "user" && contentText(mapValue(rows[i]["message"])["content"], "text") != "" {
			start = i
			break
		}
	}
	if start >= 0 {
		rows = rows[start:]
		record["inputs"] = []string{contentText(mapValue(rows[0]["message"])["content"], "text")}
		record["turn_id"] = rows[0]["uuid"]
	} else {
		rows = nil
	}
	messages := map[string]map[string]any{}
	order := []string{}
	seen := map[string]bool{}
	outputs := []string{}
	for index, row := range rows {
		message := mapValue(row["message"])
		if row["type"] != "assistant" || message == nil {
			continue
		}
		id := stringValue(message["id"])
		if id == "" {
			id = stringValue(row["uuid"])
		}
		if id == "" {
			id = strconv.Itoa(index)
		}
		if _, exists := messages[id]; !exists {
			order = append(order, id)
		}
		messages[id] = row
		text := contentText(message["content"], "text")
		if text != "" && !seen[id+"\x00"+text] {
			outputs = append(outputs, text)
			seen[id+"\x00"+text] = true
		}
	}
	for _, id := range order {
		model := mapValue(messages[id]["message"])["model"]
		if stringValue(model) != "" {
			record["model"] = model
		}
	}
	if len(outputs) == 0 && stringValue(event["last_assistant_message"]) != "" {
		outputs = append(outputs, stringValue(event["last_assistant_message"]))
	}
	record["outputs"] = outputs
	if len(messages) > 0 {
		usage := mapValue(record["usage"])
		for _, key := range []string{"input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens"} {
			sum := int64(0)
			known := true
			for _, message := range messages {
				value, ok := integer(mapValue(mapValue(message["message"])["usage"])[key])
				if !ok {
					known = false
					break
				}
				sum, ok = checkedAdd(sum, value)
				if !ok {
					known = false
					break
				}
			}
			if known {
				usage[key] = sum
			}
		}
		if usage["input_tokens"] != nil || usage["output_tokens"] != nil {
			usage["source"] = "claude_transcript_message_usage"
		}
		first, e1 := time.Parse(time.RFC3339Nano, stringValue(rows[0]["timestamp"]))
		last, e2 := time.Parse(time.RFC3339Nano, stringValue(messages[order[len(order)-1]]["timestamp"]))
		if e1 == nil && e2 == nil && !last.Before(first) {
			record["latency_ms"] = float64(last.Sub(first)) / float64(time.Millisecond)
			record["latency_source"] = "transcript_turn_span"
		}
	}
	return nil
}

func redact(value any) any {
	switch value := value.(type) {
	case string:
		for _, pattern := range secretPatterns {
			value = pattern.ReplaceAllString(value, "[REDACTED]")
		}
		if utf8.RuneCountInString(value) > maxText {
			value = string([]rune(value)[:maxText])
		}
		return value
	case []string:
		result := make([]string, min(len(value), maxRows))
		for i := range result {
			result[i] = redact(value[i]).(string)
		}
		return result
	case []any:
		result := make([]any, min(len(value), maxRows))
		for i := range result {
			result[i] = redact(value[i])
		}
		return result
	case map[string]any:
		for key, item := range value {
			value[key] = redact(item)
		}
		return value
	default:
		return value
	}
}

func normalize(client, billing string, event map[string]any) (map[string]any, error) {
	var identifier [16]byte
	if _, err := rand.Read(identifier[:]); err != nil {
		return nil, err
	}
	identifier[6] = identifier[6]&0x0f | 0x40
	identifier[8] = identifier[8]&0x3f | 0x80
	id := hex.EncodeToString(identifier[:])
	id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	billingSource := "unknown"
	if billing != "direct_unknown" {
		billingSource = "explicit_cli_option"
	}
	record := map[string]any{"schema_version": 1, "event_id": id, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "source": "client_hook", "client": client, "model": nil, "role": "conversation", "run_id": nil, "session_id": nil, "turn_id": nil, "inputs": []string{}, "outputs": []string{}, "usage": map[string]any{"input_tokens": nil, "output_tokens": nil, "cache_read_input_tokens": nil, "cache_creation_input_tokens": nil, "source": "unavailable"}, "latency_ms": nil, "latency_source": "unavailable", "truncated": false, "billing": map[string]any{"class": billing, "source": billingSource, "cost_usd": nil, "cost_source": "unavailable"}, "quality": map[string]any{"status": "unscored", "training_eligible": false}, "pair_id": nil, "reference_event_id": nil, "local_event_id": nil}
	name := stringValue(event["hook_event_name"])
	if client == "codex" {
		if name == "UserPromptSubmit" || name == "Stop" {
			record["event_type"], record["session_id"], record["turn_id"], record["model"] = name, event["session_id"], event["turn_id"], event["model"]
			if name == "UserPromptSubmit" {
				if prompt, ok := event["prompt"].(string); ok {
					record["inputs"] = []string{prompt}
				}
			} else {
				if text, ok := event["last_assistant_message"].(string); ok {
					record["outputs"] = []string{text}
				}
				if stringValue(event["transcript_path"]) != "" {
					if err := enrichCodex(record, event); err != nil {
						return nil, err
					}
				}
			}
		} else if event["type"] == "agent-turn-complete" {
			record["event_type"], record["session_id"], record["turn_id"] = event["type"], event["thread-id"], event["turn-id"]
			inputs := []string{}
			if value, exists := event["input-messages"]; exists {
				array, ok := value.([]any)
				if !ok {
					return nil, errors.New("invalid messages")
				}
				for _, item := range array {
					text, ok := item.(string)
					if !ok {
						return nil, errors.New("invalid message")
					}
					inputs = append(inputs, text)
				}
			}
			record["truncated"] = len(inputs) > maxRows
			record["inputs"] = inputs[:min(len(inputs), maxRows)]
			if text, ok := event["last-assistant-message"].(string); ok {
				record["outputs"] = []string{text}
			}
		} else {
			return nil, errors.New("unsupported Codex event")
		}
	} else {
		if name != "UserPromptSubmit" && name != "Stop" {
			return nil, errors.New("unsupported Claude event")
		}
		record["event_type"], record["session_id"] = name, event["session_id"]
		if name == "UserPromptSubmit" {
			prompt, ok := event["prompt"].(string)
			if !ok {
				return nil, errors.New("invalid prompt")
			}
			record["inputs"] = []string{prompt}
		} else if err := enrichClaude(record, event); err != nil {
			return nil, err
		}
	}
	inputs, outputs := record["inputs"].([]string), record["outputs"].([]string)
	for _, text := range append(append([]string{}, inputs...), outputs...) {
		if utf8.RuneCountInString(text) > maxText {
			record["truncated"] = true
		}
	}
	if len(inputs)+len(outputs) > 64 {
		record["inputs"] = inputs[max(0, len(inputs)-32):]
		record["outputs"] = outputs[max(0, len(outputs)-32):]
		record["truncated"] = true
	}
	return redact(record).(map[string]any), nil
}

func appendPrivate(path string, record map[string]any) error {
	canonical, err := canonicalPath(path)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if len(payload) > maxRecordBytes {
		return errors.New("record too large")
	}
	file, err := openPrivate(canonical, true)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = lockFile(file); err != nil {
		return err
	}
	if err = file.Chmod(0600); err != nil {
		return err
	}
	if _, err = file.Write(payload); err != nil {
		return err
	}
	return file.Sync()
}

func collectorAddress(value string) error {
	address, err := url.Parse(value)
	if err != nil {
		return err
	}
	if address.Scheme != "http" || (address.Hostname() != "127.0.0.1" && address.Hostname() != "::1") || address.User != nil || address.RawQuery != "" || address.ForceQuery || address.Fragment != "" {
		return errors.New("collector must be literal HTTP loopback")
	}
	if address.Port() != "" {
		port, e := strconv.Atoi(address.Port())
		if e != nil || port < 1 || port > 65535 {
			return errors.New("invalid collector port")
		}
	}
	return nil
}

func deliver(address string, record map[string]any) bool {
	payload, err := json.Marshal(record)
	if err != nil {
		return false
	}
	request, err := http.NewRequest(http.MethodPost, address, bytes.NewReader(payload))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	transport := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 300 * time.Millisecond, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 300 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func previewConfig(out io.Writer, client, output, collector, billing string, native bool) error {
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	command := []string{binary, "capture", "--client", client, "--output", output}
	if billing != "direct_unknown" {
		command = append(command, "--billing-class", billing)
	}
	if collector != "" {
		command = append(command, "--collector", collector)
	}
	if client == "codex" && !native {
		raw, e := json.Marshal(command)
		if e != nil {
			return e
		}
		_, err = fmt.Fprintf(out, "notify = %s\n", raw)
		return err
	}
	quoted := make([]string, len(command))
	for i, value := range command {
		quoted[i] = shellQuote(value)
	}
	hooks := map[string]any{}
	for _, name := range []string{"UserPromptSubmit", "Stop"} {
		hooks[name] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": strings.Join(quoted, " "), "timeout": 5}}}}
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(map[string]any{"hooks": hooks})
}

// Run captures one stdin hook or a final argv legacy notify event. Previews install nothing.
func Run(args []string, in io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("capture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	client := flags.String("client", "", "claude or codex")
	output := flags.String("output", "", "canonical absolute private JSONL file")
	collector := flags.String("collector", "", "optional literal HTTP loopback URL")
	billing := flags.String("billing-class", "direct_unknown", "direct_unknown, subscription or api")
	native := flags.Bool("native-hooks", false, "preview native Codex hooks")
	preview := flags.Bool("preview-config", false, "print configuration only")
	mod := flags.Bool("mod-event", false, "capture a structured native Claude mod observation")
	fail := func() int {
		_, _ = fmt.Fprintln(stderr, "Sentinel capture skipped: invalid event or inaccessible private file")
		return 1
	}
	if flags.Parse(args) != nil || (*client != "claude" && *client != "codex") || (*billing != "direct_unknown" && *billing != "subscription" && *billing != "api") || flags.NArg() > 1 || (*mod && (*client != "claude" || *preview || *native)) {
		return fail()
	}
	if _, err := canonicalPath(*output); err != nil {
		return fail()
	}
	if *collector != "" {
		if err := collectorAddress(*collector); err != nil {
			return fail()
		}
	}
	if *preview {
		if err := previewConfig(out, *client, *output, *collector, *billing, *native); err != nil {
			return fail()
		}
		return 0
	}
	var raw []byte
	var err error
	if flags.NArg() == 1 {
		raw = []byte(flags.Arg(0))
	} else {
		if in == nil {
			return fail()
		}
		raw, err = io.ReadAll(io.LimitReader(in, maxEventBytes+1))
	}
	if err != nil || len(raw) > maxEventBytes {
		return fail()
	}
	event, err := decodeObject(raw)
	if err != nil {
		return fail()
	}
	var record map[string]any
	if *mod {
		record, err = normalizeMod(*billing, event)
	} else {
		record, err = normalize(*client, *billing, event)
	}
	if err != nil {
		return fail()
	}
	if err = appendPrivate(*output, record); err != nil {
		return fail()
	}
	if *collector != "" && !deliver(*collector, record) {
		_, _ = fmt.Fprintln(stderr, "Sentinel collector unavailable; private capture spool retained")
	}
	return 0
}
