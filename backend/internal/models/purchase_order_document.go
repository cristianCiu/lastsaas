package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PurchaseOrderEmailDelivery is a delivery claim, not a copy of the
// document. The order and its materialized lines remain the source snapshot.
// A unique tenant/order/version key makes an explicit send idempotent.
type PurchaseOrderEmailDelivery struct {
	ID              primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID        primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	LocationID      primitive.ObjectID `json:"-" bson:"locationId" validate:"required"`
	PurchaseOrderID primitive.ObjectID `json:"purchaseOrderId" bson:"purchaseOrderId" validate:"required"`
	OrderVersion    int64              `json:"orderVersion" bson:"orderVersion" validate:"gte=1"`
	RecipientEmail  string             `json:"-" bson:"recipientEmail" validate:"required,email,max=254"`
	IdempotencyKey  string             `json:"-" bson:"idempotencyKey" validate:"required,min=8,max=128"`
	RequestHash     string             `json:"-" bson:"requestHash" validate:"required,len=64,sha256_hex"`
	Status          string             `json:"status" bson:"status" validate:"required,oneof=pending sent"`
	ClaimedAt       time.Time          `json:"claimedAt" bson:"claimedAt" validate:"required"`
	SentAt          *time.Time         `json:"sentAt,omitempty" bson:"sentAt,omitempty" validate:"omitempty"`
}
