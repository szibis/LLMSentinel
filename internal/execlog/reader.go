package execlog

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
)

// Reader loads and queries execution logs
type Reader struct {
	entries []Entry
}

// NewReader creates a reader and loads entries from file
func NewReader(logFilePath string) (*Reader, error) {
	r := &Reader{
		entries: []Entry{},
	}
	if err := r.load(logFilePath); err != nil {
		return nil, err
	}
	return r, nil
}

// load reads JSON lines from log file
func (r *Reader) load(logFilePath string) error {
	f, err := os.Open(logFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		r.entries = append(r.entries, entry)
	}
	return scanner.Err()
}

// SessionMetrics returns metrics for a specific or most recent session
func (r *Reader) SessionMetrics(sessionID string) SessionMetrics {
	if sessionID == "" && len(r.entries) > 0 {
		sessionID = r.entries[len(r.entries)-1].SessionID
	}

	var sessionEntries []Entry
	for _, e := range r.entries {
		if e.SessionID == sessionID {
			sessionEntries = append(sessionEntries, e)
		}
	}

	metrics := SessionMetrics{
		SessionID:        sessionID,
		TotalOperations:  len(sessionEntries),
		OperationsByType: make(map[string]int),
	}

	for _, e := range sessionEntries {
		metrics.TotalDurationMS += e.DurationMS
		metrics.EstimatedTokens += e.TokensEstimate
		metrics.OperationsByType[e.OperationType]++
		if e.Status == "success" {
			metrics.SuccessRate += 1
		}
	}

	if len(sessionEntries) > 0 {
		metrics.AvgDurationMS = metrics.TotalDurationMS / int64(len(sessionEntries))
		metrics.SuccessRate /= float64(len(sessionEntries))
	}

	return metrics
}

// SlowestOperations returns the N slowest operations by average duration
func (r *Reader) SlowestOperations(limit int) []OperationStats {
	stats := r.aggregateByCommand()

	var results []OperationStats
	for _, s := range stats {
		if s.AvgDurationMS > 2000 {
			results = append(results, s)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].AvgDurationMS > results[j].AvgDurationMS
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// FastestOperations returns operations under 500ms
func (r *Reader) FastestOperations(limit int) []OperationStats {
	stats := r.aggregateByCommand()

	var results []OperationStats
	for _, s := range stats {
		if s.AvgDurationMS < 500 {
			results = append(results, s)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].AvgDurationMS < results[j].AvgDurationMS
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// CachingOpportunities finds commands repeated 3+ times
func (r *Reader) CachingOpportunities() []CachingOpportunity {
	cmdMap := make(map[string][]Entry)
	for _, e := range r.entries {
		cmdMap[e.CommandNormalized] = append(cmdMap[e.CommandNormalized], e)
	}

	var opportunities []CachingOpportunity
	for cmd, entries := range cmdMap {
		if len(entries) >= 3 {
			var durations []int64
			totalTime := int64(0)
			for _, e := range entries {
				durations = append(durations, e.DurationMS)
				totalTime += e.DurationMS
			}

			var avgDur int64
			if len(durations) > 0 {
				avgDur = totalTime / int64(len(durations))
			}

			opportunities = append(opportunities, CachingOpportunity{
				Operation:        cmd,
				Repetitions:      len(entries),
				AvgDurationMS:    int64(avgDur),
				TotalTimeMS:      totalTime,
				PotentialSavings: totalTime - durations[0],
			})
		}
	}

	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].PotentialSavings > opportunities[j].PotentialSavings
	})

	if len(opportunities) > 5 {
		opportunities = opportunities[:5]
	}
	return opportunities
}

// aggregateByCommand groups entries by normalized command and calculates stats
func (r *Reader) aggregateByCommand() []OperationStats {
	cmdMap := make(map[string][]Entry)
	for _, e := range r.entries {
		cmdMap[e.CommandNormalized] = append(cmdMap[e.CommandNormalized], e)
	}

	var results []OperationStats
	for cmd, entries := range cmdMap {
		var durations []int64
		successCount := 0
		for _, e := range entries {
			durations = append(durations, e.DurationMS)
			if e.Status == "success" {
				successCount++
			}
		}

		if len(durations) == 0 {
			continue
		}

		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

		totalTime := int64(0)
		for _, d := range durations {
			totalTime += d
		}

		avgDur := totalTime / int64(len(durations))
		cachingPotential := "low"
		if len(entries) >= 3 && avgDur > 1000 {
			cachingPotential = "high"
		} else if len(entries) >= 2 {
			cachingPotential = "medium"
		}

		results = append(results, OperationStats{
			Operation:        cmd,
			AvgDurationMS:    avgDur,
			MaxDurationMS:    durations[len(durations)-1],
			MinDurationMS:    durations[0],
			ExecutionCount:   len(entries),
			SuccessCount:     successCount,
			FailureCount:     len(entries) - successCount,
			TotalTimeMS:      totalTime,
			CachingPotential: cachingPotential,
		})
	}

	return results
}

// AllEntries returns all logged entries
func (r *Reader) AllEntries() []Entry {
	return r.entries
}

// Count returns the total number of entries
func (r *Reader) Count() int {
	return len(r.entries)
}
