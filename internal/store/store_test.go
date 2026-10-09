package store

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestPersistentAccountingAndPruning(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.GetDB() == nil {
		t.Fatal("missing db")
	}
	if st, err := s.GetValidationStats(); err != nil || st["total_metrics"] != 0 {
		t.Fatalf("empty stats %v %v", st, err)
	}
	for i := 0; i < 105; i++ {
		if err := s.LogTurn(fmt.Sprint(i%2), fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	turns, err := s.RecentTurns(200)
	if err != nil || len(turns) != 100 || turns[0].Concepts != "104" || turns[99].Concepts != "5" {
		t.Fatalf("pruning %v %v", turns, err)
	}
	if n, err := s.CountRecentAttempts("0", 10); err != nil || n != 5 {
		t.Fatalf("attempts %d %v", n, err)
	}
	for _, reason := range []string{"failure", "failure", "success"} {
		if err := s.LogEscalation("small", "large", "code", reason); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.EscalationCountForType("code"); err != nil || n != 2 {
		t.Fatalf("escalations %d %v", n, err)
	}
	stats, err := s.TaskTypeStatsAll()
	if err != nil || len(stats) != 1 || stats[0].SuccessRate != 50 {
		t.Fatalf("stats %v %v", stats, err)
	}
	events, err := s.RecentEscalations(9)
	if err != nil || len(events) != 3 || events[0].ID != 3 || events[0].Reason != "success" {
		t.Fatalf("events %v %v", events, err)
	}
	if err := s.SetSession("session", "value"); err != nil {
		t.Fatal(err)
	}
	for _, m := range []ValidationMetric{{EstimatedTotalTokens: 100, ActualTotalTokens: 80, EstimatedCost: 2, ActualCost: 1, Validated: true, TokenError: 20, CostError: 50}, {EstimatedTotalTokens: 50, ActualTotalTokens: 40}} {
		if err := s.LogValidationMetric(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.LogValidationMetric(ValidationMetric{ActualCost: math.NaN()}); err == nil {
		t.Fatal("nonfinite cost accepted")
	}
	st, err := s.GetValidationStats()
	if err != nil || st["total_metrics"] != 2 || st["validated"] != 1 || st["estimated_total"] != 150 || st["actual_total"] != 120 || st["avg_token_error"] != float64(20) {
		t.Fatalf("validation %v %v", st, err)
	}
	if _, err := s.GetValidationMetric("absent"); err == nil {
		t.Fatal("missing metric accepted")
	}
	m, err := s.GetValidationMetric(string(itob(1)))
	if err != nil || m.ID != 1 {
		t.Fatalf("metric %v %v", m, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if val, err := s.GetSession("session"); err != nil || val != "value" {
		t.Fatalf("persistence %q %v", val, err)
	}
	if err := s.DeleteSession("session"); err != nil {
		t.Fatal(err)
	}
	if val, err := s.GetSession("session"); err != nil || val != "" {
		t.Fatal(val, err)
	}
	if err := s.SetSession("", "bad"); err == nil {
		t.Fatal("empty key accepted")
	}
	// Corrupt records must not prevent reading or accounting for intact data.
	if err := s.db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketEscalations, bucketTurns, bucketValidation} {
			if err := tx.Bucket(b).Put(itob(999), []byte("invalid")); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if all, err := s.GetAllValidationMetrics(); err != nil || len(all) != 2 {
		t.Fatal(all, err)
	}
	if all, err := s.GetValidationMetrics(3); err != nil || len(all) != 2 || all[0].ID != 2 {
		t.Fatal(all, err)
	}
	if _, err := s.GetValidationMetric(string(itob(999))); err == nil {
		t.Fatal("corrupt metric accepted")
	}
	if _, err := s.GetValidationStats(); err != nil {
		t.Fatal(err)
	}
	if all, err := s.RecentTurns(2); err != nil || len(all) != 1 {
		t.Fatal(all, err)
	}
	if _, err := s.CountRecentAttempts("0", 200); err != nil {
		t.Fatal(err)
	}
	if all, err := s.RecentEscalations(2); err != nil || len(all) != 1 {
		t.Fatal(all, err)
	}
	if _, err := s.TaskTypeStatsAll(); err != nil {
		t.Fatal(err)
	}
	if n, err := s.EscalationCountForType("code"); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if e, d, n, err := s.TotalStats(); err != nil || e != 2 || d != 1 || n != 101 {
		t.Fatal(e, d, n, err)
	}
}

func TestStoreFailures(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(blocker, "child")); err == nil {
		t.Fatal("invalid directory accepted")
	}
	if err := os.Mkdir(filepath.Join(dir, "escalation.db"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("invalid database path accepted")
	}
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, call := range []func() error{
		func() error { return s.LogEscalation("a", "b", "c", "d") }, func() error { return s.LogTurn("a", "b") }, func() error { _, e := s.RecentTurns(1); return e }, func() error { _, e := s.CountRecentAttempts("a", 1); return e }, func() error { _, e := s.TaskTypeStatsAll(); return e }, func() error { _, e := s.EscalationCountForType("a"); return e }, func() error { _, _, _, e := s.TotalStats(); return e }, func() error { _, e := s.RecentEscalations(1); return e }, func() error { return s.SetSession("a", "b") }, func() error { _, e := s.GetSession("a"); return e }, func() error { return s.DeleteSession("a") }, func() error { return s.LogValidationMetric(ValidationMetric{}) }, func() error { _, e := s.GetValidationMetrics(1); return e }, func() error { _, e := s.GetValidationStats(); return e }, func() error { _, e := s.GetValidationMetric("a"); return e }, func() error { _, e := s.GetAllValidationMetrics(); return e },
	} {
		if err := call(); err == nil {
			t.Fatal("closed database operation succeeded")
		}
	}
}

func TestValidationDecimalIDRetrieval(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.LogValidationMetric(ValidationMetric{EstimatedTotalTokens: 42}); err != nil {
		t.Fatal(err)
	}
	metric, err := s.GetValidationMetric("1")
	if err != nil || metric.ID != 1 || metric.EstimatedTotalTokens != 42 {
		t.Fatalf("decimal lookup %+v %v", metric, err)
	}
	id, err := s.LogValidationMetricWithID(ValidationMetric{EstimatedTotalTokens: 80})
	if err != nil || id != 2 {
		t.Fatal(id, err)
	}
	metric, err = s.GetValidationMetric(fmt.Sprint(id))
	if err != nil || metric.EstimatedTotalTokens != 80 {
		t.Fatal(metric, err)
	}
	id, err = s.LogValidationMetricWithID(ValidationMetric{ActualCost: math.NaN()})
	if err == nil || id != 0 {
		t.Fatal("failed insertion exposes committed ID", id, err)
	}
}
