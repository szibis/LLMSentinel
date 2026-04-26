package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MetricsPublisher publishes metrics to multiple targets
type MetricsPublisher struct {
	collector        *MetricsCollector
	exporter         *PrometheusExporter
	targets          []PublishTarget
	interval         time.Duration
	done             chan bool
	wg               sync.WaitGroup
	mu               sync.RWMutex
	lastPublishTime  time.Time
}

// PublishTarget represents a metrics publishing target
type PublishTarget interface {
	Publish(metrics map[string]interface{}) error
	Name() string
}

// LocalFilePublisher publishes to local debug log
type LocalFilePublisher struct {
	logDir string
}

// NewLocalFilePublisher creates a new local file publisher
func NewLocalFilePublisher(logDir string) (*LocalFilePublisher, error) {
	// Expand home directory
	if logDir[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		logDir = filepath.Join(home, logDir[1:])
	}

	// Create directory
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}

	return &LocalFilePublisher{logDir: logDir}, nil
}

// Publish publishes metrics to local file
func (lfp *LocalFilePublisher) Publish(metrics map[string]interface{}) error {
	filename := filepath.Join(lfp.logDir, fmt.Sprintf("metrics-%s.jsonl", time.Now().Format("2006-01-02")))

	// Marshal metrics
	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	// Append to file
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write JSON line
	_, err = file.WriteString(string(data) + "\n")
	return err
}

// Name returns the publisher name
func (lfp *LocalFilePublisher) Name() string {
	return "local_file"
}

// PrometheusServerPublisher publishes to Prometheus pushgateway (stub for future)
type PrometheusServerPublisher struct {
	url string
}

// NewPrometheusServerPublisher creates a new Prometheus server publisher
func NewPrometheusServerPublisher(url string) *PrometheusServerPublisher {
	return &PrometheusServerPublisher{url: url}
}

// Publish publishes metrics to Prometheus (stub)
func (psp *PrometheusServerPublisher) Publish(metrics map[string]interface{}) error {
	// TODO: Implement Prometheus pushgateway integration
	// For now, this is a stub that would send to http://pushgateway:9091/metrics/job/claude-escalate
	return nil
}

// Name returns the publisher name
func (psp *PrometheusServerPublisher) Name() string {
	return "prometheus_server"
}

// NewMetricsPublisher creates a new metrics publisher
func NewMetricsPublisher(collector *MetricsCollector, interval time.Duration) *MetricsPublisher {
	exporter := NewPrometheusExporter(collector)
	return &MetricsPublisher{
		collector:       collector,
		exporter:        exporter,
		targets:         make([]PublishTarget, 0),
		interval:        interval,
		done:            make(chan bool),
		lastPublishTime: time.Now(),
	}
}

// AddTarget adds a publishing target
func (mp *MetricsPublisher) AddTarget(target PublishTarget) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.targets = append(mp.targets, target)
}

// Start begins publishing metrics at regular intervals
func (mp *MetricsPublisher) Start() {
	mp.wg.Add(1)
	go func() {
		defer mp.wg.Done()
		ticker := time.NewTicker(mp.interval)
		defer ticker.Stop()

		for {
			select {
			case <-mp.done:
				return
			case <-ticker.C:
				mp.publishOnce()
			}
		}
	}()
}

// PublishOnce publishes metrics once to all targets
func (mp *MetricsPublisher) publishOnce() {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Also save to collector history
	mp.collector.SaveSnapshot()

	metrics := mp.exporter.ExportJSON()

	// Publish to all targets
	for _, target := range mp.targets {
		if err := target.Publish(metrics); err != nil {
			fmt.Fprintf(os.Stderr, "Error publishing to %s: %v\n", target.Name(), err)
		}
	}

	mp.lastPublishTime = time.Now()
}

// PublishNow forces an immediate publish to all targets
func (mp *MetricsPublisher) PublishNow() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	metrics := mp.exporter.ExportJSON()

	for _, target := range mp.targets {
		if err := target.Publish(metrics); err != nil {
			return fmt.Errorf("error publishing to %s: %w", target.Name(), err)
		}
	}

	mp.lastPublishTime = time.Now()
	return nil
}

// Stop stops the metrics publisher
func (mp *MetricsPublisher) Stop() error {
	close(mp.done)
	mp.wg.Wait()
	return nil
}

// GetExportedMetrics returns the current exported metrics in Prometheus format
func (mp *MetricsPublisher) GetExportedMetrics() string {
	return mp.exporter.Export()
}

// GetExportedJSON returns the current exported metrics in JSON format
func (mp *MetricsPublisher) GetExportedJSON() map[string]interface{} {
	return mp.exporter.ExportJSON()
}

// LastPublishTime returns when metrics were last published
func (mp *MetricsPublisher) LastPublishTime() time.Time {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return mp.lastPublishTime
}
