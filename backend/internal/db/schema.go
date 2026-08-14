package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// CollectionSchema pairs a collection name with its JSON Schema validator.
type CollectionSchema struct {
	Collection      string
	Schema          bson.M
	Critical        bool
	ValidationLevel string
}

// AllSchemas returns the JSON Schema validators for all validated collections.
func AllSchemas() []CollectionSchema {
	return []CollectionSchema{
		usersSchema(),
		tenantsSchema(),
		tenantMembershipsSchema(),
		invitationsSchema(),
		plansSchema(),
		creditBundlesSchema(),
		financialTransactionsSchema(),
		webhooksSchema(),
		apiKeysSchema(),
		configVarsSchema(),
		announcementsSchema(),
		customPagesSchema(),
		messagesSchema(),
		usageEventsSchema(),
		ssoConnectionsSchema(),
		eventDefinitionsSchema(),
		locationsSchema(),
		restaurantSettingsSchema(),
		tenantBrandingSchema(),
		locationBrandingSchema(),
		tenantBrandingAssetsSchema(),
		storageAreasSchema(),
		staffProfilesSchema(),
		unitsSchema(),
		categoriesSchema(),
		itemsSchema(),
		itemConversionsSchema(),
		suppliersSchema(),
		supplierItemsSchema(),
		recipesSchema(),
		recipeVersionsSchema(),
		recipeComponentsSchema(),
		externalProductMappingsSchema(),
		salesSchema(),
		salesLinesSchema(),
		unresolvedSaleLinesSchema(),
		salesImportRunsSchema(),
		purchaseOrdersSchema(),
		purchaseOrderLinesSchema(),
		deliveryCalendarsSchema(),
		goodsReceiptsSchema(),
		goodsReceiptLinesSchema(),
		purchaseOrderEmailDeliveriesSchema(),
		importRunsSchema(),
		stockPostingsSchema(),
		stockMovementsSchema(),
		stockBalancesSchema(),
		stockLotsSchema(),
		stockCountsSchema(),
		stockCountLinesSchema(),
		reconciliationRunsSchema(),
	}
}

func recipesSchema() CollectionSchema {
	return CollectionSchema{Collection: "recipes", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "code", "name", "isActive", "version", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
			"code":        bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64},
			"name":        bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160},
			"description": bson.M{"bsonType": "string", "maxLength": 2000}, "isActive": bson.M{"bsonType": "bool"},
			"version": bson.M{"bsonType": "long", "minimum": int64(1)}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func recipeVersionsSchema() CollectionSchema {
	return CollectionSchema{Collection: "recipe_versions", Critical: true, ValidationLevel: "strict", Schema: bson.M{
		"$jsonSchema": bson.M{"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "recipeId", "number", "outputUnitId", "portionCount", "yieldFactorMicros", "lossFactorMicros", "effectiveFrom", "status", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "recipeId": bson.M{"bsonType": "objectId"},
				"number": bson.M{"bsonType": "long", "minimum": int64(1)}, "outputUnitId": bson.M{"bsonType": "objectId"}, "portionCount": bson.M{"bsonType": "long", "minimum": int64(1)},
				"yieldFactorMicros": bson.M{"bsonType": "long", "minimum": int64(1), "maximum": int64(1000000)}, "lossFactorMicros": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(1000000)},
				"effectiveFrom": bson.M{"bsonType": "date"}, "effectiveTo": bson.M{"bsonType": "date"},
				"status": bson.M{"bsonType": "string", "enum": bson.A{"draft", "released", "retired"}}, "version": bson.M{"bsonType": "long", "minimum": int64(1)},
				"releasedAt": bson.M{"bsonType": "date"}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		},
		"$expr": bson.M{"$or": bson.A{
			bson.M{"$eq": bson.A{bson.M{"$type": "$effectiveTo"}, "missing"}},
			bson.M{"$gt": bson.A{"$effectiveTo", "$effectiveFrom"}},
		}},
	}}
}

