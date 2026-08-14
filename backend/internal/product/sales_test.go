package product

import (
	"errors"
	"testing"

	"lastsaas/internal/inventory"
)

func TestFixedRecipeQuantityUsesFixedPointWithoutFloatRounding(t *testing.T) {
	got, err := fixedRecipeQuantity(2*inventory.QuantityScale, 500000, 2, inventory.QuantityScale, 0)
	if err != nil {
		t.Fatalf("fixed recipe quantity: %v", err)
	}
	if got != 500000 {
		t.Fatalf("got %d micros, want 500000", got)
	}
}

func TestFixedRecipeQuantityRejectsInvalidFactors(t *testing.T) {
	if _, err := fixedRecipeQuantity(inventory.QuantityScale, inventory.QuantityScale, 0, inventory.QuantityScale, 0); !errors.Is(err, ErrSaleMappingInvalid) {
		t.Fatalf("want invalid mapping error, got %v", err)
	}
}

func TestSalesReasonDistinguishesUnmappedProductsAndRecipeCycles(t *testing.T) {
	if got := salesReason(ErrSaleMappingMissing); got != "unmapped product" {
		t.Fatalf("unexpected unmapped reason %q", got)
	}
	if got := salesReason(ErrRecipeCycle); got != "recipe cycle detected" {
		t.Fatalf("unexpected cycle reason %q", got)
	}
}
