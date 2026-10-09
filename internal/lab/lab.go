// Package lab owns isolated native-client profiles and the local lab lifecycle.
package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/szibis/claude-escalate/internal/clientcontrol"
)

const endpoint = "http://127.0.0.1:19090"
const token = "sentinel-local-lab"

type environment struct {
	project, root, executable string
	in                        io.Reader
	out, stderr               io.Writer
	transport                 http.RoundTripper // Optional instance-scoped transport for isolated protocol tests.
	portCheck                 func([]int, time.Duration) error
}

func newEnvironment(in io.Reader, out, stderr io.Writer) (*environment, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, err
	}
	project := os.Getenv("SENTINEL_PROJECT_ROOT")
	if project == "" {
		project = filepath.Dir(filepath.Dir(executable))
	}
	root := os.Getenv("SENTINEL_LAB_ROOT")
	if root == "" {
		root = filepath.Join(project, ".sentinel-lab")
	}
	project, err = filepath.Abs(project)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &environment{project: project, root: root, executable: executable, in: in, out: out, stderr: stderr}, nil
}
func Run(args []string, in io.Reader, out, stderr io.Writer) int {
	e, err := newEnvironment(in, out, stderr)
	if err == nil {
		err = e.runClient(args)
	}
	if err != nil {
		fmt.Fprintln(stderr, "Sentinel lab:", err)
		return 1
	}
	return 0
}
func RunRunner(args []string, in io.Reader, out, stderr io.Writer) int {
	e, err := newEnvironment(in, out, stderr)
	if err == nil {
		err = e.runRunner(args)
	}
	if err != nil {
		fmt.Fprintln(stderr, "Sentinel lab:", err)
		return 1
	}
	return 0
}
func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicFile(path, append(data, '\n'))
}
func atomicFile(path string, data []byte) error {
	if err := privateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := regularDestination(path); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path) // #nosec G703 -- destination is the caller's explicit local lab/profile path.
}
func seedFile(path string, data []byte) error {
	if err := privateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := regularDestination(path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}
func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func privateDirectory(path string) error {
	if err := clientcontrol.ValidateDirectory(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0700) // #nosec G703 -- every existing ancestor was checked for symlinks above.
}
func regularDestination(path string) error {
	info, err := os.Lstat(path) // #nosec G703 -- checks the caller-selected private lab file before any write; never derived from model or HTTP input.
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("lab destination must be a regular file without symlinks")
	}
	return nil
}
func (e *environment) prepare() error {
	for _, name := range []string{"codex", "claude", "workspace", "tmp"} {
		if err := privateDirectory(filepath.Join(e.root, name)); err != nil {
			return err
		}
	}
	workspace := filepath.Join(e.root, "workspace")
	if _, err := os.Stat(filepath.Join(workspace, ".git")); os.IsNotExist(err) {
		cmd := exec.Command("git", "init", "-q", workspace)
		cmd.Env = allowedEnvironment("HOME", "PATH", "LANG", "LC_ALL")
		if result, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("prepare isolated workspace: %w: %s", err, result)
		}
	}
	settings := map[string]any{"model": "local", "permissions": map[string]string{"defaultMode": "default"}, "enableAllProjectMcpServers": false, "statusLine": map[string]any{"type": "command", "command": quote(e.executable) + " statusline --root " + quote(e.root), "refreshInterval": 5}}
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := seedFile(filepath.Join(e.root, "claude", "settings.json"), append(data, '\n')); err != nil {
		return err
	}
	// Migrate only our generated legacy command, preserving every preference and
	// any user-supplied status line. The old supervisor can keep running.
	settingsPath := filepath.Join(e.root, "claude", "settings.json")
	var existing map[string]any
	if err := readJSON(settingsPath, &existing); err != nil {
		return err
	}
	if existing == nil {
		return errors.New("claude settings must be a JSON object")
	}
	if status, _ := existing["statusLine"].(map[string]any); status != nil {
		command, _ := status["command"].(string)
		if e.ownedLegacyStatus(command) {
			status["command"] = quote(e.executable) + " statusline --root " + quote(e.root)
			if err := writeJSON(settingsPath, existing); err != nil {
				return err
			}
		}
	}
	config := `model = "local"
model_provider = "sentinel"
approval_policy = "on-request"
sandbox_mode = "read-only"
web_search = "disabled"
cli_auth_credentials_store = "file"

[model_providers.sentinel]
name = "Sentinel isolated local lab"
env_http_headers = { "X-Session-ID" = "SENTINEL_LAB_SESSION" }
base_url = "http://127.0.0.1:19090/v1"
env_key = "SENTINEL_LAB_TOKEN"
wire_api = "responses"
requires_openai_auth = false
request_max_retries = 0
stream_max_retries = 0
stream_idle_timeout_ms = 300000

[analytics]
enabled = false
`
	if err := seedFile(filepath.Join(e.root, "codex", "config.toml"), []byte(config)); err != nil {
		return err
	}
	catalogPath := filepath.Join(e.root, "codex", "sentinel-model-catalog.json")
	if err := seedFile(catalogPath, clientcontrol.CodexModelCatalog()); err != nil {
		return err
	}
	configPath := filepath.Join(e.root, "codex", "config.toml")
	if err := regularDestination(configPath); err != nil {
		return err
	}
	current, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	if !regexp.MustCompile(`(?m)^\s*model_catalog_json\s*=`).Match(current) {
		current = append([]byte("model_catalog_json = "+strconv.Quote(catalogPath)+"\n"), current...)
	}
	// Only migrate the exact generated provider header. Custom providers and
	// explicit HTTP-header settings remain user-owned.
	if !strings.Contains(string(current), "http_headers") {
		current = []byte(strings.Replace(string(current), "[model_providers.sentinel]\nname = \"Sentinel isolated local lab\"\n", "[model_providers.sentinel]\nname = \"Sentinel isolated local lab\"\nenv_http_headers = { \"X-Session-ID\" = \"SENTINEL_LAB_SESSION\" }\n", 1))
	}
	if err := atomicFile(configPath, current); err != nil {
		return err
	}
	for _, client := range []string{"claude", "codex"} {
		assets, err := clientcontrol.Assets(client, endpoint, e.executable)
		if err != nil {
			return err
		}
		directory := filepath.Join(e.root, client, "commands")
		if client == "codex" {
			directory = filepath.Join(e.root, "codex", "prompts")
		}
		for name, content := range assets {
			if filepath.Base(name) != name {
				return errors.New("invalid command asset filename")
			}
			path := filepath.Join(directory, name)
			if err := privateDirectory(directory); err != nil {
				return err
			}
			if err := regularDestination(path); err != nil {
				return err
			}
			if data, err := os.ReadFile(path); err == nil && e.ownedLegacyAsset(string(data), content) {
				if err := atomicFile(path, []byte(content)); err != nil {
					return err
				}
			}
			if err := seedFile(path, []byte(content)); err != nil {
				return err
			}
			if client == "claude" {
				pluginPath := filepath.Join(e.root, "control-plugin", "skills", strings.TrimPrefix(strings.TrimSuffix(name, ".md"), "sentinel-"), "SKILL.md")
				if err := regularDestination(pluginPath); err != nil {
					return err
				}
				if err := privateDirectory(filepath.Dir(pluginPath)); err != nil {
					return err
				}
				if data, err := os.ReadFile(pluginPath); err == nil && e.ownedLegacyAsset(string(data), content) {
					if err := atomicFile(pluginPath, []byte(content)); err != nil {
						return err
					}
				}
				if err := seedFile(pluginPath, []byte(content)); err != nil {
					return err
				}
			}
		}
	}
	return seedFile(filepath.Join(e.root, "control-plugin", ".claude-plugin", "plugin.json"), []byte("{\"name\":\"sentinel\",\"version\":\"1.0.0\"}\n"))
}
func allowedEnvironment(keys ...string) []string {
	env := []string{}
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}
func (e *environment) clientEnvironment(client string) []string {
	env := allowedEnvironment("HOME", "TERM", "COLORTERM", "LANG", "LC_ALL")
	env = append(env, "PATH="+filepath.Join(e.root, "clients", "node_modules", ".bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+filepath.Join(e.root, "tmp"))
	if client == "codex" {
		return append(env, "CODEX_HOME="+filepath.Join(e.root, "codex"), "SENTINEL_LAB_TOKEN="+token, "SENTINEL_LAB_SESSION="+uuid.NewString())
	}
	env = append(env, "CLAUDE_CONFIG_DIR="+filepath.Join(e.root, "claude"), "ANTHROPIC_BASE_URL="+endpoint, "ANTHROPIC_API_KEY="+token, "ANTHROPIC_MODEL=opusplan", "ANTHROPIC_DEFAULT_HAIKU_MODEL=sentinel-haiku", "ANTHROPIC_DEFAULT_SONNET_MODEL=sentinel-sonnet", "ANTHROPIC_DEFAULT_OPUS_MODEL=sentinel-opus", "ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME=Haiku", "ANTHROPIC_DEFAULT_SONNET_MODEL_NAME=Sonnet", "ANTHROPIC_DEFAULT_OPUS_MODEL_NAME=Opus", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTOUPDATER=1", "DISABLE_UPDATES=1", "DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1", "MAX_THINKING_TOKENS=0", "CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1", "CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1", "ENABLE_TOOL_SEARCH=false")
	var settings runtimeSettings
	if readJSON(filepath.Join(e.root, "runtime.json"), &settings) == nil {
		minimum := 0
		for _, path := range []string{settings.ModelPath, settings.SmallModelPath} {
			var config struct {
				Max  int `json:"max_position_embeddings"`
				Text struct {
					Max int `json:"max_position_embeddings"`
				} `json:"text_config"`
			}
			if path != "" && readJSON(filepath.Join(path, "config.json"), &config) == nil {
				window := config.Max
				if window == 0 {
					window = config.Text.Max
				}
				if window > 0 && (minimum == 0 || window < minimum) {
					minimum = window
				}
			}
		}
		if minimum > 0 {
			env = append(env, "CLAUDE_CODE_MAX_CONTEXT_TOKENS="+strconv.Itoa(minimum))
		}
	}
	return env
}
func (e *environment) clientBinary(client string) (string, error) {
	path := filepath.Join(e.root, "clients", "node_modules", ".bin", client)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("lab %s is not installed; run make lab-clients-update; global clients are not used", client)
	}
	return path, nil
}
func localHTTP(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("local redirect refused") }}
}
func (e *environment) localHTTP(timeout time.Duration) *http.Client {
	client := localHTTP(timeout)
	if e.transport != nil {
		client.Transport = e.transport
	}
	return client
}
func fetchJSON(url string, timeout time.Duration) (map[string]any, error) {
	client := localHTTP(timeout)
	defer client.CloseIdleConnections()
	return fetchJSONWith(client, url)
}
func (e *environment) fetchJSON(url string, timeout time.Duration) (map[string]any, error) {
	client := e.localHTTP(timeout)
	defer client.CloseIdleConnections()
	return fetchJSONWith(client, url)
}
func fetchJSONWith(client *http.Client, url string) (map[string]any, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("local HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 {
		return nil, errors.New("invalid local health response")
	}
	var value map[string]any
	err = json.Unmarshal(data, &value)
	if err == nil && value == nil {
		return nil, errors.New("local health response must be a JSON object")
	}
	return value, err
}
func (e *environment) runClient(args []string) error {
	if len(args) != 1 {
		return errors.New("expected lab prepare|clients-update|doctor|claude|codex; arbitrary launch flags are refused")
	}
	if err := e.prepare(); err != nil {
		return err
	}
	switch args[0] {
	case "prepare":
		fmt.Fprintln(e.out, "Lab prepared:", e.root)
		return nil
	case "clients-update":
		return e.updateClients()
	case "doctor":
		return e.doctor()
	case "claude", "codex":
		return e.launch(args[0])
	default:
		return errors.New("unknown lab command")
	}
}
func (e *environment) updateClients() error {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return errors.New("node.js/npm is required; no installation attempted")
	}
	directory := filepath.Join(e.root, "clients")
	if err = privateDirectory(directory); err != nil {
		return err
	}
	for _, name := range []string{"npm-user.npmrc", "npm-global.npmrc"} {
		path := filepath.Join(e.root, name)
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return errors.New("preserving changed npm configuration; expected empty lab npmrc")
		}
		if err = seedFile(path, nil); err != nil {
			return err
		}
	}
	cmd := exec.Command(npm, "install", "--prefix", directory, "--registry=https://registry.npmjs.org", "--userconfig="+filepath.Join(e.root, "npm-user.npmrc"), "--globalconfig="+filepath.Join(e.root, "npm-global.npmrc"), "--cache", filepath.Join(e.root, "npm-cache"), "--no-audit", "--no-fund", "--fetch-retries=0", "--fetch-timeout=30000", "@openai/codex@latest", "@anthropic-ai/claude-code@latest")
	cmd.Dir = directory
	cmd.Env = append(allowedEnvironment("HOME", "PATH", "LANG", "LC_ALL"), "TMPDIR="+filepath.Join(e.root, "tmp"))
	cmd.Stdout = e.out
	cmd.Stderr = e.stderr
	if err = cmd.Run(); err != nil {
		return err
	}
	for _, client := range []string{"claude", "codex"} {
		if err = e.version(client); err != nil {
			return err
		}
	}
	return nil
}
func (e *environment) version(client string) error {
	binary, err := e.clientBinary(client)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	cmd.Env = e.clientEnvironment(client)
	cmd.Dir = filepath.Join(e.root, "workspace")
	cmd.Stdout = e.out
	cmd.Stderr = e.stderr
	return cmd.Run()
}
func (e *environment) doctor() error {
	var failures []error
	for _, client := range []string{"codex", "claude"} {
		if err := e.version(client); err != nil {
			fmt.Fprintln(e.out, err)
			failures = append(failures, err)
		}
	}
	health, err := e.fetchJSON(endpoint+"/health", 2*time.Second)
	if err != nil {
		failures = append(failures, err)
	} else {
		data, _ := json.Marshal(health)
		fmt.Fprintln(e.out, "Gateway:", string(data))
	}
	return errors.Join(failures...)
}
func (e *environment) launch(client string) error {
	binary, err := e.clientBinary(client)
	if err != nil {
		return err
	}
	health, err := e.fetchJSON(endpoint+"/health", 2*time.Second)
	if err != nil {
		return errors.New("gateway unavailable; run make lab-run, then inspect make lab-status")
	}
	capabilities, _ := health["capabilities"].(map[string]any)
	required := "responses"
	if client == "claude" {
		required = "messages"
	}
	if capabilities[required] != true || capabilities["tools"] != true {
		return fmt.Errorf("not launching %s: gateway needs %s/tools capabilities; rebuild this lab explicitly", client, required)
	}
	var settings runtimeSettings
	readJSON(filepath.Join(e.root, "runtime.json"), &settings)
	if client == "claude" && settings.SmallModelPath != "" && capabilities["claude_roles"] != true {
		return errors.New("selected Claude roles need the updated gateway; run make lab-rebuild explicitly")
	}
	if settings.SmallModelPath != "" {
		for _, port := range []int{19091, 19092} {
			runtimeHealth, err := e.fetchJSON(fmt.Sprintf("http://127.0.0.1:%d/health", port), 2*time.Second)
			if err != nil {
				return fmt.Errorf("MLX runtime %d is unavailable or loading; inspect make lab-status: %w", port, err)
			}
			if err := validateNativeProfiles(runtimeHealth); err != nil {
				return fmt.Errorf("MLX runtime %d: %w", port, err)
			}
		}
	}
	arguments := []string{}
	if client == "claude" {
		arguments, err = claudeArguments(e.root, os.Getenv("LAB_CLAUDE_FEATURES"))
		if err != nil {
			return err
		}
	}
	workspace := os.Getenv("LAB_WORKSPACE")
	if workspace == "" {
		workspace = filepath.Join(e.root, "workspace")
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return err
	}
	info, err := os.Stat(workspace) // #nosec G703 -- LAB_WORKSPACE is the local caller's explicit working directory; launching their chosen project is intentional.
	if err != nil || !info.IsDir() {
		return errors.New("LAB_WORKSPACE must be an existing directory")
	}
	fmt.Fprintf(e.out, "Opening real %s in %s\nBackend: Sentinel → MLX-Flash → selected local model.\n", client, workspace)
	return replaceProcess(binary, arguments, workspace, e.clientEnvironment(client), e.in, e.out, e.stderr)
}

