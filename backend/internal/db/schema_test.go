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

func TestOffboardingSchemasAreStrictAndRegistered(t *testing.T) {
	want := map[string]bool{"tenant_offboarding_tombstones": false, "tenant_offboarding_audit": false}
	for _, schema := range AllSchemas() {
		if _, ok := want[schema.Collection]; !ok {
			continue
		}
		if !schema.Critical || schema.ValidationLevel != "strict" {
			t.Errorf("%s must be critical and strict", schema.Collection)
		}
		jsonSchema := schema.Schema["$jsonSchema"].(bson.M)
		if jsonSchema["additionalProperties"] != false {
			t.Errorf("%s must reject additional properties", schema.Collection)
		}
		if jsonSchema["properties"].(bson.M)["_id"] == nil {
			t.Errorf("%s must validate its immutable id", schema.Collection)
		}
		want[schema.Collection] = true
	}
	for collection, found := range want {
		if !found {
			t.Errorf("%s schema missing", collection)
		}
	}
}

func TestRestaurantCollectionsUseStrictCriticalSchemas(t *testing.T) {
	want := map[string]int{"restaurant_settings": 8, "tenant_branding": 8, "location_branding": 10, "storage_areas": 9}
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
		if schema.Collection == "tenant_branding" || schema.Collection == "location_branding" {
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

func TestStorageAreaSchemaDefinesOptionalInventoryFields(t *testing.T) {
	var storageAreas CollectionSchema
	for _, schema := range AllSchemas() {
		if schema.Collection == "storage_areas" {
			storageAreas = schema
			break
		}
	}
	if storageAreas.Collection == "" {
		t.Fatal("storage_areas schema missing from AllSchemas")
	}

	jsonSchema := storageAreas.Schema["$jsonSchema"].(bson.M)
	properties := jsonSchema["properties"].(bson.M)
	inventoryFence := properties["inventoryFence"].(bson.M)
	if inventoryFence["bsonType"] != "long" || inventoryFence["minimum"] != 0 {
		t.Fatalf("unexpected inventoryFence schema: %#v", inventoryFence)
	}
	if properties["activeStockCountId"].(bson.M)["bsonType"] != "objectId" {
		t.Fatalf("unexpected activeStockCountId schema: %#v", properties["activeStockCountId"])
	}
	for _, field := range jsonSchema["required"].(bson.A) {
		if field == "inventoryFence" || field == "activeStockCountId" {
			t.Fatalf("storage area field must remain optional: %v", field)
		}
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

func TestInventorySchemasAreStrictAndRegistered(t *testing.T) {
	want := map[string]bool{"stock_postings": false, "stock_movements": false, "stock_balances": false, "stock_lots": false, "stock_counts": false, "stock_count_lines": false, "reconciliation_runs": false}
	for _, schema := range AllSchemas() {
		if _, ok := want[schema.Collection]; !ok {
			continue
		}
		if !schema.Critical || schema.ValidationLevel != "strict" {
			t.Errorf("%s must be critical and strict", schema.Collection)
		}
		jsonSchema, ok := schema.Schema["$jsonSchema"].(bson.M)
		if !ok || jsonSchema["additionalProperties"] != false {
			t.Errorf("%s must reject additional fields", schema.Collection)
		}
		if jsonSchema["properties"].(bson.M)["tenantId"] == nil {
			t.Errorf("%s must be tenant scoped", schema.Collection)
		}
		if schema.Collection == "stock_movements" {
			if _, nested := jsonSchema["$expr"]; nested {
				t.Fatal("stock movement $expr must not be nested in $jsonSchema")
			}
			if _, topLevel := schema.Schema["$expr"]; !topLevel {
				t.Fatal("stock movement validator must retain a top-level non-zero quantity expression")
			}
		}
		if schema.Collection == "stock_lots" {
			properties := jsonSchema["properties"].(bson.M)
			if properties["code"].(bson.M)["maxLength"] != int32(128) && properties["code"].(bson.M)["maxLength"] != 128 {
				t.Errorf("stock lot code must be bounded: %#v", properties["code"])
			}
			if properties["status"].(bson.M)["enum"] == nil || properties["itemId"] == nil {
				t.Fatal("stock lot schema missing status or item scope")
			}
		}
		if schema.Collection == "stock_counts" || schema.Collection == "stock_count_lines" || schema.Collection == "reconciliation_runs" {
			if schema.Schema["$jsonSchema"].(bson.M)["required"] == nil {
				t.Fatalf("%s must declare required workflow fields", schema.Collection)
			}
		}
		if schema.Collection == "reconciliation_runs" {
			properties := jsonSchema["properties"].(bson.M)
			if properties["mismatches"].(bson.M)["maxItems"] != int32(1000) && properties["mismatches"].(bson.M)["maxItems"] != 1000 {
				t.Fatal("reconciliation mismatch summaries must be bounded")
			}
		}
		want[schema.Collection] = true
	}
	for collection, found := range want {
		if !found {
			t.Errorf("%s schema missing", collection)
		}
	}
}

func TestInventorySchemasAllowCountLifecycleValues(t *testing.T) {
	var postings, counts CollectionSchema
	for _, schema := range AllSchemas() {
		switch schema.Collection {
		case "stock_postings":
			postings = schema
		case "stock_counts":
			counts = schema
		}
	}
	if postings.Collection == "" || counts.Collection == "" {
		t.Fatal("inventory schemas missing")
	}

	postingProperties := postings.Schema["$jsonSchema"].(bson.M)["properties"].(bson.M)
	postingTypes := postingProperties["type"].(bson.M)["enum"].(bson.A)
	if !containsSchemaEnum(postingTypes, "stock_count") {
		t.Fatalf("stock posting schema missing stock_count type: %#v", postingTypes)
	}

	countSchema := counts.Schema["$jsonSchema"].(bson.M)
	countProperties := countSchema["properties"].(bson.M)
	statuses := countProperties["status"].(bson.M)["enum"].(bson.A)
	if !containsSchemaEnum(statuses, "cancelled") {
		t.Fatalf("stock count schema missing cancelled status: %#v", statuses)
	}
	if cancelledAt, ok := countProperties["cancelledAt"].(bson.M); !ok || cancelledAt["bsonType"] != "date" {
		t.Fatalf("unexpected cancelledAt schema: %#v", countProperties["cancelledAt"])
	}
	for _, field := range countSchema["required"].(bson.A) {
		if field == "cancelledAt" {
			t.Fatal("cancelledAt must remain optional")
		}
	}
}

func containsSchemaEnum(values bson.A, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
