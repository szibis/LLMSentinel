package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LoadTestConfig struct {
	Duration         time.Duration
	TargetRate       int // requests per second
	Workers          int
	RampUpDuration   time.Duration
	RampDownDuration time.Duration
	ReportInterval   time.Duration
	GatewayURL       string // HTTP gateway endpoint
}

type LoadTestMetrics struct {
	TotalRequests  int64
	SuccessCount   int64
	FailureCount   int64
	TotalLatencyMs int64
	MinLatencyMs   int64
	MaxLatencyMs   int64
	StartTime      time.Time
	EndTime        time.Time
	LatencyValues  []int64
	mu             sync.RWMutex
}

func main() {
	// Parse flags
	scenarioName := flag.String("scenario", "", "Load test scenario (constant, burst, churn, mixed, recovery, or custom)")
	duration := flag.Duration("duration", 5*time.Minute, "Total test duration")
	rate := flag.Int("rate", 5000, "Target requests per second")
	workers := flag.Int("workers", 100, "Number of concurrent workers")
	rampUp := flag.Duration("ramp-up", 30*time.Second, "Ramp-up duration")
	rampDown := flag.Duration("ramp-down", 30*time.Second, "Ramp-down duration")
	reportInterval := flag.Duration("report", 10*time.Second, "Metrics report interval")
	gatewayURL := flag.String("gateway", "http://localhost:8080", "Gateway URL (e.g., http://localhost:8080)")
	listScenarios := flag.Bool("list-scenarios", false, "List all available scenarios and exit")

	flag.Parse()

	// List scenarios if requested
	if *listScenarios {
		PrintScenarioList()
		return
	}

	// Load scenario or use custom config
	var config LoadTestConfig
	if *scenarioName != "" {
		scenarios := AllScenarios()
		scenario, ok := scenarios[*scenarioName]
		if !ok {
			fmt.Printf("ERROR: Unknown scenario '%s'\n", *scenarioName)
			fmt.Println("\nAvailable scenarios:")
			for name := range scenarios {
				fmt.Printf("  - %s\n", name)
			}
			return
		}
		config = scenario.Config
		config.GatewayURL = *gatewayURL
		fmt.Printf("Running Scenario: %s\n\n", scenario.Description)
	} else {
		// Use custom configuration from flags
		config = LoadTestConfig{
			Duration:         *duration,
			TargetRate:       *rate,
			Workers:          *workers,
			RampUpDuration:   *rampUp,
			RampDownDuration: *rampDown,
			ReportInterval:   *reportInterval,
			GatewayURL:       *gatewayURL,
		}
	}

	// Verify gateway is reachable
	fmt.Printf("Gateway: %s\n", config.GatewayURL)
	if err := verifyGateway(config.GatewayURL); err != nil {
		fmt.Printf("ERROR: Gateway not reachable: %v\n", err)
		fmt.Println("Start the gateway first: go run cmd/gateway/main.go -provider mock")
		return
	}
	fmt.Println("✓ Gateway reachable")

	fmt.Printf("Load Test Configuration:\n")
	fmt.Printf("  Duration: %v\n", config.Duration)
	fmt.Printf("  Target Rate: %d req/sec\n", config.TargetRate)
	fmt.Printf("  Workers: %d\n", config.Workers)
	fmt.Printf("  Ramp-up: %v\n", config.RampUpDuration)
	fmt.Printf("  Ramp-down: %v\n", config.RampDownDuration)
	fmt.Printf("  Report Interval: %v\n\n", config.ReportInterval)

	metrics := runLoadTest(config)
	printFinalReport(metrics, config)
}

