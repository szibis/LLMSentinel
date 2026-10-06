// Package clientcontrol controls an existing local Sentinel without model calls.
package clientcontrol

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 512 * 1024
const defaultEndpoint = "http://127.0.0.1:19090"

func endpointURL(endpoint string) (*url.URL, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("endpoint must be a literal HTTP loopback origin without credentials")
	}
	if u.Port() != "" {
		port, e := strconv.Atoi(u.Port())
		if e != nil || port < 1 || port > 65535 {
			return nil, errors.New("invalid endpoint port")
		}
	}
	return u, nil
}
func requestSpec(args []string) (string, any, error) {
	if len(args) == 1 && args[0] == "status" {
		return http.MethodGet, nil, nil
	}
	if len(args) == 2 && args[0] == "training" && (args[1] == "on" || args[1] == "off") {
		return http.MethodPost, map[string]any{"capture_enabled": args[1] == "on"}, nil
	}
	if len(args) == 2 && args[0] == "policy" && (args[1] == "local-only" || args[1] == "balanced" || args[1] == "quality") {
		return http.MethodPost, map[string]any{"policy": args[1]}, nil
	}
	if len(args) == 3 && args[0] == "profile" && (args[1] == "haiku" || args[1] == "sonnet" || args[1] == "opus") {
		n, e := strconv.Atoi(args[2])
		if e == nil && n >= 1 && n <= 32768 {
			return http.MethodPost, map[string]any{"role_budgets": map[string]int{args[1]: n}}, nil
		}
	}
	return "", nil, errors.New("use status; training on|off; policy local-only|balanced|quality; profile haiku|sonnet|opus 1..32768")
}
func controlClient() *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}
}
func send(client *http.Client, endpoint, method string, payload any) (map[string]any, error) {
	u, err := endpointURL(endpoint)
	if err != nil {
		return nil, err
	}
	u.Path = "/sentinel/control"
	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBytes {
		return nil, errors.New("sentinel control response exceeds limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sentinel control HTTP %d; request rejected", resp.StatusCode)
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&result); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("response must be a JSON object")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("response contains trailing JSON")
	}
	return result, nil
}

// ShellQuote quotes one literal shell argument without interpolation.
func ShellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-", r)
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// Assets returns filename-relative command previews; it installs nothing.
func Assets(client, endpoint, executable string) (map[string]string, error) {
	if _, err := endpointURL(endpoint); err != nil {
		return nil, err
	}
	if client != "claude" && client != "codex" {
		return nil, errors.New("client must be claude or codex")
	}
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		return nil, errors.New("executable must be absolute and clean")
	}
	fixed := map[string][]string{"sentinel-status": {"status"}, "sentinel-training-on": {"training", "on"}, "sentinel-training-off": {"training", "off"}, "sentinel-policy-local": {"policy", "local-only"}, "sentinel-policy-balanced": {"policy", "balanced"}, "sentinel-policy-quality": {"policy", "quality"}, "sentinel-profile-haiku": {"profile", "haiku", "1024"}, "sentinel-profile-sonnet": {"profile", "sonnet", "4096"}, "sentinel-profile-opus": {"profile", "opus", "8192"}}
	assets := make(map[string]string, len(fixed))
	for name, args := range fixed {
		front := "---\ndescription: Sentinel " + strings.Join(args, " ") + "\n"
		if client == "claude" {
			front += "disable-model-invocation: true\n"
		}
		parts := []string{executable, "control", "--endpoint", endpoint}
		parts = append(parts, args...)
		for i := range parts {
			parts[i] = ShellQuote(parts[i])
		}
		assets[name+".md"] = front + "---\n\nRun the following exact local command once using the shell tool, then report its returned JSON or error briefly.\nDo not add or interpolate arguments. Do not change provider/authentication settings, start another service, or retry a rejected mutation.\nThese instructions are model-assisted and may consume CLI model tokens; the control command itself calls no model provider.\n\n```sh\n" + strings.Join(parts, " ") + "\n```\n"
	}
	return assets, nil
}

// ValidateDirectory rejects noncanonical paths and symlinks in every existing
// ancestor before callers create missing directories. It performs no mutations.
func ValidateDirectory(directory string) error {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return errors.New("directory must be canonical and absolute")
	}
	ancestor := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(directory, ancestor), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		ancestor = filepath.Join(ancestor, part)
		info, err := os.Lstat(ancestor)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("directory ancestors must be directories without symlinks")
		}
	}
	return nil
}
func writeCommands(directory string, assets map[string]string) ([]string, error) {
	if err := ValidateDirectory(directory); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(directory)
	if err != nil || real != directory {
		return nil, errors.New("preview directory must not contain symlinks")
	}
	names := make([]string, 0, len(assets))
	for name := range assets {
		if filepath.Base(name) != name {
			return nil, errors.New("invalid asset name")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, e := os.Lstat(filepath.Join(directory, name)); !os.IsNotExist(e) {
			return nil, errors.New("command preview exists; choose a new directory")
		}
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		path := filepath.Join(directory, name)
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, e = file.WriteString(assets[name])
		closeErr := file.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// Run implements the control subcommand. It never reads credentials or executes tools.
func Run(args []string, _ io.Reader, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("control", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("endpoint", defaultEndpoint, "literal HTTP loopback origin")
	client := flags.String("client", "claude", "claude or codex")
	preview := flags.Bool("preview-commands", false, "print command previews")
	directory := flags.String("write-commands", "", "write previews into a new explicit directory")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	var result any
	var err error
	if *preview || *directory != "" {
		if flags.NArg() != 0 || (*preview && *directory != "") {
			err = errors.New("command generation cannot be combined with control actions")
		} else {
			var executable string
			executable, err = os.Executable()
			if err == nil {
				executable, err = filepath.Abs(executable)
			}
			var assets map[string]string
			if err == nil {
				assets, err = Assets(*client, *endpoint, executable)
			}
			if err == nil {
				if *directory == "" {
					result = assets
				} else {
					var paths []string
					paths, err = writeCommands(*directory, assets)
					result = map[string]any{"written": paths, "installed": false}
				}
			}
		}
	} else {
		var method string
		var payload any
		method, payload, err = requestSpec(flags.Args())
		if err == nil {
			clientHTTP := controlClient()
			defer clientHTTP.CloseIdleConnections()
			result, err = send(clientHTTP, *endpoint, method, payload)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "Sentinel control failed:", err)
		return 1
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(result); err != nil {
		fmt.Fprintln(stderr, "Sentinel control failed:", err)
		return 1
	}
	return 0
}
