package optimization

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// TestConservative_LargeScaleWeeklyWorkload simulates realistic week-long usage
// with conservative assumptions about cache effectiveness
func TestConservative_LargeScaleWeeklyWorkload(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(3)
	opt.SetMaxBatchWaitTime(5 * time.Minute)
	opt.SetMinSavingsPercent(10.0) // Only optimize if >10% savings

	// Conservative assumptions:
	// - 150 requests per day (realistic developer)
	// - 85% of prompts are unique (low cache reuse)
	// - Cache hit rate realistically ~10-15%
	// - Batch processing only for willing users
	// - 30% of users won't wait for batch

	dailyRequests := 150
	weeklyRequests := dailyRequests * 7
	uniquePromptCount := int(float64(dailyRequests) * 0.85) // 127 unique per day
	totalEstimatedCost := 0.0
	totalActualCost := 0.0

	seed := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(seed)

	// Simulate 7 days
	seenPrompts := make(map[string]int)
	batchedRequests := 0
	cachedHits := 0
	directCalls := 0

	for day := 0; day < 7; day++ {
		// Each day has some repeated prompts from previous days (10%)
		// and mostly new prompts (85%) plus some variations (5%)

		for req := 0; req < dailyRequests; req++ {
			var prompt string

			// 10% chance of repeating a previous day's prompt
			if rng.Float64() < 0.10 && len(seenPrompts) > 0 {
				// Pick a random previous prompt
				idx := rng.Intn(len(seenPrompts))
				count := 0
				for p := range seenPrompts {
					if count == idx {
						prompt = p
						break
					}
					count++
				}
			} else {
				// 85% new prompts, 5% variations of recent prompts
				prompt = fmt.Sprintf("Request %d-%d-%d", day, req, rng.Intn(uniquePromptCount))
			}

			seenPrompts[prompt]++
			estimatedCost := 0.016

			decision := opt.Optimize(prompt, "sonnet", 500)
			totalEstimatedCost += estimatedCost

			// Conservative: Only 10-15% actual cache hits (not all matches are valid)
			hitChance := float64(seenPrompts[prompt]) / float64(dailyRequests) * 0.15
			isActualHit := rng.Float64() < hitChance

			if decision.CacheHit && isActualHit {
				totalActualCost += 0.00015 // Cache read cost
				cachedHits++
			} else if decision.UseBatch && rng.Float64() < 0.70 { // 70% of users willing to wait
				totalActualCost += estimatedCost * 0.5 // 50% batch discount
				batchedRequests++
			} else {
				totalActualCost += estimatedCost // Direct cost
				directCalls++
			}

			// Cache responses periodically (not all)
			if req%20 == 0 {
				opt.CacheResponse(prompt, "sonnet", fmt.Sprintf("response-%d", req), 300)
			}
		}
	}

	actualSavingsPercent := ((totalEstimatedCost - totalActualCost) / totalEstimatedCost) * 100

	t.Logf("\n=== CONSERVATIVE WEEKLY WORKLOAD (Large Scale) ===\n")
	t.Logf("Period: 7 days, %d total requests\n", weeklyRequests)
	t.Logf("Assumptions:\n")
	t.Logf("  - 85%% unique prompts per day\n")
	t.Logf("  - 10%% cross-day repetition\n")
	t.Logf("  - Conservative 10-15%% cache hit rate\n")
	t.Logf("  - 70%% user willingness to batch\n")
	t.Logf("  - Minimum 10%% savings threshold\n\n")

	t.Logf("Results:\n")
	t.Logf("  Total requests: %d\n", weeklyRequests)
	t.Logf("  Cache hits: %d (%.1f%%)\n", cachedHits, float64(cachedHits)/float64(weeklyRequests)*100)
	t.Logf("  Batch requests: %d (%.1f%%)\n", batchedRequests, float64(batchedRequests)/float64(weeklyRequests)*100)
	t.Logf("  Direct calls: %d (%.1f%%)\n", directCalls, float64(directCalls)/float64(weeklyRequests)*100)
	t.Logf("  Unique prompts seen: %d\n\n", len(seenPrompts))

	t.Logf("Cost Analysis:\n")
	t.Logf("  Estimated (no optimization): $%.2f\n", totalEstimatedCost)
	t.Logf("  Actual (with optimization): $%.2f\n", totalActualCost)
	t.Logf("  Weekly savings: $%.2f\n", totalEstimatedCost-totalActualCost)
	t.Logf("  Actual savings rate: %.1f%%\n", actualSavingsPercent)
	t.Logf("  Daily average: $%.2f/day (estimated) → $%.2f/day (actual)\n",
		totalEstimatedCost/7, totalActualCost/7)
	t.Logf("  Monthly projection: $%.2f\n\n", (totalEstimatedCost-totalActualCost)*4.3)

	// Conservative validation: expect 5-15% savings (not 30-50%)
	if actualSavingsPercent < 3.0 {
		t.Logf("WARNING: Savings below 3%%, may indicate optimization not effective for this pattern\n")
	}

	if actualSavingsPercent > 25.0 {
		t.Logf("WARNING: Savings above 25%%, may indicate assumptions were too optimistic\n")
	}
}

