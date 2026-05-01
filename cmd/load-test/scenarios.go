package main

import (
	"fmt"
	"time"
)

// Scenario defines a load testing scenario configuration
type Scenario struct {
	Name        string
	Description string
	Config      LoadTestConfig
	WorkloadFn  WorkloadFunction
}

// WorkloadFunction defines how each request is generated in the scenario
type WorkloadFunction func(workerID int, requestNum int64) (duration time.Duration, success bool)

// AllScenarios returns all available load test scenarios
func AllScenarios() map[string]*Scenario {
	return map[string]*Scenario{
		"constant":     ScenarioConstantLoad(),
		"burst":        ScenarioBurstLoad(),
		"rampup":       ScenarioRampUp(),
		"mixed":        ScenarioMixedWorkload(),
		"recovery":     ScenarioFailureRecovery(),
		"sustained":    ScenarioSustained(),
		"spike":        ScenarioSpike(),
		"degradation":  ScenarioDegradation(),
	}
}

// ScenarioConstantLoad: Sustain 1000 req/sec for 1 hour
// Tests steady-state performance, memory stability, cache consistency
func ScenarioConstantLoad() *Scenario {
	return &Scenario{
		Name: "constant",
		Description: "Constant load: 1000 req/sec sustained for 1 hour\n" +
			"Tests steady-state performance, memory stability, cache consistency\n" +
			"Target: <500ms P99 latency, <200MB memory, >90% cache hit rate",
		Config: LoadTestConfig{
			Duration:         60 * time.Minute,
			TargetRate:       1000,
			Workers:          100,
			RampUpDuration:   5 * time.Minute,      // Gradual ramp to 1000 req/sec
			RampDownDuration: 5 * time.Minute,      // Gradual ramp down
			ReportInterval:   30 * time.Second,     // Report every 30 seconds
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			// Simulate real request processing
			// Most requests: 100-200ms
			// Some outliers: 300-500ms
			// Rare timeouts: fail rate ~0.1%
			randomLatency := 100 + (int(requestNum%100) % 100)
			if requestNum%1000 == 0 {
				randomLatency = 500 // Occasional slow request
			}
			if requestNum%10000 == 0 {
				return 0, false // Rare failure (0.01%)
			}
			return time.Duration(randomLatency) * time.Millisecond, true
		},
	}
}

// ScenarioBurstLoad: Ramp from 100 to 5000 req/sec, sustain 10s, ramp down
// Tests backpressure handling, queue overflow, burst recovery
func ScenarioBurstLoad() *Scenario {
	return &Scenario{
		Name: "burst",
		Description: "Burst load: Spike to 5000 req/sec for 10 seconds\n" +
			"Tests backpressure handling, queue overflow, burst recovery\n" +
			"Target: <1000ms P99 during burst, <5% error rate during spike",
		Config: LoadTestConfig{
			Duration:         30 * time.Second,
			TargetRate:       1000,           // Target rate starts at 1000
			Workers:          200,            // More workers to handle burst
			RampUpDuration:   5 * time.Second,
			RampDownDuration: 5 * time.Second,
			ReportInterval:   2 * time.Second,
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			// During burst: higher latency expected
			// After burst: should recover to normal
			randomLatency := 150 + (int(requestNum%200) % 200)
			if requestNum%500 == 0 && requestNum > 100 {
				// During burst, some queueing expected
				randomLatency = 1000
			}
			if requestNum%2000 == 0 {
				return 0, false // Slightly higher error rate during burst
			}
			return time.Duration(randomLatency) * time.Millisecond, true
		},
	}
}

