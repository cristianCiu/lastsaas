package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PurchaseOrderStatus deliberately includes the receipt-facing states even
// though receipt posting is owned by a later lane. Approved lines are
// immutable snapshots; future receipt code consumes these snapshots.
type PurchaseOrderStatus string

const (
	PurchaseOrderDraft             PurchaseOrderStatus = "draft"
	PurchaseOrderSubmitted         PurchaseOrderStatus = "submitted"
	PurchaseOrderApproved          PurchaseOrderStatus = "approved"
	PurchaseOrderOrdered           PurchaseOrderStatus = "ordered"
	PurchaseOrderPartiallyReceived PurchaseOrderStatus = "partially_received"
	PurchaseOrderReceived          PurchaseOrderStatus = "received"
	PurchaseOrderCancelled         PurchaseOrderStatus = "cancelled"
)

func ValidPurchaseOrderStatus(status PurchaseOrderStatus) bool {
	switch status {
	case PurchaseOrderDraft, PurchaseOrderSubmitted, PurchaseOrderApproved,
		PurchaseOrderOrdered, PurchaseOrderPartiallyReceived, PurchaseOrderReceived,
		PurchaseOrderCancelled:
		return true
	default:
		return false
	}
}

type PurchaseOrderAuditEntry struct {
	Action string             `json:"action" bson:"action" validate:"required,location_code"`
	UserID primitive.ObjectID `json:"userId" bson:"userId" validate:"required"`
	At     time.Time          `json:"at" bson:"at" validate:"required"`
	Note   string             `json:"note,omitempty" bson:"note,omitempty" validate:"omitempty,max=500"`
}

