package analytics

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"
	"time"
)

func metricsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE validation_metrics (timestamp DATETIME, task_type TEXT, model TEXT, cached BOOLEAN, batched BOOLEAN, estimated_cost_usd REAL, actual_cost_usd REAL, latency_ms REAL, token_error REAL, success_rate REAL, cache_hits REAL, savings REAL)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func addMetric(t *testing.T, db *sql.DB, task, model string, latency, tokenError float64, cached bool) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO validation_metrics VALUES (datetime('now'), ?, ?, ?, 0, 2, 1, ?, ?, ?, ?, 1)`, task, model, cached, latency, tokenError, 1-tokenError, cached)
	if err != nil {
		t.Fatal(err)
	}
}

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > 1e-8 {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestPercentileMetricSemantics(t *testing.T) {
	db := metricsDB(t)
	for i := 1; i <= 3; i++ {
		addMetric(t, db, "author\u0027s task", "model\u0027quoted", float64(i*10), float64(i)/10, i == 1)
	}
	pc := NewPercentileCalculator(db)
	lp, err := pc.CalculateLatencyPercentiles(7)
	if err != nil {
		t.Fatal(err)
	}
	if lp.Overall.SampleCount != 3 {
		t.Fatalf("overall count=%d", lp.Overall.SampleCount)
	}
	near(t, lp.Overall.P50, 20)
	near(t, lp.Overall.P75, 25)
	near(t, lp.Overall.Min, 10)
	near(t, lp.Overall.Max, 30)
	near(t, lp.Overall.Mean, 20)
	near(t, lp.Overall.StdDev, 10)
	if lp.ByModel["model\u0027quoted"].SampleCount != 3 || lp.ByTask["author\u0027s task"].SampleCount != 3 {
		t.Errorf("quoted groups lost: %+v", lp)
	}
	pm, err := pc.CalculateTokenErrorPercentiles(7)
	if err != nil {
		t.Fatal(err)
	}
	near(t, pm.P50, .2)
	near(t, pm.StdDev, .1)
}

func TestBestModelUsesCostTier(t *testing.T) {
	db := metricsDB(t)
	addMetric(t, db, "code", "opus", 20, .1, false)
	addMetric(t, db, "code", "sonnet", 10, .1, true)
	addMetric(t, db, "code", "aaa-unknown-price", 5, .1, true)
	a := NewTaskAccuracyAnalyzer(db)
	model, rate, err := a.GetBestModelForTask("code", 7)
	if err != nil {
		t.Fatal(err)
	}
	if model != "sonnet" || rate != 1 {
		t.Fatalf("best model=%q rate=%v", model, rate)
	}
}

func TestStoreSentimentRoundtrip(t *testing.T) {
	db := newTestDB(t)
	db.SetMaxOpenConns(1)
	s := NewStore(db)
	sentiments := []string{"satisfied", "neutral", "frustrated", "confused", "impatient"}
	for i, sent := range sentiments {
		rec := validRecord()
		rec.ValidationID = fmt.Sprint(i)
		rec.Timestamp = time.Now().UTC()
		rec.Phase3.UserSentiment.ImplicitSentiment = sent
		rec.Phase3.Learning.Success = i == 0
		rec.Phase3.UserSentiment.FrustrationDetected = i == 2
		rec.Phase3.DecisionMade.Action = "escalate"
		rec.Phase3.DecisionMade.NextModel = "opus"
		if err := s.SaveRecord(rec); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetRecord(rec.ValidationID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Phase1, rec.Phase1) || !reflect.DeepEqual(got.Phase3, rec.Phase3) || !got.Timestamp.Equal(rec.Timestamp) {
			t.Fatalf("record did not roundtrip: %+v", got)
		}
	}
	trend, err := s.GetSentimentTrend(24)
	if err != nil {
		t.Fatal(err)
	}
	if trend.Summary.Total != 5 || trend.Summary.Satisfied != 1 || trend.Summary.Neutral != 1 || trend.Summary.Frustrated != 1 || trend.Summary.Confused != 1 || trend.Summary.Impatient != 1 {
		t.Errorf("summary=%+v", trend.Summary)
	}
	near(t, trend.Summary.SatisfactionRate, .2)
	if len(trend.Timeline) != 1 || trend.Timeline[0].Satisfied != 1 || trend.Timeline[0].Frustrated != 1 || trend.Timeline[0].Neutral != 1 || trend.Timeline[0].Confused != 1 || trend.Timeline[0].Impatient != 1 {
		t.Errorf("timeline=%+v", trend.Timeline)
	}
	if len(trend.Events) != 1 || trend.Events[0].EscalatedTo != "opus" || trend.Events[0].ResolutionTime != 2*time.Second {
		t.Errorf("events=%+v", trend.Events)
	}
	satisfaction, err := s.GetModelSatisfaction("code")
	if err != nil {
		t.Fatal(err)
	}
	if len(satisfaction) != 1 || satisfaction[0].SampleCount != 5 || satisfaction[0].SuccessCount != 1 {
		t.Errorf("satisfaction=%+v", satisfaction)
	}

	budget, err := s.GetBudgetStatus()
	if err != nil {
		t.Fatal(err)
	}
	near(t, budget.DailyBudget.Used, .6)
	near(t, budget.MonthlyBudget.Used, .6)
}

func TestBudgetStatusReportsDatabaseFailure(t *testing.T) {
	db := newTestDB(t)
	db.Close()
	if _, err := NewStore(db).GetBudgetStatus(); err == nil {
		t.Fatal("closed database silently reported zero budget")
	}
}

func TestAccuracyAggregationsFilteringAndMinimumSamples(t *testing.T) {
	db := metricsDB(t)
	for i := 0; i < 5; i++ {
		addMetric(t, db, "code", "haiku", float64(10+i), .1, i%2 == 0)
	}
	for i := 0; i < 5; i++ {
		addMetric(t, db, "hard", "opus", 100, .2, false)
	}
	addMetric(t, db, "rare", "sonnet", 10, .1, false)
	if _, err := db.Exec(`INSERT INTO validation_metrics (timestamp,task_type,model,latency_ms,token_error) VALUES (datetime('now','-40 days'),'code','haiku',999,.9)`); err != nil {
		t.Fatal(err)
	}
	a := NewTaskAccuracyAnalyzer(db)
	acc, err := a.CalculateTaskModelAccuracy("code", "haiku", 7)
	if err != nil {
		t.Fatal(err)
	}
	if acc.TotalCount != 5 || acc.SuccessCount != 5 || acc.TaskType != "code" || acc.Model != "haiku" {
		t.Fatalf("accuracy=%+v", acc)
	}
	near(t, acc.SuccessRate, 1)
	near(t, acc.AvgTokenError, .1)
	near(t, acc.AvgLatencyMs, 12)
	all, err := a.GetAllTaskAccuracies(7, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].TaskType != "code" || all[1].TaskType != "hard" || all[1].SuccessCount != 0 {
		t.Fatalf("all=%+v", all)
	}
	tasks, err := a.GetTasksByModel("haiku", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TotalCount != 5 {
		t.Fatalf("tasks=%+v", tasks)
	}
	difficulty, err := a.GetTaskDifficulty(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(difficulty) != 3 || difficulty[0].TaskType != "hard" || difficulty[0].SuccessRate != 0 {
		t.Fatalf("difficulty=%+v", difficulty)
	}
	if _, err := a.CalculateTaskModelAccuracy("missing", "none", 7); err == nil {
		t.Error("missing data accepted")
	}
	if _, _, err := a.GetBestModelForTask("hard", 7); err == nil {
		t.Error("unsuccessful model recommended")
	}
	ca := NewCorrelationAnalyzer(db)
	grouped, err := ca.TaskModelCorrelation(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped) != 2 || grouped["code:haiku"] != 1 || grouped["hard:opus"] != 0 {
		t.Fatalf("task/model correlation=%+v", grouped)
	}
}

func TestCorrelationDirectionSignificanceAndCacheCosts(t *testing.T) {
	db := metricsDB(t)
	// Every cost and latency increases while accuracy falls: exactly +/-1 correlation.
	for i := 1; i <= 10; i++ {
		_, err := db.Exec(`INSERT INTO validation_metrics VALUES (datetime('now'),'code','haiku',?, ?, 20, ?, ?, ?, ?, ?, ?)`, i <= 5, i <= 5, float64(i), float64(i*10), float64(i)/100, 1-float64(i)/100, 11-i, 11-i)
		if err != nil {
			t.Fatal(err)
		}
	}
	ca := NewCorrelationAnalyzer(db)
	corr, err := ca.AnalyzeCorrelations(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(corr) != 6 {
		t.Fatalf("correlations=%+v", corr)
	}
	for _, c := range corr {
		if !c.Significant || c.PValue >= .05 {
			t.Errorf("expected significant correlation: %+v", c)
		}
		if c.Variable1 == "batched" {
			near(t, c.Coefficient, 0.8703882797784892)
		} else if c.Variable1 == "success_rate" || c.Variable1 == "cache_hits" {
			near(t, c.Coefficient, -1)
		} else {
			near(t, c.Coefficient, 1)
		}
	}
	cache, err := ca.CacheEffectiveness(7)
	if err != nil {
		t.Fatal(err)
	}
	near(t, cache["cache_hit_rate"].(float64), .5)
	near(t, cache["cached_cost_usd"].(float64), 15)
	near(t, cache["uncached_cost_usd"].(float64), 40)
	near(t, cache["cached_avg_latency"].(float64), 30)
	near(t, cache["uncached_avg_latency"].(float64), 80)
	if _, err := db.Exec("DELETE FROM validation_metrics"); err != nil {
		t.Fatal(err)
	}
	empty, err := ca.AnalyzeCorrelations(7)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty correlations=%+v err=%v", empty, err)
	}
	emptyCache, err := ca.CacheEffectiveness(7)
	if err != nil {
		t.Fatal(err)
	}
	near(t, emptyCache["total_savings"].(float64), 0)
	addMetric(t, db, "code", "haiku", 1, .125, false)
	if got, err := ca.AnalyzeCorrelations(7); err != nil || len(got) != 0 {
		t.Fatalf("one sample correlation=%+v %v", got, err)
	}
	addMetric(t, db, "code", "haiku", 1, .125, false)
	if got, err := ca.AnalyzeCorrelations(7); err != nil || len(got) != 0 {
		t.Fatalf("constant correlation=%+v %v", got, err)
	}
}

func TestAnalyticsDatabaseErrorBoundaries(t *testing.T) {
	db := metricsDB(t)
	db.Close()
	pc := NewPercentileCalculator(db)
	if _, err := pc.CalculateTokenErrorPercentiles(7); err == nil {
		t.Error("closed percentile query accepted")
	}
	a := NewTaskAccuracyAnalyzer(db)
	if _, err := a.CalculateTaskModelAccuracy("x", "y", 7); err == nil {
		t.Error("closed accuracy query accepted")
	}
	if _, err := a.GetAllTaskAccuracies(7, 1); err == nil {
		t.Error("closed all accuracy query accepted")
	}
	if _, _, err := a.GetBestModelForTask("x", 7); err == nil {
		t.Error("closed best model query accepted")
	}
	if _, err := a.GetTaskDifficulty(7); err == nil {
		t.Error("closed difficulty query accepted")
	}
	if _, err := a.GetTasksByModel("x", 7); err == nil {
		t.Error("closed tasks query accepted")
	}
	ca := NewCorrelationAnalyzer(db)
	if _, err := ca.CacheEffectiveness(7); err == nil {
		t.Error("closed cache query accepted")
	}
	if _, err := ca.TaskModelCorrelation(7); err == nil {
		t.Error("closed correlation query accepted")
	}
	ts := NewTimeSeriesStore(db)
	if err := ts.CreateBuckets(); err == nil {
		t.Error("closed buckets accepted")
	}
	if err := ts.AggregateHourly(); err == nil {
		t.Error("closed aggregation accepted")
	}
	if _, err := ts.GetTrend("daily", 7); err == nil {
		t.Error("closed trend accepted")
	}
	if _, err := ts.GetTrend("injected", 7); err == nil {
		t.Error("invalid trend bucket accepted")
	}
	if err := ts.EnforceRetention(1, 1, 1); err == nil {
		t.Error("closed retention accepted")
	}
	s := NewStore(db)
	if _, err := s.GetRecord("missing"); err == nil {
		t.Error("closed record lookup accepted")
	}
	if _, err := s.GetSentimentTrend(24); err == nil {
		t.Error("closed sentiment query accepted")
	}
	if _, err := s.GetModelSatisfaction("code"); err == nil {
		t.Error("closed satisfaction query accepted")
	}
	if err := s.SaveRecord(validRecord()); err == nil {
		t.Error("closed record write accepted")
	}
}

func TestRecordInvalidJSONAndNonFinitePhases(t *testing.T) {
	db := newTestDB(t)
	db.SetMaxOpenConns(1)
	s := NewStore(db)
	if _, err := s.GetRecord("missing"); err == nil {
		t.Fatal("missing record accepted")
	}
	for phase, update := range map[string]string{
		"phase1_data": "UPDATE analytics_records SET phase1_data = ? WHERE validation_id = ?",
		"phase2_data": "UPDATE analytics_records SET phase2_data = ? WHERE validation_id = ?",
		"phase3_data": "UPDATE analytics_records SET phase3_data = ? WHERE validation_id = ?",
	} {
		rec := validRecord()
		rec.ValidationID = phase
		if err := s.SaveRecord(rec); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(update, "[invalid", phase); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetRecord(phase); err == nil {
			t.Errorf("invalid %s accepted", phase)
		}
	}
	for i := 0; i < 3; i++ {
		rec := validRecord()
		rec.ValidationID = fmt.Sprint("nan", i)
		switch i {
		case 0:
			rec.Phase1.Complexity = math.NaN()
		case 1:
			rec.Phase2.SentimentDuring.FrustrationRisk = math.Inf(1)
		case 2:
			rec.Phase3.ActualCostUSD = math.NaN()
		}
		if err := s.SaveRecord(rec); err == nil {
			t.Errorf("nonfinite phase %d accepted", i)
		}
	}
	if _, err := db.Exec("DROP TABLE frustration_events"); err != nil {
		t.Fatal(err)
	}
	rec := validRecord()
	rec.ValidationID = "rollback"
	rec.Phase3.UserSentiment.FrustrationDetected = true
	if err := s.SaveRecord(rec); err == nil {
		t.Error("missing frustration table accepted")
	}
	if _, err := s.GetRecord("rollback"); err == nil {
		t.Error("failed transaction left record")
	}
	if _, err := db.Exec("DROP TABLE budget_history"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetBudgetStatus(); err == nil {
		t.Error("missing budget table accepted")
	}
}

func TestPercentilesEmptySingletonAndInterpolation(t *testing.T) {
	db := metricsDB(t)
	pc := NewPercentileCalculator(db)
	pm, err := pc.CalculateTokenErrorPercentiles(7)
	if err != nil || pm.SampleCount != 0 {
		t.Fatalf("empty percentiles=%+v %v", pm, err)
	}
	addMetric(t, db, "code", "haiku", 10, .1, false)
	lp, err := pc.CalculateLatencyPercentiles(7)
	if err != nil {
		t.Fatal(err)
	}
	near(t, lp.Overall.StdDev, 0)
	near(t, lp.Overall.P99, 10)
	for _, tc := range []struct {
		data    []float64
		p, want float64
	}{{nil, .5, 0}, {[]float64{10}, .5, 10}, {[]float64{0, 10}, 0, 0}, {[]float64{0, 10}, 1, 10}, {[]float64{0, 10}, .25, 2.5}, {[]float64{0, 10}, -1, 0}, {[]float64{0, 10}, 2, 10}} {
		near(t, percentile(tc.data, tc.p), tc.want)
	}
	near(t, calculateMean(nil), 0)
	near(t, calculateStdDev(nil, 0), 0)
}

func TestForecastSingularConstantAndBudgetBoundaries(t *testing.T) {
	fm := NewForecastModel("cost")
	now := time.Now().UTC()
	if _, err := fm.Predict(1); err == nil {
		t.Error("untrained model predicted")
	}
	if err := fm.Train([]float64{1, 2}, []time.Time{now, now}); err == nil {
		t.Error("singular training accepted")
	}
	if err := fm.Train([]float64{0, 0, 0}, []time.Time{now, now.Add(24 * time.Hour), now.Add(48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if fm.RSquared != 1 || fm.DetectTrendChange(.1) {
		t.Fatal("constant forecast quality mismatch")
	}
	fm.Slope = 1
	budget := fm.PredictBudgetExceeded(11, 10)
	if budget.DaysRemaining != 0 || budget.ExceededAt == nil {
		t.Errorf("exhausted budget=%+v", budget)
	}
}

// FuzzPercentileBounds exercises the interpolation contract on arbitrary sorted samples.
func FuzzPercentileBounds(f *testing.F) {
	f.Add([]byte{0, 10, 20, 30}, uint8(50))
	f.Add([]byte{}, uint8(99))
	f.Add([]byte{42}, uint8(0))
	f.Fuzz(func(t *testing.T, raw []byte, percentage uint8) {
		if len(raw) > 1024 {
			raw = raw[:1024]
		}
		data := make([]float64, len(raw))
		for i, b := range raw {
			data[i] = float64(b)
		}
		sort.Float64s(data)
		p := float64(percentage%101) / 100
		got := percentile(data, p)
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Fatal("nonfinite percentile")
		}
		if len(data) == 0 {
			if got != 0 {
				t.Fatal("empty percentile is nonzero")
			}
			return
		}
		if got < data[0] || got > data[len(data)-1] {
			t.Fatal("percentile outside sample bounds")
		}
		if percentile(data, 0) != data[0] || percentile(data, 1) != data[len(data)-1] {
			t.Fatal("endpoint mismatch")
		}
		q := math.Min(1, p+.01)
		if percentile(data, q) < got {
			t.Fatal("percentiles are not monotonic")
		}
		shifted := make([]float64, len(data))
		for i, v := range data {
			shifted[i] = v + 10
		}
		near(t, percentile(shifted, p), got+10)
	})
}

func TestBestModelCanonicalIDs(t *testing.T) {
	db := metricsDB(t)
	addMetric(t, db, "code", "claude-opus-4-6", 20, .1, false)
	addMetric(t, db, "code", "claude-sonnet-4-6", 10, .1, false)
	model, _, err := NewTaskAccuracyAnalyzer(db).GetBestModelForTask("code", 7)
	if err != nil {
		t.Fatal(err)
	}
	if model != "claude-sonnet-4-6" {
		t.Fatalf("canonical best model=%q", model)
	}
}