func recipeComponentsSchema() CollectionSchema {
	return CollectionSchema{Collection: "recipe_components", Critical: true, ValidationLevel: "strict", Schema: bson.M{
		"$jsonSchema": bson.M{"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "recipeVersionId", "componentType", "quantityMicros", "unitId", "sortOrder", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "recipeVersionId": bson.M{"bsonType": "objectId"},
				"componentType": bson.M{"bsonType": "string", "enum": bson.A{"item", "subrecipe"}}, "itemId": bson.M{"bsonType": "objectId"}, "subrecipeId": bson.M{"bsonType": "objectId"},
				"quantityMicros": bson.M{"bsonType": "long", "minimum": int64(1)}, "unitId": bson.M{"bsonType": "objectId"}, "sortOrder": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(1000000)},
				"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		},
		"$expr": bson.M{"$or": bson.A{
			bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$componentType", "item"}}, bson.M{"$ne": bson.A{"$itemId", nil}}, bson.M{"$eq": bson.A{"$subrecipeId", nil}}}},
			bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$componentType", "subrecipe"}}, bson.M{"$eq": bson.A{"$itemId", nil}}, bson.M{"$ne": bson.A{"$subrecipeId", nil}}}},
		}},
	}}
}

func externalProductMappingsSchema() CollectionSchema {
	return CollectionSchema{Collection: "external_product_mappings", Critical: true, ValidationLevel: "strict", Schema: bson.M{
		"$jsonSchema": bson.M{"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "adapter", "externalProductId", "recipeVersionId", "effectiveFrom", "isActive", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "adapter": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64},
				"externalProductId": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 256}, "recipeVersionId": bson.M{"bsonType": "objectId"},
				"effectiveFrom": bson.M{"bsonType": "date"}, "effectiveTo": bson.M{"bsonType": "date"}, "isActive": bson.M{"bsonType": "bool"},
				"version": bson.M{"bsonType": "long", "minimum": int64(1)}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		},
		"$expr": bson.M{"$or": bson.A{bson.M{"$eq": bson.A{bson.M{"$type": "$effectiveTo"}, "missing"}}, bson.M{"$gt": bson.A{"$effectiveTo", "$effectiveFrom"}}}},
	}}
}

