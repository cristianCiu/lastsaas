package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// StockPosting is the immutable operation header for inventory journal writes.
// Corrections are represented by a new reversal posting; posted documents are
// never edited or deleted by the inventory package.
type StockPosting struct {
	ID                       primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID                 primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID               primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID            primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	UserID                   primitive.ObjectID  `json:"userId" bson:"userId" validate:"required"`
	Type                     StockPostingType    `json:"type" bson:"type" validate:"required,stock_posting_type"`
	IdempotencyKey           string              `json:"idempotencyKey" bson:"idempotencyKey" validate:"required,min=8,max=128"`
	RequestHash              string              `json:"-" bson:"requestHash" validate:"required,len=64,sha256_hex"`
	EffectiveAt              time.Time           `json:"effectiveAt" bson:"effectiveAt" validate:"required"`
	RecordedAt               time.Time           `json:"recordedAt" bson:"recordedAt" validate:"required"`
	Reason                   string              `json:"reason,omitempty" bson:"reason,omitempty" validate:"omitempty,max=500,not_blank"`
	ReversalOf               *primitive.ObjectID `json:"reversalOf,omitempty" bson:"reversalOf,omitempty"`
	DestinationLocationID    *primitive.ObjectID `json:"destinationLocationId,omitempty" bson:"destinationLocationId,omitempty"`
	DestinationStorageAreaID *primitive.ObjectID `json:"destinationStorageAreaId,omitempty" bson:"destinationStorageAreaId,omitempty"`
}

type StockPostingType string

const (
	StockPostingOpeningBalance StockPostingType = "opening_balance"
	StockPostingAdjustment     StockPostingType = "manual_adjustment"
	StockPostingReversal       StockPostingType = "reversal"
	StockPostingTransfer       StockPostingType = "transfer"
	StockPostingWaste          StockPostingType = "waste"
	StockPostingCount          StockPostingType = "stock_count"
)

func ValidStockPostingType(value StockPostingType) bool {
	return value == StockPostingOpeningBalance || value == StockPostingAdjustment || value == StockPostingReversal || value == StockPostingTransfer || value == StockPostingWaste || value == StockPostingCount
}

// StockMovement is an immutable signed fixed-point journal line. Quantities
// are stored as millionths of the item's base unit.
type StockMovement struct {
	ID             primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	PostingID      primitive.ObjectID  `json:"postingId" bson:"postingId" validate:"required"`
	TenantID       primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID     primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID  primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	ItemID         primitive.ObjectID  `json:"itemId" bson:"itemId" validate:"required"`
	LotID          *primitive.ObjectID `json:"lotId,omitempty" bson:"lotId,omitempty"`
	LineNumber     int32               `json:"lineNumber" bson:"lineNumber" validate:"gte=0"`
	QuantityMicros int64               `json:"quantityMicros,string" bson:"quantityMicros" validate:"ne=0,quantity_micros"`
	EffectiveAt    time.Time           `json:"effectiveAt" bson:"effectiveAt" validate:"required"`
	RecordedAt     time.Time           `json:"recordedAt" bson:"recordedAt" validate:"required"`
}

