package discovery

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// DiscoveryConfig represents the tool discovery configuration from YAML
type DiscoveryConfig struct {
	Discovery struct {
		RTK struct {
			SearchPaths []string `yaml:"search_paths"`
			TimeoutMs   int      `yaml:"timeout_ms"`
		} `yaml:"rtk"`
		Scrapling struct {
			SearchPaths []string `yaml:"search_paths"`
			TimeoutMs   int      `yaml:"timeout_ms"`
		} `yaml:"scrapling"`
		LanguageServers map[string]struct {
			SearchPaths []string `yaml:"search_paths"`
			TimeoutMs   int      `yaml:"timeout_ms"`
		} `yaml:"language_servers"`
		Git struct {
			SearchPaths []string `yaml:"search_paths"`
			TimeoutMs   int      `yaml:"timeout_ms"`
		} `yaml:"git"`
		Runtimes map[string]struct {
			SearchPaths []string `yaml:"search_paths"`
			TimeoutMs   int      `yaml:"timeout_ms"`
		} `yaml:"runtimes"`
		MCPServers struct {
			SocketSearchPaths []string `yaml:"socket_search_paths"`
		} `yaml:"mcp_servers"`
		PlatformOverrides map[string]struct {
			PrimaryPaths []string `yaml:"primary_paths"`
		} `yaml:"platform_overrides"`
	} `yaml:"discovery"`
	Timeout struct {
		IndividualToolCheckMs int `yaml:"individual_tool_check_ms"`
		TotalDiscoveryMs      int `yaml:"total_discovery_ms"`
		PathCheckParallelism  int `yaml:"path_check_parallelism"`
	} `yaml:"timeout"`
	Cache struct {
		Enabled    bool   `yaml:"enabled"`
		TTLMinutes int    `yaml:"ttl_minutes"`
		CacheDir   string `yaml:"cache_dir"`
	} `yaml:"cache"`
}

// DetectedTools contains information about discovered tools
type DetectedTools struct {
	RTKPath              string
	ScraplingPath        string
	LSPServers           map[string]string
	GitPath              string
	LanguageRuntimes     map[string]string
	InstalledLanguages   []string
	DetectedAt           string
	MCPServersAvailable  []string
	PlatformPrimaryPaths []string
}

// ConfigLoader loads discovery configuration from YAML
type ConfigLoader struct {
	configPath string
	mu         sync.RWMutex
	config     *DiscoveryConfig
}

// NewConfigLoader creates a new discovery config loader
func NewConfigLoader(configPath string) *ConfigLoader {
	return &ConfigLoader{
		configPath: configPath,
	}
}

// Load loads the discovery configuration
func (cl *ConfigLoader) Load() (*DiscoveryConfig, error) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	data, err := os.ReadFile(cl.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery config: %w", err)
	}

	var config DiscoveryConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse discovery config: %w", err)
	}

	cl.config = &config
	return &config, nil
}

// DetectTools performs auto-detection using YAML-based configuration
func DetectToolsWithConfig(configPath string) (*DetectedTools, error) {
	loader := NewConfigLoader(configPath)
	config, err := loader.Load()
	if err != nil {
		return nil, err
	}

	tools := &DetectedTools{
		LSPServers:       make(map[string]string),
		LanguageRuntimes: make(map[string]string),
		DetectedAt:       time.Now().Format("15:04:05"),
	}

	// Get platform-specific primary paths
	platform := runtime.GOOS
	if overrides, ok := config.Discovery.PlatformOverrides[platform]; ok {
		tools.PlatformPrimaryPaths = overrides.PrimaryPaths
	}

	// Detect RTK
	tools.RTKPath = findTool(config.Discovery.RTK.SearchPaths)

	// Detect Scrapling
	tools.ScraplingPath = findTool(config.Discovery.Scrapling.SearchPaths)

	// Detect Language Servers
	for name, lsp := range config.Discovery.LanguageServers {
		if path := findTool(lsp.SearchPaths); path != "" {
			tools.LSPServers[name] = path
		}
	}

	// Detect Git
	tools.GitPath = findTool(config.Discovery.Git.SearchPaths)

	// Detect Language Runtimes
	for name, runtime := range config.Discovery.Runtimes {
		if path := findTool(runtime.SearchPaths); path != "" {
			tools.LanguageRuntimes[name] = path
			tools.InstalledLanguages = append(tools.InstalledLanguages, name)
		}
	}

	// Detect MCP Servers
	tools.MCPServersAvailable = detectMCPServersFromConfig(config.Discovery.MCPServers.SocketSearchPaths)

	return tools, nil
}

