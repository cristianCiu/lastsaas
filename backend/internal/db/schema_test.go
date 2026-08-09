package db

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestAllSchemasIncludesStrictLocationsSchema(t *testing.T) {
	var location CollectionSchema
	for _, schema := range AllSchemas() {
		if schema.Collection == "locations" {
			location = schema
			break
		}
	}
	if location.Collection == "" {
		t.Fatal("locations schema missing from AllSchemas")
	}
	if !location.Critical {
		t.Fatal("locations schema failures must stop startup")
	}
	if location.ValidationLevel != "strict" {
		t.Fatalf("locations must use strict validation, got %q", location.ValidationLevel)
	}

	jsonSchema, ok := location.Schema["$jsonSchema"].(bson.M)
	if !ok {
		t.Fatal("locations $jsonSchema is missing")
	}
	if additional, ok := jsonSchema["additionalProperties"].(bool); !ok || additional {
		t.Fatal("locations schema must reject additional properties")
	}
	required, ok := jsonSchema["required"].(bson.A)
	if !ok || len(required) != 10 {
		t.Fatalf("unexpected locations required fields: %#v", jsonSchema["required"])
	}
	versionRequired := false
	for _, field := range required {
		if field == "version" {
			versionRequired = true
			break
		}
	}
	if !versionRequired {
		t.Fatal("locations schema must require version")
	}
	properties, ok := jsonSchema["properties"].(bson.M)
	if !ok || properties["tenantId"] == nil || properties["limitSlot"] == nil || properties["version"] == nil {
		t.Fatal("locations schema must include tenantId, limitSlot, and version")
	}
}