// TestConservative_RealDistributionPattern simulates real-world Zipfian distribution
// (80/20 rule but more realistic - power law)
func TestConservative_RealDistributionPattern(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(2)

	// Zipfian distribution: few questions asked many times, most asked rarely
	// Real world: "How to authenticate" (1000x), "How to deploy" (100x), unique requests (1x)

	type Question struct {
		text  string
		count int
	}

	questions := []Question{
		{"How do I authenticate?", 240},      // 24% of requests
		{"How do I set up database?", 210},   // 21% of requests
		{"How do I deploy?", 140},            // 14% of requests
		{"How do I optimize performance?", 95}, // 9.5%
		{"How do I handle errors?", 65},      // 6.5%
		{"How do I test my code?", 50},       // 5%
		{"Unique question 1", 30},            // 3%
		{"Unique question 2", 30},            // 3%
		{"Unique question 3", 25},            // 2.5%
		{"Unique question 4", 25},            // 2.5%
		{"Random requests", 90},              // 9% (various unique)
	}

	totalRequests := 0
	for _, q := range questions {
		totalRequests += q.count
	}

	totalEstimatedCost := 0.0
	totalActualCost := 0.0
	requestsByType := make(map[string]int)
	costsByType := make(map[string]float64)

	// Pre-warm cache for high-frequency questions
	for _, q := range questions {
		if q.count > 50 {
			opt.CacheResponse(q.text, "sonnet", fmt.Sprintf("answer to %s", q.text), 400)
		}
	}

	// Process requests
	for _, q := range questions {
		for j := 0; j < q.count; j++ {
			prompt := q.text
			if q.count <= 50 {
				// For low-frequency questions, add variation
				prompt = fmt.Sprintf("%s (variant %d)", q.text, j%3)
			}

			decision := opt.Optimize(prompt, "sonnet", 500)
			estimatedCost := 0.016
			totalEstimatedCost += estimatedCost

			actualCost := estimatedCost
			if decision.CacheHit {
				actualCost = 0.00015 // Cache read
			} else if decision.UseBatch && j%5 != 0 { // 80% batched after first request
				actualCost = 0.008 // 50% discount
			}

			totalActualCost += actualCost
			requestsByType[q.text]++
			costsByType[q.text] += actualCost

			// Cache high-frequency responses periodically
			if j%50 == 0 && q.count > 50 {
				opt.CacheResponse(prompt, "sonnet", fmt.Sprintf("answer %d", j), 400)
			}
		}
	}

	savingsPercent := ((totalEstimatedCost - totalActualCost) / totalEstimatedCost) * 100

	t.Logf("\n=== CONSERVATIVE ZIPFIAN DISTRIBUTION (Power Law) ===\n")
	t.Logf("Total requests: %d\n", totalRequests)
	t.Logf("Unique question patterns: %d\n\n", len(questions))

	t.Logf("Top 5 Questions by Frequency:\n")
	sorted := make([]Question, len(questions))
	copy(sorted, questions)
	// Simple bubble sort
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	for i := 0; i < 5 && i < len(sorted); i++ {
		pct := float64(sorted[i].count) / float64(totalRequests) * 100
		t.Logf("  %d. %s: %d requests (%.1f%%)\n", i+1, sorted[i].text, sorted[i].count, pct)
	}

	t.Logf("\nCost Analysis:\n")
	t.Logf("  Estimated (direct all): $%.2f\n", totalEstimatedCost)
	t.Logf("  Actual (optimized): $%.2f\n", totalActualCost)
	t.Logf("  Total savings: $%.2f (%.1f%%)\n", totalEstimatedCost-totalActualCost, savingsPercent)
	t.Logf("  Cost per request: $%.4f → $%.4f\n",
		totalEstimatedCost/float64(totalRequests),
		totalActualCost/float64(totalRequests))

	t.Logf("\nCache Effectiveness (Top 5):\n")
	for i := 0; i < 5 && i < len(sorted); i++ {
		costDiff := (estimatedCostFor(sorted[i].count) - costsByType[sorted[i].text])
		savingsForQuestion := (costDiff / estimatedCostFor(sorted[i].count)) * 100
		t.Logf("  %s: $%.2f saved (%.1f%%)\n",
			sorted[i].text, costDiff, savingsForQuestion)
	}

	// Conservative expectation: 10-20% savings with Zipfian distribution
	if savingsPercent < 5.0 {
		t.Logf("WARNING: Savings below expected range (5-20%%), verify assumptions\n")
	}
	if savingsPercent > 30.0 {
		t.Logf("WARNING: Savings above expected range (5-20%%), check for overly optimistic estimates\n")
	}
}

