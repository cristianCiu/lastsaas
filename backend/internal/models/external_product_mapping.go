package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ExternalProductMapping points an adapter's product identity at the exact
// recipe version that was valid when the product was sold.
type ExternalProductMapping struct {
	ID                primitive.ObjectID `json:"id" bson:"_id,omitempty" validate:"required"`
	TenantID          primitive.ObjectID `json:"-" bson:"tenantId" validate:"required"`
	Adapter           string             `json:"adapter" bson:"adapter" validate:"required,max=64,location_code"`
	ExternalProductID string             `json:"externalProductId" bson:"externalProductId" validate:"required,not_blank,max=256"`
	RecipeVersionID   primitive.ObjectID `json:"recipeVersionId" bson:"recipeVersionId" validate:"required"`
	EffectiveFrom     time.Time          `json:"effectiveFrom" bson:"effectiveFrom" validate:"required"`
	EffectiveTo       *time.Time         `json:"effectiveTo,omitempty" bson:"effectiveTo,omitempty"`
	IsActive          bool               `json:"isActive" bson:"isActive"`
	Version           int64              `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt         time.Time          `json:"createdAt" bson:"createdAt" validate:"required"`
	UpdatedAt         time.Time          `json:"updatedAt" bson:"updatedAt" validate:"required"`
}
