package patterns

import (
	"fmt"
	"os"
	"time"

	"github.com/szibis/claude-escalate/internal/execlog"
)

// Generator creates EXECUTION_PATTERNS.md from execution logs
type Generator struct {
	reader *execlog.Reader
}

// New creates a new pattern generator
func New(reader *execlog.Reader) *Generator {
	return &Generator{reader: reader}
}

// Generate creates markdown content from execution logs
func (g *Generator) Generate() string {
	stats := g.reader.SlowestOperations(10)
	fast := g.reader.FastestOperations(10)
	caching := g.reader.CachingOpportunities()
	totalOps := g.reader.Count()

	md := fmt.Sprintf(`# Execution Patterns & Optimization Guide

*Generated %s from %d operations*

## Fast Operations (< 500ms)
✅ These operations are safe to call frequently without performance concerns.

`, time.Now().Format("2006-01-02 15:04:05"), totalOps)

	for _, op := range fast {
		md += fmt.Sprintf("✅ **%s** — %dms avg (ran %dx)\n", truncate(op.Operation, 60), op.AvgDurationMS, op.ExecutionCount)
	}

	md += fmt.Sprintf(`
## Slow Operations (> 2s)
⚠️ These operations take significant time. Consider optimization or caching.

`)

	for _, op := range stats {
		savings := op.TotalTimeMS - op.AvgDurationMS
		md += fmt.Sprintf(`⚠️ **%s** — %dms avg
  - Total time spent: %dms across %d executions
  - Optimization: Consider caching results or batching calls
  - Impact: If cached, could save %dms

`, truncate(op.Operation, 60), op.AvgDurationMS, op.TotalTimeMS, op.ExecutionCount, savings)
	}

	md += fmt.Sprintf(`## Caching Opportunities
💾 Commands that repeated 3+ times (prime candidates for caching)

`)

	for _, opp := range caching {
		md += fmt.Sprintf(`💾 **%s**
  - Repetitions: %d
  - Avg Duration: %dms
  - Total Time: %dms
  - Potential Savings: %dms

`, truncate(opp.Operation, 60), opp.Repetitions, opp.AvgDurationMS, opp.TotalTimeMS, opp.PotentialSavings)
	}

	md += `## Token Savings Tips

📊 Recommended optimizations based on patterns:

1. **Use wc -l before Read tool**: Saves ~90% tokens when checking file size
2. **Use grep -n to locate, then Read with offset/limit**: Saves 90%+ tokens on large files
3. **Use LSP for symbol search instead of grep**: More accurate, better token efficiency
4. **Batch similar operations**: Reduces context switching overhead
5. **Cache repeated web fetches**: Same URL fetched multiple times = cache hit opportunity

## Best Practices for This Project

🎯 Based on your execution patterns:

`

	slowCount := len(stats)
	if slowCount >= 3 {
		md += fmt.Sprintf("- Many slow operations detected (%d). Consider profiling and optimization.\n", slowCount)
	}

	if len(caching) > 0 {
		md += fmt.Sprintf("- Found %d caching opportunities. Use cache decorator or memoization.\n", len(caching))
	}

	md += `
## Decision Pattern Analysis

📈 Efficiency notes:

- Operations that repeat often should be cached or executed once with results reused
- Slow operations should have execution time tracked and investigated
- Web fetches with CSS selectors are more efficient than full-page extracts

## Next Steps

1. Review this guide for your project's specific patterns
2. Implement caching for high-repeat, high-time operations
3. Re-generate this guide periodically to track optimization impact
4. Use escalate analytics command to get detailed performance data

---

*This guide was auto-generated. Run 'escalate generate-patterns' to refresh with latest execution data.*
`

	return md
}

// WriteFile writes the generated patterns to a file
func (g *Generator) WriteFile(filename string) error {
	content := g.Generate()
	return os.WriteFile(filename, []byte(content), 0644)
}

// truncate limits string to max length
func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