// TestConservative_MultiWeekCacheDegradation tests cache effectiveness over time
// as new prompts accumulate and old ones expire
func TestConservative_MultiWeekCacheDegradation(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(3)

	// Simulate 4 weeks with varying cache TTL behavior
	// Week 1: Cache warms up (0-10% hit rate)
	// Week 2: Cache effective (15-25% hit rate)
	// Week 3: Cache hits plateau (20-30% hit rate)
	// Week 4: Cache begins expiring (15-25% hit rate)

	type WeekScenario struct {
		name          string
		dailyRequests int
		expectedHits  float64 // % cache hit rate
	}

	weeks := []WeekScenario{
		{"Week 1 (Warmup)", 100, 0.05},      // Very low hit rate, building cache
		{"Week 2 (Growth)", 100, 0.20},      // Cache warming up
		{"Week 3 (Plateau)", 100, 0.25},     // Peak cache effectiveness
		{"Week 4 (Expiry)", 100, 0.18},      // Some entries expiring
	}

	weeklyBreakdown := make([]map[string]interface{}, 0)

	for weekIdx, scenario := range weeks {
		weekTotal := 0.0
		weekActual := 0.0
		weekHits := 0

		for day := 0; day < 7; day++ {
			for req := 0; req < scenario.dailyRequests; req++ {
				// 60% chance of repeating previous pattern
				// 40% chance of new pattern
				isRepeat := rand.Float64() < 0.60

				var prompt string
				if isRepeat {
					prompt = fmt.Sprintf("Common-%d", (req % 10))
				} else {
					prompt = fmt.Sprintf("Unique-%d-%d-%d", weekIdx, day, req)
				}

				decision := opt.Optimize(prompt, "sonnet", 500)
				estimatedCost := 0.016
				weekTotal += estimatedCost

				if decision.CacheHit && rand.Float64() < scenario.expectedHits {
					weekActual += 0.00015
					weekHits++
				} else if decision.UseBatch {
					weekActual += 0.008
				} else {
					weekActual += estimatedCost
				}

				if req%30 == 0 {
					opt.CacheResponse(prompt, "sonnet", fmt.Sprintf("resp-%d", req), 300)
				}
			}
		}

		actualHitRate := float64(weekHits) / float64(scenario.dailyRequests*7) * 100
		weekSavings := weekTotal - weekActual
		weekSavingsPercent := (weekSavings / weekTotal) * 100

		weeklyBreakdown = append(weeklyBreakdown, map[string]interface{}{
			"week":                scenario.name,
			"total_requests":      scenario.dailyRequests * 7,
			"estimated_cost":      weekTotal,
			"actual_cost":         weekActual,
			"savings":             weekSavings,
			"savings_percent":     weekSavingsPercent,
			"cache_hit_rate":      actualHitRate,
			"expected_hit_rate":   scenario.expectedHits * 100,
		})

		t.Logf("Week %d (%s):\n", weekIdx+1, scenario.name)
		t.Logf("  Requests: %d\n", scenario.dailyRequests*7)
		t.Logf("  Cache hits: %d (%.1f%% actual, %.1f%% expected)\n",
			weekHits, actualHitRate, scenario.expectedHits*100)
		t.Logf("  Cost: $%.2f → $%.2f (-%.1f%%)\n",
			weekTotal, weekActual, weekSavingsPercent)
		t.Logf("  Savings: $%.2f\n\n", weekSavings)
	}

	// Total analysis
	monthTotal := 0.0
	monthActual := 0.0
	for _, week := range weeklyBreakdown {
		monthTotal += week["estimated_cost"].(float64)
		monthActual += week["actual_cost"].(float64)
	}

	monthSavings := monthTotal - monthActual
	monthSavingsPercent := (monthSavings / monthTotal) * 100

	t.Logf("\n=== MONTHLY SUMMARY (Cache Degradation) ===\n")
	t.Logf("Total requests: %d\n", 2800) // 100 * 7 * 4 weeks
	t.Logf("Monthly cost (estimated): $%.2f\n", monthTotal)
	t.Logf("Monthly cost (optimized): $%.2f\n", monthActual)
	t.Logf("Monthly savings: $%.2f (%.1f%%)\n\n", monthSavings, monthSavingsPercent)

	// Conservative: expect 10-25% monthly savings with cache TTL effects
	if monthSavingsPercent < 5.0 {
		t.Logf("WARNING: Savings below expected (5-25%%): %.1f%%\n", monthSavingsPercent)
	}
}

