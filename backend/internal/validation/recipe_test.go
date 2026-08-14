package validation

import (
	"testing"
	"time"

	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func validRecipeVersionFixture() models.RecipeVersion {
	now := time.Now().UTC()
	return models.RecipeVersion{
		ID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(), RecipeID: primitive.NewObjectID(),
		Number: 1, OutputUnitID: primitive.NewObjectID(), PortionCount: 2,
		YieldFactorMicros: 1000000, LossFactorMicros: 50000, EffectiveFrom: now,
		Status: models.RecipeVersionDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func TestRecipeDomainValidation(t *testing.T) {
	now := time.Now().UTC()
	recipe := models.Recipe{ID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(), Code: "soup", Name: "Soup", IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := Validate(recipe); err != nil {
		t.Fatalf("valid recipe rejected: %v", err)
	}

	version := validRecipeVersionFixture()
	if err := Validate(version); err != nil {
		t.Fatalf("valid recipe version rejected: %v", err)
	}
	end := version.EffectiveFrom.Add(time.Hour)
	version.EffectiveTo = &end
	if err := Validate(version); err != nil {
		t.Fatalf("valid bounded recipe version rejected: %v", err)
	}
	version.EffectiveTo = &version.EffectiveFrom
	if err := Validate(version); err == nil {
		t.Fatal("expected effective range validation error")
	}

	item := primitive.NewObjectID()
	component := models.RecipeComponent{ID: primitive.NewObjectID(), TenantID: version.TenantID, RecipeVersionID: version.ID, ComponentType: models.RecipeComponentItem, ItemID: &item, QuantityMicros: 1000, UnitID: version.OutputUnitID, CreatedAt: now, UpdatedAt: now}
	if err := Validate(component); err != nil {
		t.Fatalf("valid item component rejected: %v", err)
	}
	component.SubrecipeID = &item
	if err := Validate(component); err == nil {
		t.Fatal("expected component reference exclusivity error")
	}
}

func TestRecipeDomainValidationRejectsInvalidIDsAndFactors(t *testing.T) {
	version := validRecipeVersionFixture()
	version.TenantID = primitive.NilObjectID
	if err := Validate(version); err == nil {
		t.Fatal("expected tenant isolation field validation error")
	}
	version = validRecipeVersionFixture()
	version.YieldFactorMicros = 1000001
	if err := Validate(version); err == nil {
		t.Fatal("expected yield factor upper bound error")
	}
	version = validRecipeVersionFixture()
	version.OutputUnitID = primitive.NilObjectID
	if err := Validate(version); err == nil {
		t.Fatal("expected output unit ID validation error")
	}
}

func TestExternalProductMappingValidation(t *testing.T) {
	now := time.Now().UTC()
	mapping := models.ExternalProductMapping{ID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(), Adapter: "pos-a", ExternalProductID: "burger-1", RecipeVersionID: primitive.NewObjectID(), EffectiveFrom: now, Version: 1, CreatedAt: now, UpdatedAt: now, IsActive: true}
	if err := Validate(mapping); err != nil {
		t.Fatalf("valid mapping rejected: %v", err)
	}
	end := now.Add(-time.Minute)
	mapping.EffectiveTo = &end
	if err := Validate(mapping); err == nil {
		t.Fatal("expected mapping effective range validation error")
	}
}

func TestSalesValidationRequiresCancellationTimestamp(t *testing.T) {
	now := time.Now().UTC()
	sale := models.Sale{ID: primitive.NewObjectID(), TenantID: primitive.NewObjectID(), Source: "pos", ExternalSaleID: "sale-1", OccurredAt: now, LocationID: primitive.NewObjectID(), StorageAreaID: primitive.NewObjectID(), Status: models.SaleStatusCancelled, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := Validate(sale); err == nil {
		t.Fatal("expected cancelled sale without timestamp to be rejected")
	}
	sale.CancelledAt = &now
	if err := Validate(sale); err != nil {
		t.Fatalf("valid cancelled sale rejected: %v", err)
	}
}
