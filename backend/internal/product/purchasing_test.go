package product

import (
	"errors"
	"math"
	"testing"

	"lastsaas/internal/models"
)

func TestRoundPurchaseQuantityUsesExactPackAndMOQArithmetic(t *testing.T) {
	rounded, err := RoundPurchaseQuantity(2500001, 1000000, 3, 125)
	if err != nil {
		t.Fatalf("rounding failed: %v", err)
	}
	if rounded.OrderedPacks != 3 || rounded.OrderedQuantityMicros != 3000000 || rounded.RoundingDeltaMicros != 499999 || rounded.LineTotalMinor != 375 {
		t.Fatalf("unexpected rounding result: %+v", rounded)
	}
}

func TestRoundPurchaseQuantityRejectsOverflow(t *testing.T) {
	if _, err := RoundPurchaseQuantity(math.MaxInt64, 2, 1, 1); !errors.Is(err, ErrPurchaseRoundingOverflow) {
		t.Fatalf("expected overflow, got %v", err)
	}
}

func TestPurchaseOrderStatusesIncludeFutureReceiptStates(t *testing.T) {
	for _, status := range []models.PurchaseOrderStatus{models.PurchaseOrderDraft, models.PurchaseOrderSubmitted, models.PurchaseOrderApproved, models.PurchaseOrderOrdered, models.PurchaseOrderPartiallyReceived, models.PurchaseOrderReceived, models.PurchaseOrderCancelled} {
		if !models.ValidPurchaseOrderStatus(status) {
			t.Fatalf("status %q was not accepted", status)
		}
	}
}
