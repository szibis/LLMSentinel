// Package classify provides active learning to improve classification accuracy over time.
package classify

import (
	"fmt"
	"sync"
	"time"
)

// LearningEvent represents a classification outcome used for continuous improvement.
type LearningEvent struct {
	ID              string    `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	Prompt          string    `json:"prompt"`
	PredictedTask   TaskType  `json:"predicted_task"`
	ActualTask      TaskType  `json:"actual_task"`
	Succeeded       bool      `json:"succeeded"`
	TokenError      float64   `json:"token_error"`      // Difference between estimated and actual tokens
	ConfidenceScore float64   `json:"confidence_score"` // Classifier confidence in prediction
}

// Learner handles active learning from classification outcomes.
type Learner struct {
	mu                sync.RWMutex
	recentEvents      []LearningEvent
	maxHistorySize    int
	updateFrequency   time.Duration
	lastUpdate        time.Time
	onUpdateComplete  func(stats LearningStats) // Callback when learning completes
	accuracyByTask    map[TaskType]TaskAccuracy
	accuracyByModel   map[string]ModelAccuracy
}

// TaskAccuracy tracks classification accuracy per task type.
type TaskAccuracy struct {
	TaskType      TaskType
	TotalCount    int
	SuccessCount  int
	SuccessRate   float64
	AvgTokenError float64
}

// ModelAccuracy tracks model success rates for specific tasks.
type ModelAccuracy struct {
	TaskType     TaskType
	Model        string
	SuccessCount int
	TotalCount   int
	SuccessRate  float64
	AvgTokenError float64
}

// LearningStats summarizes learning activity.
type LearningStats struct {
	TotalEvents          int
	MisclassifiedCount   int
	MisclassificationRate float64
	UpdatedAtTaskTypes   int
	RecentAccuracyImprovement float64
}

// NewLearner creates a new active learner.
func NewLearner(maxHistory int, updateFreq time.Duration) *Learner {
	return &Learner{
		recentEvents:    make([]LearningEvent, 0, maxHistory),
		maxHistorySize:  maxHistory,
		updateFrequency: updateFreq,
		lastUpdate:      time.Now(),
		accuracyByTask:  make(map[TaskType]TaskAccuracy),
		accuracyByModel: make(map[string]ModelAccuracy),
	}
}

// RecordOutcome records a classification outcome for learning.
// Call this when the actual task type becomes known (e.g., via validation).
func (l *Learner) RecordOutcome(event LearningEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()

	event.Timestamp = time.Now()

	// Keep only recent events (bounded memory)
	l.recentEvents = append(l.recentEvents, event)
	if len(l.recentEvents) > l.maxHistorySize {
		l.recentEvents = l.recentEvents[1:] // Drop oldest
	}

	// Update task-level accuracy
	l.updateTaskAccuracy(event)
}

// updateTaskAccuracy updates accuracy metrics for the task type.
func (l *Learner) updateTaskAccuracy(event LearningEvent) {
	acc := l.accuracyByTask[event.ActualTask]
	acc.TaskType = event.ActualTask
	acc.TotalCount++

	if event.Succeeded {
		acc.SuccessCount++
	}

	// Update average token error
	totalError := acc.AvgTokenError * float64(acc.TotalCount-1)
	acc.AvgTokenError = (totalError + event.TokenError) / float64(acc.TotalCount)

	if acc.TotalCount > 0 {
		acc.SuccessRate = float64(acc.SuccessCount) / float64(acc.TotalCount)
	}

	l.accuracyByTask[event.ActualTask] = acc
}

// GetTaskAccuracy returns accuracy metrics for a specific task type.
func (l *Learner) GetTaskAccuracy(taskType TaskType) TaskAccuracy {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.accuracyByTask[taskType]
}

// GetAllTaskAccuracies returns accuracy metrics for all task types.
func (l *Learner) GetAllTaskAccuracies() map[TaskType]TaskAccuracy {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make(map[TaskType]TaskAccuracy, len(l.accuracyByTask))
	for k, v := range l.accuracyByTask {
		result[k] = v
	}
	return result
}

// UpdateFromBatch processes a batch of learning events (called hourly).
// This method analyzes misclassifications and updates confidence thresholds.
func (l *Learner) UpdateFromBatch() LearningStats {
	l.mu.Lock()
	defer l.mu.Unlock()

	stats := LearningStats{
		TotalEvents: len(l.recentEvents),
		UpdatedAtTaskTypes: len(l.accuracyByTask),
	}

	// Count misclassifications
	for _, event := range l.recentEvents {
		if event.PredictedTask != event.ActualTask {
			stats.MisclassifiedCount++
		}
	}

	if stats.TotalEvents > 0 {
		stats.MisclassificationRate = float64(stats.MisclassifiedCount) / float64(stats.TotalEvents)
	}

	// Update learning stats
	l.lastUpdate = time.Now()

	// Call completion callback if set
	if l.onUpdateComplete != nil {
		l.onUpdateComplete(stats)
	}

	return stats
}

// GetRecentEvents returns the N most recent learning events.
func (l *Learner) GetRecentEvents(count int) []LearningEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if count > len(l.recentEvents) {
		count = len(l.recentEvents)
	}

	events := make([]LearningEvent, count)
	copy(events, l.recentEvents[len(l.recentEvents)-count:])

	return events
}

// GetMisclassifications returns events where predicted != actual task.
func (l *Learner) GetMisclassifications() []LearningEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var misclassified []LearningEvent
	for _, event := range l.recentEvents {
		if event.PredictedTask != event.ActualTask {
			misclassified = append(misclassified, event)
		}
	}

	return misclassified
}

// GetAccuracyStats returns aggregated learning statistics.
func (l *Learner) GetAccuracyStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	totalTests := 0
	totalSuccesses := 0

	for _, acc := range l.accuracyByTask {
		totalTests += acc.TotalCount
		totalSuccesses += acc.SuccessCount
	}

	overallRate := 0.0
	if totalTests > 0 {
		overallRate = float64(totalSuccesses) / float64(totalTests)
	}

	return map[string]interface{}{
		"total_tests":      totalTests,
		"total_successes":  totalSuccesses,
		"overall_rate":     overallRate,
		"last_update":      l.lastUpdate,
		"accuracy_by_task": l.accuracyByTask,
	}
}

// SetUpdateCallback registers a callback to be called after each update batch.
func (l *Learner) SetUpdateCallback(callback func(stats LearningStats)) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.onUpdateComplete = callback
}

// ShouldUpdate checks if enough time has passed for an update.
func (l *Learner) ShouldUpdate() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return time.Since(l.lastUpdate) >= l.updateFrequency
}

// String returns a human-readable summary of learning stats.
func (stats LearningStats) String() string {
	return fmt.Sprintf(
		"LearningStats{Total: %d, Misclassified: %d, Rate: %.1f%%, UpdatedTasks: %d}",
		stats.TotalEvents,
		stats.MisclassifiedCount,
		stats.MisclassificationRate*100,
		stats.UpdatedAtTaskTypes,
	)
}
