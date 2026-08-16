package forecast

import (
	"testing"
	"time"
)

func TestRecommendationUsesP90SafetyAndConfirmedInboundOnce(t *testing.T) {
	got, err := CalculateRecommendation(RecommendationInput{
		P90Micros: []int64{11, 12, 13}, HorizonDays: 3, SafetyDays: 1,
		UsableStockMicros: 10, ConfirmedInboundMicros: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	// ceil(36*1/3)=12; 36+12-10-5=33.
	if got.P90DemandMicros != 36 || got.SafetyDemandMicros != 12 || got.RequestedQuantityMicros != 33 {
		t.Fatalf("unexpected exact components: %+v", got)
	}
	if got.ConfirmedInboundMicros != 5 || got.ProjectedAvailableMicros != 15 {
		t.Fatalf("calculation must count inbound exactly once: %+v", got)
	}
	if len(got.ReasonCodes) == 0 || got.ReasonCodes[3] != "confirmed_inbound_once" {
		t.Fatalf("missing inbound explanation: %+v", got.ReasonCodes)
	}
}

func TestRecommendationPackAndMOQRoundUpIsFixedPoint(t *testing.T) {
	got, err := CalculateRecommendation(RecommendationInput{
		P90Micros: []int64{10}, HorizonDays: 1, SafetyDays: 0,
		UsableStockMicros: 0, PackSizeMicros: 6, MOQ: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RequestedQuantityMicros != 10 || got.RoundedQuantityMicros != 18 || got.PackMOQDeltaMicros != 8 || got.OrderedPacks != 3 {
		t.Fatalf("unexpected rounding: %+v", got)
	}
}

func TestRecommendationExplanationIncludesNoSupplierSelection(t *testing.T) {
	got, err := CalculateRecommendation(RecommendationInput{P90Micros: []int64{1}, HorizonDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Formula == "" || got.ReasonCodes[4] != "supplier_terms_not_selected" {
		t.Fatalf("calculation is not auditable: %+v", got)
	}
}

func TestLotUsabilityExcludesQuarantineAndHorizonExpiry(t *testing.T) {
	horizon := time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC)
	expiresAt := horizon
	if usable, reason := IsUsableAvailableLot("quarantined", nil, horizon); usable || reason != "quarantine_excluded" {
		t.Fatalf("quarantine was usable: %v %s", usable, reason)
	}
	if usable, reason := IsUsableAvailableLot("available", &expiresAt, horizon); usable || reason != "expiry_excluded_over_horizon" {
		t.Fatalf("boundary expiry was usable: %v %s", usable, reason)
	}
	if usable, reason := IsUsableAvailableLot("available", nil, horizon); !usable || reason != "" {
		t.Fatalf("usable lot rejected: %v %s", usable, reason)
	}
}
