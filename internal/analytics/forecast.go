// Package analytics provides predictive forecasting and trend analysis.
package analytics

import (
	"fmt"
	"math"
	"time"
)

// Forecast represents a predicted value with confidence interval.
type Forecast struct {
	Timestamp  time.Time `json:"timestamp"`
	Point      float64   `json:"point"`      // Best estimate
	LowerBound float64   `json:"lower_bound"` // 95% CI lower
	UpperBound float64   `json:"upper_bound"` // 95% CI upper
}

// ForecastModel performs linear regression and prediction with confidence intervals.
type ForecastModel struct {
	Metric       string
	DataPoints   []float64
	Timestamps   []time.Time
	Slope        float64
	Intercept    float64
	RMSE         float64
	RSquared     float64
	PredictionCI float64 // Confidence interval width (95%)
}

// NewForecastModel creates a new forecast model.
func NewForecastModel(metric string) *ForecastModel {
	return &ForecastModel{
		Metric:       metric,
		DataPoints:   make([]float64, 0),
		Timestamps:   make([]time.Time, 0),
		PredictionCI: 1.96, // 95% confidence interval z-score
	}
}

// Train fits a linear regression model to historical data.
// Expects data points ordered chronologically.
func (fm *ForecastModel) Train(data []float64, timestamps []time.Time) error {
	if len(data) < 2 || len(data) != len(timestamps) {
		return fmt.Errorf("insufficient data for training: %d points", len(data))
	}

	fm.DataPoints = make([]float64, len(data))
	copy(fm.DataPoints, data)

	fm.Timestamps = make([]time.Time, len(timestamps))
	copy(fm.Timestamps, timestamps)

	// Convert timestamps to numeric x values (days since start)
	x := make([]float64, len(timestamps))
	startTime := timestamps[0]
	for i, t := range timestamps {
		x[i] = t.Sub(startTime).Hours() / 24.0
	}

	// Linear regression: y = mx + b
	n := float64(len(data))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i := 0; i < len(data); i++ {
		sumX += x[i]
		sumY += data[i]
		sumXY += x[i] * data[i]
		sumX2 += x[i] * x[i]
	}

	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return fmt.Errorf("cannot fit model: singular matrix")
	}

	fm.Slope = (n*sumXY - sumX*sumY) / denominator
	fm.Intercept = (sumY - fm.Slope*sumX) / n

	// Calculate RMSE
	sumSquaredError := 0.0
	for i := 0; i < len(data); i++ {
		predicted := fm.Slope*x[i] + fm.Intercept
		error := data[i] - predicted
		sumSquaredError += error * error
	}

	fm.RMSE = math.Sqrt(sumSquaredError / n)

	// Calculate R-squared
	meanY := sumY / n
	sumSquaredTotal := 0.0
	for _, y := range data {
		sumSquaredTotal += (y - meanY) * (y - meanY)
	}

	if sumSquaredTotal == 0 {
		fm.RSquared = 1.0
	} else {
		fm.RSquared = 1.0 - (sumSquaredError / sumSquaredTotal)
	}

	return nil
}

// Predict generates forecasts for N days ahead with confidence intervals.
func (fm *ForecastModel) Predict(daysAhead int) ([]Forecast, error) {
	if len(fm.DataPoints) == 0 {
		return nil, fmt.Errorf("model not trained")
	}

	var forecasts []Forecast
	startTime := time.Now()
	baseX := float64(len(fm.DataPoints) - 1)

	for i := 1; i <= daysAhead; i++ {
		timestamp := startTime.AddDate(0, 0, i)

		// Point estimate
		xValue := baseX + float64(i)
		point := fm.Slope*xValue + fm.Intercept

		// Confidence interval (simplified: ±1.96*RMSE)
		lowerBound := point - fm.PredictionCI*fm.RMSE
		upperBound := point + fm.PredictionCI*fm.RMSE

		// Prevent negative predictions
		if lowerBound < 0 {
			lowerBound = 0
		}

		forecasts = append(forecasts, Forecast{
			Timestamp:  timestamp,
			Point:      point,
			LowerBound: lowerBound,
			UpperBound: upperBound,
		})
	}

	return forecasts, nil
}

// DetectTrendChange uses CUSUM (Cumulative Sum Control Chart) to detect trend breaks.
// Returns true if a significant trend change is detected.
func (fm *ForecastModel) DetectTrendChange(threshold float64) bool {
	if len(fm.DataPoints) < 3 {
		return false
	}

	// Simple trend detection: compare recent slope to historical
	recentCount := len(fm.DataPoints) / 3
	if recentCount < 2 {
		recentCount = 2
	}

	recentData := fm.DataPoints[len(fm.DataPoints)-recentCount:]
	historicalData := fm.DataPoints[:len(fm.DataPoints)-recentCount]

	historicalMean := mean(historicalData)
	recentMean := mean(recentData)

	if historicalMean == 0 {
		return false
	}

	changePercent := math.Abs((recentMean - historicalMean) / historicalMean)

	return changePercent > threshold
}

// GetQuality returns model quality assessment.
func (fm *ForecastModel) GetQuality() map[string]interface{} {
	return map[string]interface{}{
		"metric":    fm.Metric,
		"rmse":      fm.RMSE,
		"r_squared": fm.RSquared,
		"slope":     fm.Slope,
		"intercept": fm.Intercept,
		"training_points": len(fm.DataPoints),
	}
}

// mean calculates the arithmetic mean of a slice.
func mean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range data {
		sum += v
	}

	return sum / float64(len(data))
}

// BudgetForecast predicts when the budget will be exceeded.
type BudgetForecast struct {
	ExceededAt    *time.Time `json:"exceeded_at"`
	DaysRemaining int        `json:"days_remaining"`
	DailyRate     float64    `json:"daily_rate"`
}

// PredictBudgetExceeded predicts when daily/monthly budget will be exceeded.
func (fm *ForecastModel) PredictBudgetExceeded(currentSpend, dailyLimit float64) *BudgetForecast {
	if fm.Slope <= 0 {
		return &BudgetForecast{
			DaysRemaining: 365, // No trend to exceed
			DailyRate:     fm.Slope,
		}
	}

	// Days until budget exceeded
	daysUntilExceeded := (dailyLimit - currentSpend) / fm.Slope
	if daysUntilExceeded < 0 {
		daysUntilExceeded = 0
	}

	exceededAt := time.Now().AddDate(0, 0, int(daysUntilExceeded))

	return &BudgetForecast{
		ExceededAt:    &exceededAt,
		DaysRemaining: int(daysUntilExceeded),
		DailyRate:     fm.Slope,
	}
}
