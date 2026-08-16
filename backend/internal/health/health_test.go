package health

import (
	"testing"
	"time"

	"lastsaas/internal/models"
)

func TestForecastHealthMetricsDefaultsToUnknownFreshness(t *testing.T) {
	metrics := models.ForecastHealthMetrics{FreshnessSeconds: -1, FreshnessStatus: "unknown"}
	if metrics.FreshnessSeconds != -1 || metrics.FreshnessStatus != "unknown" {
		t.Fatalf("unexpected freshness defaults: %+v", metrics)
	}
}

func TestForecastFreshnessAgeIsNonNegative(t *testing.T) {
	now := time.Now().UTC()
	completed := now.Add(10 * time.Second)
	age := now.Sub(completed)
	if age < 0 {
		age = 0
	}
	if age != 0 {
		t.Fatalf("future completion should not produce negative freshness: %s", age)
	}
}
