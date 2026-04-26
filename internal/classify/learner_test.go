package classify

import (
	"testing"
	"time"
)

func TestNewLearner(t *testing.T) {
	learner := NewLearner(100, 1*time.Hour)

	if learner == nil {
		t.Fatal("expected non-nil learner")
	}
	if learner.maxHistorySize != 100 {
		t.Errorf("expected max history 100, got %d", learner.maxHistorySize)
	}
	if learner.updateFrequency != 1*time.Hour {
		t.Errorf("expected update frequency 1h, got %v", learner.updateFrequency)
	}
}

func TestRecordOutcome(t *testing.T) {
	learner := NewLearner(100, 1*time.Hour)

	event := LearningEvent{
		ID:            "test-1",
		Prompt:        "test prompt",
		PredictedTask: TaskConcurrency,
		ActualTask:    TaskConcurrency,
		Succeeded:     true,
		TokenError:    0.05,
	}

	learner.RecordOutcome(event)

	if len(learner.recentEvents) != 1 {
		t.Errorf("expected 1 event recorded, got %d", len(learner.recentEvents))
	}

	// Check task accuracy was updated
	acc := learner.GetTaskAccuracy(TaskConcurrency)
	if acc.TotalCount != 1 {
		t.Errorf("expected total count 1, got %d", acc.TotalCount)
	}
	if acc.SuccessCount != 1 {
		t.Errorf("expected success count 1, got %d", acc.SuccessCount)
	}
	if diff := acc.SuccessRate - 1.0; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected success rate 1.0, got %f", acc.SuccessRate)
	}
}

func TestRecordOutcomeMaxHistory(t *testing.T) {
	learner := NewLearner(3, 1*time.Hour)

	// Record 5 events, should keep only last 3
	for i := 0; i < 5; i++ {
		event := LearningEvent{
			ID:            "test-" + string(rune(i)),
			PredictedTask: TaskConcurrency,
			ActualTask:    TaskConcurrency,
			Succeeded:     true,
		}
		learner.RecordOutcome(event)
	}

	if len(learner.recentEvents) != 3 {
		t.Errorf("expected max 3 events, got %d", len(learner.recentEvents))
	}

	// Check task accuracy still counts all 5
	acc := learner.GetTaskAccuracy(TaskConcurrency)
	if acc.TotalCount != 5 {
		t.Errorf("accuracy should track all 5, got %d", acc.TotalCount)
	}
}

func TestGetTaskAccuracy(t *testing.T) {
	learner := NewLearner(100, 1*time.Hour)

	// Record mixed outcomes
	outcomes := []struct {
		task      TaskType
		predicted TaskType
		success   bool
		error     float64
	}{
		{TaskConcurrency, TaskConcurrency, true, 0.05},
		{TaskConcurrency, TaskConcurrency, false, 0.25},
		{TaskConcurrency, TaskParsing, true, 0.10},
		{TaskParsing, TaskParsing, true, 0.05},
		{TaskParsing, TaskParsing, true, 0.08},
	}

	for i, o := range outcomes {
		learner.RecordOutcome(LearningEvent{
			ID:             "test-" + string(rune(i)),
			PredictedTask:  o.predicted,
			ActualTask:     o.task,
			Succeeded:      o.success,
			TokenError:     o.error,
			ConfidenceScore: 0.8,
		})
	}

	// Check concurrency accuracy
	concAcc := learner.GetTaskAccuracy(TaskConcurrency)
	if concAcc.TotalCount != 3 {
		t.Errorf("concurrency: expected total 3, got %d", concAcc.TotalCount)
	}
	if concAcc.SuccessCount != 2 {
		t.Errorf("concurrency: expected success 2, got %d", concAcc.SuccessCount)
	}

	expectedRate := 2.0 / 3.0
	if diff := concAcc.SuccessRate - expectedRate; diff < -0.01 || diff > 0.01 {
		t.Errorf("concurrency: expected rate ~%.2f, got %f", expectedRate, concAcc.SuccessRate)
	}

	// Check parsing accuracy
	parseAcc := learner.GetTaskAccuracy(TaskParsing)
	if parseAcc.TotalCount != 2 {
		t.Errorf("parsing: expected total 2, got %d", parseAcc.TotalCount)
	}
	if parseAcc.SuccessCount != 2 {
		t.Errorf("parsing: expected success 2, got %d", parseAcc.SuccessCount)
	}
}

