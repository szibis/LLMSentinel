package labdashboard

import "sort"

type routeObservation struct {
	Role     string   `json:"role"`
	Upstream string   `json:"upstream"`
	Thinking bool     `json:"thinking"`
	Count    int      `json:"observed_attempts"`
	Accepted int      `json:"protocol_accepted"`
	P50      *float64 `json:"p50_ms"`
	P95      *float64 `json:"p95_ms"`
}

// These summaries include only attempts observed by dashboard polling. They
// are neither complete request accounting nor semantic quality scores.
func routeSummary(attempts []observedAttempt) []routeObservation {
	type identity struct {
		role, upstream string
		thinking       bool
	}
	rows := map[identity]*routeObservation{}
	latencies := map[identity][]float64{}
	for _, a := range attempts {
		key := identity{a.Role, a.Upstream, a.Thinking}
		row := rows[key]
		if row == nil {
			row = &routeObservation{Role: a.Role, Upstream: a.Upstream, Thinking: a.Thinking}
			rows[key] = row
		}
		row.Count++
		if a.Accepted {
			row.Accepted++
		}
		if a.LatencyMS >= 0 {
			latencies[key] = append(latencies[key], float64(a.LatencyMS))
		}
	}
	percentile := func(values []float64, q float64) *float64 {
		if len(values) == 0 {
			return nil
		}
		sort.Float64s(values)
		index := float64(len(values)-1) * q
		lower := int(index)
		value := values[lower]
		if lower+1 < len(values) {
			value += (values[lower+1] - value) * (index - float64(lower))
		}
		return &value
	}
	result := []routeObservation{}
	for key, row := range rows {
		row.P50 = percentile(latencies[key], 0.5)
		row.P95 = percentile(latencies[key], 0.95)
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Role != result[j].Role {
			return result[i].Role < result[j].Role
		}
		if result[i].Upstream != result[j].Upstream {
			return result[i].Upstream < result[j].Upstream
		}
		return !result[i].Thinking && result[j].Thinking
	})
	return result
}
