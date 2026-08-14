package db

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestRecipeSchemasAreStrictAndRegistered(t *testing.T) {
	want := map[string]bool{"recipes": false, "recipe_versions": false, "recipe_components": false, "external_product_mappings": false}
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
		properties, ok := jsonSchema["properties"].(bson.M)
		if !ok || properties["tenantId"] == nil {
			t.Errorf("%s must be tenant scoped", schema.Collection)
		}
		if schema.Collection == "recipe_versions" && properties["yieldFactorMicros"].(bson.M)["bsonType"] != "long" {
			t.Error("recipe factors must use int64 BSON longs")
		}
		if schema.Collection == "recipe_components" && schema.Schema["$expr"] == nil {
			t.Error("component reference exclusivity must be schema enforced")
		}
		if schema.Collection == "external_product_mappings" && schema.Schema["$expr"] == nil {
			t.Error("mapping effective range must be schema enforced")
		}
		want[schema.Collection] = true
	}
	for collection, found := range want {
		if !found {
			t.Errorf("%s schema missing from AllSchemas", collection)
		}
	}
}