func claudeArguments(root, mode string) ([]string, error) {
	if mode != "" && mode != "full" && mode != "minimal" {
		return nil, errors.New("LAB_CLAUDE_FEATURES must be full or minimal")
	}
	agents := `{"Explore":{"description":"Fast read-only discovery.","prompt":"Use Read, Glob and Grep; report evidence. Do not edit or execute commands.","tools":["Read","Glob","Grep"],"model":"haiku"},"Plan":{"description":"Read-only planning.","prompt":"Research and plan from evidence. Do not edit or execute commands.","tools":["Read","Glob","Grep"],"model":"opus"}}`
	arguments := []string{"--plugin-dir", filepath.Join(root, "control-plugin"), "--model", "opusplan", "--setting-sources", "user", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--agents", agents, "--permission-mode", "default"}
	if mode == "minimal" {
		arguments = append([]string{"--bare", "--tools", "Read,Write,Edit,Bash,Glob,Grep,Agent"}, arguments...)
	}
	return arguments, nil
}

func validateNativeProfiles(health map[string]any) error {
	capabilities, _ := health["capabilities"].(map[string]any)
	if family, ok := capabilities["model_family"].(string); ok && family != "" {
		switch family {
		case "lfm2_moe":
			if capabilities["thinking_control"] == false && capabilities["reasoning_format"] == "think" {
				return nil
			}
		case "gemma4":
			if capabilities["thinking_control"] != true || capabilities["reasoning_format"] != "gemma" {
				return errors.New("MLX-Flash Gemma profile capabilities do not match the model")
			}
		case "qwen", "qwen3", "qwen3_moe", "qwen3_5", "qwen3_5_moe", "qwen3_next":
			if capabilities["thinking_control"] != true || capabilities["reasoning_format"] != "think" {
				return errors.New("MLX-Flash Qwen profile capabilities do not match the model")
			}
		default:
			return errors.New("MLX-Flash advertises an unsupported model family")
		}
		if family == "lfm2_moe" {
			return errors.New("MLX-Flash LFM profile must not advertise a thinking toggle")
		}
	}
	options, _ := capabilities["chat_template_kwargs"].([]any)
	for _, option := range options {
		if option == "enable_thinking" {
			return nil
		}
	}
	return errors.New("MLX-Flash lacks native enable_thinking profiles; update the external MLX-Flash runtime, then explicitly run make lab-rebuild; no Sentinel compatibility shim is used")
}