// ScenarioConnectionChurn: Rapid connect/disconnect cycles
// Tests connection pool stability, resource cleanup, state management
func ScenarioConnectionChurn() *Scenario {
	return &Scenario{
		Name: "churn",
		Description: "Connection churn: Rapid connect/disconnect cycles\n" +
			"Tests connection pool stability, resource cleanup, state management\n" +
			"Target: No goroutine leaks, connection pool stable size",
		Config: LoadTestConfig{
			Duration:         30 * time.Second,
			TargetRate:       500,             // Lower sustained rate
			Workers:          200,             // High worker churn (simulates many connections)
			RampUpDuration:   2 * time.Second,
			RampDownDuration: 2 * time.Second,
			ReportInterval:   5 * time.Second,
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			// Simulate connection churn with variable latency
			// First request after "reconnect" is slightly slower
			latency := 50
			if requestNum%10 == 0 {
				latency = 200 // Occasional reconnect
			}
			if requestNum%1000 == 0 {
				return 0, false // Connection pool error
			}
			return time.Duration(latency) * time.Millisecond, true
		},
	}
}

// ScenarioMixedWorkload: 60% cache hits, 30% batch, 10% fresh
// Tests workload diversity, cache effectiveness, routing correctness
func ScenarioMixedWorkload() *Scenario {
	return &Scenario{
		Name: "mixed",
		Description: "Mixed workload: 60% cache hits, 30% batch jobs, 10% fresh requests\n" +
			"Tests workload diversity, cache effectiveness, routing correctness\n" +
			"Target: >85% overall success rate, correct routing decisions",
		Config: LoadTestConfig{
			Duration:         120 * time.Second,
			TargetRate:       1000,
			Workers:          100,
			RampUpDuration:   10 * time.Second,
			RampDownDuration: 10 * time.Second,
			ReportInterval:   10 * time.Second,
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			workloadType := requestNum % 100

			var latency int
			success := true

			switch {
			case workloadType < 60:
				// 60%: Cache hits (fast)
				latency = 20 + int(requestNum%30)
				if requestNum%500 == 0 {
					success = false // Rare cache lookup failure
				}

			case workloadType < 90:
				// 30%: Batch jobs (slower)
				latency = 500 + int(requestNum%500)
				if requestNum%1000 == 0 {
					success = false // Batch API failure
				}

			default:
				// 10%: Fresh requests (variable)
				latency = 200 + int(requestNum%300)
				if requestNum%2000 == 0 {
					success = false // API failure
				}
			}

			return time.Duration(latency) * time.Millisecond, success
		},
	}
}

// ScenarioFailureRecovery: Simulate API timeouts, network failures, database lag
// Tests error handling, retry logic, graceful degradation
func ScenarioFailureRecovery() *Scenario {
	return &Scenario{
		Name: "recovery",
		Description: "Failure recovery: Simulate API timeouts, network failures, database lag\n" +
			"Tests error handling, retry logic, graceful degradation\n" +
			"Target: Recover to baseline performance within 30 seconds of failure",
		Config: LoadTestConfig{
			Duration:         120 * time.Second,
			TargetRate:       500,             // Lower baseline rate
			Workers:          100,
			RampUpDuration:   10 * time.Second,
			RampDownDuration: 10 * time.Second,
			ReportInterval:   5 * time.Second,
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			// Simulate different failure phases:
			// 0-20s: Normal operation
			// 20-30s: High failure rate (API timeout)
			// 30-40s: Partial recovery
			// 40-60s: Back to normal
			// 60-70s: Database lag
			// 70s+: Normal again

			elapsed := requestNum / 10 // Approximate elapsed time in seconds
			baseLatency := 100
			failureRate := 0.0

			switch {
			case elapsed < 20:
				// Normal operation
				failureRate = 0.01
				baseLatency = 100

			case elapsed < 30:
				// High failure rate (API timeout)
				failureRate = 0.50
				baseLatency = 5000

			case elapsed < 40:
				// Partial recovery
				failureRate = 0.20
				baseLatency = 500

			case elapsed < 60:
				// Back to normal
				failureRate = 0.01
				baseLatency = 100

			case elapsed < 70:
				// Database lag
				failureRate = 0.05
				baseLatency = 2000

			default:
				// Back to normal
				failureRate = 0.01
				baseLatency = 100
			}

			// Determine success/failure
			success := (requestNum % 100) >= int64(failureRate*100)
			latency := baseLatency + int(requestNum%100)

			return time.Duration(latency) * time.Millisecond, success
		},
	}
}