func salesSchema() CollectionSchema {
	return CollectionSchema{Collection: "sales", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "source", "externalSaleId", "occurredAt", "locationId", "storageAreaId", "status", "version", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
			"source": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "externalSaleId": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 128},
			"occurredAt": bson.M{"bsonType": "date"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"},
			"status": bson.M{"bsonType": "string", "enum": bson.A{"completed", "cancelled"}}, "cancelledAt": bson.M{"bsonType": "date"},
			"consumptionPostingId": bson.M{"bsonType": "objectId"}, "reversalPostingId": bson.M{"bsonType": "objectId"}, "version": bson.M{"bsonType": "long", "minimum": int64(1)},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func salesLinesSchema() CollectionSchema {
	return CollectionSchema{Collection: "sales_lines", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "saleId", "source", "externalSaleId", "externalLineId", "externalProductId", "quantityMicros", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "saleId": bson.M{"bsonType": "objectId"},
			"source": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "externalSaleId": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 128}, "externalLineId": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 128}, "externalProductId": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 256},
			"quantityMicros": bson.M{"bsonType": "long", "minimum": int64(1)}, "mappingId": bson.M{"bsonType": "objectId"}, "mappingVersion": bson.M{"bsonType": "long", "minimum": int64(1)}, "recipeVersionId": bson.M{"bsonType": "objectId"}, "recipeVersionNumber": bson.M{"bsonType": "long", "minimum": int64(1)},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func unresolvedSaleLinesSchema() CollectionSchema {
	return CollectionSchema{Collection: "unresolved_sale_lines", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "importRunId", "source", "externalSaleId", "externalLineId", "externalProductId", "occurredAt", "locationId", "storageAreaId", "quantityMicros", "reason", "createdAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "importRunId": bson.M{"bsonType": "objectId"}, "source": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64},
			"externalSaleId": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 128}, "externalLineId": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 128}, "externalProductId": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 256},
			"occurredAt": bson.M{"bsonType": "date"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"}, "quantityMicros": bson.M{"bsonType": "long", "minimum": int64(1)}, "mappingId": bson.M{"bsonType": "objectId"}, "recipeVersionId": bson.M{"bsonType": "objectId"}, "reason": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 240}, "createdAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func salesImportRunsSchema() CollectionSchema {
	return CollectionSchema{Collection: "sales_import_runs", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "userId", "source", "idempotencyKey", "status", "totalRows", "createdSales", "createdLines", "unresolvedRows", "errors", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "userId": bson.M{"bsonType": "objectId"}, "source": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "status": bson.M{"bsonType": "string", "enum": bson.A{"pending", "completed", "failed"}},
			"totalRows": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(5000)}, "createdSales": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(5000)}, "createdLines": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(5000)}, "unresolvedRows": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(5000)},
			"errors":    bson.M{"bsonType": "array", "maxItems": int32(100), "items": bson.M{"bsonType": "object", "additionalProperties": false, "required": bson.A{"row", "field", "code", "message"}, "properties": bson.M{"row": bson.M{"bsonType": "int", "minimum": int32(2), "maximum": int32(5001)}, "field": bson.M{"bsonType": "string", "maxLength": 64}, "code": bson.M{"bsonType": "string", "maxLength": 64}, "message": bson.M{"bsonType": "string", "maxLength": 240}}}},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func stockPostingsSchema() CollectionSchema {
	return CollectionSchema{Collection: "stock_postings", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "storageAreaId", "userId", "type", "idempotencyKey", "requestHash", "effectiveAt", "recordedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"}, "userId": bson.M{"bsonType": "objectId"},
			"type":           bson.M{"bsonType": "string", "enum": bson.A{"opening_balance", "manual_adjustment", "reversal", "transfer", "waste", "stock_count", "sale_consumption", "goods_receipt"}},
			"idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "requestHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`},
			"effectiveAt": bson.M{"bsonType": "date"}, "recordedAt": bson.M{"bsonType": "date"}, "reversalOf": bson.M{"bsonType": "objectId"},
			"reason":                bson.M{"bsonType": "string", "maxLength": 500, "pattern": `.*\S.*`},
			"destinationLocationId": bson.M{"bsonType": "objectId"}, "destinationStorageAreaId": bson.M{"bsonType": "objectId"},
		},
	}}}
}

func stockMovementsSchema() CollectionSchema {
	return CollectionSchema{Collection: "stock_movements", Critical: true, ValidationLevel: "strict", Schema: bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "postingId", "tenantId", "locationId", "storageAreaId", "itemId", "quantityMicros", "effectiveAt", "recordedAt", "lineNumber"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "postingId": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"},
				"quantityMicros": bson.M{"bsonType": "long", "minimum": int64(-9223372036854775807), "maximum": int64(9223372036854775807)},
				"effectiveAt":    bson.M{"bsonType": "date"}, "recordedAt": bson.M{"bsonType": "date"},
				"lotId": bson.M{"bsonType": "objectId"}, "lineNumber": bson.M{"bsonType": "int", "minimum": int32(0)},
			},
		},
		// $expr is a validator-level query expression, not a JSON Schema keyword.
		"$expr": bson.M{"$ne": bson.A{"$quantityMicros", int64(0)}},
	}}
}

func stockBalancesSchema() CollectionSchema {
	return CollectionSchema{Collection: "stock_balances", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "storageAreaId", "itemId", "quantityMicros", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"},
			"quantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "updatedAt": bson.M{"bsonType": "date"},
			"lotId": bson.M{"bsonType": "objectId"},
		},
	}}}
}

func stockLotsSchema() CollectionSchema {
	return CollectionSchema{Collection: "stock_lots", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "itemId", "code", "receivedAt", "status", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"},
			"code": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 128, "pattern": `.*\S.*`}, "expiresAt": bson.M{"bsonType": "date"}, "receivedAt": bson.M{"bsonType": "date"},
			"status": bson.M{"bsonType": "string", "enum": bson.A{"available", "quarantined"}}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func stockCountsSchema() CollectionSchema {
	return CollectionSchema{Collection: "stock_counts", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "storageAreaId", "createdBy", "status", "version", "idempotencyKey", "requestHash", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"}, "createdBy": bson.M{"bsonType": "objectId"},
			"status": bson.M{"bsonType": "string", "enum": bson.A{"draft", "frozen", "reviewed", "posted", "cancelled"}}, "version": bson.M{"bsonType": "long", "minimum": int64(1)},
			"idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "requestHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`}, "postIdempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "postRequestHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`}, "postPostingId": bson.M{"bsonType": "objectId"},
			"cutoffAt": bson.M{"bsonType": "date"}, "frozenAt": bson.M{"bsonType": "date"}, "reviewedAt": bson.M{"bsonType": "date"}, "postedAt": bson.M{"bsonType": "date"}, "cancelledAt": bson.M{"bsonType": "date"},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func stockCountLinesSchema() CollectionSchema {
	return CollectionSchema{Collection: "stock_count_lines", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "countId", "tenantId", "locationId", "storageAreaId", "itemId", "snapshotQuantityMicros", "version", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "countId": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"}, "lotId": bson.M{"bsonType": "objectId"},
			"snapshotQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(9223372036854775807)}, "countedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(9223372036854775807)},
			"version": bson.M{"bsonType": "long", "minimum": int64(1)}, "note": bson.M{"bsonType": "string", "maxLength": 500, "pattern": `.*\S.*`}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func reconciliationRunsSchema() CollectionSchema {
	return CollectionSchema{Collection: "reconciliation_runs", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "userId", "locationId", "cutoffAt", "idempotencyKey", "requestHash", "status", "mismatchCount", "truncated", "mismatches", "createdAt", "completedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "userId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"},
			"cutoffAt": bson.M{"bsonType": "date"}, "idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "requestHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`}, "status": bson.M{"bsonType": "string", "enum": bson.A{"detected", "repaired"}}, "mismatchCount": bson.M{"bsonType": "long", "minimum": int64(0)}, "truncated": bson.M{"bsonType": "bool"},
			"mismatches": bson.M{"bsonType": "array", "maxItems": 1000, "items": bson.M{"bsonType": "object", "additionalProperties": false, "required": bson.A{"locationId", "storageAreaId", "itemId", "expectedQuantityMicros", "actualQuantityMicros", "deltaMicros"}, "properties": bson.M{
				"locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"}, "lotId": bson.M{"bsonType": "objectId"}, "expectedQuantityMicros": bson.M{"bsonType": "long"}, "actualQuantityMicros": bson.M{"bsonType": "long"}, "deltaMicros": bson.M{"bsonType": "long"},
			}}},
			"createdAt": bson.M{"bsonType": "date"}, "completedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func importRunsSchema() CollectionSchema {
	return CollectionSchema{Collection: "import_runs", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "userId", "target", "idempotencyKey", "status", "totalRows", "createdRows", "updatedRows", "errors", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "userId": bson.M{"bsonType": "objectId"},
			"target":         bson.M{"bsonType": "string", "enum": bson.A{"units", "categories", "items", "suppliers", "supplier_items"}},
			"idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128},
			"status":         bson.M{"bsonType": "string", "enum": bson.A{"pending", "completed", "failed"}},
			"totalRows":      bson.M{"bsonType": "int", "minimum": 0, "maximum": 5000}, "createdRows": bson.M{"bsonType": "int", "minimum": 0, "maximum": 5000}, "updatedRows": bson.M{"bsonType": "int", "minimum": 0, "maximum": 5000},
			"errors": bson.M{"bsonType": "array", "maxItems": 100, "items": bson.M{"bsonType": "object", "additionalProperties": false, "required": bson.A{"row", "field", "code", "message"}, "properties": bson.M{
				"row": bson.M{"bsonType": "int", "minimum": 2, "maximum": 5001}, "field": bson.M{"bsonType": "string", "maxLength": 64}, "code": bson.M{"bsonType": "string", "maxLength": 64}, "message": bson.M{"bsonType": "string", "maxLength": 240},
			}}},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func suppliersSchema() CollectionSchema {
	return CollectionSchema{Collection: "suppliers", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "code", "name", "defaultLeadTimeDays", "isActive", "version", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
			"code": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "name": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160},
			"contactName": bson.M{"bsonType": "string", "maxLength": 100}, "email": bson.M{"bsonType": "string", "maxLength": 254, "pattern": `^[^@\s]+@[^@\s]+\.[^@\s]+$`}, "phone": bson.M{"bsonType": "string", "maxLength": 32},
			"orderingDays": bson.M{"bsonType": "array", "uniqueItems": true, "items": bson.M{"bsonType": "int", "minimum": 1, "maximum": 7}}, "defaultLeadTimeDays": bson.M{"bsonType": "int", "minimum": 0, "maximum": 3650},
			"isActive": bson.M{"bsonType": "bool"}, "version": bson.M{"bsonType": "long", "minimum": 1}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func supplierItemsSchema() CollectionSchema {
	return CollectionSchema{Collection: "supplier_items", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "supplierId", "itemId", "packSizeMicros", "moq", "unitPriceMinor", "currency", "isActive", "version", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "supplierId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"},
			"supplierSKU": bson.M{"bsonType": "string", "maxLength": 100}, "packSizeMicros": bson.M{"bsonType": "long", "minimum": 1}, "moq": bson.M{"bsonType": "int", "minimum": 1, "maximum": 1000000},
			"unitPriceMinor": bson.M{"bsonType": "long", "minimum": 0}, "currency": bson.M{"bsonType": "string", "pattern": `^[A-Z]{3}$`}, "leadTimeDays": bson.M{"bsonType": "int", "minimum": 0, "maximum": 3650},
			"isActive": bson.M{"bsonType": "bool"}, "version": bson.M{"bsonType": "long", "minimum": 1}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func itemConversionsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "item_conversions", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "itemId", "fromUnitId", "numerator", "denominator", "isActive", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
				"itemId": bson.M{"bsonType": "objectId"}, "fromUnitId": bson.M{"bsonType": "objectId"},
				"numerator":   bson.M{"bsonType": "long", "minimum": 1, "maximum": 1000000000},
				"denominator": bson.M{"bsonType": "long", "minimum": 1, "maximum": 1000000000},
				"isActive":    bson.M{"bsonType": "bool"}, "version": bson.M{"bsonType": "long", "minimum": 1},
				"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		}},
	}
}

func itemsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "items", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "sku", "name", "categoryId", "baseUnitId", "stockable", "isActive", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
				"sku":         bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64},
				"name":        bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160},
				"description": bson.M{"bsonType": "string", "maxLength": 2000},
				"brand":       bson.M{"bsonType": "string", "maxLength": 100},
				"categoryId":  bson.M{"bsonType": "objectId"}, "baseUnitId": bson.M{"bsonType": "objectId"},
				"allergens": bson.M{"bsonType": "array", "uniqueItems": true, "items": bson.M{"bsonType": "string", "enum": bson.A{
					"celery", "cereals-gluten", "crustaceans", "eggs", "fish", "lupin", "milk", "molluscs", "mustard", "nuts", "peanuts", "sesame", "soy", "sulphites",
				}}},
				"shelfLifeDays": bson.M{"bsonType": "int", "minimum": 0, "maximum": 36500},
				"stockable":     bson.M{"bsonType": "bool"}, "lotTracking": bson.M{"bsonType": "string", "enum": bson.A{"", "optional", "required"}}, "isActive": bson.M{"bsonType": "bool"},
				"version": bson.M{"bsonType": "long", "minimum": 1}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		}},
	}
}

func categoriesSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "categories", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "code", "name", "isActive", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
				"code":     bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 32},
				"name":     bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 100},
				"isActive": bson.M{"bsonType": "bool"}, "version": bson.M{"bsonType": "long", "minimum": 1},
				"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		}},
	}
}

func unitsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "units", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "code", "name", "symbol", "dimension", "precision", "isActive", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
				"code":      bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 32},
				"name":      bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 100},
				"symbol":    bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 16},
				"dimension": bson.M{"bsonType": "string", "enum": bson.A{"mass", "volume", "count"}},
				"precision": bson.M{"bsonType": "int", "minimum": 0, "maximum": 6},
				"isActive":  bson.M{"bsonType": "bool"}, "version": bson.M{"bsonType": "long", "minimum": 1},
				"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		}},
	}
}

func locationBrandingSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "location_branding", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "locationId", "displayName", "primaryColor", "accentColor", "font", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"},
				"displayName":  bson.M{"bsonType": "string", "maxLength": 200},
				"primaryColor": bson.M{"bsonType": "string", "pattern": `^(?:|#[0-9a-f]{6})$`},
				"accentColor":  bson.M{"bsonType": "string", "pattern": `^(?:|#[0-9a-f]{6})$`},
				"font":         bson.M{"bsonType": "string", "enum": bson.A{"", "system", "humanist", "geometric", "serif"}},
				"version":      bson.M{"bsonType": "long", "minimum": 1},
				"createdAt":    bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		}},
	}
}

func tenantBrandingAssetsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "tenant_branding_assets", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "kind", "storageKey", "contentType", "data", "size", "width", "height", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
				"kind":        bson.M{"bsonType": "string", "enum": bson.A{"primary", "compact"}},
				"storageKey":  bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{32}$`},
				"contentType": bson.M{"bsonType": "string", "enum": bson.A{"image/png", "image/jpeg"}},
				"data":        bson.M{"bsonType": "binData"},
				"size":        bson.M{"bsonType": "long", "minimum": 1, "maximum": 921600},
				"width":       bson.M{"bsonType": "int", "minimum": 16, "maximum": 2048},
				"height":      bson.M{"bsonType": "int", "minimum": 16, "maximum": 2048},
				"version":     bson.M{"bsonType": "long", "minimum": 1},
				"createdAt":   bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		}},
	}
}

func tenantBrandingSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "tenant_branding", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "primaryColor", "accentColor", "font", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
				"primaryColor": bson.M{"bsonType": "string", "pattern": `^(?:|#[0-9a-f]{6})$`},
				"accentColor":  bson.M{"bsonType": "string", "pattern": `^(?:|#[0-9a-f]{6})$`},
				"font":         bson.M{"bsonType": "string", "enum": bson.A{"", "system", "humanist", "geometric", "serif"}},
				"version":      bson.M{"bsonType": "long", "minimum": 1},
				"createdAt":    bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		}},
	}
}

func staffProfilesSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "staff_profiles", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object", "additionalProperties": false,
				"required": bson.A{"_id", "tenantId", "userId", "businessRole", "allLocations", "locationIds", "permissionOverrides", "status", "version", "createdAt", "updatedAt"},
				"properties": bson.M{
					"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "userId": bson.M{"bsonType": "objectId"},
					"businessRole": bson.M{"bsonType": "string", "enum": bson.A{"company_owner", "operations_manager", "head_chef", "purchasing", "stock_service", "controller", "viewer"}},
					"allLocations": bson.M{"bsonType": "bool"},
					"locationIds":  bson.M{"bsonType": "array", "uniqueItems": true, "items": bson.M{"bsonType": "objectId"}},
					"permissionOverrides": bson.M{"bsonType": "array", "items": bson.M{
						"bsonType": "object", "additionalProperties": false, "required": bson.A{"permission", "allowed"},
						"properties": bson.M{"permission": bson.M{"bsonType": "string", "enum": bson.A{"storage_areas.read", "storage_areas.manage", "catalog.read", "catalog.manage", "inventory.read", "inventory.post", "inventory.manage", "inventory.lot_override", "sales.read", "sales.manage", "purchasing.read", "purchasing.manage", "purchasing.approve", "purchasing.receive"}}, "allowed": bson.M{"bsonType": "bool"}},
					}},
					"status":  bson.M{"bsonType": "string", "enum": bson.A{"active", "inactive"}},
					"version": bson.M{"bsonType": "long", "minimum": 1}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
				},
			},
			"$expr": bson.M{"$and": bson.A{
				bson.M{"$or": bson.A{
					bson.M{"$eq": bson.A{"$allLocations", false}},
					bson.M{"$eq": bson.A{bson.M{"$size": "$locationIds"}, 0}},
				}},
				bson.M{"$eq": bson.A{
					bson.M{"$size": "$permissionOverrides"},
					bson.M{"$size": bson.M{"$setUnion": bson.A{bson.M{"$map": bson.M{"input": "$permissionOverrides", "as": "override", "in": "$$override.permission"}}, bson.A{}}}},
				}},
			}},
		},
	}
}

func restaurantSettingsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "restaurant_settings", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "currency", "language", "defaultTimezone", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"},
				"currency":        bson.M{"bsonType": "string", "pattern": `^[A-Z]{3}$`},
				"language":        bson.M{"bsonType": "string", "pattern": `^[a-z]{2}(?:-[A-Z]{2})?$`},
				"defaultTimezone": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 100},
				"version":         bson.M{"bsonType": "long", "minimum": 1},
				"createdAt":       bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
			},
		}},
	}
}

func storageAreasSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "storage_areas", Critical: true, ValidationLevel: "strict",
		Schema: bson.M{"$jsonSchema": bson.M{
			"bsonType": "object", "additionalProperties": false,
			"required": bson.A{"_id", "tenantId", "locationId", "name", "type", "isActive", "version", "createdAt", "updatedAt"},
			"properties": bson.M{
				"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"},
				"name":     bson.M{"bsonType": "string", "minLength": 1, "maxLength": 200, "pattern": `.*\S.*`},
				"type":     bson.M{"bsonType": "string", "enum": bson.A{"refrigerated", "frozen", "bar", "dry", "other"}},
				"isActive": bson.M{"bsonType": "bool"}, "version": bson.M{"bsonType": "long", "minimum": 1},
				"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
				"inventoryFence":     bson.M{"bsonType": "long", "minimum": 0},
				"activeStockCountId": bson.M{"bsonType": "objectId"},
			},
		}},
	}
}

