package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BrandingFont string

const (
	BrandingFontSystem    BrandingFont = "system"
	BrandingFontHumanist  BrandingFont = "humanist"
	BrandingFontGeometric BrandingFont = "geometric"
	BrandingFontSerif     BrandingFont = "serif"
)

func ValidBrandingFont(value BrandingFont) bool {
	switch value {
	case "", BrandingFontSystem, BrandingFontHumanist, BrandingFontGeometric, BrandingFontSerif:
		return true
	default:
		return false
	}
}

// TenantBranding contains only restricted presentation tokens. Privileged
// platform branding content and uploaded assets use separate models.
type TenantBranding struct {
	ID           *primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty" validate:"omitempty"`
	TenantID     primitive.ObjectID  `json:"-" bson:"tenantId" validate:"required"`
	PrimaryColor string              `json:"primaryColor" bson:"primaryColor" validate:"omitempty,hex_color"`
	AccentColor  string              `json:"accentColor" bson:"accentColor" validate:"omitempty,hex_color"`
	Font         BrandingFont        `json:"font" bson:"font" validate:"branding_font"`
	Version      int64               `json:"version" bson:"version" validate:"gte=1"`
	CreatedAt    time.Time           `json:"createdAt,omitempty" bson:"createdAt" validate:"required"`
	UpdatedAt    time.Time           `json:"updatedAt,omitempty" bson:"updatedAt" validate:"required"`
}
