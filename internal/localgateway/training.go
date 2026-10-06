package localgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// TrainingConfig explicitly opts in to private conversation capture. Nil disables it.
type TrainingConfig struct {
	Directory string
	MaxBytes  int64
}

type trainingEvent struct {
	Timestamp string         `json:"timestamp"`
	Provider  string         `json:"provider"`
	Client    string         `json:"client"`
	RequestID string         `json:"request_id"`
	AttemptID string         `json:"attempt_id"`
	PairID    string         `json:"pair_id,omitempty"`
	Model     string         `json:"model"`
	Role      string         `json:"role"`
	Reason    string         `json:"reason,omitempty"`
	Thinking  bool           `json:"thinking"`
	MaxTokens int            `json:"max_tokens"`
	Input     any            `json:"input"`
	Output    string         `json:"output"`
	Usage     map[string]int `json:"usage,omitempty"`
	LatencyMS int64          `json:"latency_ms"`
	Accepted  bool           `json:"accepted"`
	Error     string         `json:"error,omitempty"`
	Quality   map[string]any `json:"quality"`
	Reference any            `json:"reference,omitempty"`
}

type trainingContextKey uint8

const (
	trainingClientKey trainingContextKey = iota
	trainingRequestIDKey
)

func withTrainingClient(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, trainingClientKey, name)
}
func trainingClient(ctx context.Context) string {
	value, _ := ctx.Value(trainingClientKey).(string)
	return value
}
func withTrainingRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, trainingRequestIDKey, id)
}
func trainingRequestID(ctx context.Context) string {
	value, _ := ctx.Value(trainingRequestIDKey).(string)
	return value
}

type trainingRecorder struct {
	paused    atomic.Bool
	mu        sync.Mutex
	directory string
	maxBytes  int64
}

func newTrainingRecorder(config *TrainingConfig) (*trainingRecorder, error) {
	if config == nil {
		return nil, nil
	}
	if strings.TrimSpace(config.Directory) == "" || config.MaxBytes < 0 {
		return nil, errors.New("training capture requires a directory and nonnegative byte limit")
	}
	directory, err := filepath.Abs(config.Directory)
	if err != nil {
		return nil, errors.New("invalid training capture directory")
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, errors.New("cannot create private training capture directory")
	}
	if err = privateTrainingDirectory(directory); err != nil {
		return nil, err
	}
	maxBytes := config.MaxBytes
	if maxBytes == 0 {
		maxBytes = 32 * 1024 * 1024
	}
	r := &trainingRecorder{directory: directory, maxBytes: maxBytes}
	for _, name := range []string{"training.jsonl", "training.jsonl.1"} {
		path := filepath.Join(directory, name)
		if _, err := os.Lstat(path); os.IsNotExist(err) && name == "training.jsonl.1" {
			continue
		}
		file, err := openPrivateTrainingFile(path)
		if err != nil {
			return nil, err
		}
		info, err := file.Stat()
		file.Close()
		if err != nil || info.Size() > maxBytes {
			return nil, errors.New("existing training capture exceeds configured byte limit")
		}
	}
	return r, nil
}

func privateTrainingDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return errors.New("training capture directory must be a private regular directory (0700)")
	}
	return nil
}

func openPrivateTrainingFile(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return nil, errors.New("training capture file must be regular and private (0600)")
		}
	} else if !os.IsNotExist(err) {
		return nil, errors.New("cannot inspect private training capture file")
	}
	// O_NOFOLLOW also rejects a link swapped in between validation and open.
	// Nonblocking avoids hanging on a FIFO swapped into the same location.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, errors.New("cannot open private training capture file")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		file.Close()
		return nil, errors.New("training capture file must be regular and private (0600)")
	}
	return file, nil
}

// record never affects model request success. Storage and serialization errors
// are reported without paths, conversation content, credentials or raw errors.
func (r *trainingRecorder) record(ctx context.Context, event trainingEvent) {
	if r == nil || r.paused.Load() {
		return
	}
	event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	if event.Provider == "" {
		event.Provider = "local"
	}
	if event.Client == "" {
		event.Client = trainingClient(ctx)
	}
	if event.RequestID == "" {
		event.RequestID = trainingRequestID(ctx)
	}
	quality := make(map[string]any, len(event.Quality)+2)
	for key, value := range event.Quality {
		quality[key] = value
	}
	if _, ok := quality["status"]; !ok {
		quality["status"] = "unscored"
	}
	// Capture is a candidate collection, never an automatic training admission.
	quality["training_eligible"] = false
	event.Quality = quality
	encoded, err := json.Marshal(event)
	if err != nil {
		log.Print("Training capture skipped: unsupported event data")
		return
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err = decoder.Decode(&value); err != nil {
		log.Print("Training capture skipped: unsupported event data")
		return
	}
	encoded, err = json.Marshal(redactTrainingValue(value))
	if err != nil {
		log.Print("Training capture skipped: unsupported event data")
		return
	}
	encoded = append(encoded, '\n')
	if int64(len(encoded)) > r.maxBytes {
		log.Print("Training capture skipped: event exceeds byte limit")
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err = r.append(encoded); err != nil {
		log.Print("Training capture skipped: private storage unavailable")
	}
}

func (r *trainingRecorder) append(encoded []byte) error {
	if err := privateTrainingDirectory(r.directory); err != nil {
		return err
	}
	path := filepath.Join(r.directory, "training.jsonl")
	file, err := openPrivateTrainingFile(path)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	if info.Size()+int64(len(encoded)) > r.maxBytes {
		file.Close()
		previous := path + ".1"
		if info, err := os.Lstat(previous); err == nil {
			if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
				return errors.New("unsafe previous training capture")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err = os.Rename(path, previous); err != nil {
			return err
		}
		file, err = openPrivateTrainingFile(path)
		if err != nil {
			return err
		}
	}
	defer file.Close()
	_, err = file.Write(encoded)
	return err
}

var trainingSecrets = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----.*?-----END (?:[A-Z ]+ )?PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|secret|password)\s*[:=]\s*[^\s,;]+`),
}

func redactTrainingValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			normalized := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(key))
			switch normalized {
			case "authorization", "proxy_authorization", "api_key", "apikey", "anthropic_api_key", "openai_api_key", "access_token", "refresh_token", "password", "secret", "client_secret", "aws_secret_access_key", "aws_session_token":
				value[key] = "[REDACTED]"
			default:
				value[key] = redactTrainingValue(item)
			}
		}
		return value
	case []any:
		for i, item := range value {
			value[i] = redactTrainingValue(item)
		}
		return value
	case string:
		for _, pattern := range trainingSecrets {
			value = pattern.ReplaceAllString(value, "[REDACTED]")
		}
		return value
	default:
		return value
	}
}
