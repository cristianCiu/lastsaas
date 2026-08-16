package db

import "go.mongodb.org/mongo-driver/bson"

func purchaseOrdersSchema() CollectionSchema {
	return CollectionSchema{Collection: "purchase_orders", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "supplierId", "orderNumber", "status", "deliveryDate", "createdBy", "audit", "version", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "supplierId": bson.M{"bsonType": "objectId"},
			"orderNumber":  bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64},
			"status":       bson.M{"bsonType": "string", "enum": bson.A{"draft", "submitted", "approved", "supplier_confirmed", "ordered", "partially_received", "received", "cancelled"}},
			"deliveryDate": bson.M{"bsonType": "date"}, "notes": bson.M{"bsonType": "string", "maxLength": 2000},
			"createdBy": bson.M{"bsonType": "objectId"}, "submittedBy": bson.M{"bsonType": "objectId"}, "submittedAt": bson.M{"bsonType": "date"},
			"approvedBy": bson.M{"bsonType": "objectId"}, "approvedAt": bson.M{"bsonType": "date"}, "supplierConfirmedBy": bson.M{"bsonType": "objectId"}, "supplierConfirmedAt": bson.M{"bsonType": "date"}, "cancelledBy": bson.M{"bsonType": "objectId"}, "cancelledAt": bson.M{"bsonType": "date"},
			"approvalNote": bson.M{"bsonType": "string", "maxLength": 500}, "reorderRecommendationId": bson.M{"bsonType": "objectId"}, "idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "requestHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`}, "version": bson.M{"bsonType": "long", "minimum": int64(1)},
			"audit":     bson.M{"bsonType": "array", "maxItems": int32(100), "items": bson.M{"bsonType": "object", "additionalProperties": false, "required": bson.A{"action", "userId", "at"}, "properties": bson.M{"action": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`}, "userId": bson.M{"bsonType": "objectId"}, "at": bson.M{"bsonType": "date"}, "note": bson.M{"bsonType": "string", "maxLength": 500}}}},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
		"$expr": bson.M{"$and": bson.A{
			bson.M{"$or": bson.A{
				bson.M{"$ne": bson.A{"$status", "supplier_confirmed"}},
				bson.M{"$and": bson.A{bson.M{"$ne": bson.A{bson.M{"$type": "$supplierConfirmedBy"}, "missing"}}, bson.M{"$ne": bson.A{bson.M{"$type": "$supplierConfirmedAt"}, "missing"}}}},
			}},
			bson.M{"$or": bson.A{
				bson.M{"$and": bson.A{bson.M{"$eq": bson.A{bson.M{"$type": "$reorderRecommendationId"}, "missing"}}, bson.M{"$eq": bson.A{bson.M{"$type": "$idempotencyKey"}, "missing"}}, bson.M{"$eq": bson.A{bson.M{"$type": "$requestHash"}, "missing"}}}},
				bson.M{"$and": bson.A{bson.M{"$ne": bson.A{bson.M{"$type": "$reorderRecommendationId"}, "missing"}}, bson.M{"$ne": bson.A{bson.M{"$type": "$idempotencyKey"}, "missing"}}, bson.M{"$ne": bson.A{bson.M{"$type": "$requestHash"}, "missing"}}}},
			}},
		}},
	}}}
}

