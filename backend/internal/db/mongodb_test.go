package db

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestStockCountIndexDeclarationsIncludeActiveListIndex(t *testing.T) {
	wantKeys := bson.D{
		{Key: "tenantId", Value: 1},
		{Key: "locationId", Value: 1},
		{Key: "status", Value: 1},
		{Key: "createdAt", Value: -1},
		{Key: "_id", Value: -1},
	}

	for _, model := range stockCountIndexModels() {
		if model.Options == nil || model.Options.Name == nil || *model.Options.Name != "stock_counts_tenant_location_status_created" {
			continue
		}
		if !reflect.DeepEqual(model.Keys, wantKeys) {
			t.Fatalf("unexpected active stock count index keys: %#v", model.Keys)
		}
		if model.Options.Unique != nil && *model.Options.Unique {
			t.Fatal("active stock count list index must not be unique")
		}
		return
	}

	t.Fatal("active stock count list index declaration missing")
}