func runLoadTest(config LoadTestConfig) *LoadTestMetrics {
	metrics := &LoadTestMetrics{
		StartTime:     time.Now(),
		LatencyValues: make([]int64, 0),
	}

	// HTTP client with connection pooling
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        config.Workers * 2,
			MaxIdleConnsPerHost: config.Workers,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	defer client.CloseIdleConnections()

	// Channels for coordination
	stopChan := make(chan struct{})
	tickerChan := time.NewTicker(config.ReportInterval).C
	requestChan := make(chan struct{}, config.Workers*10)

	// WaitGroup for workers
	var wg sync.WaitGroup
	wg.Add(config.Workers)

	// Start workers
	for i := 0; i < config.Workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			reqNum := int64(0)
			for range requestChan {
				start := time.Now()

				// Make HTTP request to gateway
				payload := map[string]interface{}{
					"model": "mock-model",
					"messages": []map[string]string{
						{"role": "user", "content": fmt.Sprintf("Test message %d from worker %d", reqNum, workerID)},
					},
				}

				body, _ := json.Marshal(payload)
				resp, err := client.Post(
					config.GatewayURL+"/v1/chat/completions",
					"application/json",
					bytes.NewReader(body),
				)

				latencyMs := time.Since(start).Milliseconds()

				if resp != nil && resp.Body != nil {
					io.ReadAll(resp.Body)
					resp.Body.Close()
				}

				metrics.mu.Lock()
				metrics.TotalLatencyMs += latencyMs
				if metrics.MinLatencyMs == 0 || latencyMs < metrics.MinLatencyMs {
					metrics.MinLatencyMs = latencyMs
				}
				if latencyMs > metrics.MaxLatencyMs {
					metrics.MaxLatencyMs = latencyMs
				}
				metrics.LatencyValues = append(metrics.LatencyValues, latencyMs)

				if err != nil || (resp != nil && resp.StatusCode >= 400) {
					atomic.AddInt64(&metrics.FailureCount, 1)
				} else {
					atomic.AddInt64(&metrics.SuccessCount, 1)
				}
				atomic.AddInt64(&metrics.TotalRequests, 1)
				metrics.mu.Unlock()

				reqNum++
			}
		}(i)
	}

	// Main control loop
	go func() {
		startTime := time.Now()
		sustainDuration := config.Duration - config.RampUpDuration - config.RampDownDuration

		for {
			elapsed := time.Since(startTime)

			// Determine current request rate based on phase
			var currentRate int
			if elapsed < config.RampUpDuration {
				// Ramp-up phase
				progress := float64(elapsed) / float64(config.RampUpDuration)
				currentRate = int(float64(config.TargetRate) * progress)
			} else if elapsed < config.RampUpDuration+sustainDuration {
				// Sustained phase
				currentRate = config.TargetRate
			} else if elapsed < config.Duration {
				// Ramp-down phase
				progress := float64(elapsed-config.RampUpDuration-sustainDuration) / float64(config.RampDownDuration)
				currentRate = int(float64(config.TargetRate) * (1 - progress))
			} else {
				// Test complete
				break
			}

			// Send requests to achieve target rate
			ratePerWorker := float64(currentRate) / float64(config.Workers)
			interval := time.Duration(float64(time.Second) / ratePerWorker)

			select {
			case <-stopChan:
				return
			case <-time.After(interval):
				select {
				case requestChan <- struct{}{}:
				case <-stopChan:
					return
				default:
					// Channel full, skip
				}
			}
		}
		close(requestChan)
	}()

	// Report metrics periodically
	go func() {
		for range tickerChan {
			printInterimReport(metrics)
		}
	}()

	// Wait for all workers to finish
	wg.Wait()
	metrics.EndTime = time.Now()

	return metrics
}

func printInterimReport(metrics *LoadTestMetrics) {
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()

	elapsed := time.Since(metrics.StartTime)
	avgLatency := int64(0)
	if metrics.TotalRequests > 0 {
		avgLatency = metrics.TotalLatencyMs / metrics.TotalRequests
	}

	fmt.Printf("[%6.1fs] Requests: %d | Rate: %.0f req/s | Latency: min=%dms avg=%dms max=%dms | Success: %d | Errors: %d\n",
		elapsed.Seconds(),
		metrics.TotalRequests,
		float64(metrics.TotalRequests)/elapsed.Seconds(),
		metrics.MinLatencyMs,
		avgLatency,
		metrics.MaxLatencyMs,
		metrics.SuccessCount,
		metrics.FailureCount,
	)
}