type PurchaseOrder struct {
	ID           primitive.ObjectID        `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID     primitive.ObjectID        `json:"-" bson:"tenantId" validate:"required"`
	LocationID   primitive.ObjectID        `json:"locationId" bson:"locationId" validate:"required"`
	SupplierID   primitive.ObjectID        `json:"supplierId" bson:"supplierId" validate:"required"`
	OrderNumber  string                    `json:"orderNumber" bson:"orderNumber" validate:"required,location_code,max=64"`
	Status       PurchaseOrderStatus       `json:"status" bson:"status" validate:"required,purchase_order_status"`
	DeliveryDate time.Time                 `json:"deliveryDate" bson:"deliveryDate" validate:"required"`
	Notes        string                    `json:"notes,omitempty" bson:"notes,omitempty" validate:"omitempty,max=2000"`
	CreatedBy    primitive.ObjectID        `json:"createdBy" bson:"createdBy" validate:"required"`
	SubmittedBy  *primitive.ObjectID       `json:"submittedBy,omitempty" bson:"submittedBy,omitempty"`
	SubmittedAt  *time.Time                `json:"submittedAt,omitempty" bson:"submittedAt,omitempty"`
	ApprovedBy   *primitive.ObjectID       `json:"approvedBy,omitempty" bson:"approvedBy,omitempty"`
	ApprovedAt   *time.Time                `json:"approvedAt,omitempty" bson:"approvedAt,omitempty"`
	CancelledBy  *primitive.ObjectID       `json:"cancelledBy,omitempty" bson:"cancelledBy,omitempty"`
	CancelledAt  *time.Time                `json:"cancelledAt,omitempty" bson:"cancelledAt,omitempty"`
	ApprovalNote string                    `json:"approvalNote,omitempty" bson:"approvalNote,omitempty" validate:"omitempty,max=500"`
	Audit        []PurchaseOrderAuditEntry `json:"audit" bson:"audit" validate:"required,max=100,dive"`
	Version      int64                     `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt    time.Time                 `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt    time.Time                 `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

// PurchaseOrderLine is a historical supplier-term snapshot. It must not be
// rebuilt from mutable catalog records after submission/approval.
type PurchaseOrderLine struct {
	ID                      primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID                primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	PurchaseOrderID         primitive.ObjectID `json:"purchaseOrderId" bson:"purchaseOrderId" validate:"required"`
	LocationID              primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	LineNumber              int32              `json:"lineNumber" bson:"lineNumber" validate:"gte=1,lte=1000"`
	SupplierItemID          primitive.ObjectID `json:"supplierItemId" bson:"supplierItemId" validate:"required"`
	SupplierItemVersion     int64              `json:"supplierItemVersion" bson:"supplierItemVersion" validate:"gte=1"`
	SupplierCode            string             `json:"supplierCode" bson:"supplierCode" validate:"required,location_code,max=64"`
	SupplierName            string             `json:"supplierName" bson:"supplierName" validate:"required,not_blank,max=160"`
	ItemID                  primitive.ObjectID `json:"itemId" bson:"itemId" validate:"required"`
	ItemSKU                 string             `json:"itemSku" bson:"itemSku" validate:"required,not_blank,max=64"`
	ItemName                string             `json:"itemName" bson:"itemName" validate:"required,not_blank,max=160"`
	SupplierItemCode        string             `json:"supplierItemCode" bson:"supplierItemCode" validate:"required,not_blank,max=100"`
	Description             string             `json:"description" bson:"description" validate:"required,not_blank,max=2000"`
	RequestedQuantityMicros int64              `json:"requestedQuantityMicros,string" bson:"requestedQuantityMicros" validate:"gt=0"`
	OrderedQuantityMicros   int64              `json:"orderedQuantityMicros,string" bson:"orderedQuantityMicros" validate:"gt=0"`
	PackSizeMicros          int64              `json:"packSizeMicros,string" bson:"packSizeMicros" validate:"gt=0"`
	MOQ                     int32              `json:"moq" bson:"moq" validate:"gte=1,lte=1000000"`
	OrderedPacks            int64              `json:"orderedPacks" bson:"orderedPacks" validate:"gt=0"`
	ReceivedQuantityMicros  int64              `json:"receivedQuantityMicros,string" bson:"receivedQuantityMicros" validate:"gte=0,quantity_micros"`
	RoundingDeltaMicros     int64              `json:"roundingDeltaMicros,string" bson:"roundingDeltaMicros" validate:"gte=0"`
	UnitPriceMinor          int64              `json:"unitPriceMinor,string" bson:"unitPriceMinor" validate:"gte=0"`
	LineTotalMinor          int64              `json:"lineTotalMinor,string" bson:"lineTotalMinor" validate:"gte=0"`
	Currency                string             `json:"currency" bson:"currency" validate:"required,currency_code"`
	ItemBaseUnitID          primitive.ObjectID `json:"itemBaseUnitId" bson:"itemBaseUnitId" validate:"required"`
	ItemBaseUnitCode        string             `json:"itemBaseUnitCode" bson:"itemBaseUnitCode" validate:"required,location_code,max=32"`
	OrderedUnitID           primitive.ObjectID `json:"orderedUnitId" bson:"orderedUnitId" validate:"required"`
	OrderedUnitCode         string             `json:"orderedUnitCode" bson:"orderedUnitCode" validate:"required,location_code,max=32"`
	CreatedAt               time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt               time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

// GoodsReceipt is the immutable receiving record. Corrections are represented
// by an inventory reversal and a cancelled receipt; the original document and
// its line snapshots are never edited.
type GoodsReceipt struct {
	ID                     primitive.ObjectID        `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID               primitive.ObjectID        `json:"-" bson:"tenantId" validate:"required"`
	LocationID             primitive.ObjectID        `json:"locationId" bson:"locationId" validate:"required"`
	StorageAreaID          primitive.ObjectID        `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	PurchaseOrderID        primitive.ObjectID        `json:"purchaseOrderId" bson:"purchaseOrderId" validate:"required"`
	OrderNumberSnapshot    string                    `json:"orderNumberSnapshot" bson:"orderNumberSnapshot" validate:"required,location_code,max=64"`
	OrderVersionSnapshot   int64                     `json:"orderVersionSnapshot" bson:"orderVersionSnapshot" validate:"gte=1"`
	SupplierID             primitive.ObjectID        `json:"supplierId" bson:"supplierId" validate:"required"`
	ReceivedBy             primitive.ObjectID        `json:"receivedBy" bson:"receivedBy" validate:"required"`
	ReceivedAt             time.Time                 `json:"receivedAt" bson:"receivedAt" validate:"required"`
	Status                 GoodsReceiptStatus        `json:"status" bson:"status" validate:"required,goods_receipt_status"`
	Audit                  []PurchaseOrderAuditEntry `json:"audit" bson:"audit" validate:"required,max=100,dive"`
	IdempotencyKey         string                    `json:"-" bson:"idempotencyKey" validate:"required,min=8,max=128"`
	RequestHash            string                    `json:"-" bson:"requestHash" validate:"required,len=64,sha256_hex"`
	PostingID              primitive.ObjectID        `json:"postingId" bson:"postingId" validate:"required"`
	ReversalPostingID      *primitive.ObjectID       `json:"reversalPostingId,omitempty" bson:"reversalPostingId,omitempty"`
	ReversalIdempotencyKey string                    `json:"-" bson:"reversalIdempotencyKey,omitempty" validate:"omitempty,min=8,max=128"`
	ReversalRequestHash    string                    `json:"-" bson:"reversalRequestHash,omitempty" validate:"omitempty,len=64,sha256_hex"`
	CreatedAt              time.Time                 `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt              time.Time                 `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

type GoodsReceiptStatus string

const (
	GoodsReceiptPosted    GoodsReceiptStatus = "posted"
	GoodsReceiptCancelled GoodsReceiptStatus = "cancelled"
)

func ValidGoodsReceiptStatus(value GoodsReceiptStatus) bool {
	return value == GoodsReceiptPosted || value == GoodsReceiptCancelled
}

// GoodsReceiptLine copies all order terms needed to audit receiving without
// consulting mutable supplier/catalog records.
type GoodsReceiptLine struct {
	ID                       primitive.ObjectID  `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID                 primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID               primitive.ObjectID  `json:"locationId" bson:"locationId" validate:"required"`
	GoodsReceiptID           primitive.ObjectID  `json:"goodsReceiptId" bson:"goodsReceiptId" validate:"required"`
	PurchaseOrderID          primitive.ObjectID  `json:"purchaseOrderId" bson:"purchaseOrderId" validate:"required"`
	PurchaseOrderLineID      primitive.ObjectID  `json:"purchaseOrderLineId" bson:"purchaseOrderLineId" validate:"required"`
	LineNumber               int32               `json:"lineNumber" bson:"lineNumber" validate:"gte=1,lte=1000"`
	ItemID                   primitive.ObjectID  `json:"itemId" bson:"itemId" validate:"required"`
	ItemSKU                  string              `json:"itemSku" bson:"itemSku" validate:"required,not_blank,max=64"`
	ItemName                 string              `json:"itemName" bson:"itemName" validate:"required,not_blank,max=160"`
	Description              string              `json:"description" bson:"description" validate:"required,not_blank,max=2000"`
	SupplierCode             string              `json:"supplierCode" bson:"supplierCode" validate:"required,location_code,max=64"`
	SupplierName             string              `json:"supplierName" bson:"supplierName" validate:"required,not_blank,max=160"`
	SupplierItemCode         string              `json:"supplierItemCode" bson:"supplierItemCode" validate:"required,not_blank,max=100"`
	SupplierItemVersion      int64               `json:"supplierItemVersion" bson:"supplierItemVersion" validate:"gte=1"`
	ItemBaseUnitCode         string              `json:"itemBaseUnitCode" bson:"itemBaseUnitCode" validate:"required,location_code,max=32"`
	OrderedUnitCode          string              `json:"orderedUnitCode" bson:"orderedUnitCode" validate:"required,location_code,max=32"`
	OrderedQuantityMicros    int64               `json:"orderedQuantityMicros,string" bson:"orderedQuantityMicros" validate:"gt=0"`
	PreviouslyReceivedMicros int64               `json:"previouslyReceivedMicros,string" bson:"previouslyReceivedMicros" validate:"gte=0,quantity_micros"`
	ReceivedQuantityMicros   int64               `json:"receivedQuantityMicros,string" bson:"receivedQuantityMicros" validate:"gt=0,quantity_micros"`
	QuantityVarianceMicros   int64               `json:"quantityVarianceMicros,string" bson:"quantityVarianceMicros" validate:"quantity_micros"`
	OrderedUnitPriceMinor    int64               `json:"orderedUnitPriceMinor,string" bson:"orderedUnitPriceMinor" validate:"gte=0"`
	ActualUnitPriceMinor     int64               `json:"actualUnitPriceMinor,string" bson:"actualUnitPriceMinor" validate:"gte=0"`
	PriceVarianceMinor       int64               `json:"priceVarianceMinor,string" bson:"priceVarianceMinor" validate:"quantity_micros"`
	Currency                 string              `json:"currency" bson:"currency" validate:"required,currency_code"`
	VarianceReason           string              `json:"varianceReason,omitempty" bson:"varianceReason,omitempty" validate:"omitempty,max=500,not_blank"`
	StorageAreaID            primitive.ObjectID  `json:"storageAreaId" bson:"storageAreaId" validate:"required"`
	LotCode                  string              `json:"lotCode,omitempty" bson:"lotCode,omitempty" validate:"omitempty,max=128,not_blank"`
	LotID                    *primitive.ObjectID `json:"lotId,omitempty" bson:"lotId,omitempty"`
	ExpiresAt                *time.Time          `json:"expiresAt,omitempty" bson:"expiresAt,omitempty"`
	CreatedAt                time.Time           `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt                time.Time           `json:"updatedAt" bson:"updatedAt" validate:"required"`
}

// DeliveryCalendarRule is a location-specific supplier delivery weekday.
// Weekday uses ISO-8601 numbering: Monday=1 through Sunday=7.
type DeliveryCalendarRule struct {
	ID            primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID      primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID    primitive.ObjectID `json:"locationId" bson:"locationId" validate:"required"`
	SupplierID    primitive.ObjectID `json:"supplierId" bson:"supplierId" validate:"required"`
	Weekday       int32              `json:"weekday" bson:"weekday" validate:"gte=1,lte=7"`
	CutoffMinutes int32              `json:"cutoffMinutes" bson:"cutoffMinutes" validate:"gte=0,lte=1439"`
	LeadTimeDays  int32              `json:"leadTimeDays" bson:"leadTimeDays" validate:"gte=0,lte=3650"`
	IsActive      bool               `json:"isActive" bson:"isActive"`
	Version       int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt     time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt     time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
