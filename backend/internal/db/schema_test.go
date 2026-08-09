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

func TestRestaurantCollectionsUseStrictCriticalSchemas(t *testing.T) {
	want := map[string]int{"restaurant_settings": 8, "storage_areas": 9}
	for _, schema := range AllSchemas() {
		requiredCount, ok := want[schema.Collection]
		if !ok {
			continue
		}
		if !schema.Critical || schema.ValidationLevel != "strict" {
			t.Errorf("%s must be critical and strict", schema.Collection)
		}
		jsonSchema := schema.Schema["$jsonSchema"].(bson.M)
		if additional, ok := jsonSchema["additionalProperties"].(bool); !ok || additional {
			t.Errorf("%s must reject additional properties", schema.Collection)
		}
		if required := jsonSchema["required"].(bson.A); len(required) != requiredCount {
			t.Errorf("%s required fields: %#v", schema.Collection, required)
		}
		if schema.Collection == "storage_areas" {
			properties := jsonSchema["properties"].(bson.M)
			name := properties["name"].(bson.M)
			if name["pattern"] != `.*\S.*` {
				t.Errorf("storage area names must require non-whitespace: %#v", name)
			}
		}
		delete(want, schema.Collection)
	}
	for missing := range want {
		t.Errorf("%s schema missing from AllSchemas", missing)
	}
}