func printFinalReport(metrics *LoadTestMetrics, config LoadTestConfig) {
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()

	fmt.Print("\n" + strings.Repeat("=", 80) + "\n")
	fmt.Printf("LOAD TEST RESULTS\n")
	fmt.Print(strings.Repeat("=", 80) + "\n\n")

	duration := metrics.EndTime.Sub(metrics.StartTime)
	successRate := 0.0
	if metrics.TotalRequests > 0 {
		successRate = float64(metrics.SuccessCount) / float64(metrics.TotalRequests) * 100
	}

	avgLatency := int64(0)
	if metrics.TotalRequests > 0 {
		avgLatency = metrics.TotalLatencyMs / metrics.TotalRequests
	}

	fmt.Printf("Test Duration: %v\n", duration)
	fmt.Printf("Total Requests: %d\n", metrics.TotalRequests)
	fmt.Printf("Successful: %d (%.1f%%)\n", metrics.SuccessCount, successRate)
	fmt.Printf("Failed: %d\n", metrics.FailureCount)
	fmt.Printf("Requests/sec: %.0f\n\n", float64(metrics.TotalRequests)/duration.Seconds())

	fmt.Printf("Latency Metrics:\n")
	fmt.Printf("  Min: %dms\n", metrics.MinLatencyMs)
	fmt.Printf("  Avg: %dms\n", avgLatency)
	fmt.Printf("  Max: %dms\n", metrics.MaxLatencyMs)
	fmt.Printf("  P50: %dms\n", calculatePercentile(metrics.LatencyValues, 50))
	fmt.Printf("  P95: %dms\n", calculatePercentile(metrics.LatencyValues, 95))
	fmt.Printf("  P99: %dms\n", calculatePercentile(metrics.LatencyValues, 99))
	fmt.Printf("  P99.9: %dms\n\n", calculatePercentile(metrics.LatencyValues, 99.9))

	// Target validation
	fmt.Printf("Target Validation:\n")
	p99 := calculatePercentile(metrics.LatencyValues, 99)
	if p99 < 200 {
		fmt.Printf("  ✓ P99 Latency Target (<200ms): PASS (actual: %dms)\n", p99)
	} else {
		fmt.Printf("  ✗ P99 Latency Target (<200ms): FAIL (actual: %dms)\n", p99)
	}

	targetRate := float64(config.TargetRate)
	actualRate := float64(metrics.TotalRequests) / duration.Seconds()
	if actualRate >= targetRate*0.95 {
		fmt.Printf("  ✓ Throughput Target (%.0f req/s): PASS (actual: %.0f req/s)\n", targetRate, actualRate)
	} else {
		fmt.Printf("  ✗ Throughput Target (%.0f req/s): FAIL (actual: %.0f req/s)\n", targetRate, actualRate)
	}

	if successRate >= 99.5 {
		fmt.Printf("  ✓ Success Rate Target (>99.5%%): PASS (actual: %.1f%%)\n", successRate)
	} else {
		fmt.Printf("  ✗ Success Rate Target (>99.5%%): FAIL (actual: %.1f%%)\n", successRate)
	}

	fmt.Print("\n" + strings.Repeat("=", 80) + "\n")
}

func calculatePercentile(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}

	if percentile <= 0 {
		return values[0]
	}
	if percentile >= 100 {
		return values[len(values)-1]
	}

	// Simple percentile calculation
	index := int(float64(len(values)) * percentile / 100)
	if index >= len(values) {
		index = len(values) - 1
	}

	return values[index]
}

func verifyGateway(gatewayURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(gatewayURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}
	return nil
}
