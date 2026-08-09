package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Location is a tenant-owned operating location. TenantID and LimitSlot are
// persistence details and must never cross the product API boundary.
type Location struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	TenantID  primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	Code      string             `json:"code" bson:"code" validate:"required,min=1,max=64,location_code"`
	Name      string             `json:"name" bson:"name" validate:"required,min=1,max=200"`
	Timezone  string             `json:"timezone" bson:"timezone" validate:"required,max=100,iana_timezone"`
	IsActive  bool               `json:"isActive" bson:"isActive"`
	Version   int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
	LimitSlot int64              `json:"-" bson:"limitSlot" validate:"gte=1"`
}