// StockBalance is a disposable materialized projection of StockMovement.
type StockBalance struct {
	ID             primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID       primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID     primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID  primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	ItemID         primitive.ObjectID  `json:"itemId" bson:"itemId" validate:"required"`
	LotID          *primitive.ObjectID `json:"lotId,omitempty" bson:"lotId,omitempty"`
	QuantityMicros int64               `json:"quantityMicros,string" bson:"quantityMicros" validate:"gte=0"`
	UpdatedAt      time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type LotTrackingMode string

const (
	LotTrackingDisabled LotTrackingMode = ""
	LotTrackingOptional LotTrackingMode = "optional"
	LotTrackingRequired LotTrackingMode = "required"
)

func ValidLotTrackingMode(value LotTrackingMode) bool {
	return value == LotTrackingDisabled || value == LotTrackingOptional || value == LotTrackingRequired
}

type LotStatus string

const (
	LotStatusAvailable   LotStatus = "available"
	LotStatusQuarantined LotStatus = "quarantined"
)

func ValidLotStatus(value LotStatus) bool {
	return value == LotStatusAvailable || value == LotStatusQuarantined
}

type StockLot struct {
	ID         primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID   primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	ItemID     primitive.ObjectID `json:"itemId" bson:"itemId" validate:"required"`
	Code       string             `json:"code" bson:"code" validate:"required,not_blank,max=128"`
	ExpiresAt  *time.Time         `json:"expiresAt,omitempty" bson:"expiresAt,omitempty"`
	ReceivedAt time.Time          `json:"receivedAt" bson:"receivedAt" validate:"required"`
	Status     LotStatus          `json:"status" bson:"status" validate:"required,lot_status"`
	CreatedAt  time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt  time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type StockCountStatus string

const (
	StockCountDraft     StockCountStatus = "draft"
	StockCountFrozen    StockCountStatus = "frozen"
	StockCountReviewed  StockCountStatus = "reviewed"
	StockCountPosted    StockCountStatus = "posted"
	StockCountCancelled StockCountStatus = "cancelled"
)

func ValidStockCountStatus(value StockCountStatus) bool {
	return value == StockCountDraft || value == StockCountFrozen || value == StockCountReviewed || value == StockCountPosted || value == StockCountCancelled
}

// StockCount is the mutable workflow record for a physical count. Its
// snapshot and lifecycle metadata are separate from the immutable ledger.
type StockCount struct {
	ID                 primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID           primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID         primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID      primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	CreatedBy          primitive.ObjectID  `json:"createdBy" bson:"createdBy" validate:"required"`
	Status             StockCountStatus    `json:"status" bson:"status" validate:"required,stock_count_status"`
	Version            int64               `json:"version" bson:"version" validate:"gte=1"`
	CutoffAt           *time.Time          `json:"cutoffAt,omitempty" bson:"cutoffAt,omitempty"`
	IdempotencyKey     string              `json:"-" bson:"idempotencyKey" validate:"required,min=8,max=128"`
	RequestHash        string              `json:"-" bson:"requestHash" validate:"required,len=64,sha256_hex"`
	PostIdempotencyKey string              `json:"-" bson:"postIdempotencyKey,omitempty" validate:"omitempty,min=8,max=128"`
	PostRequestHash    string              `json:"-" bson:"postRequestHash,omitempty" validate:"omitempty,len=64,sha256_hex"`
	PostPostingID      *primitive.ObjectID `json:"postPostingId,omitempty" bson:"postPostingId,omitempty"`
	CreatedAt          time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt          time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
	FrozenAt           *time.Time          `json:"frozenAt,omitempty" bson:"frozenAt,omitempty"`
	ReviewedAt         *time.Time          `json:"reviewedAt,omitempty" bson:"reviewedAt,omitempty"`
	PostedAt           *time.Time          `json:"postedAt,omitempty" bson:"postedAt,omitempty"`
	CancelledAt        *time.Time          `json:"cancelledAt,omitempty" bson:"cancelledAt,omitempty"`
}

// StockCountLine is a versioned physical observation against one exact
// balance key. A nil counted quantity means the line has not been recorded.
type StockCountLine struct {
	ID                     primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	CountID                primitive.ObjectID  `json:"countId" bson:"countId" validate:"required"`
	TenantID               primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID             primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID          primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	ItemID                 primitive.ObjectID  `json:"itemId" bson:"itemId" validate:"required"`
	LotID                  *primitive.ObjectID `json:"lotId,omitempty" bson:"lotId,omitempty"`
	SnapshotQuantityMicros int64               `json:"snapshotQuantityMicros,string" bson:"snapshotQuantityMicros" validate:"gte=0,quantity_micros"`
	CountedQuantityMicros  *int64              `json:"countedQuantityMicros,omitempty,string" bson:"countedQuantityMicros,omitempty" validate:"omitempty,gte=0,quantity_micros"`
	Version                int64               `json:"version" bson:"version" validate:"gte=1"`
	Note                   string              `json:"note,omitempty" bson:"note,omitempty" validate:"omitempty,max=500,not_blank"`
	CreatedAt              time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt              time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type ReconciliationStatus string

const (
	ReconciliationDetected ReconciliationStatus = "detected"
	ReconciliationRepaired ReconciliationStatus = "repaired"
)

func ValidReconciliationStatus(value ReconciliationStatus) bool {
	return value == ReconciliationDetected || value == ReconciliationRepaired
}

type ReconciliationMismatch struct {
	LocationID             primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID          primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	ItemID                 primitive.ObjectID  `json:"itemId" bson:"itemId" validate:"required"`
	LotID                  *primitive.ObjectID `json:"lotId,omitempty" bson:"lotId,omitempty"`
	ExpectedQuantityMicros int64               `json:"expectedQuantityMicros,string" bson:"expectedQuantityMicros" validate:"quantity_micros"`
	ActualQuantityMicros   int64               `json:"actualQuantityMicros,string" bson:"actualQuantityMicros" validate:"quantity_micros"`
	DeltaMicros            int64               `json:"deltaMicros,string" bson:"deltaMicros" validate:"quantity_micros"`
}

type ReconciliationRun struct {
	ID             primitive.ObjectID       `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID       primitive.ObjectID       `json:"-" bson:"tenantId" validate:"required"`
	UserID         primitive.ObjectID       `json:"userId" bson:"userId" validate:"required"`
	LocationID     primitive.ObjectID       `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID  *primitive.ObjectID      `json:"storageAreaId,omitempty" bson:"storageAreaId,omitempty"`
	CutoffAt       time.Time                `json:"cutoffAt" bson:"cutoffAt" validate:"required"`
	IdempotencyKey string                   `json:"-" bson:"idempotencyKey" validate:"required,min=8,max=128"`
	RequestHash    string                   `json:"-" bson:"requestHash" validate:"required,len=64,sha256_hex"`
	Status         ReconciliationStatus     `json:"status" bson:"status" validate:"required,reconciliation_status"`
	MismatchCount  int64                    `json:"mismatchCount" bson:"mismatchCount" validate:"gte=0"`
	Truncated      bool                     `json:"truncated" bson:"truncated"`
	Mismatches     []ReconciliationMismatch `json:"mismatches" bson:"mismatches" validate:"max=1000,dive"`
	CreatedAt      time.Time                `json:"createdAt" bson:"createdAt" validate:"required"`
	CompletedAt    time.Time                `json:"completedAt" bson:"completedAt" validate:"required"`
}
