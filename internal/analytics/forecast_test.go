package analytics

import (
	"testing"
	"time"
)

func TestNewForecastModel(t *testing.T) {
	fm := NewForecastModel("cost")

	if fm.Metric != "cost" {
		t.Errorf("expected metric 'cost', got '%s'", fm.Metric)
	}
	if fm.PredictionCI != 1.96 {
		t.Errorf("expected default CI 1.96, got %f", fm.PredictionCI)
	}
}

func TestTrainForecastModel(t *testing.T) {
	fm := NewForecastModel("daily_cost")

	// Generate sample data: linear trend 10 -> 110 over 10 days
	data := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	now := time.Now()
	timestamps := make([]time.Time, len(data))
	for i := range timestamps {
		timestamps[i] = now.AddDate(0, 0, i)
	}

	err := fm.Train(data, timestamps)
	if err != nil {
		t.Fatalf("training failed: %v", err)
	}

	// Check slope should be ~10 (roughly 10 units per day)
	if fm.Slope < 9 || fm.Slope > 11 {
		t.Errorf("expected slope ~10, got %f", fm.Slope)
	}

	// Check intercept (should be ~0 since first point is 10 at day 0)
	if fm.Intercept < -5 || fm.Intercept > 15 {
		t.Errorf("expected intercept near 10, got %f", fm.Intercept)
	}

	// Check R-squared (should be very high for linear data)
	if fm.RSquared < 0.99 {
		t.Errorf("expected R² > 0.99, got %f", fm.RSquared)
	}
}

func TestPredictForecast(t *testing.T) {
	fm := NewForecastModel("cost")

	// Train on sample data
	data := []float64{10, 20, 30, 40, 50}
	now := time.Now()
	timestamps := make([]time.Time, len(data))
	for i := range timestamps {
		timestamps[i] = now.AddDate(0, 0, i)
	}

	_ = fm.Train(data, timestamps)

	// Predict next 7 days
	forecasts, err := fm.Predict(7)
	if err != nil {
		t.Fatalf("prediction failed: %v", err)
	}

	if len(forecasts) != 7 {
		t.Errorf("expected 7 forecasts, got %d", len(forecasts))
	}

	// Verify forecasts are valid (basic sanity check)
	for i, f := range forecasts {
		if f.Point == 0 {
			t.Errorf("forecast %d: point estimate should be non-zero", i)
		}
		// Bounds should be reasonable (not checking exact CI since implementation may vary)
		if f.UpperBound < f.LowerBound {
			t.Errorf("forecast %d: upper bound should be >= lower bound", i)
		}
	}
}

func TestDetectTrendChange(t *testing.T) {
	fm := NewForecastModel("trend_test")

	// Flat data (no trend)
	flatData := []float64{50, 50, 50, 50, 50, 50}
	timestamps := make([]time.Time, len(flatData))
	now := time.Now()
	for i := range timestamps {
		timestamps[i] = now.AddDate(0, 0, i)
	}

	_ = fm.Train(flatData, timestamps)

	// Should not detect change (high threshold)
	if fm.DetectTrendChange(0.5) {
		t.Error("flat data should not trigger trend change at 50% threshold")
	}

	// Data with strong increase
	increaseData := []float64{10, 10, 10, 100, 100, 100}
	timestamps2 := make([]time.Time, len(increaseData))
	for i := range timestamps2 {
		timestamps2[i] = now.AddDate(0, 0, i)
	}

	_ = fm.Train(increaseData, timestamps2)

	// Should detect change (low threshold)
	if !fm.DetectTrendChange(0.2) {
		t.Error("should detect trend change with 20% threshold")
	}
}

func TestPredictBudgetExceeded(t *testing.T) {
	fm := NewForecastModel("cost")

	// Train with increasing costs
	data := []float64{10, 20, 30, 40, 50}
	now := time.Now()
	timestamps := make([]time.Time, len(data))
	for i := range timestamps {
		timestamps[i] = now.AddDate(0, 0, i)
	}

	_ = fm.Train(data, timestamps)

	// Predict when $200 budget will be exceeded
	forecast := fm.PredictBudgetExceeded(100, 200)

	if forecast == nil {
		t.Fatal("forecast should not be nil")
	}

	if forecast.DaysRemaining < 1 || forecast.DaysRemaining > 50 {
		t.Errorf("expected reasonable days remaining, got %d", forecast.DaysRemaining)
	}

	if forecast.ExceededAt == nil {
		t.Error("exceeded_at should not be nil")
	}

	if forecast.DailyRate <= 0 {
		t.Error("daily rate should be positive")
	}
}

func TestTrainWithInsufficientData(t *testing.T) {
	fm := NewForecastModel("test")

	// Only 1 data point
	data := []float64{100}
	timestamps := []time.Time{time.Now()}

	err := fm.Train(data, timestamps)
	if err == nil {
		t.Error("expected error with insufficient data")
	}
}

func TestTrainWithMismatchedLengths(t *testing.T) {
	fm := NewForecastModel("test")

	data := []float64{1, 2, 3}
	timestamps := []time.Time{time.Now(), time.Now().AddDate(0, 0, 1)}

	err := fm.Train(data, timestamps)
	if err == nil {
		t.Error("expected error with mismatched lengths")
	}
}

func TestGetQuality(t *testing.T) {
	fm := NewForecastModel("quality_test")

	data := []float64{10, 20, 30, 40, 50}
	timestamps := make([]time.Time, len(data))
	now := time.Now()
	for i := range timestamps {
		timestamps[i] = now.AddDate(0, 0, i)
	}

	_ = fm.Train(data, timestamps)

	quality := fm.GetQuality()

	if quality["metric"] != "quality_test" {
		t.Error("metric should be in quality map")
	}

	if rmse, ok := quality["rmse"].(float64); !ok || rmse < 0 {
		t.Error("RMSE should be non-negative float")
	}

	if r2, ok := quality["r_squared"].(float64); !ok || r2 < 0 || r2 > 1 {
		t.Error("R² should be between 0 and 1")
	}

	if slope, ok := quality["slope"].(float64); !ok || slope <= 0 {
		t.Error("slope should be positive float for increasing data")
	}
}

func TestMeanCalculation(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	m := mean(data)

	expected := 3.0
	if diff := m - expected; diff < -0.001 || diff > 0.001 {
		t.Errorf("expected mean 3.0, got %f", m)
	}

	// Empty slice
	emptyMean := mean([]float64{})
	if emptyMean != 0 {
		t.Errorf("mean of empty slice should be 0, got %f", emptyMean)
	}

	// Single value
	singleMean := mean([]float64{42})
	if singleMean != 42 {
		t.Errorf("mean of [42] should be 42, got %f", singleMean)
	}
}

func TestPredictNegativeValues(t *testing.T) {
	fm := NewForecastModel("negative_cost")

	// Data that would naturally trend negative (shouldn't happen for costs, but test the guard)
	data := []float64{100, 90, 80, 70, 60}
	now := time.Now()
	timestamps := make([]time.Time, len(data))
	for i := range timestamps {
		timestamps[i] = now.AddDate(0, 0, i)
	}

	_ = fm.Train(data, timestamps)

	forecasts, _ := fm.Predict(5)

	// Lower bounds should not go below 0 (cost protection)
	for i, f := range forecasts {
		if f.LowerBound < 0 {
			t.Errorf("forecast %d: lower bound should not be negative", i)
		}
	}
}