func locationsSchema() CollectionSchema {
	return CollectionSchema{
		Collection:      "locations",
		Critical:        true,
		ValidationLevel: "strict",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType":             "object",
				"additionalProperties": false,
				"required":             bson.A{"_id", "tenantId", "code", "name", "timezone", "isActive", "version", "createdAt", "updatedAt", "limitSlot"},
				"properties": bson.M{
					"_id":       bson.M{"bsonType": "objectId"},
					"tenantId":  bson.M{"bsonType": "objectId"},
					"code":      bson.M{"bsonType": "string", "minLength": 1, "maxLength": 64, "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`},
					"name":      bson.M{"bsonType": "string", "minLength": 1, "maxLength": 200},
					"timezone":  bson.M{"bsonType": "string", "minLength": 1, "maxLength": 100},
					"isActive":  bson.M{"bsonType": "bool"},
					"version":   bson.M{"bsonType": "long", "minimum": 1},
					"createdAt": bson.M{"bsonType": "date"},
					"updatedAt": bson.M{"bsonType": "date"},
					"limitSlot": bson.M{"bsonType": "long", "minimum": 1},
				},
			},
		},
	}
}

// EnsureSchemaValidation applies JSON Schema validators to all validated
// collections using collMod with moderate validation level.
func (m *MongoDB) EnsureSchemaValidation() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, cs := range AllSchemas() {
		// Ensure the collection exists (ignore "already exists" errors).
		_ = m.Database.CreateCollection(ctx, cs.Collection)

		validationLevel := cs.ValidationLevel
		if validationLevel == "" {
			validationLevel = "moderate"
		}
		cmd := bson.D{
			{Key: "collMod", Value: cs.Collection},
			{Key: "validator", Value: cs.Schema},
			{Key: "validationLevel", Value: validationLevel},
			{Key: "validationAction", Value: "error"},
		}

		if err := m.Database.RunCommand(ctx, cmd).Err(); err != nil {
			if cs.Critical {
				return fmt.Errorf("apply schema validation to critical collection %s: %w", cs.Collection, err)
			}
			slog.Warn("failed to apply schema validation", "collection", cs.Collection, "error", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Individual collection schemas
// ---------------------------------------------------------------------------

func usersSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "users",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"email", "displayName", "authMethods", "createdAt", "updatedAt"},
				"properties": bson.M{
					"email": bson.M{
						"bsonType": "string",
					},
					"displayName": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"authMethods": bson.M{
						"bsonType": "array",
						"minItems": 1,
						"items": bson.M{
							"bsonType": "string",
							"enum":     bson.A{"password", "google", "github", "microsoft", "magic_link", "passkey"},
						},
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
					"emailVerified": bson.M{
						"bsonType": "bool",
					},
					"isActive": bson.M{
						"bsonType": "bool",
					},
					"themePreference": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"light", "dark", "system", ""},
					},
				},
			},
		},
	}
}

func tenantsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "tenants",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"name", "slug", "createdAt", "updatedAt"},
				"properties": bson.M{
					"name": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"slug": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 100,
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
					"isRoot": bson.M{
						"bsonType": "bool",
					},
					"isActive": bson.M{
						"bsonType": "bool",
					},
					"billingStatus": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"none", "active", "past_due", "canceled", ""},
					},
					"onboardingCompletedAt": bson.M{
						"bsonType": "date",
					},
					"seatQuantity": bson.M{
						"bsonType": "int",
					},
				},
			},
		},
	}
}

func tenantMembershipsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "tenant_memberships",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"userId", "tenantId", "role", "joinedAt", "updatedAt"},
				"properties": bson.M{
					"userId": bson.M{
						"bsonType": "objectId",
					},
					"tenantId": bson.M{
						"bsonType": "objectId",
					},
					"role": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"owner", "admin", "user"},
					},
					"joinedAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func invitationsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "invitations",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"tenantId", "email", "role", "token", "status", "invitedBy", "expiresAt", "createdAt"},
				"properties": bson.M{
					"tenantId": bson.M{
						"bsonType": "objectId",
					},
					"email": bson.M{
						"bsonType": "string",
					},
					"role": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"owner", "admin", "user"},
					},
					"token": bson.M{
						"bsonType": "string",
					},
					"status": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"pending", "accepted"},
					},
					"invitedBy": bson.M{
						"bsonType": "objectId",
					},
					"expiresAt": bson.M{
						"bsonType": "date",
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func plansSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "plans",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"name", "pricingModel", "creditResetPolicy", "createdAt", "updatedAt"},
				"properties": bson.M{
					"name": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"pricingModel": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"flat", "per_seat"},
					},
					"creditResetPolicy": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"reset", "accrue"},
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
					"monthlyPriceCents": bson.M{
						"bsonType": "long",
						"minimum":  0,
					},
					"annualDiscountPct": bson.M{
						"bsonType": "int",
						"minimum":  0,
						"maximum":  100,
					},
					"trialDays": bson.M{
						"bsonType": "int",
						"minimum":  0,
					},
				},
			},
		},
	}
}

func creditBundlesSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "credit_bundles",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"name", "credits", "priceCents", "createdAt", "updatedAt"},
				"properties": bson.M{
					"name": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"credits": bson.M{
						"bsonType": "long",
						"minimum":  1,
					},
					"priceCents": bson.M{
						"bsonType": "long",
						"minimum":  1,
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func financialTransactionsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "financial_transactions",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"tenantId", "userId", "type", "currency", "invoiceNumber", "createdAt"},
				"properties": bson.M{
					"tenantId": bson.M{
						"bsonType": "objectId",
					},
					"userId": bson.M{
						"bsonType": "objectId",
					},
					"type": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"subscription", "credit_purchase", "refund"},
					},
					"currency": bson.M{
						"bsonType": "string",
					},
					"invoiceNumber": bson.M{
						"bsonType": "string",
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func webhooksSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "webhooks",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"name", "url", "secret", "secretPreview", "events", "createdBy", "createdAt", "updatedAt"},
				"properties": bson.M{
					"name": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 100,
					},
					"url": bson.M{
						"bsonType": "string",
					},
					"secret": bson.M{
						"bsonType": "string",
					},
					"secretPreview": bson.M{
						"bsonType": "string",
					},
					"events": bson.M{
						"bsonType": "array",
						"minItems": 1,
					},
					"createdBy": bson.M{
						"bsonType": "objectId",
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func apiKeysSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "api_keys",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"name", "keyHash", "keyPreview", "authority", "createdBy", "createdAt"},
				"properties": bson.M{
					"name": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 100,
					},
					"keyHash": bson.M{
						"bsonType": "string",
					},
					"keyPreview": bson.M{
						"bsonType": "string",
					},
					"authority": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"admin", "user"},
					},
					"createdBy": bson.M{
						"bsonType": "objectId",
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func configVarsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "config_vars",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"name", "type", "createdAt", "updatedAt"},
				"properties": bson.M{
					"name": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"type": bson.M{
						"bsonType": "string",
						"enum":     bson.A{"string", "numeric", "enum", "template"},
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func announcementsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "announcements",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"title", "body", "createdAt", "updatedAt"},
				"properties": bson.M{
					"title": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"body": bson.M{
						"bsonType":  "string",
						"minLength": 1,
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func customPagesSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "custom_pages",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"slug", "title", "createdAt", "updatedAt"},
				"properties": bson.M{
					"slug": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"title": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func messagesSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "messages",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"userId", "subject", "body", "createdAt"},
				"properties": bson.M{
					"userId": bson.M{
						"bsonType": "objectId",
					},
					"subject": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 200,
					},
					"body": bson.M{
						"bsonType":  "string",
						"minLength": 1,
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func usageEventsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "usage_events",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"tenantId", "userId", "type", "quantity", "createdAt"},
				"properties": bson.M{
					"tenantId": bson.M{
						"bsonType": "objectId",
					},
					"userId": bson.M{
						"bsonType": "objectId",
					},
					"type": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 100,
					},
					"quantity": bson.M{
						"bsonType": "int",
						"minimum":  1,
					},
					"metadata": bson.M{
						"bsonType": "object",
						"additionalProperties": bson.M{
							"bsonType": "string",
						},
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func ssoConnectionsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "sso_connections",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"tenantId", "idpEntityId", "idpSsoUrl", "idpCertificate", "createdAt", "updatedAt"},
				"properties": bson.M{
					"tenantId": bson.M{
						"bsonType": "objectId",
					},
					"idpEntityId": bson.M{
						"bsonType": "string",
					},
					"idpSsoUrl": bson.M{
						"bsonType": "string",
					},
					"idpCertificate": bson.M{
						"bsonType": "string",
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}

func eventDefinitionsSchema() CollectionSchema {
	return CollectionSchema{
		Collection: "event_definitions",
		Schema: bson.M{
			"$jsonSchema": bson.M{
				"bsonType": "object",
				"required": bson.A{"name", "createdAt", "updatedAt"},
				"properties": bson.M{
					"name": bson.M{
						"bsonType":  "string",
						"minLength": 1,
						"maxLength": 128,
					},
					"description": bson.M{
						"bsonType":  "string",
						"maxLength": 256,
					},
					"parentId": bson.M{
						"bsonType": "objectId",
					},
					"createdAt": bson.M{
						"bsonType": "date",
					},
					"updatedAt": bson.M{
						"bsonType": "date",
					},
				},
			},
		},
	}
}