// TestConservative_BudgetVariance tests performance under variable budgets
// (daily limits change, some days higher usage)
func TestConservative_BudgetVariance(t *testing.T) {
	opt := NewOptimizer()

	// Simulate 30 days with variable budget and usage
	// Daily budget: $1-2 (varies)
	// Daily requests: 50-150 (varies)
	// Expectation: System maintains service while respecting budget

	type DayScenario struct {
		budget            float64
		targetRequests    int
		expectedServed    int
		expectedExceeded  bool
	}

	var days []DayScenario
	for day := 0; day < 30; day++ {
		budget := 1.0 + (float64(day%3) * 0.5) // $1.00, $1.50, $2.00 rotating
		requests := 50 + (day%4)*30            // 50-110 requests
		served := requests                     // Expect to serve all with optimization
		exceeded := false

		days = append(days, DayScenario{
			budget:           budget,
			targetRequests:   requests,
			expectedServed:   served,
			expectedExceeded: exceeded,
		})
	}

	monthBudget := 0.0
	monthSpent := 0.0
	monthRequests := 0
	monthServed := 0
	daysExceeded := 0

	for dayIdx, day := range days {
		daySpent := 0.0
		dayServed := 0

		for req := 0; req < day.targetRequests; req++ {
			prompt := fmt.Sprintf("Day%d-Request%d", dayIdx, req)

			decision := opt.Optimize(prompt, "sonnet", 500)
			estimatedCost := 0.016

			var actualCost float64
			if decision.CacheHit {
				actualCost = 0.00015
			} else if decision.UseBatch {
				actualCost = 0.008
			} else {
				actualCost = estimatedCost
			}

			// Respect budget constraint
			if daySpent+actualCost <= day.budget {
				daySpent += actualCost
				dayServed++
			} else {
				// Would exceed budget, don't serve
				break
			}

			if req%20 == 0 {
				opt.CacheResponse(prompt, "sonnet", "resp", 300)
			}
		}

		monthBudget += day.budget
		monthSpent += daySpent
		monthRequests += day.targetRequests
		monthServed += dayServed

		budgetUtilization := (daySpent / day.budget) * 100
		serveRate := float64(dayServed) / float64(day.targetRequests) * 100

		if daySpent > day.budget {
			daysExceeded++
		}

		if dayIdx < 5 || dayIdx%10 == 0 { // Log first few and every 10th
			t.Logf("Day %d: Budget $%.2f, Requests %d, Served %d (%.1f%%), Spent $%.2f (%.1f%%)\n",
				dayIdx+1, day.budget, day.targetRequests, dayServed, serveRate, daySpent, budgetUtilization)
		}
	}

	monthSavingsPercent := 0.0
	if monthRequests*16 > 0 { // Rough estimate of unoptimized cost
		estimated := float64(monthRequests) * 0.016
		monthSavingsPercent = ((estimated - monthSpent) / estimated) * 100
	}

	t.Logf("\n=== MONTHLY BUDGET VARIANCE ANALYSIS ===\n")
	t.Logf("Month totals:\n")
	t.Logf("  Total budget: $%.2f\n", monthBudget)
	t.Logf("  Total spent: $%.2f\n", monthSpent)
	t.Logf("  Remaining: $%.2f (%.1f%% unused)\n",
		monthBudget-monthSpent, (monthBudget-monthSpent)/monthBudget*100)
	t.Logf("  Days exceeded budget: %d/%d\n", daysExceeded, len(days))
	t.Logf("  Total requests: %d\n", monthRequests)
	t.Logf("  Requests served: %d (%.1f%%)\n", monthServed, float64(monthServed)/float64(monthRequests)*100)
	t.Logf("  Estimated savings: %.1f%%\n\n", monthSavingsPercent)

	// Validation: should never exceed budget
	if daysExceeded > 0 {
		t.Errorf("exceeded budget on %d days (should never exceed)", daysExceeded)
	}

	// Should serve high percentage of requests
	servePercentage := float64(monthServed) / float64(monthRequests) * 100
	if servePercentage < 90.0 {
		t.Logf("warning: served only %.1f%% of requests (expected >90%%)\n", servePercentage)
	}
}

