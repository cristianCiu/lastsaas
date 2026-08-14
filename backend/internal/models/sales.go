package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SaleStatus string

const (
	SaleStatusCompleted SaleStatus = "completed"
	SaleStatusCancelled SaleStatus = "cancelled"
)

func ValidSaleStatus(status SaleStatus) bool {
	return status == SaleStatusCompleted || status == SaleStatusCancelled
}

type SalesImportStatus string

const (
	SalesImportPending   SalesImportStatus = "pending"
	SalesImportCompleted SalesImportStatus = "completed"
	SalesImportFailed    SalesImportStatus = "failed"
)

func ValidSalesImportStatus(status SalesImportStatus) bool {
	return status == SalesImportPending || status == SalesImportCompleted || status == SalesImportFailed
}

// Sale is the immutable normalized sale header. Cancellation is represented
// by a compensating inventory posting and never by changing journal history.
type Sale struct {
	ID                   primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID             primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	Source               string              `json:"source" bson:"source" validate:"required,max=64,location_code"`
	ExternalSaleID       string              `json:"externalSaleId" bson:"externalSaleId" validate:"required,not_blank,max=128"`
	OccurredAt           time.Time           `json:"occurredAt" bson:"occurredAt" validate:"required"`
	LocationID           primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID        primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	Status               SaleStatus          `json:"status" bson:"status" validate:"required,sale_status"`
	CancelledAt          *time.Time          `json:"cancelledAt,omitempty" bson:"cancelledAt,omitempty"`
	ConsumptionPostingID *primitive.ObjectID `json:"consumptionPostingId,omitempty" bson:"consumptionPostingId,omitempty"`
	ReversalPostingID    *primitive.ObjectID `json:"reversalPostingId,omitempty" bson:"reversalPostingId,omitempty"`
	Version              int64               `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt            time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt            time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type SaleLine struct {
	ID                  primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID            primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	SaleID              primitive.ObjectID  `json:"saleId" bson:"saleId" validate:"required"`
	Source              string              `json:"source" bson:"source" validate:"required,max=64,location_code"`
	ExternalSaleID      string              `json:"externalSaleId" bson:"externalSaleId" validate:"required,not_blank,max=128"`
	ExternalLineID      string              `json:"externalLineId" bson:"externalLineId" validate:"required,not_blank,max=128"`
	ExternalProductID   string              `json:"externalProductId" bson:"externalProductId" validate:"required,not_blank,max=256"`
	QuantityMicros      int64               `json:"quantityMicros,string" bson:"quantityMicros" validate:"gt=0"`
	MappingID           *primitive.ObjectID `json:"mappingId,omitempty" bson:"mappingId,omitempty"`
	MappingVersion      int64               `json:"mappingVersion,omitempty" bson:"mappingVersion,omitempty" validate:"omitempty,gte=1"`
	RecipeVersionID     *primitive.ObjectID `json:"recipeVersionId,omitempty" bson:"recipeVersionId,omitempty"`
	RecipeVersionNumber int64               `json:"recipeVersionNumber,omitempty" bson:"recipeVersionNumber,omitempty" validate:"omitempty,gte=1"`
	CreatedAt           time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt           time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type UnresolvedSaleLine struct {
	ID                primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID          primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	ImportRunID       primitive.ObjectID  `json:"importRunId" bson:"importRunId" validate:"required"`
	Source            string              `json:"source" bson:"source" validate:"required,max=64,location_code"`
	ExternalSaleID    string              `json:"externalSaleId" bson:"externalSaleId" validate:"required,not_blank,max=128"`
	ExternalLineID    string              `json:"externalLineId" bson:"externalLineId" validate:"required,not_blank,max=128"`
	ExternalProductID string              `json:"externalProductId" bson:"externalProductId" validate:"required,not_blank,max=256"`
	OccurredAt        time.Time           `json:"occurredAt" bson:"occurredAt" validate:"required"`
	LocationID        primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID     primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	QuantityMicros    int64               `json:"quantityMicros,string" bson:"quantityMicros" validate:"gt=0"`
	MappingID         *primitive.ObjectID `json:"mappingId,omitempty" bson:"mappingId,omitempty"`
	RecipeVersionID   *primitive.ObjectID `json:"recipeVersionId,omitempty" bson:"recipeVersionId,omitempty"`
	Reason            string              `json:"reason" bson:"reason" validate:"required,not_blank,max=240"`
	CreatedAt         time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
}

type SalesImportRun struct {
	ID             primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID       primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	UserID         primitive.ObjectID `json:"-" bson:"userId" validate:"required"`
	Source         string             `json:"source" bson:"source" validate:"required,max=64,location_code"`
	IdempotencyKey string             `json:"-" bson:"idempotencyKey" validate:"required,min=8,max=128"`
	Status         SalesImportStatus  `json:"status" bson:"status" validate:"required,sales_import_status"`
	TotalRows      int32              `json:"totalRows" bson:"totalRows" validate:"gte=0,lte=5000"`
	CreatedSales   int32              `json:"createdSales" bson:"createdSales" validate:"gte=0,lte=5000"`
	CreatedLines   int32              `json:"createdLines" bson:"createdLines" validate:"gte=0,lte=5000"`
	UnresolvedRows int32              `json:"unresolvedRows" bson:"unresolvedRows" validate:"gte=0,lte=5000"`
	Errors         []ImportRowError   `json:"errors" bson:"errors" validate:"required,max=100,dive"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt      time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
