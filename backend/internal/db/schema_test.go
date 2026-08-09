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
	want := map[string]int{"restaurant_settings": 8, "tenant_branding": 8, "storage_areas": 9}
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
		if schema.Collection == "tenant_branding" {
			properties := jsonSchema["properties"].(bson.M)
			if properties["primaryColor"].(bson.M)["pattern"] != `^(?:|#[0-9a-f]{6})$` {
				t.Errorf("tenant primary color must be empty or canonical hex")
			}
			if properties["font"].(bson.M)["enum"] == nil {
				t.Errorf("tenant fonts must use an allowlist")
			}
		}
		delete(want, schema.Collection)
	}
	for missing := range want {
		t.Errorf("%s schema missing from AllSchemas", missing)
	}
}

func TestStaffProfilesSchemaIsStrictAndComplete(t *testing.T) {
	var staff CollectionSchema
	for _, schema := range AllSchemas() {
		if schema.Collection == "staff_profiles" {
			staff = schema
			break
		}
	}
	if staff.Collection == "" || !staff.Critical || staff.ValidationLevel != "strict" {
		t.Fatal("staff_profiles must have a critical strict schema")
	}
	jsonSchema := staff.Schema["$jsonSchema"].(bson.M)
	if additional := jsonSchema["additionalProperties"].(bool); additional {
		t.Fatal("staff_profiles must reject additional properties")
	}
	if required := jsonSchema["required"].(bson.A); len(required) != 11 {
		t.Fatalf("unexpected staff profile required fields: %#v", required)
	}
	if staff.Schema["$expr"] == nil {
		t.Fatal("staff profile schema must enforce cross-field and override uniqueness rules")
	}
}

func TestTenantBrandingAssetsSchemaIsStrictAndBounded(t *testing.T) {
	var asset CollectionSchema
	for _, schema := range AllSchemas() {
		if schema.Collection == "tenant_branding_assets" {
			asset = schema
			break
		}
	}
	if asset.Collection == "" || !asset.Critical || asset.ValidationLevel != "strict" {
		t.Fatal("tenant_branding_assets must have a critical strict schema")
	}
	jsonSchema := asset.Schema["$jsonSchema"].(bson.M)
	if jsonSchema["additionalProperties"].(bool) {
		t.Fatal("tenant_branding_assets must reject additional properties")
	}
	if required := jsonSchema["required"].(bson.A); len(required) != 12 {
		t.Fatalf("unexpected tenant branding asset required fields: %#v", required)
	}
	properties := jsonSchema["properties"].(bson.M)
	if properties["data"].(bson.M)["bsonType"] != "binData" || properties["size"].(bson.M)["maximum"] != 921600 {
		t.Fatal("tenant branding asset binary data must be size bounded")
	}
	if properties["kind"].(bson.M)["enum"] == nil || properties["contentType"].(bson.M)["enum"] == nil {
		t.Fatal("tenant branding asset kinds and content types must use allowlists")
	}
}