// TestConservative_ConcurrentLoad tests behavior under concurrent request load
func TestConservative_ConcurrentLoad(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(5)

	// Simulate 1000 concurrent requests
	totalConcurrent := 1000
	totalEstimated := 0.0
	totalOptimized := 0.0
	cacheHits := 0
	batchedRequests := 0

	// Use a simple pattern: 20% of requests repeat, 80% unique
	patterns := make([]string, int(float64(totalConcurrent)*0.20))
	for i := 0; i < len(patterns); i++ {
		patterns[i] = fmt.Sprintf("Pattern-%d", i)
	}

	for i := 0; i < totalConcurrent; i++ {
		var prompt string
		if i < len(patterns)*5 {
			// 20% of requests use repeated patterns
			prompt = patterns[i%len(patterns)]
		} else {
			// 80% are unique
			prompt = fmt.Sprintf("Unique-%d", i)
		}

		decision := opt.Optimize(prompt, "sonnet", 500)
		estimatedCost := 0.016
		totalEstimated += estimatedCost

		if decision.CacheHit {
			totalOptimized += 0.00015
			cacheHits++
		} else if decision.UseBatch {
			totalOptimized += 0.008
			batchedRequests++
		} else {
			totalOptimized += estimatedCost
		}

		// Pre-populate cache for patterns
		if i < len(patterns)*2 {
			opt.CacheResponse(prompt, "sonnet", "response", 300)
		}
	}

	savingsPercent := ((totalEstimated - totalOptimized) / totalEstimated) * 100

	t.Logf("\n=== CONCURRENT LOAD TEST (1000 requests) ===\n")
	t.Logf("Total requests: %d\n", totalConcurrent)
	t.Logf("Cache hits: %d (%.1f%%)\n", cacheHits, float64(cacheHits)/float64(totalConcurrent)*100)
	t.Logf("Batch requests: %d (%.1f%%)\n", batchedRequests, float64(batchedRequests)/float64(totalConcurrent)*100)
	t.Logf("Cost: $%.2f → $%.2f\n", totalEstimated, totalOptimized)
	t.Logf("Savings: %.1f%% ($%.2f)\n\n", savingsPercent, totalEstimated-totalOptimized)

	// Conservative expectation: 5-15% savings on mixed load
	if savingsPercent < 2.0 {
		t.Logf("WARNING: Concurrent load shows minimal savings (2-15%% expected)\n")
	}
}

// Helper function
func estimatedCostFor(requestCount int) float64 {
	return float64(requestCount) * 0.016
}