func TestUpdateFromBatch(t *testing.T) {
	learner := NewLearner(100, 1*time.Hour)

	// Record some misclassified events
	for i := 0; i < 5; i++ {
		learner.RecordOutcome(LearningEvent{
			ID:             "test-" + string(rune(i)),
			PredictedTask:  TaskConcurrency,
			ActualTask:     TaskConcurrency,
			Succeeded:      true,
		})
	}

	// Add misclassifications
	learner.RecordOutcome(LearningEvent{
		ID:            "wrong-1",
		PredictedTask: TaskConcurrency,
		ActualTask:    TaskParsing,
		Succeeded:     false,
	})
	learner.RecordOutcome(LearningEvent{
		ID:            "wrong-2",
		PredictedTask: TaskOptimization,
		ActualTask:    TaskDatabase,
		Succeeded:     false,
	})

	stats := learner.UpdateFromBatch()

	if stats.TotalEvents != 7 {
		t.Errorf("expected 7 total events, got %d", stats.TotalEvents)
	}
	if stats.MisclassifiedCount != 2 {
		t.Errorf("expected 2 misclassified, got %d", stats.MisclassifiedCount)
	}

	expectedRate := 2.0 / 7.0
	if diff := stats.MisclassificationRate - expectedRate; diff < -0.01 || diff > 0.01 {
		t.Errorf("expected misclass rate ~%.2f, got %f", expectedRate, stats.MisclassificationRate)
	}
}

func TestGetMisclassifications(t *testing.T) {
	learner := NewLearner(100, 1*time.Hour)

	// Record some events
	learner.RecordOutcome(LearningEvent{
		ID:            "correct",
		PredictedTask: TaskConcurrency,
		ActualTask:    TaskConcurrency,
	})

	learner.RecordOutcome(LearningEvent{
		ID:            "wrong",
		PredictedTask: TaskConcurrency,
		ActualTask:    TaskParsing,
	})

	misclass := learner.GetMisclassifications()

	if len(misclass) != 1 {
		t.Errorf("expected 1 misclassified, got %d", len(misclass))
	}
	if misclass[0].ID != "wrong" {
		t.Errorf("expected wrong event, got %s", misclass[0].ID)
	}
}

func TestGetRecentEvents(t *testing.T) {
	learner := NewLearner(100, 1*time.Hour)

	for i := 0; i < 10; i++ {
		learner.RecordOutcome(LearningEvent{
			ID: "test-" + string(rune(i)),
		})
	}

	recent := learner.GetRecentEvents(5)

	if len(recent) != 5 {
		t.Errorf("expected 5 recent events, got %d", len(recent))
	}

	// Should be the last 5 (indices 5-9)
	for i, event := range recent {
		expectedID := "test-" + string(rune(5+i))
		if event.ID != expectedID {
			t.Errorf("recent event %d: expected %s, got %s", i, expectedID, event.ID)
		}
	}
}

func TestGetAccuracyStats(t *testing.T) {
	learner := NewLearner(100, 1*time.Hour)

	for i := 0; i < 10; i++ {
		learner.RecordOutcome(LearningEvent{
			ID:            "test-" + string(rune(i)),
			ActualTask:    TaskConcurrency,
			Succeeded:     i < 7, // 7 successes, 3 failures
			TokenError:    0.05,
		})
	}

	stats := learner.GetAccuracyStats()

	if totalTests, ok := stats["total_tests"].(int); !ok || totalTests != 10 {
		t.Errorf("expected 10 total tests")
	}

	if totalSuccesses, ok := stats["total_successes"].(int); !ok || totalSuccesses != 7 {
		t.Errorf("expected 7 successes")
	}

	if overallRate, ok := stats["overall_rate"].(float64); !ok || (overallRate < 0.69 || overallRate > 0.71) {
		t.Errorf("expected overall rate ~0.7, got %v", stats["overall_rate"])
	}
}

func TestSetUpdateCallback(t *testing.T) {
	learner := NewLearner(100, 1*time.Hour)

	callCount := 0
	learner.SetUpdateCallback(func(stats LearningStats) {
		callCount++
	})

	learner.UpdateFromBatch()

	if callCount != 1 {
		t.Errorf("expected callback to be called once, got %d", callCount)
	}
}

func TestShouldUpdate(t *testing.T) {
	learner := NewLearner(100, 1*time.Millisecond) // Very short interval

	if !learner.ShouldUpdate() {
		t.Error("should update immediately after creation")
	}

	// After update
	learner.UpdateFromBatch()

	// Should not need update immediately
	if learner.ShouldUpdate() {
		t.Error("should not update immediately after last update")
	}

	// Wait for interval
	time.Sleep(2 * time.Millisecond)

	if !learner.ShouldUpdate() {
		t.Error("should need update after interval elapsed")
	}
}
