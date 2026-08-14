package db

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestPurchasingSchemasAreStrictAndTenantScoped(t *testing.T) {
	want := map[string]bool{"purchase_orders": false, "purchase_order_lines": false, "delivery_calendars": false, "goods_receipts": false, "goods_receipt_lines": false, "purchase_order_email_deliveries": false}
	for _, schema := range AllSchemas() {
		if _, ok := want[schema.Collection]; !ok {
			continue
		}
		jsonSchema, ok := schema.Schema["$jsonSchema"].(bson.M)
		if !ok || !schema.Critical || schema.ValidationLevel != "strict" || jsonSchema["additionalProperties"] != false {
			t.Errorf("%s must be critical and strict", schema.Collection)
		}
		properties, ok := jsonSchema["properties"].(bson.M)
		if !ok || properties["tenantId"] == nil || properties["locationId"] == nil {
			t.Errorf("%s must be tenant and location scoped", schema.Collection)
		}
		want[schema.Collection] = true
	}
	for collection, found := range want {
		if !found {
			t.Errorf("%s schema missing", collection)
		}
	}
}
