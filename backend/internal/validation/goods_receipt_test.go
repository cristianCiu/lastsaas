package validation

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"lastsaas/internal/models"
)

func validGoodsReceiptLineForTest() models.GoodsReceiptLine {
	now := time.Now().UTC()
	id := primitive.NewObjectID()
	return models.GoodsReceiptLine{ID: id, TenantID: id, LocationID: id, GoodsReceiptID: id, PurchaseOrderID: id, PurchaseOrderLineID: id, LineNumber: 1, ItemID: id, ItemSKU: "rice", ItemName: "Rice", Description: "Rice", SupplierCode: "supplier", SupplierName: "Supplier", SupplierItemCode: "rice-1", SupplierItemVersion: 1, ItemBaseUnitCode: "kg", OrderedUnitCode: "kg", OrderedQuantityMicros: 10_000_000, PreviouslyReceivedMicros: 2_000_000, ReceivedQuantityMicros: 3_000_000, QuantityVarianceMicros: -7_000_000, OrderedUnitPriceMinor: 100, ActualUnitPriceMinor: 105, PriceVarianceMinor: 5, Currency: "USD", VarianceReason: "supplier price changed", StorageAreaID: id, CreatedAt: now, UpdatedAt: now}
}

func TestGoodsReceiptLineRequiresExactSignedVariances(t *testing.T) {
	line := validGoodsReceiptLineForTest()
	if err := Validate(&line); err != nil {
		t.Fatalf("valid receipt line rejected: %v", err)
	}
	line.QuantityVarianceMicros = 0
	if err := Validate(&line); err == nil {
		t.Fatal("expected quantity variance mismatch")
	}
	line = validGoodsReceiptLineForTest()
	line.PriceVarianceMinor = -5
	if err := Validate(&line); err == nil {
		t.Fatal("expected price variance mismatch")
	}
}
