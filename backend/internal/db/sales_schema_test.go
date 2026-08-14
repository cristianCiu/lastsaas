package db

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestSalesSchemasAreTenantScopedAndStrict(t *testing.T) {
	want := map[string]bool{"sales": false, "sales_lines": false, "unresolved_sale_lines": false, "sales_import_runs": false}
	for _, schema := range AllSchemas() {
		if _, ok := want[schema.Collection]; !ok {
			continue
		}
		jsonSchema, ok := schema.Schema["$jsonSchema"].(bson.M)
		if !ok || schema.ValidationLevel != "strict" || !schema.Critical || jsonSchema["additionalProperties"] != false {
			t.Errorf("%s must use a critical strict schema", schema.Collection)
		}
		properties, ok := jsonSchema["properties"].(bson.M)
		if !ok || properties["tenantId"] == nil {
			t.Errorf("%s must require tenant scope", schema.Collection)
		}
		want[schema.Collection] = true
	}
	for collection, found := range want {
		if !found {
			t.Errorf("%s schema missing from AllSchemas", collection)
		}
	}
}
