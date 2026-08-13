package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ImportTarget string

const (
	ImportTargetUnits         ImportTarget = "units"
	ImportTargetCategories    ImportTarget = "categories"
	ImportTargetItems         ImportTarget = "items"
	ImportTargetSuppliers     ImportTarget = "suppliers"
	ImportTargetSupplierItems ImportTarget = "supplier_items"
)

func ValidImportTarget(target ImportTarget) bool {
	switch target {
	case ImportTargetUnits, ImportTargetCategories, ImportTargetItems, ImportTargetSuppliers, ImportTargetSupplierItems:
		return true
	default:
		return false
	}
}

type ImportRunStatus string

const (
	ImportRunPending   ImportRunStatus = "pending"
	ImportRunCompleted ImportRunStatus = "completed"
	ImportRunFailed    ImportRunStatus = "failed"
)

type ImportRowError struct {
	Row     int    `json:"row" bson:"row" validate:"gte=2"`
	Field   string `json:"field" bson:"field" validate:"required,max=64"`
	Code    string `json:"code" bson:"code" validate:"required,max=64"`
	Message string `json:"message" bson:"message" validate:"required,max=240"`
}

type ImportRun struct {
	ID             primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID       primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	UserID         primitive.ObjectID `json:"-" bson:"userId" validate:"required"`
	Target         ImportTarget       `json:"target" bson:"target" validate:"required,import_target"`
	IdempotencyKey string             `json:"-" bson:"idempotencyKey" validate:"required,min=8,max=128"`
	Status         ImportRunStatus    `json:"status" bson:"status" validate:"required,import_run_status"`
	TotalRows      int32              `json:"totalRows" bson:"totalRows" validate:"gte=0,lte=5000"`
	CreatedRows    int32              `json:"createdRows" bson:"createdRows" validate:"gte=0,lte=5000"`
	UpdatedRows    int32              `json:"updatedRows" bson:"updatedRows" validate:"gte=0,lte=5000"`
	Errors         []ImportRowError   `json:"errors" bson:"errors" validate:"required,max=100,dive"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt      time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
