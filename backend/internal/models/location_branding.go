package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// LocationBranding contains safe presentation overrides for one location.
// Empty visual tokens inherit tenant branding and then platform defaults.
type LocationBranding struct {
	ID           *primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty" validate:"omitempty"`
	TenantID     primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	LocationID   primitive.ObjectID  `json:"-" bson:"locationId" validate:"required"`
	DisplayName  string              `json:"displayName" bson:"displayName" validate:"omitempty,max=200"`
	PrimaryColor string              `json:"primaryColor" bson:"primaryColor" validate:"omitempty,hex_color"`
	AccentColor  string              `json:"accentColor" bson:"accentColor" validate:"omitempty,hex_color"`
	Font         BrandingFont        `json:"font" bson:"font" validate:"branding_font"`
	Version      int64               `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt    time.Time           `json:"createdAt,omitempty" bson:"createdAt" validate:"required"`
	UpdatedAt    time.Time           `json:"updatedAt,omitempty" bson:"updatedAt" validate:"required"`
}

type ResolvedLocationBranding struct {
	LocationID              primitive.ObjectID `json:"locationId"`
	DisplayName             string             `json:"displayName"`
	PrimaryColor            string             `json:"primaryColor"`
	AccentColor             string             `json:"accentColor"`
	Font                    BrandingFont       `json:"font"`
	LocationBrandingVersion int64              `json:"locationBrandingVersion"`
	TenantBrandingVersion   int64              `json:"tenantBrandingVersion"`
	Sources                 map[string]string  `json:"sources"`
}
