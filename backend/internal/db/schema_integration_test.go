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