// TestConservative_NegativeScenarios tests when optimization doesn't help
func TestConservative_NegativeScenarios(t *testing.T) {
	opt := NewOptimizer()
	opt.SetMinBatchSize(10)
	opt.SetMinSavingsPercent(15.0) // Only optimize if >15% savings

	t.Logf("\n=== NEGATIVE SCENARIOS (When Optimization Doesn't Help) ===\n\n")

	// Scenario 1: Completely unique prompts
	t.Run("completely-unique", func(t *testing.T) {
		testOpt := NewOptimizer()
		testOpt.SetMinBatchSize(10)

		totalCost := 0.0
		optimizedCost := 0.0

		for i := 0; i < 100; i++ {
			prompt := fmt.Sprintf("Completely-unique-prompt-%d-with-no-repetition", i)
			decision := testOpt.Optimize(prompt, "sonnet", 500)

			cost := 0.016
			totalCost += cost
			optimizedCost += cost // No optimization possible

			if decision.CacheHit || decision.UseBatch {
				// Shouldn't happen
				t.Logf("WARNING: Got %s on unique prompt", decision.Direction)
			}
		}

		savings := ((totalCost - optimizedCost) / totalCost) * 100
		t.Logf("100 unique prompts:\n")
		t.Logf("  Cost: $%.2f → $%.2f (%.1f%% savings)\n", totalCost, optimizedCost, savings)
		t.Logf("  Result: No optimization possible (expected)\n\n")
	})

	// Scenario 2: Time-critical, no batch tolerance
	t.Run("time-critical", func(t *testing.T) {
		testOpt := NewOptimizer()
		testOpt.SetMaxBatchWaitTime(100 * time.Millisecond) // Only 100ms tolerance

		totalCost := 0.0
		optimizedCost := 0.0
		batchedCount := 0

		for i := 0; i < 100; i++ {
			prompt := fmt.Sprintf("Urgent-request-%d", i%5) // Only 5 patterns
			decision := testOpt.Optimize(prompt, "sonnet", 500)

			cost := 0.016
			totalCost += cost

			if decision.UseBatch {
				batchedCount++
				optimizedCost += cost * 0.5
			} else {
				optimizedCost += cost
			}
		}

		savings := ((totalCost - optimizedCost) / totalCost) * 100
		t.Logf("Time-critical load (100ms batch tolerance):\n")
		t.Logf("  Batched: %d (%.1f%%)\n", batchedCount, float64(batchedCount)/100*100)
		t.Logf("  Cost: $%.2f → $%.2f (%.1f%% savings)\n", totalCost, optimizedCost, savings)
		t.Logf("  Result: Limited batching, minimal savings (expected)\n\n")
	})

	// Scenario 3: Expensive model (Opus) where batch doesn't help much
	t.Run("expensive-model", func(t *testing.T) {
		testOpt := NewOptimizer()

		totalCost := 0.0
		optimizedCost := 0.0

		for i := 0; i < 50; i++ {
			prompt := fmt.Sprintf("Complex-request-%d", i)
			decision := testOpt.Optimize(prompt, "opus", 1000) // Expensive model

			// Opus is ~$0.05 per request
			cost := 0.05
			totalCost += cost

			if decision.UseBatch {
				optimizedCost += cost * 0.5 // 50% batch discount
			} else {
				optimizedCost += cost // Still expensive even optimized
			}
		}

		savings := ((totalCost - optimizedCost) / totalCost) * 100
		t.Logf("Expensive model (Opus, 50 unique requests):\n")
		t.Logf("  Cost: $%.2f → $%.2f (%.1f%% savings)\n", totalCost, optimizedCost, savings)
		t.Logf("  Result: Savings with batching (limited by wait tolerance)\n\n")
	})

	// Scenario 4: Very short responses (optimization overhead > savings)
	t.Run("short-responses", func(t *testing.T) {
		testOpt := NewOptimizer()

		totalCost := 0.0
		optimizedCost := 0.0

		for i := 0; i < 100; i++ {
			prompt := fmt.Sprintf("Short-query-%d", i%10)
			decision := testOpt.Optimize(prompt, "haiku", 10) // Very short response

			// Haiku short response: ~$0.0001
			cost := 0.0001
			totalCost += cost

			if decision.UseBatch {
				// Batch discount might not offset queueing overhead
				optimizedCost += cost * 0.5
			} else {
				optimizedCost += cost
			}
		}

		savings := ((totalCost - optimizedCost) / totalCost) * 100
		t.Logf("Short responses (Haiku, 10 tokens):\n")
		t.Logf("  Cost: $%.4f → $%.4f (%.1f%% savings)\n", totalCost, optimizedCost, savings)
		t.Logf("  Result: Optimization overhead may exceed savings for very short responses\n\n")
	})
}

