package db_test

import (
	"context"
	"testing"
	"time"

	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestIntegrationLocationsSchemaRejectsInvalidDocuments(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()

	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}

	_, err := database.Locations().InsertOne(context.Background(), bson.M{
		"_id":        primitive.NewObjectID(),
		"tenantId":   primitive.NewObjectID(),
		"code":       "INVALID CODE",
		"name":       "Invalid location",
		"timezone":   "Europe/Berlin",
		"isActive":   true,
		"version":    int64(1),
		"createdAt":  time.Now(),
		"updatedAt":  time.Now(),
		"limitSlot":  int64(1),
		"unexpected": true,
	})
	if err == nil {
		t.Fatal("expected MongoDB schema validation to reject malformed location")
	}

	_, err = database.Locations().InsertOne(context.Background(), bson.M{
		"_id":       primitive.NewObjectID(),
		"tenantId":  primitive.NewObjectID(),
		"code":      "missing-version",
		"name":      "Missing version",
		"timezone":  "UTC",
		"isActive":  true,
		"createdAt": time.Now(),
		"updatedAt": time.Now(),
		"limitSlot": int64(1),
	})
	if err == nil {
		t.Fatal("expected MongoDB schema validation to require location version")
	}
}

func TestIntegrationRestaurantSchemasRejectMalformedDocuments(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now()
	_, err := database.RestaurantSettings().InsertOne(context.Background(), bson.M{"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "currency": "euro", "language": "de", "defaultTimezone": "UTC", "version": int64(1), "createdAt": now, "updatedAt": now})
	if err == nil {
		t.Fatal("expected invalid currency to be rejected")
	}
	_, err = database.StorageAreas().InsertOne(context.Background(), bson.M{"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "locationId": primitive.NewObjectID(), "name": "Store", "type": "warehouse", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now})
	if err == nil {
		t.Fatal("expected invalid storage area type to be rejected")
	}
	_, err = database.StorageAreas().InsertOne(context.Background(), bson.M{"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "locationId": primitive.NewObjectID(), "name": "   ", "type": "dry", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now})
	if err == nil {
		t.Fatal("expected whitespace-only storage area name to be rejected")
	}
}

func TestIntegrationUnitsSchemaRejectsMalformedDocuments(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now()
	base := bson.M{"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "code": "kg", "name": "Kilogram", "symbol": "kg", "dimension": "mass", "precision": int32(3), "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}
	if _, err := database.Units().InsertOne(context.Background(), base); err != nil {
		t.Fatalf("insert valid unit: %v", err)
	}
	invalid := bson.M{}
	for key, value := range base {
		invalid[key] = value
	}
	invalid["_id"] = primitive.NewObjectID()
	invalid["code"] = "INVALID CODE"
	invalid["precision"] = int32(7)
	invalid["unexpected"] = true
	if _, err := database.Units().InsertOne(context.Background(), invalid); err == nil {
		t.Fatal("expected malformed unit to be rejected")
	}
}

func TestIntegrationCategoriesSchemaRejectsMalformedDocuments(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now()
	base := bson.M{"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "code": "hot-food", "name": "Hot Food", "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}
	if _, err := database.Categories().InsertOne(context.Background(), base); err != nil {
		t.Fatalf("insert valid category: %v", err)
	}
	invalid := bson.M{}
	for key, value := range base {
		invalid[key] = value
	}
	invalid["_id"] = primitive.NewObjectID()
	invalid["code"] = "INVALID CODE"
	invalid["unexpected"] = true
	if _, err := database.Categories().InsertOne(context.Background(), invalid); err == nil {
		t.Fatal("expected malformed category to be rejected")
	}
}

func TestIntegrationItemsSchemaRejectsMalformedDocuments(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now()
	base := bson.M{"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "sku": "burger-001", "name": "Burger", "categoryId": primitive.NewObjectID(), "baseUnitId": primitive.NewObjectID(), "allergens": bson.A{"milk"}, "shelfLifeDays": int32(30), "stockable": true, "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now}
	if _, err := database.Items().InsertOne(context.Background(), base); err != nil {
		t.Fatalf("insert valid item: %v", err)
	}
	invalid := bson.M{}
	for key, value := range base {
		invalid[key] = value
	}
	invalid["_id"] = primitive.NewObjectID()
	invalid["sku"] = "INVALID SKU"
	invalid["allergens"] = bson.A{"milk", "milk"}
	invalid["unexpected"] = true
	if _, err := database.Items().InsertOne(context.Background(), invalid); err == nil {
		t.Fatal("expected malformed item to be rejected")
	}
}

func TestIntegrationItemConversionsSchemaRejectsMalformedDocuments(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now()
	base := bson.M{
		"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "itemId": primitive.NewObjectID(), "fromUnitId": primitive.NewObjectID(),
		"numerator": int64(10), "denominator": int64(1), "isActive": true, "version": int64(1), "createdAt": now, "updatedAt": now,
	}
	if _, err := database.ItemConversions().InsertOne(context.Background(), base); err != nil {
		t.Fatalf("insert valid item conversion: %v", err)
	}
	invalid := bson.M{}
	for key, value := range base {
		invalid[key] = value
	}
	invalid["_id"] = primitive.NewObjectID()
	invalid["numerator"] = int64(0)
	invalid["unexpected"] = true
	if _, err := database.ItemConversions().InsertOne(context.Background(), invalid); err == nil {
		t.Fatal("expected malformed item conversion to be rejected")
	}
}

func TestIntegrationTenantBrandingSchemaRejectsUnsafeTokens(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now()
	base := bson.M{
		"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(),
		"primaryColor": "#123456", "accentColor": "", "font": "system",
		"version": int64(1), "createdAt": now, "updatedAt": now,
	}
	unsafe := bson.M{}
	for key, value := range base {
		unsafe[key] = value
	}
	unsafe["primaryColor"] = "red; background: url(https://example.test)"
	if _, err := database.TenantBranding().InsertOne(context.Background(), unsafe); err == nil {
		t.Fatal("expected unsafe color token to be rejected")
	}
	unknownFont := bson.M{}
	for key, value := range base {
		unknownFont[key] = value
	}
	unknownFont["_id"] = primitive.NewObjectID()
	unknownFont["font"] = "url-font"
	if _, err := database.TenantBranding().InsertOne(context.Background(), unknownFont); err == nil {
		t.Fatal("expected unknown font token to be rejected")
	}
	if _, err := database.TenantBranding().InsertOne(context.Background(), base); err != nil {
		t.Fatalf("insert valid tenant branding: %v", err)
	}
}

func TestIntegrationLocationBrandingSchemaRejectsUnsafeTokens(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now()
	base := bson.M{
		"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "locationId": primitive.NewObjectID(),
		"displayName": "Flagship", "primaryColor": "#123456", "accentColor": "", "font": "system",
		"version": int64(1), "createdAt": now, "updatedAt": now,
	}
	unsafe := bson.M{}
	for key, value := range base {
		unsafe[key] = value
	}
	unsafe["primaryColor"] = "var(--remote-color)"
	if _, err := database.LocationBranding().InsertOne(context.Background(), unsafe); err == nil {
		t.Fatal("expected unsafe location color token to be rejected")
	}
	unknown := bson.M{}
	for key, value := range base {
		unknown[key] = value
	}
	unknown["_id"] = primitive.NewObjectID()
	unknown["script"] = "alert(1)"
	if _, err := database.LocationBranding().InsertOne(context.Background(), unknown); err == nil {
		t.Fatal("expected additional location branding fields to be rejected")
	}
	if _, err := database.LocationBranding().InsertOne(context.Background(), base); err != nil {
		t.Fatalf("insert valid location branding: %v", err)
	}
}

func TestIntegrationStaffProfileSchemaRejectsInvalidDocuments(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now()
	base := bson.M{
		"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "userId": primitive.NewObjectID(),
		"businessRole": "viewer", "allLocations": false, "locationIds": bson.A{}, "permissionOverrides": bson.A{},
		"status": "active", "version": int64(1), "createdAt": now, "updatedAt": now,
	}
	invalidRole := bson.M{}
	for key, value := range base {
		invalidRole[key] = value
	}
	invalidRole["businessRole"] = "administrator"
	if _, err := database.StaffProfiles().InsertOne(context.Background(), invalidRole); err == nil {
		t.Fatal("expected unknown business role to be rejected")
	}
	invalidScope := bson.M{}
	for key, value := range base {
		invalidScope[key] = value
	}
	invalidScope["_id"] = primitive.NewObjectID()
	invalidScope["allLocations"] = true
	invalidScope["locationIds"] = bson.A{primitive.NewObjectID()}
	if _, err := database.StaffProfiles().InsertOne(context.Background(), invalidScope); err == nil {
		t.Fatal("expected ambiguous location scope to be rejected")
	}
	duplicateOverrides := bson.M{}
	for key, value := range base {
		duplicateOverrides[key] = value
	}
	duplicateOverrides["_id"] = primitive.NewObjectID()
	duplicateOverrides["permissionOverrides"] = bson.A{
		bson.M{"permission": "storage_areas.read", "allowed": true},
		bson.M{"permission": "storage_areas.read", "allowed": false},
	}
	if _, err := database.StaffProfiles().InsertOne(context.Background(), duplicateOverrides); err == nil {
		t.Fatal("expected duplicate permission overrides to be rejected")
	}
	duplicateLocations := bson.M{}
	for key, value := range base {
		duplicateLocations[key] = value
	}
	locationID := primitive.NewObjectID()
	duplicateLocations["_id"] = primitive.NewObjectID()
	duplicateLocations["locationIds"] = bson.A{locationID, locationID}
	if _, err := database.StaffProfiles().InsertOne(context.Background(), duplicateLocations); err == nil {
		t.Fatal("expected duplicate location IDs to be rejected")
	}
	extraField := bson.M{}
	for key, value := range base {
		extraField[key] = value
	}
	extraField["_id"] = primitive.NewObjectID()
	extraField["unexpected"] = true
	if _, err := database.StaffProfiles().InsertOne(context.Background(), extraField); err == nil {
		t.Fatal("expected additional fields to be rejected")
	}
	if _, err := database.StaffProfiles().InsertOne(context.Background(), base); err != nil {
		t.Fatalf("insert valid staff profile: %v", err)
	}
	duplicateIdentity := bson.M{}
	for key, value := range base {
		duplicateIdentity[key] = value
	}
	duplicateIdentity["_id"] = primitive.NewObjectID()
	if _, err := database.StaffProfiles().InsertOne(context.Background(), duplicateIdentity); err == nil {
		t.Fatal("expected tenant and user identity to be unique")
	}
}

func TestIntegrationStockMovementsSchemaRejectsZeroQuantity(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now().UTC()
	base := bson.M{
		"_id": primitive.NewObjectID(), "postingId": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(),
		"locationId": primitive.NewObjectID(), "storageAreaId": primitive.NewObjectID(), "itemId": primitive.NewObjectID(), "lineNumber": int32(0),
		"quantityMicros": int64(0), "effectiveAt": now, "recordedAt": now,
	}
	if _, err := database.StockMovements().InsertOne(context.Background(), base); err == nil {
		t.Fatal("expected zero stock movement quantity to be rejected")
	}
	base["_id"] = primitive.NewObjectID()
	base["quantityMicros"] = int64(1)
	if _, err := database.StockMovements().InsertOne(context.Background(), base); err != nil {
		t.Fatalf("valid stock movement rejected: %v", err)
	}
}

func TestIntegrationStockCountLifecycleSchemaValues(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	if err := database.EnsureSchemaValidation(); err != nil {
		t.Fatalf("apply schema validation: %v", err)
	}
	now := time.Now().UTC()
	posting := bson.M{
		"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "locationId": primitive.NewObjectID(),
		"storageAreaId": primitive.NewObjectID(), "userId": primitive.NewObjectID(), "type": "stock_count",
		"idempotencyKey": "stock-count-post-key", "requestHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"effectiveAt": now, "recordedAt": now,
	}
	if _, err := database.StockPostings().InsertOne(context.Background(), posting); err != nil {
		t.Fatalf("stock_count posting type rejected: %v", err)
	}

	count := bson.M{
		"_id": primitive.NewObjectID(), "tenantId": primitive.NewObjectID(), "locationId": primitive.NewObjectID(),
		"storageAreaId": primitive.NewObjectID(), "createdBy": primitive.NewObjectID(), "status": "cancelled",
		"version": int64(1), "idempotencyKey": "stock-count-lifecycle-key", "requestHash": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"createdAt": now, "updatedAt": now, "cancelledAt": now,
	}
	if _, err := database.StockCounts().InsertOne(context.Background(), count); err != nil {
		t.Fatalf("cancelled stock count rejected: %v", err)
	}

	invalidPosting := bson.M{}
	for key, value := range posting {
		invalidPosting[key] = value
	}
	invalidPosting["_id"] = primitive.NewObjectID()
	invalidPosting["type"] = "count_adjustment"
	if _, err := database.StockPostings().InsertOne(context.Background(), invalidPosting); err == nil {
		t.Fatal("unknown stock posting type should be rejected")
	}
	invalidCount := bson.M{}
	for key, value := range count {
		invalidCount[key] = value
	}
	invalidCount["_id"] = primitive.NewObjectID()
	invalidCount["status"] = "abandoned"
	if _, err := database.StockCounts().InsertOne(context.Background(), invalidCount); err == nil {
		t.Fatal("unknown stock count status should be rejected")
	}
}