// findTool searches for a tool in the given paths
func findTool(searchPaths []string) string {
	for _, pathPattern := range searchPaths {
		// Expand home directory and environment variables
		expanded := expandPath(pathPattern)

		// Handle wildcards and glob patterns
		if strings.Contains(expanded, "*") {
			// For patterns like ~/.nvm/*/bin/node, try to find matching paths
			if matches := findGlob(expanded); len(matches) > 0 {
				return matches[0]
			}
			continue
		}

		// Check if file exists and is executable
		info, err := os.Stat(expanded)
		if err != nil {
			continue
		}

		// Check if it's executable
		if !info.IsDir() && (info.Mode()&0111) != 0 {
			return expanded
		}
	}

	// Fallback: try using exec.LookPath for tools in PATH
	baseName := filepath.Base(searchPaths[len(searchPaths)-1])
	if path, err := exec.LookPath(baseName); err == nil {
		return path
	}

	return ""
}

// expandPath expands ~ and environment variables in a path
func expandPath(p string) string {
	// Expand environment variables
	p = os.ExpandEnv(p)

	// Expand home directory
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		p = filepath.Join(home, p[1:])
	}

	return p
}

// findGlob finds files matching a glob pattern
func findGlob(pattern string) []string {
	var results []string

	// Use filepath.Glob for proper wildcard handling
	expanded := expandPath(pattern)
	matches, err := filepath.Glob(expanded)
	if err != nil {
		return results
	}

	// Filter to only executable files
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		if !info.IsDir() && (info.Mode()&0111) != 0 {
			results = append(results, match)
		}
	}

	return results
}

// detectMCPServersFromConfig detects MCP servers from configured socket paths
func detectMCPServersFromConfig(socketPaths []string) []string {
	var servers []string

	for _, path := range socketPaths {
		expanded := expandPath(path)
		entries, err := os.ReadDir(expanded)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				servers = append(servers, entry.Name())
			}
		}
	}

	return servers
}

// DetectTools is the legacy interface (uses hardcoded paths)
func DetectTools() *DetectedTools {
	tools := &DetectedTools{
		LSPServers:       make(map[string]string),
		LanguageRuntimes: make(map[string]string),
		DetectedAt:       time.Now().Format("15:04:05"),
	}

	// Use default hardcoded paths for backward compatibility
	defaultRTKPaths := []string{
		"~/.cargo/bin/rtk",
		"/usr/local/bin/rtk",
		"/opt/homebrew/bin/rtk",
		"~/.local/bin/rtk",
	}

	defaultScraplingPaths := []string{
		"~/.cargo/bin/scrapling",
		"/usr/local/bin/scrapling",
		"~/.local/bin/scrapling",
	}

	defaultGitPaths := []string{
		"/usr/bin/git",
		"/usr/local/bin/git",
		"/opt/homebrew/bin/git",
	}

	tools.RTKPath = findTool(defaultRTKPaths)
	tools.ScraplingPath = findTool(defaultScraplingPaths)
	tools.GitPath = findTool(defaultGitPaths)

	return tools
}

// DetectMCPServers detects available MCP servers (legacy hardcoded)
func DetectMCPServers() []string {
	defaultMCPPaths := []string{
		"~/.mcp/servers",
		"~/.claude/mcp",
		"/tmp/mcp-servers",
	}

	return detectMCPServersFromConfig(defaultMCPPaths)
}

// DetectInstalledLanguages detects programming languages available on system
func DetectInstalledLanguages() []string {
	var langs []string

	langChecks := map[string]string{
		"python": "python3",
		"go":     "go",
		"rust":   "rustc",
		"node":   "node",
		"ruby":   "ruby",
		"java":   "java",
		"php":    "php",
		"csharp": "dotnet",
		"cpp":    "g++",
		"c":      "gcc",
	}

	for lang, cmd := range langChecks {
		_, err := exec.LookPath(cmd)
		if err == nil {
			langs = append(langs, lang)
		}
	}

	return langs
}
