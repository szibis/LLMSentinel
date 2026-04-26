package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Reloader watches config file for changes and reloads configuration
type Reloader struct {
	configPath string
	loader     *Loader
	lastModTime time.Time
	mu         sync.RWMutex
	done       chan bool
	wg         sync.WaitGroup
	callbacks  []ReloadCallback
}

// ReloadCallback is called when config is reloaded
type ReloadCallback func() error

// NewReloader creates a new config reloader
func NewReloader(configPath string, loader *Loader) *Reloader {
	return &Reloader{
		configPath: configPath,
		loader:     loader,
		done:       make(chan bool),
		callbacks:  make([]ReloadCallback, 0),
	}
}

// OnReload adds a callback to be called when config reloads
func (r *Reloader) OnReload(callback ReloadCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, callback)
}

// Start begins watching the config file
func (r *Reloader) Start() error {
	// Get initial modification time
	info, err := os.Stat(r.configPath)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}
	r.lastModTime = info.ModTime()

	// Start watcher goroutine
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.done:
				return
			case <-ticker.C:
				r.checkAndReload()
			}
		}
	}()

	return nil
}

// checkAndReload checks if config file changed and reloads if needed
func (r *Reloader) checkAndReload() {
	info, err := os.Stat(r.configPath)
	if err != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if file was modified
	if info.ModTime().After(r.lastModTime) {
		// File was modified, reload
		if err := r.reload(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reloading config: %v\n", err)
			return
		}

		r.lastModTime = info.ModTime()
	}
}

// reload reloads the configuration and calls callbacks
func (r *Reloader) reload() error {
	// Load new config
	_, err := r.loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Call all callbacks
	for _, callback := range r.callbacks {
		if err := callback(); err != nil {
			return fmt.Errorf("callback failed: %w", err)
		}
	}

	fmt.Printf("[CONFIG] Configuration reloaded at %s (0 downtime)\n", time.Now().Format("15:04:05"))
	return nil
}

// ReloadNow forces an immediate reload of the configuration
func (r *Reloader) ReloadNow() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reload()
}

// Stop stops watching the config file
func (r *Reloader) Stop() error {
	close(r.done)
	r.wg.Wait()
	return nil
}

// GetConfigPath returns the configuration file path
func (r *Reloader) GetConfigPath() string {
	return r.configPath
}

// ExpandPath expands ~ to home directory in paths
func ExpandPath(p string) (string, error) {
	if len(p) == 0 {
		return p, nil
	}

	if p[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[1:]), nil
	}

	return p, nil
}