func purchaseOrderLinesSchema() CollectionSchema {
	return CollectionSchema{Collection: "purchase_order_lines", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "purchaseOrderId", "locationId", "lineNumber", "supplierItemId", "supplierItemVersion", "supplierCode", "supplierName", "itemId", "itemSku", "itemName", "supplierItemCode", "description", "requestedQuantityMicros", "orderedQuantityMicros", "receivedQuantityMicros", "packSizeMicros", "moq", "orderedPacks", "roundingDeltaMicros", "unitPriceMinor", "lineTotalMinor", "currency", "itemBaseUnitId", "itemBaseUnitCode", "orderedUnitId", "orderedUnitCode", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "purchaseOrderId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "lineNumber": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(1000)},
			"supplierItemId": bson.M{"bsonType": "objectId"}, "supplierItemVersion": bson.M{"bsonType": "long", "minimum": int64(1)}, "supplierCode": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "supplierName": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160}, "itemId": bson.M{"bsonType": "objectId"},
			"itemSku": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 64}, "itemName": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160}, "supplierItemCode": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 100}, "description": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 2000},
			"requestedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(1)}, "orderedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(1)}, "receivedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "packSizeMicros": bson.M{"bsonType": "long", "minimum": int64(1)}, "moq": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(1000000)}, "orderedPacks": bson.M{"bsonType": "long", "minimum": int64(1)}, "roundingDeltaMicros": bson.M{"bsonType": "long", "minimum": int64(0)},
			"unitPriceMinor": bson.M{"bsonType": "long", "minimum": int64(0)}, "lineTotalMinor": bson.M{"bsonType": "long", "minimum": int64(0)}, "currency": bson.M{"bsonType": "string", "pattern": `^[A-Z]{3}$`},
			"itemBaseUnitId": bson.M{"bsonType": "objectId"}, "itemBaseUnitCode": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 32}, "orderedUnitId": bson.M{"bsonType": "objectId"}, "orderedUnitCode": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 32},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func deliveryCalendarsSchema() CollectionSchema {
	return CollectionSchema{Collection: "delivery_calendars", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "supplierId", "weekday", "cutoffMinutes", "leadTimeDays", "isActive", "version", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "supplierId": bson.M{"bsonType": "objectId"},
			"weekday": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(7)}, "cutoffMinutes": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(1439)}, "leadTimeDays": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(3650)},
			"isActive": bson.M{"bsonType": "bool"}, "version": bson.M{"bsonType": "long", "minimum": int64(1)}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func goodsReceiptsSchema() CollectionSchema {
	return CollectionSchema{Collection: "goods_receipts", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "storageAreaId", "purchaseOrderId", "orderNumberSnapshot", "orderVersionSnapshot", "supplierId", "receivedBy", "receivedAt", "status", "audit", "idempotencyKey", "requestHash", "postingId", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "storageAreaId": bson.M{"bsonType": "objectId"}, "purchaseOrderId": bson.M{"bsonType": "objectId"},
			"orderNumberSnapshot": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "orderVersionSnapshot": bson.M{"bsonType": "long", "minimum": int64(1)}, "supplierId": bson.M{"bsonType": "objectId"}, "receivedBy": bson.M{"bsonType": "objectId"}, "receivedAt": bson.M{"bsonType": "date"},
			"status": bson.M{"bsonType": "string", "enum": bson.A{"posted", "cancelled"}}, "idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "requestHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`},
			"audit":     bson.M{"bsonType": "array", "maxItems": int32(100), "items": bson.M{"bsonType": "object", "additionalProperties": false, "required": bson.A{"action", "userId", "at"}, "properties": bson.M{"action": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`}, "userId": bson.M{"bsonType": "objectId"}, "at": bson.M{"bsonType": "date"}, "note": bson.M{"bsonType": "string", "maxLength": 500}}}},
			"postingId": bson.M{"bsonType": "objectId"}, "reversalPostingId": bson.M{"bsonType": "objectId"}, "reversalIdempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "reversalRequestHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func goodsReceiptLinesSchema() CollectionSchema {
	return CollectionSchema{Collection: "goods_receipt_lines", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "goodsReceiptId", "purchaseOrderId", "purchaseOrderLineId", "lineNumber", "itemId", "itemSku", "itemName", "description", "supplierCode", "supplierName", "supplierItemCode", "supplierItemVersion", "itemBaseUnitCode", "orderedUnitCode", "orderedQuantityMicros", "previouslyReceivedMicros", "receivedQuantityMicros", "quantityVarianceMicros", "orderedUnitPriceMinor", "actualUnitPriceMinor", "priceVarianceMinor", "currency", "storageAreaId", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "goodsReceiptId": bson.M{"bsonType": "objectId"}, "purchaseOrderId": bson.M{"bsonType": "objectId"}, "purchaseOrderLineId": bson.M{"bsonType": "objectId"}, "lineNumber": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(1000)},
			"itemId": bson.M{"bsonType": "objectId"}, "itemSku": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 64}, "itemName": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160}, "description": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 2000}, "supplierCode": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "supplierName": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160}, "supplierItemCode": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 100}, "supplierItemVersion": bson.M{"bsonType": "long", "minimum": int64(1)}, "itemBaseUnitCode": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 32}, "orderedUnitCode": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 32},
			"orderedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(1)}, "previouslyReceivedMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "receivedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(1)}, "quantityVarianceMicros": bson.M{"bsonType": "long"},
			"orderedUnitPriceMinor": bson.M{"bsonType": "long", "minimum": int64(0)}, "actualUnitPriceMinor": bson.M{"bsonType": "long", "minimum": int64(0)}, "priceVarianceMinor": bson.M{"bsonType": "long"}, "currency": bson.M{"bsonType": "string", "pattern": `^[A-Z]{3}$`},
			"varianceReason": bson.M{"bsonType": "string", "maxLength": 500, "pattern": `.*\S.*`}, "storageAreaId": bson.M{"bsonType": "objectId"}, "lotCode": bson.M{"bsonType": "string", "maxLength": 128, "pattern": `.*\S.*`}, "lotId": bson.M{"bsonType": "objectId"}, "expiresAt": bson.M{"bsonType": "date"}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func purchaseOrderEmailDeliveriesSchema() CollectionSchema {
	return CollectionSchema{Collection: "purchase_order_email_deliveries", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "purchaseOrderId", "orderVersion", "recipientEmail", "idempotencyKey", "requestHash", "status", "claimedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "purchaseOrderId": bson.M{"bsonType": "objectId"},
			"orderVersion":   bson.M{"bsonType": "long", "minimum": int64(1)},
			"recipientEmail": bson.M{"bsonType": "string", "maxLength": 254, "pattern": `^[^@\s]+@[^@\s]+\.[^@\s]+$`},
			"idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "requestHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`},
			"status":    bson.M{"bsonType": "string", "enum": bson.A{"pending", "sent"}},
			"claimedAt": bson.M{"bsonType": "date"}, "sentAt": bson.M{"bsonType": "date"},
		},
		"$expr": bson.M{"$or": bson.A{
			bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$status", "sent"}}, bson.M{"$ne": bson.A{bson.M{"$type": "$sentAt"}, "missing"}}}},
			bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$status", "pending"}}, bson.M{"$eq": bson.A{bson.M{"$type": "$sentAt"}, "missing"}}}},
		}},
	}}}
}
