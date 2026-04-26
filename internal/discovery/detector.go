package discovery

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// DetectedTools contains information about discovered tools
type DetectedTools struct {
	RTKPath      string
	ScraplingAvailable bool
	LSPAvailable bool
	GitAvailable bool
	DetectedAt   string
}

// DetectTools performs auto-detection of installed tools
func DetectTools() *DetectedTools {
	tools := &DetectedTools{}

	// Detect RTK
	tools.RTKPath = detectRTK()

	// Detect Scrapling (check MCP sockets and common paths)
	tools.ScraplingAvailable = detectScrapling()

	// Detect LSP (check common language server paths)
	tools.LSPAvailable = detectLSP()

	// Detect Git
	tools.GitAvailable = detectGit()

	tools.DetectedAt = fmt.Sprintf("Auto-detected at startup")

	return tools
}

// detectRTK checks for RTK installation at ~/.local/bin/rtk
func detectRTK() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	rtkPath := filepath.Join(homeDir, ".local", "bin", "rtk")

	// Check if file exists and is executable
	info, err := os.Stat(rtkPath)
	if err != nil {
		return ""
	}

	// Check if it's executable
	if info.Mode()&0111 != 0 {
		return rtkPath
	}

	return ""
}

// detectScrapling checks for Scrapling MCP availability
func detectScrapling() bool {
	// Check if scrapling command exists in PATH
	_, err := exec.LookPath("scrapling")
	if err == nil {
		return true
	}

	// Check common installation paths
	commonPaths := []string{
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "scrapling"),
		filepath.Join(os.Getenv("HOME"), ".cargo", "bin", "scrapling"),
		"/usr/local/bin/scrapling",
		"/usr/bin/scrapling",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	// Check if MCP server is running (would need separate mechanism)
	// For now, we detect presence of the binary
	return false
}

// detectLSP checks for Language Server Protocol availability
func detectLSP() bool {
	lspServers := []string{
		"gopls",       // Go
		"pyright",     // Python
		"node_modules/.bin/typescript-language-server", // TypeScript
		"clangd",      // C/C++
		"rust-analyzer", // Rust
	}

	for _, server := range lspServers {
		_, err := exec.LookPath(server)
		if err == nil {
			return true
		}
	}

	return false
}

// detectGit checks for Git availability
func detectGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// DetectMCPServers attempts to detect running MCP servers
func DetectMCPServers() []string {
	var servers []string

	// Common MCP socket paths
	commonMCPPaths := []string{
		filepath.Join(os.Getenv("HOME"), ".mcp", "servers"),
		filepath.Join(os.Getenv("HOME"), ".claude", "mcp"),
		"/tmp/mcp-servers",
	}

	for _, path := range commonMCPPaths {
		if entries, err := os.ReadDir(path); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					servers = append(servers, entry.Name())
				}
			}
		}
	}

	return servers
}

// DetectInstalledLanguages detects programming languages available on system
func DetectInstalledLanguages() []string {
	var langs []string

	langChecks := map[string]string{
		"python":   "python3",
		"go":       "go",
		"rust":     "rustc",
		"node":     "node",
		"ruby":     "ruby",
		"java":     "java",
		"php":      "php",
		"csharp":   "dotnet",
		"cpp":      "g++",
		"c":        "gcc",
	}

	for lang, cmd := range langChecks {
		_, err := exec.LookPath(cmd)
		if err == nil {
			langs = append(langs, lang)
		}
	}

	return langs
}
