package db

import "go.mongodb.org/mongo-driver/bson"

func forecastDatasetSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_datasets", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "name", "source", "status", "effectiveFrom", "effectiveTo", "rowCount", "createdBy", "version", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"},
			"name": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160}, "source": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64},
			"status": bson.M{"bsonType": "string", "enum": bson.A{"draft", "sealed"}}, "effectiveFrom": bson.M{"bsonType": "date"}, "effectiveTo": bson.M{"bsonType": "date"}, "cutoffAt": bson.M{"bsonType": "date"}, "manifestVersion": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64},
			"rowCount": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(10000000)}, "contentHash": bson.M{"bsonType": "string", "pattern": `^[0-9a-f]{64}$`},
			"sealedBy": bson.M{"bsonType": "objectId"}, "sealedAt": bson.M{"bsonType": "date"}, "createdBy": bson.M{"bsonType": "objectId"},
			"version": bson.M{"bsonType": "long", "minimum": int64(1)}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}, "$expr": bson.M{"$and": bson.A{
		bson.M{"$gt": bson.A{"$effectiveTo", "$effectiveFrom"}},
		bson.M{"$or": bson.A{
			bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$status", "draft"}}, bson.M{"$eq": bson.A{bson.M{"$type": "$contentHash"}, "missing"}}, bson.M{"$eq": bson.A{bson.M{"$type": "$sealedAt"}, "missing"}}, bson.M{"$eq": bson.A{bson.M{"$type": "$sealedBy"}, "missing"}}}},
			bson.M{"$and": bson.A{bson.M{"$eq": bson.A{"$status", "sealed"}}, bson.M{"$eq": bson.A{bson.M{"$type": "$contentHash"}, "string"}}, bson.M{"$ne": bson.A{bson.M{"$type": "$sealedAt"}, "missing"}}, bson.M{"$ne": bson.A{bson.M{"$type": "$sealedBy"}, "missing"}}}},
		}},
	}}}}
}

func forecastInputRowsSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_input_rows", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "datasetId", "rowNumber", "kind", "quantityMicros", "effectiveAt", "recordedAt", "createdAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "datasetId": bson.M{"bsonType": "objectId"},
			"rowNumber": bson.M{"bsonType": "long", "minimum": int64(1), "maximum": int64(10000000)}, "kind": bson.M{"bsonType": "string", "enum": bson.A{"demand", "guest_plan", "confirmed_inbound"}},
			"itemId": bson.M{"bsonType": "objectId"}, "servicePeriod": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "sourceIds": bson.M{"bsonType": "array", "maxItems": int32(1000), "items": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 128}}, "quantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "guestCount": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(1000000000)},
			"effectiveAt": bson.M{"bsonType": "date"}, "recordedAt": bson.M{"bsonType": "date"}, "createdAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func forecastJobsSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_jobs", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "datasetId", "status", "idempotencyKey", "attempt", "maxAttempts", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "datasetId": bson.M{"bsonType": "objectId"}, "policyId": bson.M{"bsonType": "objectId"}, "runId": bson.M{"bsonType": "objectId"},
			"status": bson.M{"bsonType": "string", "enum": bson.A{"queued", "running", "retry_wait", "succeeded", "failed", "dead_letter", "cancelled"}}, "idempotencyKey": bson.M{"bsonType": "string", "minLength": 8, "maxLength": 128}, "owner": bson.M{"bsonType": "string", "minLength": 1, "maxLength": 128}, "leaseToken": bson.M{"bsonType": "string", "minLength": 16, "maxLength": 256}, "leaseExpiresAt": bson.M{"bsonType": "date"}, "heartbeatAt": bson.M{"bsonType": "date"}, "nextAttemptAt": bson.M{"bsonType": "date"},
			"attempt": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(100)}, "maxAttempts": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(100)}, "lastError": bson.M{"bsonType": "string", "maxLength": 2000},
			"createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func forecastRunsSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_runs", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "datasetId", "policyId", "status", "algorithm", "algorithmVersion", "createdAt", "updatedAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "datasetId": bson.M{"bsonType": "objectId"}, "policyId": bson.M{"bsonType": "objectId"},
			"status": bson.M{"bsonType": "string", "enum": bson.A{"pending", "running", "succeeded", "failed"}}, "algorithm": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "algorithmVersion": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64},
			"parameters": bson.M{"bsonType": "object", "additionalProperties": bson.M{"bsonType": "string"}}, "metrics": bson.M{"bsonType": "object", "additionalProperties": bson.M{"bsonType": "long"}},
			"startedAt": bson.M{"bsonType": "date"}, "completedAt": bson.M{"bsonType": "date"}, "error": bson.M{"bsonType": "string", "maxLength": 2000}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func forecastPointsSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_points", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required":   bson.A{"_id", "tenantId", "locationId", "runId", "itemId", "targetDate", "forecastMicros", "lowerMicros", "upperMicros", "createdAt"},
		"properties": bson.M{"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "runId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"}, "modelId": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "targetDate": bson.M{"bsonType": "date"}, "forecastMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "lowerMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "upperMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "createdAt": bson.M{"bsonType": "date"}},
	}}}
}

func forecastMetricsSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_metrics", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required":   bson.A{"_id", "tenantId", "locationId", "runId", "name", "value", "createdAt"},
		"properties": bson.M{"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "runId": bson.M{"bsonType": "objectId"}, "name": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 96}, "value": bson.M{"bsonType": "long"}, "createdAt": bson.M{"bsonType": "date"}},
	}}}
}

func forecastPoliciesSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_policies", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false, "required": bson.A{"_id", "tenantId", "locationId", "name", "horizonDays", "lookbackDays", "safetyStockDays", "isActive", "version", "createdBy", "createdAt", "updatedAt"},
		"properties": bson.M{"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "name": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 160}, "horizonDays": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(365)}, "lookbackDays": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(3650)}, "safetyStockDays": bson.M{"bsonType": "int", "minimum": int32(0), "maximum": int32(365)}, "isActive": bson.M{"bsonType": "bool"}, "version": bson.M{"bsonType": "long", "minimum": int64(1)}, "createdBy": bson.M{"bsonType": "objectId"}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"}},
	}}}
}

func guestPlansSchema() CollectionSchema {
	return CollectionSchema{Collection: "guest_plans", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false, "required": bson.A{"_id", "tenantId", "locationId", "planDate", "servicePeriod", "guestCount", "source", "isActual", "createdBy", "version", "createdAt", "updatedAt"},
		"properties": bson.M{"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "planDate": bson.M{"bsonType": "date"}, "servicePeriod": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "guestCount": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(1000000000)}, "source": bson.M{"bsonType": "string", "enum": bson.A{"manual"}}, "isActual": bson.M{"bsonType": "bool"}, "notes": bson.M{"bsonType": "string", "maxLength": 1000}, "createdBy": bson.M{"bsonType": "objectId"}, "version": bson.M{"bsonType": "long", "minimum": int64(1)}, "createdAt": bson.M{"bsonType": "date"}, "updatedAt": bson.M{"bsonType": "date"}},
	},
		"$expr": bson.M{"$eq": bson.A{"$isActual", false}},
	}}
}

func forecastOverridesSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_overrides", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{"bsonType": "object", "additionalProperties": false, "required": bson.A{"_id", "tenantId", "locationId", "itemId", "targetDate", "quantityMicros", "reason", "createdBy", "createdAt"}, "properties": bson.M{"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"}, "targetDate": bson.M{"bsonType": "date"}, "quantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "reason": bson.M{"bsonType": "string", "pattern": `.*\S.*`, "maxLength": 500}, "createdBy": bson.M{"bsonType": "objectId"}, "createdAt": bson.M{"bsonType": "date"}}}}}
}

