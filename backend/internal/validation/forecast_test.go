package validation

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"lastsaas/internal/models"
)

func TestForecastSnapshotsAndManualGuestPlans(t *testing.T) {
	id := primitive.NewObjectID()
	now := time.Now().UTC()
	dataset := models.ForecastDataset{ID: id, TenantID: id, LocationID: id, Name: "baseline", Source: "manual", Status: models.ForecastDatasetDraft, EffectiveFrom: now, EffectiveTo: now.Add(time.Hour), CreatedBy: id, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := Validate(&dataset); err != nil {
		t.Fatalf("valid draft dataset failed: %v", err)
	}
	dataset.Status = models.ForecastDatasetSealed
	if err := Validate(&dataset); err == nil {
		t.Fatal("sealed dataset without hash and seal metadata must fail")
	}

	plan := models.GuestPlan{ID: id, TenantID: id, LocationID: id, PlanDate: now, ServicePeriod: "dinner", GuestCount: 40, Source: "manual", IsActual: false, CreatedBy: id, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := Validate(&plan); err != nil {
		t.Fatalf("valid manual guest plan failed: %v", err)
	}
	plan.IsActual = true
	if err := Validate(&plan); err == nil {
		t.Fatal("guest plan must never validate as an actual")
	}
}

func TestShadowKPIReportValidation(t *testing.T) {
	id := primitive.NewObjectID()
	now := time.Now().UTC()
	report := models.ShadowKPIReport{ID: id, TenantID: id, LocationID: id, RunID: id, EvaluationStart: now.Add(-24 * time.Hour), EvaluationEnd: now, Metrics: map[string]float64{"wape": 0.12}, CreatedBy: id, CreatedAt: now}
	if err := Validate(&report); err != nil {
		t.Fatalf("valid shadow KPI report failed: %v", err)
	}
	report.Metrics = nil
	if err := Validate(&report); err == nil {
		t.Fatal("shadow KPI report without metrics must fail")
	}
}