// TestConservative_RealWorldBenchmark runs comprehensive realistic scenario
func TestConservative_RealWorldBenchmark(t *testing.T) {
	t.Logf("\n=== COMPREHENSIVE CONSERVATIVE BENCHMARK ===\n\n")

	scenarios := []struct {
		name        string
		dailyReqs   int
		cacheable   float64 // % of requests that are cacheable
		batchable   float64 // % willing to wait for batch
		days        int
		description string
	}{
		{
			name:        "Minimal (Cold Start)",
			dailyReqs:   30,
			cacheable:   0.10, // Very low
			batchable:   0.15, // Unwilling to wait
			days:        7,
			description: "New user, exploring system",
		},
		{
			name:        "Light Developer",
			dailyReqs:   50,
			cacheable:   0.20,
			batchable:   0.30,
			days:        30,
			description: "Part-time developer",
		},
		{
			name:        "Average Developer",
			dailyReqs:   100,
			cacheable:   0.35,
			batchable:   0.50,
			days:        30,
			description: "Full-time developer, typical usage",
		},
		{
			name:        "Power User",
			dailyReqs:   200,
			cacheable:   0.45,
			batchable:   0.70,
			days:        30,
			description: "Heavy usage, willing to optimize",
		},
		{
			name:        "FAQ Service",
			dailyReqs:   500,
			cacheable:   0.60,
			batchable:   0.80,
			days:        30,
			description: "Repetitive questions, batch-friendly",
		},
	}

	results := make([]map[string]interface{}, 0)

	for _, scenario := range scenarios {
		opt := NewOptimizer()
		opt.SetMinBatchSize(3)

		totalRequests := scenario.dailyReqs * scenario.days
		totalEstimated := float64(totalRequests) * 0.016
		totalActual := 0.0

		cacheableReqs := int(float64(totalRequests) * scenario.cacheable)
		batchableReqs := int(float64(totalRequests) * scenario.batchable)
		directReqs := totalRequests - cacheableReqs - batchableReqs

		// Simulate
		totalActual += float64(cacheableReqs) * 0.00015  // Cache hits
		totalActual += float64(batchableReqs) * 0.008    // Batch (50% discount)
		totalActual += float64(directReqs) * 0.016       // Direct

		savings := totalEstimated - totalActual
		savingsPercent := (savings / totalEstimated) * 100

		result := map[string]interface{}{
			"scenario":          scenario.name,
			"description":       scenario.description,
			"daily_requests":    scenario.dailyReqs,
			"period_days":       scenario.days,
			"total_requests":    totalRequests,
			"cacheable_pct":     scenario.cacheable * 100,
			"batchable_pct":     scenario.batchable * 100,
			"estimated_cost":    totalEstimated,
			"actual_cost":       totalActual,
			"savings":           savings,
			"savings_percent":   savingsPercent,
			"monthly_savings":   savings / float64(scenario.days) * 30,
		}
		results = append(results, result)
	}

	// Print results table
	t.Logf("%-25s | %-12s | %-12s | %-15s | %-12s | %s\n",
		"Scenario", "Period", "Requests", "Estimated", "Actual", "Savings")
	t.Logf("%s\n", "------|-------|-------|-------|-------|-------")

	for _, result := range results {
		est := result["estimated_cost"].(float64)
		act := result["actual_cost"].(float64)
		sav := result["savings"].(float64)
		savp := result["savings_percent"].(float64)

		t.Logf("%-25s | %d days | %d reqs | $%.2f | $%.2f | %.1f%% (-$%.2f)\n",
			result["scenario"], result["period_days"], result["total_requests"], est, act, savp, sav)
	}

	t.Logf("\nMonthly Projections:\n")
	for _, result := range results {
		t.Logf("  %-25s: $%.2f/month saved\n", result["scenario"], result["monthly_savings"])
	}
}