func reorderRecommendationsSchema() CollectionSchema {
	return CollectionSchema{Collection: "reorder_recommendations", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "runId", "datasetId", "policyId", "itemId", "modelId", "algorithmVersion", "horizonDays", "p90DemandMicros", "safetyDemandMicros", "usableStockMicros", "confirmedInboundMicros", "requestedQuantityMicros", "roundedQuantityMicros", "quantityMicros", "packSizeMicros", "moq", "packMoqDeltaMicros", "status", "observedDays", "expectedDays", "coveragePermille", "maturity", "formula", "reasonCodes", "createdAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "runId": bson.M{"bsonType": "objectId"}, "datasetId": bson.M{"bsonType": "objectId"}, "policyId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"},
			"modelId": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "algorithmVersion": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "horizonDays": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(365)},
			"p90DemandMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "safetyDemandMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "usableStockMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "confirmedInboundMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "requestedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "roundedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "quantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "packSizeMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "moq": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(1000000)}, "packMoqDeltaMicros": bson.M{"bsonType": "long", "minimum": int64(0)},
			"status": bson.M{"bsonType": "string", "enum": bson.A{"ready", "no-reorder-needed"}}, "qualityFlags": bson.M{"bsonType": "array", "maxItems": int32(32), "items": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(64)}}, "observedDays": bson.M{"bsonType": "int", "minimum": int32(0)}, "expectedDays": bson.M{"bsonType": "int", "minimum": int32(0)}, "coveragePermille": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(1000)}, "maturity": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(32)}, "formula": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(1000)}, "reasonCodes": bson.M{"bsonType": "array", "minItems": int32(1), "maxItems": int32(32), "items": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(64)}}, "inboundSourceIds": bson.M{"bsonType": "array", "maxItems": int32(1000), "items": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(128)}}, "createdAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func forecastCoveragesSchema() CollectionSchema {
	return CollectionSchema{Collection: "forecast_coverages", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "runId", "datasetId", "policyId", "itemId", "modelId", "algorithmVersion", "horizonDays", "p90DemandMicros", "safetyDemandMicros", "usableStockMicros", "confirmedInboundMicros", "projectedAvailableMicros", "requestedQuantityMicros", "roundedQuantityMicros", "coverageDays", "observedDays", "expectedDays", "coveragePermille", "maturity", "formula", "reasonCodes", "createdAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "runId": bson.M{"bsonType": "objectId"}, "datasetId": bson.M{"bsonType": "objectId"}, "policyId": bson.M{"bsonType": "objectId"}, "itemId": bson.M{"bsonType": "objectId"}, "modelId": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "algorithmVersion": bson.M{"bsonType": "string", "pattern": `^[a-z0-9]+(?:-[a-z0-9]+)*$`, "maxLength": 64}, "horizonDays": bson.M{"bsonType": "int", "minimum": int32(1), "maximum": int32(365)},
			"p90DemandMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "safetyDemandMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "usableStockMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "confirmedInboundMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "projectedAvailableMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "requestedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "roundedQuantityMicros": bson.M{"bsonType": "long", "minimum": int64(0)}, "coverageDays": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(100000)},
			"qualityFlags": bson.M{"bsonType": "array", "maxItems": int32(32), "items": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(64)}}, "observedDays": bson.M{"bsonType": "int", "minimum": int32(0)}, "expectedDays": bson.M{"bsonType": "int", "minimum": int32(0)}, "coveragePermille": bson.M{"bsonType": "long", "minimum": int64(0), "maximum": int64(1000)}, "maturity": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(32)}, "formula": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(1000)}, "reasonCodes": bson.M{"bsonType": "array", "minItems": int32(1), "maxItems": int32(32), "items": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(64)}}, "inboundSourceIds": bson.M{"bsonType": "array", "maxItems": int32(1000), "items": bson.M{"bsonType": "string", "minLength": int32(1), "maxLength": int32(128)}}, "createdAt": bson.M{"bsonType": "date"},
		},
	}}}
}

func shadowKPIReportsSchema() CollectionSchema {
	return CollectionSchema{Collection: "shadow_kpi_reports", Critical: true, ValidationLevel: "strict", Schema: bson.M{"$jsonSchema": bson.M{
		"bsonType": "object", "additionalProperties": false,
		"required": bson.A{"_id", "tenantId", "locationId", "runId", "evaluationStart", "evaluationEnd", "metrics", "createdBy", "createdAt"},
		"properties": bson.M{
			"_id": bson.M{"bsonType": "objectId"}, "tenantId": bson.M{"bsonType": "objectId"}, "locationId": bson.M{"bsonType": "objectId"}, "runId": bson.M{"bsonType": "objectId"},
			"evaluationStart": bson.M{"bsonType": "date"}, "evaluationEnd": bson.M{"bsonType": "date"}, "actualsThrough": bson.M{"bsonType": "date"},
			"metrics":   bson.M{"bsonType": "object", "minProperties": int32(1), "maxProperties": int32(64), "additionalProperties": bson.M{"bsonType": "double"}},
			"createdBy": bson.M{"bsonType": "objectId"}, "createdAt": bson.M{"bsonType": "date"},
		},
	}, "$expr": bson.M{"$gt": bson.A{"$evaluationEnd", "$evaluationStart"}}}}
}