// ScenarioSustained: Maintain 1000 req/sec for 30 minutes
// Tests sustained performance, memory stability over extended period
func ScenarioSustained() *Scenario {
	return &Scenario{
		Name: "sustained",
		Description: "Sustained load: 1000 req/sec for 30 minutes\n" +
			"Tests sustained performance, memory stability, long-running consistency\n" +
			"Target: <200ms P99 latency held throughout, memory growth <5%",
		Config: LoadTestConfig{
			Duration:         30 * time.Minute,
			TargetRate:       1000,
			Workers:          100,
			RampUpDuration:   3 * time.Minute,
			RampDownDuration: 3 * time.Minute,
			ReportInterval:   30 * time.Second,
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			// Consistent latency throughout sustained period
			latency := 80 + (int(requestNum%100) % 60)
			if requestNum%5000 == 0 {
				latency = 300 // Occasional spike
			}
			if requestNum%50000 == 0 {
				return 0, false // Very rare failure
			}
			return time.Duration(latency) * time.Millisecond, true
		},
	}
}

// ScenarioSpike: Sudden spike from 100 to 2000 req/sec
// Tests adaptive throttling, queue management, backpressure response
func ScenarioSpike() *Scenario {
	return &Scenario{
		Name: "spike",
		Description: "Spike scenario: Sudden jump from 100 to 2000 req/sec\n" +
			"Tests adaptive throttling, queue management, backpressure response\n" +
			"Target: Recovery to baseline within 30 seconds, <5% error rate during spike",
		Config: LoadTestConfig{
			Duration:         90 * time.Second,
			TargetRate:       100,  // Start low
			Workers:          200,  // High workers for spike
			RampUpDuration:   2 * time.Second,
			RampDownDuration: 5 * time.Second,
			ReportInterval:   5 * time.Second,
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			elapsed := requestNum / 50 // Approximate elapsed time in seconds
			var latency int
			failureRate := 0.0

			// 0-20s: Normal at 100 req/sec
			// 20-25s: Spike to 2000 req/sec
			// 25-60s: Recovery and return to normal
			switch {
			case elapsed < 20:
				// Normal baseline
				latency = 100 + int(requestNum%50)
				failureRate = 0.01
			case elapsed < 25:
				// Spike period
				latency = 800 + int(requestNum%400)
				failureRate = 0.05
			default:
				// Recovery
				latency = 200 - (int(elapsed-25) * 5) // Gradual improvement
				if latency < 100 {
					latency = 100
				}
				failureRate = 0.02
			}

			success := (requestNum % 100) >= int64(failureRate*100)
			return time.Duration(latency) * time.Millisecond, success
		},
	}
}

// ScenarioDegradation: Extreme load test - progressive increase from 200 to 5000 req/sec
// Tests system limits, graceful degradation under extreme conditions
func ScenarioDegradation() *Scenario {
	return &Scenario{
		Name: "degradation",
		Description: "Degradation test: Progressive load from 200 to 5000 req/sec\n" +
			"Tests system limits, graceful degradation under extreme conditions\n" +
			"Target: Identify breaking point, <20% error rate at max load",
		Config: LoadTestConfig{
			Duration:         120 * time.Second,
			TargetRate:       5000, // Peak target
			Workers:          300,  // Many workers for extreme load
			RampUpDuration:   60 * time.Second,
			RampDownDuration: 30 * time.Second,
			ReportInterval:   10 * time.Second,
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			// Load increases every 30 seconds: 200 → 500 → 1000 → 2000 → 5000 req/sec
			stage := int(requestNum / 1500) // Roughly 30s per stage
			var baseLatency int
			failureRate := 0.0

			switch stage {
			case 0:
				// 200 req/sec baseline
				baseLatency = 50
				failureRate = 0.01
			case 1:
				// 500 req/sec
				baseLatency = 100
				failureRate = 0.02
			case 2:
				// 1000 req/sec
				baseLatency = 200
				failureRate = 0.05
			case 3:
				// 2000 req/sec
				baseLatency = 400
				failureRate = 0.10
			default:
				// 5000 req/sec extreme load
				baseLatency = 1000
				failureRate = 0.20
			}

			latency := baseLatency + int(requestNum%(int64(baseLatency/2)+1))
			success := (requestNum % 100) >= int64(failureRate*100)
			return time.Duration(latency) * time.Millisecond, success
		},
	}
}

