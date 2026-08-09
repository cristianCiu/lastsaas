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
