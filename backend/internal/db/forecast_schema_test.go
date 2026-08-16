package db

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestForecastSchemasAreStrictAndRegistered(t *testing.T) {
	want := []string{"forecast_datasets", "forecast_input_rows", "forecast_jobs", "forecast_runs", "forecast_points", "forecast_metrics", "forecast_policies", "guest_plans", "forecast_overrides", "reorder_recommendations", "forecast_coverages"}
	for _, name := range want {
		var found *CollectionSchema
		for _, schema := range AllSchemas() {
			if schema.Collection == name {
				copy := schema
				found = &copy
				break
			}
		}
		if found == nil {
			t.Fatalf("%s schema missing", name)
		}
		if !found.Critical || found.ValidationLevel != "strict" {
			t.Errorf("%s must be critical and strict", name)
		}
		jsonSchema := found.Schema["$jsonSchema"].(bson.M)
		if jsonSchema["additionalProperties"] != false {
			t.Errorf("%s must reject unknown fields", name)
		}
	}
}