// ScenarioRampUp: Gradual increase from 100 to 1000 req/sec
// Tests ramp behavior, smooth load increase handling
func ScenarioRampUp() *Scenario {
	return &Scenario{
		Name: "rampup",
		Description: "Ramp-up test: Gradual increase from 100 to 1000 req/sec\n" +
			"Tests smooth load increase, adaptive scaling, resource allocation\n" +
			"Target: Latency increases proportional to load, no sudden failures",
		Config: LoadTestConfig{
			Duration:         40 * time.Minute, // 5 stages of 8 min each
			TargetRate:       1000,             // Ramp reaches 1000
			Workers:          100,
			RampUpDuration:   35 * time.Minute, // Slow ramp
			RampDownDuration: 5 * time.Minute,
			ReportInterval:   1 * time.Minute, // Report every minute
		},
		WorkloadFn: func(workerID int, requestNum int64) (time.Duration, bool) {
			// 5 stages of ~480s each: 100→250→500→750→1000 req/sec
			stage := int(requestNum / 2400) // ~480 requests per stage at different rates
			var latency int
			failureRate := 0.0

			// Latency should scale with stage
			switch stage {
			case 0:
				// 100 req/sec
				latency = 50
				failureRate = 0.01
			case 1:
				// 250 req/sec
				latency = 75
				failureRate = 0.01
			case 2:
				// 500 req/sec
				latency = 100
				failureRate = 0.02
			case 3:
				// 750 req/sec
				latency = 150
				failureRate = 0.02
			default:
				// 1000 req/sec
				latency = 200
				failureRate = 0.03
			}

			latency += int(requestNum % 50)
			success := (requestNum % 100) >= int64(failureRate*100)
			return time.Duration(latency) * time.Millisecond, success
		},
	}
}

// PrintScenarioList prints all available scenarios
func PrintScenarioList() {
	separator := "================================================================================"
	fmt.Println("\n" + separator)
	fmt.Println("AVAILABLE LOAD TEST SCENARIOS")
	fmt.Println(separator + "\n")

	scenarios := AllScenarios()
	for _, scenarioName := range []string{"constant", "burst", "rampup", "mixed", "recovery", "sustained", "spike", "degradation"} {
		scenario := scenarios[scenarioName]
		fmt.Printf("Scenario: %s\n", scenario.Name)
		fmt.Printf("Description: %s\n", scenario.Description)
		fmt.Printf("Duration: %v | Rate: %d req/s | Workers: %d\n",
			scenario.Config.Duration, scenario.Config.TargetRate, scenario.Config.Workers)
		fmt.Println()
	}

	fmt.Println(separator + "\n")
	fmt.Println("Usage: ./load-test -scenario=constant")
	fmt.Println("       ./load-test -scenario=burst")
	fmt.Println("       ./load-test -scenario=rampup")
	fmt.Println("       ./load-test -scenario=mixed")
	fmt.Println("       ./load-test -scenario=recovery")
	fmt.Println("       ./load-test -scenario=sustained")
	fmt.Println("       ./load-test -scenario=spike")
	fmt.Println("       ./load-test -scenario=degradation")
	fmt.Println()
}
